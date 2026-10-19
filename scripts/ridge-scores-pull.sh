#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
mkdir -p data
tmp="data/ridge_scores.csv.new"
scp -q -o BatchMode=yes oleg@192.168.88.193:moex-shadow/out/ridge_table.csv "$tmp"
head -1 "$tmp" | grep -q '^ticker,date,p$'
test "$(wc -l < "$tmp")" -gt 1000
mv "$tmp" data/ridge_scores.csv
echo "=== $(date '+%F %T') ridge scores updated: $(tail -1 data/ridge_scores.csv)"
