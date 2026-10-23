#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
mkdir -p bin data/algopack
go build -o bin/algopackfetch ./cmd/algopackfetch
set -a
. "$ROOT/.env"
set +a
DAYS=""
for back in 0 1 2 3; do
  d=$(date -v-${back}d +%F)
  DAYS="${DAYS:+$DAYS,}$d"
  for ds in tradestats obstats orderstats hi2; do
    rm -f "data/algopack/$ds/$d.csv.gz"
  done
done
echo "=== $(date '+%F %T') algopack daily $DAYS"
bin/algopackfetch -dates "$DAYS" -workers 8 -out data/algopack
