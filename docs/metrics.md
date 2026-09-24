# Metrics

Single source of truth for backtest and analysis numbers. Formal structure
(columns: window | realized vs MTM | artifact (commit) | costs | date) is the
Metrics ledger section below; per-task sections that follow record the
derivation details. Realized P&L is the AGENTS.md invariant for go/no-go
judgments; MTM is shown separately and never drives a decision.

> **Pending recomputation (2026-09-17 review fixes):** the backtest/exportdataset,
> training sample builder, live ingestion, and the tail-precision/momentum/regime
> analysis CLIs now fetch the full 300-bar feature lookback (434 calendar days) so
> EMA/SMMA indicators converge exactly as they do live, and `realized_volatility`
> is bounded to that same trailing 300-bar window in live, backtest, and training.
> The deployed `ensemble_model.json` (277ebf7) predates these corrections, so it
> must be retrained on an `exportdataset` run built with the corrected feature
> construction before preflight/live numbers can be cited; the OOS freeze must also
> be re-frozen on the corrected code. Every backtest, preflight, and analysis number
> in this file was produced before these corrections and must be re-derived after
> that retrain; the ledger rows below are retained for traceability only until then.

> **Backtest/news weighting alignment (2026-09-18 review fixes):** `backtest.LoadNewsOverrides`
> now weight-averages `news_sentiment` by `trust_weight`, matching
> `model.AggregateDailySentiment`. `-news-history` backtests recorded before this
> alignment used unweighted averaging and must be re-derived before reuse.

## Tail-precision falsification (Task 1)

Pre-registered pooled metric over the available window of the deployed
`ensemble_model.json` (commit 277ebf7). Outcome "in the signal's direction":
BUY -> move > +0.5%, SELL -> move < -0.5% over 10 trading days.

| date | window | tail decisions | pooled precision | pooled base rate | right-tailed bootstrap p | artifact |
|---|---|---|---|---|---|---|
| 2026-09-17 | 2026-06-19 -> 2026-09-17 | 500 (303 BUY / 197 SELL) | 42.4% (212/500) | 47.7% | 0.9123 (83 episodes, 10000 trials) | ensemble_model.json @ 277ebf7 |

Decision-day samples: 1260; P(move > +0.5%) = 47.1%, P(move < -0.5%) = 48.7%.

Conclusion: the tail does not beat the base rate (slightly below it); the
right-tailed block-bootstrap p-value gives no evidence of tail edge. This run
is an approximation — per-quarter models were not saved, so the exact
walk-forward version depends on Task 13.

## Metrics ledger

Formal single source of truth for headline P&L numbers. Column order is
pre-registered in the strategy-validation plan: `window | realized vs MTM |
artifact (commit) | costs | date`. Realized P&L is gross minus commission and
borrow; MTM is open-position mark-to-market and never drives a go/no-go.
Historical rows where the original run did not record a field are marked
`not recorded` and must be re-verified before reuse.

| window | realized vs MTM | artifact (commit) | costs | date |
|---|---|---|---|---|
| 2025-04-01 -> 2026-09-17 (6 quarters) | realized +110742; MTM not recorded | abs-10d walk-forward recipe (AGENTS.md), per-quarter models not saved | commission 0.05%, no spread/slippage | 2026-09-16 |
| 2026-06-19 -> 2026-09-17 (90d preflight) | MTM +79453; realized not recorded | ensemble_model.json @ 722353e | zero (pre-Task-5 preflight) | 2026-09-17 |
| not recorded | +75028; realized vs MTM not recorded | not recorded | not recorded | not recorded |
| not recorded | +63212; realized vs MTM not recorded | not recorded | not recorded | not recorded |
| 2026-06-19 -> 2026-09-17 (holdout) | realized +56379; MTM not recorded | news-aware ensemble_model.json @ 277ebf7 | commission 0.05%, spread 0.05%, slippage 0.05% | 2026-09-17 |
| 2026-06-19 -> 2026-09-17 (holdout) | realized +40102; MTM not recorded | no-news control @ 277ebf7 | commission 0.05%, spread 0.05%, slippage 0.05% | 2026-09-17 |
| 2026-06-19 -> 2026-09-17 (90d preflight) | MTM +25699; realized not recorded | ensemble_model.json @ 277ebf7 | zero (pre-Task-5 preflight) | 2026-09-17 |
| 2025-03-05 -> 2026-09-16 (6 windows) | realized +101246.75 (18f), +103739.34 (20f); MTM not recorded | negotiations/sanctions evidence-gate walk-forward (Task 6) | commission 0.05%, spread+slippage 0.05% each | 2026-09-18 |

