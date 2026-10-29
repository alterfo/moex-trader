#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
mkdir -p bin data
go build -o bin/borrowterms ./cmd/borrowterms
set -a
. "$ROOT/.env"
set +a
echo "=== $(date '+%F %T') borrowterms"
bin/borrowterms -config config.sandbox.yaml \
  -tickers "$(cat "${TICKERS_FILE:-$ROOT/data/borrow_universe.txt}")" \
  -out data/borrow_terms.jsonl
