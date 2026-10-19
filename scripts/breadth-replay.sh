#!/bin/bash
set -euo pipefail
WORK=/home/oleg/moex-research/hypo
PY=/home/oleg/moex-venv/bin/python
BT="$WORK/backtest"
CFG=/home/oleg/moex-trader-daily/work/config.sandbox.yaml
T17="YDEX,OZON,SBER,LKOH,GAZP,GMKN,ROSN,NVTK,TATN,MTSS,MGNT,PLZL,CHMF,T,VTBR,RUAL,POSI"
OUT="${OUT:-bt24}"
ARMS="${ARMS:-base18 wide wide2020}"
cd "$WORK"
for s in 0 1 2 3 4; do
  specs=""
  for a in $ARMS; do specs="$specs,${a}@${s}"; done
  "$PY" breadth_wf.py probs "ds_shard_a?.csv" candles_clean.csv "probs_b_s${s}.csv" "${specs#,}" &
done
wait
"$PY" - <<PYEOF
import pandas as pd
d = None
for s in range(5):
    x = pd.read_csv(f"probs_b_s{s}.csv")
    d = x if d is None else d.merge(x.drop(columns=["window"]), on=["ticker", "date"], how="outer")
d.to_csv("probs24.csv", index=False)
print(d.shape)
PYEOF
mkdir -p "$OUT"
echo "arm,seed,window,realized_net,max_dd,closed" > "$OUT/summary.csv"
run_one() {
  local arm="$1" seed="$2" name="$3" from="$4" till="$5"
  local tag="${arm}_s${seed}-${name}"
  "$BT" -config "$CFG" -tickers "$T17" -from "$from" -till "$till" -deposit 1000000 -max-lots 1000 \
    -commission-rate 0.0005 -spread-pct 0.0005 -slippage-pct 0.0005 -target-notional 15000 -lookback-days 0 \
    -max-net-short 90000 -max-net-long 90000 -borrow-pct-day 0.00005 \
    -signal-source csvprob -csv-prob-path probs24.csv -csv-prob-col "p_${arm}_s${seed}" -csv-buy-pct 0.60 -csv-sell-pct 0.40 \
    -out "$OUT/$tag.md" > "$OUT/$tag.log" 2>&1
  r=$(grep -a "Realized P&L net of borrow" "$OUT/$tag.md" | grep -o '\*\*-\?[0-9.]*' | tr -d '*')
  d=$(grep -a "Max drawdown" "$OUT/$tag.md" | grep -o '[0-9.]*%' | head -1)
  t=$(grep -a "Closed trades:" "$OUT/$tag.md" | head -1 | grep -o '[0-9]*$')
  echo "$arm,$seed,$name,$r,$d,$t" >> "$OUT/summary.csv"
}
export -f run_one
export BT CFG T17 OUT
JOBS=()
for a in $ARMS; do
  for s in 0 1 2 3 4; do
    while IFS="|" read -r name from till; do
      JOBS+=("$a|$s|$name|$from|$till")
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
done
printf '%s\n' "${JOBS[@]}" | xargs -P 4 -I{} bash -c 'IFS="|" read -r arm seed name from till <<< "{}"; run_one "$arm" "$seed" "$name" "$from" "$till"'
echo ALLDONE
