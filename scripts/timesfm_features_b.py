import argparse
import csv
import hashlib
import sys

import numpy as np
from timesfm3 import ModelConfig, TimesFM3Evaluator

FLOWS = ["A2", "LevAsym", "SpLv"]
SHUFFLE_TAIL = 20


def num(x):
    try:
        return float(x)
    except ValueError:
        return float("nan")


def load(path):
    with open(path) as f:
        r = csv.reader(f)
        hdr = next(r)
        ix = {h: i for i, h in enumerate(hdr)}
        cols = ["rb1"] + FLOWS
        return [(rec[ix["date"]], rec[ix["secid"]], [num(rec[ix[c]]) for c in cols]) for rec in r]


def seed_of(d, s):
    return int(hashlib.sha256((d + "|" + s).encode()).hexdigest()[:8], 16)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--matrix", required=True)
    ap.add_argument("--out", required=True)
    ap.add_argument("--from-date", default="2022-04-01")
    ap.add_argument("--till-date", default="2026-10-16")
    ap.add_argument("--max-dates", type=int, default=0)
    ap.add_argument("--debug-shapes", action="store_true")
    args = ap.parse_args()

    rows = load(args.matrix)
    dates = sorted({d for d, _, _ in rows})
    by_date = {}
    for d, s, v in rows:
        by_date.setdefault(d, {})[s] = v
    market = {}
    for d in dates:
        vals = [v[0] for v in by_date[d].values() if not np.isnan(v[0])]
        market[d] = float(np.mean(vals)) if len(vals) >= 30 else float("nan")
    seq = {}
    for d in dates:
        for s, v in by_date[d].items():
            ab = v[0] - market[d] if not np.isnan(v[0]) and not np.isnan(market[d]) else 0.0
            fl = [0.0 if np.isnan(x) else x for x in v[1:]]
            seq.setdefault(s, []).append((d, ab, fl))
    pos = {s: {d: i for i, (d, _, _) in enumerate(q)} for s, q in seq.items()}

    cfg = ModelConfig(checkpoint_path="google/timesfm-3.0-pytorch", per_core_batch_size=64, device="cuda")
    model = TimesFM3Evaluator(cfg)
    base = dict(use_symmetric_averaging=False, make_positive=False, use_znorm=True)

    eval_dates = [d for d in dates if args.from_date <= d <= args.till_date]
    if args.max_dates:
        eval_dates = eval_dates[: args.max_dates]
    header = [
        "date", "secid", "u64", "u128", "u256", "u512",
        "q10s1", "q90s1", "q10s5", "q90s5", "q10s10", "q90s10", "sp",
        "sh256", "shapeA", "shapeB", "flA2", "flLev", "flSp",
    ]
    out = open(args.out, "w", newline="")
    w = csv.writer(out)
    w.writerow(header)
    for n, d in enumerate(eval_dates):
        names = sorted(s for s in by_date[d] if pos[s].get(d, 0) >= 255)
        if len(names) < 30:
            continue

        def ctx(s, length, field=1):
            i = pos[s][d]
            q = seq[s][i - length + 1 : i + 1]
            if field == 1:
                return np.array([x[1] for x in q], dtype=np.float32)
            return np.array([x[2][field - 2] for x in q], dtype=np.float32)

        def psum(ctxs, h=10):
            return [float(np.sum(o.forecast[:h])) for o in list(model.predict_batch(ctxs, horizon=h, return_quantiles=False, **base))]

        c256 = [ctx(s, 256) for s in names]
        u64 = psum([c[-64:] for c in c256])
        u128 = psum([c[-128:] for c in c256])
        u256 = psum(c256)
        long_idx = [k for k, s in enumerate(names) if pos[s][d] >= 511]
        u512 = {}
        if long_idx:
            res = psum([ctx(names[k], 512) for k in long_idx])
            u512 = dict(zip(long_idx, res))
        qo = list(model.predict_batch(c256, horizon=10, return_quantiles=True, **base))
        if args.debug_shapes and n == 0:
            print("quantile shape", np.asarray(qo[0].quantiles).shape, file=sys.stderr, flush=True)
        qa = [np.asarray(o.quantiles) for o in qo]
        sh = []
        for s, c in zip(names, c256):
            rng = np.random.default_rng(seed_of(d, s))
            c2 = c.copy()
            tail = c2[-SHUFFLE_TAIL:].copy()
            rng.shuffle(tail)
            c2[-SHUFFLE_TAIL:] = tail
            sh.append(c2)
        sh256 = psum(sh)
        h20 = [o.forecast for o in list(model.predict_batch(c256, horizon=20, return_quantiles=False, **base))]
        fl = [psum([ctx(s, 256, 2 + j) for s in names]) for j in range(3)]
        for k, s in enumerate(names):
            q = qa[k]
            f = h20[k]
            shape_a = float(np.mean(f[0:3]) - np.mean(f[3:10]))
            shape_b = float(np.mean(f[10:20]) - np.mean(f[0:10]))
            sp = float(np.mean(q[:10, 8] - q[:10, 0]))
            w.writerow([
                d, s, u64[k], u128[k], u256[k], u512.get(k, ""),
                q[0, 0], q[0, 8], q[4, 0], q[4, 8], q[9, 0], q[9, 8], sp,
                sh256[k], shape_a, shape_b, fl[0][k], fl[1][k], fl[2][k],
            ])
        if n % 20 == 0:
            print(d, len(names), file=sys.stderr, flush=True)
    out.close()


if __name__ == "__main__":
    main()
