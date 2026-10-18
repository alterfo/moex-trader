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

Pre-registered protocol: `docs/plans/completed/20260925-event-driven-dividend-drift.md`;
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

PBO is implemented as generic CSCV code with tests, but it was NOT computable
from this summary-only registry: no per-strategy period-return matrices were
archived for the ~40 attempts. This is superseded by the 2026-10-04
`cmd/strategyvalidation -returns-dirs` path, which persisted per-window
`period_returns.csv` and computed CSCV PBO = 0.1529 for the Task 13 registry —
see "Deployment decision 2026-10 (Task 13)".

## Period-return matrices and pre-registered attempts (Task 4)

`cmd/walkforward` and `cmd/backtest -wf-dir` now persist one
`period_returns.csv` per window (`date,realized_net`, one row per trading day,
decimal strings) alongside `window.json`; the window record stores the file
name and a SHA256 of the CSV and `Window.Validate`/`Load` check both, so a
tampered or missing daily series fails loudly. Daily realized net is the sum
of closed-trade `NetPnl` by close date, matching the existing realized-P&L
definition (gross minus commission).

`cmd/strategyvalidation -returns-dirs a,b,c` loads each variant's per-window
CSVs, joins them by date into a `trading_dates x variants` matrix (missing
dates filled with 0), and computes CSCV PBO via
`ProbabilityOfBacktestOverfitting` with `DefaultSplits`. Disjoint variant
calendars are an error. This makes PBO computable from separately-run
walk-forwards without the old single-run `-candidates` grid.

The registry now pre-registers the six attempts of this plan before any of
their numbers exist, so the multiple-testing penalty is honest from the start:
`purged_walkforward_control` (Task 5) plus `experiment_a`..`experiment_e`
(Tasks 7-11), all `realized_pnl_6q_purged` with value 0 and status `pending`
(the control is `control`). Runtime `DefaultRegistry` attempt count is now
47 = 41 historical attempts + 6 pre-registered. The Task 7 table above is the
historical 41-attempt DSR computation and is not recomputed here.

## Purged walk-forward baseline (2026-10-04)

Honest six-window walk-forward of the deployed recipe (absolute-10d label,
18 price/flow features, buy/sell 0.60/0.40, 15000 RUB target notional, 1M RUB
deposit, commission/spread/slippage 0.05% each, hold-until-flip, kill switch
on) on the current 17-ticker config universe (DATA excluded — dropped live on
2026-09-21 for `short-enabled=false`). Each window retrains a fresh ensemble
via `scripts/export_ensemble.py` on rows with `date <= train_till` (the day
before the OOS window), then backtests OOS via `cmd/backtest -signal-source
ensemble` on the portfolio-level engine. Two training variants per window:
`-embargo-bars 0` (old behaviour) and `-embargo-bars 10` (purge every training
row whose `label_date` reaches into the test window). Embargo is implemented
in calendar days on both the Go and Python paths. The purged (embargo 10) run
is the deployment-gate control (`purged_walkforward_control`).

Dataset: the 2026-09-18 corrected `data-28f-full.csv` export from the ai-box
`moex-trader-ns-task5` workspace (434-day EMA/SMMA convergence warmup),
filtered to the 17 config tickers. `val_auc` is the fixed held-out window
2026-06-18 -> 2026-09-16 that `scripts/export_ensemble.py` prints; for w1
(2026-06-07 -> 2026-09-16) that window overlaps the OOS window, so w1's val
AUC is not a clean out-of-sample measure. Persisted per-window
`-wf-dir` (window.json + period_returns.csv) on the ai-box under
`/home/oleg/moex-trader-task5/wf/embargo0_17` and `.../embargo10_17`.

| window | emb0 realized | emb0 trades | emb0 max DD | emb0 kill | emb0 val AUC | emb10 realized | emb10 trades | emb10 max DD | emb10 kill | emb10 val AUC |
|---|---|---|---|---|---|---|---|---|---|---|
| q1 (2025-03-05 -> 2025-06-05) | +8373.93 | 251 | 3.08% | no | 0.5941 | +4301.27 | 66 | 3.57% | yes | 0.5947 |
| q2 (2025-06-06 -> 2025-09-05) | +2935.09 | 216 | 3.51% | no | 0.5654 | +9.36 | 215 | 3.50% | no | 0.5730 |
| q3 (2025-09-06 -> 2025-12-03) | +36239.52 | 194 | 0.66% | no | 0.5357 | +34089.03 | 193 | 0.90% | no | 0.5301 |
| q4 (2025-12-04 -> 2026-03-04) | -9612.54 | 185 | 2.27% | no | 0.5246 | -12339.10 | 187 | 2.46% | no | 0.5257 |
| w2 (2026-03-05 -> 2026-06-06) | -538.58 | 133 | 1.78% | no | 0.4962 | +2676.41 | 141 | 1.17% | no | 0.4872 |
| w1 (2026-06-07 -> 2026-09-16) | +47635.22 | 292 | 1.33% | no | 0.5313 | +31092.50 | 283 | 3.05% | no | 0.5115 |
| total | +85032.64 | 1271 | — | — | — | +59829.47 | 1085 | — | — | — |

Realized P&L is closed-trade gross minus commission (MTM never drives the
verdict). Mean val AUC: embargo 0 = 0.5412, embargo 10 = 0.5370 (delta
-0.0042, inside the +/-0.005 noise band). Purging costs -25203.16 RUB
(-29.6%) of realized P&L but the control stays strongly positive, so Task 5
does not trigger the stop-before-Task-7 warning.

Two engine notes for the deployment gate (Task 13). (1) Several windows show
max DD above 3% without the persistent kill switch tripping (q1 emb0 3.08%,
q2 emb0 3.51%, q2 emb10 3.50%): the gate evaluates drawdown on the risk-gate
equity basis at order time while the reported max DD is the day-close MTM
curve. (2) `scripts/export_ensemble.py` embargo purge had a string-vs-
Timestamp comparison bug that crashed the embargo-10 training path; fixed by
parsing `label_date` with `pd.to_datetime` before the cutoff comparison.

## Experiment A: dividend-adjusted (total-return) labels (2026-10-04)

Same pipeline as the Task 5 control but with `LabelModeAbsoluteTR`: whenever a
dividend ex-date (first trading session after `last_buy_date`) falls inside
(entry, exit], `dividend_net` is added to the exit price before the 10-day
forward return is computed. Dataset rebuilt on the ai-box with the identical
17-ticker, 434-day-warmup, cost-widened recipe
(`-label-mode absolute_tr -commission-pct 0.0005 -dividends data/dividends.jsonl`),
then the same per-window embargo-10 ensemble retrain + OOS backtest. Label
balance moves 2855 pos / 3509 neg (control) to 2934 pos / 3347 neg (TR).

| window | control realized | TR realized | control trades | TR trades | control max DD | TR max DD | control kill | TR kill | control val AUC | TR val AUC |
|---|---|---|---|---|---|---|---|---|---|---|
| q1 (2025-03-05 -> 2025-06-05) | +4301.27 | +5866.61 | 66 | 64 | 3.57% | 3.49% | yes | yes | 0.5947 | 0.5978 |
| q2 (2025-06-06 -> 2025-09-05) | +9.36 | -304.50 | 215 | 212 | 3.50% | 3.63% | no | no | 0.5730 | 0.5777 |
| q3 (2025-09-06 -> 2025-12-03) | +34089.03 | +17936.85 | 193 | 201 | 0.90% | 1.58% | no | no | 0.5301 | 0.5288 |
| q4 (2025-12-04 -> 2026-03-04) | -12339.10 | -10116.08 | 187 | 184 | 2.46% | 2.07% | no | no | 0.5257 | 0.5195 |
| w2 (2026-03-05 -> 2026-06-06) | +2676.41 | -8232.75 | 141 | 131 | 1.17% | 2.60% | no | no | 0.4872 | 0.4943 |
| w1 (2026-06-07 -> 2026-09-16) | +31092.50 | +25446.12 | 283 | 276 | 3.05% | 3.89% | no | no | 0.5115 | 0.5113 |
| total | +59829.47 | +30596.25 | 1085 | 1068 | — | — | — | — | — | — |

Realized P&L is closed-trade gross minus commission. Verdict: **REJECT**.
Dividend-adjusted labels lose -29233.22 RUB (-48.9%) of realized P&L versus
the purged control and stay positive in only 3 of 6 windows (control: 5 of 6),
while mean val AUC is statistically unchanged (0.5382 vs 0.5370, +0.0012,
inside the +/-0.005 noise band) — the dividend signal is too small relative to
the 10-day holding window to help, and it degrades the windows where it adds
noise (q2, w2). `experiment_a_dividend_adjusted` is marked rejected in the
registry. Persisted per-window `period_returns.csv` under
`/home/oleg/moex-trader-task7/wf/tr_17`.

## Experiment B: wide training universe, unchanged trading universe (2026-10-04)

Trains the same absolute-10d, 18-feature, 0.60/0.40 ensemble on a wider
universe while trading and evaluating only the 17 config tickers. The wide
list is `data/train_tickers.json`: 262 common+preferred shares on ISS TQBR
(`SECTYPE` 1/2, fetched 2026-10-04 via `cmd/listtqbr`). A name contributes a
training row only on dates where its trailing median 20-day turnover is at
least 10,000,000 RUB, computed from bars available on that date only
(`-train-tickers` + `-train-min-turnover 10000000`, `internal/model/liquidity.go`),
and only train-split rows are kept for the 140 wide-only names, so the
validation/evaluation set stays exactly the 17 trading tickers. FX tickers are
rejected outright. The 17-ticker train/val rows are byte-identical to the
control (train 5398, val 966, none 4380); the wide set adds 27557 labeled
training rows across 140 names (label 0 = 15965, label 1 = 11592).

| window | control realized | wide realized | control trades | wide trades | control max DD | wide max DD | control kill | wide kill | control val AUC | wide val AUC |
|---|---|---|---|---|---|---|---|---|---|---|
| q1 (2025-03-05 -> 2025-06-05) | +4301.27 | +2757.66 | 66 | 36 | 3.57% | 4.38% | yes | yes | 0.5947 | 0.5679 |
| q2 (2025-06-06 -> 2025-09-05) | +9.36 | -193.10 | 215 | 162 | 3.50% | 3.37% | no | no | 0.5730 | 0.5452 |
| q3 (2025-09-06 -> 2025-12-03) | +34089.03 | +36987.00 | 193 | 157 | 0.90% | 0.93% | no | no | 0.5301 | 0.5065 |
| q4 (2025-12-04 -> 2026-03-04) | -12339.10 | -9034.41 | 187 | 139 | 2.46% | 2.36% | no | no | 0.5257 | 0.4880 |
| w2 (2026-03-05 -> 2026-06-06) | +2676.41 | +15475.07 | 141 | 87 | 1.17% | 0.59% | no | no | 0.4872 | 0.4927 |
| w1 (2026-06-07 -> 2026-09-16) | +31092.50 | +40825.25 | 283 | 186 | 3.05% | 2.20% | no | no | 0.5115 | 0.5106 |
| total | +59829.47 | +86817.47 | 1085 | 767 | — | — | — | — | — | — |

Realized P&L is closed-trade gross minus commission. Verdict: **REJECT**.
The wide set raises total realized P&L to +86817.47 RUB (+45.1% vs control)
and matches/exceeds the control in 4 of 6 windows, but it fails the
pre-registered gate on two other criteria: mean val AUC drops to 0.5185 vs
control 0.5370 (-0.0185, outside the +/-0.005 band) and q1 has max DD 4.38%
with the kill switch tripping (criterion requires DD <= 3% in every window
and the kill switch never trips). The extra names help realized P&L mostly in
w2/w1 while diluting discriminative signal on the early windows.
`experiment_b_wide_train_narrow_trade` is marked rejected in the registry.
Persisted per-window `period_returns.csv` under
`/home/oleg/moex-trader-task8/wf/wide_17`.

## Experiment C: probability calibration and ensemble-vs-members (2026-10-04)

Adds probability-calibration tooling (`internal/model/reliability.go`: Brier
score, 10-bin ECE, reliability table) and a `cmd/strategyvalidation
-reliability <wf-dir>` reader. `cmd/backtest -wf-dir` now persists one
`reliability.csv` per window (`date,ticker,probability,label`) where the label
is the same absolute-10d / 0.5%-deadband forward-return label used for
training, computed only for decision dates whose forward window is complete.
The same Task 5 embargo-10 per-window ensemble artifacts are replayed four
ways through the new `-ensemble-member` flag: ensemble average, LGBM only,
XGBoost only, logistic regression only, all at buy/sell 0.60/0.40.

Reliability over the six purged windows (pooled decision/realized-label
pairs):

| member | samples | Brier | ECE (10 bins) |
|---|---:|---:|---:|
| ensemble | 7046 | 0.302700 | 0.209763 |
| lgb | 7825 | 0.330966 | 0.255687 |
| xgb | 7030 | 0.311151 | 0.222077 |
| logistic | 7046 | 0.290461 | 0.199672 |

