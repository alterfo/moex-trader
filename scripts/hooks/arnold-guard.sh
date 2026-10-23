#!/usr/bin/env bash
# Arnold Guard — agent reminder hook
# Triggers when editing sensitive trading parameters (thresholds, sizing, exposure)
# Usage: ./scripts/hooks/arnold-guard.sh <file> [file...]
#        git diff --cached --name-only | xargs ./scripts/hooks/arnold-guard.sh

set -e

RED='\033[0;31m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

PATTERNS='(0\.6[0-9]|0\.4[0-9]|15000|90000|max_net_(long|short)|target_notional|buy.*threshold|sell.*threshold|rebalance_min_deviation_pct|circuit_breaker_max_loss)'

files=("$@")
[ ${#files[@]} -eq 0 ] && exit 0

touch=0
for f in "${files[@]}"; do
  [ -z "$f" ] && continue
  case "$f" in
    *config*.yaml|*internal/risk/*|*internal/model/*|*cmd/trader/*|*cmd/trainmodel/*|*cmd/placebocheck/*|*scripts/*tune*|*scripts/*sweep*) touch=1; break;;
  esac
done

[ $touch -eq 0 ] && exit 0

echo -e "${RED}╔══════════════════════════════════════════════════════════╗${NC}"
echo -e "${RED}║                    ARNOLD GUARD                          ║${NC}"
echo -e "${RED}╚══════════════════════════════════════════════════════════╝${NC}"
echo
echo -e "${YELLOW}You're about to edit tuned trading parameters.${NC}"
echo
echo -e "${CYAN}Arnold's lesson:${NC}"
echo "  Optimizing a parameter to its historical maximum puts the system"
echo "  exactly on the boundary of structural stability. The shape of the"
echo "  rule matters more than the value. Prefer state-dependent feedback"
echo "  over constants tuned on history."
echo
if [ -d /Users/olegsidorkin/dev/fin/moex-trader ]; then
  repo=/Users/olegsidorkin/dev/fin/moex-trader
  echo -e "${CYAN}Current consistency (ctl R2 from ai-box):${NC}"
  if [ -d /tmp/wf-ctl-flat ]; then
    (cd "$repo" && go run ./cmd/strategyvalidation -consistency /tmp/wf-ctl-flat -pass-horizon 4 2>&1 | grep -E "positive windows|Pass\^|smoothed") || true
  else
    echo "  /tmp/wf-ctl-flat not found (run from ai-box copy or see docs/reports/)"
  fi
  echo
fi
echo -e "${CYAN}Before committing ask yourself:${NC}"
echo "  1. Have I tested perturbation (±0.02 thresholds / drop-one-ticker / window shift)?"
echo "  2. Is this value near a peak in a grid or on a cliff/plateau?"
echo "  3. Can this be a rule reading live state instead of a constant?"
echo "  4. If metrics change, did I update docs/metrics.md in the same commit?"
echo
echo -e "${YELLOW}Read: docs/knowledge/soft-models-and-optimization.md${NC}"
echo
