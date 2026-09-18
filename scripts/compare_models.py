import os

import lightgbm as lgb
import numpy as np
import pandas as pd
import xgboost as xgb
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import accuracy_score, roc_auc_score
from sklearn.pipeline import make_pipeline
from sklearn.preprocessing import StandardScaler

SPLIT = "2026-06-18"
TILL = "2026-09-16"
PRED_FROM = "2026-05-12"
FEATURES = [
    "return_pct", "realized_volatility", "news_sentiment", "news_count",
    "order_book_imbalance", "mom_5d", "mom_21d", "mom_63d", "reversal_1d",
    "rsi_14", "dist_ma20_pct", "dist_ma50_pct", "realized_vol_21d_annualized_pct",
    "volume_zscore_20d", "macd_hist_pct", "stoch_k_14", "williams_r_14",
    "alligator_spread_pct",
    "event_dividend", "event_buyback", "event_sanctions", "event_ipo",
    "event_report", "event_delisting", "event_mna", "event_default",
    "negotiations_signal", "sanctions_signal",
]

DATA = os.environ.get("DATASET", "/tmp/moex-dataset.csv")
OUT = os.environ.get("PREDS", "/tmp/moex-preds.csv")


def main():
    df = pd.read_csv(DATA)
    train = df[df["split"] == "train"]
    pred_rows = df[(df["date"] >= PRED_FROM) & (df["date"] <= TILL)].copy()

    X_train = train[FEATURES].to_numpy(dtype=float)
    y_train = train["label"].to_numpy(dtype=int)
    X_pred = pred_rows[FEATURES].to_numpy(dtype=float)

    labeled = df["label"].notna().to_numpy()
    val_lab = df[labeled & (df["date"] >= SPLIT)].copy()

    models = {
        "logreg": make_pipeline(StandardScaler(), LogisticRegression(C=1.0, max_iter=2000)),
        "lgbm": lgb.LGBMClassifier(
            n_estimators=300, learning_rate=0.03, num_leaves=15,
            colsample_bytree=0.8, subsample=0.8, random_state=42, verbose=-1,
        ),
        "xgb": xgb.XGBClassifier(
            n_estimators=300, learning_rate=0.03, max_depth=3,
            colsample_bytree=0.8, subsample=0.8, random_state=42,
            eval_metric="logloss",
        ),
    }

    out = pred_rows[["ticker", "date"]].copy()
    for name, model in models.items():
        model.fit(X_train, y_train)
        p_all = model.predict_proba(X_pred)[:, 1]
        out[f"p_{name}"] = p_all
        p_lab = model.predict_proba(val_lab[FEATURES].to_numpy(dtype=float))[:, 1]
        y_val = val_lab["label"].to_numpy(dtype=int)
        auc = roc_auc_score(y_val, p_lab)
        acc = accuracy_score(y_val, (p_lab >= 0.5).astype(int))
        print(f"{name}: val_auc={auc:.4f} val_acc={acc:.4f}")

    out.to_csv(OUT, index=False)
    print(f"wrote {OUT} ({len(out)} pred rows)")


if __name__ == "__main__":
    main()