The logistic member has the best calibration; every model is still materially
miscalibrated in the tails (positive rate near 0.42-0.55 in both extreme bins
despite predicted probabilities near 0.07 and 0.93). Per-window realized P&L
(closed trades, gross minus commission):

| window | ensemble realized | lgb realized | xgb realized | logistic realized |
|---|---:|---:|---:|---:|
| q1 (2025-03-05 -> 2025-06-05) | +4301.27 | +2729.41 | +3653.26 | -1249.64 |
| q2 (2025-06-06 -> 2025-09-05) | +9.36 | -10924.04 | -5940.92 | -4772.99 |
| q3 (2025-09-06 -> 2025-12-03) | +34089.03 | +10658.77 | +24486.59 | +43237.16 |
| q4 (2025-12-04 -> 2026-03-04) | -12339.10 | -9134.91 | -10402.34 | -7407.85 |
| w2 (2026-03-05 -> 2026-06-06) | +2676.41 | +432.66 | +3356.23 | +15346.89 |
| w1 (2026-06-07 -> 2026-09-16) | +31092.50 | +25405.11 | +26728.56 | +36229.09 |
| total | +59829.47 | +19166.99 | +41881.38 | +81382.67 |

Per-member summary:

| member | total realized | trades | positive windows | worst max DD | kill-tripped windows | mean val AUC |
|---|---:|---:|---:|---:|---:|---:|
| ensemble | +59829.47 | 1085 | 5/6 | 3.57% | 1 | 0.5370 |
| lgb | +19166.99 | 1578 | 4/6 | 3.26% | 0 | 0.5245 |
| xgb | +41881.38 | 1175 | 4/6 | 3.79% | 1 | 0.5037 |
| logistic | +81382.67 | 904 | 3/6 | 3.69% | 1 | 0.5653 |

Realized P&L is closed-trade gross minus commission; the ensemble rerun is
bit-identical to the Task 5 control at +59829.47 RUB. The logistic single
member leads on both realized P&L and calibration, but no deploy decision is
made here — Task 13 evaluates all C variants against the pre-registered gate
(note logistic and xgb trip the kill switch in q1 and have worst DD above 3%).
Persisted `wf-dir` runs and `reliability.csv` files are under
`/home/oleg/moex-trader-task9/wf/c_{ensemble,lgb,xgb,logistic}`.

## Experiment D: volatility-scaled position size (2026-10-04)

Adds `risk.vol_scale` (enabled/min_mult/max_mult, default disabled) and applies
it inside `EnsembleSignalSource` sizing: the target notional is multiplied by
`clamp(σ_median / σ_ticker, min_mult, max_mult)` before lots are derived, where
`σ_ticker` is the existing `realized_volatility` feature and `σ_median` is the
cross-sectional median of positive `realized_volatility` across the configured
tickers with data on the same decision bar. A zero or missing `σ_ticker` (or a
bar with no positive volatilities) falls back to multiplier 1. All arithmetic
is `decimal.Decimal`; the median is injected per bar by the backtest engine via
`FeatureContext.CrossSectionalVolatility`, so live and backtest share the same
sizing path. The option is wired as `-vol-scale -vol-scale-min -vol-scale-max`
in both `cmd/backtest` and `cmd/walkforward`, and persisted in the per-window
`walkforward.Config`. The pre-registered setting is min 0.5 / max 1.5, replayed
over the same Task 5 embargo-10 per-window ensemble artifacts (no retraining —
vol-scale is a sizing-only change).

| window | control realized | vol-scale realized | control trades | vol-scale trades | control max DD | vol-scale max DD | control kill | vol-scale kill |
|---|---:|---:|---:|---:|---:|---:|---|---|
| q1 (2025-03-05 -> 2025-06-05) | +4301.27 | +3930.80 | 66 | 65 | 3.57% | 3.60% | yes | yes |
| q2 (2025-06-06 -> 2025-09-05) | +9.36 | +270.78 | 215 | 215 | 3.50% | 3.42% | no | no |
| q3 (2025-09-06 -> 2025-12-03) | +34089.03 | +34211.13 | 193 | 186 | 0.90% | 0.77% | no | no |
| q4 (2025-12-04 -> 2026-03-04) | -12339.10 | -12406.69 | 187 | 189 | 2.46% | 2.45% | no | no |
| w2 (2026-03-05 -> 2026-06-06) | +2676.41 | +3425.01 | 141 | 152 | 1.17% | 1.02% | no | no |
| w1 (2026-06-07 -> 2026-09-16) | +31092.50 | +30378.44 | 283 | 263 | 3.05% | 3.13% | no | no |
| total | +59829.47 | +59809.47 | 1085 | 1070 | — | — | — | — |

Realized P&L is closed-trade gross minus commission. Verdict: **REJECT**.
Volatility scaling with min 0.5 / max 1.5 is essentially P&L-neutral: total
realized +59809.47 RUB vs the purged control +59829.47 RUB (-20.00 RUB,
-0.03%), so it fails deployment-gate criterion 1 (total realized P&L must
exceed control). It stays positive in 5 of 6 windows like the control, and the
per-window max DD / kill-switch profile is nearly identical (q1 still trips the
kill switch; q1/q2/w1 still print day-close max DD above 3%). The sizing
change redistributes a few percent of notional across names but does not add
edge over the flat 15000₽/position control. `experiment_d_volatility_scaled` is
marked rejected in the registry. Persisted `-wf-dir` runs are under
`/home/oleg/moex-trader-task10/wf-volscale`.

## Experiment E: sector exposure cap (2026-10-04)

Adds `risk.sector_caps` (sector -> max gross position notional) and
`risk.sectors` (ticker -> sector) to config, enforced in
`HardenedGate.ApproveReason` with the same per-ticker projection as
`max_net_exposure`: the order replaces only its own ticker's leg, so the
check is `Σ_{t∈sector, t≠ticker} |notional_t| + |notional_ticker after
order| ≤ cap`. Risk-reducing orders (trim/flatten) therefore always pass when
the projected sector gross is inside the cap, a cap of zero allows only
flatten-to-flat, and a ticker without a sector is never blocked. Default
sector map: banks SBER/VTBR/T, oil&gas LKOH/ROSN/TATN/NVTK/GAZP, metals
GMKN/PLZL/CHMF/RUAL, all others unassigned (uncapped); default caps off.
Pre-registered setting: 45000 RUB per sector (= 3 positions at the 15000 RUB
target notional), replayed over the same Task 5 embargo-10 per-window ensemble
artifacts (no retraining — the cap is a risk-gate-only change). Wired as
`-sector-caps`/`-sectors` in `cmd/backtest` and persisted in the per-window
`walkforward.Config`.

| window | control realized | sector realized | control trades | sector trades | control max DD | sector max DD | control kill | sector kill |
|---|---:|---:|---:|---:|---:|---:|---|---|
| q1 (2025-03-05 -> 2025-06-05) | +4301.27 | -5697.73 | 66 | 79 | 3.57% | 2.76% | yes | no |
| q2 (2025-06-06 -> 2025-09-05) | +9.36 | +2533.41 | 215 | 46 | 3.50% | 2.65% | no | no |
| q3 (2025-09-06 -> 2025-12-03) | +34089.03 | +20352.66 | 193 | 89 | 0.90% | 0.90% | no | no |
| q4 (2025-12-04 -> 2026-03-04) | -12339.10 | -7523.17 | 187 | 49 | 2.46% | 1.74% | no | no |
| w2 (2026-03-05 -> 2026-06-06) | +2676.41 | +2386.11 | 141 | 60 | 1.17% | 1.11% | no | no |
| w1 (2026-06-07 -> 2026-09-16) | +31092.50 | +22805.32 | 283 | 107 | 3.05% | 2.49% | no | no |
| total | +59829.47 | +34856.60 | 1085 | 430 | — | — | — | — |

Realized P&L is closed-trade gross minus commission. Verdict: **REJECT**.
The 45000-per-sector cap costs realized P&L: +34856.60 RUB vs the purged
control +59829.47 RUB (-24972.87 RUB, -41.7%), so it fails deployment-gate
criterion 1 (total realized P&L must exceed control). It wins on risk: every
window stays under the 3% drawdown floor, the kill switch never trips, and
only 4 of 6 windows are positive (q1 flips from +4301.27 to -5697.73, the
only window the control tripped its kill switch on). The cap truncates the
concentrated sector legs that drove the control's best windows, cutting the
trade count from 1085 to 430; the reduced gross exposure is safer but the
income being cut is market beta, matching the `max_net_exposure` finding on
the unhedged book. `experiment_e_sector_caps` is marked rejected in the
registry. Persisted `-wf-dir` runs are under
`/home/oleg/moex-trader-task11/wf-sector`.

## Experiment F: excess-to-IMOEX labels (2026-10-18)

Pre-registered protocol: `docs/plans/completed/20261018-excess-label-walkforward.md` (committed before the run). Question: does the excess-label ensemble pass the Task 13 deployment gate against a fresh same-run absolute control, and does the 2026-10-03 single-window val-AUC advantage (+0.040) transfer to the purged 6-window walk-forward? Protocol: the Task 5 recipe unchanged (6 quarterly windows, `-embargo-bars 10`, 17 tickers, 1M deposit, 15000₽/position, commission/spread/slippage 0.05% each, hold-until-flip, kill switch on, thresholds 0.60/0.40, 18 price/flow features), `-label-mode excess` (F) vs `-label-mode absolute` (F-control), both exported/retrained/backtested on HEAD on the ai-box (`~/moex-trader-task12`), same news archive as Task 5.

Reproducibility verification (pre-registered): the fresh absolute export reproduces the Task 5 dataset byte-for-byte — all 27 non-key fields identical for every common (ticker, date) row (SBER spot check, 0 diffs); the only difference is one extra tail row per ticker (2026-09-17), inside the pre-registered ±10-row tolerance.

Stack drift disclosure: training output changed vs the 2026-10-04 Task 5 run despite identical data, script and env vars — the venv package versions drifted (xgb 3.4.1, lgb 4.7.0, sklearn 1.9.1, pandas 3.0.5, numpy 2.5.3 as of 2026-10-18; Task 5 versions not recorded). The engine is not the cause: the historical `ens-emb10-q1.json` artifact replayed through the HEAD engine gives realized +4002.28 / 74 trades / DD 3.56% / kill yes vs the recorded +4301.27 / 66 / 3.57% / yes (≈93% reproduction). All gate comparisons are therefore against the fresh same-run control, never the stale ledger number.

| window | control realized | F realized | control trades | F trades | control max DD | F max DD | control kill | F kill | control val AUC | F val AUC |
|---|---|---:|---:|---:|---:|---:|---:|---|---|---:|---:|
| q1 (2025-03-05 -> 2025-06-05) | +858.10 | +13303.17 | 243 | 194 | 3.07% | 1.63% | no | no | 0.5673 | 0.4559 |
| q2 (2025-06-06 -> 2025-09-05) | +355.57 | -10261.75 | 205 | 174 | 3.53% | 2.35% | no | no | 0.5550 | 0.4858 |
| q3 (2025-09-06 -> 2025-12-03) | +32191.60 | -9240.48 | 158 | 100 | 0.94% | 1.97% | no | no | 0.5178 | 0.4767 |
| q4 (2025-12-04 -> 2026-03-04) | -10183.21 | -1543.91 | 183 | 33 | 2.43% | 0.46% | no | no | 0.4999 | 0.4915 |
| w2 (2026-03-05 -> 2026-06-06) | +16747.59 | -6836.71 | 115 | 40 | 0.68% | 1.43% | no | no | 0.4836 | 0.4892 |
| w1 (2026-06-07 -> 2026-09-16) | +27846.13 | +24248.90 | 252 | 184 | 3.68% | 1.71% | no | no | 0.4785 | 0.4989 |
| total | +67815.78 | +9669.22 | 1156 | 725 | — | — | — | — | mean 0.5170 | mean 0.4830 |

Realized P&L is closed-trade gross minus commission. Gate against the fresh same-run control:

1. total realized: +9669.22 vs +67815.78 — **FAIL**;
2. realized ≥ control in ≥ 4/6 windows: 1/6 (q1 only) — **FAIL**;
3. max DD ≤ 3% everywhere and kill never trips: **PASS for F** (max 2.35%, no kill; the control itself prints 3.07–3.68% day-close DD without kill trips — the same order-time vs day-close basis documented in Task 5);
4. mean val AUC not below control by more than 0.005: 0.4830 vs 0.5170 (−0.034) — **FAIL**;
5. PBO {F-control, F}: 0.1242 (s=16, 510 periods × 2 candidates) — PASS, but degenerate at 2 candidates, reported descriptively;
6. 90-day preflight: not run (criteria 1–4 failed).

**Verdict: REJECTED.** H_predictability is rejected: the excess val-AUC advantage does not transfer to walk-forward; on the current stack excess is −0.034 mean val AUC vs absolute, and q1 excess AUC (0.4559) is below chance. H_income is rejected (fails criteria 1, 2, 4). The excess book does show the expected lower-beta profile: max DD ≤ 2.35% everywhere and total realized +9669.22 RUB > 0 — the label removes most of the beta income by design. Consequences: the excess model is not wired as challenger (the precondition in the plan was a positive H_predictability); the 2026-10-03 single-window AUC +0.040 is recorded as non-reproducible in the walk-forward protocol on the current stack. Persisted `-wf-dir` runs under `/home/oleg/moex-trader-task12/wf/{abs17,exc17}`.

