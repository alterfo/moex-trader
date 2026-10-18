import sys
import time
import numpy as np
import pandas as pd
import torch
import timesfm

CONTEXT = 512
HORIZON = 11
BATCH = 32


def main():
    data_path, prices_path, out_path = sys.argv[1], sys.argv[2], sys.argv[3]
    torch.set_num_threads(6)
    rows = pd.read_csv(data_path, usecols=["ticker", "date"], parse_dates=["date"])
    px = pd.read_csv(prices_path, parse_dates=["date"]).sort_values(["ticker", "date"])
    series = {t: (g["date"].to_numpy(), g["close"].to_numpy(float)) for t, g in px.groupby("ticker")}
    model = timesfm.TimesFM_2p5_200M_torch.from_pretrained("google/timesfm-2.5-200m-pytorch")
    model.compile(timesfm.ForecastConfig(max_context=CONTEXT, max_horizon=HORIZON, normalize_inputs=True, use_continuous_quantile_head=True, force_flip_invariance=True, infer_is_positive=True, fix_quantile_crossing=True))
    keys, inputs, last = [], [], []
    for t, d in zip(rows["ticker"], rows["date"]):
        if t not in series:
            continue
        dates, close = series[t]
        n = int(np.searchsorted(dates, np.datetime64(d), side="left"))
        if n < 64:
            continue
        ctx = close[max(0, n - CONTEXT):n]
        keys.append((t, d))
        inputs.append(ctx)
        last.append(ctx[-1])
    print("forecasts", len(inputs), flush=True)
    out = []
    started = time.time()
    for i in range(0, len(inputs), BATCH):
        chunk = inputs[i:i + BATCH]
        point, quant = model.forecast(horizon=HORIZON, inputs=chunk)
        for j in range(len(chunk)):
            l = last[i + j]
            q = quant[j, HORIZON - 1]
            out.append((keys[i + j][0], keys[i + j][1], point[j, HORIZON - 1] / l - 1, q[5] / l - 1, (q[9] - q[1]) / l, (l - q[1]) / max(q[9] - q[1], 1e-9)))
        if (i // BATCH) % 20 == 0:
            print(i, len(inputs), round(time.time() - started), flush=True)
            pd.DataFrame(out, columns=["ticker", "date", "tfm_point", "tfm_ret", "tfm_iqr", "tfm_pos"]).to_csv(out_path, index=False)
    pd.DataFrame(out, columns=["ticker", "date", "tfm_point", "tfm_ret", "tfm_iqr", "tfm_pos"]).to_csv(out_path, index=False)
    print("done", round(time.time() - started), flush=True)


main()
