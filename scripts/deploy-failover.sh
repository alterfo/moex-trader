#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# Fill in the placeholders (or override via env) before deploying.
AIBOX="${AIBOX:-user@<aibox-lan-ip>}"
AIBOX_DIR="${AIBOX_DIR:-/home/user/moex-trader}"
VPS="${VPS:-root@<vps-host>}"
MAC_LABEL="com.example.moex-trader.watchdog"
MAC_PLIST="$HOME/Library/LaunchAgents/$MAC_LABEL.plist"
BUILD_DIR="$REPO_DIR/.deploy"

die() {
  echo "error: $*" >&2
  exit 1
}

ensure_tokens() {
  if [ ! -f "$REPO_DIR/watchdog.env" ]; then
    umask 077
    {
      echo "MOEX_WITNESS_TOKEN=$(openssl rand -hex 32)"
      echo "MOEX_FAILOVER_TOKEN=$(openssl rand -hex 32)"
    } >"$REPO_DIR/watchdog.env"
    echo "generated $REPO_DIR/watchdog.env"
  fi
  if ! grep -q '^MOEX_TRADER_TELEGRAM_BOT_TOKEN=' "$REPO_DIR/watchdog.env" && [ -f "$REPO_DIR/.env" ]; then
    telegram_token="$(grep '^MOEX_TRADER_TELEGRAM_BOT_TOKEN=' "$REPO_DIR/.env" | head -1 | cut -d= -f2-)"
    if [ -n "$telegram_token" ]; then
      echo "MOEX_TRADER_TELEGRAM_BOT_TOKEN=$telegram_token" >>"$REPO_DIR/watchdog.env"
    fi
  fi
  set -a
  # shellcheck disable=SC1091
  . "$REPO_DIR/watchdog.env"
  set +a
  [ -n "${MOEX_WITNESS_TOKEN:-}" ] || die "MOEX_WITNESS_TOKEN is empty in watchdog.env"
  [ -n "${MOEX_FAILOVER_TOKEN:-}" ] || die "MOEX_FAILOVER_TOKEN is empty in watchdog.env"
}

