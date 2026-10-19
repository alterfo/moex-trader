import sys
import json
import subprocess
import datetime as dt
import numpy as np
import pandas as pd

import factors_wide as fw

REBAL = 21


def latest_date(path):
    d = pd.read_csv(path, usecols=["date"], parse_dates=["date"])["date"]
    return d.max()


def update_snapshot(snap, binary, today):
    last = latest_date(snap)
    start = (last + pd.Timedelta(days=1)).strftime("%Y-%m-%d")
    if pd.Timestamp(start) > today:
        return
    tmp = snap + ".new"
    subprocess.run([binary, "-from", start, "-till", today.strftime("%Y-%m-%d"), "-out", tmp], check=True)
    new = pd.read_csv(tmp)
    if len(new):
        new.to_csv(snap, mode="a", header=False, index=False)


def main():
    snap, binary, divs, out, anchor = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4], pd.Timestamp(sys.argv[5])
    today = pd.Timestamp(dt.date.today())
    update_snapshot(snap, binary, today)
    op, cl, vo = fw.load(snap)
    idx = cl.index
    adj_op, adj_cl, ret = fw.adjust(op, cl, fw.dividends_matrix(divs, idx, cl.columns))
    s, elig = fw.signals(adj_cl, cl, vo, ret)
    cols = list(cl.columns)
    elig = elig.reindex(columns=cols).fillna(False)
    z = {k: fw.zscore_rows(v[cols].where(elig)) for k, v in s.items()}
    c2 = (z["mom_12_1"] - z["rev_1m"] + z["high52"]) / 3
    d = idx[-1]
    sc = c2.iloc[-1][elig.iloc[-1]].dropna().sort_values()
    k = max(1, len(sc) // 5)
    h52 = z["high52"].iloc[-1][elig.iloc[-1]].dropna().sort_values()
    kh = max(1, len(h52) // 5)
    feats = {}
    for name, v in z.items():
        feats[name] = v
        feats[name + "_rk"] = v.rank(axis=1, pct=True)
    fwd = (adj_cl.shift(-fw.REBAL) / adj_op.shift(-1) - 1)[cols].where(elig)
    target = fwd.rank(axis=1, pct=True)
    ridge = fw.ml_scores(feats, target, idx, [len(idx) - 1], kind="ridge").iloc[-1]
    ridge = ridge[elig.iloc[-1]].dropna().sort_values()
    kr = max(1, len(ridge) // 5)
    n_days = int(((idx >= anchor) & (idx <= d)).sum())
    rec = dict(date=d.strftime("%Y-%m-%d"), n_eligible=int(len(sc)), rebalance=bool(n_days > 0 and (n_days - 1) % REBAL == 0),
               long=list(sc.index[-k:][::-1]), short=list(sc.index[:k]), long_h52=list(h52.index[-kh:][::-1]), long_ridge=list(ridge.index[-kr:][::-1]), scores={t: round(float(v), 4) for t, v in sc.items()})
    seen = set()
    try:
        with open(out) as fh:
            seen = {json.loads(l)["date"] for l in fh if l.strip()}
    except FileNotFoundError:
        pass
    if rec["date"] in seen:
        print("already recorded", rec["date"])
        return
    with open(out, "a") as fh:
        fh.write(json.dumps(rec, ensure_ascii=False) + "\n")
    print("recorded", rec["date"], "eligible", rec["n_eligible"], "rebalance", rec["rebalance"])


main()
