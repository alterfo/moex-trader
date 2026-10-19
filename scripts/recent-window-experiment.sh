#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT/work"
BIN="$ROOT/bin"
PY="${PY:-/home/oleg/moex-venv/bin/python}"
DATA="${DATA:-data-daily.csv}"
OUT="$ROOT/recent"
mkdir -p "$OUT"

T17="YDEX,OZON,SBER,LKOH,GAZP,GMKN,ROSN,NVTK,TATN,MTSS,MGNT,PLZL,CHMF,T,VTBR,RUAL,POSI"
FEAT18="return_pct,realized_volatility,news_sentiment,news_count,order_book_imbalance,mom_5d,mom_21d,mom_63d,reversal_1d,rsi_14,dist_ma20_pct,dist_ma50_pct,realized_vol_21d_annualized_pct,volume_zscore_20d,macd_hist_pct,stoch_k_14,williams_r_14,alligator_spread_pct"
ORDER="$FEAT18,event_dividend,event_buyback,event_sanctions,event_ipo,event_report,event_delisting,event_mna,event_default,negotiations_signal,sanctions_signal"

WINDOWS="q1|2025-03-05|2025-06-05
q2|2025-06-06|2025-09-05
q3|2025-09-06|2025-12-03
q4|2025-12-04|2026-03-04
w2|2026-03-05|2026-06-06
w1|2026-06-07|2026-09-16
h|2026-09-17|2026-10-17"
SIZES="all 90 30 10"

train_one() {
  local name="$1" s="$2" n="$3"
  local tag="$name-$n"
  [ -f "$OUT/m-$tag.json" ] && return 0
  local csv="$OUT/d-$tag.csv"
  if [ "$n" = all ]; then
    cp "$DATA" "$csv"
  else
    local e from
    e="$(date -u -d "$s -20 days" +%F)"
    from="$(date -u -d "$e -$n days" +%F)"
    awk -F, -v f="$from" 'NR==1 || $2 >= f' "$DATA" > "$csv"
  fi
  local till
  till="$(date -u -d "$s -1 day" +%F)"
  DATASET="$csv" ENSEMBLE_FEATURES14="$FEAT18" ENSEMBLE_FEATURE_ORDER="$ORDER" \
    ENSEMBLE_TRAIN_TILL="$till" ENSEMBLE_EMBARGO_SPLIT="$till" ENSEMBLE_EMBARGO_BARS=10 \
    ENSEMBLE_OUT="$OUT/m-$tag.json" "$PY" "$ROOT/scripts/export_ensemble.py" > "$OUT/train-$tag.log" 2>&1
  awk -F, 'NR>1{c++} END{print c+0}' "$csv" > "$OUT/rows-$tag.txt"
}

bt_one() {
  local name="$1" s="$2" e="$3" n="$4" member="$5"
  local tag="$name-$n-$member"
  [ -f "$OUT/bt-$tag.md" ] && return 0
  local extra=()
  [ "$member" = logistic ] && extra=(-ensemble-member logistic)
  "$BIN/backtest" -config config.sandbox.yaml -tickers "$T17" -from "$s" -till "$e" \
    -deposit 1000000 -max-lots 1000 -commission-rate 0.0005 -spread-pct 0.0005 -slippage-pct 0.0005 \
    -target-notional 15000 -lookback-days 0 -signal-source ensemble -ensemble-path "$OUT/m-$name-$n.json" \
    -news-history news_history.jsonl "${extra[@]}" -out "$OUT/bt-$tag.md" > "$OUT/bt-$tag.log" 2>&1
}

while IFS="|" read -r name s e; do
  for n in $SIZES; do train_one "$name" "$s" "$n"; done
done <<< "$WINDOWS"

jobs_file="$OUT/jobs.txt"
: > "$jobs_file"
while IFS="|" read -r name s e; do
  for n in $SIZES; do
    for member in ens logistic; do echo "$name $s $e $n $member" >> "$jobs_file"; done
  done
done <<< "$WINDOWS"
export -f bt_one
export ROOT BIN OUT T17
xargs -P 3 -L 1 bash -c 'bt_one "$@"' _ < "$jobs_file"

echo "window,size,member,rows,realized,net_mtm,trades,max_dd_pct" > "$OUT/summary.csv"
while IFS="|" read -r name s e; do
  for n in $SIZES; do
    for member in ens logistic; do
      f="$OUT/bt-$name-$n-$member.md"
      [ -f "$f" ] || continue
      r=$(grep -m1 "^- Realized P&L" "$f" | sed -E 's/.*\*\*(-?[0-9.]+)\*\*.*/\1/')
      m=$(grep -m1 "^- Net P&L" "$f" | sed -E 's/.*\*\*(-?[0-9.]+)\*\*.*/\1/')
      t=$(grep -m1 "^- Closed trades" "$f" | sed -E 's/[^0-9]*([0-9]+).*/\1/')
      d=$(grep -m1 "^- Max drawdown" "$f" | sed -E 's/[^0-9]*([0-9.]+)%.*/\1/')
      echo "$name,$n,$member,$(cat "$OUT/rows-$name-$n.txt"),$r,$m,$t,$d" >> "$OUT/summary.csv"
    done
  done
done <<< "$WINDOWS"
echo "done $(date -u +%FT%TZ)"
