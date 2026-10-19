import os
import sys
import json
import numpy as np
import pandas as pd
from scipy.stats import binomtest
from sklearn.decomposition import PCA
from sklearn.linear_model import Ridge
from sklearn.pipeline import make_pipeline
from sklearn.preprocessing import StandardScaler

import factors_wide as fw

STEP26_FIRST = pd.Timestamp("2019-01-01")
IDX_COST = 0.0003


def prepare(snap, divpath, macro):
    op, cl, vo = fw.load(snap)
    idx = cl.index
    adj_op, adj_cl, ret = fw.adjust(op, cl, fw.dividends_matrix(divpath, idx, cl.columns))
    w = pd.read_csv(macro, parse_dates=["date"]).set_index("date").sort_index()
    w = w.reindex(w.index.union(idx)).ffill(limit=6).reindex(idx)
    return op, cl, vo, adj_op, adj_cl, ret, w


def macro_changes(w):
    m = pd.DataFrame(index=w.index)
    cut = pd.Timestamp("2024-06-11")
    usd = np.log(w["USDRUB"].where(w.index <= cut)).diff()
    cny = np.log(w["CNYRUB"]).diff()
    m["usd"] = usd.where(w.index <= cut, cny)
    m["brent"] = np.log(w["BRENT"]).diff()
    m["spx"] = np.log(w["SPX"]).diff()
    m["vix"] = np.log(w["VIX"]).diff()
    m["ust"] = w["UST10"].diff()
    m["dxy"] = np.log(w["DXY"]).diff()
    m["rvi"] = np.log(w["RVI"]).diff()
    m["key"] = w["KEYRATE"].diff()
    m["imoex"] = np.log(w["IMOEX"]).diff()
    return m


def carry_daily(w, idx):
    kr = w["KEYRATE"].ffill().bfill() / 100 / 365
    days = pd.Series(idx, index=idx).diff().dt.days.fillna(1)
    return kr.shift(1).fillna(kr.iloc[0]) * days


def r2(y, p):
    y, p = np.asarray(y), np.asarray(p)
    return 1 - ((y - p) ** 2).sum() / (y ** 2).sum()


SECTORS = ["MOEXOG", "MOEXFN", "MOEXMM", "MOEXTL", "MOEXCN", "MOEXEU", "MOEXCH", "MOEXTN"]