## Sell-bias diagnostic: event features, out-of-range guard, staleness (2026-10-18)

Pre-registered in `docs/plans/completed/20261018-sell-bias-diagnostic.md` before the runs. Data: `trader-sandbox.db` audit events (`ingest` payloads carry the full feature vector, `llm` payloads carry action/probability/hold_reason), 2026-09-16..10-18; ~38.3k decisions since the 2026-10-10 news-masking fix.

D1 (descriptive). Live decisions by day: 09-23..10-09 ran ~4200-4800 SELL/day with near-zero HOLD (the news-feature train/serve skew documented 2026-10-10). From 10-10: ~3600-4200 HOLD/day and ~460-1200 SELL/day, 0 BUY. Post-fix hold_reason split: `out_of_range` ~50-60% of decisions, `model` ~30%, SELL ~14%. Post-fix probability bins: <0.40: ~31% (of which <0.35: ~10%), [0.40,0.60]: ~69%, >=0.60: 0.11% (42 decisions).

D2 (guard). 100% of post-fix `out_of_range` blocks are `event_dividend` with z=14..38. The deployed `ensemble_model.json` carries mean=0/std=1/coef=0 for all 8 event features and both topic signals, and no tree references them — they cannot move the probability. The guard (added 2026-10-10 to catch the news blow-up) therefore vetoed decisions on features that do not influence the model: post-fix it swallowed 7138 would-SELL, 42 would-BUY (all VTBR — the only p>=0.60 decisions) and 14861 would-HOLD. Live pre-guard SELL share: (5202+7138)/38281 = 32.2%. **Verdict: the guard vetoes on structurally-untrained event features during dividend season.** Fix: `outOfRangeFeature` skips features with logistic coef==0 that no tree uses (`internal/model/ensemble.go`), regression test `TestEnsembleIgnoresUntrainedEventFeatures`; `TestEnsembleHoldsOnOutOfRangeFeature` (mom_21d=500) still vetoes. Deploying the fix to the live trader is a separate owner decision (watchdog restart).

D3 (offline replay, deployed model, fixed guard, 2026-10-10..10-18, 136 decisions): 95 HOLD / 41 SELL / 0 BUY, p in [0.263, 0.597]. Replay SELL share 30.1% vs live pre-guard 32.2% (within the pre-registered ±10pp); replay BUY share 0% < 1%. **Verdict: the lean reproduces offline from data+model — not a serving-path defect.**

D4 (fresh model, same recipe, trained till 2026-09-16, same window): 94 HOLD / 39 SELL / **3 BUY**, p in [0.254, 0.626]. BUY share 2.2% vs deployed 0% — the pre-registered criterion (+1pp) is met, but on 3 decisions it is weak evidence: **part of the 0-BUY is model staleness**; SELL shares match (28.7% vs 30.1%). No auto-swap; a manual retrain decision stays with the owner (AGENTS.md invariant).

Conclusion: the residual post-fix sell-lean decomposes into (a) the out-of-range guard vetoing on untrained event features — fixed in code, not yet deployed; (b) the model's own p-distribution on recent data (>=0.60 in 0.11% live, 0% offline replay); (c) a small staleness component in the BUY tail (fresh model 2.2% BUY).

**Deployment attempt and revert (2026-10-18, same day).** The guard fix was deployed to the live Mac trader (rebuild + watchdog restart) and the preflight gate refused to start: 90-day replay with the fix realized **-4859.13** net of borrow (gross -4361.55, borrow 497.58, 98 closed trades, hit 41.8%, DD 1.12%) vs **+602.32** (gross +1105.67, borrow 503.35, 107 trades, hit 48.6%, DD 0.90%) for the vetoing build on the same window (2026-10-17 pass) and +7156.33 on 2026-10-15. The gate is honest: unblocking the vetoed decisions turns the recent 90-day book negative, i.e. **the accidental event-dividend veto acts as a profitable regime filter** and the model underneath is not profitable without it. The code fix is reverted in HEAD (the pre-fix binary was restored and the trader restarted through the watchdog); the veto behaviour stays live. Re-landing the fix requires a candidate model that passes the preflight gate with the veto removed — pre-registered as a separate hypothesis, not a code change.

## Manual retrain candidate (2026-10-18) — REJECTED, no swap

Pre-registered in `docs/plans/completed/20261018-manual-retrain.md` (owner-approved manual retrain). Candidate: same recipe as the deployed ensemble (absolute-10d, 18 features incl. news, 28-name order, 0.60/0.40), trained on a fresh export (`-from 2024-01-01 -till 2026-10-18`, 18 tickers incl. DATA, current 14619-record news archive) with `ENSEMBLE_TRAIN_TILL=2026-10-17`, embargo 10. Both models replayed through the HEAD engine (guard fix included), 17 tickers, 1M deposit, 15000₽/position, costs 0.0005×3, kill switch on.

| window | deployed realized | candidate realized | deployed trades | candidate trades | deployed max DD | candidate max DD |
|---|---|---|---:|---:|---:|---:|
| hold30 (2026-09-17 -> 2026-10-17) | +583.21 | -3281.95 | 32 | 45 | 1.24% | 1.45% |
| pre90 (2026-07-20 -> 2026-10-17) | +10419.63 | -1508.68 | 137 | 164 | 1.69% | 2.27% |

Gate: candidate realized P&L >= deployed on hold30 (FAIL: -3281.95 vs +583.21) and on pre90 (FAIL: -1508.68 vs +10419.63). **No swap: `ensemble_model.json` stays.** The candidate's fresh BUY signals (2.2% vs 0% in the D4 diagnostic) are unprofitable in this window; extending the training window degrades the SELL side more than it repairs the BUY tail. Retrain again only via the same gate after at least a quarter of new data from 2026-10-17.

## Preflight-concentration analysis (2026-10-18)

Pre-registered in `docs/plans/completed/20261018-preflight-concentration.md` (owner chose "stop and investigate" after the gate refused the live config). `cmd/backtest` gained `-max-net-short`/`-max-net-long` so the gate replay is reproducible; gate-shaped replays (90d rolling, 17 tickers, 1M deposit, costs 0.0005×3 + per-ticker spreads from `trader-sandbox.db`, borrow 0.00005/day, caps 90000/90000, kill switch on, deployed `ensemble_model.json`, no news — exactly like the live gate) give realized gross +9843.33 (till 10-14) → +3637.89 (till 10-16) → **-1990.32** (till 10-18), matching the gate's own decline (+7156 → +602 → -4859 net of borrow; the CLI lacks lot quantization, hence the level offset). ISS has no bars for 10-17/10-18, so the decline is the left boundary of the rolling window, not new bars.

Closed trades of the till-10-18 gate replay, by close month (realized net, RUB):

| month | trades | sum | mean per trade |
|---|---:|---:|---:|
| 2026-07 | 20 | +414.93 | +20.75 |
| 2026-08 | 35 | +2828.55 | +80.82 |
| 2026-09 | 37 | -4821.86 | -130.32 |
| 2026-10 | 24 | -411.94 | -17.16 |

By ticker: OZON +4952.20 (3), MGNT +1444.44 (1), GAZP +885.65 (10), SBER +594.66 (14), VTBR +237.61 (19), RUAL +78.45 (10), TATN +65.69 (10), T -32.16 (4), MTSS -48.65 (9), LKOH -100.52 (3), GMKN -171.25 (8), ROSN -697.07 (6), NVTK -992.47 (4), POSI -1367.08 (3), CHMF -2331.33 (5), **PLZL -4508.49 (7)**. Daily series (84 days): 28 negative days sum -14480.71; top-10 positive days sum ~12000 (top day 2026-08-25 +3988.12).

Verdicts against the pre-registered hypotheses: **I1 (day concentration)** — partially confirmed: positives are month-concentrated (Jul-Aug) rather than day-concentrated; the top day is 33% of positive days' total. **I2 (recent decay)** — CONFIRMED: September is deeply negative (-4821.86, mean -130.32/trade), concentrated in PLZL/CHMF/POSI/NVTK; October still negative. **I3 (self-recovery)** — NOT expected soon: the window keeps dropping profitable August days while September losses stay inside until December; the rolling sum drifts further negative before stabilizing. The live book was protected because live news populates `event_dividend` and the `out_of_range` guard vetoes those decisions — the gate replays without news, so it measures the unvetoed book, which September defeats. Fixing this is a model-quality problem (September regime), not a gate problem: do not loosen the threshold.

## Deployment decision 2026-10 (Task 13)

Pre-registered gate (fixed 2026-10-04, before any experiment numbers): against
the Task 5 purged control (`purged_walkforward_control`), on the same 6
quarterly purged windows (`-embargo-bars 10`), 1M RUB deposit, costs 0.0005
commission + 0.0005 spread + 0.0005 slippage, kill switch on. A variant is
deployable only if ALL hold: (1) total realized P&L > control; (2) realized
P&L >= control in at least 4 of 6 windows; (3) max drawdown <= 3% in every
window and the kill switch never trips; (4) for label/training changes (A, B,
C), mean val AUC not below control by more than 0.005; (5) PBO over the full
registry of this plan's attempts < 0.5; (6) 90-day preflight passes.

PBO is now computable: `cmd/strategyvalidation -returns-dirs` joins the eight
persisted per-window `period_returns.csv` variants (control, A, B, C-lgb,
C-xgb, C-logistic, D, E) into a 510-period x 8-candidate matrix and computes
CSCV PBO with s=16. Result: PBO = 0.1529 (1968 of 12870 combinations
overfit), so criterion 5 passes for the set as a whole. Registry-level
deflated Sharpe (47 attempts, 6 observations, abs-10d 18-month series) =
0.6260 (p 0.3740). Per-variant deflated Sharpe on the six purged windows is
reported below but is NOT a deployment criterion.

| variant | total realized | windows >= control | worst max DD | kill-tripped | mean val AUC | fails criteria |
|---|---:|---:|---:|---:|---:|---|
| control | +59829.47 | — | 3.57% | 1 | 0.5370 | (baseline) |
| A dividend-adjusted | +30596.25 | 2/6 | 3.89% | 1 | 0.5382 | 1, 2, 3 |
| B wide-train | +86817.47 | 4/6 | 4.38% | 1 | 0.5185 | 3, 4 |
| C lgb | +19166.99 | 1/6 | 3.26% | 0 | 0.5245 | 1, 2, 3, 4 |
| C xgb | +41881.38 | 2/6 | 3.79% | 1 | 0.5037 | 1, 2, 3, 4 |
| C logistic | +81382.67 | 4/6 | 3.69% | 1 | 0.5653 | 3 |
| D vol-scale | +59809.47 | 3/6 | 3.60% | 1 | — | 1, 2, 3 |
| E sector caps | +34856.60 | 2/6 | 2.76% | 0 | — | 1, 2 |

Per-variant deflated Sharpe (six purged windows, 47-trial deflation, not a
criterion): control 0.1290, A 0.0641, B 0.2306, C-lgb 0.0347, C-xgb 0.0942,
C-logistic 0.1683, D 0.1323, E 0.0899.

Verdict: NOTHING PASSES. No candidate clears all of criteria 1-4, so criteria
5 and 6 are moot. The closest candidate is the logistic single member
(C-logistic): it passes realized P&L (1), windows >= control (2) and val AUC
(4), but fails criterion 3 — worst day-close max DD 3.69% with the q1 kill
switch tripping — so it is not deployable on the pre-registered gate. B leads
on realized P&L but fails the DD/kill and AUC criteria. E is the only variant
that satisfies criterion 3 (all DD <= 2.76%, kill never trips) but fails
criteria 1 and 2. The live recipe (`ensemble_model.json` +
`config.sandbox.yaml`) is left untouched. `experiment_c_probability_calibration`
is marked rejected in the registry (was pending).

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

Pre-registered protocol: `docs/plans/completed/20260925-alpha-without-beta-test.md`
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

## Market-neutral retraining: excess-to-IMOEX labels improve predictability and neutrality, residual alpha marginal — 2026-10-03

Phase 2 of the beta/regime work (phase 1 = real futures overlay above).
Question: does the model have residual (market-neutral) signal when trained on
**excess-to-IMOEX** labels instead of absolute forward return? Pipeline (both
label modes, otherwise identical):

```
exportdataset -config config.sandbox.yaml -from 2024-01-01 -till 2026-09-17
  -split 2026-06-18 -horizon-days 10 -deadband-pct 0.5 -label-mode <absolute|excess>
  -news-history data/news_history.jsonl -out /tmp/p2_<mode>.csv
scripts/export_ensemble.py  (18 price/flow columns, 28-name order, colsample 0.8)
cmd/alphahedge -ensemble-path <model> [-overlay-mode synthetic|futures]
```

Label balance (13 438 rows): absolute 5398 pos / 6802 neg / 1238 hold; excess
4116 / 4179 / 5143 hold (excess is balanced but the 0.5%-deadband swallows far
more rows — an important caveat: fewer directional training labels).

