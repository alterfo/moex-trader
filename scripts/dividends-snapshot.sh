#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
mkdir -p bin data/dividends-snapshots
set -a
. "$ROOT/.env"
set +a
ASOF=$(date +%F)
LATEST=$(ls data/shadow-composite/scores-*.csv | tail -1)
TICKERS=$(awk -F, 'NR>2 && !seen[$2]++ {printf "%s%s", (n++ ? "," : ""), $2}' "$LATEST")
go run ./cmd/dividendscalendar -config config.sandbox.yaml -tickers "$TICKERS" -from "$ASOF" -till 2027-12-31 -out "data/dividends-snapshots/dividends-$ASOF.jsonl"
echo "$(date '+%F %T') dividend calendar snapshot: data/dividends-snapshots/dividends-$ASOF.jsonl ($(wc -l < data/dividends-snapshots/dividends-$ASOF.jsonl) records)"
