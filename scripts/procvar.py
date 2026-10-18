import sys, json, itertools
import numpy as np, pandas as pd, lightgbm as lgb
from joblib import Parallel, delayed
from sklearn.linear_model import LogisticRegression
from sklearn.pipeline import make_pipeline
from sklearn.preprocessing import StandardScaler
from sklearn.metrics import roc_auc_score

PRICE = ["return_pct","realized_volatility","mom_5d","mom_21d","mom_63d","reversal_1d","rsi_14","dist_ma20_pct","dist_ma50_pct","realized_vol_21d_annualized_pct","volume_zscore_20d","macd_hist_pct","stoch_k_14","williams_r_14","alligator_spread_pct"]
FX = {"GLDRUB_TOM","SLVRUB_TOM","CNYRUB_TOM"}


def load(path):
    df = pd.read_csv(path)
    df = df[~df.ticker.isin(FX)]
    df = df[df.label.notna()].copy()
    df["date"] = pd.to_datetime(df["date"])
    df["label_date"] = pd.to_datetime(df["label_date"])
    return df


def fit_predict(tr, te, feats, seed):
    X, y = tr[feats].to_numpy(float), tr.label.to_numpy(int)
    Xt = te[feats].to_numpy(float)
    m1 = lgb.LGBMClassifier(n_estimators=300, learning_rate=0.03, num_leaves=15, colsample_bytree=0.8, subsample=0.8, subsample_freq=1, random_state=seed, verbose=-1, n_jobs=1).fit(X, y)
    m2 = make_pipeline(StandardScaler(), LogisticRegression(C=1.0, max_iter=2000)).fit(X, y)
    return (m1.predict_proba(Xt)[:, 1] + m2.predict_proba(Xt)[:, 1]) / 2


def wf(df, seed, first_test, test_days, train_start):
    d = df[df.date >= pd.Timestamp(train_start)]
    start = pd.Timestamp(first_test)
    end = d.date.max()
    folds, ps, ys = [], [], []
    while start < end:
        stop = start + pd.Timedelta(days=test_days)
        te = d[(d.date >= start) & (d.date < stop)]
        tr = d[d.label_date < start]
        if len(te) > 50 and te.label.nunique() == 2 and len(tr) > 500:
            p = fit_predict(tr, te, PRICE, seed)
            folds.append(roc_auc_score(te.label, p))
            ps.append(p)
            ys.append(te.label.to_numpy())
        start = stop
    return dict(pooled=roc_auc_score(np.concatenate(ys), np.concatenate(ps)), mean=float(np.mean(folds)), folds=len(folds), rows=int(sum(map(len, ys))))


def run(cfg, df):
    r = wf(df, cfg["seed"], cfg["first_test"], cfg["test_days"], cfg["train_start"])
    return {**cfg, **r}


def main():
    path, out = sys.argv[1], sys.argv[2]
    df = load(path)
    base = pd.Timestamp("2025-04-01")
    shifts = [0, 27, 54]
    starts = ["2023-07-01", "2023-10-01", "2024-01-01", "2024-04-01", "2024-07-01"]
    cfgs = []
    for seed, sh, st in itertools.product(range(5), shifts, starts):
        cfgs.append(dict(recipe="A", seed=seed, shift=sh, first_test=str((base + pd.Timedelta(days=sh)).date()), test_days=63, train_start=st))
    for seed, sh in itertools.product(range(5), shifts):
        cfgs.append(dict(recipe="D", seed=seed, shift=sh, first_test=str((base + pd.Timedelta(days=sh)).date()), test_days=63, train_start="2015-01-01"))
    for td in [42, 63, 84]:
        cfgs.append(dict(recipe="A", seed=42, shift=0, first_test=str(base.date()), test_days=td, train_start="2024-01-01", axis="test_days"))
    res = Parallel(n_jobs=6)(delayed(run)(c, df) for c in cfgs)
    json.dump(res, open(out, "w"))


if __name__ == "__main__":
    main()