| model | val AUC (2026-06-18..09-16) xgb / lgbm / logreg / ensemble |
|---|---|
| absolute | 0.4895 / 0.4922 / 0.4634 / **0.4824** |
| excess | 0.5447 / 0.5237 / 0.4570 / **0.5226** |

Ensemble AUC gap **+0.040**, far outside the documented +/-0.005 noise band:
the market-neutral label is materially more predictable.

Book results (alphahedge, 2025-04-01 -> 2026-09-17, 1M, 15000 RUB/pos, costs
0.0005/0.0005/0.0005, borrow 0 unless noted, 2000 repl). Both models are
**rebuilt here**, not the deployed artifact, so the comparison is internal.

| model | overlay | trades / realized | leg P&L | avg contracts | alpha/day (cluster t, df=5) | beta1 unhedged/hedged | CI {20,40} excludes 0 |
|---|---|---|---|---|---|---|---|
| absolute | none | 789 / +221316 | 0 | - | +0.000363 (t=6.02) | -0.302 / - | YES |
| absolute | synthetic 1.0 | - | -143051 | - | +0.000130 (t=4.43) | / -0.066 | YES |
| absolute | real MX nearest 1.0 | - | -189900 | 0.42 | +0.000007 (t=0.12) | / -0.056 | **NO** |
| absolute | real MX fine 1.0 | - | -165809 | 39.8 | +0.000062 (t=1.21) | / -0.057 | NO |
| excess | none | 392 / +81881 | 0 | - | +0.000175 (t=2.96) | -0.189 / - | YES |
| excess | synthetic 1.0 | - | -43117 | - | +0.000108 (t=3.46) | / -0.073 | YES |
| excess | real MX nearest 1.0 | - | -33100 | 0.10 | +0.000125 (t=2.49, p=0.055) | / -0.029 | YES |
| excess | real MX fine 1.0 | - | -48030 | 21.7 | +0.000085 (t=2.04, p=0.097) | / -0.079 | **NO** |
| excess, borrow 5bp/d | synthetic 1.0 | - | -43117 | - | +0.000102 (t=3.17) | / -0.073 | YES |

Findings:

- **Excess labels work as intended**: the unhedged excess book loads beta1
  **-0.189 vs -0.302** for absolute (roughly halved), and carries a real
  intercept (alpha +0.000175, t=2.96) rather than a pure net-short beta bet.
- The excess book's realized P&L is ~2.6x lower (+81881 vs +221316) with ~half
  the trades — the missing chunk is exactly the beta the absolute model was
  harvesting.
- The **"real MX nearest, share 1.0" CI-excludes-0 for excess is an artifact**:
  at this book's lower beta the coarse 230k contract fires only rarely
  (avg 0.10 contracts, max 1), so that row is essentially unhedged. It is NOT
  evidence the alpha survives neutralization.
- **With a fine, actually-neutralizing hedge** (avg 21.7 contracts, fee 0 to
  isolate basis) the excess alpha is 0.000085/day (t=2.04, p=0.097, CI touches
  -0.000006 at L5) — positive and ~2.7x the dead absolute-model residual
  (0.000062, t=1.21), but **just short of the pre-registered significance bar**.
  ~0.85 bp/day ~ +4.2% on deposit over 18 months before the (0) borrow caveat.
- Borrow 5bp/day only shaves the synthetic share-1 excess alpha 0.000108 ->
  0.000102 (CI still excludes 0); the excess book is less short, so borrow drag
  is milder than for the absolute book.
- 2026-Q3 is the only true OOS quarter (model val ends 2026-06-18); its excess
  alpha is positive, but one quarter is not enough to confirm.

**Verdict: market-neutral retraining is a genuine improvement over the absolute
recipe (AUC +0.040, beta loading halved, a positive residual alpha that the
absolute model does not have), but the residual alpha after a realistic fine
hedge is still only marginal (t~2.0) and does not clear the pre-registered bar.**
Direction is worth continuing (next: beta-residualized price features and
regime-balanced validation; and/or a finer hedge instrument or a larger book so
the hedge actually neutralizes without swamping the signal). It is NOT a
deployment-ready income source on its own. Do not change the live config.

### Phase 2b: regime-balanced training — MEASURED 2026-10-03, REJECTED

Hypothesis: the excess model overfits the dominant (down) market regime, so
reweighting training samples to balance market regimes should help. Implemented
as an env-gated, **inference-neutral** option in `scripts/export_ensemble.py`
(`ENSEMBLE_REGIME_BALANCE`, `ENSEMBLE_REGIME_WINDOW`, `ENSEMBLE_REGIME_BUCKETS`);
default off, so the deployed pipeline is byte-for-byte unchanged. Market state at
the decision date is proxied by the **trailing cross-sectional mean of
`return_pct`** (recoverable from the dataset itself, so no Go change and no
look-ahead); training rows are ranked by that trailing return, split into
equal-size quantile buckets, and weighted inversely to bucket frequency. Same
excess dataset and 18-feature recipe as phase 2.

| variant | window / buckets | weighted rows | val AUC xgb / lgbm / logreg / ensemble |
|---|---|---|---|
| excess unweighted (phase-2 baseline) | - | - | 0.5447 / 0.5237 / 0.4570 / **0.5226** |
| rb21 b3 | 21 / 3 | 7483 | 0.5075 / 0.5099 / 0.4025 / 0.4964 |
| rb21 b2 | 21 / 2 | 7483 | 0.5045 / 0.5058 / 0.4055 / 0.4948 |
| rb63 b3 | 63 / 3 | 7483 | 0.4977 / 0.4886 / 0.4706 / 0.4804 |
| rb63 b4 | 63 / 4 | 7483 | 0.5028 / 0.5003 / 0.4717 / 0.4887 |

Every balanced variant lands **below** the unweighted excess AUC (0.5226), and
below the absolute baseline too. Hedged book (real MX fine, share 1.0, fee 0,
2000 repl) confirms it is not an AUC-only artifact:

| variant | trades / realized | leg P&L | avg contracts | alpha/day (cluster t, df=5) | CI {20,40} excludes 0 |
|---|---|---|---|---|---|
| rb21 b3 | 1336 / +24629 | -53186 | 32.65 | -0.000124 (t=-1.574, p=0.176) | NO |
| rb21 b2 | 1374 / +6051 | -51259 | 32.67 | -0.000165 (t=-1.942, p=0.110) | YES (entirely negative) |

**Verdict: regime-balanced training REJECTED.** It systematically lowers val AUC
and, after a realistic fine hedge, the residual alpha turns **negative** (rb21b2
CI excludes 0 on the negative side). The unweighted excess model already handles
the regime mix; forcing regime parity adds variance without signal. Separately,
**beta-residualized price features are deferred**: they require the IMOEX series
inside the live feature path (`internal/orchestrator` ingest -> `internal/features`),
i.e. a cross-zone change with a live/backtest parity risk, and the label change
already captures most of the market-neutrality — expected marginal gain is low
relative to that risk. The live config and `ensemble_model.json` are untouched.

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

## Observability metrics and alerts (Task 6)

Live-trader Prometheus metrics added in `internal/metrics/metrics.go`, wired
through `internal/orchestrator` and `cmd/trader`. The old
`llm_inference_duration_seconds` was renamed to
`moex_trader_inference_duration_seconds` (the pipeline is a model, not an LLM).

Metrics exposed on the trader's `/metrics` endpoint:

| metric | type | labels | source |
|---|---|---|---|
| `moex_trader_inference_duration_seconds` | histogram | — | orchestrator signal generation |
| `signals_generated_total` | counter | — | orchestrator |
| `risk_rejections_total` | counter | `reason` | risk gate decision |
| `executor_skips_total` | counter | `reason` | approved decisions the executor skipped |
| `signal_probability` | histogram | — | model probability per poll |
| `candle_age_seconds` | gauge | `ticker` | newest ingested candle age |
| `feature_psi` | gauge | `feature` | drift monitor warnings |
| `position_notional` | gauge | `ticker` | signed exposure per ticker (RUB) |
| `gross_exposure` | gauge | — | sum of absolute per-ticker notionals (RUB) |
| `lease_held` | gauge | — | 1 while this process holds the trading lease |
| `var_95` | gauge | — | 1-day 95% VaR of the current book (RUB) |
| `var_99` | gauge | — | 1-day 99% VaR of the current book (RUB) |
| `es_95` | gauge | — | 1-day 95% expected shortfall (RUB) |
| `es_99` | gauge | — | 1-day 99% expected shortfall (RUB) |
| `incident_shorts_total` | counter | `ticker` | SELL decisions emitted by the news gate during an incident (not every one becomes a fill) |
| `news_exit_total` | counter | `ticker`, `reason` | First exit decision for a news-opened position (`stop_loss`, `take_profit`, `time_exit`); each also writes an `audit_events` row, stage `news_exit`, with entry/peak/result for the parameter fit in `docs/plans/20261010-news-exit-tuning-and-buy-side.md` |

The `var_95`/`var_99`/`es_95`/`es_99` gauges are Task 12 monitoring only (not a
risk gate): historical 1-day book VaR/ES from 250-day trailing returns in
`internal/risk/var.go`, also reported as one line in the daily Telegram digest.

Telegram alerts (`internal/alert/telegram/watch.go`), each gated by a cooldown:

| alert | trigger | default threshold | default cooldown |
|---|---|---|---|
| probability collapse | more than 90% of the last N probabilities inside [0.45, 0.55] | N = 20, fraction 0.9, band 0.45–0.55 | 1h |
| stale candles | candle age above threshold during the trading session | 24h (daily candle pipeline) | 1h |
| PSI breach | drift monitor warning above the configured PSI threshold | drift_psi_threshold 0.2 | 1h |

Defaults come from `config.alerts` (`probability_collapse_window`,
`stale_candle_age`, `cooldown`); negative values are rejected. Nothing here
changes the live recipe: all new config keys default to the values above and
the alert senders are no-ops unless Telegram credentials are configured.

## Horizon sweep 9–40d and news-set features (2026-10-08)

Question: does a longer label horizon, or news aggregated over a window per
ticker instead of per headline, lift validation AUC toward 0.7? Verdict: no.
Nothing deploys; the live recipe (absolute 10d, 18 price/flow columns) is
unchanged. Research only, no repo code changed: one-off scripts, not committed.

Pipeline: `exportdataset -config config.sandbox.yaml -from 2024-01-01
-till 2026-10-07 -split 2026-06-18 -horizon-days <h> -deadband-pct 0.5
-label-mode absolute -news-history data/news_history.jsonl`, run on the ai-box
(`~/moex-research`, separate from the deploy dir). FX tickers excluded.
Evaluation: expanding-window purged walk-forward, 9 test folds of 63 days from
2025-04-01, training rows limited to `label_date < fold start` (embargo equals
the horizon), 15 price/flow columns, LightGBM + logistic average.

| horizon (days) | mean fold AUC | pooled AUC |
|---|---|---|
| 9 | 0.5292 | 0.5038 |
| 10 | 0.5311 | 0.4974 |
| 12 | 0.5204 | 0.4875 |
| 15 | 0.5018 | 0.4799 |
| 20 | 0.4864 | 0.4646 |
| 25 | 0.4818 | 0.4695 |
| 30 | 0.4730 | 0.4734 |
| 40 | 0.4617 | 0.4678 |

AUC falls monotonically with horizon and is below 0.5 from 15d. Per-fold AUC
ranges 0.33–0.64, so the result is regime-dependent. Horizon 10d is the best of
the sweep and is already deployed.

News-set features: rolling 5/10/20-day windows per ticker, lagged one day —
log article count, surge vs 60-day mean, negative-minus-positive keyword
balance, 8 TF-IDF/SVD topic means, distinct sources, market-wide news count
(`mkt10`). The news archive spans only 2026-04-01..2026-09-29 (9596 records),
so evaluation uses 21-day folds from 2026-06-01 (4–5 folds, 300+ training rows
minimum). "Within-day" AUC ranks tickers inside each day, removing market
timing; CI is a bootstrap over days and is too narrow because 10–20 day labels
overlap.

| horizon | features | pooled AUC | within-day AUC [95% CI] |
|---|---|---|---|
| 10 | price | 0.404 | 0.353 [0.306, 0.402] |
| 10 | news, all | 0.604 | 0.556 [0.504, 0.612] |
| 10 | news without `mkt10` | 0.541 | 0.547 [0.489, 0.607] |
| 10 | `mkt10` only | 0.579 | 0.500 |
| 20 | price | 0.389 | 0.388 [0.339, 0.439] |
| 20 | news, all | 0.565 | 0.569 [0.517, 0.620] |
| 20 | news without `mkt10` | 0.525 | 0.570 [0.512, 0.627] |
| 20 | `mkt10` only | 0.606 | 0.500 |

Reading:
- The pooled news gain is market timing: the single market-wide count `mkt10`
  matches the full news model (0.58–0.61).
- The cross-sectional news signal is 0.55–0.57; at 10d the CI includes 0.5, at
  20d the lower bound is 0.51–0.52 before correcting for label overlap. Not
  significant.