The two `not recorded` rows (+75028, +63212) were reported in the two-session
source debate but their window/cost/realized-vs-MTM provenance was not
committed. They are kept as ledger rows so the numbers are not lost, but they
must be re-derived before they can be cited as evidence.

## Process rule (incident E4, two-session race)

Metric-bearing changes are committed immediately. A session must only read
metrics from `HEAD` or from an explicitly requested uncommitted file — an
empty `git log -S` search does not prove absence when the working tree has
uncommitted changes.

## Momentum benchmark (Task 2)

Pre-registered top-k momentum benchmark on `mom_21d` (k=5), rebalanced every
10 trading days, on the same 18 tickers with the same commission/spread/
slippage/kill-switch/deposit as the live config, compared against the deployed
ensemble over the same window. Costs per fill: commission 0.05%, spread 0.05%,
slippage 0.05%. Window: 2026-06-19 -> 2026-09-17 (81 complete trading days;
the incomplete current bar is dropped for reproducibility).

| date | strategy | realized P&L | MTM P&L | closed trades | win rate | max DD | artifact |
|---|---|---|---|---|---|---|---|
| 2026-09-17 | ensemble (deployed) | +34690.64 | +29646.56 | 200 | 48.0% | 1.07% | ensemble_model.json |
| 2026-09-17 | momentum long-only | -6697.58 | -3736.18 | 107 | 59.8% | 1.87% | momentumbench |
| 2026-09-17 | momentum long+short | -13111.02 | -7635.80 | 219 | 59.4% | 1.18% | momentumbench |

Verdict: the ensemble's realized P&L beats both momentum variants net of costs,
so "the model is momentum + noise" is not supported on this window. This is a
relative comparison — both sides run through the same engine, so engine-wide
biases (including the Task 4 kill-switch gap) cancel out. Absolute ensemble
numbers come from the momentumbench engine and differ from other headline
numbers (reconciled in Task 17).

## Live/backtest feature parity fix (Task 3)

Live feature construction now uses only closed daily bars. Before this fix the
live input carried the current unclosed bar and an intraday `LastPrice`, so
`return_pct` was an intraday return and the candle-derived indicators were
computed over a different bar set than backtest. After the fix:

- `internal/orchestrator/ingest.go` drops the current unclosed daily bar
  (`closedDailyCandles`) and fetches enough history for the full daily
  indicator window (`dailyCandleLookback`, ~434 calendar days, covering the
  300-bar EMA/SMMA convergence margin used by backtest).
- `internal/features/builder.go` computes `return_pct` from the last two closed
  bars (`closedBarReturnPct`), matching backtest's `LastPrice = prev close`.

The prior 10-day candle lookback left 12 of the 18 deployed features
(mom_21d/63d, rsi_14, dist_ma20/50, realized_vol_21d, volume_zscore_20d,
macd_hist, stoch_k, williams_r, alligator_spread) at zero in live; that gap is
now closed as part of this task.

## Portfolio risk gate in backtest (Task 4)

The backtest now runs tickers in a single chronological portfolio loop instead
of ticker-by-ticker loops. Each gate request receives portfolio-level
`CurrentEquity` and the previous portfolio close as `DayStartEquity`, so the
3% drawdown kill switch and 0.5% daily-loss limit evaluate the same aggregate
curve the live trader sees. Decision: the kill switch keeps the current live
behavior — block new entries only, do not liquidate open positions — and Task 8's
gap-stress test is the compensating control for the un-liquidated exposure.

Six-quarter grid re-run after the fix. Same deployed `ensemble_model.json`,
18 sandbox tickers, 1M RUB deposit, 15000₽/position, commission/spread/
slippage 0.05% each, hold-until-flip. The exact per-quarter models are still
not persisted, so this substitutes the deployed artifact on each window; Task 13
will make the true walk-forward rerunnable.

