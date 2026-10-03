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

## Dividend capture event-study (2) — REJECTED 2026-09-25

Pre-registered protocol: `docs/plans/20260925-event-driven-dividend-drift.md`;
tool `cmd/dividendstudy`; calendar `data/dividends.jsonl` (123 events 2021-2027
via T-Invest `GetDividends`, 65 in study window 2024-01-01..2026-12-19).
Position 30000 RUB notional, lot-quantized (GAZP/GMKN/MTSS/RUAL=10, rest 1),
costs comm+spread+slip 0.05% per leg, exit at ex-date close.

Mean excess return over IMOEX by entry offset (td before LastBuyDate);
bootstrap CI resampling whole waves (seed 42, 2000 repl); tax = holding-tax on
received dividend (0.13 worst-case, 0.00 if `DividendNet` is already net):

| offset | excess (tax 0.13) | CI (tax 0.13) | excess (tax 0.00) | CI (tax 0.00) | LOO (0.13 / 0.00) |
|---|---|---|---|---|---|
| -10 | +0.0050 | [-0.0149,+0.0198] | +0.0114 | [-0.0083,+0.0263] | year fail / all pass |
| -5 | -0.0027 | [-0.0165,+0.0104] | +0.0037 | [-0.0102,+0.0168] | ticker..wave fail / year+wave fail |
| -3 (primary) | -0.0063 | [-0.0223,+0.0097] | +0.0001 | [-0.0165,+0.0165] | all 4 fail / all 4 fail |
| -2 | -0.0034 | [-0.0124,+0.0067] | +0.0030 | [-0.0066,+0.0137] | all 4 fail / season+year+wave fail |
| -1 | +0.0006 | [-0.0052,+0.0079] | +0.0071 | [+0.0008,+0.0151] * | all 4 fail / all pass |

Mean gap ratio **-0.97** (ex-date open ~ one full dividend below LastBuy close:
the drop is ~fully priced). Cover: 65 events, 9 waves, 80% outside largest
wave — mass bars pass; rejection is falsification, not data scarcity.

Verdict: **REJECT** — pre-registered acceptance needs a contiguous subset
containing {-2,-3,-5}; all of those contain 0 in CI and fail leave-one-out
under both tax conventions. Only offset -1 excludes 0 (and only at tax 0.00)
— outside the pre-registered subset, a single lucky offset decides nothing by
protocol. Dividend capture is not deployable as the (2) income bet; the income
lever reverts to (1) at 2-3x notional under a `max_net_exposure` cap. Detail:
`docs/dividendstudy-report.md`.

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

## News classifier recalibration attempt (2026-09-25) — negative, not deployed

Attempted to recalibrate `news_classifier.json` on current market data: corpus
`~/dev/fin/finanalys/cache/news_history.jsonl` (1517 headlines, 2026-08-14..
09-24, 20 tickers), labels = forward excess return over IMOEX from live MOEX
ISS candles (`cmd/trainnewsmodel`). Results on date splits:

- horizon 3d (current recipe), split 2026-09-05: val AUC 0.568, val acc 0.605
  vs baseline 0.591. Current deployed classifier (h=3, trained 2026-09-16 on
  the 48k-sample Telegram backfill) scores **all** corpus headlines inside
  [-0.22, 0.09] — effectively dead weight, never fires, consistent with its
  val AUC 0.533 < baseline 0.552.
