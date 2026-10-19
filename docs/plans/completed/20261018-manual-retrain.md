# Manual retraining of the ensemble (2026-10-18)

Russian translation: [20261018-manual-retrain.ru.md](20261018-manual-retrain.ru.md). This file is canonical.

The owner approved on 2026-10-18 ("go 1 2 3", item 2). Reason: D4 of the SELL-skew diagnosis — a fresh model gives 2.2% BUY against 0% for the deployed one on the same window (staleness of the BUY tail). Auto-replacement is still forbidden; this is a one-off manual retraining with explicit consent.

## Candidate

The recipe of the deployed model is unchanged: absolute-10d, 18 features (including `news_sentiment`/`news_count`), 28-name order, colsample 0.8, thresholds 0.60/0.40, hold until flip. Only the training window changes.

- Data (ai-box, `~/moex-trader-task12`): `exportdataset -config config.sandbox.yaml -tickers <17 live + DATA> -from 2024-01-01 -till 2026-10-18 -split 2026-06-18 -horizon-days 10 -deadband-pct 0.5 -label-mode absolute -news-history news_history.jsonl -out data-18f-retrain.csv`; `news_history.jsonl` is the current archive from the Mac (14619 records).
- Training: `scripts/export_ensemble.py`, `ENSEMBLE_TRAIN_TILL=2026-10-17`, `ENSEMBLE_EMBARGO_SPLIT=2026-10-17`, `ENSEMBLE_EMBARGO_BARS=10`, FEAT18/ORDER from Task 5, a venv with pinned versions (`scripts/requirements-ensemble.txt`).
- The server path does not change: news features are masked (`untrainedFeatures`), the out_of_range gate with the coef=0 fix — both are already in HEAD.

## Replacement gate (fixed before the run)

Both replays use the HEAD engine (`backtest-fixed` with the gate fix, linux/amd64), identical costs (0.0005×3), 1M deposit, 15000 RUB/position, kill-switch on, 17 tickers:

1. 30-day holdout (2026-09-17..2026-10-17): the candidate's realized P&L ≥ the deployed model's realized P&L on the same window.
2. 90-day preflight window (2026-07-20..2026-10-17): the candidate's realized P&L ≥ the deployed model's realized P&L on the same window, and ≥ `preflight.min_net_pnl`/`min_closed_trades` (1/1), and the candidate's max DD ≤ the deployed model's max DD + 0.5 pp.

Replace only if both items hold; otherwise the numbers are recorded and the model is not changed.

## After replacement

- `ensemble_model.json` is replaced; the golden backtest is updated (`-update`), the parity fixture is regenerated (`scripts/dump_parity_fixture.py` on the ai-box); `make check` passes; metrics and the registry (`manual_retrain_20261018`) in the same commit.
- Deployment (item 1): rebuild `./trader` on the Mac, restart through the watchdog (the watchdog holds the lease, the failure counter resets after activation); the ai-box is updated by the normal aibox-sync (standby).
- OOS: `cmd/oosfreeze -action freeze` right after deployment.

## Retraining outcome (2026-10-18): no replacement

The gate is not passed, numbers in `docs/metrics.md`, section "Manual retrain candidate (2026-10-18)". The candidate (trained up to 2026-10-17, embargo 10, 18 tickers, the same recipe) loses to the deployed model in both windows:

| window | deployed realized | candidate realized | deployed trades | candidate trades | deployed DD | candidate DD |
|---|---|---|---|---|---|---|
| hold30 (2026-09-17..10-17) | +583.21 | -3281.95 | 32 | 45 | 1.24% | 1.45% |
| pre90 (2026-07-20..10-17) | +10419.63 | -1508.68 | 137 | 164 | 1.69% | 2.27% |

Criterion 1 (candidate ≥ deployed on hold30) and criterion 2 (candidate ≥ deployed on pre90) are not met; `ensemble_model.json` does not change. The candidate's fresh BUY signals (2.2% against 0% in D4) are loss-making in this window: refreshing the training window worsens the SELL side more than it fixes the BUY tail. This is the third independent confirmation that the current recipe is sensitive to the training window with a base AUC around noise: manual retraining from now on only through this gate. Condition for re-checking: a new candidate runs no earlier than noticeably more data has accumulated (at least a quarter since 2026-10-17), and only through the same gate.