| window | realized P&L | MTM P&L | closed trades | max DD | kill-frozen days | daily-loss blocked days |
|---|---|---|---|---|---|---|
| 2025-04-01 -> 2025-06-30 | +42453.50 | +42093.83 | 199 | 0.77% | 0 | 0 |
| 2025-07-01 -> 2025-09-30 | +9171.72 | +27576.50 | 104 | 0.90% | 0 | 0 |
| 2025-10-01 -> 2025-12-30 | +19600.27 | +5964.14 | 108 | 1.82% | 0 | 1 |
| 2026-01-05 -> 2026-03-31 | -6655.63 | -6729.68 | 95 | 1.80% | 0 | 0 |
| 2026-04-01 -> 2026-06-30 | +41695.40 | +54945.79 | 110 | 0.90% | 0 | 1 |
| 2026-07-01 -> 2026-09-17 | +47216.95 | +40051.28 | 181 | 1.07% | 0 | 2 |
| total | +153482.21 | +163901.86 | 797 | — | 0 | 4 |

The persistent drawdown kill switch froze 0 days on these windows. The
portfolio-level daily-loss limit blocked new entries on 4 days total (Q3, Q5,
Q6); under the old ticker-local backtest gate this limit was dead code because
`DayStartEquity` was never set.

## Preflight hardening (Task 5)

Preflight now replays with `preflight.spread_pct` + `preflight.slippage_pct`
fill costs, judges by **realized** P&L (closed trades, gross − commission), and
requires at least `preflight.min_closed_trades` before checking
`preflight.min_net_pnl`. Gate decisions log a SHA-256 hash of the
gate-relevant configuration.

Post-fix 90-day preflight replay (2026-06-19 -> 2026-09-17, 18 sandbox tickers,
1M RUB deposit, 15000₽/position, ensemble_model.json, commission/spread/
slippage 0.05% each, hold-until-flip, kill switch on):

| date | realized P&L | MTM P&L | closed trades | hit rate | max DD | daily-loss blocked days | artifact |
|---|---|---|---|---|---|---|---|
| 2026-09-17 | +43040.38 | +38818.21 | 197 | 49.2% | 1.07% | 3 | ensemble_model.json |

Replayed through read-only `cmd/backtest` because a sandbox trader was already
running; the live `cmd/trader` gate additionally injects real Tinkoff lot sizes,
so the exact gate numbers may differ by the lot-quantization step.

## Attempt registry, DSR and PBO (Task 7)

`cmd/strategyvalidation` and `internal/strategyvalidation` record every
configuration tried so far from `AGENTS.md` and `docs/metrics.md`. The source
debate artifacts (`critique-left.md`, `rebuttal-right.md`,
`plan-additions-right.md`) were never committed, so the registry is labelled
from the surviving documentation; `docs/habr-ai-article/backtest-results.md`
is not in this tree.

| metric | value |
|---|---:|
| documented attempts | 41 |
| selected | 7 |
| rejected | 24 |
| control/reference/benchmark | 10 |
| realized-series observations | 6 quarterly returns |
| mean return | 0.025580 |
| sample stdev | 0.021710 |
| Sharpe | 1.178255 |
| skewness | -0.309658 |
| kurtosis (non-excess) | 1.141267 |
| probabilistic Sharpe vs 0 | 0.986645 |
| expected max Sharpe (41 trials) | 0.983521 |
| deflated Sharpe (Bailey & Lopez de Prado) | 0.642892 |
| deflated Sharpe p-value | 0.357108 |
| Harvey-Liu multiple-testing t | 3.5 |
| Harvey-Liu critical Sharpe | 1.565248 |
| conservative DSR, max(expected, Harvey-Liu) | 0.233384 |

The only archived realized per-period series is the Task 4 six-quarter
grid (realized P&L divided by the 1M RUB deposit): +42453.50, +9171.72,
+19600.27, -6655.63, +41695.40, +47216.95. Both the standard DSR and the
Harvey-Liu conservative DSR are below the pre-registered go/no-go bar of
DSR > 0.95.

PBO is implemented as generic CSCV code with tests, but it is NOT computable
from this summary-only registry: no per-strategy period-return matrices were
archived for the ~40 attempts. No daily return series are fabricated to force
the computation. The PBO < 0.2 go/no-go check therefore stays open until the
per-attempt matrices are persisted (Task 13 walk-forward persistence and the
Task 17 formal metrics table are the natural sources).

