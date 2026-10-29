#!/bin/bash
set -euo pipefail
WORK="${WORK:-/home/oleg/moex-research/memberagree}"
REPO="${REPO:-/home/oleg/moex-trader}"
PY="${PY:-/home/oleg/moex-venv/bin/python}"
TRAIN="${TRAIN:-$WORK/export_ensemble.py}"
BT="${BT:-$WORK/bin/backtest}"
CONFIG="${CONFIG:-$REPO/config.sandbox.yaml}"
DATASET="${DATASET:-/home/oleg/moex-rerun/r2/data-abs.csv}"
DATASET_SHA="${DATASET_SHA:-e9898b68e8e98964fc01482742c0530029931142690cae391b75a172c75ff140}"
NEWS="${NEWS:-$WORK/news-20261029.jsonl}"
NEWS_SHA="${NEWS_SHA:-536979e8b2f8fc4fcbfe3ab4ececdfe64d692fedbf240860fbef166a5f7dcf1b}"
FEAT18="return_pct,realized_volatility,news_sentiment,news_count,order_book_imbalance,mom_5d,mom_21d,mom_63d,reversal_1d,rsi_14,dist_ma20_pct,dist_ma50_pct,realized_vol_21d_annualized_pct,volume_zscore_20d,macd_hist_pct,stoch_k_14,williams_r_14,alligator_spread_pct"
ORDER="return_pct,realized_volatility,news_sentiment,news_count,order_book_imbalance,mom_5d,mom_21d,mom_63d,reversal_1d,rsi_14,dist_ma20_pct,dist_ma50_pct,realized_vol_21d_annualized_pct,volume_zscore_20d,macd_hist_pct,stoch_k_14,williams_r_14,alligator_spread_pct,event_dividend,event_buyback,event_sanctions,event_ipo,event_report,event_delisting,event_mna,event_default,negotiations_signal,sanctions_signal"
TICKERS_17="YDEX,OZON,SBER,LKOH,GAZP,GMKN,ROSN,NVTK,TATN,MTSS,MGNT,PLZL,CHMF,T,VTBR,RUAL,POSI"
SEEDS="0 1 2 3 4 42"
WINDOWS="q1|2025-03-04|2025-03-05|2025-06-05
q2|2025-06-05|2025-06-06|2025-09-05
q3|2025-09-05|2025-09-06|2025-12-03
q4|2025-12-03|2025-12-04|2026-03-04
w2|2026-03-04|2026-03-05|2026-06-06
w1|2026-06-06|2026-06-07|2026-09-16"

actual=$(sha256sum "$DATASET" | cut -d' ' -f1)
[ "$actual" = "$DATASET_SHA" ] || { echo "dataset sha256 $actual != $DATASET_SHA" >&2; exit 2; }
actual=$(sha256sum "$NEWS" | cut -d' ' -f1)
[ "$actual" = "$NEWS_SHA" ] || { echo "news sha256 $actual != $NEWS_SHA" >&2; exit 2; }

mkdir -p "$WORK/models" "$WORK/rows" "$WORK/logs"
cd "$WORK"

echo "=== TRAIN ==="
while IFS="|" read -r name train_till bt_from bt_till; do
  [ -z "$name" ] && continue
  for seed in $SEEDS; do
    out="models/ens-s${seed}-${name}.json"
    if [ -s "$out" ]; then
      echo "== skip existing $out"
      continue
    fi
    echo "== train window=$name seed=$seed till=$train_till"
    DATASET="$DATASET" ENSEMBLE_FEATURES14="$FEAT18" ENSEMBLE_FEATURE_ORDER="$ORDER" \
      ENSEMBLE_TRAIN_TILL="$train_till" ENSEMBLE_EMBARGO_SPLIT="$train_till" \
      ENSEMBLE_EMBARGO_BARS=10 ENSEMBLE_SEED="$seed" ENSEMBLE_OUT="$out" \
      "$PY" "$TRAIN" > "logs/train-s${seed}-${name}.log" 2>&1
    grep -E "wrote|val_auc" "logs/train-s${seed}-${name}.log" | tail -2
  done
done <<< "$WINDOWS"

echo "=== BACKTEST ==="
while IFS="|" read -r name train_till bt_from bt_till; do
  [ -z "$name" ] && continue
  for seed in $SEEDS; do
    rows="rows/s${seed}-${name}.csv"
    if [ -s "$rows" ]; then
      echo "== skip existing $rows"
      continue
    fi
    echo "== backtest window=$name seed=$seed $bt_from -> $bt_till"
    "$BT" -config "$CONFIG" -tickers "$TICKERS_17" \
      -from "$bt_from" -till "$bt_till" -deposit 1000000 -max-lots 1000 \
      -commission-rate 0.0005 -spread-pct 0.0005 -slippage-pct 0.0005 \
      -target-notional 15000 -lookback-days 0 -signal-source ensemble \
      -ensemble-path "models/ens-s${seed}-${name}.json" -news-history "$NEWS" \
      -members-csv "$rows" -members-window "$name" -members-seed "$seed" \
      -out "logs/bt-s${seed}-${name}.md" > "logs/bt-s${seed}-${name}.log" 2>&1
    grep -E "Realized P&L|Closed trades|Max drawdown|member rows" "logs/bt-s${seed}-${name}.md" "logs/bt-s${seed}-${name}.log" | head -4
  done
done <<< "$WINDOWS"

echo "=== DONE ==="
