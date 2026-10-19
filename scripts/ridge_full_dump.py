import sys
import factors_wide as fw


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
    r = sc.rank(axis=1, pct=True).dropna(how="all")
    r.index = r.index.strftime("%Y-%m-%d")
    r.index.name = "date"
    r.to_csv(outpath)
    elig.reindex(idx).loc[[d for d in idx if d.strftime("%Y-%m-%d") in set(r.index)]].astype(int).to_csv(outpath.replace(".csv", "_elig.csv"))


if __name__ == "__main__":
    main(*sys.argv[1:4])