## Gap-stress test (Task 8)

Models overnight gaps against the deployed strategy's open portfolio at the
end of a 90-day replay (2026-06-19 -> 2026-09-17, 18 sandbox tickers, 1M RUB
deposit, 15000₽/position, ensemble_model.json, hold-until-flip, commission/
spread/slippage 0.05% each). Gap history lookback: 365 calendar days. The
sticky kill switch blocks new entries at 3% drawdown / 0.5% daily loss but
does NOT liquidate open positions, so a gap hits equity in full first.

| date | scenario | P&L | P&L % of deposit | breaches 3% DD | breaches 0.5% daily | artifact |
|---|---|---|---|---|---|---|
| 2026-09-17 | worst historical day (2026-03-09) | -3827.21 | -0.38% | false | false | ensemble_model.json |
| 2026-09-17 | worst-per-ticker-combined | -6731.51 | -0.67% | false | true | ensemble_model.json |
| 2026-09-17 | synthetic -10% (net-short favorable) | +17919.97 | +1.79% | false | false | ensemble_model.json |
| 2026-09-17 | synthetic -20% (net-short favorable) | +35839.93 | +3.58% | false | false | ensemble_model.json |
| 2026-09-17 | synthetic +10% (net-short adverse) | -17919.97 | -1.79% | false | true | ensemble_model.json |
| 2026-09-17 | synthetic +20% (net-short adverse) | -35839.93 | -3.58% | true | true | ensemble_model.json |

Current open portfolio at cutoff: 18 positions, gross exposure 266573₽
(long 43687₽, short 222887₽, net -179200₽). The worst observed aligned
historical day (2026-03-09) and the worst-per-ticker composite stay below the
3% drawdown floor; a uniform +20% adverse gap would breach it (-3.58%).
Marks use the live ISS daily close and shift slightly between runs.

## Beta / regime decomposition (Task 9)

`cmd/betaregime` replays the deployed `ensemble_model.json` as one continuous
run (2025-04-01 -> 2026-09-17, 18 sandbox tickers, 1M RUB deposit, 15000 RUB
notional per position, commission/spread/slippage 0.05% each, hold-until-flip,
kill switch on) and decomposes realized P&L net of IMOEX. The equal-weight
18-ticker benchmark (long each name at the same notional, daily rebalanced) is
the null-universe context; the momentum benchmarks are the Task 2 top-k
`mom_21d` plans (k=5, rebalance every 10 trading days). Continuous-run numbers
differ from the Task 4 six-window grid because positions carry across quarter
boundaries instead of being cut at each window.

Results net of costs:

| strategy | realized P&L | MTM P&L | closed trades | return on trade notional | max DD |
|---|---|---|---|---|---|
| ensemble (deployed) | +194293.10 | +188225.76 | 862 | +4.77% | 2.11% |
| equal-weight 18-ticker | -673.13 | -68102.99 | 516 | -0.24% | 11.97% |
| momentum long-only | -20948.09 | -38708.67 | 500 | -1.27% | 4.68% |
| momentum long+short | -32082.28 | -26713.25 | 1151 | -0.80% | 3.20% |

Realized P&L decomposition net of IMOEX (long and short legs separate; return
measured on gross trade notional, not the 1M deposit):

| leg | trades | realized P&L | notional | IMOEX component | excess | return on notional |
|---|---|---|---|---|---|---|
| long | 261 | +67261.43 | 1986656.57 | +28429.45 | +38831.98 | +3.39% |
| short | 601 | +127031.67 | 2090319.43 | +100892.32 | +26139.35 | +6.08% |
| total | 862 | +194293.10 | 4076976.00 | +129321.77 | +64971.33 | +4.77% |

The IMOEX (beta) component is ~2x the excess (alpha) component: on this window
the deployed book is short-tilted (601 short vs 261 long trades) and earned a
large part of its P&L from short beta in a declining IMOEX.

Regression `P&L ~ alpha + beta1*equal-weight + beta2*momentum`, cluster-robust
standard errors:

