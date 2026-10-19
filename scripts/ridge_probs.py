import sys
import numpy as np
import pandas as pd
import factors_wide as fw

T17 = ["YDEX", "OZON", "SBER", "LKOH", "GAZP", "GMKN", "ROSN", "NVTK", "TATN", "MTSS", "MGNT", "PLZL", "CHMF", "T", "VTBR", "RUAL", "POSI"]


def main(snap, divpath, outpath):
    op, cl, vo = fw.load(snap)
    idx = cl.index
    adj_op, adj_cl, ret = fw.adjust(op, cl, fw.dividends_matrix(divpath, idx, cl.columns))
    s, elig = fw.signals(adj_cl, cl, vo, ret)
    cols = [c for c in cl.columns if c != "IMOEX"]
    elig = elig.reindex(columns=cols).fillna(False)
    z = {k: fw.zscore_rows(v[cols].where(elig)) for k, v in s.items()}
    feats = {}
    for k, v in z.items():
        feats[k] = v
        feats[k + "_rk"] = v.rank(axis=1, pct=True)
    fwd = (adj_cl.shift(-fw.REBAL) / adj_op.shift(-1) - 1)[cols].where(elig)
    target = fwd.rank(axis=1, pct=True)
    positions = list(range(300, len(idx) - 1, fw.REBAL))
    sc = fw.ml_scores(feats, target, idx, positions, kind="ridge")
    r = sc.rank(axis=1, pct=True)
    rb = r.dropna(how="all")
    daily = rb.reindex(idx).ffill()
    daily = daily[daily.index >= "2025-03-01"]
    rows = []
    for t in T17:
        if t not in daily.columns:
            continue
        x = daily[t]
        d = pd.DataFrame({"ticker": t, "date": x.index.strftime("%Y-%m-%d"), "r": x.values}).dropna()
        d["p_ridge_ls"] = 0.5 + (d["r"] - 0.5) / 3
        d["p_ridge_bias"] = np.where(d["r"] <= 0.1, 0.4, np.where(d["r"] >= 0.8, 0.6 + (d["r"] - 0.8) / 3, 0.5))
        d["p_ridge_flip"] = 1 - d["p_ridge_ls"]
        rows.append(d)
    out = pd.concat(rows)
    out.to_csv(outpath, index=False)
    print(out.groupby("ticker").size().describe()[["min", "max"]].to_dict(), out["date"].min(), out["date"].max(), "rebalances", len(rb[rb.index >= "2025-03-01"]))
    print("share buy/sell", float((out["p_ridge_ls"] >= 0.6).mean()), float((out["p_ridge_ls"] <= 0.4).mean()))


if __name__ == "__main__":
    main(*sys.argv[1:4])
