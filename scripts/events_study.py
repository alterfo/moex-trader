import sys
import json
import numpy as np
import pandas as pd

import factors_wide as fw

CUT = pd.Timestamp("2026-10-17")


def load_events(path):
    rows = []
    divs = []
    for line in open(path):
        r = json.loads(line)
        for e in r["reports"].get("events", []):
            rows.append((r["ticker"], pd.Timestamp(e["reportDate"][:10]), e.get("periodType", ""), e.get("periodYear", 0), e.get("periodNum", 0)))
        for d in r["dividends"].get("dividends", []):
            try:
                net = float(d["dividendNet"]["units"]) + float(d["dividendNet"]["nano"]) / 1e9
                divs.append((r["ticker"], pd.Timestamp(d["lastBuyDate"][:10]), net))
            except (KeyError, ValueError):
                pass
    ev = pd.DataFrame(rows, columns=["ticker", "date", "ptype", "year", "num"]).drop_duplicates(["ticker", "date"])
    dv = pd.DataFrame(divs, columns=["ticker", "date", "net"]).drop_duplicates()
    return ev, dv


def cluster_t(values, dates, n_boot=2000, seed=0):
    df = pd.DataFrame({"v": np.asarray(values, float), "d": dates}).dropna()
    g = df.groupby("d")["v"].agg(["sum", "count"])
    s, c = g["sum"].to_numpy(), g["count"].to_numpy()
    mean = s.sum() / c.sum()
    rng = np.random.default_rng(seed)
    n = len(g)
    boots = np.empty(n_boot)
    for i in range(n_boot):
        ix = rng.integers(0, n, n)
        boots[i] = s[ix].sum() / c[ix].sum()
    sd = boots.std()
    return float(mean), float(mean / sd) if sd > 0 else float("nan"), int(c.sum())


