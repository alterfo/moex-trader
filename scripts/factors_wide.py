import os
import sys
import json
import numpy as np
import pandas as pd
import lightgbm as lgb

COST = 0.0015
BORROW = 0.00005
REBAL = 21
HALT_FROM = pd.Timestamp("2022-02-25")
HALT_TO = pd.Timestamp("2022-03-24")
POST = pd.Timestamp("2022-06-01")
MIN_VALUE = 30e6
FIRST_TEST = pd.Timestamp("2019-01-01")


def load(path):
    px = pd.read_csv(path, parse_dates=["date"])
    if "value" in px.columns:
        px = px[px["close"].notna() & (px["close"] > 0) & px["open"].notna() & (px["open"] > 0)]
        counts = px.groupby("date")["ticker"].count()
        cal = counts[counts >= 30].index
        px = px[px["date"].isin(cal)].copy()
        px["volume"] = px["value"] / px["close"]
    else:
        px = px[px["close"] > 0]
        cal = px[px.ticker == "IMOEX"]["date"].sort_values().unique()
        px = px[px["date"].isin(cal)]
    exclude = os.environ.get("FW_EXCLUDE")
    if exclude:
        drop = set(open(exclude).read().replace("\n", ",").split(","))
        px = px[~px["ticker"].isin(drop)]
    op = px.pivot(index="date", columns="ticker", values="open").sort_index()
    cl = px.pivot(index="date", columns="ticker", values="close").sort_index()
    vo = px.pivot(index="date", columns="ticker", values="volume").sort_index()
    return op, cl, vo


def dividends_matrix(path, idx, columns):
    div = pd.DataFrame(0.0, index=idx, columns=columns)
    if not path:
        return div
    with open(path) as fh:
        for line in fh:
            line = line.strip()
            if not line:
                continue
            r = json.loads(line)
            t = r["ticker"]
            if t not in div.columns:
                continue
            try:
                net = float(r["dividend_net"])
            except (TypeError, ValueError):
                continue
            d = pd.Timestamp(r["last_buy_date"][:10])
            pos = idx.searchsorted(d, side="right")
            if pos < len(idx):
                div.iloc[pos, div.columns.get_loc(t)] += net
    return div


def adjust(op, cl, div=None):
    prev = cl.shift(1)
    if div is None:
        div = pd.DataFrame(0.0, index=cl.index, columns=cl.columns)
    ret_price = cl / prev - 1
    bad = (ret_price < -0.45) | (ret_price > 0.8)
    div = div.where(div <= 0.35 * prev, 0.0)
    gap = ((op + div) / prev).where(~bad, 1.0)
    intra = (cl / op).where(~bad, 1.0)
    factor = (gap * intra).fillna(1.0)
    adj_cl = factor.cumprod().where(cl.notna())
    adj_op = adj_cl.shift(1) * gap
    ret = (adj_cl / adj_cl.shift(1) - 1).where(cl.notna() & prev.notna())
    return adj_op, adj_cl, ret


def signals(adj_cl, cl, vo, ret):
    value = cl * vo
    s = {}
    s["mom_12_1"] = adj_cl.shift(21) / adj_cl.shift(252) - 1
    s["rev_1m"] = -(adj_cl / adj_cl.shift(21) - 1)
    s["low_vol"] = -ret.rolling(60, min_periods=50).std()
    s["high52"] = adj_cl / adj_cl.rolling(252, min_periods=240).max()
    s["low_max"] = -ret.rolling(21, min_periods=18).max()
    elig = (cl.notna().rolling(252).sum() >= 252) & (value.rolling(60, min_periods=50).median() >= MIN_VALUE)
    return s, elig


def zscore_rows(df):
    mu = df.mean(axis=1)
    sd = df.std(axis=1).replace(0, np.nan)
    return df.sub(mu, axis=0).div(sd, axis=0)


def nw_t(x, lags):
    x = np.asarray(x, float)
    x = x[~np.isnan(x)]
    n = len(x)
    if n < 10:
        return float("nan")
    m = x.mean()
    d = x - m
    g0 = (d * d).sum() / n
    v = g0
    for l in range(1, lags + 1):
        w = 1 - l / (lags + 1)
        v += 2 * w * (d[l:] * d[:-l]).sum() / n
    return float(m / np.sqrt(v / n)) if v > 0 else float("nan")


