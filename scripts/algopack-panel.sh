#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PY="${PY:-/home/oleg/moex-venv/bin/python}"
WORK="${WORK:-$ROOT/work}"
REGIME="${REGIME:?REGIME must be hist, rt or fwd}"
SEEDS="${SEEDS:-20261023 20261024 20261025 20261026 20261027}"
STARTS=(2025-01-03 2025-04-01 2025-07-01 2025-10-01 2026-01-01 2026-04-01 2026-07-01 2026-10-21)
case "$REGIME" in
  hist) FOLDS="1 2 3 4 5 6 7"; SUF="" ;;
  rt) FOLDS="3 4 5 6 7"; SUF="-rt" ;;
  fwd) FOLDS="8"; SUF="-rt" ;;
  *) echo "REGIME must be hist, rt or fwd" >&2; exit 1 ;;
esac
AP="ap_a2,ap_a2_m5,ap_a1,ap_a4,ap_a5,ap_lev_asym,ap_spread_ratio,ap_depth_growth,ap_ticket_z,ap_otr_z"

train() {
  local arm="$1" data="$2" feats="$3"
  for seed in $SEEDS; do
    for k in $FOLDS; do
      out="$WORK/models-p-$REGIME/$arm/s$seed/f$k.json"
      [ -f "$out" ] && continue
      mkdir -p "$(dirname "$out")"
      DATASET="$data" ENSEMBLE_FEATURES14="$feats" ENSEMBLE_FEATURE_ORDER="$feats" \
        ENSEMBLE_TRAIN_TILL="${STARTS[$((k-1))]}" ENSEMBLE_EMBARGO_SPLIT="${STARTS[$((k-1))]}" ENSEMBLE_EMBARGO_BARS=10 \
        ENSEMBLE_SEED="$seed" ENSEMBLE_OUT="$out" OMP_NUM_THREADS="${THREADS:-4}" \
        "$PY" "$ROOT/scripts/export_ensemble.py" > "$WORK/models-p-$REGIME/$arm/s$seed/f$k.log" 2>&1
      echo "trained $REGIME $arm seed $seed fold $k"
    done
  done
}

train A "$WORK/panel$SUF.csv" "$AP"
train S "$WORK/panel-shuf$SUF.csv" "$AP"
train A2 "$WORK/panel$SUF.csv" "ap_a2"
echo "all arms trained ($REGIME)"
