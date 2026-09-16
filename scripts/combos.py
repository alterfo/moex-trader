import os

import numpy as np
import pandas as pd

PREDS = os.environ.get("PREDS", "/tmp/moex-preds.csv")
DATASET = os.environ.get("DATASET", "/tmp/moex-dataset.csv")
OUT = os.environ.get("COMBO", "/tmp/moex-combo.csv")

REV_THRESHOLD = float(os.environ.get("REV_THRESHOLD", "0.5"))


def main():
    df = pd.read_csv(PREDS)
    ds = pd.read_csv(DATASET, usecols=["ticker", "date", "reversal_1d"])
    df = df.merge(ds, on=["ticker", "date"], how="left")
    p_ens = (df["p_lgbm"] + df["p_xgb"]) / 2.0
    df["p_ens2"] = p_ens
    df["p_ens3"] = (df["p_lgbm"] + df["p_xgb"] + df["p_logreg"]) / 3.0
    df["p_ens2_rev"] = np.where(df["reversal_1d"].abs() >= REV_THRESHOLD, p_ens, 0.5)
    df["p_ens3_rev"] = np.where(df["reversal_1d"].abs() >= REV_THRESHOLD, df["p_ens3"], 0.5)
    df.to_csv(OUT, index=False)
    print(f"wrote {OUT} ({len(df)} rows)")


if __name__ == "__main__":
    main()