| level | N | clusters | alpha (t) | beta1 equal-weight (t) | beta2 momentum (t) | R2 |
|---|---|---|---|---|---|---|
| per-trade | 862 | 18 (ticker) | +0.01056 (1.47) | -1.681 (-6.19) | +1.070 (0.78) | 0.111 |
| per-day | 486 | 6 (quarter) | +0.00031 (4.66) | -0.274 (-2.64) | -0.197 (-1.05) | 0.233 |

OOS quarter regimes (IMOEX return, pre-registered +/-3% flat band):

| window | IMOEX return | regime |
|---|---|---|
| 2025-04-01 -> 2025-06-30 | -3.95% | trend-down |
| 2025-07-01 -> 2025-09-30 | -5.75% | trend-down |
| 2025-10-01 -> 2025-12-30 | +4.51% | trend-up |
| 2026-01-05 -> 2026-03-31 | +0.70% | flat |
| 2026-04-01 -> 2026-06-30 | -15.39% | trend-down |
| 2026-07-01 -> 2026-09-17 | -2.55% | flat |

Readout: only one of six OOS quarters is a bull (trend-up) quarter, so "two
bull quarters" is not the story; the tail risk is the opposite direction —
three trend-down quarters, including a -15.39% Q2 2026, are where the short
book earned. Per-day alpha is positive and significant under quarter clusters
(t=4.66, df=5), but per-trade alpha is not (t=1.47), and both regressions show
a significant negative loading on the long-only equal-weight benchmark,
consistent with the short tilt. These are observations for the human go/no-go
review, not an automated decision.

## OOS freeze (Task 10)

The abs-10d recipe is frozen for the next two quarters. The manifest stores the
model and config SHA256, feature order, thresholds, tickers, sizing and cost
fields; `cmd/oosfreeze` recomputes the recipe hash from the current files and
resets the collection count to zero when it changes.

| field | value |
|---|---|
| frozen at | 2026-09-17T16:09:40Z |
| code SHA | 30a422a9eb7193ab9300d00b4ec7c9190b98e94e |
| recipe SHA256 | 7d6e616c8973e8d2c32110e049b5a539c616ba367fab86732ac0aa850e51de9e |
| model SHA256 | feb827494ba0bc943ab722d1dff888e0fe4c523bd170d463a5e3ca2fa11e6140 |
| config SHA256 | 54c8d1949fd8c3b514c8a2b9c23a6c9048d7e20a433ad24038ab8eec6540b1ef |
| minimum collection | 183 days |
| real-money execution | disabled |

## Short-borrow measurement and stress (Task 11)

Live Tinkoff sandbox measurement via `cmd/borrowmeasure` (180-day operations
lookback). The broker API exposes short availability and short margin risk
rates per share but does not expose a borrow fee rate; no margin-fee
operations were observed in the account history, so the pre-registered
0.005%/day stress is the go/no-go borrow rate.

| date | short-enabled names | margin-fee ops | measured rate | applied rate | artifact |
|---|---|---|---|---|---|
| 2026-09-17 | 17 of 18 (DATA not shortable) | 0 (0 RUB) | unmeasurable | 0.005%/day stress | docs/borrow-report.md |

Per-ticker short risk rates (Dshort = minimal-margin short risk rate,
DshortMin = initial-margin short risk rate) are in `docs/borrow-report.md`.
DATA reports short-enabled=false with zero short risk rates, so a SELL signal
on DATA cannot open a short in the sandbox. The engine now supports
`-borrow-pct-day` (`backtest.Config.BorrowPctPerDay`), charging the rate daily
against short-leg notional and reporting `TotalBorrow` plus
`RealizedPnlNetBorrow = RealizedPnl - TotalBorrow`.

<!-- shadow-reconciliation:start -->

## Shadow reconciliation (Task 3)

Live decision vs replayed backtest decision for the same ticker and last closed
day. `FeatureMatch` compares candle-derived features (return_pct and
price/volume indicators); news, order-book and event fields are live-only and
excluded. `SignalMatch` compares the model action. Rows are appended by the
live trader when run with `--shadow-digest docs/metrics.md`; no live sandbox
rows are available yet at commit time.

| ticker | day | live | replay | feature match | max delta | signal match |
|---|---|---|---|---|---|---|

<!-- shadow-reconciliation:end -->

