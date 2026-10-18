#!/bin/bash
set -euo pipefail
WORK=/home/oleg/moex-research/regimewf
T5=/home/oleg/moex-trader-task5
PY=/home/oleg/moex-venv/bin/python
SRC=/home/oleg/moex-research/depth/h10.csv
FEAT18=$(grep -o 'FEAT18="[^"]*"' "$T5/run-task5.sh" | head -1 | sed 's/FEAT18="//; s/"$//')
ORDER=$(grep -o '^ORDER="[^"]*"' "$T5/run-task5.sh" | head -1 | sed 's/ORDER="//; s/"$//')
TICKERS_17="YDEX,OZON,SBER,LKOH,GAZP,GMKN,ROSN,NVTK,TATN,MTSS,MGNT,PLZL,CHMF,T,VTBR,RUAL,POSI"
WINDOWS="p1|2024-03-04|2024-03-05|2024-06-05
p2|2024-06-05|2024-06-06|2024-09-05
p3|2024-09-05|2024-09-06|2024-12-03
p4|2024-12-03|2024-12-04|2025-03-04"
mkdir -p "$WORK/logs" "$WORK/wf"
cd "$WORK"
"$PY" - <<PYEOF
import pandas as pd
df = pd.read_csv("$SRC")
df["negotiations_signal"] = 0
df["sanctions_signal"] = 0
df[pd.to_datetime(df["date"]) >= "2021-01-01"].to_csv("data-R.csv", index=False)
PYEOF
while IFS="|" read -r name train_till bt_from bt_till; do
  [ -z "$name" ] && continue
  DATASET=data-R.csv ENSEMBLE_FEATURES14="$FEAT18" ENSEMBLE_FEATURE_ORDER="$ORDER" \
    ENSEMBLE_TRAIN_TILL="$train_till" ENSEMBLE_EMBARGO_SPLIT="$train_till" \
    ENSEMBLE_EMBARGO_BARS=10 ENSEMBLE_OUT="ens-$name.json" \
    "$PY" "$T5/scripts/export_ensemble.py" > "logs/train-$name.log" 2>&1
  "$T5/backtest" -config "$T5/config.sandbox.yaml" -tickers "$TICKERS_17" \
    -from "$bt_from" -till "$bt_till" -deposit 1000000 -max-lots 1000 \
    -commission-rate 0.0005 -spread-pct 0.0005 -slippage-pct 0.0005 \
    -target-notional 15000 -lookback-days 0 -signal-source ensemble \
    -ensemble-path "ens-$name.json" -wf-dir wf -embargo-bars 10 -out "bt-$name.md" > "logs/bt-$name.log" 2>&1
  grep -E "Realized P&L \(closed|Max drawdown|Kill switch" "bt-$name.md" | head -3
done <<< "$WINDOWS"
echo ALLDONE
