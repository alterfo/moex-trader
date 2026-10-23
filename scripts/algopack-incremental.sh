#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PY="${PY:-/home/oleg/moex-venv/bin/python}"
WORK="${WORK:-$ROOT/work}"
REGIME="${REGIME:?REGIME must be hist or rt}"
TAG="${TAG:-}"
SEEDS="${SEEDS:-20261023 20261024 20261025 20261026 20261027}"
EMBARGO_DAYS=10
STARTS=(2025-01-03 2025-04-01 2025-07-01 2025-10-01 2026-01-01 2026-04-01 2026-07-01)
case "$REGIME" in
  hist) FOLDS="1 2 3 4 5 6 7"; SUF="" ;;
  rt) FOLDS="3 4 5 6 7"; SUF="-rt" ;;
  *) echo "REGIME must be hist or rt" >&2; exit 1 ;;
esac

FEAT18="return_pct,realized_volatility,news_sentiment,news_count,order_book_imbalance,mom_5d,mom_21d,mom_63d,reversal_1d,rsi_14,dist_ma20_pct,dist_ma50_pct,realized_vol_21d_annualized_pct,volume_zscore_20d,macd_hist_pct,stoch_k_14,williams_r_14,alligator_spread_pct"
ORDER28="$FEAT18,event_dividend,event_buyback,event_sanctions,event_ipo,event_report,event_delisting,event_mna,event_default,negotiations_signal,sanctions_signal"
AP="ap_a2,ap_a2_m5,ap_a1,ap_a4,ap_a5,ap_lev_asym,ap_spread_ratio,ap_depth_growth,ap_ticket_z,ap_otr_z"

train() {
  local arm="$1" data="$2" feats="$3" order="$4"
  for seed in $SEEDS; do
    for k in $FOLDS; do
      out="$WORK/models-$REGIME$TAG/$arm/s$seed/f$k.json"
      [ -f "$out" ] && continue
      mkdir -p "$(dirname "$out")"
      DATASET="$data" ENSEMBLE_FEATURES14="$feats" ENSEMBLE_FEATURE_ORDER="$order" \
        ENSEMBLE_TRAIN_TILL="${STARTS[$((k-1))]}" ENSEMBLE_EMBARGO_SPLIT="${STARTS[$((k-1))]}" ENSEMBLE_EMBARGO_BARS=$EMBARGO_DAYS \
        ENSEMBLE_SEED="$seed" ENSEMBLE_OUT="$out" OMP_NUM_THREADS="${THREADS:-4}" \
        "$PY" "$ROOT/scripts/export_ensemble.py" > "$WORK/models-$REGIME$TAG/$arm/s$seed/f$k.log" 2>&1
      echo "trained $REGIME $arm seed $seed fold $k"
    done
  done
}

train B "$WORK/merged$TAG$SUF.csv" "$FEAT18" "$ORDER28"
train A "$WORK/merged$TAG$SUF.csv" "$FEAT18,$AP" "$ORDER28,$AP"
train S "$WORK/merged$TAG-shuf$SUF.csv" "$FEAT18,$AP" "$ORDER28,$AP"
echo "all arms trained ($REGIME)"
