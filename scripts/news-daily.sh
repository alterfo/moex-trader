#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

CONFIG="${CONFIG:-config.sandbox.yaml}"
OUT="${OUT:-$ROOT/data/news_history.jsonl}"
QUARANTINE="${QUARANTINE:-$ROOT/data/news_quarantine.jsonl}"
RSS_RAW="${RSS_RAW:-$ROOT/data/news_raw_rss.jsonl}"
SPOOL="${SPOOL:-$ROOT/data/news_spool.jsonl}"
SINCE_DAYS="${SINCE_DAYS:-3}"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

mkdir -p bin
go build -o bin/newsfetch ./cmd/newsfetch
go build -o bin/newsimport ./cmd/newsimport
go build -o bin/newsclean ./cmd/newsclean

bin/newsfetch -config "$CONFIG" -since-days "$SINCE_DAYS" -out "$RSS_RAW" -merge-spool "$SPOOL"

cp "$OUT" "$WORK/merged.jsonl"
ARGS=(-config "$CONFIG" -out "$WORK/merged.jsonl" -names data/ticker_names.json -alias-blocklist data/news_alias_blocklist.json -aliases-out "$WORK/aliases_unused.json")
for f in "$RSS_RAW" "$ROOT"/data/news_raw_tg_live.jsonl; do
  [ -f "$f" ] && ARGS+=(-jsonl "$f")
done
bin/newsimport "${ARGS[@]}"
bin/newsclean -in "$WORK/merged.jsonl" -out "$WORK/clean.jsonl" -quarantine "$WORK/quarantine.jsonl" -names data/ticker_names.json -alias-blocklist data/news_alias_blocklist.json -aliases data/ticker_aliases.json

before=$(wc -l < "$OUT")
after=$(wc -l < "$WORK/clean.jsonl")
if [ "$after" -lt "$before" ]; then
  echo "refusing to install: clean archive shrank from $before to $after" >&2
  exit 1
fi
cp "$WORK/clean.jsonl" "$OUT.tmp" && mv "$OUT.tmp" "$OUT"
cp "$WORK/quarantine.jsonl" "$QUARANTINE.new"
cat "$QUARANTINE" "$QUARANTINE.new" 2>/dev/null | sort -u > "$QUARANTINE.tmp" && mv "$QUARANTINE.tmp" "$QUARANTINE"
rm -f "$QUARANTINE.new"
echo "$(date -u +%FT%TZ) archive $before -> $after pairs"
