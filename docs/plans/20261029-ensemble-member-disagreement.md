# H15: disagreement of the ensemble members as a reason to abstain (2026-10-29)

Status: registered before the run. English only. Model zone (`internal/model`, `internal/memberagree`, `cmd/backtest`, `cmd/memberagree`); owner approved the study on 2026-10-29. Nothing enters the live path, no artifact is swapped, no config key changes.

## Why

`docs/plans/20261015-hypotheses-from-llm-review.md` H15 was written on 2026-10-15 and never run: the live decision is made on the averaged ensemble probability (`internal/model/ensemble.go` `Probability`), and the spread between the LightGBM, XGBoost and logistic members is discarded. Sizing uses realized volatility only (`internal/model/volscale.go`). The question is whether a trade taken while the members disagree is worse than a trade they agree on, which is the cheapest available form of uncertainty-aware sizing.

Source that reopened the item: the curated list `leoncuhk/awesome-quant-ai` carries a maintainer note "Uncertainty-Driven Position Sizing" (predictor exports forecast uncertainty, sizing layer consumes it). No carried-over number is used as evidence here; only the question is taken.

## Fixed inputs

| Item | Value |
|---|---|
| Dataset | ai-box `/home/oleg/moex-rerun/r2/data-abs.csv`, sha256 `e9898b68e8e98964fc01482742c0530029931142690cae391b75a172c75ff140`, 11254 rows, 2024-09-02..2026-10-16 |
| News overrides | ai-box `/home/oleg/moex-research/memberagree/news-20261029.jsonl`, sha256 `536979e8b2f8fc4fcbfe3ab4ececdfe64d692fedbf240860fbef166a5f7dcf1b`, 157093 lines |
| Tickers | the 17 of `config.sandbox.yaml` (`YDEX,OZON,SBER,LKOH,GAZP,GMKN,ROSN,NVTK,TATN,MTSS,MGNT,PLZL,CHMF,T,VTBR,RUAL,POSI`) |
| Recipe | abs-10d as in `scripts/run-task5.sh`: 18 price/flow features in the 28-name order, deadband 0.5, horizon 10 bars, thresholds 0.60/0.40, embargo 10 bars |
| Windows (train till / replay) | q1 2025-03-04 / 2025-03-05..06-05, q2 2025-06-05 / 06-06..09-05, q3 2025-09-05 / 09-06..12-03, q4 2025-12-03 / 12-04..2026-03-04, w2 2026-03-04 / 03-05..06-06, w1 2026-06-06 / 06-07..09-16 |
| Seeds | 0, 1, 2, 3, 4 (`ENSEMBLE_SEED`); seed 42 is the deployed one and is the primary arm |
| Replay flags | `-deposit 1000000 -max-lots 1000 -commission-rate 0.0005 -spread-pct 0.0005 -slippage-pct 0.0005 -target-notional 15000 -lookback-days 0`, kill switch on, no net caps, no borrow (as in `run-task5.sh`) |
| Hold-out | window h (2026-09-17..10-17) and G are not used |

## Definitions

- `p_lgb`, `p_xgb`, `p_lr` are the member probabilities and `p = (p_lgb+p_xgb+p_lr)/3` the deployed average, all taken at the decision bar that opened the trade (`FeatureContext.GeneratedAt` of the decision, equal to `Trade.OpenedAt`).
- A trade is **unanimous** when all three members cross the same threshold the ensemble crossed: `p_lgb,p_xgb,p_lr >= 0.60` for a BUY or `<= 0.40` for a SELL. Otherwise it is **split**. This is the abstain rule itself, so the segment sizes are the sizes an abstain filter would leave.
- The observation is a closed trade: `Trade.NetPnl` (gross minus commission), with the configured spread and slippage already in the fill prices. Realized only, never MTM.
- Trades opened by the engine without a model decision (`target-flat`, `time-exit`, and any trade whose opening decision is missing from the record) are excluded and counted.

## Tests

1. **Primary.** Seed 42, six windows pooled: mean net P&L per trade of unanimous minus split, with a 95% bootstrap interval over ISO-week blocks of the trade's open date (5000 repeats, seed 42).
   Pass: interval lower bound > 0, at least 200 trades in total, at least 40 distinct open dates, and both segments at least 100 trades.
2. **Seed robustness.** The same contrast for seeds 0-4; the primary sign must hold in at least 4 of 5 seeds.
3. **Conviction confound (verdict-relevant).** Unanimous trades are also high-conviction trades, so the difference may be `|p-0.5|` rather than agreement. Split all trades into terciles of `p` distance from 0.5 (`|p-0.5|`); the unanimous-minus-split difference must keep its sign in at least 2 of 3 terciles. If it does not, the verdict is stated as "pass, explained by conviction" and the abstain rule is not recommended; the follow-up must then use conviction directly.
4. **Placebo (descriptive).** 2000 permutations of the segment labels with the observed sizes, seed 42; report the percentile of the observed difference. It verifies the procedure, it does not decide the verdict.
5. **Descriptive only, no verdict.** Sign agreement (`p > 0.5` for all three, or `< 0.5` for all three) as an alternative split; terciles of the spread `max(p) - min(p)`; per-window breakdown; hit rate per segment.

Verdict: H15 closes as **null** if the primary interval contains zero or the seed or conviction conditions fail; it closes as **confirmed** if all three hold, in which case a separate plan registers the abstain filter as a book-level change (rerun of the six windows with the filter, gate criteria of `20261018-gate-failure-remediation.md`) before any live wiring.

## Notes and limits

- The comparison is per trade, not per book: removing the split trades changes exposure caps, the kill switch and the cash path, so a positive difference is a candidate for a book-level test, not a P&L claim.
- A same-direction scale-up of an open position does not change `OpenedAt`; such a trade is attributed to the decision that opened it.
- The deployed ensemble has no demonstrable edge (`docs/metrics.md` abstract item 1), so a null result is the ex-ante expected outcome. The test is run for the decision, and because the abstain rule is cheap to wire if it works.
- Code: `internal/model` (`MemberProbabilitiesFor`), `internal/memberagree` (recorder, join, segments, bootstrap), `cmd/backtest -members-csv`, `cmd/memberagree -analyze`. One run; a rerun after a code defect is allowed only with both numbers reported.
