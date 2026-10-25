#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
BOX=oleg@192.168.88.193
STUDY=algopack-study
ASOF=$(date +%F)
FROM=${SHADOW_FROM:-2026-10-22}
export NO_PROXY=192.168.88.193
mkdir -p data/shadow-composite bin
GOOS=linux GOARCH=amd64 go build -o bin/alphafit-linux ./cmd/alphafit
GOOS=linux GOARCH=amd64 go build -o bin/flowscan-linux ./cmd/flowscan
scp -q bin/alphafit-linux bin/flowscan-linux "$BOX:$STUDY/bin/"
scp -q data/dividends.jsonl "$BOX:$STUDY/data/dividends.jsonl"
for back in 0 1 2 3 4 5 6; do
  d=$(date -v-${back}d +%F)
  for ds in tradestats obstats orderstats hi2; do
    f="data/algopack/$ds/$d.csv.gz"
    if [ -s "$f" ]; then
      scp -q "$f" "$BOX:$STUDY/data/algopack/$ds/$d.csv.gz"
    fi
  done
done
TILL=$(date -v-1d +%F)
ssh "$BOX" "cd $STUDY && bin/flowscan-linux -mode matrix -dir data/algopack -from 2020-01-01 -till $TILL -lag 30 -div data/dividends.jsonl -out work/matrix-shadow.csv > work/matrix-shadow.log 2>&1 && bin/alphafit-linux -mode shadow -matrix work/matrix-shadow.csv -models frozen/alphafit-models-20261023.json -from $FROM -out work/shadow-$ASOF.csv"
scp -q "$BOX:$STUDY/work/shadow-$ASOF.csv" "data/shadow-composite/scores-$ASOF.csv"
echo "$(date '+%F %T') composite shadow scores written: data/shadow-composite/scores-$ASOF.csv"
