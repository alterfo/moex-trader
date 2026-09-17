#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_DIR"

AIBOX_HEALTHZ="${AIBOX_HEALTHZ:-http://<aibox-lan-ip>:9091/healthz}"
MARKER="$REPO_DIR/.deploy/aibox-synced-commit"
LOG="$HOME/Library/Logs/moex-trader-aibox-sync.log"
TELEGRAM_CHAT_ID="${TELEGRAM_CHAT_ID:-<telegram-chat-id>}"

log() {
  echo "$(date '+%Y-%m-%d %H:%M:%S') $*" >>"$LOG"
}

notify_failure() {
  local token
  token="$(grep '^MOEX_TRADER_TELEGRAM_BOT_TOKEN=' "$REPO_DIR/.env" 2>/dev/null | head -1 | cut -d= -f2-)"
  [ -n "$token" ] || return 0
  curl -fsS -m 10 "https://api.telegram.org/bot${token}/sendMessage" \
    --data-urlencode "chat_id=${TELEGRAM_CHAT_ID}" \
    --data-urlencode "text=aibox-sync: $1" >/dev/null 2>&1 || true
}

if ! git diff --quiet HEAD --; then
  log "skip: working tree dirty (uncommitted changes)"
  exit 0
fi

head_sha="$(git rev-parse HEAD)"
last_sha="$(cat "$MARKER" 2>/dev/null || true)"
if [ "$head_sha" = "$last_sha" ]; then
  exit 0
fi

if health="$(curl -fsS -m 5 "$AIBOX_HEALTHZ" 2>/dev/null)"; then
  running="$(echo "$health" | jq -r '.trader.running // false' 2>/dev/null || echo "false")"
  if [ "$running" = "true" ]; then
    log "skip: ai-box trader.running=true (holds the lease), not restarting its watchdog"
    exit 0
  fi
else
  log "skip: ai-box /healthz unreachable, not deploying blind"
  exit 0
fi

log "syncing aibox at $head_sha"
if "$REPO_DIR/scripts/deploy-failover.sh" aibox >>"$LOG" 2>&1; then
  echo "$head_sha" >"$MARKER"
  log "sync ok at $head_sha"
else
  log "sync FAILED at $head_sha"
  notify_failure "deploy to ai-box failed at $head_sha, see $LOG"
  exit 1
fi
