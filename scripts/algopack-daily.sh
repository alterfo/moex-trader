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
  for m in fo fx; do
    for ds in tradestats obstats orderstats; do
      rm -f "data/algopack-$m/$ds/$d.csv.gz"
    done
  done
done
echo "=== $(date '+%F %T') algopack daily $DAYS"
bin/algopackfetch -dates "$DAYS" -workers 8 -out data/algopack
for m in fo fx; do
  bin/algopackfetch -market "$m" -dates "$DAYS" -workers 8 -out "data/algopack-$m"
done
scripts/ridge-scores-pull.sh || echo "ridge pull failed"
go build -o bin/algopackgrafana ./cmd/algopackgrafana
bin/algopackgrafana -out data/grafana/algopack.db
rsync -a --inplace data/grafana/algopack.db "${AIBOX:-oleg@192.168.88.193}:monitoring/grafana/data/algopack.db" || echo "grafana sync failed"
