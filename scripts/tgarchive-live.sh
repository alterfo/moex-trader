#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
mkdir -p bin
go build -o bin/tgarchive ./cmd/tgarchive
SINCE="$(date -v-3d +%F)"
exec bin/tgarchive -config config.sandbox.yaml -out data/news_raw_tg_live.jsonl -since "$SINCE" -delay 1500ms
