#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT/work"
BIN="$ROOT/bin"
PY="${PY:-/home/oleg/moex-venv/bin/python}"

if [ -z "${LAST:-}" ]; then
  LAST="$(date -u -d yesterday +%F)"
  while [ "$(date -u -d "$LAST" +%u)" -ge 6 ]; do
    LAST="$(date -u -d "$LAST -1 day" +%F)"
  done
fi
if [ -f last_done ] && [ "$(cat last_done)" = "$LAST" ] && [ -z "${FORCE:-}" ]; then
  echo "already done for $LAST"
  exit 0
fi
TODAY="$(date -u -d "$LAST +1 day" +%F)"
H30="$(date -u -d "$LAST -30 days" +%F)"
P90="$(date -u -d "$LAST -89 days" +%F)"

T18="YDEX,OZON,SBER,LKOH,GAZP,GMKN,ROSN,NVTK,TATN,MTSS,MGNT,PLZL,CHMF,T,VTBR,RUAL,POSI,DATA"
T17="YDEX,OZON,SBER,LKOH,GAZP,GMKN,ROSN,NVTK,TATN,MTSS,MGNT,PLZL,CHMF,T,VTBR,RUAL,POSI"
FEAT18="return_pct,realized_volatility,news_sentiment,news_count,order_book_imbalance,mom_5d,mom_21d,mom_63d,reversal_1d,rsi_14,dist_ma20_pct,dist_ma50_pct,realized_vol_21d_annualized_pct,volume_zscore_20d,macd_hist_pct,stoch_k_14,williams_r_14,alligator_spread_pct"
ORDER="$FEAT18,event_dividend,event_buyback,event_sanctions,event_ipo,event_report,event_delisting,event_mna,event_default,negotiations_signal,sanctions_signal"

OUT="$ROOT/reports/$LAST"
mkdir -p "$OUT"
echo "last=$LAST hold30=$H30 pre90=$P90 start=$(date -u +%FT%TZ)" | tee "$OUT/meta.txt"

"$BIN/exportdataset" -config config.sandbox.yaml -tickers "$T18" -from 2024-01-01 -till "$TODAY" \
  -split "$TODAY" -horizon-days 10 -deadband-pct 0.5 -label-mode absolute \
  -news-history news_history.jsonl -out data-daily.csv > "$OUT/export.log" 2>&1

train() {
  local till="$1" out="$2" log="$3"
  DATASET=data-daily.csv ENSEMBLE_FEATURES14="$FEAT18" ENSEMBLE_FEATURE_ORDER="$ORDER" \
    ENSEMBLE_TRAIN_TILL="$till" ENSEMBLE_EMBARGO_SPLIT="$till" ENSEMBLE_EMBARGO_BARS=10 \
    ENSEMBLE_OUT="$out" "$PY" "$ROOT/scripts/export_ensemble.py" > "$log" 2>&1
}
train "$LAST" candidate.json "$OUT/train.log"
train "$(date -u -d "$H30 -1 day" +%F)" lag30.json "$OUT/train-lag30.log"
train "$(date -u -d "$P90 -1 day" +%F)" lag90.json "$OUT/train-lag90.log"

cfg_value() {
  awk -v key="$1" '$1 == key":" { gsub(/"/, "", $2); print $2; exit }' config.sandbox.yaml
}
NET_SHORT="$(cfg_value max_net_short)"
NET_LONG="$(cfg_value max_net_long)"
[ -n "$NET_SHORT" ] && [ -n "$NET_LONG" ] || { echo "max_net_short/max_net_long missing in config.sandbox.yaml" >&2; exit 1; }
BORROW_PCT_DAY="0.00005"

replay() {
  local model="$1" file="$2" from="$3" name="$4"
  "$BIN/backtest" -config config.sandbox.yaml -max-net-short "$NET_SHORT" -max-net-long "$NET_LONG" -borrow-pct-day "$BORROW_PCT_DAY" -tickers "$T17" -from "$from" -till "$LAST" \
    -deposit 1000000 -max-lots 1000 -commission-rate 0.0005 -spread-pct 0.0005 -slippage-pct 0.0005 \
    -target-notional 15000 -lookback-days 0 -signal-source ensemble -ensemble-path "$file" \
    -news-history news_history.jsonl -out "$OUT/bt-$model-$name.md" > "$OUT/bt-$model-$name.log" 2>&1
}
replay deployed deployed.json "$H30" hold30
replay candidate lag30.json "$H30" hold30
replay deployed deployed.json "$P90" pre90
replay candidate lag90.json "$P90" pre90
cp candidate.json "$OUT/candidate.json"
echo "$LAST" > last_done
echo "done=$(date -u +%FT%TZ)" | tee -a "$OUT/meta.txt"
