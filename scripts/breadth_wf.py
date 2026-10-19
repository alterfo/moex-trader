import sys
import glob
import json
import itertools
import numpy as np
import pandas as pd
from joblib import Parallel, delayed
from scipy.stats import spearmanr
from sklearn.metrics import roc_auc_score

import hypo_wf as hw

T18 = ["YDEX", "OZON", "SBER", "LKOH", "GAZP", "GMKN", "ROSN", "NVTK", "TATN", "MTSS", "MGNT", "PLZL", "CHMF", "T", "VTBR", "RUAL", "POSI", "DATA"]
PRICE = hw.PRICE


def load(pattern, candles_path):
    df = pd.concat([pd.read_csv(f) for f in sorted(glob.glob(pattern))], ignore_index=True)
    df["date"] = pd.to_datetime(df["date"])
    df["label_date"] = pd.to_datetime(df["label_date"])
    px = pd.read_csv(candles_path, parse_dates=["date"]).sort_values(["ticker", "date"])
    px["next_open"] = px.groupby("ticker")["open"].shift(-1)
    ent = px[["ticker", "date", "next_open"]]
    ext = px[["ticker", "date", "close"]].rename(columns={"date": "label_date", "close": "exit_close"})
    df = df.merge(ent, on=["ticker", "date"], how="left").merge(ext, on=["ticker", "label_date"], how="left")
    df["fwd"] = (df["exit_close"] / df["next_open"] - 1) * 100
    df = df[df["fwd"].notna() & df[PRICE].notna().all(axis=1)].copy()
    df["orig"] = (df["fwd"] > 0).astype(int)
    df["keep"] = df["fwd"].abs() >= hw.DEADBAND
    return df.reset_index(drop=True)


def evaluate(df, cfg, arms):
    seed, shift = cfg
    test_pool = df[df["ticker"].isin(T18) & df["keep"]]
    res = {a: dict(p=[], y=[], f=[]) for a in arms}
    start = hw.FIRST_TEST + pd.Timedelta(days=shift)
    end = test_pool["date"].max()
    while start < end:
        stop = start + pd.Timedelta(days=hw.TEST_DAYS)
        te = test_pool[(test_pool["date"] >= start) & (test_pool["date"] < stop)]
        if len(te) > 50 and te["orig"].nunique() == 2:
            for a in arms:
                tr = train_rows(df, a, start)
                if len(tr) < 500:
                    continue
                p = hw.models(tr[PRICE].to_numpy(float), tr["orig"].to_numpy(), te[PRICE].to_numpy(float), seed)
                res[a]["p"].append(p)
                res[a]["y"].append(te["orig"].to_numpy())
                res[a]["f"].append(te["fwd"].to_numpy())
        start = stop
    out = dict(seed=seed, shift=shift)
    for a in arms:
        P, Y, F = np.concatenate(res[a]["p"]), np.concatenate(res[a]["y"]), np.concatenate(res[a]["f"])
        out[a] = dict(auc=float(roc_auc_score(Y, P)), rho=float(spearmanr(P, F)[0]), rows=int(len(P)))
    return out


def train_rows(df, arm, start):
    base = df[(df["label_date"] < start) & df["keep"]]
    if arm == "base18":
        return base[base["ticker"].isin(T18) & (base["date"] >= pd.Timestamp("2024-01-01"))]
    if arm == "wide":
        return base
    if arm == "wide2020":
        return base[base["date"] >= pd.Timestamp("2020-01-01")]
    raise ValueError(arm)


def stage1(pattern, candles, out):
    df = load(pattern, candles)
    print("rows", len(df), "tickers", df["ticker"].nunique(), "test-ticker rows", int(df["ticker"].isin(T18).sum()), flush=True)
    arms = ["base18", "wide", "wide2020"]
    cfgs = list(itertools.product(range(5), [0, 27, 54]))
    res = Parallel(n_jobs=5)(delayed(evaluate)(df, c, arms) for c in cfgs)
    json.dump(res, open(out, "w"))
    for a in arms:
        print(a, "mean auc", round(np.mean([r[a]["auc"] for r in res]), 4), "mean rho", round(np.mean([r[a]["rho"] for r in res]), 4), "rows", res[0][a]["rows"])
    for a in ("wide", "wide2020"):
        d = np.array([r[a]["auc"] - r["base18"]["auc"] for r in res])
        print(a, "minus base18: mean", round(d.mean(), 4), "pos", int((d > 0).sum()), "of", len(d), "min/max", round(d.min(), 4), round(d.max(), 4))


def probs(pattern, candles, out, specs):
    df = load(pattern, candles)
    rows = []
    for name, start, stop in hw.WINDOWS:
        s, e = pd.Timestamp(start), pd.Timestamp(stop)
        te = df[df["ticker"].isin(T18) & (df["date"] >= s) & (df["date"] <= e)]
        o = te[["ticker", "date"]].copy()
        for spec in specs:
            arm, seed = spec.split("@")
            tr = train_rows(df, arm, s)
            o["p_" + arm + "_s" + seed] = hw.models(tr[PRICE].to_numpy(float), tr["orig"].to_numpy(), te[PRICE].to_numpy(float), int(seed))
        o["window"] = name
        rows.append(o)
    pd.concat(rows).to_csv(out, index=False)


if __name__ == "__main__":
    if sys.argv[1] == "stage1":
        stage1(sys.argv[2], sys.argv[3], sys.argv[4])
    elif sys.argv[1] == "probs":
        probs(sys.argv[2], sys.argv[3], sys.argv[4], sys.argv[5].split(","))
