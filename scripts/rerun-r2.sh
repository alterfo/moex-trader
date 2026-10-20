#!/bin/bash
set -euo pipefail
WORK="${WORK:-/home/oleg/moex-rerun/r2}"
BIN="${BIN:-/home/oleg/moex-rerun/bin}"
SCRIPTS="${SCRIPTS:-/home/oleg/moex-trader-task12/scripts}"
NEWS="${NEWS:-/home/oleg/moex-rerun/news_history.jsonl}"
CONFIG="${CONFIG:-/home/oleg/moex-rerun/config.sandbox.yaml}"
PY="${PY:-/home/oleg/moex-venv/bin/python}"
DFREEZE="${DFREEZE:-2026-10-16}"
FEAT18="return_pct,realized_volatility,news_sentiment,news_count,order_book_imbalance,mom_5d,mom_21d,mom_63d,reversal_1d,rsi_14,dist_ma20_pct,dist_ma50_pct,realized_vol_21d_annualized_pct,volume_zscore_20d,macd_hist_pct,stoch_k_14,williams_r_14,alligator_spread_pct"
ORDER="$FEAT18,event_dividend,event_buyback,event_sanctions,event_ipo,event_report,event_delisting,event_mna,event_default,negotiations_signal,sanctions_signal"
T17="YDEX,OZON,SBER,LKOH,GAZP,GMKN,ROSN,NVTK,TATN,MTSS,MGNT,PLZL,CHMF,T,VTBR,RUAL,POSI"
WINDOWS="q1|2025-03-04|2025-03-05|2025-06-05
q2|2025-06-05|2025-06-06|2025-09-05
q3|2025-09-05|2025-09-06|2025-12-03
q4|2025-12-03|2025-12-04|2026-03-04
w2|2026-03-04|2026-03-05|2026-06-06
w1|2026-06-06|2026-06-07|2026-09-16
x|2026-09-16|2026-09-17|$DFREEZE"
mkdir -p "$WORK/logs" "$WORK/wf"
cd "$WORK"
for variant in abs exc; do
  [ -f "data-$variant.csv" ] || "$BIN/exportdataset" -config "$CONFIG" -tickers "$T17" \
    -from 2024-09-01 -till "$DFREEZE" -split 2026-06-18 -horizon-days 10 -deadband-pct 0.5 \
    -label-mode "$([ $variant = abs ] && echo absolute || echo excess)" \
    -news-history "$NEWS" -out "data-$variant.csv" > "logs/export-$variant.log" 2>&1
done
run_bt() {
  local tag="$1" model="$2" from="$3" till="$4" member="$5"
  local extra=()
  [ "$member" = logistic ] && extra=(-ensemble-member logistic)
  "$BIN/backtest" -config "$CONFIG" -tickers "$T17" -from "$from" -till "$till" \
    -deposit 1000000 -max-lots 1000 -commission-rate 0.0005 -spread-pct 0.0005 -slippage-pct 0.0005 \
    -target-notional 15000 -lookback-days 0 -signal-source ensemble -ensemble-path "$model" \
    -news-history "$NEWS" "${extra[@]}" -wf-dir "wf/$tag" -embargo-bars 10 \
    -out "bt-$tag.md" > "logs/bt-$tag.log" 2>&1
}
while IFS="|" read -r name train_till bt_from bt_till; do
  for variant in abs exc; do
    [ -f "ens-$variant-$name.json" ] || DATASET="data-$variant.csv" ENSEMBLE_FEATURES14="$FEAT18" ENSEMBLE_FEATURE_ORDER="$ORDER" \
      ENSEMBLE_TRAIN_TILL="$train_till" ENSEMBLE_EMBARGO_SPLIT="$train_till" ENSEMBLE_EMBARGO_BARS=10 \
      ENSEMBLE_OUT="ens-$variant-$name.json" "$PY" "$SCRIPTS/export_ensemble.py" > "logs/train-$variant-$name.log" 2>&1
    grep -E "val_auc" "logs/train-$variant-$name.log" | tail -1
  done
  run_bt "ctl-$name" "ens-abs-$name.json" "$bt_from" "$bt_till" ens
  run_bt "clg-$name" "ens-abs-$name.json" "$bt_from" "$bt_till" logistic
  run_bt "f-$name" "ens-exc-$name.json" "$bt_from" "$bt_till" ens
  for t in ctl clg f; do
    echo "$t-$name $(grep -m1 '^- Realized P&L' bt-$t-$name.md) $(grep -m1 '^- Max drawdown' bt-$t-$name.md)"
  done
done <<< "$WINDOWS"
echo ALLDONE
