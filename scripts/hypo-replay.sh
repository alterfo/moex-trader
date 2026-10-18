#!/bin/bash
set -euo pipefail
WORK=/home/oleg/moex-research/hypo
PY=/home/oleg/moex-venv/bin/python
BT="$WORK/backtest"
CFG=/home/oleg/moex-trader-daily/work/config.sandbox.yaml
T17="YDEX,OZON,SBER,LKOH,GAZP,GMKN,ROSN,NVTK,TATN,MTSS,MGNT,PLZL,CHMF,T,VTBR,RUAL,POSI"
VARIANTS="${VARIANTS:-base,rank,sigma}"
cd "$WORK"
"$PY" hypo_wf.py probs data0.csv prices.csv probs.csv "$VARIANTS"
mkdir -p bt
for v in ${VARIANTS//,/ }; do
  while IFS="|" read -r name from till; do
    "$BT" -config "$CFG" -tickers "$T17" -from "$from" -till "$till" -deposit 1000000 -max-lots 1000 \
      -commission-rate 0.0005 -spread-pct 0.0005 -slippage-pct 0.0005 -target-notional 15000 -lookback-days 0 \
      -max-net-short 90000 -max-net-long 90000 -borrow-pct-day 0.00005 \
      -signal-source csvprob -csv-prob-path probs.csv -csv-prob-col "p_$v" -csv-buy-pct 0.60 -csv-sell-pct 0.40 \
      -out "bt/$v-$name.md" > "bt/$v-$name.log" 2>&1
    echo "$v $name $(grep -E 'Realized P&L net of borrow|Max drawdown|Kill switch|Closed trades' "bt/$v-$name.md" | tr '\n' ' ' | cut -c1-260)"
  done <<'WIN'
q1|2025-03-05|2025-06-05
q2|2025-06-06|2025-09-05
q3|2025-09-06|2025-12-03
q4|2025-12-04|2026-03-04
w2|2026-03-05|2026-06-06
w1|2026-06-07|2026-09-16
WIN
done
echo ALLDONE
