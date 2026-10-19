import sys
import numpy as np
import pandas as pd

WINDOW_DAYS = 60
MIN_HISTORY = 200


def transform(df, col):
    d = df[["ticker", "date", col]].copy()
    d["date"] = pd.to_datetime(d["date"])
    d = d.sort_values("date").reset_index(drop=True)
    out = np.full(len(d), 0.5)
    dates = d["date"].to_numpy()
    p = d[col].to_numpy()
    uniq = np.unique(dates)
    for day in uniq:
        lo = day - np.timedelta64(WINDOW_DAYS, "D")
        hist = p[(dates >= lo) & (dates < day)]
        if len(hist) < MIN_HISTORY:
            continue
        q_hi, q_lo = np.percentile(hist, 80), np.percentile(hist, 20)
        m = dates == day
        out[m] = np.where(p[m] >= q_hi, 0.9, np.where(p[m] <= q_lo, 0.1, 0.5))
    d[col + "_q"] = out
    return d


def main():
    src, dst = sys.argv[1], sys.argv[2]
    df = pd.read_csv(src)
    cols = [c for c in df.columns if c.startswith("p_")]
    res = df[["ticker", "date", "window"]].copy()
    res["date"] = pd.to_datetime(res["date"])
    for c in cols:
        t = transform(df, c)
        name = c.replace("p_", "p_", 1)
        key = t[["ticker", "date", c + "_q"]].drop_duplicates(["ticker", "date"])
        res = res.merge(key, on=["ticker", "date"], how="left").rename(columns={c + "_q": c.replace("_s", "q_s")})
    res.to_csv(dst, index=False, date_format="%Y-%m-%d")
    print(res.shape, [c for c in res.columns if c.startswith("p_")][:4])


main()
