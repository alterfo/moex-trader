#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
NEWSSENT_DIR="${NEWSSENT_DIR:-$REPO_DIR/../newssent}"
AIBOX="${AIBOX:-oleg@192.168.88.193}"
DRY="${1:-}"

RSYNC=(rsync -a --partial --prune-empty-dirs -e "ssh -o BatchMode=yes -o ConnectTimeout=10")
[ "$DRY" = "--dry-run" ] && RSYNC+=(-i -n)

pull() {
  local src="$1" dst="$2"
  shift 2
  mkdir -p "$dst"
  "${RSYNC[@]}" "$@" "$AIBOX:$src" "$dst/"
}

ssh -o BatchMode=yes -o ConnectTimeout=10 "$AIBOX" true

pull moex-trades/data/ "$REPO_DIR/data/aibox/trades"
pull moex-trades/record.log "$REPO_DIR/data/aibox/trades"
pull moex-shadow/shadow.jsonl "$REPO_DIR/data/aibox/shadow"
pull moex-shadow/out/ "$REPO_DIR/data/aibox/shadow/out"
pull moex-shadow/hr8/ "$REPO_DIR/data/aibox/shadow/hr8"
pull moex-shadow/hr9/ "$REPO_DIR/data/aibox/shadow/hr9"
pull moex-rerun/forward/ "$REPO_DIR/data/aibox/rerun/forward"
pull moex-rerun/out/ "$REPO_DIR/data/aibox/rerun/out"
pull moex-rerun/r2/ "$REPO_DIR/data/aibox/rerun/r2"
pull moex-rerun/depth/ "$REPO_DIR/data/aibox/rerun/depth"

pull newssent-train/data/ "$NEWSSENT_DIR/data/aibox-train/data"
pull newssent-train/out/ "$NEWSSENT_DIR/data/aibox-train/out" --exclude='*.pt'

echo "=== $(date '+%F %T') aibox pull done${DRY:+ ($DRY)}"