- Price features are anti-predictive across tickers in 2026-06..09 (within-day
  0.35–0.39, CI excludes 0.5). Flipping them would be fitting a known regime
  and is not a candidate.
- AUC above 0.7 was not reached by any variant; the best is about 0.57.

Revisit news-set features only after months of live `cmd/newsfetch` appends.

## Event causal study: sanctions / negotiations / incident (2026-10-09)

Protocol (fixed before the first run): `docs/plans/completed/20261009-event-causal-study.md`. Tool: `cmd/eventstudy`
(`go build ./cmd/eventstudy && ./eventstudy -ar-mode beta|excess`). Panel: 17 `config.sandbox.yaml` tickers x
140 trading days (2026-04-01..2026-10-09), 10213 archive records, abnormal return vs IMOEX (market-model beta
fitted on 2025-10-01..2026-03-31; `excess` mode as robustness), 2000 date-bootstrap and 1000 placebo draws, seed 42.
No P&L is involved: nothing here is realized or MTM, all numbers are market-adjusted returns around news days.

| topic | events / dates | stacked DiD CAR[0,+1] [95% CI] | AIPW ATT CAR[0,+1] (t) | placebo p | Holm p | MDE | verdict |
|---|---|---|---|---|---|---|---|
| sanctions | 71 / 45 | -0.0018 [-0.0094, +0.0045] | +0.0011 (0.35) | 0.675 | 0.675 | 0.0100 | not supported |
| negotiations | 58 / 37 | -0.0051 [-0.0124, +0.0015] | -0.0002 (-0.05) | 0.337 | 0.673 | 0.0098 | not supported |
| incident | 12 / 9 | -0.0230 [-0.0662, +0.0074] | -0.0238 (-1.42) | 0.026 | 0.078 | 0.0518 | underpowered |

Reading:
- **sanctions and negotiations: no detectable causal effect.** Both estimators straddle 0 on CAR[0,+1] and
  CAR[+1,+5] (e.g. negotiations drift CAR[+1,+5] -0.0084 [-0.0163, -0.0001] is the raw event-study only; the
  DiD [-0.0219, +0.0031] and AIPW -0.0023 (t -0.58) both include 0). With 58-71 events the minimum detectable
  effect is about 1.0% over two days, so effects below that are not excluded. Consistent with the 2026-09-18
  AUC rejection of `negotiations_signal`/`sanctions_signal`; this does not reopen them.
- **incident: direction matches the live BUY veto, evidence insufficient.** Raw CAR[0,+1] is -2.6% [-6.7%, -0.2%]
  and the unadjusted placebo p is 0.026, but stacked DiD and AIPW (t -1.42) include 0, Holm p is 0.078, and
  N=12 on 9 dates fails the protocol power floor (MDE 5.2%). The veto stays as defence-in-depth; this study
  neither confirms nor refutes its P&L value. Revisit when live `incident` triggers reach >= 30 events.
- **Pre-trend is clean** for all three (CAR[-5,-1] CIs include 0), so no leakage artefact explains the nulls.
- **TWFE vs stacked DiD agree in sign and size for sanctions/negotiations** (k=0 coefficients -0.0013 and -0.0021,
  SE ~0.002-0.003, all |t| < 1), so staggered-timing bias is not what drives the null here. The `mna` TWFE row
  (1 event) is degenerate and must be ignored.
- Exploratory (not gated): dividend and report topics also show no effect; `dividend` market-level IMOEX
  2-day difference -0.0101 [-0.0181, -0.0022] on 16 top-decile days is a single unadjusted contrast across 7
  topics and is not evidence of tradable drift (consistent with the 2026-09-25 dividend-capture rejection).
- `excess` mode reproduces every verdict (sanctions DiD -0.0004, negotiations -0.0060, incident -0.0235;
  Holm p 0.933 / 0.517 / 0.090).
- Limits: 6 months, one regime; ticker attribution of generic news biases effects toward 0 (VKCO is 41% of
  records); timestamps can lag the first reaction, so [0,+1] is partly pre-news. IV was not run (no valid
  instrument). Doubly robust SEs ignore nuisance-model estimation error.

## Macro and dividend-yield features (2026-10-10)

Question: does adding trailing dividend yield and daily macro series (IMOEX, CNY/RUB, Brent, key rate) lift
walk-forward AUC? Protocol pre-registered in `docs/plans/completed/20261010-macro-fundamental-features.md` section 8
before the first run. Verdict: no. Nothing deploys; the live recipe (absolute 10d, price/flow columns) is
unchanged and the success criterion was not met by any arm, so the P&L backtest step was not run.

Pipeline: `exportdataset -config config.sandbox.yaml -from 2024-01-01 -till 2026-10-09 -split 2026-06-18
-horizon-days 10 -deadband-pct 0.5 -label-mode absolute -macro-dir data/macro` (commit 4a49b9d), run on the
ai-box; 12540 labelled rows, 44.4% positive, FX tickers excluded, no news history. Evaluation: `evalh.py wf()`,
expanding-window purged walk-forward, 9 test folds of 63 days from 2025-04-01, LightGBM + logistic average.

| arm | columns | mean fold AUC | pooled AUC | folds > 0.5 |
|---|---|---|---|---|
| A base | 15 price/flow | 0.5320 | 0.4976 | 6/9 |
| B +div | A + `div_yield_ttm_pct` | 0.5270 | 0.4963 | 6/9 |
| C +macro | A + 9 macro | 0.4912 | 0.4175 | 3/9 |
| D +both | A + B + C | 0.4853 | 0.4178 | 4/9 |

- Arm A reproduces the 2026-10-08 horizon-sweep baseline (0.5311 / 0.4974) to within 0.002, so the harness is unchanged.
- Dividend yield is neutral to slightly negative (-0.005 mean fold, -0.001 pooled). Macro columns are clearly
  harmful (-0.041 mean fold, -0.080 pooled): they are identical for every ticker on a given day, so the trees
  can only use them to memorise the date/regime, which does not transfer to the next fold.
- Criterion (>= +0.010 pooled AUC over A and a majority of folds > 0.5): met by no arm.
- Limits: one regime, 9 folds, 17 tickers; macro could still help as a regime filter in the risk gate rather
  than as a model input; fundamentals other than dividend yield were not tested (no point-in-time source).

## Level-crossing event study: stops and take-profits (H20, 2026-10-15)

Question: does the price behave differently after crossing a level where stop-loss and take-profit orders plausibly
cluster (recent extremes, round numbers) than after crossing an ordinary level at the same distance? Criteria, levels
and placebo were fixed in `docs/plans/20261015-hypotheses-from-llm-review.md` (H20) before the run. Verdict: not
tradable as a signal; the one statistically positive primary test is far below the cost hurdle and is produced by the
placebo, not by the real level.

Code: `newssent/cmd/levelstudy` (the `newssent` repo has no git, so the code is not versioned; the result is also in
`newssent/data/experiments.jsonl`, kind `level_study_H20`). Data: 10-minute ISS bars for the 16 `BasketTickers`,
2024-11-15..2026-10-11, main session only, forward window inside the same day, gaps through a level excluded,
levels from closes (the cache has Open and Close only). 12213 events: 5784 real, 6429 placebo (extreme 1368 / 1989,
round 4416 / 4440 at the 1h horizon).

Outcome: signed abnormal return against the basket (beta over the full sample) divided by the hour-of-day robust sigma;
positive = continuation, negative = reversal. Difference = real level minus placebo level, 97.5% day-bootstrap CI
(2 primary tests), 2000 draws.

| level | placebo | horizon | real signed z | placebo signed z | diff (CI97.5) | real mean bps |
|---|---|---|---:|---:|---|---:|
| 20-day extreme | 4th extreme | 10m | +0.164 | -0.026 | +0.190 [-0.014, +0.410] | +1.8 |
| 20-day extreme | 4th extreme | **1h (primary)** | +0.048 | -0.125 | **+0.173 [+0.034, +0.317]** | +1.3 |
| 20-day extreme | 4th extreme | 4h | +0.024 | -0.010 | +0.034 [-0.103, +0.163] | +1.2 |
| round number | half-step grid | 10m | -0.019 | +0.109 | -0.127 [-0.281, +0.008] | -0.4 |
| round number | half-step grid | **1h (primary)** | -0.063 | -0.054 | **-0.009 [-0.105, +0.080]** | -1.4 |
| round number | half-step grid | 4h | -0.047 | -0.040 | -0.008 [-0.086, +0.068] | -1.8 |

- Primary (extreme, 1h): the CI excludes zero, both halves of the period have the same sign (first +0.179 [-0.024,
  +0.390], second +0.167 [-0.037, +0.357]), 1368 real events on 424 dates, so the statistical criterion is met.
- Economic criterion (mean signed return above 30 bp round trip): not met. The real extreme earns +1.3 bp at 1h; the
  difference comes from the placebo level reversing (-2.8 bp), which is ordinary range behaviour, not a stop run.
- Primary (round number, 1h): no effect (-0.009, CI spans zero), 4416 real events on 586 dates.
- Share of crossings that return through the level within 30 minutes: extreme 0.471 real vs 0.473 placebo (CI95 of the
  difference [-0.038, +0.034]); round 0.490 vs 0.473 ([-0.003, +0.039]). No stop-run signature.
- By direction, extremes broken downward at 1h: diff +0.187 [+0.004, +0.388]; upward +0.165 [-0.020, +0.344]. Descriptive
  only (not primary tests).
- Limits: no order book or order history, only a price trace; liquid basket only (mid caps could show more, but the
  spread removes it); closes instead of High/Low; round step is 1-10% of price, finer rounding was not tested; gap
  crossings excluded; the 200-day moving average level was not tested (history too short).
- Use: do not build an entry signal from this. No evidence that placing our own stops or entries next to round numbers
  or 20-day extremes costs anything on this basket, so the planned protective filter is not justified by this data.

### H20 extension: fine rounding, 200-day average, illiquid names, news reversal (2026-10-15)

Family of six primary tests fixed in the plan before the run, 99.2% day-bootstrap CI (Bonferroni over six), same event,
outcome and placebo definitions as above, signed z at 1h, difference = real minus placebo. Result: no test rejects the
null and none reaches the economic hurdle (30 bp, 100 bp for illiquid names). Code `newssent/cmd/levelstudy`
(`-universe`, `-sets`, `-maxgap`) and `newssent/cmd/newsreversal`, result also in `newssent/data/experiments.jsonl`
(kind `level_study_H20_extension`). The first run of `levelstudy` after the refactor reproduces the earlier numbers
exactly (12213 events, extreme 1h diff +0.173).

| universe | level (real vs placebo) | n real / placebo | diff (CI99.2) | real mean bp | dates real |
|---|---|---|---|---:|---:|
| basket 16 | fine round (0.1-1% step) vs half-step grid | 36277 / 36235 | -0.005 [-0.023, +0.012] | -0.5 | 595 |
| basket 16 | SMA200 vs SMA173 | 326 / 336 | +0.062 [-0.260, +0.379] | -2.6 | 231 |
| illiquid 38 | 20-day extreme vs 4th extreme | 2129 / 3451 | +0.101 [-0.080, +0.297] | -13.1 | 504 |
| illiquid 38 | round 1-10% step vs half-step grid | 10184 / 10215 | -0.027 [-0.099, +0.046] | -17.2 | 591 |
| illiquid 38 | fine round vs half-step grid | 72434 / 71283 | +0.011 [-0.005, +0.026] | -13.0 | 595 |
| news, 1203 events | extreme crossed in the first hour vs not, 1h->4h continuation | 93 crossed / 1110 | +0.100 [-0.816, +1.010] | +0.8 | 84 |

- Descriptive: SMA50 vs SMA37 at 1h: -0.183 [-0.441, +0.039], n 555 / 732. SMA200 halves have opposite signs (first
  +0.177, second -0.055); the sample is small (231 dates), the test is underpowered, but any effect inside its CI is at
  most a few bp, far below the cost hurdle.
- Illiquid universe: 38 of 115 eligible shares outside the basket (>= 300 trading days), top tercile by share of zero-return
  10-minute bars (0.198..0.350); crossings require consecutive bars within 20 minutes. After a crossing both arms
  reverse by about -13 to -17 bp at 1h (signed z -0.27..-0.33). It is the same for real and placebo levels, so it is not
  level specific; it is far below 100 bp and is most likely last-trade price noise (bid-ask bounce) in illiquid names,
  which a spread would eat. The share of crossings that return through the level within 30 minutes is 0.510 vs 0.521
  at fine round numbers (diff -0.011, CI95 [-0.014, -0.008]), and 0.493 vs 0.520 at extremes (diff -0.028, CI95
  [-0.053, -0.003]): statistically visible, practically negligible.
- News reversal: first news of a ticker within 24h, 2025-01-01 onward, `|z1h| >= 2`: 5378 kept, 1223 significant, 1203
  usable. The criterion needed >= 100 crossed events; there are 93, so the test is not conclusive (CI is wide). Without
  conditioning, continuation after the first hour is -0.053 z (-1.8 bp, n 1203): essentially zero, no systematic
  reversal at the 1h->4h scale. Secondary: crossing a round number 1h->4h +0.097 [-0.371, +0.560] (410 crossed); extreme
  crossed, 1h->24h +0.189 [-0.476, +0.858].
