#!/bin/bash
set -euo pipefail
WORK=/home/oleg/moex-research/depthwf
T5=/home/oleg/moex-trader-task5
PY=/home/oleg/moex-venv/bin/python
SRC=/home/oleg/moex-research/depth/h10.csv
FEAT18="return_pct,realized_volatility,news_sentiment,news_count,order_book_imbalance,mom_5d,mom_21d,mom_63d,reversal_1d,rsi_14,dist_ma20_pct,dist_ma50_pct,realized_vol_21d_annualized_pct,volume_zscore_20d,macd_hist_pct,stoch_k_14,williams_r_14,alligator_spread_pct"
ORDER=$(grep -o 'ORDER="[^"]*"' "$T5/run-task5.sh" | head -1 | sed 's/ORDER="//; s/"$//')
TICKERS_17="YDEX,OZON,SBER,LKOH,GAZP,GMKN,ROSN,NVTK,TATN,MTSS,MGNT,PLZL,CHMF,T,VTBR,RUAL,POSI"
WINDOWS="q1|2025-03-04|2025-03-05|2025-06-05
q2|2025-06-05|2025-06-06|2025-09-05
q3|2025-09-05|2025-09-06|2025-12-03
q4|2025-12-03|2025-12-04|2026-03-04
w2|2026-03-04|2026-03-05|2026-06-06
w1|2026-06-06|2026-06-07|2026-09-16"
mkdir -p "$WORK/logs"
cd "$WORK"
"$PY" - <<PYEOF
import pandas as pd
df = pd.read_csv("$SRC")
df["negotiations_signal"] = 0
df["sanctions_signal"] = 0
df.to_csv("data-D.csv", index=False)
df[pd.to_datetime(df["date"]) >= "2024-01-01"].to_csv("data-A.csv", index=False)
PYEOF
for arm in A D; do
  while IFS="|" read -r name train_till bt_from bt_till; do
    [ -z "$name" ] && continue
    DATASET="data-$arm.csv" ENSEMBLE_FEATURES14="$FEAT18" ENSEMBLE_FEATURE_ORDER="$ORDER" \
      ENSEMBLE_TRAIN_TILL="$train_till" ENSEMBLE_EMBARGO_SPLIT="$train_till" \
      ENSEMBLE_EMBARGO_BARS=10 ENSEMBLE_OUT="ens-$arm-$name.json" \
      PYTHONPATH="$T5/scripts" "$PY" "$T5/scripts/export_ensemble.py" > "logs/train-$arm-$name.log" 2>&1
    grep -E "val_auc|wrote" "logs/train-$arm-$name.log" | tail -2
    "$T5/backtest" -config "$T5/config.sandbox.yaml" -tickers "$TICKERS_17" \
      -from "$bt_from" -till "$bt_till" -deposit 1000000 -max-lots 1000 \
      -commission-rate 0.0005 -spread-pct 0.0005 -slippage-pct 0.0005 \
      -target-notional 15000 -lookback-days 0 -signal-source ensemble \
      -ensemble-path "ens-$arm-$name.json" -out "bt-$arm-$name.md" > "logs/bt-$arm-$name.log" 2>&1
    grep -E "Realized P&L \(closed|Max drawdown|Kill switch" "bt-$arm-$name.md" | head -3
  done <<< "$WINDOWS"
done
echo ALLDONE
