import sys
import numpy as np
import pandas as pd

MIN_VALUE = 30e6


def main():
    src, out_csv, out_list = sys.argv[1], sys.argv[2], sys.argv[3]
    px = pd.read_csv(src, parse_dates=["date"])
    px = px[(px["close"] > 0) & (px["open"] > 0) & (px["high"] > 0) & (px["low"] > 0)]
    px = px.sort_values(["ticker", "date"])
    parts, keep = [], []
    for t, g in px.groupby("ticker"):
        if len(g) < 320:
            continue
        g = g.reset_index(drop=True)
        value = g["value"].fillna(g["close"] * g["volume"])
        if value.rolling(60, min_periods=50).median().max() < MIN_VALUE:
            continue
        ret = g["close"] / g["close"].shift(1) - 1
        bad = (ret < -0.45) | (ret > 0.8)
        scale = pd.Series(1.0, index=g.index)
        for i in g.index[bad.fillna(False)]:
            r = g.loc[i, "close"] / g.loc[i - 1, "close"]
            scale.iloc[:i] *= r
        for c in ("open", "high", "low", "close"):
            g[c] = g[c] * scale
        g["ticker"] = t
        parts.append(g[["ticker", "date", "open", "high", "low", "close", "volume"]])
        keep.append(t)
    pd.concat(parts).to_csv(out_csv, index=False, date_format="%Y-%m-%d")
    open(out_list, "w").write(",".join(keep))
    print("tickers", len(keep))


main()
