#!/bin/bash
set -euo pipefail
REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BOX="${BOX:-oleg@192.168.88.193}"
STATE_DIR="$HOME/Library/Application Support/moex-daily-retrain"
LOG="$HOME/Library/Logs/moex-daily-retrain.log"
mkdir -p "$STATE_DIR/reports"
exec >>"$LOG" 2>&1
echo "=== $(date '+%F %T') start"

scp -q "$REPO/ensemble_model.json" "$BOX:moex-trader-daily/work/deployed.json"
scp -q "$REPO/data/news_history.jsonl" "$REPO/config.sandbox.yaml" "$BOX:moex-trader-daily/work/"
scp -q "$REPO/scripts/export_ensemble.py" "$REPO/scripts/daily-retrain-box.sh" "$BOX:moex-trader-daily/scripts/"

out="$(ssh -o ConnectTimeout=15 "$BOX" 'cd ~/moex-trader-daily && scripts/daily-retrain-box.sh' 2>&1)" || { echo "$out"; echo "box run failed"; exit 1; }
echo "$out"
if echo "$out" | grep -q '^already done'; then
  echo "nothing new"
  exit 0
fi
last="$(ssh -o ConnectTimeout=15 "$BOX" 'cat ~/moex-trader-daily/work/last_done')"
mkdir -p "$STATE_DIR/reports/$last"
scp -q "$BOX:moex-trader-daily/reports/$last/bt-*.md" "$BOX:moex-trader-daily/reports/$last/meta.txt" "$STATE_DIR/reports/$last/"
"$STATE_DIR/retrainreport" -dir "$STATE_DIR/reports/$last" -date "$last" -state "$STATE_DIR/state.json" \
  -env "$REPO/.env,$REPO/watchdog.env"
echo "=== $(date '+%F %T') end"