- Not tested: order book and order history. `MOEX_TRADER_ALGOPACK_TOKEN` is not set, public ISS has only a current
  snapshot, the broker book is a live stream without history. Needs an AlgoPack subscription (`obstats`, `orderstats`)
  or our own snapshot collector.

## H22: news burst as a risk regime for model decisions (2026-10-17)

Pre-registered in `docs/plans/20261015-hypotheses-from-llm-review.md` (commit d42ef3e) before the run. Tool: `newssent/cmd/newsburst` (not under git). Decisions: BUY and SELL from the six `wf-volscale` walk-forward windows since 2026-04-01 (the first month with dense `data/news_history.jsonl` coverage), 1492 decisions, all with price data and a 10-trading-day forward. Burst: at least 3 articles on the ticker in the 24 hours ending 10:00 MSK of the decision date. Outcome 1: signed return close-to-close over 10 trading days. Outcome 2: worst signed daily close within those 10 days (a lower bound of the adverse excursion, no intraday High/Low). Bootstrap by ISO week, 2000 draws, Bonferroni for two primary tests (CI 97.5%).

| test | burst n (dates) | burst mean | rest n | rest mean | difference | CI 97.5% |
|---|---|---:|---:|---:|---:|---|
| outcome 1, signed 10d return | 304 (113) | +0.34% | 1188 | +0.75% | -0.41% | [-2.20%, +1.33%] |
| outcome 2, worst close in 10d | 304 (113) | -4.15% | 1188 | -3.78% | -0.37% | [-1.71%, +0.92%] |

Both intervals contain zero, so the pre-registered criterion is not met (the sample requirement of 200 decisions and 40 dates is met). Descriptive splits (CI 95%, not part of the criterion): thresholds 1, 2 and 5 articles give outcome-1 differences -0.62%, -0.30%, -0.08% and outcome-2 differences -0.46%, -0.29%, -0.20%, every interval contains zero; BUY only (191 burst decisions) -0.05% and -0.71%; SELL only (113 burst decisions) -1.23% and +0.05%, intervals contain zero. The point estimates lean toward a worse outcome after a burst but are far inside the noise; no news-based filter is justified by this data. Window: 5.5 months, one market regime; news carry no sentiment sign, only a count.

## Data depth, step 1: training-history length (2026-10-18)

Pre-registered in `docs/plans/20261018-data-depth-and-open-problems.md` (commits ae62a19, 878c3ae) before the run. Data: `exportdataset -from 2015-01-01 -till 2026-10-07 -horizon-days {10,20} -deadband-pct 0.5 -label-mode absolute`, 17 trading tickers, FX excluded, no news or macro, 15 price columns, 54017 rows per horizon. Protocol: `evalh.py` `wf()` unchanged (expanding purged walk-forward, 9 folds of 63 days from 2025-04-01, LightGBM + logistic average). Test rows are identical across arms (8064 at h10, 8177 at h20); only the training start differs. Run on the ai-box (`~/moex-research`), scripts `depth_eval.py`, `depth_boot.py`.

| h | arm | train rows | pooled AUC | mean fold AUC | folds > 0.5 | Brier | ECE |
|---|---|---:|---:|---:|---:|---:|---:|
| 10 | A from 2024 | 4762 | 0.5005 | 0.5371 | 6/9 | 0.2564 | 0.0792 |
| 10 | B from 2021 | 15490 | 0.4958 | 0.5102 | 6/9 | 0.2503 | 0.0410 |
| 10 | C from 2018 | 24745 | 0.4921 | 0.5076 | 5/9 | 0.2519 | 0.0654 |
| 10 | D from 2015 | 33548 | 0.5199 | 0.5287 | 6/9 | 0.2511 | 0.0734 |
| 10 | E from 2015 excl. 2022-02-24..12-31 | 30542 | 0.5331 | 0.5378 | 6/9 | 0.2511 | 0.0809 |
| 20 | A from 2024 | 4740 | 0.4696 | 0.4963 | 5/9 | 0.2645 | 0.1211 |
| 20 | B from 2021 | 15752 | 0.4558 | 0.4754 | 5/9 | 0.2553 | 0.0759 |
| 20 | C from 2018 | 25268 | 0.4539 | 0.4901 | 5/9 | 0.2561 | 0.0981 |
| 20 | D from 2015 | 34331 | 0.4883 | 0.5111 | 5/9 | 0.2551 | 0.0919 |
| 20 | E from 2015 excl. 2022-02-24..12-31 | 31284 | 0.4985 | 0.5025 | 5/9 | 0.2559 | 0.0977 |

D minus A, pooled AUC, block bootstrap over trading dates (2000 repeats, seed 42): h10 +0.0194, CI95 [-0.0062, +0.0457], D beats A in 3 of 9 folds; h20 +0.0188, CI95 [-0.0037, +0.0420], 5 of 9 folds.

Verdict against the pre-registered criteria:
- Primary (D beats A by >= +0.010 pooled AUC on both horizons and more than half of folds > 0.5): met on the point estimates (+0.0194, +0.0188; 6/9 and 5/9 folds). It is not a swap decision: both intervals contain zero, and the pooled gain at h10 comes with a lower mean fold AUC (0.5287 vs 0.5371) and 3 of 9 fold wins.
- Secondary (weak positive effect: lower CI bound > -0.002 and D beats A in >= 7 of 9 folds, per horizon): not met on either horizon (lower bounds -0.0062 and -0.0037; fold wins 3 and 5).
- Monotonicity A, B, C, D is absent (B and C are below A in pooled AUC at both horizons); the gain appears only at 2015, which looks like the effect of more regimes, not of steady accumulation.
- Absolute level stays at noise: best pooled AUC 0.5199 at h10 (arm D) and 0.4883 at h20, below 0.5. Deeper history does not make the 15 price features predictive on its own.
- Calibration improves with depth: Brier falls 0.2564 -> 0.2511 (h10) and 0.2645 -> 0.2551 (h20), ECE 0.0792 -> 0.0734 and 0.1211 -> 0.0919 (arm B has the lowest ECE at both horizons). This matters for step 3, not as an alpha claim.
- Arm E (without 2022) vs D: +0.0132 (h10) and +0.0102 (h20) pooled AUC, descriptive only; 2022 may be a regime that hurts a stationary model.

Consequence: no model swap and no change to `ensemble_model.json`. Depth is recorded as "no reliable AUC effect, some calibration effect"; steps 3-5 may use the deep history for calibration, but the window question for the live model is open until a purged walk-forward through `cmd/backtest` with costs.

## Data depth, step 2: news classifier trained on the 2021-2024 archive (2026-10-18)

