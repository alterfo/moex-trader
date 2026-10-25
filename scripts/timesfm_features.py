import argparse
import csv
import sys

import numpy as np
from timesfm3 import ModelConfig, TimesFM3Evaluator

HORIZON = 10
CONTEXT = 256
COVARIATES = ["A2", "LevAsym", "SpLv", "ImbVol"]


def load(path):
    with open(path) as f:
        r = csv.reader(f)
        hdr = next(r)
        ix = {h: i for i, h in enumerate(hdr)}
        rows = [(rec[ix["date"]], rec[ix["secid"]], [rec[ix[c]] for c in ["rb1"] + COVARIATES]) for rec in r]
    return rows


def num(x):
    try:
        v = float(x)
    except ValueError:
        return float("nan")
    return v


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--matrix", required=True)
    ap.add_argument("--out", required=True)
    ap.add_argument("--from-date", default="2025-01-03")
    ap.add_argument("--till-date", default="2026-10-16")
    ap.add_argument("--max-dates", type=int, default=0)
    args = ap.parse_args()

    rows = load(args.matrix)
    dates = sorted({d for d, _, _ in rows})
    by_date = {}
    for d, s, v in rows:
        by_date.setdefault(d, {})[s] = [num(x) for x in v]
    market = {}
    for d in dates:
        vals = [v[0] for v in by_date[d].values() if not np.isnan(v[0])]
        market[d] = float(np.mean(vals)) if len(vals) >= 30 else float("nan")
    seq = {}
    for d in dates:
        for s, v in by_date[d].items():
            ab = v[0] - market[d] if not np.isnan(v[0]) and not np.isnan(market[d]) else 0.0
            cov = [0.0 if np.isnan(x) else x for x in v[1:]]
            seq.setdefault(s, []).append((d, ab, cov))
    pos = {s: {d: i for i, (d, _, _) in enumerate(q)} for s, q in seq.items()}

    cfg = ModelConfig(checkpoint_path="google/timesfm-3.0-pytorch", per_core_batch_size=64, device="cuda")
    model = TimesFM3Evaluator(cfg)

    eval_dates = [d for d in dates if args.from_date <= d <= args.till_date]
    if args.max_dates:
        eval_dates = eval_dates[: args.max_dates]
    out = open(args.out, "w", newline="")
    w = csv.writer(out)
    w.writerow(["date", "secid", "tfm_uni", "tfm_cov", "tfm_plc"])
    for n, d in enumerate(eval_dates):
        names = sorted(s for s in by_date[d] if pos[s].get(d, 0) >= CONTEXT - 1)
        if len(names) < 30:
            continue
        uni, tgt, cov, plc = [], [], [], []
        for k, s in enumerate(names):
            i = pos[s][d]
            q = seq[s][i - CONTEXT + 1 : i + 1]
            a = np.array([x[1] for x in q], dtype=np.float32)
            c = np.array([x[2] for x in q], dtype=np.float32).T
            o = names[(k + 1) % len(names)]
            j = pos[o][d]
            qo = seq[o][j - CONTEXT + 1 : j + 1]
            co = np.array([x[2] for x in qo], dtype=np.float32).T
            uni.append(a)
            tgt.append(a.reshape(1, -1))
            cov.append(c)
            plc.append(co)
        kw = dict(horizon=HORIZON, return_quantiles=False, use_symmetric_averaging=False, make_positive=False, use_znorm=True)
        pu = [float(np.sum(o.forecast)) for o in model.predict_batch(uni, **kw)]
        pc = [float(np.sum(o.forecast)) for o in model.predict_batch(tgt, past_only_covariates=cov, **kw)]
        pp = [float(np.sum(o.forecast)) for o in model.predict_batch(tgt, past_only_covariates=plc, **kw)]
        for s, a, b, c in zip(names, pu, pc, pp):
            w.writerow([d, s, a, b, c])
        if n % 20 == 0:
            print(d, len(names), file=sys.stderr, flush=True)
    out.close()


if __name__ == "__main__":
    main()
