#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")/.."
GOOS=linux GOARCH=amd64 go build -o /tmp/tradesrecord-linux ./cmd/tradesrecord
scp -q /tmp/tradesrecord-linux oleg@192.168.88.193:moex-trades/tradesrecord