def summarize(r, dates, ppy, lags):
    r = pd.Series(np.asarray(r, float), index=pd.DatetimeIndex(dates)).dropna()
    r = r[~((r.index >= HALT_FROM) & (r.index <= HALT_TO))]
    if len(r) < 10:
        return dict(n=len(r))
    yearly = r.groupby(r.index.year).agg(["sum", "count"])
    yearly = yearly[yearly["count"] >= 6]
    post = r[r.index >= POST]
    eq = (1 + r).cumprod()
    dd = (eq / eq.cummax() - 1).min()
    return dict(n=int(len(r)), mean=float(r.mean()), t_nw=nw_t(r.values, lags), sharpe=float(r.mean() / r.std() * np.sqrt(ppy)) if r.std() > 0 else float("nan"),
                pos_years=int((yearly["sum"] > 0).sum()), years=int(len(yearly)), post_mean=float(post.mean()) if len(post) else float("nan"), post_n=int(len(post)),
                maxdd=float(dd), total=float(eq.iloc[-1] - 1))


def periods(idx, step):
    pos = list(range(300, len(idx) - step - 2, step))
    return pos


def portfolio(score, elig, adj_op, adj_cl, cl, idx, positions, mode, step=REBAL):
    rows = []
    adj_cl_ff = adj_cl.ffill()
    prev_long, prev_short = set(), set()
    for p in positions:
        d = idx[p]
        if d < pd.Timestamp("2016-01-01"):
            continue
        sc = score.iloc[p]
        ok = elig.iloc[p] & sc.notna()
        names = sc[ok].index
        if len(names) < 30:
            continue
        ranked = sc[names].sort_values()
        k = max(1, len(names) // 5)
        top = list(ranked.index[-k:])
        bot = list(ranked.index[:k])
        entry = adj_op.iloc[p + 1].fillna(adj_cl.iloc[p])
        fwd = (adj_cl_ff.iloc[p + step] / entry - 1)
        fwd = fwd.reindex(names).fillna(0.0)
        r_top = float(fwd[top].mean())
        r_bot = float(fwd[bot].mean())
        r_all = float(fwd.mean())
        cal_days = (idx[p + step] - idx[p]).days
        top_s, bot_s = set(top), set(bot)
        tn_long = (1.0 if not prev_long else len(top_s ^ prev_long) / len(top_s))
        tn_short = (1.0 if not prev_short else len(bot_s ^ prev_short) / len(bot_s))
        prev_long, prev_short = top_s, bot_s
        if mode == "ls":
            gross = r_top - r_bot
            cost = COST * (tn_long + tn_short) + BORROW * cal_days
        else:
            gross = r_top - r_all
            cost = COST * tn_long
        rows.append((d, gross, gross - cost, tn_long, tn_short, len(names), r_top, r_bot, r_all, cal_days))
    return pd.DataFrame(rows, columns=["date", "gross", "net", "tn_long", "tn_short", "n", "r_top", "r_bot", "r_all", "cal_days"]).set_index("date")


class MlpEnsemble:
    def __init__(self, n=5):
        from sklearn.neural_network import MLPRegressor
        from sklearn.preprocessing import StandardScaler
        self.scaler = StandardScaler()
        self.members = [MLPRegressor(hidden_layer_sizes=(32, 16), alpha=1e-3, early_stopping=True, n_iter_no_change=5, max_iter=60, random_state=i) for i in range(n)]

    def fit(self, X, y):
        Xs = self.scaler.fit_transform(X)
        for m in self.members:
            m.fit(Xs, y)
        return self

    def predict(self, X):
        Xs = self.scaler.transform(X)
        return np.mean([m.predict(Xs) for m in self.members], axis=0)


def fit_model(kind, X, y):
    if kind == "ridge":
        from sklearn.linear_model import Ridge
        from sklearn.pipeline import make_pipeline
        from sklearn.preprocessing import StandardScaler
        return make_pipeline(StandardScaler(), Ridge(alpha=10.0)).fit(X, y)
    if kind == "mlp":
        return MlpEnsemble().fit(X, y)
    return lgb.LGBMRegressor(n_estimators=300, learning_rate=0.03, num_leaves=15, colsample_bytree=0.8, subsample=0.8, subsample_freq=1, random_state=42, verbose=-1, n_jobs=6).fit(X, y)


def ml_scores(feats, target, idx, positions, step=REBAL, kind="lgb"):
    out = pd.DataFrame(np.nan, index=feats[list(feats)[0]].index, columns=feats[list(feats)[0]].columns)
    names = list(feats)
    last_fit = None
    model = None
    train_pos = list(range(300, len(idx) - step - 2, 5))
    for p in positions:
        d = idx[p]
        if d < FIRST_TEST:
            continue
        if model is None or (last_fit is not None and p - last_fit >= 63):
            usable = [q for q in train_pos if q + step + 1 < p - 1]
            X, y = [], []
            for q in usable:
                f = pd.concat([feats[n].iloc[q] for n in names], axis=1)
                t = target.iloc[q]
                m = f.notna().all(axis=1) & t.notna()
                X.append(f[m].to_numpy(float))
                y.append(t[m].to_numpy(float))
            X, y = np.vstack(X), np.concatenate(y)
            model = fit_model(kind, X, y)
            last_fit = p
        f = pd.concat([feats[n].iloc[p] for n in names], axis=1)
        m = f.notna().all(axis=1)
        if not m.any():
            continue
        out.iloc[p, [out.columns.get_loc(c) for c in f[m].index]] = model.predict(f[m].to_numpy(float))
    return out


def run_factors(path, outpath, divpath=None):
    op, cl, vo = load(path)
    idx = cl.index
    adj_op, adj_cl, ret = adjust(op, cl, dividends_matrix(divpath, idx, cl.columns))
    s, elig = signals(adj_cl, cl, vo, ret)
    elig = elig.reindex(columns=[c for c in cl.columns if c != "IMOEX"]).fillna(False)
    cols = elig.columns
    z = {k: zscore_rows(v[cols].where(elig)) for k, v in s.items()}
    composite = sum(z.values()) / len(z)
    positions = periods(idx, REBAL)
    res = {}
    for name, mode in (("V1", "ls"), ("V2", "lo")):
        pf = portfolio(composite, elig, adj_op[cols], adj_cl[cols], cl[cols], idx, positions, mode)
        res[name] = dict(summary=summarize(pf["net"], pf.index, 252 / REBAL, 3), gross=summarize(pf["gross"], pf.index, 252 / REBAL, 3),
                         turnover_long=float(pf["tn_long"].mean()), turnover_short=float(pf["tn_short"].mean()), names=float(pf["n"].mean()))
        pf.to_csv(outpath + f".{name}.csv")
    feats = {}
    for k, v in z.items():
        feats[k] = v
        feats[k + "_rk"] = v.rank(axis=1, pct=True)
    fwd = (adj_cl.shift(-REBAL) / adj_op.shift(-1) - 1)[cols].where(elig)
    target = fwd.rank(axis=1, pct=True)
    ml = ml_scores(feats, target, idx, positions)
    for name, mode in (("V3", "ls"), ("V4", "lo")):
        pf = portfolio(ml, elig, adj_op[cols], adj_cl[cols], cl[cols], idx, positions, mode)
        res[name] = dict(summary=summarize(pf["net"], pf.index, 252 / REBAL, 3), gross=summarize(pf["gross"], pf.index, 252 / REBAL, 3),
                         turnover_long=float(pf["tn_long"].mean()), turnover_short=float(pf["tn_short"].mean()), names=float(pf["n"].mean()))
        pf.to_csv(outpath + f".{name}.csv")
    positions10 = periods(idx, 10)
    for name, mode in (("V1_10", "ls"), ("V2_10", "lo")):
        pf = portfolio(composite, elig, adj_op[cols], adj_cl[cols], cl[cols], idx, positions10, mode, step=10)
        res[name] = dict(summary=summarize(pf["net"], pf.index, 252 / 10, 3), gross=summarize(pf["gross"], pf.index, 252 / 10, 3), turnover_long=float(pf["tn_long"].mean()))
    attrib = {}
    for k, v in z.items():
        for mode in ("ls", "lo"):
            pf = portfolio(v, elig, adj_op[cols], adj_cl[cols], cl[cols], idx, positions, mode)
            attrib[k + "_" + mode] = summarize(pf["net"], pf.index, 252 / REBAL, 3)
    res["attribution"] = attrib
    json.dump(res, open(outpath + ".json", "w"), indent=1, default=float)
    for k, v in attrib.items():
        print("attrib", k, json.dumps({a: v.get(a) for a in ("mean", "t_nw", "sharpe", "pos_years", "years", "post_mean")}, default=float))
    for k, v in res.items():
        if k == "attribution":
            continue
        print(k, json.dumps(v["summary"], default=float))
        print("  gross", json.dumps(v["gross"], default=float), "tn_long", round(v["turnover_long"], 3))


def run_overnight(path, outpath, divpath=None):
    op, cl, vo = load(path)
    adj_op, adj_cl, ret = adjust(op, cl, dividends_matrix(divpath, cl.index, cl.columns))
    prev = adj_cl.shift(1)
    over = adj_op / prev - 1
    intra = adj_cl / adj_op - 1
    value = cl * vo
    elig = (cl.notna().rolling(252).sum() >= 252) & (value.rolling(60, min_periods=50).median() >= MIN_VALUE)
    cols = [c for c in cl.columns if c != "IMOEX"]
    over = over[cols].where(elig[cols])
    intra = intra[cols].where(elig[cols])
    over = over.mask((over.abs() > 0.3))
    ov = over.mean(axis=1)
    it = intra.mean(axis=1)
    keep = ~((ov.index >= HALT_FROM) & (ov.index <= HALT_TO))
    df = pd.DataFrame({"overnight": ov[keep], "intraday": it[keep]}).dropna()
    g = df.groupby(df.index.year).mean()
    g["n"] = df.groupby(df.index.year).size()
    print(g.round(5).to_string())
    print("all", df.mean().round(5).to_dict(), "t_nw overnight", round(nw_t(df["overnight"], 5), 2), "intraday", round(nw_t(df["intraday"], 5), 2))
    g.to_csv(outpath + ".csv")


def pair_list(op):
    cols = set(op.columns)
    return sorted((c[:-1], c) for c in cols if c.endswith("P") and c[:-1] in cols)


def run_pairs(path, outpath):
    op, cl, vo = load(path)
    idx = cl.index
    adj_op, adj_cl, ret = adjust(op, cl)
    pairs = [(a, b) for a, b in pair_list(op) if cl[a].notna().sum() >= 1000 and cl[b].notna().sum() >= 1000]
    print("pairs", pairs, flush=True)
    daily = {}
    for a, b in pairs:
        s = np.log(adj_cl[a] / adj_cl[b])
        mu = s.rolling(60, min_periods=60).mean()
        sd = s.rolling(60, min_periods=60).std()
        z = (s - mu) / sd
        over_a = adj_op[a] / adj_cl[a].shift(1) - 1
        over_b = adj_op[b] / adj_cl[b].shift(1) - 1
        in_a = adj_cl[a] / adj_op[a] - 1
        in_b = adj_cl[b] / adj_op[b] - 1
        n = len(idx)
        held = 0
        age = 0
        pnl = np.full(n, np.nan)
        valid = (adj_cl[a].notna() & adj_cl[b].notna() & adj_op[a].notna() & adj_op[b].notna()).to_numpy()
        zv = z.to_numpy()
        oa, ob, ia, ib = over_a.to_numpy(), over_b.to_numpy(), in_a.to_numpy(), in_b.to_numpy()
        target_prev = 0
        for t in range(1, n):
            if not valid[t]:
                continue
            held_open = target_prev
            r = 0.0
            if held_open != 0 and not np.isnan(oa[t]) and not np.isnan(ob[t]):
                r += held_open * (oa[t] - ob[t])
            target = target_prev
            zt_prev = zv[t - 1]
            if not np.isnan(zt_prev):
                if target_prev == 0:
                    if zt_prev < -2:
                        target = 1
                    elif zt_prev > 2:
                        target = -1
                    age = 0
                else:
                    age += 1
                    if abs(zt_prev) < 0.5 or age >= 30 or abs(zt_prev) > 4:
                        target = 0
            exec_cost = COST * 2 * abs(target - held_open)
            if target != 0 and not np.isnan(ia[t]) and not np.isnan(ib[t]):
                r += target * (ia[t] - ib[t])
            if target != 0 or held_open != 0:
                r -= BORROW * 1.4 * (1 if target != 0 else 0.5)
            r -= exec_cost
            pnl[t] = r
            target_prev = target
        daily[f"{a}/{b}"] = pd.Series(pnl, index=idx)
    d = pd.DataFrame(daily)
    d = d[(d.index >= pd.Timestamp("2016-01-01"))]
    port = d.mean(axis=1, skipna=True).fillna(0.0)
    port = port[~((port.index >= HALT_FROM) & (port.index <= HALT_TO))]
    res = dict(portfolio=summarize(port, port.index, 252, 5), pairs={c: summarize(d[c].dropna(), d[c].dropna().index, 252, 5) for c in d.columns})
    json.dump(res, open(outpath + ".json", "w"), indent=1, default=float)
    print("portfolio", json.dumps(res["portfolio"], default=float))
    for c, v in res["pairs"].items():
        print(c, json.dumps({k: v.get(k) for k in ("n", "mean", "t_nw", "sharpe", "total")}, default=float))


def run_attrib(path, outpath, divpath=None):
    op, cl, vo = load(path)
    idx = cl.index
    adj_op, adj_cl, ret = adjust(op, cl, dividends_matrix(divpath, idx, cl.columns))
    s, elig = signals(adj_cl, cl, vo, ret)
    cols = [c for c in cl.columns if c != "IMOEX"]
    elig = elig.reindex(columns=cols).fillna(False)
    z = {k: zscore_rows(v[cols].where(elig)) for k, v in s.items()}
    positions = periods(idx, REBAL)
    cut = pd.Timestamp("2022-02-25")
    combos = dict(z)
    combos["V1_composite"] = sum(z.values()) / len(z)
    combos["C2"] = (z["mom_12_1"] - z["rev_1m"] + z["high52"]) / 3
    combos["mom_1m"] = -z["rev_1m"]
    for k, v in combos.items():
        for mode in ("ls", "lo"):
            pf = portfolio(v, elig, adj_op[cols], adj_cl[cols], cl[cols], idx, positions, mode)
            a = pf[pf.index < cut]["net"]
            b = pf[pf.index >= POST]["net"]
            t = summarize(pf["net"], pf.index, 252 / REBAL, 3)
            print(k, mode, "all %.4f t=%.2f sh=%.2f yrs %d/%d | pre %.4f t=%.2f n=%d | post %.4f t=%.2f n=%d | names %.1f" % (t["mean"], t["t_nw"], t["sharpe"], t["pos_years"], t["years"], a.mean(), nw_t(a.values, 3), len(a), b.mean(), nw_t(b.values, 3), len(b), pf["n"].mean()))


def run_absolute(path, outpath, divpath=None):
    op, cl, vo = load(path)
    idx = cl.index
    adj_op, adj_cl, ret = adjust(op, cl, dividends_matrix(divpath, idx, cl.columns))
    s, elig = signals(adj_cl, cl, vo, ret)
    cols = [c for c in cl.columns if c != "IMOEX"]
    elig = elig.reindex(columns=cols).fillna(False)
    z = {k: zscore_rows(v[cols].where(elig)) for k, v in s.items()}
    combos = {"high52": z["high52"], "C2": (z["mom_12_1"] - z["rev_1m"] + z["high52"]) / 3, "mom_12_1": z["mom_12_1"]}
    positions = periods(idx, REBAL)
    kr = pd.read_csv("keyrate.csv", parse_dates=[0], header=0)
    kr.columns = ["date", "rate"]
    kr = kr.set_index("date")["rate"].sort_index()
    for k, v in combos.items():
        pf = portfolio(v, elig, adj_op[cols], adj_cl[cols], cl[cols], idx, positions, "lo")
        pf = pf[~((pf.index >= HALT_FROM) & (pf.index <= HALT_TO))]
        top = pf["r_top"] - COST * pf["tn_long"]
        uni = pf["r_all"]
        carry = pd.Series([float(kr[:d].iloc[-1]) / 100 / 365 * c if len(kr[:d]) else np.nan for d, c in zip(pf.index, pf["cal_days"])], index=pf.index)
        rows = pd.DataFrame({"top_net": top, "universe": uni, "carry": carry})
        yr = rows.groupby(rows.index.year).apply(lambda g: pd.Series({"top_net": (1 + g.top_net).prod() - 1, "universe": (1 + g.universe).prod() - 1, "carry": (1 + g.carry.fillna(0)).prod() - 1, "n": len(g)}))
        print(k)
        print(yr.round(3).to_string())
        print("compounded total top_net %.2f universe %.2f carry %.2f" % ((1 + rows.top_net).prod() - 1, (1 + rows.universe).prod() - 1, (1 + rows.carry.fillna(0)).prod() - 1))


def run_models(path, outpath, divpath=None):
    op, cl, vo = load(path)
    idx = cl.index
    adj_op, adj_cl, ret = adjust(op, cl, dividends_matrix(divpath, idx, cl.columns))
    s, elig = signals(adj_cl, cl, vo, ret)
    cols = [c for c in cl.columns if c != "IMOEX"]
    elig = elig.reindex(columns=cols).fillna(False)
    z = {k: zscore_rows(v[cols].where(elig)) for k, v in s.items()}
    feats = {}
    for k, v in z.items():
        feats[k] = v
        feats[k + "_rk"] = v.rank(axis=1, pct=True)
    fwd = (adj_cl.shift(-REBAL) / adj_op.shift(-1) - 1)[cols].where(elig)
    target = fwd.rank(axis=1, pct=True)
    positions = periods(idx, REBAL)
    res = {}
    for kind in os.environ.get("FW_KINDS", "ridge,mlp").split(","):
        ml = ml_scores(feats, target, idx, positions, kind=kind)
        for suffix, mode in (("ls", "ls"), ("lo", "lo")):
            pf = portfolio(ml, elig, adj_op[cols], adj_cl[cols], cl[cols], idx, positions, mode)
            pf.to_csv(outpath + "." + kind + "_" + suffix + ".csv")
            t = summarize(pf["net"], pf.index, 252 / REBAL, 3)
            g = summarize(pf["gross"], pf.index, 252 / REBAL, 3)
            res[kind + "_" + suffix] = dict(net=t, gross=g)
            print(kind, suffix, "net mean %.4f t=%.2f sharpe=%.2f yrs %d/%d post %.4f maxdd %.2f | gross t=%.2f" % (t["mean"], t["t_nw"], t["sharpe"], t["pos_years"], t["years"], t["post_mean"], t["maxdd"], g["t_nw"]), flush=True)
    json.dump(res, open(outpath + ".json", "w"), indent=1, default=float)


def run_sequential(path, outpath, divpath=None):
    op, cl, vo = load(path)
    idx = cl.index
    adj_op, adj_cl, ret = adjust(op, cl, dividends_matrix(divpath, idx, cl.columns))
    s, elig = signals(adj_cl, cl, vo, ret)
    cols = [c for c in cl.columns if c != "IMOEX"]
    elig = elig.reindex(columns=cols).fillna(False)
    z = {k: zscore_rows(v[cols].where(elig)) for k, v in s.items()}
    c2 = (z["mom_12_1"] - z["rev_1m"] + z["high52"]) / 3
    positions = periods(idx, REBAL)
    out = {}
    for name, mode in (("C2_ls", "ls"), ("C2_lo", "lo")):
        pf = portfolio(c2, elig, adj_op[cols], adj_cl[cols], cl[cols], idx, positions, mode)
        pf.to_csv(outpath + "." + name + ".csv")
        test = pf[pf.index >= POST]
        ref = pf[pf.index < HALT_FROM]
        out[name] = dict(test=summarize(test["net"], test.index, 252 / REBAL, 3), test_gross=summarize(test["gross"], test.index, 252 / REBAL, 3),
                         ref=summarize(ref["net"], ref.index, 252 / REBAL, 3), turnover_long=float(test["tn_long"].mean()),
                         years={int(y): float(g["net"].sum()) for y, g in test.groupby(test.index.year)})
        print(name, json.dumps(out[name], default=float))
    json.dump(out, open(outpath + ".json", "w"), indent=1, default=float)


if __name__ == "__main__":
    mode, path, out = sys.argv[1], sys.argv[2], sys.argv[3]
    divpath = sys.argv[4] if len(sys.argv) > 4 else None
    if mode == "pairs":
        run_pairs(path, out)
    else:
        {"factors": run_factors, "overnight": run_overnight, "sequential": run_sequential, "attrib": run_attrib, "absolute": run_absolute, "models": run_models}[mode](path, out, divpath)
