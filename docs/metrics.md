# Metrics

Single source of truth for backtest and analysis numbers. Formal structure
(columns: window | realized vs MTM | artifact (commit) | costs | date) is the
Task 17 deliverable; until then, results are appended here as they are produced.

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

## P&L results (pending Task 17 migration)

Historical headline numbers to be re-verified as realized-vs-MTM and migrated
into the formal table by Task 17: +110742, +79453, +75028, +63212,
+56379/+40102, +25699.

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