def block_m1(ret, m, out, w=None):
    idx = ret.index
    rows = []
    shares = []
    resid = []
    pcmap = []
    for year in range(2019, 2027):
        start = pd.Timestamp(f"{year}-01-01")
        end = pd.Timestamp(f"{year}-12-31")
        te_idx = idx[(idx >= start) & (idx <= end)]
        tr_idx = idx[idx < start][-504:]
        if len(te_idx) < 20 or len(tr_idx) < 400:
            continue
        cov = ret.loc[tr_idx.union(te_idx)].notna().mean()
        cols = [c for c in ret.columns if cov[c] >= 0.9 and c != "IMOEX"]
        R_tr = ret.loc[tr_idx, cols].fillna(0.0)
        R_te = ret.loc[te_idx, cols].fillna(0.0)
        mk_tr = m.loc[tr_idx, "imoex"].fillna(0.0).to_numpy()
        mk_te = m.loc[te_idx, "imoex"].fillna(0.0).to_numpy()
        macro_cols = ["usd", "brent", "vix", "ust", "rvi", "key"]
        Mac = m[macro_cols].copy()
        Mac["spx_l1"] = m["spx"].shift(1)
        Mac = Mac.fillna(0.0)
        Mtr, Mte = Mac.loc[tr_idx].to_numpy(), Mac.loc[te_idx].to_numpy()
        beta = (R_tr.to_numpy() * mk_tr[:, None]).sum(0) / (mk_tr ** 2).sum()
        res_tr = R_tr.to_numpy() - np.outer(mk_tr, beta)
        pca = PCA(n_components=5).fit(res_tr)
        L = pca.components_.T
        Ftr = res_tr @ L
        res_te_A = R_te.to_numpy() - np.outer(mk_te, beta)
        Fte = res_te_A @ L
        preds = {}
        preds["A"] = np.outer(mk_te, beta)
        def fit_pred(Xtr, Xte):
            mdl = Ridge(alpha=1e-6).fit(Xtr, R_tr.to_numpy())
            return mdl.predict(Xte)
        Xs = {"A": (mk_tr[:, None], mk_te[:, None]),
              "B": (np.column_stack([mk_tr, Ftr]), np.column_stack([mk_te, Fte])),
              "C": (np.column_stack([mk_tr, Mtr]), np.column_stack([mk_te, Mte])),
              "D": (np.column_stack([mk_tr, Ftr, Mtr]), np.column_stack([mk_te, Fte, Mte]))}
        Y = R_te.to_numpy()
        predB = fit_pred(*Xs["B"])
        resid.append(pd.DataFrame(Y - predB, index=te_idx, columns=cols))
        if w is not None:
            sec = np.log(w[SECTORS]).diff().loc[te_idx].fillna(0.0)
            for k in range(5):
                cs = {c: float(np.corrcoef(Fte[:, k], sec[c].to_numpy())[0, 1]) for c in SECTORS}
                best = max(cs, key=lambda c: abs(cs[c]))
                pcmap.append(dict(year=year, pc=k + 1, best=best, corr=cs[best], mkt_corr=float(np.corrcoef(Fte[:, k], mk_te)[0, 1])))
        row = {"year": year, "n_tickers": len(cols), "days": len(te_idx)}
        for k, (a, b) in Xs.items():
            row["r2_" + k] = r2(Y, fit_pred(a, b))
        rows.append(row)
        Cm = Xs["C"]
        mdl = Ridge(alpha=1e-6).fit(Cm[0], R_tr.to_numpy())
        pred_mkt = np.outer(mk_te, beta)
        pred_C = mdl.predict(Cm[1])
        macro_part = pred_C - pred_mkt
        shares.append(dict(year=year, market=float(r2(Y, pred_mkt)), macro_incremental=float(r2(Y, pred_C) - r2(Y, pred_mkt)),
                           stat_incremental=float(r2(Y, fit_pred(*Xs["B"])) - r2(Y, pred_mkt)),
                           full=float(r2(Y, fit_pred(*Xs["D"])))))
    df = pd.DataFrame(rows)
    sh = pd.DataFrame(shares)
    print(df.round(4).to_string(index=False))
    print(sh.round(4).to_string(index=False))
    w = df["days"] * df["n_tickers"]
    pooled = {k: float((df[k] * w).sum() / w.sum()) for k in ["r2_A", "r2_B", "r2_C", "r2_D"]}
    post = df[df["year"] >= 2023]
    wp = post["days"] * post["n_tickers"]
    pooled_post = {k: float((post[k] * wp).sum() / wp.sum()) for k in ["r2_A", "r2_B", "r2_C", "r2_D"]}
    print("pooled", {k: round(v, 4) for k, v in pooled.items()})
    print("pooled 2023+", {k: round(v, 4) for k, v in pooled_post.items()})
    out["m1"] = dict(years=df.to_dict("records"), shares=sh.to_dict("records"), pooled=pooled, pooled_2023=pooled_post, pcmap=pcmap)
    if pcmap:
        pm = pd.DataFrame(pcmap)
        print(pm.pivot(index="year", columns="pc", values="best").to_string())
        print(pm.pivot(index="year", columns="pc", values="corr").round(2).to_string())
    return pd.concat(resid).sort_index()


