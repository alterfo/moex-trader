# Metrics



Empirical results of the MOEX Trader research programme, written as a single paper-style record: setting, method, results by topic, synthesis. It is the single source of truth for every backtest and analysis number in the repository (rule 8 in `AGENTS.md`: a change that adds or edits a number lands in the same commit as the code or plan it concerns). Status: 2026-10-19. The previous, chronological version (about 2500 lines, one section per task) is preserved in git history at commit `82a124b` (`git show 82a124b:docs/metrics.md`); Appendix A maps its section names to the sections below, so older citations stay resolvable.

## Abstract

1. **The deployed price/flow ensemble has no demonstrable predictive edge.** Validation AUC sits at 0.46-0.55 depending on the recipe and evaluation window; the noise of the evaluation procedure itself is 0.016 AUC at the 95th percentile (section 3.6). Tail decisions (p >= 0.60 or p <= 0.40) hit 42.4% against a 47.7% base rate (bootstrap p = 0.9123, section 3.1).
2. **Its realized P&L is mostly market beta.** On 2025-04..2026-09 the book earned +194293 RUB realized, of which the IMOEX component is about two thirds (short tilt in a falling index). Neutralising the beta with the real IMOEX future removes the alpha (t = -0.49 to +0.39, section 5.4); excess-to-IMOEX labels halve the beta loading but leave a residual alpha of t about 2.0, short of the registered bar (section 5.5).
3. **No variation of the recipe passes the pre-registered deployment gate** (dividend-adjusted labels, wide training universe, calibration and single members, volatility scaling, sector caps, excess labels, deeper history, volatility-unit labels, rank features, TimesFM, open interest, wide-panel breadth; sections 4 and 6). The closest, the logistic single member, fails the drawdown criterion.
4. **News and event information adds little.** Headline-classifier AUC reaches 0.52 only with 19-24 thousand samples; net of costs the top-minus-bottom quintile spread is +0.263% per tranche with a confidence interval spanning zero (section 7). Zero-shot LLM direction on Russian headlines is not better than random (AUC 0.511-0.526). Event studies of sanctions, negotiations, level crossings and news bursts find no tradable effect.
5. **Calendar effects, cycles and intraday patterns are absent after 1990 / after costs** (section 8.11). The overnight premium (+0.07-0.10% per day) is real but smaller than the 0.30% needed to trade it.
6. **The one positive result is cross-sectional.** A ridge model on a point-in-time panel of the whole TQBR board (monthly rebalancing, 2019-2026) gives a long book minus universe of +1.07% per period (Newey-West t = 3.35, Sharpe 1.11), passing the registered rule; its long-short form misses on t (2.54) and needs a short-borrow cost below about 5% per year, which was never measured (section 8.6). As a live gate candidate it passes the rolling gate in 59% of windows, not the registered 70% (section 8.8).
7. **A live-path defect, not the model, produced the September-October SELL drift**: until 2026-10-09 the running trader fed unmasked `news_count` and `news_sentiment` to a model trained on zeros (section 9.1).
8. **The risk-free hurdle is high**: the 14% key rate pays +12347 RUB per month on 1M RUB; every strategy window recorded here earned less (section 8.12).

## 0. Conventions

### 0.1 Quantities

- **Realized P&L** is gross P&L of closed trades minus commission (and minus borrow where stated); **MTM** is the mark-to-market of open positions. Only realized P&L drives go/no-go decisions; MTM is reported separately.
- **Costs** ("0.0005 x 3"): commission 0.05%, spread 0.05% and slippage 0.05% per fill, unless stated. **Borrow stress**: 0.00005 per day (0.005% per day, 1.8% per year) on short notional. Deposit 1M RUB, 15000 RUB target notional per position, thresholds BUY p >= 0.60 and SELL p <= 0.40, hold until the signal flips, drawdown kill switch on.
- **AUC** is the area under the ROC curve of the predicted probability against the realized direction label; 0.5 is chance. A single-configuration difference below 0.005 AUC is treated as noise (original band); the measured noise of the evaluation procedure is larger (section 3.6).
- **Sharpe** in `internal/backtest` is annualised from daily equity returns with sample standard deviation times sqrt(252); `internal/strategyvalidation.SharpeRatio` is a per-period, non-annualised sample Sharpe, deliberately matching the granularity of the series it deflates (PSR/DSR). The two are not comparable. Sortino uses the downside returns only; CAGR = (final/initial)^(365.25/days) - 1; Calmar = CAGR / |max drawdown|. Runs with fewer than 30 closed trades carry a warning.
- **Intervals** are date-block or ISO-week bootstrap confidence intervals (seed 42 or the seed stated; 2000-5000 repeats); multiple-test corrections (Bonferroni, Holm) are stated per test. `t` is Newey-West or date-clustered as stated.

