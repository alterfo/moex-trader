# Plans and pre-registrations

Rules: criteria are written before the run; numbers go into `docs/metrics.md` (index) and `docs/metrics/*.md` in the same commit; the `Status:` line of a plan carries its outcome. Plans are English-only. Closed plans are deleted from the tree: their text lives in git history and their numbers in the metrics files. Only `completed/20261018-manual-retrain.md` stays, as the manual retraining procedure.

## Open

| Plan | State |
|---|---|
| `20261018-data-depth-and-open-problems.md` | In progress: steps 1-3, 5, 6, H23 done (price depth, news classifier, calibration null; explainability accepted; VAR-ICA-MGARCHSK rejected); step 2b no verdict (38 < 40 dates) |
| `20261018-seven-hypotheses.md` | Registry of steps 1-21 (LLM direction, news decay, evaluation variance, regime filter, ranking, parts B and C); status per step in the file |
| `20261015-hypotheses-from-llm-review.md` | Registry H1-H22; closed ones (H20, H22: null) in the metrics files |
| `20261018-gate-failure-remediation.md` | Acceptance protocol accepted 2026-10-18; the current model stays forbidden to start until a candidate passes it |
| `20261019-core-rerun.md` | Draft: rerun of the metrics core on refreshed data, paper in `docs/research/` |
| `20261019-research-to-live-gap-hypotheses.md` | PLANNED: H-R2, H-R3, H-R45, H-R6, H-R7, H-R9 |
| `20261019-placebo-check.md` | PLANNED: placebo check of the ridge score table (`cmd/placebocheck`) |
| `20261020-structural-stability-candidates.md` | Draft: state-dependent sizing, ablation audit, first-digit diagnostic |
| `20261010-news-exit-tuning-and-buy-side.md` | PLANNED, blocked by data |
| `20260927-moex-pulse.md` | All tasks unchecked (cloud signal serving); check whether alive |
| `20261022-algopack-orderflow.md` | DONE: 12 order-flow tests, A2 passed (`docs/metrics/s8b-portfolio.md` 8.15); frozen A2 rule under forward evaluation until about 2027-01-20 |
| `20261023-algopack-100-hypotheses.md` | DONE: 100 hypotheses, Tier 1 passed 0 of 28, 17 Tier 2 leads (metrics 8.16); tool `cmd/flowscan` |
| `20261029-closing-the-limitations.md` | REGISTERED: registry of 10 programme limitations with the action that removes each; E1 (value beyond 12-month momentum) runs first |

## Closed (files deleted, see git history; numbers in the metrics files)

| Plan | Outcome |
|---|---|
| `20260914-moex-multi-agent-trader` | Initial single-process build |
| `20260915-commissions-and-finam-broker` | Commission accounting, second broker design |
| `20260915-remove-llm-critical-path` | DONE: pooled logistic model, no LLM in the live path |
| `20260917-strategy-validation-action-plan`, `-implementation-plan` | Agreed validation protocol and its implementation (metrics sections 3 and 4) |
| `20260917-negotiations-sanctions-signal-feature` | Signed negotiation/sanction feature, no gain |
| `20260925-alpha-without-beta-test` | Alpha survives beta-neutral execution (metrics 5.x) |
| `20260925-event-driven-dividend-drift` | Dividend capture rejected (metrics 7.11; rechecked 2026-10-22 on 1591 events, rejected again) |
| `20261004-model-quality-and-robustness` | Purged walk-forward, CI, golden backtest, observability; experiments A-E all failed the gate (metrics 4.2) |
| `20261009-event-causal-study` | No causal effect found (metrics section 7) |
| `20261010-macro-fundamental-features` | No AUC gain, model not retrained |
| `20261018-excess-label-walkforward` | Experiment F rejected on both hypotheses (metrics 4.3) |
| `20261018-sell-bias-diagnostic`, `-preflight-concentration` | SELL skew diagnosis; preflight refusal from P&L concentration (metrics sections 6 and 9) |
| `20261018-recent-window-training`, `-ticker-breaker-parity` | Criterion met by no variant, branches closed |
| `20261022-pre-token-recheck`, `-ticker-selection-check` | Only dividend capture was re-testable (rejected again); dropping losing tickers is indistinguishable from random (metrics 8.14) |
