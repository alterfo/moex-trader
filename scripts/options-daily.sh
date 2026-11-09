#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
mkdir -p bin data/options
go build -o bin/optfetch ./cmd/optfetch
go build -o bin/optscan ./cmd/optscan
FROM=$(date -v-4d +%F)
TILL=$(date +%F)
echo "=== $(date '+%F %T') options daily $FROM..$TILL"
bin/optfetch -mode chain || echo "chain failed"
bin/optfetch -mode day -from "$FROM" -till "$TILL" -workers 3 || echo "day pull failed"
bin/optfetch -mode cbr -from "$(date +%Y)-01-01" -till "$TILL" || echo "cbr failed"
bin/optfetch -mode series -prefix BR -from "$(date -v-30d +%F)" -till "$TILL" -workers 2 -refresh || echo "BR series refresh failed"
bin/optscan -mode fwd -families BR -date "$(date -v-1d +%F)" || echo "forward snapshot failed"
bin/optscan -mode fwd -families BR -date "$TILL" || echo "forward snapshot failed"
bin/optscan -mode fwdsettle -families BR -target 5 -slack 3 -ivrv 0.06 || echo "forward settle failed"