def block_m5(resid, news_path, idx, out):
    import datetime as dt
    z = resid / resid.rolling(250, min_periods=120).std().shift(1)
    z = z[(z.index >= "2021-01-01") & (z.index <= "2024-12-31")]
    cnt = {}
    for line in open(news_path):
        r = json.loads(line)
        t = r["ticker"]
        if t not in z.columns:
            continue
        ts = dt.datetime.utcfromtimestamp(r["published_ts"]) + dt.timedelta(hours=3)
        d = ts.date()
        if ts.hour * 60 + ts.minute >= 18 * 60 + 50:
            d = d + dt.timedelta(days=1)
        pos = idx.searchsorted(pd.Timestamp(d), side="left")
        if pos >= len(idx):
            continue
        cnt[(idx[pos], t)] = cnt.get((idx[pos], t), 0) + 1
    news = pd.Series(cnt).unstack().reindex(index=z.index, columns=z.columns).fillna(0.0)
    has = news > 0
    covered = has.sum(0) > 20
    zc = z.loc[:, covered]
    hc = has.loc[:, covered]
    absz = zc.abs()
    big = absz > 3
    rows = {}
    rows["tickers"] = int(covered.sum())
    rows["ticker_days"] = int(zc.notna().sum().sum())
    rows["base_news_rate"] = float(hc.where(zc.notna()).stack().mean())
    rows["big_moves"] = int(big.sum().sum())
    rows["news_rate_on_big"] = float(hc[big].stack().mean())
    rows["news_rate_on_quiet"] = float(hc[absz < 1].stack().mean())
    rows["mean_abs_z_news"] = float(absz[hc].stack().mean())
    rows["mean_abs_z_nonews"] = float(absz[~hc].stack().mean())
    big_up = (zc > 3)
    big_dn = (zc < -3)
    rows["big_without_news_share"] = float(1 - rows["news_rate_on_big"])
    out["m5"] = rows
    print("m5", json.dumps({k: round(v, 4) if isinstance(v, float) else v for k, v in rows.items()}))


def strat_stats(pos, y, carry, cost, label):
    pos = pd.Series(pos, index=y.index)
    gross = pos * y + (1 - pos.clip(0, 1)) * carry
    turn = pos.diff().abs().fillna(0)
    net = gross - cost * turn
    return net


def sharpe(x, carry):
    ex = (x - carry).dropna()
    return float(ex.mean() / ex.std() * np.sqrt(252)) if ex.std() > 0 else float("nan")


def maxdd(x):
    eq = (1 + x.fillna(0)).cumprod()
    return float((eq / eq.cummax() - 1).min())


def block_m2(m, w, idx, out, no_spx=False):
    feat = pd.DataFrame(index=idx)
    for c in ["spx", "vix", "brent", "usd", "dxy", "ust", "rvi"]:
        if no_spx and c == "spx":
            continue
        feat[c] = m[c]
    im = m["imoex"]
    feat["im_l1"] = im
    feat["im_5"] = im.rolling(5).sum()
    feat["im_21"] = im.rolling(21).sum()
    feat["vol21"] = im.rolling(21).std()
    feat["key"] = w["KEYRATE"].ffill()
    y = im.shift(-1)
    carry = carry_daily(w, idx).shift(-1)
    data = pd.concat([feat, y.rename("y"), carry.rename("carry")], axis=1).dropna()
    data = data[~((data.index >= fw.HALT_FROM - pd.Timedelta(days=1)) & (data.index <= fw.HALT_TO))]
    dates = data.index
    preds = pd.Series(np.nan, index=dates)
    base = pd.Series(np.nan, index=dates)
    model = None
    last = None
    importance = []
    for i, d in enumerate(dates):
        if d < STEP26_FIRST:
            continue
        if model is None or i - last >= 63:
            tr = data.iloc[:i - 1]
            model = make_pipeline(StandardScaler(), Ridge(alpha=10.0)).fit(tr[feat.columns], tr["y"])
            last = i
            importance.append(dict(zip(feat.columns, np.abs(model[-1].coef_))))
        preds.iloc[i] = model.predict(data.iloc[[i]][feat.columns])[0]
        base.iloc[i] = data["y"].iloc[:i - 1].mean()
    t = pd.DataFrame({"p": preds, "b": base, "y": data["y"], "carry": data["carry"]}).dropna()
    t = t[t.index >= STEP26_FIRST]
    R2 = 1 - ((t["y"] - t["p"]) ** 2).sum() / ((t["y"] - t["b"]) ** 2).sum()
    hits = int(((t["p"] > 0) == (t["y"] > 0)).sum())
    n = len(t)
    pval = binomtest(hits, n, 0.5, alternative="greater").pvalue
    pos = (t["p"] > 0).astype(float)
    net = strat_stats(pos, t["y"], t["carry"], IDX_COST, "lf")
    bh = t["y"]
    diff = (net - t["carry"]) - (bh - t["carry"])
    post = diff[diff.index >= fw.POST]
    res = dict(n=n, r2_oos=float(R2), hit=hits / n, hit_p=float(pval), t_diff=float(fw.nw_t(diff.values, 5)), mean_diff_daily=float(diff.mean()),
               sharpe_strat=sharpe(net, t["carry"]), sharpe_bh=sharpe(bh, t["carry"]), post_mean_diff=float(post.mean()), post_t=float(fw.nw_t(post.values, 5)),
               maxdd_strat=maxdd(net), maxdd_bh=maxdd(bh), long_share=float(pos.mean()))
    imp = pd.DataFrame(importance).mean().sort_values(ascending=False)
    res["importance"] = {k: float(v) for k, v in imp.items()}
    yearly = (diff.groupby(diff.index.year).sum()).round(4).to_dict()
    res["yearly_diff"] = {int(k): float(v) for k, v in yearly.items()}
    key = "m2_nospx" if no_spx else "m2"
    out[key] = res
    print(key, json.dumps({k: (round(v, 4) if isinstance(v, float) else v) for k, v in res.items() if k not in ("importance", "yearly_diff")}))
    print(" importance", {k: round(v, 5) for k, v in imp.items()})
    print(" yearly diff", res["yearly_diff"])