build_all() {
  mkdir -p "$BUILD_DIR"
  echo "building binaries"
  (cd "$REPO_DIR" && CGO_ENABLED=0 GOOS=freebsd GOARCH=amd64 go build -o "$BUILD_DIR/moex-witness-freebsd-amd64" ./cmd/witness)
  (cd "$REPO_DIR" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$BUILD_DIR/watchdog-linux-amd64" ./cmd/watchdog)
  (cd "$REPO_DIR" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$BUILD_DIR/trader-linux-amd64" ./cmd/trader)
  (cd "$REPO_DIR" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$BUILD_DIR/witness-linux-amd64" ./cmd/witness)
  (cd "$REPO_DIR" && CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o "$REPO_DIR/watchdog" ./cmd/watchdog)
}

deploy_witness() {
  ensure_tokens
  echo "deploying witness to $VPS"
  ssh "$VPS" "service moex_witness stop" >/dev/null 2>&1 || true
  scp "$BUILD_DIR/moex-witness-freebsd-amd64" "$VPS:/usr/local/bin/moex-witness"
  scp "$REPO_DIR/deploy/moex_witness" "$VPS:/usr/local/etc/rc.d/moex_witness"
  ssh "$VPS" "sh -s" <<EOF
set -e
chmod 755 /usr/local/bin/moex-witness /usr/local/etc/rc.d/moex_witness
mkdir -p /var/db/moex-witness
test -f /var/db/moex-witness/state.json || echo '{}' > /var/db/moex-witness/state.json
umask 077
echo '$MOEX_WITNESS_TOKEN' > /usr/local/etc/moex-witness.token
chmod 600 /usr/local/etc/moex-witness.token
sysrc moex_witness_enable=YES
service moex_witness restart >/dev/null 2>&1 || service moex_witness start
sleep 1
EOF
  curl -fsS "https://<witness-host>/witness/healthz" >/dev/null && echo "witness is up"
}

deploy_aibox() {
  ensure_tokens
  echo "deploying to $AIBOX"
  ssh "$AIBOX" "systemctl --user stop moex-trader-watchdog.service" >/dev/null 2>&1 || true
  ssh "$AIBOX" "mkdir -p '$AIBOX_DIR/certs' ~/.config/systemd/user"
  scp "$BUILD_DIR/watchdog-linux-amd64" "$AIBOX:$AIBOX_DIR/watchdog"
  scp "$BUILD_DIR/trader-linux-amd64" "$AIBOX:$AIBOX_DIR/trader"
  scp "$REPO_DIR/config.sandbox.yaml" "$AIBOX:$AIBOX_DIR/config.sandbox.yaml"
  scp "$REPO_DIR/ensemble_model.json" "$REPO_DIR/model.json" "$AIBOX:$AIBOX_DIR/"
  scp "$REPO_DIR/.env" "$AIBOX:$AIBOX_DIR/.env"
  scp "$REPO_DIR/watchdog.env" "$AIBOX:$AIBOX_DIR/watchdog.env"
  scp "$REPO_DIR/deploy/watchdog.standby.yaml" "$AIBOX:$AIBOX_DIR/watchdog.yaml"

  ssh "$AIBOX" "cat /etc/ssl/certs/ca-certificates.crt" >"$BUILD_DIR/system-ca.crt"
  cat "$BUILD_DIR/system-ca.crt" "$REPO_DIR/deploy/certs/russian-trusted-ca.pem" >"$BUILD_DIR/ca-bundle.pem"
  scp "$BUILD_DIR/ca-bundle.pem" "$AIBOX:$AIBOX_DIR/certs/ca-bundle.pem"

  ssh "$AIBOX" "chmod 600 '$AIBOX_DIR/.env' '$AIBOX_DIR/watchdog.env'; chmod 755 '$AIBOX_DIR/watchdog' '$AIBOX_DIR/trader'"

  ssh "$AIBOX" "test -f ~/.ssh/id_moex_tunnel || ssh-keygen -t ed25519 -N '' -f ~/.ssh/id_moex_tunnel -C moex-tunnel"
  ssh "$AIBOX" "cat ~/.ssh/id_moex_tunnel.pub" >"$BUILD_DIR/aibox_tunnel.pub"
  scp "$BUILD_DIR/aibox_tunnel.pub" "$VPS:/tmp/aibox_tunnel.pub"
  ssh "$VPS" "sh -s" <<'EOF'
set -e
mkdir -p /root/.ssh
grep -qF -f /tmp/aibox_tunnel.pub /root/.ssh/authorized_keys || cat /tmp/aibox_tunnel.pub >> /root/.ssh/authorized_keys
rm -f /tmp/aibox_tunnel.pub
EOF
  echo "testing tunnel key from $AIBOX to the VPS"
  ssh "$AIBOX" "ssh -i ~/.ssh/id_moex_tunnel -o BatchMode=yes -o StrictHostKeyChecking=accept-new -o ConnectTimeout=8 $VPS 'echo TUNNEL_OK'" | grep -q TUNNEL_OK

  scp "$REPO_DIR/deploy/moex-trader-watchdog.service" "$AIBOX:$AIBOX_DIR/moex-trader-watchdog.service"
  ssh "$AIBOX" "mkdir -p ~/.config/systemd/user && cp '$AIBOX_DIR/moex-trader-watchdog.service' ~/.config/systemd/user/moex-trader-watchdog.service"
  ssh "$AIBOX" "systemctl --user start moex-trader-watchdog.service" >/dev/null 2>&1 || true
  echo "aibox files installed; enable with: ssh $AIBOX 'systemctl --user enable moex-trader-watchdog'"
}

start_aibox() {
  ssh "$AIBOX" "systemctl --user daemon-reload && systemctl --user enable --now moex-trader-watchdog.service && systemctl --user --no-pager status moex-trader-watchdog.service | head -5"
}

deploy_mac() {
  ensure_tokens
  echo "installing watchdog on this Mac"
  (cd "$REPO_DIR" && go build -o "$REPO_DIR/trader" ./cmd/trader)
  cp "$REPO_DIR/deploy/watchdog.primary.yaml" "$REPO_DIR/watchdog.yaml"
  mkdir -p "$HOME/Library/LaunchAgents" "$HOME/Library/Logs"
  cp "$REPO_DIR/deploy/com.example.moex-trader.watchdog.plist" "$MAC_PLIST"
  launchctl bootout "gui/$(id -u)/$MAC_LABEL" 2>/dev/null || true
  launchctl bootstrap "gui/$(id -u)" "$MAC_PLIST"
  launchctl kickstart -k "gui/$(id -u)/$MAC_LABEL"
  echo "mac watchdog installed"
}

install_aibox_sync() {
  local label="com.example.moex-trader.aibox-sync"
  local plist="$HOME/Library/LaunchAgents/$label.plist"
  mkdir -p "$HOME/Library/LaunchAgents" "$HOME/Library/Logs"
  cp "$REPO_DIR/deploy/$label.plist" "$plist"
  launchctl bootout "gui/$(id -u)/$label" 2>/dev/null || true
  launchctl bootstrap "gui/$(id -u)" "$plist"
  echo "aibox-sync timer installed (every 30 min); logs at ~/Library/Logs/moex-trader-aibox-sync*.log"
}

status() {
  echo "== witness =="
  curl -fsS --max-time 5 "https://<witness-host>/witness/healthz" && echo
  echo "== mac =="
  curl -fsS --max-time 5 "http://<mac-lan-ip>:9091/healthz" && echo
  echo "== aibox =="
  curl -fsS --max-time 5 "http://<aibox-lan-ip>:9091/healthz" && echo
}

case "${1:-}" in
  build)
    build_all
    ;;
  witness)
    build_all
    deploy_witness
    ;;
  aibox)
    build_all
    deploy_aibox
    ;;
  start-aibox)
    start_aibox
    ;;
  mac)
    build_all
    deploy_mac
    ;;
  status)
    status
    ;;
  *)
    cat <<USAGE
usage: $0 <command>

  build        cross-compile witness, watchdog and trader binaries
  witness      build and deploy the lease witness to the VPS
  aibox        build and copy watchdog/trader/configs to the ai-box
  start-aibox  enable and start the standby watchdog on the ai-box
  mac          build and install the launchd watchdog on this Mac
  status       query /healthz on the witness, Mac and ai-box
USAGE
    exit 1
    ;;
esac
