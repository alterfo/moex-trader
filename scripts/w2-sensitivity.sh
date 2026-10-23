#!/bin/bash
set -euo pipefail
WORK="${WORK:-/home/oleg/moex-rerun/w2-sensitivity}"
BIN="${BIN:-/home/oleg/moex-rerun/bin}"
CONFIG="${CONFIG:-/home/oleg/moex-rerun/config.sandbox.yaml}"
SCORES="${SCORES:-/home/oleg/moex-shadow/out/ridge_table.csv}"
SHA_PREFIX="${SHA_PREFIX:-a34e7854}"
FROM="${FROM:-2025-03-05}"
TILL="${TILL:-2026-10-18}"
BASE_REALIZED="${BASE_REALIZED:--2788}"
BASE_TRADES="${BASE_TRADES:-205}"
BORROW="${BORROW:-0.00005}"
T17="YDEX,OZON,SBER,LKOH,GAZP,GMKN,ROSN,NVTK,TATN,MTSS,MGNT,PLZL,CHMF,T,VTBR,RUAL,POSI"
mkdir -p "$WORK"
cd "$WORK"

actual=$(sha256sum "$SCORES" | cut -d' ' -f1)
case "$actual" in
  "$SHA_PREFIX"*) ;;
  *) echo "score table sha256 $actual does not start with $SHA_PREFIX" >&2; exit 2 ;;
esac

[ -f results.csv ] || echo "id,kind,tickers_dropped,buy,sell,realized,trades" > results.csv

run_one() {
  local id=$1 kind=$2 dropped=$3 buy=$4 sell=$5 tickers=$6
  if grep -q "^$id," results.csv; then
    return
  fi
  "$BIN/backtest" -config "$CONFIG" -tickers "$tickers" -from "$FROM" -till "$TILL" \
    -deposit 1000000 -max-lots 1000 -commission-rate 0.0005 -spread-pct 0.0005 -slippage-pct 0.0005 \
    -borrow-pct-day "$BORROW" -max-net-long 90000 -max-net-short 90000 -kill-switch=true \
    -target-notional 15000 -lookback-days 0 -signal-source csvprob -csv-prob-path "$SCORES" \
    -csv-prob-col p -csv-buy-pct "$buy" -csv-sell-pct "$sell" -out "bt-$id.md" > "bt-$id.log" 2>&1 \
    || { echo "$id,$kind,$dropped,$buy,$sell,,"; echo "$id,$kind,$dropped,$buy,$sell,," >> results.csv; return; }
  local r t
  r=$(grep -m1 "^- Realized P&L net of borrow" "bt-$id.md" | sed -E 's/.*\*\*(-?[0-9.]+)[^*]*\*\*.*/\1/')
  t=$(grep -m1 "^- Closed trades" "bt-$id.md" | sed -E 's/[^0-9]*([0-9]+).*/\1/')
  echo "$id,$kind,$dropped,$buy,$sell,$r,$t" | tee -a results.csv
}

run_one base base - 0.60 0.40 "$T17"

row=$(grep "^base," results.csv)
r=$(echo "$row" | cut -d, -f6)
t=$(echo "$row" | cut -d, -f7)
r_int=$(awk -v x="$r" 'BEGIN{printf "%.0f", x}')
if [ "$r_int" != "$BASE_REALIZED" ] || [ "$t" != "$BASE_TRADES" ]; then
  echo "STOP: base is $r RUB / $t trades, ledger says $BASE_REALIZED / $BASE_TRADES" >&2
  exit 3
fi

for buy in 0.58 0.60 0.62; do
  for sell in 0.38 0.40 0.42; do
    [ "$buy" = 0.60 ] && [ "$sell" = 0.40 ] && continue
    run_one "thr-$buy-$sell" threshold - "$buy" "$sell" "$T17"
  done
done

IFS=, read -ra all <<< "$T17"
for drop in "${all[@]}"; do
  rest=$(echo "$T17" | tr ',' '\n' | grep -vx "$drop" | paste -sd, -)
  run_one "drop-$drop" dropone "$drop" 0.60 0.40 "$rest"
done

echo "done: $(grep -c . results.csv) rows in $WORK/results.csv"