def block_m3(m, w, idx, out):
    from statsmodels.tsa.regime_switching.markov_regression import MarkovRegression
    im = (m["imoex"] * 100).dropna()
    im = im[~((im.index >= fw.HALT_FROM) & (im.index <= fw.HALT_TO))]
    carry = carry_daily(w, idx)
    dates = im.index
    prob = pd.Series(np.nan, index=dates)
    last = None
    params = None
    hi_state = None
    for i, d in enumerate(dates):
        if d < STEP26_FIRST:
            continue
        if params is None or i - last >= 63:
            tr = im.iloc[:i]
            mod = MarkovRegression(tr.to_numpy(), k_regimes=2, trend="c", switching_variance=True)
            try:
                res = mod.fit(disp=False, maxiter=200)
            except Exception:
                continue
            params = res.params
            sig = [params[k] for k in range(len(params)) if False]
            names = mod.param_names
            s0 = params[names.index("sigma2[0]")]
            s1 = params[names.index("sigma2[1]")]
            hi_state = 0 if s0 > s1 else 1
            last = i
            ext_start = i
            full = MarkovRegression(im.iloc[:].to_numpy(), k_regimes=2, trend="c", switching_variance=True)
            filt = full.smooth(params).filtered_marginal_probabilities
            filt = np.asarray(filt)
            chunk_end = min(len(dates), i + 63)
            for j in range(i, chunk_end):
                prob.iloc[j] = filt[j, hi_state]
    t = pd.DataFrame({"p": prob, "y": im / 100}).dropna()
    t = t[t.index >= STEP26_FIRST]
    nxt = t["y"].shift(-1)
    c = carry.reindex(t.index).shift(-1)
    d = pd.DataFrame({"p": t["p"], "y": nxt, "carry": c}).dropna()
    pos = (d["p"] < 0.5).astype(float)
    net = strat_stats(pos, d["y"], d["carry"], IDX_COST, "regime")
    bh = d["y"]
    s_st, s_bh = sharpe(net, d["carry"]), sharpe(bh, d["carry"])
    post_idx = d.index >= fw.POST
    res = dict(n=len(d), sharpe_strat=s_st, sharpe_bh=s_bh, sharpe_diff=s_st - s_bh, maxdd_strat=maxdd(net), maxdd_bh=maxdd(bh),
               post_sharpe_strat=sharpe(net[post_idx], d["carry"][post_idx]), post_sharpe_bh=sharpe(bh[post_idx], d["carry"][post_idx]),
               in_market=float(pos.mean()), total_strat=float((1 + net).prod() - 1), total_bh=float((1 + bh).prod() - 1))
    res["post_sharpe_diff"] = res["post_sharpe_strat"] - res["post_sharpe_bh"]
    out["m3"] = res
    print("m3", json.dumps({k: round(v, 4) if isinstance(v, float) else v for k, v in res.items()}))


