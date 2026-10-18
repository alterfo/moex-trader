#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

BASE="${BASE:?BASE: premerge archive (finanalys-format JSONL) to start from}"
PARQUET_DIR="${PARQUET_DIR:?PARQUET_DIR: directory with Kasymkhan train-*.parquet and test-*.parquet}"
NEWSSENT="${NEWSSENT:-$HOME/dev/fin/newssent/data}"
ISS_CONVERTED="${ISS_CONVERTED:-}"
OUT="${OUT:-$ROOT/data/news_history.jsonl}"
QUARANTINE="${QUARANTINE:-$ROOT/data/news_quarantine.jsonl}"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

go build -o "$WORK/newsimport" ./cmd/newsimport
go build -o "$WORK/newsclean" ./cmd/newsclean

cp "$BASE" "$WORK/merged.jsonl"
ARGS=(-config config.yaml -out "$WORK/merged.jsonl" -names data/ticker_names.json -alias-blocklist data/news_alias_blocklist.json)
for f in "$PARQUET_DIR"/train-*.parquet "$PARQUET_DIR"/test-*.parquet; do ARGS+=(-parquet "$f"); done
ARGS+=(-jsonl "$BASE")
for f in news_all.jsonl news_backfill.jsonl live/news_merged.jsonl live/wires.jsonl live/news_live.jsonl; do
  [ -f "$NEWSSENT/$f" ] && ARGS+=(-jsonl "$NEWSSENT/$f")
done
[ -n "$ISS_CONVERTED" ] && ARGS+=(-jsonl "$ISS_CONVERTED")
for f in "$ROOT"/data/news_raw_tg*.jsonl; do
  [ -f "$f" ] && ARGS+=(-jsonl "$f")
done

"$WORK/newsimport" "${ARGS[@]}"
"$WORK/newsclean" -in "$WORK/merged.jsonl" -out "$WORK/clean.jsonl" -quarantine "$WORK/quarantine.jsonl"

cp "$WORK/clean.jsonl" "$OUT.tmp" && mv "$OUT.tmp" "$OUT"
cp "$WORK/quarantine.jsonl" "$QUARANTINE"
echo "installed $(wc -l < "$OUT") pairs into $OUT, quarantine $(wc -l < "$QUARANTINE")"