- horizon 10d (matches the ensemble's 10d label) looks good on paper —
  val AUC 0.80-0.89 across splits — but is a **label-imbalance/drift
  artifact**: 84% of corpus headlines score < -0.3 (model learned "everything
  negative"). Label diagnostics: h=10 val labels are 43 pos / 91 neg (68%
  majority), train 87/300; a blanket-negative bias therefore fakes edge.
  Semantically wrong: "Сбербанк рекордная прибыль" -> -0.625,
  "АКРА повысило до AA(RU)" -> -0.566.
- Horizon 5d: val AUC 0.587 — noise.

Conclusion: the news classifier cannot be meaningfully recalibrated on
1517 rows / 42 days; both the current noise classifier and the horizon-10
candidate are unusable (dead / degenerate negative bias). This is the
documented data-scarcity bottleneck (AGENTS.md), not a tuning issue. The
deployed `news_classifier.json` is kept unchanged; a live AA-upgrade headline
still scores -0.058 (wrong sign). Retry only after a substantially larger,
direction-label-balanced news corpus accumulates (the 9171-article Telegram
backfill archive itself was not retained).

## Alpha-without-beta hedge test (0) — 2026-09-25

Pre-registered protocol: `docs/plans/20260925-alpha-without-beta-test.md`
(committed `d39da47` BEFORE the run; read-before-numbers gate passed by the
peer session). Executor: `cmd/alphahedge`. Decision test = does the deployed
ensemble's daily alpha survive **execution-level** beta-neutralization via a
synthetic IMOEX overlay (no position sign or size ever changes; per-trade
alpha is invariant by construction and is NOT an acceptance condition).

Pipeline: rerun of the deployed `ensemble_model.json` on the canonical 18
tickers (incl. DATA), 2025-04-01 -> 2026-09-17, 1M deposit, 15000 RUB/pos,
costs 0.0005 comm + 0.0005 spread + 0.0005 slip, kill-switch on. Note: this
HEAD rerun gives 867 trades / +202640 RUB realized vs the 2026-09-17
`betaregime-report` (862 / +194293 on an earlier engine state) — same
structure, ~4% engine-fix drift; all numbers below are the HEAD rerun.

| overlay share | leg P&L | leg costs | daily alpha (cluster t, df=5) | beta1 (eqw) | beta2 (mom) | bootstrap CI {20,40} | leave-one-out sign |
|---|---|---|---|---|---|---|---|
| 0.00 (unhedged) | 0 | 0 | +0.000325 (t=3.98) | -0.291 | -0.084 | + / + outside 0 | all + |
| 0.50 | -68502 | 3744 | +0.000216 (t=3.45) | -0.181 | -0.125 | + / + outside 0 | all + |
| 1.00 (neutral) | -137004 | 7488 | +0.000101 (t=2.10) | -0.065 | -0.171 | L20 [+0.000017,+0.000190], L40 [+0.000018,+0.000179] | all + |

Block-bootstrap CI(alpha) full grid at share 1.00: L5 [-0.000002, +0.000209]
(CI touches 0), L10 [+0.000012,+0.000196], L20 [+0.000017,+0.000190],
L40 [+0.000018,+0.000179], L60 [+0.000030,+0.000179] (stationary, geometric
blocks, 2000 repl, seed 42).

Quarterly mean daily alpha at share 1.00 (N=487): 2025-Q2 +0.000012,
2025-Q3 +0.000237, 2025-Q4 +0.000152, 2026-Q1 -0.000102, 2026-Q2 +0.000088,
2026-Q3 +0.000214.

**Verdict (per pre-registered bar): (A) CI excludes 0 on contiguous {20,40}:
YES; (B) leave-one-quarter-out keeps alpha positive across all six drops:
YES. The daily alpha survives full beta-neutral execution.** The structural
net-short (beta1 = -0.291) contributed ~+137k RUB of the +203k unhedged
realized P&L; removing it leaves a small but positive daily edge of ~1.0 bp/day
(~+4.9% on deposit over the 18 months) that is robust to block length >= 10d
and to dropping any single quarter. Consequences per the protocol's
bifurcation: path (1) is authorized — an exposure-managed variant
(inverse-vol per-name sizing and a config'd `max_net_exposure` as SEPARATE,
independently validated axes; regime-gate stays demoted). Caveats that bound
the interpretation: the edge does not survive the 5-day block (extremely
short-memory dissolve), 2026-Q1 is negative post-hedge, per-trade alpha
remains statistically insignificant (t≈1.47), and the overlay assumes a
frictionless IMOEX leg (live futures would add basis/roll/financing).
**Borrow caveat (added 2026-09-25): all (0) numbers above were produced with
`-borrow-pct-day` not wired (engine default = 0) — the book is structurally
net-short, so the alpha was computed without short-borrow cost. Re-run with
`-borrow-pct-day 0.00005` (short-leg notional x 0.00005/day) via the same
executor: share=0 alpha 0.000316 (t=3.84), share=0.5 alpha 0.000206 (t=3.25),
share=1.0 **alpha 0.000090 (t=1.85, p=0.124)**, CI {20,40} still excludes 0
(L10 [+0.000000,+0.000185] touches 0, L20 [+0.000004,+0.000179],
L40 [+0.000006,+0.000169]), all six LOO drops positive -> pre-registered
acceptance (A)+(B) still HOLD at the base rate, but the edge is thinner
(1.0 bp -> 0.9 bp/day) and the shortest block is borderline. At 2x the rate
(`0.00010`) acceptance **(A) FAILS**: CI {20,40} no longer excludes 0
(L20 [-0.000008,+0.000170], L40 [-0.000006,+0.000158], only L60 positive;
LOO (B) still holds). Survival of the alpha is therefore sensitive to the
actual live short-borrow rate; treat ~0.00005/day as the load-bearing input.

## Real IMOEX-futures overlay: the (0) alpha does NOT survive the actual instrument — 2026-10-02

The (0) test above assumed a **frictionless** IMOEX leg. This run replaces it
with the tradable instrument. Executor: `cmd/alphahedge -overlay-mode futures`
(new `cmd/alphahedge/futures.go`). Real MX front-month series fetched from MOEX
ISS FORTS (contracts MXM5/MXU5/MXZ5/MXH6/MXM6/MXU6, quarterly roll **3 calendar
days before the last-trade date**), contract notional = `LASTSETTLEPRICE x
(STEPPRICE/MINSTEP) x LOTVOLUME` = 230000 x 1 x 1 = **230000 RUB** (GO 27464
RUB ~ 11.9%), fee 15.18 RUB/contract/side, GO financing optional (`-futures-go-
financing-pct-day`, 0 here = free margin). Everything else identical to (0):
canonical 18 tickers, 2025-04-01 -> 2026-09-17, 1M deposit, 15000 RUB/pos,
costs comm/spread/slip 0.0005 each, borrow 0, 2000 bootstrap repl.

HEAD rerun base for this table: 778 closed trades, realized +184077 RUB
(engine drift vs the 2026-09-25 ledger's +202640; **all rows below are the same
run**, so the comparison is internally consistent).

| overlay | share | avg contracts | leg P&L | leg costs | avg daily alpha (cluster t, df=5) | beta1 eqw | bootstrap CI {20,40} excludes 0 |
|---|---|---|---|---|---|---|---|
| none (unhedged) | 0.00 | - | 0 | 0 | +0.000300 (t=3.68) | -0.287 | YES |
| synthetic (cash IMOEX) | 0.50 | - | -66079 | 3622 | +0.000193 (t=3.18) | -0.179 | YES |
| synthetic (cash IMOEX) | 1.00 | - | -132158 | 7244 | +0.000080 (t=1.83) | -0.065 | YES |
| **real MX, nearest** | 0.50 | 0.03 | -19000 | 1681 | +0.000266 (t=3.12) | -0.284 | YES* |
| **real MX, nearest** | 1.00 | 0.48 | **-174800** | 16906 | **-0.000036 (t=-0.49)** | -0.053 | **NO** |
| real MX, fine (unit 0.01) | 1.00 | 46.7 | -144940 | 14314 | +0.000028 (t=0.39) | -0.054 | NO |

\* share 0.50 on the full-size contract "survives" only because the hedge almost
never fires (avg 0.03 contracts) — it is effectively unhedged, not a hedge.

Findings:

- **One MX contract is 230000 RUB against an average beta-exposure of ~107000
  RUB** (avg 0.48 contracts at share 1.0, max 1). The hedge is a **bang-bang**
  0/1 instrument at this book size; the quantization residual (up to +/- half a
  contract) is comparable to the signal being hedged.
- At share 1.0 the real hedge **flips the alpha negative and insignificant**
  (t=-0.49, CI includes 0) and costs 2.3x the synthetic overlay (16906 vs 7244
  RUB in fees/rolls/notional).
- **Even with a hypothetical fine-grained instrument** (finely divisible MX,
  avg 46.7 contracts) the alpha loses significance (t=0.39, CI includes 0):
  ~12800 RUB of P&L separates it from the synthetic overlay — basis/roll yield
  and close-vs-index tracking that the frictionless cash-IMOEX leg omitted.
- LOO/quarter tables: share 1.0 real-MX quarterly alphas are
  +0.000074/+0.000111/-0.000264/-0.000193/-0.000065/+0.000263 (2 of 6 negative,
  the trend-down quarters carry it), consistent with the synthetic sign but
  swamped by the hedge tracking error.

**Verdict:** the "alpha without beta" claim of (0) **does not survive contact
with the real instrument**. The book's realized P&L is predominantly a net-short
IMOEX-beta bet (unhedged alpha t=3.68); once beta is actually neutralized with
available futures, the residual is statistically indistinguishable from zero net
of realistic basis/roll/fees. This **supersedes the "(0) alpha survives"
conclusion and closes the beta-neutral income lever at the current ~200k book
size**; the live 1x unhedged config remains the operating optimum. Reopening
requires either a much larger book (so one contract is a small fraction of
exposure) or per-name single-stock futures (finer granularity + exact per-name
beta) — neither is measured here.

## Managed-beta leverage grid (1) — 2026-09-25

Same executor, canonical 18 tickers, window 2025-04-01 -> 2026-09-17, 1M
deposit, **`-shares 1` (beta-neutral book)**, costs comm/spread/slip
0.0005/0.0005/0.0005, **`-borrow-pct-day 0.00005`** (now mandatory per the
2026-09-25 decision: borrow drag scales with short notional, and leverage
widens it). "Leverage" here = raising `-target-notional` (bigger positions,
same 1M deposit) — NO broker margin/borrowed capital. Acceptance: max DD
<= 3% in all six quarters AND realized net P&L positive at every multiplier
(judged on realized, not MTM, per the house invariant).

| target notional | realized net of borrow | daily alpha (t, df=5) | CI {20,40} | max quarterly DD | result |
|---|---|---|---|---|---|
| 15000 (1x) | +197275 | +0.000090 (t=1.85) | exclude 0 (marginal L10) | 1.11% (2026-Q1) | PASS (baseline) |
| 30000 (2x) | +395785 | +0.000195 (t=1.92) | exclude 0 | 2.15% (2026-Q1) | **PASS** |
| 45000 (3x) | +589992 | +0.000293 (t=2.01) | exclude 0 incl. L5 | 2.56% (2025-Q2) | **PASS** |
| 60000 (4x) | +753632 | +0.000364 (t=1.95) | exclude 0 | **3.55% (2025-Q3)**, 3.39% (2025-Q2), 3.02% (2026-Q1) | **FAIL** |

Verdict: the beta-neutral book supports 2-3x position-notional scaling under
the 3% drawdown budget: realized P&L scales near-linearly with notional
(1x->3x roughly triples it), and the 2026-09-16 notional-20000 precedent of
breaching the 3% cap is confirmed on the hedged book at 4x (~5-7%/yr clean
alpha on 1M at 2-3x, borrow at base rate). Beyond 3x the 3% risk-gate binds;
combined with the borrow-rate sensitivity above, the margin of safety at the
top of the ladder is thin. consequence for the (1) axis: wire
`risk.target_notional` at **30000-45000** (2-3x) with `max_net_exposure`
bounded so the per-quarter DD stays under 3%, and re-confirm the live
short-borrow rate before setting more than 3x.

## `max_net_exposure` on the unhedged book — 2026-09-25, recomputed at HEAD on 2026-09-30

> **Semantics note (2026-10-01):** every row below was produced with the OLD
> signed-net cap, before it was changed to gross. On this book the two agree for
> most rows only because the positions happen not to cancel; re-read these
> numbers as "signed-net cap", not as current `max_net_exposure` behaviour. See
> "`max_net_exposure` semantics change: signed net → gross — 2026-10-01".

Same executor, canonical 18 tickers, identical window/costs/borrow as the
grid above, but **`-shares 0` (the actual live book — no futures hedge)**.
`max_net_exposure` caps |signed aggregate net position notional| at
OrderPrice-based deltas (lot-aware in live, LotSize=1 in backtest). Combined
with the unhedged baseline from `docs/dividendstudy-report.md` (2026-09-25:
unhedged 2x/3x DD 3.71%/4.43% > 3% kill-switch limit).

**All 10 rows below were re-run on 2026-09-30** at HEAD (`88d0958`→`8157903`,
`cmd/alphahedge -shares 0`) with the same 2025-04-01 → 2026-09-17 window,
deposit 1000000, costs 0.0005/0.0005/0.0005 and borrow 0.00005/day. The
original 2026-09-25 numbers are preserved below in the "was" column. Every
row moved up, and the live 1x baseline rose from +179045 to +197275, so the
2026-09-25 figures predate the 2026-09-17 feature fixes. Conclusions are
unchanged; the cliff is now measured, not inferred.

| target notional | max_net_exposure | realized net of borrow | closed trades | max quarterly DD | result | was (2026-09-25) |
|---|---|---|---|---|---|---|
| 15000 (1x, live) | 0 | +197275 | 871 | 1.96% (2026-Q1) | **PASS** | +179045 / 781 / 2.05% |
| 15000 (1x) | 90000 | +87861 | 341 | 1.71% (2026-Q1) | PASS (income cut 55% by cap) | +78592 / 295 / 1.59% |
| 30000 (2x) | 0 | +395785 | 1012 | **3.57% (2026-Q1)** | FAIL | +361018 / 914 / 3.76% |
| 30000 (2x) | 60000 | +81730 | 234 | 2.21% (2026-Q1) | PASS, wide margin | +72545 / 124 / 2.21% |
| 30000 (2x) | 60000, borrow 0.0001 (2x-stress) | +74033 | 234 | 2.31% (2026-Q1) | PASS | +64833 / 125 / 2.30% |
| 30000 (2x) | 75000 | −6555 | 50 | 2.52% (2025-Q3) | **FAIL (income turned negative)** | +84341 / 246 / 3.46% |
| 30000 (2x) | 90000 | +144058 | 249 | 1.94% (2026-Q3) | PASS | +96699 / 249 / 2.91% |
| 45000 (3x) | 0 | +589992 | 1050 | **4.26% (2026-Q1)** | FAIL | +526517 / 939 / 4.66% |
| 45000 (3x) | 90000 | −19338 | 74 | **4.95% (2026-Q3)** | **FAIL (income and DD both worse)** | +84165 / 205 / 4.00% |
| 45000 (3x) | 120000 | +127923 | 188 | **3.85% (2026-Q1)** | FAIL | +106789 / 220 / 3.99% |

Verdict: `max_net_exposure` does **not** turn 2-3x on the unhedged book into a
safe income upgrade. The only cap-level that keeps every quarter <= 3% (2x +
60k, DD 2.21%, 2.31% under 2x-borrow stress) earns +81730 over the window —
**less than half the current live 1x (+197275, DD 1.96%)**. 3x fails at every
cap, and at 90k the cap now makes results *worse on both axes at once*:
realized −19338 and DD 4.95% (vs 4.00% uncapped), the only row in the grid that
is worse than its own uncapped control. The cap is discontinuous and its
cliff is non-monotonic: 60k→75k at 2x turns +81730 into −6555 (trade count
collapsing 234 → 50 as the cap empties the book), while 90k recovers to
+144058 — so the profitable region is non-contiguous and cannot be tuned
into. Root cause: the unhedged book's income IS its market beta, and an
aggregate cap barrels away the cross-name diversification the strategy
depends on (234-341 trades vs 871-1050 uncapped). Consequences: (i) stay at
`target_notional 15000` (1x) unhedged — it is the operating optimum of the
live book; (ii) the real 2-3x income lever is the futures hedge ([0] path),
not a net-exposure cap; (iii) `max_net_exposure` ships and stays available as
a defense-in-depth fail-closed guardrail only, not as the sizing mechanism.

## News classifier rebuild from a fresh 9596-article backfill (2026-09-29) — negative, NOT deployed

Second, larger attempt at the news sentiment classifier, prompted by the live
trader's own `news_classifier.json` (2026-09-16) scoring **below the majority
class baseline** (val acc 0.529 vs baseline 0.552, val AUC 0.533). The
2026-09-25 attempt used only 1517 finanalys headlines; this one rebuilt a
9596-article archive from scratch (the 9171-article 2026-09-17 backfill was
never retained — see the section above).

Archive: `cmd/newsfetch -out data/news_history.jsonl` — RSS/Google/ЦБ/MOEX at
`-since-days 30` (2315 records) plus `-retro-from 2026-04-01` Telegram retro
(7281 records), 2026-04-01..09-29, 42 tickers. Labels: forward excess return
over IMOEX from live MOEX ISS candles, `cmd/trainnewsmodel`, horizon 3d.

| variant | articles | train_n | val_n | train AUC | val AUC | val acc | baseline | Δ acc |
|---|---|---|---|---|---|---|---|---|
| deployed 2026-09-16 | 48294 (Telegram) | 48294 | 2364 | 0.690 | 0.533 | 0.529 | 0.552 | −0.023 |
| A all tickers, split 08-15 | 9596 | 2855 | 1349 | 0.840 | 0.531 | 0.538 | 0.570 | −0.032 |
| B config tickers only, split 08-15 | 3415 | 759 | 497 | 0.950 | **0.493** | 0.469 | 0.511 | −0.042 |
| C all tickers, split 09-01 | 9596 | 3115 | 1089 | 0.830 | 0.527 | 0.534 | 0.576 | −0.042 |
| D all tickers, `-max-tickers-per-article 1` | 6858 | 2125 | 970 | 0.880 | **0.540** | 0.535 | 0.580 | −0.045 |

**Every variant is below the majority-class baseline on accuracy**, and the
train→val AUC gap (0.31–0.46) is severe overfitting. With ~1000 validation
samples the standard error on AUC is ≈0.016, so the 0.527–0.540 spread across
variants is noise — none of them is distinguishable from the deployed model,
and none beats chance direction.

Sample loss is the mechanism: 9596 articles yield only 2855 train samples
because `BuildNewsLabels` drops tickers whose `source.History` fails
(`internal/model/newsclassifier.go:246`) and truncates the tail where
`exitIdx >= len(candles)`. The 2026-09-16 classifier had 48294 train samples;
today's rebuild has 2855 — a 17x reduction that no longer reaches the
original archive depth (Telegram retro reaches 2 channels only, RSS has no
history API).

**The decisive finding is behavioural, not the AUC.** Scoring 800 unique
archive headlines through each model (`cmd/newsscore -method model`):

| model | score range | mean | share \|score\|>0.1 | share < 0 |
|---|---|---|---|---|
| deployed `news_classifier.json` | [−0.247, +0.266] | −0.039 | 7.8% | 83.0% |
| A | [−0.699, +0.796] | −0.061 | 63.5% | 66.9% |
| D | [−0.807, +0.691] | −0.076 | 63.6% | 66.4% |
| B | [−0.925, +0.966] | −0.090 | 67.1% | 70.6% |

The deployed classifier is **dead** — 92% of its scores sit inside ±0.1, so
`news_sentiment` contributes ~0 to the live feature vector. The freshly trained
variants are **alive**: they fire with confident values on ~64% of headlines.
Since their accuracy is below the majority baseline, deploying them would swap
a currently-harmless near-zero feature for an actively wrong one. The dead
deployed model is the safer of the two, and no change is justified.

Conclusion: confirms and strengthens the 2026-09-25 verdict on 6x more data.
The blocker is the unrecoverable archive depth, not tuning. `news_classifier.json`
stays at the 2026-09-16 artifact. Retry only when a substantially larger,
direction-label-balanced corpus accumulates (i.e. after months of live
`cmd/newsfetch` appends, not a one-off backfill).

ToS note: this training set intentionally includes the 2 Telegram channels and
17 per-ticker Google News feeds, per the user's explicit decision recorded in
AGENTS.md. These numbers are therefore measured on ToS-restricted sources and
must not be quoted as if the corpus were clean.

## `max_net_exposure` semantics change: signed net → gross — 2026-10-01

Decision (user, final): the cap now bounds **GROSS** exposure — the sum over
tickers of `|signed position notional|` — instead of `|signed aggregate net|`.
The flag, config key (`max_net_exposure`) and `risk.Config.MaxNetExposure` keep
their names; only the meaning changed. Implemented as `risk.ExposureReader`
(`ExposureByTicker`), with `NetExposureReader` kept only as a fallback for
readers that do not expose per-ticker legs.

Why: the net cap is blind to gross risk. A long 500k in SBER and a short 500k
in OZON net to **zero** and sail past a 60k cap while carrying **1M** of gross
risk, purely because the two legs happened to match in size. Net also made
*which* names got filled depend on how much opposing exposure happened to be
booked first. Gross removes that order dependence. For defence-in-depth this
matters more than income — every income lever on the unhedged book is already
closed by evidence, and the cap ships as a guardrail, not a lever.

Two implementation details that are load-bearing:

- **Projection is per ticker, not book-wide addition.** The order replaces only
  its own ticker's leg; what is tested is `|other names| + |this name after|`.
  Treating every order as purely additive to gross makes the cap block the very
  orders that *reduce* risk (closing/trimming), which strands open positions.
- **`Signal.TargetLots` is not the delta.** In the target-position path it is
  the absolute target that `recordFill` reconciles to, while exposure must be
  projected from the change the order makes. Reading the target as a delta
  double-counts the position already held. `risk.Request.ExposureDeltaLots`
  now carries the signed delta explicitly; nil means "TargetLots is the order
  size" (the live `EnsembleSignalSource` path, which is incremental).

This also fixed a latent bug: the target-position path used to hand the gate an
absolute target that the gate read as a delta, double-counting every existing
position in the cap projection.

### Regression check: 100% short grid unchanged

Per the acceptance criterion, the 2026-09-30 all-short grid was re-run after the
change. On that book no positions cancel by sign, so net ≡ gross and every
number must be identical. All five rows reproduce **exactly** (realized net of
borrow, closed trades and max DD):

| cap | realized net of borrow | closed trades | max DD | vs 8157903 |
|---|---|---|---|---|
| 0 | +51381.3719969013531337807737386387384896435 | 1581 | 5.83% | identical |
| 30000 | +4981.29619215 | 2 | 0.86% | identical |
| 45000 | +5863.689259878 | 3 | 1.04% | identical |
| 60000 | +8830.730678178 | 4 | 0.87% | identical |
| 90000 | +11483.628720054 | 6 | 1.68% | identical |

An intermediate implementation that added `|delta|` to book-wide gross passed
this check on cap 0 but collapsed the capped rows to **0 closed trades** at an
unchanged drawdown — i.e. it silently blocked the final flatten-to-flat order
and left the whole result as open-position MTM. That is why the grid was re-run
rather than trusted, and why the per-ticker projection and the regression test
`TestHardenedGateMaxExposureAllowsReducingOrder` exist.

Live is unaffected: `config.sandbox.yaml` keeps `max_net_exposure: 0` (cap
disabled), so no live behavior changed with this commit.

## Where the 9596 news articles are actually lost — 2026-10-01

Measurement, not opinion. Re-audited the 2026-09-29 archive
(`data/news_history.jsonl`, 9596 records, `cmd/trainnewsmodel` defaults
horizon 3d, split 2026-08-15) with an instrumented replica of
`BuildNewsLabels` that counts every drop point and **asserts it produces the
same sample count as the real function**. Reproduce with:

```
MOEX_TRADER_NEWS_AUDIT_ARCHIVE=$PWD/data/news_history.jsonl \
  go test ./internal/model/ -run TestNewsLabelsDropAudit -v
```

| stage | articles lost | note |
|---|---|---|
| input archive | 9596 | 42 tickers |
| dropped: empty ticker/date | 0 | |
| **(a)** ticker dropped, `source.History` error | **0** | no ticker fails lookup |
| **(a)** ticker dropped, `< horizon+2` candles | **0** | no ticker is short of history |
| **(b)** article `pubIdx < 0` | **0** | no article predates its last candle |
| **(c)** article `exitIdx >= len(candles)` | 385 | genuine tail, expected |
| (d) entry/exit price <= 0 | 0 | |
| **(e)** IMOEX entry candle missing | **2757** | **not one of the three suspected points** |
| **(f)** IMOEX exit candle missing | **2055** | **not one of the three suspected points** |
| samples produced | **4399** | train 2855 / val 1544 at the 2026-08-15 split |

`train = 2855` reproduces the documented figure exactly, confirming the audit
matches the original run.

**The hypothesis was wrong: (a) and (b) cost nothing.** The entire loss is
the IMOEX benchmark lookup at (e)/(f), which kills 4812 of the 9596 pairs —
more than the 5197 "lost" headline implies, because those drops happen after
(c). Cause: **96.5% of the missing index dates are weekends** (Sat 2347 + Sun
2300 of 4812; only 165 are Fridays). Verified directly against MOEX ISS:

- `SBER` returns **real weekend bars** — 15 bars for 2026-06-01..06-15, 4 of
  them weekend, with genuine OHLC (Sat 06-06 open 322.21 close 322.45).
- `IMOEX` returns **no weekend bars** — 10 bars for the same window, 0 weekend.

So MOEX publishes weekend sessions for equities while the index is not
calculated then. A stock's `entryIdx`/`exitIdx` lands on a Saturday/Sunday bar,
`indexByDate[dateKey(...)]` misses, and the pair is dropped. This is a
**calendar-alignment bug, not a ticker-mapping problem** — worth fixing on its
own merits, and it is cheap: dropping weekend bars before the loop would
recover up to ~4647 samples, roughly doubling the corpus.

### Same root cause reaches the PRICE model — MEASURED 2026-10-02

`internal/model/dataset.go` builds price labels with `candles[d+1+horizonDays]`
— bar-index arithmetic on the same unfiltered ISS series. It was measured with
the env-gated `TestHorizonAudit` / `TestHorizonModeLabelDiff`
(`MOEX_TRADER_HORIZON_AUDIT=1`), 18 deployed tickers, 2024-01-01..2026-10-01.
**Both earlier hypotheses were wrong:**

1. **The weekend bars are real sessions, not phantom.** `SBER` Sat
   2026-06-06 carries volume 1 373 638 and 55 intra-day 10-min bars; Sun
   2026-06-07 volume 930 144; `OZON`/`YDEX` likewise (10-26k). So MOEX trades
   equities 7 days/week while IMOEX is not calculated on weekends.
2. **There is no live/backtest parity gap.** Live's `marketHoursExecutor`
   (cmd/trader/main.go:1030) asks the broker `TradingStatus`
   (internal/broker/tinkoff/sandbox.go:168) — in a weekend session it returns
   open, so live trades exactly the bars backtest does. The `+179k`/`+197k`
   track record is **not** invalidated by this mechanism.

What *is* real: a fixed **10-bar** window spans a different **calendar** time
per ticker because weekend-session participation differs. Pooled over 14 041
labels: effective **weekday** horizon 7-11 (mean **9.31**); only **9.3%** of
labels get exactly 10 weekday sessions. Mean **calendar** span per ticker ranges
**11.33 d** (DATA) to **13.69 d** (OZON) with within-ticker ranges 10-63 d —
i.e. "abs-10d" actually measures an **11.3-13.7-day** window, and it is
**not constant across tickers** (~+21% OZON vs DATA). This is the label-timing
inconsistency, and it is a real candidate for the AUC~0.50-0.52 ceiling.

Rebuilding through the real `BuildSamples` with `HorizonModeCalendarDays`
(the new `-horizon-mode calendar_days` option, `internal/model/dataset.go`
`forwardExitIndex`, default unchanged = bars): bars emits 12 926 labels,
calendar 12 819; of 12 328 common `(ticker, decision-day)` keys **667 (5.41%)**
change sign, plus 598 only-bars and 491 only-calendar status changes.

The bars-vs-calendar comparison ran on the deployed ensemble pipeline:
`exportdataset -news-history data/news_history.jsonl` (18 tickers, 2024-01-01..
2026-09-17, split 2026-06-18, horizon 10, absolute, deadband 0.5%) →
`scripts/export_ensemble.py` (18 price/flow columns, colsample 0.8) →
`cmd/backtest -signal-source ensemble`. Identical holdout 2026-06-19..
2026-09-17, 1M deposit, 15 000₽/position, commission 0.05%, spread+slippage
0.05%+0.05%, `-lookback-days 0`:

| horizon unit | val AUC (2026-06-18..09-16) | realized | closed trades | max DD |
|---|---|---|---|---|
| bars (deployed) | 0.4610 | +53 786.79 | 204 | 1.58% |
| calendar_days | 0.4635 | +40 355.35 | 217 | 1.97% |

The bars baseline reproduces the documented news-aware holdout realized
(+56 379 @ 277ebf7) to ~5%, validating the pipeline. Calendar mode moves AUC by
**+0.0025** (inside the ±0.005 noise band) while realized P&L drops **−25%** and
max DD rises **1.58% → 1.97%**. **The label-timing inconsistency is real, but it
is not the AUC ceiling, and redefining the horizon to calendar days is worse** —
the deployed bars recipe stays. Caveat: this is a single 3-month holdout (204-217
trades); a 6-quarter walk-forward would strengthen it but is not run because the
AUC signal — the quantity the hypothesis is actually about — is flat.

## `max_net_exposure` on a 100% short book — 2026-09-30

Trigger: the live book failed on 2026-09-30 with 17 short positions, net
≈ −210k RUB against a gross of ≈ +210k RUB, and realized P&L of −4.53 RUB
over 60 closed trades. The question this grid answers is what a
`max_net_exposure` cap would have cost **that** structure — not whether it
would have saved it.

Method: synthetic forced-short via `-signal-source csvprob`, SELL on every bar
and BUY on the final bar (2026-09-17) so the result is realized rather than
open-position MTM. The CSV date set was taken from MOEX ISS and verified
identical to the engine's candle set (8674 rows == the ensemble control's
`Decisions: 8674`). Protocol otherwise identical to `abe614d`: canonical 18
tickers, 2025-04-01 → 2026-09-17, deposit 1000000, `target_notional` 15000,
`max_lots` 1000, commission 0.0005, spread 0.0005, slippage 0.0005, borrow
0.00005/day, kill-switch on, `-lookback-days 0`, realized net of borrow.

