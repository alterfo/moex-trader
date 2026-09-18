#!/usr/bin/env python3
import json
import os

import numpy as np
import pandas as pd
import xgboost as xgb
import lightgbm as lgb
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import roc_auc_score
from sklearn.pipeline import make_pipeline
from sklearn.preprocessing import StandardScaler

import os
FEATURES = os.environ.get("ENSEMBLE_FEATURES14","").split(",") if os.environ.get("ENSEMBLE_FEATURES14") else [
    "return_pct", "realized_volatility", "news_sentiment", "news_count", "order_book_imbalance",
    "mom_5d", "mom_21d", "mom_63d", "reversal_1d", "rsi_14",
    "dist_ma20_pct", "dist_ma50_pct", "realized_vol_21d_annualized_pct", "volume_zscore_20d",
    "macd_hist_pct", "stoch_k_14", "williams_r_14", "alligator_spread_pct",
    "event_dividend", "event_buyback", "event_sanctions", "event_ipo",
    "event_report", "event_delisting", "event_mna", "event_default",
    "negotiations_signal", "sanctions_signal",
]
ORDER = os.environ.get("ENSEMBLE_FEATURE_ORDER", "")
ORDER = ORDER.split(",") if ORDER else FEATURES
TRAIN_TILL = os.environ.get("ENSEMBLE_TRAIN_TILL", "")


def expand(values, default, names):
    out = [default] * len(ORDER)
    for name, value in zip(names, values):
        out[ORDER.index(name)] = value
    return out


def to_flat(tree, positions):
    return {
        "si": [positions[x] if x >= 0 else -1 for x in tree["split_indices"]],
        "sc": [float(np.float32(x)) for x in tree["split_conditions"]],
        "lc": tree["left_children"],
        "rc": tree["right_children"],
        "dl": tree["default_left"],
    }


def lgb_recursive_to_flat(node, positions):
    si, sc, lc, rc, dl = [], [], [], [], []
    nid = [0]

    def walk(n):
        if "leaf_value" in n:
            i = nid[0]
            nid[0] += 1
            si.append(-1)
            sc.append(n["leaf_value"])
            lc.append(-1)
            rc.append(-1)
            dl.append(1)
            return i
        i = nid[0]
        nid[0] += 1
        si.append(positions[n["split_feature"]])
        sc.append(float(n["threshold"]))
        lc.append(-1)
        rc.append(-1)
        dl.append(0)
        left = walk(n["left_child"])
        right = walk(n["right_child"])
        lc[i] = left
        rc[i] = right
        return i

    walk(node)
    return {"si": si, "sc": sc, "lc": lc, "rc": rc, "dl": dl}


def main():
    df = pd.read_csv(os.environ.get("DATASET", "/tmp/moex-dataset.csv"))
    train = df[df["split"] == "train"]
    if TRAIN_TILL:
        train = train[train["date"] <= TRAIN_TILL]
    X = train[FEATURES].to_numpy(dtype=float)
    y = train["label"].to_numpy(dtype=int)

    xgb_model = xgb.XGBClassifier(
        n_estimators=300, learning_rate=0.03, max_depth=3,
        colsample_bytree=0.8, subsample=0.8, random_state=42,
        base_score=0.5, eval_metric="logloss",
    ).fit(X, y)

    lgb_model = lgb.LGBMClassifier(
        n_estimators=300, learning_rate=0.03, num_leaves=15,
        colsample_bytree=0.8, subsample=0.8, random_state=42, verbose=-1,
    ).fit(X, y)

    lr = make_pipeline(StandardScaler(), LogisticRegression(C=1.0, max_iter=2000)).fit(X, y)
    scaler = lr.named_steps["standardscaler"]
    reg = lr.named_steps["logisticregression"]

    raw = json.loads(xgb_model.get_booster().save_raw(raw_format="json"))
    positions = [ORDER.index(name) for name in FEATURES]
    xgb_trees = [to_flat(t, positions) for t in raw["learner"]["gradient_booster"]["model"]["trees"]]
    base = json.loads(raw["learner"]["learner_model_param"]["base_score"])
    xgb_base_logit = float(np.log(base[0] / (1.0 - base[0])))

    dump = lgb_model.booster_.dump_model()
    lgb_trees = [lgb_recursive_to_flat(t["tree_structure"], positions) for t in dump["tree_info"]]

    model = {
        "feature_order": ORDER,
        "buy_threshold": 0.60,
        "sell_threshold": 0.40,
        "lgb_base_logit": 0.0,
        "xgb_base_logit": xgb_base_logit,
        "logistic": {
            "mean": expand(scaler.mean_.tolist(), 0.0, FEATURES),
            "std": expand(scaler.scale_.tolist(), 1.0, FEATURES),
            "coef": expand(reg.coef_[0].tolist(), 0.0, FEATURES),
            "bias": float(reg.intercept_[0]),
        },
        "lgb_trees": lgb_trees,
        "xgb_trees": xgb_trees,
    }
    out = os.environ.get("ENSEMBLE_OUT", "ensemble_model.json")
    with open(out, "w") as f:
        json.dump(model, f)
    print("wrote", out, len(xgb_trees), "xgb trees,", len(lgb_trees), "lgb trees")

    val = df[(df["date"] >= "2026-06-18") & (df["date"] <= "2026-09-16")]
    val = val[val["label"].notna()]
    Xv = val[FEATURES].to_numpy(dtype=float)
    yv = val["label"].to_numpy(dtype=int)
    p_xgb = xgb_model.predict_proba(Xv)[:, 1]
    p_lgb = lgb_model.predict_proba(Xv)[:, 1]
    p_lr = lr.predict_proba(Xv)[:, 1]
    ens = (p_xgb + p_lgb + p_lr) / 3
    print("val_auc xgb=%.4f lgbm=%.4f logreg=%.4f ensemble=%.4f" % (
        roc_auc_score(yv, p_xgb), roc_auc_score(yv, p_lgb),
        roc_auc_score(yv, p_lr), roc_auc_score(yv, ens)))
    np.save("/tmp/ens_val_proba.npy", ens)
    np.save("/tmp/ens_val_x.npy", Xv)


if __name__ == "__main__":
    main()
