#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
mkdir -p bin data
go build -o bin/fundsnap ./cmd/fundsnap
set -a
. "$ROOT/.env"
set +a
echo "=== $(date '+%F %T') fundsnap"
if [ -n "${T_TOKEN:-}" ]; then
  bin/fundsnap -out data/fundamentals.jsonl -token-env T_TOKEN
else
  bin/fundsnap -out data/fundamentals.jsonl -token-env T_SAND_TOKEN \
    -base "https://sandbox-invest-public-api.tbank.ru/rest/tinkoff.public.invest.api.contract.v1."
fi