def block_m4(adj_op, adj_cl, cl, vo, ret, m, idx, out, outpath):
    s, elig = fw.signals(adj_cl, cl, vo, ret)
    cols = [c for c in cl.columns if c != "IMOEX"]
    elig = elig.reindex(columns=cols).fillna(False)
    z = {k: fw.zscore_rows(v[cols].where(elig)) for k, v in s.items()}
    base_feats = {}
    for k, v in z.items():
        base_feats[k] = v
        base_feats[k + "_rk"] = v.rank(axis=1, pct=True)
    rel = ret[cols].sub(m["imoex"].reindex(idx).fillna(0.0), axis=0)
    mac = {}
    for name in ["usd", "brent", "vix", "ust"]:
        f = m[name].fillna(0.0)
        var = f.rolling(252, min_periods=200).var()
        cov = rel.rolling(252, min_periods=200).cov(f)
        beta = cov.div(var, axis=0)
        sd = f.rolling(252, min_periods=200).std()
        trend = f.rolling(63).sum() / (sd * np.sqrt(63))
        expo = beta.mul(sd, axis=0).mul(trend, axis=0)
        mac[name] = fw.zscore_rows(expo.where(elig))
    macro_feats = {}
    for k, v in mac.items():
        macro_feats["mac_" + k] = v
        macro_feats["mac_" + k + "_rk"] = v.rank(axis=1, pct=True)
    fwd = (adj_cl.shift(-fw.REBAL) / adj_op.shift(-1) - 1)[cols].where(elig)
    target = fwd.rank(axis=1, pct=True)
    positions = fw.periods(idx, fw.REBAL)
    res = {}
    series = {}
    for name, feats in (("ridge", base_feats), ("ridge_macro", {**base_feats, **macro_feats}), ("macro_only", {**macro_feats})):
        sc = fw.ml_scores(feats, target, idx, positions, kind="ridge")
        pf = fw.portfolio(sc, elig, adj_op[cols], adj_cl[cols], cl[cols], idx, positions, "lo")
        pf = pf[~((pf.index >= fw.HALT_FROM) & (pf.index <= fw.HALT_TO))]
        pf.to_csv(outpath + "." + name + "_lo.csv")
        series[name] = pf["net"]
        sm = fw.summarize(pf["net"], pf.index, 252 / fw.REBAL, 3)
        res[name] = sm
        print(name, json.dumps({k: round(v, 4) if isinstance(v, float) else v for k, v in sm.items()}), flush=True)
    d = (series["ridge_macro"] - series["ridge"]).dropna()
    res["paired_vs_ridge"] = dict(mean=float(d.mean()), t_nw=float(fw.nw_t(d.values, 3)), n=int(len(d)))
    yr = series["ridge_macro"].groupby(series["ridge_macro"].index.year).sum()
    res["years_sum"] = {int(k): float(v) for k, v in yr.items()}
    print("paired", res["paired_vs_ridge"], "years", {k: round(v, 3) for k, v in res["years_sum"].items()})
    out["m4"] = res


if __name__ == "__main__":
    snap, divpath, macro, outpath = sys.argv[1:5]
    blocks = sys.argv[5].split(",") if len(sys.argv) > 5 else ["m1", "m2", "m3", "m4"]
    op, cl, vo, adj_op, adj_cl, ret, w = prepare(snap, divpath, macro)
    idx = cl.index
    m = macro_changes(w)
    out = {}
    resid = None
    if "m1" in blocks or "m5" in blocks:
        resid = block_m1(ret, m, out, w)
    if "m5" in blocks:
        block_m5(resid, "../data/news_history.jsonl", idx, out)
    if "m2" in blocks:
        block_m2(m, w, idx, out)
        block_m2(m, w, idx, out, no_spx=True)
    if "m3" in blocks:
        block_m3(m, w, idx, out)
    if "m4" in blocks:
        block_m4(adj_op, adj_cl, cl, vo, ret, m, idx, out, outpath)
    json.dump(out, open(outpath + ".json", "w"), indent=1, default=float)