| max_net_exposure | realized net of borrow | income hit vs all-short cap 0 | closed trades | max DD | names in book |
|---|---|---|---|---|---|
| 0 (all-short control) | +51381 | — | 1581 | 5.83% | 18 |
| 30000 | +4981 | **−46400 (−90.3%)** | 2 | 0.86% | 2 (YDEX, OZON) |
| 45000 | +5864 | **−45518 (−88.6%)** | 3 | 1.04% | 3 (+ SBER) |
| 60000 | +8831 | **−42551 (−82.8%)** | 4 | 0.87% | 4 (+ LKOH) |
| 90000 | +11484 | **−39898 (−77.7%)** | 6 | 1.68% | 6 (+ GAZP, GMKN) |

Verdict: **rejected.** A cap costs 78–90% of the directional book's realized
income and is not a risk knob for it. The mechanism is not gradual
de-risking — `risk.Gate` is an order-level fail-closed check
(`internal/risk/gate.go:227-257`) that never resizes an existing position.
Against a constant SELL every bar, the first N tickers to fill the cap
consume it, every later order is rejected, and the book freezes for the
remaining 18 months: closed trades collapse 1581 → 2/3/4/6, and only the
final close-out ever books. So the capped rows are **not** a scaled version of
the same book but an arbitrary subset picked by ticker iteration order.

Two caveats that cut against over-reading this table:

- The income hits are therefore a **lower bound** on the cap's true cost. The
  rows measure a frozen arbitrary subset, whereas a real concentrated book
  rebalances; the comparable rebalancing variant is worse, not better. Name
  selection here is not reproducible as a strategy.
- The uncapped all-short control breaches the 3% kill-switch DD limit on its
  own (DD 5.83%, 39 daily-loss blocked days) before any cap is applied. That
  is a property of holding 18×15000 short simultaneously, not of the cap —
  and it means the live loss was **not** caused by a missing
  `max_net_exposure`.

Baseline drift, recorded for honesty: rerunning the `abe614d` protocol
unchanged at HEAD (ensemble source, 18 tickers, same window/costs/borrow)
gives realized **+197275 net of borrow / 871 closed trades / DD 2.63%**,
not the +179045 / 781 / 2.05% recorded in the `abe614d` table above — that row
predates the 2026-09-17 feature fixes. Every income hit in *this* table is
therefore computed against the fresh all-short cap-0 control measured in the
same run, never against the stale ledger number.

This strengthens rather than revises the 2026-09-25 verdict: `max_net_exposure`
ships as defence-in-depth only. Live stays 1x, `target_notional` 15000, cap 0.