def event_table(ar, elig, ev, idx):
    pos_map = {}
    out = []
    ar_np = ar.to_numpy()
    cols = {c: i for i, c in enumerate(ar.columns)}
    el = elig.reindex(columns=ar.columns).fillna(False).to_numpy()
    for t, d in zip(ev["ticker"], ev["date"]):
        if t not in cols or d > CUT or d < pd.Timestamp("2016-01-01"):
            continue
        if fw.HALT_FROM - pd.Timedelta(days=30) <= d <= fw.HALT_TO + pd.Timedelta(days=30):
            continue
        p = idx.searchsorted(d, side="left")
        if p < 5 or p + 22 >= len(idx):
            continue
        j = cols[t]
        if not el[p - 1, j]:
            continue
        def cum(a, b):
            x = ar_np[p + a:p + b + 1, j]
            return float(np.nansum(x)) if np.isfinite(x).sum() >= max(1, (b - a + 1) // 2) else np.nan
        out.append(dict(ticker=t, date=idx[p], r01=cum(0, 1), pre=cum(-2, 1), drift=cum(2, 21), abs0=abs(ar_np[p, j]) if np.isfinite(ar_np[p, j]) else np.nan))
    return pd.DataFrame(out).dropna(subset=["r01", "drift"])


def block_events(ar, elig, ev, idx, res):
    et = event_table(ar, elig, ev, idx)
    typical = ar.abs().where(elig.reindex(columns=ar.columns).fillna(False)).stack().mean()
    res["events_n"] = int(len(et))
    res["e1"] = dict(mean_r01=float(et["r01"].mean()), abs_day0=float(et["abs0"].mean()), abs_typical=float(typical), ratio=float(et["abs0"].mean() / typical))
    m, t, n = cluster_t(et["pre"], et["date"])
    et["year"] = et["date"].dt.year
    yearly = et.groupby("year")["pre"].mean()
    ys = yearly.loc[2017:2025]
    res["e2"] = dict(mean=m, t=t, n=n, pos_years=int((ys > 0).sum()), years=int(len(ys)), yearly={int(k): float(v) for k, v in yearly.items()})
    q = et["r01"].quantile([1 / 3, 2 / 3]).to_numpy()
    top, bot = et[et["r01"] >= q[1]], et[et["r01"] <= q[0]]
    tmp = pd.concat([top.assign(sgn=1.0), bot.assign(sgn=-1.0)])
    m3, t3, n3 = cluster_t((tmp["drift"] * tmp["sgn"]) * 2 / 1.0, tmp["date"])
    dm = float(top["drift"].mean() - bot["drift"].mean())
    yd = pd.DataFrame({"top": top.groupby("year")["drift"].mean(), "bot": bot.groupby("year")["drift"].mean()}).loc[2017:2025]
    res["e3"] = dict(diff=dm, t=t3, top_mean=float(top["drift"].mean()), bot_mean=float(bot["drift"].mean()), n_top=int(len(top)), n_bot=int(len(bot)),
                     pos_years=int(((yd["top"] - yd["bot"]) > 0).sum()), years=int(len(yd)))
    print("events", len(et), "E1", {k: round(v, 4) for k, v in res["e1"].items()})
    print("E2", {k: (round(v, 4) if isinstance(v, float) else v) for k, v in res["e2"].items() if k != "yearly"}, {k: round(v, 4) for k, v in res["e2"]["yearly"].items()})
    print("E3", {k: round(v, 4) if isinstance(v, float) else v for k, v in res["e3"].items()})
    return et


def upcoming_flag(ev, idx, cols, positions, step=fw.REBAL):
    flag = pd.DataFrame(False, index=idx, columns=cols)
    by = ev.groupby("ticker")["date"].apply(lambda s: np.sort(s.values))
    for t, dates in by.items():
        if t not in flag.columns:
            continue
        for p in positions:
            if p + step >= len(idx):
                continue
            lo, hi = np.datetime64(idx[p]), np.datetime64(idx[p + step])
            k = np.searchsorted(dates, lo, side="right")
            if k < len(dates) and dates[k] <= hi:
                flag.iloc[p, flag.columns.get_loc(t)] = True
    return flag


def main(snap, divpath, evpath, outpath):
    op, cl, vo = fw.load(snap)
    idx = cl.index
    adj_op, adj_cl, ret = fw.adjust(op, cl, fw.dividends_matrix(divpath, idx, cl.columns))
    s, elig = fw.signals(adj_cl, cl, vo, ret)
    cols = [c for c in cl.columns if c != "IMOEX"]
    elig = elig.reindex(columns=cols).fillna(False)
    ev, dv = load_events(evpath)
    print("events raw", len(ev), "tickers", ev["ticker"].nunique(), "dividends", len(dv))
    mkt = ret[cols].where(elig).mean(axis=1)
    ar = ret[cols].sub(mkt, axis=0)
    res = {}
    block_events(ar, elig, ev, idx, res)
    z = {k: fw.zscore_rows(v[cols].where(elig)) for k, v in s.items()}
    feats = {}
    for k, v in z.items():
        feats[k] = v
        feats[k + "_rk"] = v.rank(axis=1, pct=True)
    fwd = (adj_cl.shift(-fw.REBAL) / adj_op.shift(-1) - 1)[cols].where(elig)
    target = fwd.rank(axis=1, pct=True)
    positions = fw.periods(idx, fw.REBAL)
    base_sc = fw.ml_scores(feats, target, idx, positions, kind="ridge")
    flag = upcoming_flag(ev, idx, cols, positions)
    series = {}
    def run(name, sc, exclude=None):
        pf = fw.portfolio(sc, elig, adj_op[cols], adj_cl[cols], cl[cols], idx, positions, "lo", exclude=exclude)
        pf = pf[~((pf.index >= fw.HALT_FROM) & (pf.index <= fw.HALT_TO))]
        series[name] = pf["net"]
        sm = fw.summarize(pf["net"], pf.index, 252 / fw.REBAL, 3)
        print(name, json.dumps({k: round(v, 4) if isinstance(v, float) else v for k, v in sm.items()}), flush=True)
        return sm
    res["ridge"] = run("ridge", base_sc)
    res["ridge_excl_reports"] = run("ridge_excl_reports", base_sc, exclude=flag)
    d = (series["ridge_excl_reports"] - series["ridge"]).dropna()
    res["e4_paired"] = dict(mean=float(d.mean()), t=float(fw.nw_t(d.values, 3)), n=int(len(d)))
    only = flag.reindex(columns=cols)
    topf = []
    for p in positions:
        sc = base_sc.iloc[p].dropna()
        sc = sc[elig.iloc[p].reindex(sc.index).fillna(False)]
        if len(sc) < 30:
            continue
        top = sc.sort_values().index[-max(1, len(sc) // 5):]
        f = only.iloc[p].reindex(top).fillna(False)
        topf.append(float(f.mean()))
    res["e4_flagged_share"] = float(np.mean(topf))
    print("E4 paired", res["e4_paired"], "flagged share of top quintile", round(res["e4_flagged_share"], 3))
    dd = dv.copy()
    dm = pd.DataFrame(0.0, index=idx, columns=cols)
    for t, d_, net in zip(dd["ticker"], dd["date"], dd["net"]):
        if t in dm.columns:
            p = idx.searchsorted(d_, side="right")
            if p < len(idx):
                dm.iloc[p, dm.columns.get_loc(t)] += net
    yld = (dm.rolling(252, min_periods=1).sum() / cl[cols]).clip(upper=0.35)
    yz = fw.zscore_rows(yld.where(elig))
    res["e5_yield"] = run("yield", yz)
    f2 = {**feats, "yield": yz, "yield_rk": yz.rank(axis=1, pct=True)}
    sc2 = fw.ml_scores(f2, target, idx, positions, kind="ridge")
    res["e5_ridge_yield"] = run("ridge_yield", sc2)
    d2 = (series["ridge_yield"] - series["ridge"]).dropna()
    res["e5_paired"] = dict(mean=float(d2.mean()), t=float(fw.nw_t(d2.values, 3)), n=int(len(d2)))
    yrs = series["yield"].groupby(series["yield"].index.year).sum()
    res["e5_yield_years"] = {int(k): float(v) for k, v in yrs.items()}
    print("E5 paired", res["e5_paired"], "yield years", {k: round(v, 3) for k, v in res["e5_yield_years"].items()})
    json.dump(res, open(outpath + ".json", "w"), indent=1, default=float)


if __name__ == "__main__":
    main(*sys.argv[1:5])
