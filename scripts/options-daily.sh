#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
mkdir -p bin data/options
go build -o bin/optfetch ./cmd/optfetch
FROM=$(date -v-4d +%F)
TILL=$(date +%F)
echo "=== $(date '+%F %T') options daily $FROM..$TILL"
bin/optfetch -mode chain || echo "chain failed"
bin/optfetch -mode day -from "$FROM" -till "$TILL" -workers 3 || echo "day pull failed"
bin/optfetch -mode cbr -from "$(date +%Y)-01-01" -till "$TILL" || echo "cbr failed"