## Sandbox/paper execution quality tracking (Task 16)

Live tracking of expected (bounded limit) versus actual fill price, broker
rejection rate under `risk.max_slippage_pct`, and actual short-borrow charges.
Expected price is the limit price capped by the slippage band; a positive
slippage in basis points means the fill was better than the cap, a negative
value means worse. The trader logs each fill in real time and emits a periodic
digest (every 6 hours) that cross-references observed slippage against the
Task 12 per-ticker half-spread and observed margin-fee borrow charges against
the Task 11 0.005%/day stress rate.

| date | window | fills | rejected | rejection rate | mean slippage bps | max adverse bps | max favorable bps | borrow fees | artifact |
|---|---|---|---|---|---|---|---|---|---|
| 2026-09-17 | live sandbox (no fills yet at commit time) | - | - | - | - | - | - | - | `internal/filltracking`, `internal/borrowcost.SumMarginFees` |

The table is populated by the live trader's periodic `executionQualityReporter`
log once the sandbox produces orders; there are no live fill numbers to record
at commit time.

## Negotiations/sanctions signal evidence gate (Task 6)

Pre-registered with-vs-without comparison of the two new topic signals. Both
variants train and backtest on the identical 18-ticker daily dataset
(`data-28f-full.csv`, absolute-10d label, deadband 0.5%, colsample 0.8,
thresholds 0.60/0.40, 15000₽/position, 1M deposit, commission 0.05%,
spread+slippage 0.05%+0.05%, realized P&L). The only difference is the feature
set: 18 price/flow columns vs 18 + `negotiations_signal` + `sanctions_signal`.

Validation AUC (held-out decision dates 2026-06-18 -> 2026-09-16):

| variant | xgb | lgbm | logreg | ensemble |
|---|---|---|---|---|
| 18-feature baseline | 0.4896 | 0.5172 | 0.5080 | 0.5084 |
| 20-feature (+2 topic) | 0.4920 | 0.5172 | 0.5080 | 0.5098 |

Delta: ensemble +0.0014 — within the ±0.005 AUC noise band recorded in AGENTS.md.

6-quarter walk-forward realized P&L (closed trades, gross - commission), RUB:

| window | 18-feature | 18-feature max DD | 20-feature | 20-feature max DD |
|---|---|---|---|---|
| q1 (2025-03-05 -> 2025-06-05) | +5405.91 | 2.85% | +4519.17 | 2.84% |
| q2 (2025-06-06 -> 2025-09-05) | -1650.63 | 3.52% | -454.51 | 3.55% |
| q3 (2025-09-06 -> 2025-12-03) | +38813.88 | 0.66% | +39302.95 | 0.64% |
| q4 (2025-12-04 -> 2026-03-04) | -1828.22 | 1.94% | -2798.89 | 1.79% |
| w2 (2026-03-05 -> 2026-06-06) | -1639.66 | 2.04% | +2045.18 | 1.67% |
| w1 (2026-06-07 -> 2026-09-16) | +62145.47 | 1.26% | +61125.45 | 1.26% |
| total | +101246.75 | 3.52% | +103739.34 | 3.55% |

Delta: 20-feature is +2492.59 RUB (+2.5%) higher, but the sign is window-mixed
(worse in q1, q4, and w1) and the AUC delta is below the noise threshold.

Verdict: no confident measurable improvement. The available news archive
covers only 2026-08-14 -> 2026-09-17, so `negotiations_signal` and
`sanctions_signal` are zero in every training window (9 and 5 non-zero rows,
all in the final test window). The early-window differences are colsample
dilution from two extra zero-variance columns, not learned signal value. This
needs a richer historical news backfill before the comparison can test the
signals themselves (plan data-availability caveat).

## Backtest metrics: Sortino, Calmar, CAGR, min-trades floor

`internal/backtest.Result` (`engine.go`) now also reports `Sortino`, `Calmar`,
and `CAGR`, and exposes `Result.StatisticallySignificant()` /
`MinTradesForSignificance = 30`. `Result.Markdown()` prints all three plus a
`⚠ N closed trades < 30 — metrics are not statistically significant` warning
line when the run has fewer than 30 closed trades.

Two Sharpe conventions coexist in this codebase and are **not** directly
comparable:

- `internal/backtest.curveStats` (engine) computes an **annualized** Sharpe
  and Sortino from daily equity-curve returns, using **sample standard
  deviation** (N-1, via `meanStd`) times `sqrt(252)`. This changed from
  population std (N) to sample std (N-1) so it agrees with the estimator
  below; the shift moves historical backtest Sharpe values by a few tenths
  of a percent, not their sign or order of magnitude.
- `internal/strategyvalidation.SharpeRatio` (used by PSR/DSR/Harvey-Liu/PBO
  in the Attempt registry above) is a **per-period, non-annualized** sample
  Sharpe computed directly on the registry's quarterly return series. It is
  deliberately left un-annualized because DSR/PSR are calibrated on the same
  period granularity as the input series.

`Sortino` uses the same sample-std convention, restricted to the downside
(negative daily returns only), annualized the same way. `CAGR` is computed
from the first/last equity-curve point and the elapsed calendar days
(`(final/initial)^(365.25/days) - 1`, in percent). `Calmar = CAGR /
|MaxDrawdownPct|` (0 when max drawdown is 0).

## HTML tearsheet and an automated walk-forward harness

`internal/tearsheet` renders a self-contained HTML report (`html/template` +
hand-built inline SVG, no external assets or JS) from a `backtest.Result`:
key-metrics grid, equity-curve and drawdown SVGs, per-ticker attribution, and
the full closed-trades table, plus a sibling `<path>.metrics.json` summary.
Wired into `cmd/backtest` as `-tearsheet <path>`.

`internal/trainrun` is `cmd/trainmodel`'s former `runPipeline` extracted into
an importable package (`trainrun.Run`) so both `cmd/trainmodel` and the new
`cmd/walkforward` share one train+OOS-backtest implementation; behavior is
unchanged (verified by moving the existing end-to-end/leakage tests into
`internal/trainrun` and rerunning them).

`cmd/walkforward` is a new CLI that actually iterates walk-forward windows
instead of only recording them for audit (`internal/walkforward` previously
only persisted a single window's decisions). `internal/walkforward.GenerateWindowSpecs`
builds rolling or anchored (`-anchored`) `(from, split, till)` windows from
`-train-days/-test-days/-step-days`; the CLI retrains via `trainrun.Run` on
each window's `[from, split)`, evaluates OOS on `[split, till)`, and persists
each window's config/model hash (`walkforward.Save`, unchanged) plus its OOS
metrics (`walkforward.SaveMetrics`, new: Sharpe/Sortino/Calmar/CAGR/MaxDD/hit
rate/`StatisticallySignificant`). `internal/walkforward.Aggregate` chains the
per-window OOS equity curves into one compounding index (each window's
returns are stitched onto the previous window's ending level, since each
window is its own fresh-deposit backtest) and reuses the now-exported
`backtest.CurveStats` / `backtest.ComputeAttribution` to produce a single
aggregate `backtest.Result`, which gets the same `Markdown()` report and
`-tearsheet` HTML as a normal backtest.

**PBO was dormant** (see the Task 7 section above: `Registry.Matrices` empty,
`PBOComputable() == false`). `cmd/walkforward -candidates <file.json>` runs a
grid of candidate configs (buy/sell thresholds, horizon, deadband) across the
same window periods, builds a `periods x candidates` matrix of
`NetPnl/deposit` returns, archives it as JSON (`{"<pbo-key>": matrix}`,
default path `<wf-dir>/pbo_matrix.json`), and calls the existing
`strategyvalidation.ProbabilityOfBacktestOverfitting` (CSCV) directly, split
count via the new `strategyvalidation.DefaultSplits` (largest even value <=
`min(periods, 16)`). `cmd/strategyvalidation -matrices <path>` loads that
archive into `Registry.Matrices` so `PBOComputable()` and the printed PBO are
real instead of the permanent "NOT computable" placeholder — verified
end-to-end with a synthetic matrix (`PBO[key]: 0.0000 (s=6, 0/20 combinations
overfit, ...)`) and confirmed the no-`-matrices` default path still prints
"NOT computable".

No real-market walk-forward/PBO run has been recorded yet (this section adds
the harness and tooling, not a new go/no-go number); the existing Task 7
DSR/PBO figures above remain the current evidence and are not superseded.
