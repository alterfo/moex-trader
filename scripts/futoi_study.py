import sys
import json
import numpy as np
import pandas as pd

import factors_wide as fw

MAP = {"SR": "SBER", "GZ": "GAZP", "LK": "LKOH", "GK": "GMKN", "RN": "ROSN", "TN": "TATN", "MG": "MGNT", "VB": "VTBR", "YD": "YDEX", "AF": "AFLT", "AL": "ALRS", "MT": "MTSS", "NM": "NLMK", "NV": "NVTK", "MM": "MAGN", "HY": "HYDR", "SN": "SNGS"}
TEST_FROM = pd.Timestamp("2022-06-01")
COST = 0.0003


def load_ratio(path, tm="18:45:00"):
    f = pd.read_csv(path, parse_dates=["date"])
    f = f[(f["time"] == tm) & (f["group"] == "FIZ")].copy()
    f = f.drop_duplicates(["ticker", "date"], keep="last")
    f["r"] = f["pos"] / (f["pos_long"] + f["pos_short"].abs())
    return f.pivot(index="date", columns="ticker", values="r").sort_index()


def zscore_expanding(x, min_n=250):
    mu = x.expanding(min_n).mean().shift(1)
    sd = x.expanding(min_n).std().shift(1)
    return (x - mu) / sd


def boot_mean_t(v, n_boot=2000, seed=1):
    v = np.asarray(v, float)
    v = v[~np.isnan(v)]
    rng = np.random.default_rng(seed)
    b = np.array([v[rng.integers(0, len(v), len(v))].mean() for _ in range(n_boot)])
    return float(v.mean()), float(v.mean() / b.std()) if b.std() > 0 else float("nan")


def block_f1(r, w, res, label):
    imoex = w["IMOEX"].dropna()
    key = w["KEYRATE"].ffill().bfill() / 100 / 365
    dr = r["MX"] - r["MX"].shift(5)
    z = zscore_expanding(dr.dropna())
    dates = [d for d in z.index if d in imoex.index and d >= TEST_FROM]
    idx = imoex.index
    rows = []
    prev = 0.0
    p0 = None
    for d in dates:
        if p0 is not None and idx.get_loc(d) < p0 + 5:
            continue
        p = idx.get_loc(d)
        if p + 5 >= len(idx):
            break
        p0 = p
        pos = -np.sign(z[d])
        ret = imoex.iloc[p + 5] / imoex.iloc[p] - 1
        carry = float(key.reindex([idx[p]], method="ffill").iloc[0]) * (idx[p + 5] - idx[p]).days
        strat = pos * ret + carry - COST * abs(pos - prev)
        prev = pos
        rows.append((idx[p], strat, ret, pos * ret, ret * pos > 0))
    t = pd.DataFrame(rows, columns=["date", "strat", "bh", "overlay", "hit"]).set_index("date")
    diff = t["strat"] - t["bh"]
    mid = t.index[len(t) // 2]
    m, tt = fw.nw_t(diff.values, 2) * 0 + float(diff.mean()), fw.nw_t(diff.values, 2)
    out = dict(n=int(len(t)), mean_diff=float(diff.mean()), t_nw=float(tt), hit=float(t["hit"].mean()), overlay_mean=float(t["overlay"].mean()),
               overlay_t=float(fw.nw_t(t["overlay"].values, 2)), first_half=float(diff[diff.index < mid].mean()), second_half=float(diff[diff.index >= mid].mean()),
               long_share=float((t["overlay"] * 0 + 1).mean()))
    res[label] = out
    print(label, {k: round(v, 4) if isinstance(v, float) else v for k, v in out.items()})


def block_f2(r, ar, res, label):
    rows = []
    idx = ar.index
    for fut, stock in MAP.items():
        if fut not in r.columns or stock not in ar.columns:
            continue
        dr = r[fut] - r[fut].shift(5)
        z = zscore_expanding(dr.dropna())
        for d in z.index:
            if d not in idx:
                continue
            p = idx.get_loc(d)
            if p % 5 != 0 or p + 5 >= len(idx):
                continue
            y = ar[stock].iloc[p + 1:p + 6]
            if y.notna().sum() < 3:
                continue
            rows.append((d, fut, float(z[d]), float(y.sum())))
    t = pd.DataFrame(rows, columns=["date", "fut", "z", "y"]).dropna()
    t = t[~((t["date"] >= fw.HALT_FROM - pd.Timedelta(days=10)) & (t["date"] <= fw.HALT_TO + pd.Timedelta(days=10)))]
    def slope(df, seed=3, n_boot=2000):
        groups = [g for _, g in df.groupby("date")]
        sxy = np.array([(g["z"] * g["y"]).sum() for g in groups])
        sxx = np.array([(g["z"] ** 2).sum() for g in groups])
        b0 = sxy.sum() / sxx.sum()
        rng = np.random.default_rng(seed)
        bs = np.empty(n_boot)
        for i in range(n_boot):
            ix = rng.integers(0, len(groups), len(groups))
            bs[i] = sxy[ix].sum() / sxx[ix].sum()
        return float(b0), float(b0 / bs.std()), int(len(df))
    b, tt, n = slope(t)
    h1, h2 = t[t["date"] < "2024-01-01"], t[t["date"] >= "2024-01-01"]
    b1, t1, n1 = slope(h1)
    b2, t2, n2 = slope(h2)
    res[label] = dict(coef=b, t=tt, n=n, coef_2020_23=b1, t_2020_23=t1, n1=n1, coef_2024_26=b2, t_2024_26=t2, n2=n2)
    print(label, {k: round(v, 4) if isinstance(v, float) else v for k, v in res[label].items()})


def main(snap, divpath, macro, futoi, outpath):
    op, cl, vo = fw.load(snap)
    idx = cl.index
    adj_op, adj_cl, ret = fw.adjust(op, cl, fw.dividends_matrix(divpath, idx, cl.columns))
    cols = [c for c in cl.columns if c != "IMOEX"]
    s, elig = fw.signals(adj_cl, cl, vo, ret)
    elig = elig.reindex(columns=cols).fillna(False)
    mkt = ret[cols].where(elig).mean(axis=1)
    ar = ret[cols].sub(mkt, axis=0)
    w = pd.read_csv(macro, parse_dates=["date"]).set_index("date").sort_index()
    res = {}
    for tm in ("18:45:00", "23:50:00"):
        r = load_ratio(futoi, tm)
        print(tm, "tickers", list(r.columns)[:30], "days", len(r))
        suffix = "" if tm == "18:45:00" else "_2350"
        block_f1(r, w, res, "f1" + suffix)
        block_f2(r, ar, res, "f2" + suffix)
    json.dump(res, open(outpath, "w"), indent=1, default=float)


if __name__ == "__main__":
    main(*sys.argv[1:6])
