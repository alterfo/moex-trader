import sys
import json
import numpy as np
import pandas as pd

import factors_wide as fw


def main():
    snap, divs, shadow = sys.argv[1], sys.argv[2], sys.argv[3]
    recs = [json.loads(l) for l in open(shadow) if l.strip()]
    reb = [r for r in recs if r["rebalance"]]
    op, cl, vo = fw.load(snap)
    idx = cl.index
    adj_op, adj_cl, ret = fw.adjust(op, cl, fw.dividends_matrix(divs, idx, cl.columns))
    ff = adj_cl.ffill()
    rows = []
    prev_l, prev_s = set(), set()
    for r in reb:
        d = pd.Timestamp(r["date"])
        p = idx.get_loc(d)
        if p + fw.REBAL >= len(idx):
            continue
        entry = adj_op.iloc[p + 1].fillna(adj_cl.iloc[p])
        fwd = (ff.iloc[p + fw.REBAL] / entry - 1).fillna(0.0)
        L, S = set(r["long"]), set(r["short"])
        rl = float(fwd.reindex(list(L)).mean())
        rs = float(fwd.reindex(list(S)).mean())
        univ = float(fwd.reindex(list(r["scores"].keys())).mean())
        tl = 1.0 if not prev_l else len(L ^ prev_l) / len(L)
        ts = 1.0 if not prev_s else len(S ^ prev_s) / len(S)
        prev_l, prev_s = L, S
        days = (idx[p + fw.REBAL] - idx[p]).days
        rows.append(dict(date=d, ls_net=rl - rs - fw.COST * (tl + ts) - fw.BORROW * days, lo_net=rl - univ - fw.COST * tl))
    df = pd.DataFrame(rows).set_index("date")
    print(df)
    for c in ("ls_net", "lo_net"):
        print(c, "periods", len(df), "mean", round(df[c].mean(), 4), "t_nw", round(fw.nw_t(df[c].values, 3), 2))


main()