### 0.2 Windows

The six purged walk-forward windows ("Task 5 windows") are q1 2025-03-05..06-05, q2 2025-06-06..09-05, q3 2025-09-06..12-03, q4 2025-12-04..2026-03-04, w2 2026-03-05..06-06 and w1 2026-06-07..09-16; window h is 2026-09-17..10-17 (hold-out) and window G is 2026-07-20..2026-10-18 (the 90-day preflight window of the gate).

### 0.3 Method of record

Each experiment follows a pre-registered plan (`docs/plans/`, committed before the first run) with numeric acceptance criteria. A verdict is stated against those criteria only; anything computed after seeing results is labelled descriptive and cannot support a decision. Comparisons that depend on a recipe with training randomness are made against a fresh same-run control, never against an older ledger number (stack drift: library versions changed between 2026-10-04 and 2026-10-18; the HEAD engine reproduces an archived artifact to about 93%).

### 0.4 Provenance of numbers

Results dated up to 2026-09-17 (ledger rows, sections 3.1-3.4, 5.1-5.2) were produced before the review fixes of that date: live and backtest features now use the full 300-bar lookback (434 calendar days) so EMA/SMMA indicators converge as in live, `realized_volatility` is bounded to that window, live features use closed daily bars only, and `backtest.LoadNewsOverrides` weights news by `trust_weight`. Those results are retained for traceability and must be re-derived before reuse as evidence; the purged baseline of 2026-10-04 (section 3.5) and everything after it use the corrected pipeline.

### 0.5 Process rule (incident E4)

Metric-bearing changes are committed immediately. A session reads metrics from `HEAD` or from an explicitly requested uncommitted file; an empty `git log -S` search does not prove absence while the working tree has uncommitted changes.

### 0.6 Structural stability and boundary optima

A conclusion drawn from one specification (one seed, one window split, one cost level, one universe definition) is a hard-model conclusion and counts as evidence only if it survives a declared family of small perturbations (see [knowledge/soft-models-and-optimization.md](knowledge/soft-models-and-optimization.md)). Results here are therefore read as stable-pass, stable-fail or unstable; an unstable result is "no finding". Records of instability in this file: the composite factor rule at t 3.67 on the survivor panel against 1.42 on the point-in-time panel (section 8.5), the volatility-unit labels of step 12 not confirmed in step 17 (section 6.7), the 0.08 AUC swing from holding out 20% of training rows (section 6.4), and the sign change of the ridge long-short book under higher borrow (section 8.6). A parameter tuned to a historical optimum sits at the edge of its stability region: the non-monotonic cliff of the exposure cap (section 5.7) is the example, and new studies report a chosen value with its grid neighbours and flag it as a boundary optimum when a neighbour loses more than half of the realized P&L or changes its sign.

## 1. Metrics ledger

Formal single source of truth for headline P&L numbers. Column order is pre-registered in the strategy-validation plan: `window | realized vs MTM | artifact (commit) | costs | date`. Realized P&L is gross minus commission and borrow; MTM never drives a go/no-go. Historical rows where the original run did not record a field are marked `not recorded` and must be re-verified before reuse. The format is linted by `internal/metricsdoc`.

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

The two `not recorded` rows (+75028, +63212) were reported in the two-session source debate but their window, cost and realized-vs-MTM provenance was not committed. They are kept so the numbers are not lost, and must be re-derived before they are cited as evidence.

## 2. Apparatus

**Universe and data.** Trading universe: the 17 tickers of `config.sandbox.yaml` (18 with DATA, which was dropped live on 2026-09-21 because it cannot be shorted). Prices: MOEX ISS daily candles (ISS also publishes weekend equity sessions, while IMOEX is not calculated on weekends, which matters for label timing, section 6.2). Training sets: `exportdataset` from 2024-01-01 unless stated; wide sets use the TQBR board (262 common and preferred shares) with a point-in-time liquidity filter (trailing median 20-day turnover of at least 10 million RUB). Research runs execute on the ai-box (`~/moex-research`), separate from the deploy directory.

**Engine.** `internal/backtest` is the live path without the network: features on day `d` come from candles `[:d]`, entry at `candles[d+1].Open`, mark-to-market at `candles[d].Close`, the same `risk.HardenedGate`, a single chronological portfolio loop (so the 3% drawdown kill switch and the 0.5% daily-loss limit see the aggregate equity curve; the kill switch blocks new entries and does not liquidate). Live features use closed daily bars only (`closedDailyCandles`, `closedBarReturnPct`); before this fix 12 of the 18 deployed features were zero in live because the lookback was 10 days.

**Preflight.** The live gate replays the last 90 days with spread and slippage costs, judges **realized** P&L, requires `preflight.min_closed_trades` before `preflight.min_net_pnl`, and logs a SHA-256 of the gate-relevant configuration.

