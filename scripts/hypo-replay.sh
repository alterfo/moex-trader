#!/bin/bash
set -euo pipefail
WORK=/home/oleg/moex-research/hypo
PY=/home/oleg/moex-venv/bin/python
BT="$WORK/backtest"
CFG=/home/oleg/moex-trader-daily/work/config.sandbox.yaml
T17="YDEX,OZON,SBER,LKOH,GAZP,GMKN,ROSN,NVTK,TATN,MTSS,MGNT,PLZL,CHMF,T,VTBR,RUAL,POSI"
SPECS="${SPECS:-base@0,base@1,base@2,base@3,base@4,sigma@0,sigma@1,sigma@2,sigma@3,sigma@4,base@42,sigma25@42,sigma75@42,sigma@42}"
OUT="${OUT:-bt17}"
cd "$WORK"
"$PY" hypo_wf.py probs data0.csv prices.csv probs17.csv "$SPECS"
mkdir -p "$OUT"
run_one() {
  local spec="$1" name="$2" from="$3" till="$4"
  local arm="${spec%@*}" seed="${spec#*@}"
  local tag="${arm}_s${seed}-${name}"
  "$BT" -config "$CFG" -tickers "$T17" -from "$from" -till "$till" -deposit 1000000 -max-lots 1000 \
    -commission-rate 0.0005 -spread-pct 0.0005 -slippage-pct 0.0005 -target-notional 15000 -lookback-days 0 \
    -max-net-short 90000 -max-net-long 90000 -borrow-pct-day 0.00005 \
    -signal-source csvprob -csv-prob-path probs17.csv -csv-prob-col "p_${arm}_s${seed}" -csv-buy-pct 0.60 -csv-sell-pct 0.40 \
    -out "$OUT/$tag.md" > "$OUT/$tag.log" 2>&1
  r=$(grep -a "Realized P&L net of borrow" "$OUT/$tag.md" | grep -o '\*\*-\?[0-9.]*' | tr -d '*')
  d=$(grep -a "Max drawdown" "$OUT/$tag.md" | grep -o '[0-9.]*%' | head -1)
  t=$(grep -a "Closed trades:" "$OUT/$tag.md" | head -1 | grep -o '[0-9]*$')
  echo "$arm,$seed,$name,$r,$d,$t" >> "$OUT/summary.csv"
}
echo "arm,seed,window,realized_net,max_dd,closed" > "$OUT/summary.csv"
export -f run_one
export BT CFG T17 OUT
JOBS=()
for spec in ${SPECS//,/ }; do
  while IFS="|" read -r name from till; do
    JOBS+=("$spec|$name|$from|$till")
  done <<'WIN'
q1|2025-03-05|2025-06-05
q2|2025-06-06|2025-09-05
q3|2025-09-06|2025-12-03
q4|2025-12-04|2026-03-04
w2|2026-03-05|2026-06-06
w1|2026-06-07|2026-09-16
h|2026-09-17|2026-10-17
WIN
done
printf '%s\n' "${JOBS[@]}" | xargs -P 4 -I{} bash -c 'IFS="|" read -r spec name from till <<< "{}"; run_one "$spec" "$name" "$from" "$till"'
echo ALLDONE