Pre-registered in `docs/plans/20261018-data-depth-and-open-problems.md` (commit 5705414) before the run. Training: `Kasymkhan/RussianFinancialNews` through `cmd/newsimport` (alias match on tags + title, 18 trading tickers), records dated before 2025-01-01: 18957 pairs, 18375 labeled samples (the previous archive gave 2855). Validation: `data/news_history.jsonl`, 17 config tickers, records from 2026-04-01 (7129 pairs). `cmd/trainnewsmodel` unchanged (bag-of-words logistic, vocab 3000, min doc freq 3, label = excess return vs IMOEX > 0, entry at the next trading day's open), split 2026-04-01. Per-sample scores from the new read-only `cmd/newsvalidate`; CI is a date-block bootstrap (2000 repeats, seed 42).

| horizon (trading days) | train samples | val samples | val dates | train AUC | val AUC | CI95 | val accuracy | majority baseline |
|---:|---:|---:|---:|---:|---:|---|---:|---:|
| 10 (primary) | 18375 | 2866 | 110 | 0.689 | 0.528 | [0.479, 0.574] | 0.540 | 0.547 |
| 5 | 18375 | 4019 | 105 | 0.699 | 0.486 | [0.425, 0.538] | 0.519 | 0.543 |
| 1 | 18375 | 4488 | 135 | 0.682 | 0.516 | [0.448, 0.579] | 0.548 | 0.571 |

Slices at the primary horizon: articles mapped to at most 3 tickers, n 2781, AUC 0.533, CI95 [0.483, 0.579]; sources other than Telegram and Google News, n 157 (25 dates), AUC 0.432, CI95 [0.340, 0.533] (too few to read; at h5 and h1 the same slice is 0.558 and 0.563 with CIs from 0.36 to 0.72).

Verdict against the pre-registered criterion (val AUC >= 0.530 at 10 days with the lower CI bound above 0.500, at least 1000 samples and 40 dates): NOT met. AUC 0.528, lower bound 0.479; the sample requirement is met. Six times more training data (18375 vs 2855) moved validation AUC from 0.49-0.53 to 0.49-0.53: the train-validation gap (0.69 vs 0.53) is overfitting to vocabulary, not a data-volume problem. Recorded as "the Russian 2021-2024 archive does not give a directional headline signal on 2026"; the news feature is not added to the price model on this step, and the second stage of step 2 is not run.

Post-hoc observation (not part of the criterion, not a result): at h10 the lowest score quintile has mean excess return -1.04% and positive share 0.406, while the other four quintiles sit at -0.09..-0.22% and 0.452-0.485. A one-sided negative-news effect would match the literature note in `docs/plans/20261015-hypotheses-from-llm-review.md` (H4); it needs its own pre-registration (short-side only, tail of the score) before any use.

## Data depth, step 3: calibration across horizons and regimes (2026-10-18)

Pre-registered in `docs/plans/20261018-data-depth-and-open-problems.md` (commit 68e79fa) before the run. Same datasets and folds as step 1 (9 purged folds of 63 days from 2025-04-01, 15 price columns, LightGBM + logistic average), horizons 5, 10, 20; training windows A (from 2024) and D (from 2015). Per fold the model is fit on the first 80% of training rows by date and the calibrator (Platt on the logit, or isotonic) on the last 20%, both strictly before the fold start. Run on the ai-box, script `calib_eval.py`. Pooled over the 9 folds.

| h | window | method | ECE | Brier | AUC | folds with lower ECE than raw |
|---:|---|---|---:|---:|---:|---:|
| 5 | A | raw | 0.0439 | 0.2492 | 0.5512 | - |
| 5 | A | Platt | 0.0866 | 0.2601 | 0.4392 | 2/9 |
| 5 | A | isotonic | 0.0932 | 0.2615 | 0.4455 | 2/9 |
| 5 | D | raw | 0.0649 | 0.2518 | 0.5271 | - |
| 5 | D | Platt | 0.0194 | 0.2498 | 0.4542 | 4/9 |
| 5 | D | isotonic | 0.0207 | 0.2526 | 0.4543 | 4/9 |
| 10 | A | raw | 0.0514 | 0.2473 | 0.5549 | - |
| 10 | A | Platt | 0.1488 | 0.2702 | 0.3988 | 3/9 |
| 10 | A | isotonic | 0.1628 | 0.2725 | 0.4205 | 2/9 |
| 10 | D | raw | 0.0896 | 0.2518 | 0.5465 | - |
| 10 | D | Platt | 0.0416 | 0.2503 | 0.4130 | 5/9 |
| 10 | D | isotonic | 0.0430 | 0.2507 | 0.4202 | 5/9 |
| 20 | A | raw | 0.0701 | 0.2489 | 0.5512 | - |
| 20 | A | Platt | 0.2348 | 0.3039 | 0.3578 | 2/9 |
| 20 | A | isotonic | 0.2320 | 0.3058 | 0.3544 | 2/9 |
| 20 | D | raw | 0.1069 | 0.2563 | 0.5190 | - |
| 20 | D | Platt | 0.0415 | 0.2562 | 0.3250 | 4/9 |
| 20 | D | isotonic | 0.0545 | 0.2545 | 0.3238 | 5/9 |

Verdict against the pre-registered criterion (best calibrator lowers ECE by at least 20% with Brier not worse and pooled AUC within +-0.005, for all of h5, h10, h20 and both windows): NOT met on any of the six combinations.
- Window A: both calibrators make ECE and Brier worse at every horizon (ECE x2 to x3.3).
- Window D: ECE falls by 54-70% and Brier is level or slightly better, but AUC drops by 0.07-0.19. A calibrator that moves AUC from 0.52-0.55 to 0.32-0.45 has inverted the ranking: on the most recent 20% of training dates the base model's score is anti-correlated with the outcome, so the fitted monotone map takes a negative slope. This is a property of the calibration sample (a regime reversal between the last training months and the following test fold), not a coding artifact; the same thing explains why the folds-better count is 2-5 of 9.
- Regimes: the high/low IMOEX volatility split works and is reported in `calib_result.txt`; the key-rate split is degenerate, the rate did not rise before any of the 9 test folds, so the "rate up" bucket is empty.
- In this research ensemble the raw probabilities sit close to 0.5 (raw ECE 0.044-0.107), unlike the deployed ensemble whose tails reach 0.07 and 0.93 (Experiment C, ECE 0.21); the miscalibration that matters for the 0.60/0.40 thresholds is not reproduced here and needs the deployed artifact.
- Sensitivity worth recording: the raw AUC of arm A in this table (0.551-0.555) is far above the same arm in step 1 (0.470-0.501), the only difference being that the last 20% of training rows are held out for the calibrator. A swing of up to 0.08 AUC from dropping the latest training months means every AUC difference of 0.01-0.02 in steps 1-3 is within recipe noise, and arm D is below A here (h10 0.5465 vs 0.5549, h20 0.5190 vs 0.5512). The step 1 "primary criterion met" is therefore not robust.

Consequence: no calibrator is wired into the trader. Calibration stays a diagnostic until it is rerun on the deployed artifact with thresholds recomputed.

## Explainability of signals: feature drivers (2026-10-18)

Pre-registered in `docs/plans/20261018-data-depth-and-open-problems.md` (commit 7d3d42b) before the implementation. Method: occlusion attribution, each trained feature replaced by its training mean (`logistic.mean` in `ensemble_model.json`), driver = change in ensemble probability; the top 3 by absolute change are appended to `Reasoning` as ` drivers=<feature>:<signed percentage points>`. It is not a Shapley decomposition (interactions are not split; the tree artifact has no node covers, so exact TreeSHAP is not available). `news_sentiment` and `news_count` are masked as untrained and never appear.

- Criterion 1 (unit tests, `internal/model/explain_test.go`): for the logistic member the occlusion logit delta equals the exact linear contribution `coef*(x-mean)/std` to 1e-9; the baseline vector yields no drivers for any member; output is deterministic and ordered by absolute change; untrained features are skipped. Met.
- Criteria 2 and 3 (`cmd/explaincheck`, deployed `ensemble_model.json`, `h10.csv` rows from 2025-04-01, 17 tickers, 10195 rows, seed 42, 20 random draws per row): replacing the 3 top drivers moves the probability by 0.0822 on average against 0.0236 for 3 random trained features, ratio 3.48 (needed 1.5); in the decision zone (p <= 0.40 or p >= 0.60, 3964 rows) the sign of the top driver matches the side of 0.5 in 95.9% of rows (needed 70%). Met.
- Compatibility: `tailprecision.probabilityFromReasoning` stops at the first space, so ` drivers=...` after `p=` does not change parsing (regression case added); the Telegram decision text strips only the `ensemble:` prefix, so the drivers reach alerts and the audit log without a separate code path.
- Scope of the claim: this measures how the model's own output depends on its inputs, not whether the features cause returns. Dataset rows overlap the deployed model's training period, which is irrelevant for a faithfulness check but means no outcome claim is made.

## H23 stage 0: VAR-ICA-MGARCHSK portfolio formation, signal-free (2026-10-18)

Pre-registered in `docs/plans/20261018-data-depth-and-open-problems.md` (commits 4010c4c, b7b27ad) before the run. Method after Mikhaylov, Yousif, Sotskov, Khomyakova, Finance: Theory and Practice 2026, 30(4), 19-37 (read in full). Tool: `cmd/icaportfolio` on `internal/icaport` (9 unit tests: Jacobi eigendecomposition, inverse, FastICA source recovery, recursion fit, portfolio moments vs sample moments, CARA gradient vs finite differences, simplex optimizer, metrics, determinism). Universe: config equities with at least 740 of 750 days (16 tickers; YDEX and DATA skipped for short history), ISS daily closes (price return, dividends not included, identical for every rung), 750-day estimation window, rebalance every 21 trading days, 99 rebalances 2019-01-02..2026-10-07, long-only, 0.05% cost on turnover including the initial purchase, weights drift between rebalances. Deviations from the paper, fixed beforehand: least-squares fit of the moment recursions (the paper does not state its estimator), cumulant form of portfolio moments (`k4 = sum a^4 kappa4 + 3 sigma^4`), deterministic multi-start projected gradient instead of the genetic algorithm, moment forecast averaged over the 21-day holding period. 0 of 99 fits failed. Statistics: stationary block bootstrap (block 20, 2000 repeats, seed 42), paired daily returns.

| rung | ann. return | ann. vol | CVaR95 (daily) | max drawdown | Sharpe | turnover per rebalance |
|---|---:|---:|---:|---:|---:|---:|
| R0 equal weight (control) | 0.65% | 24.99% | 3.785% | 53.81% | 0.15 | 0.070 |
| R1 inverse volatility 63d | 0.55% | 24.19% | 3.675% | 53.24% | 0.15 | 0.125 |
| R2 minimum variance, sample covariance | -2.68% | 23.58% | 3.529% | 63.28% | 0.00 | 0.116 |
| R3 VAR-ICA-GARCH, variance only | 0.30% | 26.17% | 3.834% | 59.35% | 0.14 | 1.010 |
| R4 full VAR-ICA-MGARCHSK, rho 1 | -5.76% | 26.84% | 3.952% | 74.28% | -0.09 | 0.851 |
| R4 with rho 5 (descriptive) | -1.31% | 28.19% | 4.276% | 65.82% | 0.10 | 1.398 |
| R4 with mu 0 (descriptive) | -8.46% | 23.66% | 3.672% | 77.21% | -0.25 | 0.239 |
| R3 with mu 0 (post-hoc, not pre-registered) | -3.92% | 22.57% | 3.530% | 66.00% | -0.06 | 0.252 |

| pair (strategy / control) | CVaR95 ratio [CI95] | max drawdown ratio [CI95] | annual return difference |
|---|---|---|---:|
| R1 / R0 | 0.971 [0.961, 0.979] | 0.989 [0.940, 1.040] | -0.11pp |
| R2 / R0 | 0.932 [0.884, 0.984] | 1.176 [0.818, 1.374] | -3.34pp |
| R3 / R0 | 1.013 [0.955, 1.076] | 1.103 [0.744, 1.402] | -0.35pp |
| R4 / R0 | 1.044 [0.976, 1.121] | 1.380 [0.852, 1.739] | -6.41pp |
| R4 / R3 | 1.031 [0.983, 1.089] | 1.251 [0.919, 1.556] | -6.06pp |
| R4 / R2 | 1.120 [1.064, 1.180] | 1.174 [0.884, 1.520] | -3.07pp |
| R4 (rho 5) / R0 | 1.130 [1.060, 1.227] | 1.223 [0.789, 1.723] | -1.96pp |
| R4 (mu 0) / R0 | 0.970 [0.909, 1.039] | 1.435 [0.927, 1.736] | -9.12pp |
| R3 (mu 0, post-hoc) / R0 | 0.933 [0.884, 0.988] | 1.226 [0.834, 1.423] | -4.57pp |
| R3 (mu 0, post-hoc) / R2 | 1.000 [0.979, 1.023] | 1.043 [0.933, 1.163] | -1.23pp |

Verdict against the pre-registered criterion (R3 or R4 versus equal weight: CVaR95 at least 15% lower with the upper CI bound below 1.0, maximum drawdown at least 15% lower, annual return not more than 1pp below; R4 must also beat R3 on CVaR by 5% and beat R2): NOT met by R3 or by R4 on any of the three conditions. R3 CVaR ratio 1.013, R4 1.044; drawdown ratios 1.10 and 1.38 (worse); returns -0.35pp and -6.41pp. R4 does not beat R3 (CVaR 1.031) or R2 (1.120). H23 is rejected for this universe; the signal-free stage failing means no replay on the `wf-volscale` artifacts is run and nothing is wired into the trader.

Observations (descriptive):
- The paper's weights are unstable here as in its own figures 7-8: turnover per rebalance is about 100% for R3 and 85% for R4, against 7% for equal weight, and the cost is already included. With the VAR mean forecast switched off, turnover drops to 25% (R3) and 24% (R4); the noisy VAR(1) mean term in a CARA objective with percent-unit returns and rho 1 dominates the weights.
- Without the mean term the ICA-GARCH variance forecast is statistically indistinguishable from the plain sample-covariance minimum-variance portfolio (R3 mu0 versus R2: CVaR ratio 1.000, CI [0.979, 1.023]), so the independent-component volatility model adds nothing over the sample covariance on 16 names with 750 observations.
- Adding skewness and kurtosis makes the tail risk worse (R4 versus R3, CVaR 1.031, drawdown 1.25); the estimated conditional third and fourth moments are too noisy to optimise against at this sample size, which matches the instability visible in the paper's own figures 5-6.
- The only rungs with a CVaR CI entirely below 1 are R1 (0.971), R2 (0.932) and the post-hoc R3 without mean (0.933); all of them pay with a higher drawdown and lower return than equal weight over the period, so none passes the pre-registered criterion either.
- Limits: price returns without dividends (the same for every rung), a single path that includes 2020 and 2022 (the 2022 exchange halt is in the data for every rung), 16 names and one estimation window length; max drawdown of equal weight is 53.8%, so differences of 10-20% in drawdown ratio are inside the bootstrap intervals.

## Data depth, step 2b: news domain shift test (2026-10-18)

Pre-registered in `docs/plans/20261018-data-depth-and-open-problems.md` (commit ea43010) before the run. Data: `data/news_history.jsonl`, 2026-04-01 onward, 17 config tickers, 7257 pairs, 203 duplicates (same ticker and normalised title) removed, 7054 kept. Train: records before 2026-08-01 (3023 pairs); 14-day gap (322 pairs dropped); test: records from 2026-08-15 (3709 pairs), none of whose normalised titles occur in train. Arms: A trained on 2026 train only (`-vocab-size 1000`), B the step 2 classifier trained on 2021-2024 only (no 2026 data, control), C trained on 2021-2024 plus 2026 train. Score = headline score, label = excess return over IMOEX > 0 with entry at the next trading day's open. CI: date-block bootstrap (2000 repeats, seed 42) at the Bonferroni level 99.17% (6 checks: arms A and C on 3 horizons).

| horizon | arm | train samples | test samples | test dates | AUC | CI 99.17% | source-clean slice (n, AUC) |
|---:|---|---:|---:|---:|---:|---|---|
| 10 | A (2026 only) | 1187 | 1427 | 38 | 0.4396 | [0.371, 0.512] | 155, 0.460 |
| 10 | B (2021-2024 only, control) | 18375 | 1427 | 38 | 0.6132 | [0.534, 0.678] | 155, 0.424 |
| 10 | C (2021-2024 + 2026) | 19562 | 1427 | 38 | 0.5823 | [0.514, 0.647] | 155, 0.424 |
| 5 | A | 1603 | 2168 | 39 | 0.4703 | [0.388, 0.570] | 313, 0.482 |
| 5 | B | 18375 | 2168 | 39 | 0.4797 | [0.355, 0.590] | 313, 0.552 |
| 5 | C | 19978 | 2168 | 39 | 0.4693 | [0.352, 0.605] | 313, 0.525 |
| 1 | A | 1328 | 2817 | 49 | 0.5438 | [0.461, 0.616] | 599, 0.572 |
| 1 | B | 18375 | 2817 | 49 | 0.5338 | [0.410, 0.646] | 599, 0.557 |
| 1 | C | 19703 | 2817 | 49 | 0.5424 | [0.421, 0.663] | 599, 0.567 |

Verdict against the pre-registered criterion (arm A or C: AUC >= 0.530 and the lower corrected CI bound above 0.500, with at least 1000 test samples and 40 test dates): no arm qualifies. Arm C at 10 days has AUC 0.5823 and lower bound 0.514, but the test has 38 dates against the required 40, so the condition fails and there is no verdict for it; arms A and C at 5 and 1 days fail on the CI bound. Domain-shift reading under the pre-registered rule: not supported. Training on 2026 Telegram and Google text does not help (A is 0.44 at 10 days, below chance, train AUC 0.87), and the control B, which never saw 2026, scores as high as or higher than C (0.613 vs 0.582).

Descriptive, not pre-registered: the step 2 classifier (B) scored month by month at 10 days gives AUC 0.459 (April), 0.407 (May), 0.579 (June), 0.373 (July), 0.502 (August), 0.614 (September), 0.753 (October); at 5 and 1 days every month stays near 0.4-0.57. The 10-day figure on the August-October test is therefore a window effect (September-October only), the same time instability seen in every other test of this series, not evidence of a stable headline signal. The news classifier stays switched off and direction 2 (news as a directional signal) is closed unless a source-diverse archive several times larger accumulates; the data-depth step 4 and the news direction share the same limit, signal non-stationarity.

## Data depth, step 2c: news classifier on the expanded 260-ticker archive (2026-10-18)

Pre-registered in `docs/plans/20261018-data-depth-and-open-problems.md` (commit fb8f160) before the run. Archive: `data/news_history.jsonl` after the Telegram backfill and cleaning (153151 pairs, 260 tickers; `docs/knowledge/news-and-events.md`). Filters, as registered: source "MOEX сайт-новости" excluded; articles mapped to four or more tickers excluded (`-max-tickers-per-article 3`, and the same rule applied to the test file, which removed 27938 of 72132 test pairs: `newsvalidate` has no such flag, caught and fixed before the first scoring); train = pairs before 2025-01-01 (63581 pairs; 40413 labeled samples after the liquidity filter), test = pairs from 2025-03-01 (31 pairs with a headline seen in train removed), median 20-day turnover at least 10 million RUB on the publication day (`-min-turnover`, new in `BuildNewsLabelsWithOptions`). `cmd/trainnewsmodel` otherwise unchanged (bag-of-words logistic, vocab 3000, min doc freq 3), excess-return label against IMOEX with entry at the next day's open. CI: date-block bootstrap (2000 repeats, seed 42), Bonferroni level 98.33% for three horizons. Scores from `cmd/newsvalidate`, analysis `analyze.py` in the session scratch.

| horizon | test samples | test dates | AUC | CI 98.33% | quarters with AUC >= 0.5 (of 6) | within-date permutation AUC |
|---:|---:|---:|---:|---|---:|---:|
| 10 | 19085 | 417 | 0.5234 | [0.5022, 0.5424] | 4 | 0.5021 |
| 5 | 23095 | 381 | 0.5212 | [0.5007, 0.5421] | 5 | 0.5016 |
| 1 | 23766 | 471 | 0.4963 | [0.4735, 0.5196] | 4 | 0.4917 |

Quarterly AUC at 10 days: 2025Q2 0.564 (3118 samples), 2025Q3 0.520, 2025Q4 0.477, 2026Q1 0.534, 2026Q2 0.592, 2026Q3 0.488. At 5 days: 0.519, 0.529, 0.491, 0.524, 0.573, 0.506. By source group at 10 days: Telegram 0.5238 (17153), Google 0.5442 (1796), finance sites 0.295 (47), RSS and agencies 0.220 (89); the last two are too small to read. Top 30 tickers by news volume: 0.526 (10 days), 0.524 (5 days), 0.489 (1 day).

Verdict against the pre-registered criterion (AUC >= 0.530 and the lower CI bound above 0.500, at least 1000 samples and 40 dates, AUC >= 0.5 in at least 4 of 6 quarters, permutation control inside [0.48, 0.52]): NOT met at any horizon. The control and the sample requirements pass everywhere; the AUC bar fails (0.5234, 0.5212, 0.4963), and at 1 day there is no effect. 

Descriptive, and the reason this is not the same null as steps 2 and 2b: with 19-24 thousand samples over 380-470 dates the 10-day and 5-day intervals lie entirely above 0.5, which the earlier 1.4-2.8 thousand sample tests could not show. The effect is small (about +0.02 AUC), unstable across quarters (0.48-0.59 at 10 days), carried almost entirely by Telegram posts, and measured on pairs that are not independent (several channels repost the same wire; the bootstrap blocks by date but not across neighbouring days), so the interval is optimistic. No economic claim follows: AUC 0.52 says nothing about returns net of costs. By the registered rule the news direction stays closed as a trading signal; reopening it needs its own pre-registration with a net-of-cost portfolio test (for example a score-ranked long-short of liquid names with the cost model of step 1), not an ad hoc threshold change.

## Data depth, step 2g: news score net of costs (2026-10-18)

`go run ./cmd/newsportfolio -scores scores_h10.csv` on the step 2c scores (classifier trained before 2025-03-01, horizon 10 trading days, turnover ≥ 10M RUB). Pre-registered in `docs/plans/20261018-data-depth-and-open-problems.md` (commit 8a925ef, before the run). Ticker-day score = mean of article scores; per date with ≥ 10 scored tickers (226 dates, 2025-03-17..2026-10-05) long the top quintile, short the bottom quintile, equal weights, 10-day hold, one tranche per date; cost 0.15% per fill (0.30% round trip per leg, 0.60% per tranche), no short borrow; date-block bootstrap, block 10, 5000 reps, seed 20261018.

| quantity | value |
|---|---|
| gross long-short spread per tranche | +0.863% |
| break-even cost per side | 0.216% |
| net at 0.15%/side (primary) | +0.263%, CI95 [-0.305, +0.930], pass = false |
| net at 0.10%/side | +0.463%, CI95 [-0.105, +1.130] |
| net at 0.20%/side | +0.063%, CI95 [-0.505, +0.730] |
| long-only top quintile, net of 0.30% round trip | +0.045%, CI95 [-0.258, +0.350] |
| quintile mean 10d excess, low to high score | -0.518, -0.385, -0.012, +0.206, +0.280 (%) |
| quarterly net (primary cost) | -0.23, +0.48, -0.55, -0.74, -0.23, +2.40, +0.48, -0.25 (%, 7-50 dates each) |

Verdict by the registered rule: criterion not met (lower CI bound below 0 at every cost level). The quintiles are monotone and the point estimate of the net spread is positive, but the interval is wide, 5 of 8 quarters are negative and one quarter (2026Q2, +2.40%) carries the mean. The long-only leg earns nothing after costs; the spread comes from the short leg (bottom quintile -0.52%), which is the part that is not tradable here (no borrow cost modelled, spot shorts not available on the sandbox book). The news branch stays closed as a trading signal; reopening needs an independent period after 2026-10-05.

## Ticker circuit breaker in the gate replay (2026-10-18)

Pre-registered in `docs/plans/20261018-ticker-breaker-parity.md` (commit 0144c6e, before the run). `backtest.Config.Breaker` and `cmd/backtest -breaker` feed the live `risk.TickerBreaker` (defaults: 3 consecutive losing round trips or cumulative loss 5% of target_notional 15000; a tripped ticker stays blocked) from every replay fill. Gate-shaped replays (17 tickers, 90 days, 1M deposit, costs 0.0005×3 + per-ticker spreads, borrow 0.00005/day, caps 90000/90000, deployed `ensemble_model.json`, no news), one run per cell, no tuning. The breaker is NOT wired into the preflight.

| window (90d till) | breaker | closed trades | realized net of borrow | unrealized MTM |
|---|---|---|---|---|
| 2026-10-18 | off | 117 | -2455.72 | -2066.01 |
| 2026-10-18 | on | 96 | -3356.64 | +6139.47 |
| 2026-07-20 | off | 90 | +34737.78 | +189.38 |
| 2026-07-20 | on | 85 | +30484.28 | +3086.18 |
| 2026-04-21 | off | 34 | -3842.92 | +4610.19 |
| 2026-04-21 | on | 22 | -1183.71 | +3291.07 |

Sum of realized net of borrow over the three windows: off +28439.15, on +25943.92.

Verdict by the registered rule: criterion not met on both parts (the gate window with the breaker is -3356.64, below the 1 RUB threshold; the three-window sum is lower with the breaker). The breaker does not rescue the gate. Side finding: `risk.Gate` rejects every order for a tripped ticker with `ticker_breaker_blocked`, including orders that would close the position, so a tripped ticker's open position is frozen. In the 10-18 window this moves roughly 8.2k RUB from realized to unrealized (MTM -2066 → +6139) and the realized figure, which is what the gate measures, gets worse. Whether blocking exits is intended for live is an open owner question; it is not changed here.

## Seven hypotheses, step 2: news score decay by horizon and liquidity/sign split (H1, H4; 2026-10-18)

Pre-registered in `docs/plans/20261018-seven-hypotheses.md` (commit 1c43147 and the step 2 commit that follows it, before the run). `go run ./cmd/newsdecay -classifier news_classifier_h10.json -news <archive without "MOEX сайт-новости">`. Classifier retrained with the step 2c flags (`-split 2025-01-01 -horizon-days 10 -vocab-size 3000 -min-doc-freq 3 -max-tickers-per-article 3 -min-turnover 10000000`; train_samples 40413 as in 2c, val AUC 0.518). Test pairs 2025-03-01..2026-09-30: 40270 after the filters (25 seen in train removed), 13175 ticker-days with 20-day median turnover >= 10M RUB, 132 tickers, 575 dates (435 with >= 10 scored tickers). Rank-weighted cross-sectional portfolio per date (weights = demeaned rank, absolute weights sum to 1), gross of costs, entry at the next trading-day open after the MSK publication date, exit at the close h trading days later, excess over IMOEX; date-block bootstrap (block 10, 5000 repeats, seed 20261018). Bars on days without an IMOEX bar are dropped (weekend sessions), so counts differ from step 2c.

| h (trading days) | dates | gross spread, % per tranche | CI95 | per day, % |
|---:|---:|---:|---|---:|
| 1 | 435 | +0.0944 | [+0.0210, +0.1706] | +0.0944 |
| 2 | 435 | +0.1131 | [+0.0123, +0.2141] | +0.0565 |
| 5 | 435 | +0.2116 | [+0.0700, +0.3603] | +0.0423 |
| 10 | 435 | +0.4258 | [+0.2117, +0.6627] | +0.0426 |
| 20 | 435 | +0.5401 | [+0.2352, +0.8645] | +0.0270 |

- Primary criterion (H1: gross spread at h = 5 > 0, lower CI bound > 0, >= 100 dates): MET (+0.2116, [+0.0700, +0.3603], 435 dates). It is a gross figure; step 2g already showed that the 0.60% per tranche cost level removes it at h = 10 (gross 0.86% there on the older set), here h = 10 gross is +0.4258%, below 0.60%.
- Decay: the per-day spread at h = 1 is 2.2 times the h = 10 one (+0.0944 vs +0.0426), which matches the registered "more than twice" descriptive wording, but the difference (+0.0518 per day) has CI95 [-0.0270, +0.1308], so front-loading is not established. The spread keeps growing with the horizon (0.09 -> 0.43 -> 0.54), so the effect does not vanish by day 5 or 10.
- H4 liquidity: terciles by ticker median turnover, h = 10. Least liquid tercile +1.2117 [-0.2371, +3.1680] on only 33 dates (the filter leaves too few small names per date), middle +0.3495 [+0.0017, +0.6955] (339 dates), most liquid +0.3774 [+0.1508, +0.6342] (499 dates). Low minus high +0.8343 CI95 [-0.6336, +2.7335]: not significant, H4 liquidity part not supported.
- H4 sign (h = 10): bottom score quintile mean excess -0.9441 [-1.5271, -0.4061], top quintile +0.2120 [-0.1156, +0.5333]; asymmetry |bottom| minus top +0.7321 CI95 [+0.0924, +1.3906]: significant, the negative-news side carries the effect, as in step 2g (short leg). Spot shorts are not available on the sandbook, so the long side is the tradable one and it is not distinguishable from zero.

Verdict: H1 gross criterion met, H4 sign part supported, H4 liquidity part not supported. No trading conclusion: net of costs the step 2g result stands, and the tradable (long) leg is not significant.

## Seven hypotheses, step 5: variance of the evaluation procedure (2026-10-18)

Pre-registered in `docs/plans/20261018-seven-hypotheses.md` (commit "step 5 pre-registration", before the run). Harness: `scripts/procvar.py` (copy of `evalh.py` `wf()`/`fit_predict()` logic with the seed, first-test date, fold length and training start as parameters), run on the ai-box over `~/moex-research/depth/h10.csv` (17 tickers, 15 price columns, horizon 10, 8064 test rows from 2025-04-01). 75 configurations of recipe A (5 seeds x 3 first-test shifts {0, 27, 54 days} x 5 training starts), 15 of recipe D (training from 2015-01-01, 5 seeds x 3 shifts), 3 fold lengths.

| quantity | value |
|---|---|
| recipe A pooled AUC over the 75 configurations: mean / SD / min / max | 0.4829 / 0.0108 / 0.4605 / 0.5009 |
| SD over seed (others fixed) | 0.0019 |
| SD over training start (others fixed) | 0.0059 |
| SD over first-test shift (others fixed) | 0.0116 |
| N95: 95th percentile of the absolute pooled-AUC difference between A configurations with the same shift (900 pairs; median 0.0055) | 0.0160 |
| recipe A (start 2024-01-01) pooled AUC by shift 0 / 27 / 54 | 0.4959 / 0.4765 / 0.4901 |
| recipe D pooled AUC by shift 0 / 27 / 54 | 0.5199 / 0.4917 / 0.5117 |
| D minus A, 15 paired configurations: mean / SD / min / max / positive | +0.0203 / 0.0052 / +0.0112 / +0.0272 / 15 of 15 |
| fold length 42 / 63 / 84 days (seed 42, shift 0, same 8064 rows): pooled AUC | 0.4933 / 0.5005 / 0.5140 |
| SD of mean-fold AUC over the 75 A configurations | 0.0145 |

Interpretation under the registered rule: N95 = 0.0160 > 0.010, so every earlier comparison with |ΔAUC| below 0.016 that was judged on a single configuration (depth steps 1-3, experiments A-E of 2026-10-04 by AUC, macro and dividend features) is inconclusive, and new comparisons must be paired over at least 15 configurations. The dominant source is where the fold boundaries fall (SD 0.0116), not the seed (0.0019).

What survives: a paired depth comparison. D beats A in all 15 paired configurations (+0.0112 .. +0.0272, mean +0.0203; per shift +0.024, +0.015, +0.022), so the +0.019 of depth step 1 is not a one-configuration accident. It is still a small gain from a level at the noise floor: recipe A's pooled AUC averages 0.483, below 0.5, and D reaches 0.492-0.520. Absolute predictability on the 2025-04..2026-10 test rows is not demonstrated for either recipe.

Side observations, single seed each and not registered as criteria: lengthening the refit interval from 42 to 84 days raised pooled AUC from 0.493 to 0.514 on identical test rows; the pooled AUC below 0.5 for recipe A agrees with the autumn losses and is the subject of the planned post-mortem of September-October 2026.