**Recipe under test ("abs-10d").** Absolute 10-bar forward-return label with a 0.5% dead band, 18 price/flow columns plus news (28-name feature order), LightGBM + XGBoost + logistic regression averaged, thresholds 0.60/0.40. Embargo of 10 days removes training rows whose label date reaches into the test window (the purged control). Reporting tools: `Result.Markdown()`, an HTML tearsheet (`-tearsheet`), `cmd/walkforward` (rolling or anchored windows, aggregate equity chaining), per-window `period_returns.csv` with a SHA-256 for the PBO matrix.


## Index of sections

Sections 3-9 live in `docs/metrics/`; numbering is unchanged, so `metrics.md 8.15` means subsection 8.15 in the file below.

| subsection | file |
|---|---|
| 3.1 Tail-precision falsification (generation 1, 2026-09-17) | s3-baseline.md |
| 3.2 Momentum benchmark (generation 1) | s3-baseline.md |
| 3.3 Six-quarter grid on the portfolio engine (generation 1) | s3-baseline.md |
| 3.4 Preflight replay after hardening (generation 1) | s3-baseline.md |
| 3.5 Purged walk-forward baseline (2026-10-04) | s3-baseline.md |
| 3.6 Variance of the evaluation procedure (step 5, 2026-10-18) | s3-baseline.md |
| 3.7 Multiple testing: attempt registry, DSR and PBO | s3-baseline.md |
| 4.1 The gate | s4-gate.md |
| 4.2 Experiments A-E (2026-10-04) | s4-gate.md |
| 4.3 Experiment F: excess-to-IMOEX labels (2026-10-18) | s4-gate.md |
| 4.4 Rerun of the gate on refreshed data (2026-10-20) | s4-gate.md |
| 5.1 Overnight gap stress (generation 1) | s5-exposure.md |
| 5.2 Beta and regime decomposition (generation 1) | s5-exposure.md |
| 5.3 Alpha without beta: synthetic IMOEX overlay (2026-09-25) | s5-exposure.md |
| 5.4 Real IMOEX-futures overlay (2026-10-02) | s5-exposure.md |
| 5.5 Market-neutral retraining (2026-10-03) | s5-exposure.md |
| 5.6 Managed-beta leverage grid (2026-09-25) | s5-exposure.md |
| 5.7 Aggregate exposure cap (`max_net_exposure`) | s5-exposure.md |
| 5.8 Short-borrow cost | s5-exposure.md |
| 5.9 Out-of-sample freeze (2026-09-17) | s5-exposure.md |
| 6.1 Horizon sweep and news-set features (2026-10-08) | s6-labels-features.md |
| 6.2 Label timing: weekend sessions (2026-10-01/02) | s6-labels-features.md |
| 6.3 Training-history depth (step 1, 2026-10-18) | s6-labels-features.md |
| 6.4 Calibration across horizons and regimes (step 3) | s6-labels-features.md |
| 6.5 Breadth of the training panel (step 24/24b, 2026-10-19) | s6-labels-features.md |
| 6.6 Deep history as a walk-forward candidate with costs (step 4) | s6-labels-features.md |
| 6.7 Feature and label variants (part B, steps 11-17, 2026-10-19) | s6-labels-features.md |
| 6.8 Macro and dividend-yield features (2026-10-10) | s6-labels-features.md |
| 6.9 Negotiations and sanctions topic signals (2026-09-18) | s6-labels-features.md |
| 6.10 Explainability of signals (2026-10-18) | s6-labels-features.md |
| 7.1 Headline-classifier recalibration attempts (2026-09) | s7-news.md |
| 7.2 Where the 9596 articles are lost (2026-10-01) | s7-news.md |
| 7.3 Classifier trained on the 2021-2024 archive (step 2, 2026-10-18) | s7-news.md |
| 7.4 Domain-shift test (step 2b) | s7-news.md |
| 7.5 Expanded 260-ticker archive (step 2c) | s7-news.md |
| 7.6 News score net of costs and decay (steps 2g and 2, seven hypotheses) | s7-news.md |
| 7.7 Zero-shot LLM direction on Russian headlines (steps 1 and H3, final 2026-10-19) | s7-news.md |
| 7.8 Event causal study: sanctions, negotiations, incidents (2026-10-09) | s7-news.md |
| 7.9 Level-crossing event study: stops and take-profits (H20, 2026-10-15) | s7-news.md |
| 7.10 News burst as a risk regime (H22, 2026-10-17) | s7-news.md |
| 7.11 Dividend capture event study (2026-09-25), rejected | s7-news.md |
| 8.1 VAR-ICA-MGARCHSK portfolio formation (H23 stage 0, signal-free) | s8a-portfolio.md |
| 8.2 IMOEX regime mask on new BUY decisions (steps 6 and 10) | s8a-portfolio.md |
| 8.3 Cross-sectional ranking of OOS ensemble probabilities (step 7) | s8a-portfolio.md |
| 8.4 Cross-sectional factors on the whole board (part C, steps 18-21 and corrections) | s8a-portfolio.md |
| 8.5 Sequential test of a hand-set composite (step 23) | s8a-portfolio.md |
| 8.6 Ridge and MLP against LightGBM (step 25, 2026-10-19) | s8a-portfolio.md |
| 8.7 Universe model: decomposition, lead-lag, regimes, macro (step 26) | s8a-portfolio.md |
| 8.8 Gate-passing candidate search (step 27) | s8a-portfolio.md |
| 8.9 Report events and dividend history (step 28) | s8a-portfolio.md |
| 8.10 Futures open interest with the AlgoPack key (step 29) | s8a-portfolio.md |
| 8.11 Intraday candles, calendar effects and cycles (step 30, 2026-10-19) | s8b-portfolio.md |
| 8.12 Risk-free hurdle (step 21) | s8b-portfolio.md |
| 8.13 Research-to-live gap attribution (H-R9, 2026-10-19) | s8b-portfolio.md |
| 8.14 Dropping losing tickers: walk-forward vs random drop (2026-10-22) | s8b-portfolio.md |
| 8.15 Order flow on AlgoPack Super Candles, 2020-2026 (2026-10-22) | s8b-portfolio.md |
| 8.16 One hundred AlgoPack hypotheses, verdict window 2025-2026 (2026-10-23) | s8b-portfolio.md |
| 8.17 Does AlgoPack improve the forecasts? Incremental, non-return, market-level and economic-gate tests (2026-10-23) | s8b-portfolio.md |
| 8.18 AlgoPack futures and FX: 101 hypotheses on untouched data (2026-10-23) | s8b-portfolio.md |
| 8.19 AlgoPack, third untouched-data plan: equity flow to futures outcomes, futures hi2, thin equities (2026-10-23) | s8b-portfolio.md |
| 8.20 Composite alpha and cost-aware execution, new model class, realtime verdict run once (2026-10-23) | s8b-portfolio.md |
| 8.21 Meta-variables: macro and exogenous states against the composite (2026-10-24) | s8b-portfolio.md |
| 8.22 Meta-variables wave 2: new series, controls and the decomposition of the RVI lead (2026-10-24) | s8b-portfolio.md |
| 8.23 Wave 3: realistic spread costs, horizon 10/20/40, next-open entry (2026-10-24) | s8b-portfolio.md |
| 8.24 Wave 3b: five ideas from the local models added to the composite (2026-10-24) | s8b-portfolio.md |
| 8.25 Library v2: 100 transforms and combinations of the vital features against the composite (2026-10-24) | s8b-portfolio.md |
| 8.26 Library v3: 105 stock-level moderators of the composite (2026-10-25) | s8b-portfolio.md |
| 8.27 Wave 4: weather, commodities, news tone and a resource-equilibrium index (2026-10-25) | s8b-portfolio.md |
| 8.28 Deflated Sharpe, market beta and sector neutrality of the book (2026-10-25) | s8b-portfolio.md |
| 8.29 Ordinary and preferred share pairs: statistical arbitrage (2026-10-25) | s8b-portfolio.md |
| 8.30 Thin universe (ranks 101-400): the composite with real spreads (2026-10-25) | s8b-portfolio.md |
| 8.31 Execution timing: when a ticker is cheapest to trade (2026-10-25) | s8b-portfolio.md |
| 8.32 News impulse: attention and sentiment shocks (2026-10-25) | s8b-portfolio.md |
| 8.33 Dividend run-up: trading ahead of a predictable public flow (2026-10-25) | s8b-portfolio.md |
| 8.34 TimesFM 3 zero-shot forecasts as a feature and a 12-month momentum lead (2026-10-25) | s8b-portfolio.md |
| 8.35 TimesFM 3 wave A: calibration, surprise, spread, disagreement, flow forecast, shape, sensitivity (2026-10-25) | s8b-portfolio.md |
| 9.1 Root cause of the September-October SELL drift (step 9) | s9-live.md |
| 9.2 Sell-bias diagnostic and the out-of-range guard (2026-10-18) | s9-live.md |
| 9.3 Preflight concentration (2026-10-18) | s9-live.md |
| 9.4 Manual retrain candidate (2026-10-18), rejected | s9-live.md |
| 9.5 Ticker circuit breaker in the gate replay (2026-10-18) | s9-live.md |
| 9.6 Nightly candidate gate aligned with the preflight (2026-10-19) | s9-live.md |
| 9.7 Live sandbox P&L, 2026-09-17..2026-10-18 | s9-live.md |
| 9.8 Execution-quality tracking (Task 16) | s9-live.md |
