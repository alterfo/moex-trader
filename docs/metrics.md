# Metrics

Russian translation: [metrics.ru.md](metrics.ru.md). This file is canonical.

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

## 3. Baseline performance of the deployed recipe

### 3.1 Tail-precision falsification (generation 1, 2026-09-17)

Pre-registered pooled metric over 2026-06-19..2026-09-17 of the deployed `ensemble_model.json` (277ebf7); outcome "in the signal's direction": BUY -> move > +0.5%, SELL -> move < -0.5% over 10 trading days.

| tail decisions | pooled precision | pooled base rate | right-tailed bootstrap p |
|---|---|---|---|
| 500 (303 BUY / 197 SELL) | 42.4% (212/500) | 47.7% | 0.9123 (83 episodes, 10000 trials) |

Decision-day samples 1260; P(move > +0.5%) = 47.1%, P(move < -0.5%) = 48.7%. The tail does not beat the base rate. Per-quarter models were not saved, so this is an approximation of the walk-forward version.

### 3.2 Momentum benchmark (generation 1)

Top-k momentum on `mom_21d` (k = 5, rebalance every 10 trading days), same 18 tickers and costs, window 2026-06-19..2026-09-17 (81 complete trading days).

| strategy | realized | MTM | closed trades | win rate | max DD |
|---|---|---|---|---|---|
| ensemble (deployed) | +34690.64 | +29646.56 | 200 | 48.0% | 1.07% |
| momentum long-only | -6697.58 | -3736.18 | 107 | 59.8% | 1.87% |
| momentum long+short | -13111.02 | -7635.80 | 219 | 59.4% | 1.18% |

The ensemble beats both momentum variants net of costs, so "the model is momentum plus noise" is not supported on this window. Both sides run through one engine, so engine-wide biases cancel in this relative comparison.

### 3.3 Six-quarter grid on the portfolio engine (generation 1)

Deployed artifact substituted on each window (per-quarter models were not persisted), 18 tickers, 1M deposit, 15000 RUB per position, costs 0.0005 x 3.

| window | realized | MTM | closed trades | max DD | kill-frozen days | daily-loss blocked days |
|---|---|---|---|---|---|---|
| 2025-04-01 -> 2025-06-30 | +42453.50 | +42093.83 | 199 | 0.77% | 0 | 0 |
| 2025-07-01 -> 2025-09-30 | +9171.72 | +27576.50 | 104 | 0.90% | 0 | 0 |
| 2025-10-01 -> 2025-12-30 | +19600.27 | +5964.14 | 108 | 1.82% | 0 | 1 |
| 2026-01-05 -> 2026-03-31 | -6655.63 | -6729.68 | 95 | 1.80% | 0 | 0 |
| 2026-04-01 -> 2026-06-30 | +41695.40 | +54945.79 | 110 | 0.90% | 0 | 1 |
| 2026-07-01 -> 2026-09-17 | +47216.95 | +40051.28 | 181 | 1.07% | 0 | 2 |
| total | +153482.21 | +163901.86 | 797 | - | 0 | 4 |

The drawdown kill switch froze 0 days; the portfolio-level daily-loss limit blocked entries on 4 days (under the earlier ticker-local gate it was dead code because `DayStartEquity` was never set).

### 3.4 Preflight replay after hardening (generation 1)

90-day replay 2026-06-19..2026-09-17, 18 tickers, `ensemble_model.json`: realized +43040.38, MTM +38818.21, 197 closed trades, hit rate 49.2%, max DD 1.07%, 3 daily-loss blocked days. Replayed through read-only `cmd/backtest`; the live gate additionally injects real lot sizes, so exact gate numbers differ by lot quantisation.

### 3.5 Purged walk-forward baseline (2026-10-04)

Six-window walk-forward of the recipe on 17 tickers: each window retrains a fresh ensemble on rows with `date <= train_till` (the day before the window) and backtests out of sample on the portfolio engine, with embargo 0 (old behaviour) and embargo 10 (the control `purged_walkforward_control`). Dataset: the 2026-09-18 corrected export with the 434-day warmup. `val_auc` is the fixed held-out window 2026-06-18..09-16 printed by `scripts/export_ensemble.py`; for w1 it overlaps the test window, so w1's AUC is not clean out of sample.

| window | emb0 realized | emb0 trades | emb0 max DD | emb0 kill | emb0 val AUC | emb10 realized | emb10 trades | emb10 max DD | emb10 kill | emb10 val AUC |
|---|---|---|---|---|---|---|---|---|---|---|
| q1 | +8373.93 | 251 | 3.08% | no | 0.5941 | +4301.27 | 66 | 3.57% | yes | 0.5947 |
| q2 | +2935.09 | 216 | 3.51% | no | 0.5654 | +9.36 | 215 | 3.50% | no | 0.5730 |
| q3 | +36239.52 | 194 | 0.66% | no | 0.5357 | +34089.03 | 193 | 0.90% | no | 0.5301 |
| q4 | -9612.54 | 185 | 2.27% | no | 0.5246 | -12339.10 | 187 | 2.46% | no | 0.5257 |
| w2 | -538.58 | 133 | 1.78% | no | 0.4962 | +2676.41 | 141 | 1.17% | no | 0.4872 |
| w1 | +47635.22 | 292 | 1.33% | no | 0.5313 | +31092.50 | 283 | 3.05% | no | 0.5115 |
| total | +85032.64 | 1271 | - | - | mean 0.5412 | +59829.47 | 1085 | - | - | mean 0.5370 |

Purging costs -25203.16 RUB (-29.6%) of realized P&L; the control stays strongly positive. The mean validation-AUC difference (-0.0042) is inside the noise band. Max DD above 3% without a kill trip (q1 emb0, q2 emb0, q2 emb10) reflects that the gate evaluates drawdown on the risk-gate equity basis at order time while the reported figure is the day-close mark-to-market curve. An embargo purge bug in `scripts/export_ensemble.py` (string against Timestamp comparison) was fixed.

### 3.6 Variance of the evaluation procedure (step 5, 2026-10-18)

75 configurations of recipe A (5 seeds x 3 first-test shifts {0, 27, 54 days} x 5 training starts), 15 of recipe D (training from 2015, 5 seeds x 3 shifts) and 3 fold lengths, on 17 tickers, 15 price columns, horizon 10, 8064 test rows (`scripts/procvar.py`).

| quantity | value |
|---|---|
| recipe A pooled AUC over 75 configurations: mean / SD / min / max | 0.4829 / 0.0108 / 0.4605 / 0.5009 |
| SD over seed / over training start / over first-test shift (others fixed) | 0.0019 / 0.0059 / 0.0116 |
| N95: 95th percentile of the absolute pooled-AUC difference between A configurations with the same shift (900 pairs; median 0.0055) | 0.0160 |
| recipe A (start 2024-01-01) pooled AUC by shift 0 / 27 / 54 | 0.4959 / 0.4765 / 0.4901 |
| recipe D pooled AUC by shift 0 / 27 / 54 | 0.5199 / 0.4917 / 0.5117 |
| D minus A over 15 paired configurations: mean / SD / min / max / positive | +0.0203 / 0.0052 / +0.0112 / +0.0272 / 15 of 15 |
| fold length 42 / 63 / 84 days (seed 42, shift 0, same rows): pooled AUC | 0.4933 / 0.5005 / 0.5140 |

Since N95 = 0.0160 > 0.010, any comparison below 0.016 AUC judged on one configuration is inconclusive (this covers depth steps 1-3, experiments A-E by AUC, macro and dividend features), and new comparisons must be paired over at least 15 configurations. The dominant source is where the fold boundaries fall (SD 0.0116), not the seed. What survives is the paired depth comparison (D beats A in 15 of 15), a small gain at the noise floor: recipe A averages 0.483, below chance, and D reaches 0.492-0.520. Absolute predictability on 2025-04..2026-10 is not demonstrated for either recipe.

### 3.7 Multiple testing: attempt registry, DSR and PBO

`cmd/strategyvalidation` / `internal/strategyvalidation` record every configuration tried. On the archived six-quarter realized series (+42453.50, +9171.72, +19600.27, -6655.63, +41695.40, +47216.95, divided by the 1M deposit; the only archived per-period series):

| metric | value |
|---|---:|
| documented attempts (selected / rejected / control, reference, benchmark) | 41 (7 / 24 / 10) |
| observations | 6 quarterly returns |
| mean / sample stdev | 0.025580 / 0.021710 |
| Sharpe (per period) | 1.178255 |
| skewness / kurtosis (non-excess) | -0.309658 / 1.141267 |
| probabilistic Sharpe vs 0 | 0.986645 |
| expected max Sharpe (41 trials) | 0.983521 |
| deflated Sharpe (Bailey and Lopez de Prado) / p-value | 0.642892 / 0.357108 |
| Harvey-Liu t / critical Sharpe | 3.5 / 1.565248 |
| conservative DSR, max(expected, Harvey-Liu) | 0.233384 |

Both DSR forms are below the pre-registered bar of 0.95. PBO (CSCV) was not computable from the summary-only registry; since 2026-10-04 `cmd/walkforward` and `cmd/backtest -wf-dir` persist per-window `period_returns.csv`, and `cmd/strategyvalidation -returns-dirs a,b,c` joins variants into a date x variant matrix (missing dates 0) and computes CSCV PBO (section 4.1). The registry pre-registers the six attempts of the 2026-10-04 plan (control plus experiments A-E) before their numbers existed; the default registry holds 47 attempts (41 historical + 6).

## 4. Deployment gate and single-change experiments

### 4.1 The gate

Fixed on 2026-10-04 before any experiment numbers, against the purged control (`purged_walkforward_control`, section 3.5) on the same six windows with embargo 10, 1M deposit, costs 0.0005 x 3, kill switch on. A variant is deployable only if all hold: (1) total realized P&L exceeds the control; (2) realized P&L is at least the control in at least 4 of 6 windows; (3) max drawdown is at most 3% in every window and the kill switch never trips; (4) for label or training changes (A, B, C) mean validation AUC is not below the control by more than 0.005; (5) PBO over the plan's attempts is below 0.5; (6) the 90-day preflight passes.

PBO over the eight persisted variants (control, A, B, C-lgb, C-xgb, C-logistic, D, E; a 510-period x 8-candidate matrix, s = 16) is 0.1529 (1968 of 12870 combinations overfit), so criterion 5 passes for the set. Registry-level deflated Sharpe (47 attempts, 6 observations) is 0.6260 (p 0.3740). Per-variant DSR on the six windows (not a criterion): control 0.1290, A 0.0641, B 0.2306, C-lgb 0.0347, C-xgb 0.0942, C-logistic 0.1683, D 0.1323, E 0.0899.

| variant | total realized | trades | windows >= control | worst max DD | kill-tripped windows | mean val AUC | failed criteria |
|---|---:|---:|---:|---:|---:|---:|---|
| control | +59829.47 | 1085 | - | 3.57% | 1 | 0.5370 | (baseline) |
| A dividend-adjusted labels | +30596.25 | 1068 | 2/6 | 3.89% | 1 | 0.5382 | 1, 2, 3 |
| B wide training universe | +86817.47 | 767 | 4/6 | 4.38% | 1 | 0.5185 | 3, 4 |
| C lgb only | +19166.99 | 1578 | 1/6 | 3.26% | 0 | 0.5245 | 1, 2, 3, 4 |
| C xgb only | +41881.38 | 1175 | 2/6 | 3.79% | 1 | 0.5037 | 1, 2, 3, 4 |
| C logistic only | +81382.67 | 904 | 4/6 | 3.69% | 1 | 0.5653 | 3 |
| D volatility-scaled size | +59809.47 | 1070 | 3/6 | 3.60% | 1 | - | 1, 2, 3 |
| E sector cap | +34856.60 | 430 | 2/6 | 2.76% | 0 | - | 1, 2 |

**Verdict: nothing passes.** The closest is C-logistic (passes 1, 2, 4; fails 3 on a 3.69% drawdown with a q1 kill trip); B leads on realized P&L but fails drawdown and AUC; E is the only variant satisfying criterion 3 but fails 1 and 2. The live recipe is untouched.

### 4.2 Experiments A-E (2026-10-04)

- **A, dividend-adjusted (total-return) labels.** `LabelModeAbsoluteTR` adds `dividend_net` to the exit price when an ex-date (first session after `last_buy_date`) falls inside (entry, exit]. Label balance moves from 2855 pos / 3509 neg to 2934 / 3347. Realized -29233.22 (-48.9%) against the control, positive in 3 of 6 windows (control 5 of 6); mean val AUC unchanged (0.5382 vs 0.5370). Rejected.
- **B, wide training universe, unchanged trading universe.** Training on the 262-share TQBR list (`data/train_tickers.json`) with the 10 million RUB trailing-turnover filter adds 27557 labelled rows over 140 names (label 0 = 15965, label 1 = 11592); the 17-ticker validation rows stay byte-identical to the control. Realized +86817.47 (+45.1%) but mean val AUC 0.5185 (-0.0185, outside the band) and q1 max DD 4.38% with the kill switch tripping. Rejected.
- **C, calibration and single members.** `internal/model/reliability.go` provides Brier score, 10-bin ECE and reliability tables (`cmd/strategyvalidation -reliability`). Replaying the control artifacts one member at a time:

| member | samples | Brier | ECE (10 bins) | total realized | trades | worst max DD | kill-tripped | mean val AUC |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| ensemble | 7046 | 0.302700 | 0.209763 | +59829.47 | 1085 | 3.57% | 1 | 0.5370 |
| lgb | 7825 | 0.330966 | 0.255687 | +19166.99 | 1578 | 3.26% | 0 | 0.5245 |
| xgb | 7030 | 0.311151 | 0.222077 | +41881.38 | 1175 | 3.79% | 1 | 0.5037 |
| logistic | 7046 | 0.290461 | 0.199672 | +81382.67 | 904 | 3.69% | 1 | 0.5653 |

The logistic member is the best calibrated and earns the most, but every model is materially miscalibrated in the tails (positive rate near 0.42-0.55 in both extreme bins despite predicted probabilities near 0.07 and 0.93). The ensemble rerun is bit-identical to the control. Rejected on the gate.
- **D, volatility-scaled position size.** `risk.vol_scale` multiplies the target notional by `clamp(sigma_median / sigma_ticker, 0.5, 1.5)` (sizing only, no retraining). Realized +59809.47 against +59829.47 (-20.00 RUB, -0.03%): P&L-neutral. Rejected.
- **E, sector exposure cap.** `risk.sector_caps` at 45000 RUB per sector (banks SBER/VTBR/T; oil and gas LKOH/ROSN/TATN/NVTK/GAZP; metals GMKN/PLZL/CHMF/RUAL), per-ticker projection so risk-reducing orders always pass. Realized +34856.60 (-24972.87, -41.7%), trades 1085 -> 430; every window stays under 3% drawdown and the kill switch never trips, but only 4 of 6 windows are positive (q1 flips from +4301.27 to -5697.73). The cap truncates the concentrated legs that carried the best windows, i.e. cuts market beta. Rejected.

### 4.3 Experiment F: excess-to-IMOEX labels (2026-10-18)

Protocol `docs/plans/completed/20261018-excess-label-walkforward.md`, committed before the run: the section 3.5 recipe with `-label-mode excess` (F) against a fresh same-run `-label-mode absolute` control, both exported, retrained and replayed at HEAD on the ai-box. The fresh absolute export reproduces the earlier dataset byte-for-byte except one extra tail row per ticker. Library versions had drifted since 2026-10-04 (xgb 3.4.1, lgb 4.7.0, sklearn 1.9.1, pandas 3.0.5, numpy 2.5.3); the archived `ens-emb10-q1.json` replays at HEAD to +4002.28 / 74 trades against the recorded +4301.27 / 66 (about 93%), so every gate comparison here is against the fresh control.

| window | control realized | F realized | control trades | F trades | control val AUC | F val AUC |
|---|---:|---:|---:|---:|---:|---:|
| q1 | +858.10 | +13303.17 | 243 | 194 | 0.5673 | 0.4559 |
| q2 | +355.57 | -10261.75 | 205 | 174 | 0.5550 | 0.4858 |
| q3 | +32191.60 | -9240.48 | 158 | 100 | 0.5178 | 0.4767 |
| q4 | -10183.21 | -1543.91 | 183 | 33 | 0.4999 | 0.4915 |
| w2 | +16747.59 | -6836.71 | 115 | 40 | 0.4836 | 0.4892 |
| w1 | +27846.13 | +24248.90 | 252 | 184 | 0.4785 | 0.4989 |
| total / mean | +67815.78 | +9669.22 | 1156 | 725 | 0.5170 | 0.4830 |

Gate: total realized FAIL (+9669.22 vs +67815.78); windows >= control 1/6 FAIL; drawdown PASS (F max 2.35%, no kill; the control itself prints 3.07-3.68% day-close DD without kill trips); mean val AUC 0.4830 vs 0.5170 (-0.034) FAIL; PBO {control, F} = 0.1242 (degenerate at two candidates); preflight not run. **Rejected.** The 2026-10-03 single-window AUC advantage (+0.040, section 5.5) does not transfer to the purged walk-forward on the current stack; the excess book has the expected lower-beta profile (max DD at most 2.35%) because the label removes most of the beta income by design.

### 4.4 Rerun of the gate on refreshed data (2026-10-20)

Core rerun study R2 (`docs/research/20261019-core-rerun.md`, plan `docs/plans/20261019-core-rerun.md`): `scripts/rerun-r2.sh` on the ai-box, data frozen at D = 2026-10-16, the six windows of section 4.1 plus a new window x (train to 2026-09-16, test 2026-09-17..2026-10-16), fresh same-run control, C-logistic (logistic member of the control models) and F (excess labels). Costs, embargo 10 and sizing as in section 4.1.

| window | control realized | C-logistic realized | F realized | control trades | C-logistic trades | F trades |
|---|---:|---:|---:|---:|---:|---:|
| q1 | +2607.26 | +1797.63 | +13050.18 | 237 | 193 | 203 |
| q2 | -650.70 | -1132.66 | -6211.63 | 212 | 212 | 182 |
| q3 | +36768.40 | +44663.60 | -11808.39 | 157 | 117 | 94 |
| q4 | -12043.88 | -6176.50 | -3403.71 | 182 | 187 | 38 |
| w2 | +17530.66 | +15832.40 | -3604.79 | 113 | 56 | 33 |
| w1 | +24237.51 | +11510.17 | +13888.98 | 226 | 48 | 178 |
| x (new) | -2475.87 | -676.86 | -4347.77 | 55 | 41 | 19 |
| total, six original windows | +68449.25 | +66494.64 | +1910.64 | 1127 | 813 | 728 |
| total, seven windows | +65973.38 | +65817.78 | -2437.13 | 1182 | 854 | 747 |

Worst max drawdown over seven windows: control 3.52%, C-logistic 4.35% (kill switch trips in w1), F 2.24%; mean validation AUC over seven windows: control 0.5105, C-logistic member 0.5269, F 0.4859. Windows at or above control: C-logistic 3 of 7, F 2 of 7.

Gate (criteria 1-4 of section 4.1; 5 and 6 not run): C-logistic fails 1 (-155.60), 2 and 3; F fails 1, 2 and 4 (-0.0246). **Nothing passes**, as in the original run. Reproduction against section 4.3: the control's six-window total is +68449.25 against +67815.78 (within 1%), per-window values differ by up to 6000 RUB (q1 +2607.26 against +858.10, w1 +24237.51 against +27846.13) because the news archive grew and libraries drifted; F's six-window total is +1910.64 against +9669.22, same sign pattern (F loses in q3, q4, w2 in both). The C-logistic lead of section 4.1 (+81382.67 against +59829.47) does not reproduce: on the same fresh control it is level (-0.2% over seven windows). The new window x is a small out-of-sample extension, below the noise of single-window comparisons.

## 5. Market exposure, hedging and borrow

### 5.1 Overnight gap stress (generation 1)

Open portfolio at the end of the 2026-06-19..09-17 replay: 18 positions, gross 266573 RUB (long 43687, short 222887, net -179200). The kill switch blocks new entries and does not liquidate, so a gap hits equity in full first.

| scenario | P&L | % of deposit | breaches 3% DD | breaches 0.5% daily |
|---|---|---|---|---|
| worst historical day (2026-03-09) | -3827.21 | -0.38% | no | no |
| worst per-ticker composite | -6731.51 | -0.67% | no | yes |
| synthetic -10% | +17919.97 | +1.79% | no | no |
| synthetic -20% | +35839.93 | +3.58% | no | no |
| synthetic +10% | -17919.97 | -1.79% | no | yes |
| synthetic +20% | -35839.93 | -3.58% | yes | yes |

### 5.2 Beta and regime decomposition (generation 1)

One continuous replay of the deployed ensemble, 2025-04-01..2026-09-17, 18 tickers, costs 0.0005 x 3, decomposed net of IMOEX.

| strategy | realized | MTM | closed trades | return on trade notional | max DD |
|---|---|---|---|---|---|
| ensemble (deployed) | +194293.10 | +188225.76 | 862 | +4.77% | 2.11% |
| equal-weight 18 tickers | -673.13 | -68102.99 | 516 | -0.24% | 11.97% |
| momentum long-only | -20948.09 | -38708.67 | 500 | -1.27% | 4.68% |
| momentum long+short | -32082.28 | -26713.25 | 1151 | -0.80% | 3.20% |

| leg | trades | realized | notional | IMOEX component | excess | return on notional |
|---|---|---|---|---|---|---|
| long | 261 | +67261.43 | 1986656.57 | +28429.45 | +38831.98 | +3.39% |
| short | 601 | +127031.67 | 2090319.43 | +100892.32 | +26139.35 | +6.08% |
| total | 862 | +194293.10 | 4076976.00 | +129321.77 | +64971.33 | +4.77% |

The IMOEX (beta) component is about twice the excess component: the book is short-tilted (601 short vs 261 long trades) and earned much of its P&L from short beta in a declining index. Regression of P&L on alpha, equal-weight and momentum benchmarks with cluster-robust errors: per trade (862 trades, 18 ticker clusters) alpha +0.01056 (t 1.47), equal-weight beta -1.681 (t -6.19), momentum beta +1.070 (t 0.78), R2 0.111; per day (486 days, 6 quarter clusters) alpha +0.00031 (t 4.66), equal-weight beta -0.274 (t -2.64), momentum beta -0.197 (t -1.05), R2 0.233. Quarter regimes (IMOEX return, +-3% flat band): 2025Q2 -3.95% down, 2025Q3 -5.75% down, 2025Q4 +4.51% up, 2026Q1 +0.70% flat, 2026Q2 -15.39% down, 2026Q3 -2.55% flat. Only one of six quarters is a bull quarter; the short book earned in the three down quarters.

### 5.3 Alpha without beta: synthetic IMOEX overlay (2026-09-25)

Pre-registered (commit `d39da47`). Does the deployed ensemble's daily alpha survive execution-level beta neutralisation by a synthetic IMOEX overlay (no position sign or size changes)? HEAD rerun: 867 trades, +202640 realized (about 4% engine drift against section 5.2).

| overlay share | leg P&L | leg costs | daily alpha (cluster t, df = 5) | beta equal-weight | beta momentum | bootstrap CI at block 20 / 40 |
|---|---|---|---|---|---|---|
| 0.00 | 0 | 0 | +0.000325 (3.98) | -0.291 | -0.084 | excludes 0 / excludes 0 |
| 0.50 | -68502 | 3744 | +0.000216 (3.45) | -0.181 | -0.125 | excludes 0 / excludes 0 |
| 1.00 | -137004 | 7488 | +0.000101 (2.10) | -0.065 | -0.171 | [+0.000017, +0.000190] / [+0.000018, +0.000179] |

At share 1.00 the CI at block 5 touches zero ([-0.000002, +0.000209]); blocks 10/60 give [+0.000012, +0.000196] / [+0.000030, +0.000179]. All six leave-one-quarter-out drops keep alpha positive. Registered bar met (CI excludes 0 on contiguous {20, 40}; leave-one-out positive), with the caveat that the overlay leg is frictionless and that **all numbers were produced without short borrow**. Rerun with borrow 0.00005 per day: share 0 alpha 0.000316 (t 3.84), 0.5 alpha 0.000206 (t 3.25), 1.0 alpha 0.000090 (t 1.85, p 0.124), CI {20, 40} still excludes 0 (L10 touches 0); at twice the rate (0.00010) the condition fails (L20 [-0.000008, +0.000170], L40 [-0.000006, +0.000158]). The alpha is sensitive to the real borrow rate.

### 5.4 Real IMOEX-futures overlay (2026-10-02)

The preceding test replaced by the tradable instrument: MX front-month series from ISS FORTS (quarterly roll 3 calendar days before last trade), contract notional 230000 RUB (GO 27464 RUB, about 11.9%), fee 15.18 RUB per contract per side. HEAD rerun base: 778 trades, +184077 realized.

| overlay | share | avg contracts | leg P&L | leg costs | avg daily alpha (cluster t) | beta equal-weight | CI {20, 40} excludes 0 |
|---|---|---|---|---|---|---|---|
| none | 0.00 | - | 0 | 0 | +0.000300 (3.68) | -0.287 | yes |
| synthetic cash IMOEX | 0.50 | - | -66079 | 3622 | +0.000193 (3.18) | -0.179 | yes |
| synthetic cash IMOEX | 1.00 | - | -132158 | 7244 | +0.000080 (1.83) | -0.065 | yes |
| real MX, whole contracts | 0.50 | 0.03 | -19000 | 1681 | +0.000266 (3.12) | -0.284 | yes (hedge almost never fires) |
| real MX, whole contracts | 1.00 | 0.48 | -174800 | 16906 | -0.000036 (-0.49) | -0.053 | no |
| real MX, divisible (unit 0.01) | 1.00 | 46.7 | -144940 | 14314 | +0.000028 (0.39) | -0.054 | no |

One contract (230000 RUB) against an average beta exposure of about 107000 RUB makes the hedge bang-bang; the fully divisible variant loses significance too (about 12800 RUB of basis, roll and tracking separate it from the frictionless overlay). **The alpha-without-beta claim does not survive the real instrument**: realized P&L is predominantly a net-short IMOEX-beta bet. This supersedes section 5.3 and closes the beta-neutral income lever at the current book size (about 200k); reopening needs a much larger book or per-name single-stock futures.

### 5.5 Market-neutral retraining (2026-10-03)

Excess-to-IMOEX labels against absolute labels, otherwise identical (13438 rows; absolute 5398 pos / 6802 neg / 1238 hold, excess 4116 / 4179 / 5143 hold: the dead band swallows more rows). Validation AUC (2026-06-18..09-16), xgb / lgbm / logreg / ensemble: absolute 0.4895 / 0.4922 / 0.4634 / 0.4824; excess 0.5447 / 0.5237 / 0.4570 / 0.5226 (ensemble +0.040, outside the noise band; not reproduced in the walk-forward of section 4.3). Hedged books (2025-04-01..2026-09-17, models rebuilt here):

| model | overlay | trades / realized | leg P&L | alpha per day (cluster t) | beta unhedged / hedged | CI {20, 40} excludes 0 |
|---|---|---|---|---|---|---|
| absolute | none | 789 / +221316 | 0 | +0.000363 (6.02) | -0.302 / - | yes |
| absolute | synthetic 1.0 | - | -143051 | +0.000130 (4.43) | / -0.066 | yes |
| absolute | real MX whole 1.0 | - | -189900 | +0.000007 (0.12) | / -0.056 | no |
| absolute | real MX divisible 1.0 | - | -165809 | +0.000062 (1.21) | / -0.057 | no |
| excess | none | 392 / +81881 | 0 | +0.000175 (2.96) | -0.189 / - | yes |
| excess | synthetic 1.0 | - | -43117 | +0.000108 (3.46) | / -0.073 | yes |
| excess | real MX whole 1.0 | - | -33100 | +0.000125 (2.49, p 0.055) | / -0.029 | yes (hedge fires rarely) |
| excess | real MX divisible 1.0 | - | -48030 | +0.000085 (2.04, p 0.097) | / -0.079 | no |
| excess, borrow 5 bp/day | synthetic 1.0 | - | -43117 | +0.000102 (3.17) | / -0.073 | yes |

Excess labels work as intended (beta loading -0.189 vs -0.302, a real intercept) at 2.6x lower realized P&L. With a divisible hedge the excess alpha is 0.000085 per day (t 2.04, p 0.097), about 2.7x the dead absolute residual but short of the registered bar. Not a deployment-ready income source.

**Phase 2b, regime-balanced training (2026-10-03), rejected.** Inverse-frequency weights by trailing cross-sectional mean return (`ENSEMBLE_REGIME_BALANCE`, window 21 or 63, 2-4 buckets) lower validation AUC in every variant (ensemble 0.4964 / 0.4948 / 0.4804 / 0.4887 against 0.5226 unweighted); after a divisible hedge the residual alpha turns negative (rb21 b3 -0.000124, t -1.57; rb21 b2 -0.000165, t -1.94, CI entirely negative). Beta-residualised price features were deferred (cross-zone live/backtest parity risk, small expected gain).

### 5.6 Managed-beta leverage grid (2026-09-25)

Beta-neutral book (`-shares 1`), borrow 0.00005 per day, leverage = larger `-target-notional` on the same 1M deposit (no broker margin). Acceptance: max quarterly DD at most 3% and positive realized net at every multiplier.

| target notional | realized net of borrow | daily alpha (t) | max quarterly DD | result |
|---|---|---|---|---|
| 15000 (1x) | +197275 | +0.000090 (1.85) | 1.11% | pass (baseline) |
| 30000 (2x) | +395785 | +0.000195 (1.92) | 2.15% | pass |
| 45000 (3x) | +589992 | +0.000293 (2.01) | 2.56% | pass |
| 60000 (4x) | +753632 | +0.000364 (1.95) | 3.55% | fail |

The hedged book scaled 2-3x under the 3% budget; this lever depends on the hedge, which section 5.4 shows is not achievable at the current book size.

### 5.7 Aggregate exposure cap (`max_net_exposure`)

**Unhedged live book, HEAD rerun 2026-09-30** (signed-net semantics; window 2025-04-01..2026-09-17, borrow 0.00005 per day; the 2026-09-25 figures predate the 2026-09-17 feature fixes):

| target notional | cap | realized net of borrow | trades | max quarterly DD | result |
|---|---|---|---|---|---|
| 15000 (1x, live) | 0 | +197275 | 871 | 1.96% | pass |
| 15000 | 90000 | +87861 | 341 | 1.71% | pass (income -55%) |
| 30000 (2x) | 0 | +395785 | 1012 | 3.57% | fail |
| 30000 | 60000 | +81730 | 234 | 2.21% | pass (2.31% at twice the borrow) |
| 30000 | 75000 | -6555 | 50 | 2.52% | fail (income negative) |
| 30000 | 90000 | +144058 | 249 | 1.94% | pass |
| 45000 (3x) | 0 | +589992 | 1050 | 4.26% | fail |
| 45000 | 90000 | -19338 | 74 | 4.95% | fail (worse than uncapped on both axes) |
| 45000 | 120000 | +127923 | 188 | 3.85% | fail |

The only cap keeping every quarter under 3% (2x with 60k) earns less than half the live 1x. The cap is discontinuous and non-monotonic (60k to 75k turns +81730 into -6555 as trades collapse 234 to 50), so the profitable region cannot be tuned into: the book's income is its market beta and an aggregate cap removes the cross-name diversification. Stay at 1x; the cap ships as a fail-closed guardrail only.

**Synthetic 100% short book (2026-09-30)**, SELL on every bar and BUY on the last: cap 0 +51381 (1581 trades, max DD 5.83%, 18 names); 30000 +4981 (2 trades, 0.86%); 45000 +5864 (3, 1.04%); 60000 +8831 (4, 0.87%); 90000 +11484 (6, 1.68%). A cap costs 78-90% of the income because `risk.Gate` is an order-level fail-closed check that never resizes: the first tickers fill the cap and the book freezes. The uncapped all-short control breaches the 3% drawdown on its own (5.83%), so the live loss of 2026-09-30 (17 shorts, net about -210k, -4.53 RUB realized over 60 trades) was not caused by a missing cap.

**Semantics change, signed net to gross (2026-10-01).** The cap now bounds the sum of `|signed position notional|`; the net cap is blind to gross risk (long 500k SBER and short 500k OZON net to zero). Per-ticker projection (the order replaces only its own leg) and an explicit `risk.Request.ExposureDeltaLots` are load-bearing: treating `TargetLots` as a delta double-counted held positions (a latent bug). Regression: the 100% short grid reproduces exactly (cap 0 +51381.37, 1581 trades, 5.83%; 30000 +4981.30, 2, 0.86%; 45000 +5863.69, 3, 1.04%; 60000 +8830.73, 4, 0.87%; 90000 +11483.63, 6, 1.68%); an intermediate book-wide-additive implementation collapsed the capped rows to 0 closed trades by blocking the final flatten-to-flat order (`TestHardenedGateMaxExposureAllowsReducingOrder`). Live keeps `max_net_exposure: 0`.

### 5.8 Short-borrow cost

**Measurement.** `cmd/borrowmeasure` (180-day lookback) shows 17 of 18 names shortable (DATA is not) and 0 margin-fee operations (0 RUB); the broker API exposes short margin risk rates but no borrow fee. The real T-Invest accounts (brokerage and investment piggy bank, 2020-2026, 271 and 2354 operations) contain no `MARGIN_FEE` operation and no short trade, and margin status is disabled on both (code 30051); the sandbox charges no rollover. Public terms: intraday shorts are free, overnight carry is charged per calendar day; a secondary source quotes the REPO rate at about 17% per year (unconfirmed by the account tariff). The project's stress rate of 0.005% per day (1.8% per year) is about ten times below this estimate. `-borrow-pct-day` charges the rate daily on short notional and reports `RealizedPnlNetBorrow`.

**Sensitivity of the ridge long-short replay** (section 8.8 recipe, 17 stocks, caps 90000), realized net of borrow, RUB:

| rate per day | window G | sum of six windows | positive windows | worst window |
|---|---|---|---|---|
| 0.00005 (1.8% per year) | +3916 | +684 | 3 of 6 | -2697 |
| 0.00023 (8.4%) | +3583 | -1611 | 2 of 6 | -3097 |
| 0.00047 (17.2%) | +3140 | -4672 | 2 of 6 | -3631 |
| 0.00070 (25.6%) | +2715 | -7605 | 2 of 6 | -4143 |

The gate window passes at any rate up to 25% per year, but at about 17% per year the six historical windows sum negative; if the real rate is close to REPO the short part of ridge long-short is loss-making on average.

### 5.9 Out-of-sample freeze (2026-09-17)

The abs-10d recipe is frozen for two quarters (minimum collection 183 days; real-money execution disabled): frozen at 2026-09-17T16:09:40Z, code SHA `30a422a9eb7193ab9300d00b4ec7c9190b98e94e`, recipe SHA256 `7d6e616c8973e8d2c32110e049b5a539c616ba367fab86732ac0aa850e51de9e`, model SHA256 `feb827494ba0bc943ab722d1dff888e0fe4c523bd170d463a5e3ca2fa11e6140`, config SHA256 `54c8d1949fd8c3b514c8a2b9c23a6c9048d7e20a433ad24038ab8eec6540b1ef`. `cmd/oosfreeze` recomputes the recipe hash from the current files and resets the collection count when it changes.

## 6. Labels, horizon, training data and features

### 6.1 Horizon sweep and news-set features (2026-10-08)

Expanding purged walk-forward, 9 folds of 63 days from 2025-04-01, 15 price columns, LightGBM + logistic average.

| horizon (days) | 9 | 10 | 12 | 15 | 20 | 25 | 30 | 40 |
|---|---|---|---|---|---|---|---|---|
| mean fold AUC | 0.5292 | 0.5311 | 0.5204 | 0.5018 | 0.4864 | 0.4818 | 0.4730 | 0.4617 |
| pooled AUC | 0.5038 | 0.4974 | 0.4875 | 0.4799 | 0.4646 | 0.4695 | 0.4734 | 0.4678 |

AUC falls monotonically with horizon (below 0.5 from 15 days; per-fold range 0.33-0.64); 10 days is the best and is deployed. Rolling per-ticker news-set features (5/10/20-day counts, surge, keyword balance, topic means, sources, market-wide count `mkt10`; archive 2026-04-01..09-29, 21-day folds from 2026-06-01): pooled AUC gains are market timing (`mkt10` alone 0.58-0.61); the within-day cross-sectional news AUC is 0.55-0.57 (10-day CI [0.504, 0.612], 20-day [0.517, 0.620]) before correcting for label overlap, not significant. Price features are anti-predictive across tickers in 2026-06..09 (within-day AUC 0.35-0.39); flipping them would fit a known regime. No variant approached 0.7 (best about 0.57).

### 6.2 Label timing: weekend sessions (2026-10-01/02)

MOEX trades equities on weekends (SBER Saturday 2026-06-06: volume 1373638, 55 intraday bars) while IMOEX is not calculated, so a 10-bar window spans a different calendar time per ticker: over 14041 labels the effective weekday horizon is 7-11 (mean 9.31, only 9.3% exactly 10), and the mean calendar span ranges from 11.33 days (DATA) to 13.69 (OZON). Live trades the same bars as the backtest (the broker reports weekend sessions as open), so the track record is not invalidated. Rebuilding labels with `-horizon-mode calendar_days` changes the sign of 667 of 12328 common keys (5.41%); on the holdout 2026-06-19..09-17 (bars vs calendar): validation AUC 0.4610 vs 0.4635 (+0.0025, inside the band), realized +53786.79 vs +40355.35 (-25%), max DD 1.58% vs 1.97%. The inconsistency is real but is not the AUC ceiling; the bars recipe stays.

### 6.3 Training-history depth (step 1, 2026-10-18)

`exportdataset -from {2015,2018,2021,2024}`, 17 tickers, 15 price columns, 54017 rows per horizon, the section 6.1 walk-forward; test rows identical across arms.

| h | arm | train rows | pooled AUC | mean fold AUC | folds > 0.5 | ECE |
|---|---|---:|---:|---:|---:|---:|
| 10 | A from 2024 | 4762 | 0.5005 | 0.5371 | 6/9 | 0.0792 |
| 10 | B from 2021 | 15490 | 0.4958 | 0.5102 | 6/9 | 0.0410 |
| 10 | C from 2018 | 24745 | 0.4921 | 0.5076 | 5/9 | 0.0654 |
| 10 | D from 2015 | 33548 | 0.5199 | 0.5287 | 6/9 | 0.0734 |
| 10 | E from 2015 without 2022-02-24..12-31 | 30542 | 0.5331 | 0.5378 | 6/9 | 0.0809 |
| 20 | A | 4740 | 0.4696 | 0.4963 | 5/9 | 0.1211 |
| 20 | B | 15752 | 0.4558 | 0.4754 | 5/9 | 0.0759 |
| 20 | C | 25268 | 0.4539 | 0.4901 | 5/9 | 0.0981 |
| 20 | D | 34331 | 0.4883 | 0.5111 | 5/9 | 0.0919 |
| 20 | E | 31284 | 0.4985 | 0.5025 | 5/9 | 0.0977 |

D minus A pooled AUC: h10 +0.0194 (CI95 [-0.0062, +0.0457], D wins 3 of 9 folds), h20 +0.0188 ([-0.0037, +0.0420], 5 of 9). The primary criterion (+0.010 on both horizons, majority of folds above 0.5) is met on the point estimates but is not a swap decision (both intervals contain zero; the secondary criterion fails); B and C are below A, so the gain appears only at 2015 (more regimes), and the absolute level stays at noise. Calibration improves with depth (Brier 0.2564 -> 0.2511 at h10). Excluding 2022 adds +0.0132 / +0.0102 (descriptive). The paired comparison of section 3.6 later confirmed D > A (15 of 15) as a small gain at the noise floor. No model swap.

### 6.4 Calibration across horizons and regimes (step 3)

Platt and isotonic calibrators fitted on the last 20% of training rows, horizons 5/10/20, windows A and D, pooled over 9 folds. Window A: both calibrators worsen ECE and Brier at every horizon (ECE x2 to x3.3). Window D: ECE falls 54-70% with Brier level or slightly better, but AUC drops by 0.07-0.19 (e.g. h10 raw 0.5465 -> 0.4130 Platt): on the most recent 20% of training dates the base score is anti-correlated with the outcome, so the fitted monotone map inverts the ranking. Criterion (ECE -20%, Brier not worse, AUC within +-0.005, all six combinations): not met on any. The research ensemble's raw probabilities sit near 0.5 (raw ECE 0.044-0.107), unlike the deployed ensemble whose tails reach 0.07 and 0.93 (ECE 0.21, section 4.2). The raw AUC of arm A here (0.551-0.555) is far above its step-1 value (0.470-0.501) merely by holding the last 20% of training rows out: an up-to-0.08 swing that places every 0.01-0.02 difference of steps 1-3 inside recipe noise. No calibrator is wired in.

### 6.5 Breadth of the training panel (step 24/24b, 2026-10-19)

172 securities that ever passed the liquidity threshold on TQBR 2014-2026 (including delisted), the project's own 15 features from split-adjusted candles, 376553 labelled rows (43395 for the 18 trading tickers); test = the 18 trading tickers on the 15 configurations of section 6.7. Pooled AUC on the original label: base18 0.4786 (Spearman -0.030), wide 0.4957 (+0.018), wide2020 0.5051 (+0.035); paired difference to base18 +0.0171 (14/15) and +0.0265 (15/15): stage 1 passes, but absolute AUC stays at chance. At the fixed 0.60/0.40 thresholds the wide models almost never trade (4-5 closed trades over six windows for wide, 11-17 for wide2020, about 490 for base18), so the formal stage-2 pass is not informative. With quantile thresholds (BUY above the 80th and SELL below the 20th percentile of the model's own probabilities over the previous 60 days; about 580 trades per arm and seed), sum of the six windows by seed 0..4: base18 -28053, -32124, -30521, -27037, -20835 (mean -27714); wide -12376, -14832, +139, -7442, -17370 (mean -10376); wide2020 -2853, -24862, -14365, -10516, +662 (mean -10387); worst DD 2.37% / 2.06% / 1.83%; hold-out h mean +384 / -623 / -1391. Criteria hold-out not worse and sum positive FAIL. Breadth reduces the loss by about 17000 RUB but does not make the recipe profitable.

### 6.6 Deep history as a walk-forward candidate with costs (step 4)

Per-window ensembles trained from 2024-01-01 (A, fresh control) and from 2015-01-01 (D), replayed with the Task 5 parameters.

| window | A realized | D realized | A trades | D trades | A val AUC | D val AUC |
|---|---:|---:|---:|---:|---:|---:|
| q1 | -8823.41 | +1083.86 | 171 | 8 | 0.4932 | 0.5083 |
| q2 | -8151.57 | +1913.04 | 174 | 5 | 0.4882 | 0.4873 |
| q3 | +28863.29 | 0.00 | 122 | 0 | 0.4354 | 0.4755 |
| q4 | -6022.44 | 0.00 | 94 | 0 | 0.4302 | 0.4770 |
| w2 | -3793.02 | 0.00 | 71 | 0 | 0.4246 | 0.4689 |
| w1 | +38865.30 | +1512.39 | 181 | 39 | 0.4350 | 0.4790 |
| total / mean | +40938.14 | +4509.29 | 813 | 52 | 0.4511 | 0.4827 |

D fails criterion 1 (+4509 vs +40938) while passing the others: validation AUC is higher in every window, but the deep-history probabilities sit closer to 0.5 and the fixed 0.60/0.40 thresholds are almost never crossed (52 trades vs 813). Thresholds were not retuned on the test windows. A's profit comes from q3 and w1, the two windows where the short-tilted book rode falling prices (section 5.2).

### 6.7 Feature and label variants (part B, steps 11-17, 2026-10-19)

Data: `exportdataset` 2024-01-01..2026-10-19 (18 tickers, 13211 labelled rows), 15 configurations (5 seeds x shifts 0/27/54, 63-day folds from 2025-04-01, 6403 test rows each), purged on `label_date`. A look-ahead bug (per-date ranks computed after the dead-band filter) was fixed before any result was recorded.

| step | variant | base pooled AUC | variant pooled AUC | mean paired d | positive pairs | stage 1 |
|---|---|---|---|---|---|---|
| 11 | per-date ranks of the 15 price features + 3 momentum-minus-IMOEX features | 0.4905 | 0.5161 | +0.0256 | 15/15 | pass |
| 12 | labels in volatility units (+-0.5 sigma20 sqrt(10)), scored on the original label | 0.4905 | 0.5019 | +0.0114 | 15/15 | pass |
| 13 | meta-model filter, top 30% of rows | - | - | AUC d -0.0004 | 4/15 | fail |
| 14 | FORTS open interest of individuals (own futures 5-day change and level for 9 shares + MX index future), previous-day data | 0.4749 | 0.4825 | +0.0076 | 11/15 | fail |
| 14b | same, lagged 14 days (free-tier availability) | 0.4760 | 0.4471 | -0.0289 | 0/15 | fail, harmful |
| 16a | TimesFM 2.5 zero-shot median forecast return | 0.5 | 0.5220 (shift 27: 0.5200, 54: 0.5253) | - | - | fail: CI [0.487, 0.553] includes 0.5 |
| 16b | base + TimesFM forecast features | 0.4905 | 0.4927 | +0.0022 | 14/15 | fail |

Stage 2 (csvprob replay of the walk-forward probabilities, 0.60/0.40, net caps 90000, borrow stress), realized net of borrow:

| window | base | rank (11) | sigma (12) |
|---|---|---|---|
| q1 | -10968 | -8035 | -7451 |
| q2 | -4057 | -6344 | -6168 |
| q3 | +2982 | +8685 | +12633 |
| q4 | -11438 | -18071 | -7866 |
| w2 | -9831 | -2392 | -8606 |
| w1 | +37654 | +7198 | +29069 |
| sum | +4342 | -18959 | +11611 |
| worst max DD | 1.82% | 2.55% | 1.99% |

Step 11 fails stage 2 (sum and drawdown), step 12 passes it, but every figure is inside replay noise. **Step 17 (pre-registered, 98 replays across seeds 0-4 and the hold-out h):** sigma-label six-window sum by seed +11542, +22722, +1613, +4569, +17338 against base -10218, +7988, -13842, +7015, +17461 (means +11557 vs +1681); hold-out h mean -882 vs -603; worst DD 2.27% vs 2.36%. Sigma beats base in 3 of 5 seeds (needs 4) and fails the hold-out: **the step-12 result is not confirmed** (2 of 4 criteria fail). Descriptive (seed 42, k = 0.25 / 0.5 / 0.75): sums +768 / +11611 / +33464, hold-out -595 / -3058 / -5308, non-monotonic; the seed-to-seed range of the base sum (-13842..+17461) exceeds any base-sigma difference.

Step 14 detail: `iss.moex.com/iss/analyticalproducts/futoi` is open without a token with history from 2023-12 but the free tier withholds the last 14 days, so a live model would see the lagged arm (14b), which is worse than base. Step 15 (fundamentals): the T-Invest `GetAssetFundamentals` returns a current snapshot of about 60 fields without an as-of date (and `GetAssetReports` report dates with `createdAt`), so any historical join leaks the future; `cmd/fundsnap` records a daily append-only snapshot (`data/fundamentals.jsonl`, first snapshot 2026-10-19); no experiment before at least 120 daily snapshots (about 2027-02). TimesFM extraction: 14545 forecasts in 2432 s on CPU.

### 6.8 Macro and dividend-yield features (2026-10-10)

Walk-forward as in section 6.1 on 12540 labelled rows (44.4% positive, no news): base A (15 price/flow) mean fold AUC 0.5320 / pooled 0.4976 (folds > 0.5: 6/9); +dividend yield 0.5270 / 0.4963 (6/9); +9 macro series (IMOEX, CNY/RUB, Brent, key rate) 0.4912 / 0.4175 (3/9); both 0.4853 / 0.4178 (4/9). Macro columns are identical for all tickers on a date, so trees only memorise the regime (-0.041 mean fold, -0.080 pooled). Criterion (+0.010 pooled and a majority of folds above 0.5): met by no arm; no backtest was run. Macro may still help as a risk-gate regime filter; step 26 (section 8.7) later shows no out-of-sample macro information.

### 6.9 Negotiations and sanctions topic signals (2026-09-18)

With versus without `negotiations_signal` and `sanctions_signal` (same 18-ticker dataset, absolute-10d, costs 0.0005 x 3): validation AUC xgb / lgbm / logreg / ensemble 0.4896 / 0.5172 / 0.5080 / 0.5084 (18 features) vs 0.4920 / 0.5172 / 0.5080 / 0.5098 (20 features), ensemble +0.0014 (inside the band). Six-window realized: 18-feature +101246.75 (max DD 3.52%), 20-feature +103739.34 (3.55%), +2492.59 (+2.5%) with mixed signs (worse in q1, q4, w1). The news archive then covered only 2026-08-14..09-17, so the signals were zero in every training window (9 and 5 non-zero rows, all in the last test window). No measurable improvement; the comparison needs a richer historical backfill. The topic features have zero coefficients in the deployed model.

### 6.10 Explainability of signals (2026-10-18)

Occlusion attribution: each trained feature is replaced by its training mean and the change in ensemble probability is the driver; the top 3 are appended to `Reasoning` as ` drivers=<feature>:<signed percentage points>` (not Shapley: interactions are unsplit and the tree artifact has no node covers). Unit tests confirm the logistic occlusion delta equals the exact linear contribution to 1e-9. `cmd/explaincheck` on the deployed artifact (10195 rows from 2025-04-01, 17 tickers, 20 draws per row): replacing the 3 top drivers moves the probability by 0.0822 against 0.0236 for 3 random trained features (ratio 3.48, needed 1.5); in the decision zone (3964 rows) the sign of the top driver matches the side of 0.5 in 95.9% of rows (needed 70%). Both criteria met. The claim is faithfulness of the model's own dependence on inputs, not causality of returns.

## 7. News and event information

### 7.1 Headline-classifier recalibration attempts (2026-09)

Both attempts label headlines by forward excess return over IMOEX (`cmd/trainnewsmodel`, horizon 3 days unless stated, bag-of-words logistic).

- **2026-09-25, 1517 headlines (20 tickers, 2026-08-14..09-24).** Horizon 3: val AUC 0.568, accuracy 0.605 vs baseline 0.591. The deployed classifier (h = 3, trained 2026-09-16 on 48k Telegram samples; val AUC 0.533 below baseline 0.552) scores every corpus headline in [-0.22, 0.09]: dead weight. Horizon 10 shows val AUC 0.80-0.89 but is a label-imbalance artifact (84% of headlines score below -0.3; validation labels 43 pos / 91 neg). Horizon 5: 0.587, noise.
- **2026-09-29, 9596 articles rebuilt from RSS/Google/CBR/MOEX (2315 records) and Telegram retro (7281), 2026-04-01..09-29, 42 tickers.**

| variant | articles | train_n | val_n | train AUC | val AUC | val acc | baseline |
|---|---|---|---|---|---|---|---|
| deployed 2026-09-16 | 48294 | 48294 | 2364 | 0.690 | 0.533 | 0.529 | 0.552 |
| A all tickers, split 08-15 | 9596 | 2855 | 1349 | 0.840 | 0.531 | 0.538 | 0.570 |
| B config tickers only | 3415 | 759 | 497 | 0.950 | 0.493 | 0.469 | 0.511 |
| C all tickers, split 09-01 | 9596 | 3115 | 1089 | 0.830 | 0.527 | 0.534 | 0.576 |
| D one ticker per article | 6858 | 2125 | 970 | 0.880 | 0.540 | 0.535 | 0.580 |

Every variant is below the majority baseline; the train-validation AUC gap (0.31-0.46) is overfitting, and with about 1000 validation samples the AUC standard error is about 0.016, so the 0.527-0.540 spread is noise. Behaviourally the deployed classifier is dead (92% of scores inside +-0.1, range [-0.247, +0.266]) while the new ones fire on about 64% of headlines (range about +-0.8) with below-baseline accuracy: deploying them would swap a harmless near-zero feature for an actively wrong one. `news_classifier.json` stays. These numbers use the ToS-restricted Telegram and Google News sources (owner decision recorded in `AGENTS.md`) and must not be quoted as a clean corpus.

### 7.2 Where the 9596 articles are lost (2026-10-01)

An instrumented replica of `BuildNewsLabels` (asserted to reproduce its sample count; `TestNewsLabelsDropAudit`) accounts for every drop: ticker lookup failures 0, short history 0, articles before the first candle 0, tail without an exit bar 385, non-positive prices 0, **IMOEX entry bar missing 2757**, **IMOEX exit bar missing 2055**, samples produced 4399 (train 2855 / val 1544). 96.5% of the missing index dates are weekends (Saturday 2347, Sunday 2300 of 4812): MOEX publishes weekend sessions for equities, IMOEX is not calculated, and the date lookup misses. It is a calendar-alignment bug rather than a ticker-mapping problem; dropping weekend bars before the loop would recover up to about 4647 samples.

### 7.3 Classifier trained on the 2021-2024 archive (step 2, 2026-10-18)

Training: `Kasymkhan/RussianFinancialNews` via `cmd/newsimport` (18 tickers, records before 2025-01-01): 18957 pairs, 18375 labelled samples (previously 2855). Validation: `data/news_history.jsonl`, 17 tickers, records from 2026-04-01 (7129 pairs); unchanged `cmd/trainnewsmodel` (vocab 3000, min doc freq 3), entry at the next open; `cmd/newsvalidate` scores; date-block bootstrap.

| horizon (trading days) | val samples | val dates | train AUC | val AUC | CI95 | val accuracy | majority baseline |
|---:|---:|---:|---:|---:|---|---:|---:|
| 10 (primary) | 2866 | 110 | 0.689 | 0.528 | [0.479, 0.574] | 0.540 | 0.547 |
| 5 | 4019 | 105 | 0.699 | 0.486 | [0.425, 0.538] | 0.519 | 0.543 |
| 1 | 4488 | 135 | 0.682 | 0.516 | [0.448, 0.579] | 0.548 | 0.571 |

Criterion (val AUC >= 0.530 at 10 days, lower bound above 0.5): not met. Six times more training data left validation AUC at 0.49-0.53; the train-validation gap is overfitting to vocabulary. Post-hoc: the lowest score quintile has mean 10-day excess return -1.04% and positive share 0.406 against -0.09..-0.22% and 0.452-0.485 for the other quintiles, a one-sided negative-news pattern needing its own pre-registration.

### 7.4 Domain-shift test (step 2b)

7054 unique 2026 pairs on 17 tickers; train before 2026-08-01 (3023 pairs), 14-day gap, test from 2026-08-15 (3709 pairs, no title seen in train). Arms: A trained on 2026 only; B the section 7.3 classifier (never saw 2026, control); C both. CI at the Bonferroni level 99.17%.

| horizon | arm | train samples | test samples | test dates | AUC | CI 99.17% |
|---:|---|---:|---:|---:|---:|---|
| 10 | A | 1187 | 1427 | 38 | 0.4396 | [0.371, 0.512] |
| 10 | B | 18375 | 1427 | 38 | 0.6132 | [0.534, 0.678] |
| 10 | C | 19562 | 1427 | 38 | 0.5823 | [0.514, 0.647] |
| 5 | A / B / C | 1603 / 18375 / 19978 | 2168 | 39 | 0.4703 / 0.4797 / 0.4693 | lower bounds 0.388 / 0.355 / 0.352 |
| 1 | A / B / C | 1328 / 18375 / 19703 | 2817 | 49 | 0.5438 / 0.5338 / 0.5424 | lower bounds 0.461 / 0.410 / 0.421 |

No arm qualifies (arm C at 10 days has 38 test dates against the required 40). Training on 2026 text does not help (A is below chance at 10 days with train AUC 0.87) and the control B scores as high as C: domain shift is not supported. Month by month, B at 10 days gives AUC 0.459 (April), 0.407, 0.579, 0.373, 0.502, 0.614 (September), 0.753 (October): the 10-day figure is a September-October window effect, the same time instability seen in every test of this series.

### 7.5 Expanded 260-ticker archive (step 2c)

Archive after the Telegram backfill and cleaning: 153151 pairs, 260 tickers. Filters as registered: the `MOEX` website-news source excluded, articles mapped to four or more tickers excluded (also applied to the test file, removing 27938 of 72132 test pairs), training pairs before 2025-01-01 (63581 pairs, 40413 samples after the liquidity filter), test from 2025-03-01 (31 pairs with a headline seen in train removed), median 20-day turnover at least 10 million RUB. CI at the Bonferroni level 98.33%.

| horizon | test samples | test dates | AUC | CI 98.33% | quarters with AUC >= 0.5 (of 6) | within-date permutation AUC |
|---:|---:|---:|---:|---|---:|---:|
| 10 | 19085 | 417 | 0.5234 | [0.5022, 0.5424] | 4 | 0.5021 |
| 5 | 23095 | 381 | 0.5212 | [0.5007, 0.5421] | 5 | 0.5016 |
| 1 | 23766 | 471 | 0.4963 | [0.4735, 0.5196] | 4 | 0.4917 |

Quarterly AUC at 10 days: 0.564, 0.520, 0.477, 0.534, 0.592, 0.488. By source at 10 days: Telegram 0.5238 (17153), Google 0.5442 (1796). Criterion (AUC >= 0.530 with lower bound above 0.5, plus control, at least 4 of 6 quarters): not met at any horizon. Descriptively the 10- and 5-day intervals now lie above 0.5 (effect about +0.02 AUC), unstable across quarters, carried by Telegram posts and measured on non-independent pairs (reposted wires), so the interval is optimistic and no economic claim follows.

### 7.6 News score net of costs and decay (steps 2g and 2, seven hypotheses)

Ticker-day score = mean article score; per date with at least 10 scored tickers (226 dates, 2025-03-17..2026-10-05), long the top quintile and short the bottom, 10-day hold, one tranche per date, 0.15% per fill (0.60% per tranche), no borrow, date-block bootstrap (block 10, 5000 reps).

| quantity | value |
|---|---|
| gross long-short spread per tranche | +0.863% |
| break-even cost per side | 0.216% |
| net at 0.15% / 0.10% / 0.20% per side | +0.263% [-0.305, +0.930] / +0.463% [-0.105, +1.130] / +0.063% [-0.505, +0.730] |
| long-only top quintile net of 0.30% round trip | +0.045% [-0.258, +0.350] |
| quintile mean 10-day excess, low to high score | -0.518, -0.385, -0.012, +0.206, +0.280 (%) |
| quarterly net (8 quarters) | -0.23, +0.48, -0.55, -0.74, -0.23, +2.40, +0.48, -0.25 (%) |

The criterion (lower CI bound above 0) fails at every cost level; five of eight quarters are negative and 2026Q2 (+2.40%) carries the mean; the spread comes from the short leg, which is not tradable here.

Decay by horizon (retrained classifier, val AUC 0.518; 435 dates with at least 10 scored tickers in 2025-03-01..2026-09-30; rank-weighted cross-sectional portfolio, gross of costs, excess over IMOEX):

| h (days) | gross spread % per tranche | CI95 | per day % |
|---:|---:|---|---:|
| 1 | +0.0944 | [+0.0210, +0.1706] | +0.0944 |
| 2 | +0.1131 | [+0.0123, +0.2141] | +0.0565 |
| 5 | +0.2116 | [+0.0700, +0.3603] | +0.0423 |
| 10 | +0.4258 | [+0.2117, +0.6627] | +0.0426 |
| 20 | +0.5401 | [+0.2352, +0.8645] | +0.0270 |

H1 (gross spread at 5 days above 0): met (gross only; at 10 days +0.4258% is below the 0.60% tranche cost). Front-loading is not established (per-day difference h1 minus h10 +0.0518, CI [-0.0270, +0.1308]). H4 liquidity: least liquid tercile +1.2117 [-0.2371, +3.1680] on 33 dates, middle +0.3495 [+0.0017, +0.6955], most liquid +0.3774 [+0.1508, +0.6342]; low minus high +0.8343 [-0.6336, +2.7335], not supported. H4 sign: bottom quintile mean excess -0.9441 [-1.5271, -0.4061], top +0.2120 [-0.1156, +0.5333], asymmetry +0.7321 [+0.0924, +1.3906]: the negative-news side carries the effect, and the long (tradable) side is indistinguishable from zero. The news direction stays closed as a trading signal; reopening needs an independent period after 2026-10-05.

### 7.7 Zero-shot LLM direction on Russian headlines (steps 1 and H3, final 2026-10-19)

`cmd/llmdirection` (qwen3.8, digest `3f181bb5737e`, temperature 0, no thinking, JSON answer direction plus confidence 1..5). Sample A (clean): 1200 articles over 157 dates, 2026-04-01..09-30; sample B (pre-cutoff, lookahead test): 1114 articles over 399 dates, 2023-01-01..2024-06-30; 0 unparsable answers. Label: sign of excess return over IMOEX, entry at the next open.

| sample | h | AUC | CI95 | AUC without neutral (n) | Spearman with prior 5-day return | AUC within return terciles |
|---|---|---|---|---|---|---|
| A | 1 | 0.5111 | [0.4793, 0.5407] | 0.5325 (520) | 0.263 | 0.5009 |
| A | 5 | 0.5272 | [0.4941, 0.5597] | 0.5598 (520) | 0.263 | 0.5198 |
| A | 10 | 0.5260 | [0.4921, 0.5605] | 0.5519 (520) | 0.263 | 0.5156 |
| B | 1 | 0.5290 | [0.4987, 0.5599] | 0.5525 (398) | 0.123 | 0.5283 |
| B | 5 | 0.5465 | [0.5170, 0.5765] | 0.5996 (398) | 0.123 | 0.5427 |
| B | 10 | 0.5524 | [0.5232, 0.5821] | 0.6060 (398) | 0.123 | 0.5469 |

H2 (AUC >= 0.55 with lower bound above 0.5): fails on sample A at h = 1 and h = 10; the claim "zero-shot LLM direction on Russian headlines is not better than random" is recorded and H6 and H9 are closed without a run. H3 (lookahead): B minus A at h = 10 is +0.0264 [-0.0175, +0.0695]; the registered leakage rule (+0.030 with lower bound above 0) is not met, so no lookahead is detected, yet sample B alone is above chance at h = 5 and 10, so the old archive is not cleared for calibration use. Part of the signal is trailing momentum (rho 0.263 on A, below the 0.3 limit).

### 7.8 Event causal study: sanctions, negotiations, incidents (2026-10-09)

Protocol `docs/plans/completed/20261009-event-causal-study.md`. Panel: 17 tickers x 140 trading days (2026-04-01..10-09), 10213 archive records, abnormal return against IMOEX (market model beta fitted on 2025-10-01..2026-03-31; `excess` mode as robustness), 2000 date-bootstrap and 1000 placebo draws, seed 42. No P&L is involved.

| topic | events / dates | stacked DiD CAR[0,+1] [95% CI] | AIPW ATT (t) | placebo p | Holm p | MDE | verdict |
|---|---|---|---|---|---|---|---|
| sanctions | 71 / 45 | -0.0018 [-0.0094, +0.0045] | +0.0011 (0.35) | 0.675 | 0.675 | 0.0100 | not supported |
| negotiations | 58 / 37 | -0.0051 [-0.0124, +0.0015] | -0.0002 (-0.05) | 0.337 | 0.673 | 0.0098 | not supported |
| incident | 12 / 9 | -0.0230 [-0.0662, +0.0074] | -0.0238 (-1.42) | 0.026 | 0.078 | 0.0518 | underpowered |

Sanctions and negotiations show no detectable effect (minimum detectable effect about 1.0% over two days; consistent with the AUC rejection of the topic signals, section 6.9). The incident direction matches the live BUY veto (raw CAR[0,+1] -2.6% [-6.7%, -0.2%]) but N = 12 on 9 dates fails the power floor; the veto stays as defence in depth, revisit at 30 or more live triggers. Pre-trends are clean for all three; TWFE and stacked DiD agree in sign and size; `excess` mode reproduces every verdict. Limits: six months, one regime, generic-news ticker attribution biasing toward zero (VKCO 41% of records), timestamps that lag the first reaction, no instrument for IV.

### 7.9 Level-crossing event study: stops and take-profits (H20, 2026-10-15)

Do prices behave differently after crossing a level where stop and take-profit orders plausibly cluster (20-day extremes, round numbers) than after an ordinary level at the same distance? Criteria and placebo fixed in `docs/plans/20261015-hypotheses-from-llm-review.md`. Data: 10-minute ISS bars for 16 basket tickers, 2024-11-15..2026-10-11, main session, gaps excluded, 12213 events (5784 real, 6429 placebo). Outcome: signed abnormal return against the basket divided by the hour-of-day robust sigma (positive = continuation); difference = real minus placebo with a 97.5% day-bootstrap CI.

| level | placebo | horizon | real signed z | placebo signed z | diff (CI97.5) | real mean bp |
|---|---|---|---:|---:|---|---:|
| 20-day extreme | 4th extreme | 10m | +0.164 | -0.026 | +0.190 [-0.014, +0.410] | +1.8 |
| 20-day extreme | 4th extreme | 1h (primary) | +0.048 | -0.125 | +0.173 [+0.034, +0.317] | +1.3 |
| 20-day extreme | 4th extreme | 4h | +0.024 | -0.010 | +0.034 [-0.103, +0.163] | +1.2 |
| round number | half-step grid | 10m | -0.019 | +0.109 | -0.127 [-0.281, +0.008] | -0.4 |
| round number | half-step grid | 1h (primary) | -0.063 | -0.054 | -0.009 [-0.105, +0.080] | -1.4 |
| round number | half-step grid | 4h | -0.047 | -0.040 | -0.008 [-0.086, +0.068] | -1.8 |

The primary extreme test meets the statistical criterion (CI excludes zero, both halves positive, 1368 events on 424 dates) but fails the economic one (+1.3 bp against a 30 bp round-trip hurdle) and the difference comes from the placebo level reversing (-2.8 bp). The share of crossings returning through the level within 30 minutes is 0.471 real vs 0.473 placebo: no stop-run signature. **Extension** (six primary tests, 99.2% CI): fine rounding (basket) -0.005 [-0.023, +0.012]; SMA200 vs SMA173 +0.062 [-0.260, +0.379]; illiquid 38 names: extreme +0.101 [-0.080, +0.297], round 1-10% -0.027 [-0.099, +0.046], fine round +0.011 [-0.005, +0.026] (both arms reverse about 13-17 bp at 1h, most likely bid-ask bounce, far below the 100 bp hurdle); news reversal (first-hour extreme crossing, 93 crossed of 1203) +0.100 [-0.816, +1.010], inconclusive (needed at least 100 crossed events). No test rejects the null and none reaches the cost hurdle; no entry signal and no protective filter is justified. Order-book tests need an AlgoPack subscription or our own snapshot collector.

### 7.10 News burst as a risk regime (H22, 2026-10-17)

Decisions from the six `wf-volscale` windows since 2026-04-01 (1492 BUY/SELL decisions with a 10-day forward). Burst = at least 3 articles on the ticker in the 24 hours before 10:00 MSK of the decision date. Bootstrap by ISO week, 2000 draws, CI 97.5%.

| test | burst n (dates) | burst mean | rest n | rest mean | difference | CI 97.5% |
|---|---|---:|---|---:|---:|---|
| signed 10-day return | 304 (113) | +0.34% | 1188 | +0.75% | -0.41% | [-2.20%, +1.33%] |
| worst close within 10 days | 304 (113) | -4.15% | 1188 | -3.78% | -0.37% | [-1.71%, +0.92%] |

Both intervals contain zero: criterion not met (the sample requirement is). Thresholds of 1, 2 and 5 articles, BUY-only and SELL-only splits all contain zero; the point estimates lean toward a worse outcome after a burst but are far inside the noise. No news-based filter is justified (five and a half months, one regime, counts without sentiment sign).

### 7.11 Dividend capture event study (2026-09-25), rejected

Pre-registered protocol `docs/plans/completed/20260925-event-driven-dividend-drift.md`; `cmd/dividendstudy`; calendar `data/dividends.jsonl` (123 events 2021-2027 from T-Invest `GetDividends`, 65 in the study window 2024-01-01..2026-12-19, 9 waves). Position 30000 RUB, costs 0.05% per leg, exit at the ex-date close; mean excess return over IMOEX by entry offset (trading days before the last buy date), bootstrap over whole waves (seed 42, 2000 repeats), holding tax 0.13 (worst case) or 0.00:

| offset | excess (tax 0.13) | CI (tax 0.13) | excess (tax 0.00) | CI (tax 0.00) |
|---|---|---|---|---|
| -10 | +0.0050 | [-0.0149, +0.0198] | +0.0114 | [-0.0083, +0.0263] |
| -5 | -0.0027 | [-0.0165, +0.0104] | +0.0037 | [-0.0102, +0.0168] |
| -3 (primary) | -0.0063 | [-0.0223, +0.0097] | +0.0001 | [-0.0165, +0.0165] |
| -2 | -0.0034 | [-0.0124, +0.0067] | +0.0030 | [-0.0066, +0.0137] |
| -1 | +0.0006 | [-0.0052, +0.0079] | +0.0071 | [+0.0008, +0.0151] |

The mean gap ratio is -0.97: the ex-date open sits about one full dividend below the last-buy close, so the drop is almost fully priced. Acceptance needed a contiguous subset containing {-2, -3, -5} with the CI excluding zero and leave-one-out stability under both tax conventions; all fail, and only offset -1 (tax 0.00) excludes zero, outside the registered subset. **Rejected**: dividend capture is not deployable; the income lever reverts to position scaling under an exposure cap, which sections 5.6-5.7 then closed. Detail: `docs/reports/dividendstudy-report.md`.

## 8. Portfolio construction and cross-sectional signals

### 8.1 VAR-ICA-MGARCHSK portfolio formation (H23 stage 0, signal-free)

Method of Mikhaylov et al. (Finance: Theory and Practice 2026, 30(4)); `cmd/icaportfolio` on `internal/icaport`. Universe: 16 tickers with at least 740 of 750 days, ISS daily price returns, 750-day estimation window, rebalance every 21 trading days, 99 rebalances 2019-01-02..2026-10-07, long-only, 0.05% cost on turnover; least-squares moment recursions, multi-start projected gradient instead of the genetic algorithm; stationary block bootstrap (block 20, 2000 repeats).

| rung | ann. return | ann. vol | CVaR95 (daily) | max drawdown | Sharpe | turnover per rebalance |
|---|---:|---:|---:|---:|---:|---:|
| R0 equal weight (control) | 0.65% | 24.99% | 3.785% | 53.81% | 0.15 | 0.070 |
| R1 inverse volatility 63d | 0.55% | 24.19% | 3.675% | 53.24% | 0.15 | 0.125 |
| R2 minimum variance, sample covariance | -2.68% | 23.58% | 3.529% | 63.28% | 0.00 | 0.116 |
| R3 VAR-ICA-GARCH, variance only | 0.30% | 26.17% | 3.834% | 59.35% | 0.14 | 1.010 |
| R4 full VAR-ICA-MGARCHSK, rho 1 | -5.76% | 26.84% | 3.952% | 74.28% | -0.09 | 0.851 |
| R4 with rho 5 (descriptive) | -1.31% | 28.19% | 4.276% | 65.82% | 0.10 | 1.398 |
| R4 with mu 0 (descriptive) | -8.46% | 23.66% | 3.672% | 77.21% | -0.25 | 0.239 |

CVaR95 ratios against equal weight (CI95): R1 0.971 [0.961, 0.979], R2 0.932 [0.884, 0.984], R3 1.013 [0.955, 1.076], R4 1.044 [0.976, 1.121]; R4 / R3 1.031 [0.983, 1.089], R4 / R2 1.120 [1.064, 1.180]. Criterion (R3 or R4 at least 15% lower CVaR with upper bound below 1, at least 15% lower drawdown, return not more than 1pp below; R4 must beat R3 and R2): not met. **H23 rejected for this universe**; nothing is wired in. The noisy VAR(1) mean term drives the weights (turnover about 100% for R3, 85% for R4 against 7%; 25% with the mean off); without it the ICA variance forecast is indistinguishable from the sample-covariance minimum-variance portfolio (R3 with mu 0 vs R2: CVaR ratio 1.000 [0.979, 1.023]); higher moments make tail risk worse (R4 / R3 drawdown 1.25).

### 8.2 IMOEX regime mask on new BUY decisions (steps 6 and 10)

`cmd/regimemask` rewrites persisted walk-forward probabilities into a `csvprob` file removing BUY decisions (p >= 0.60) on risk-off dates; R1 = IMOEX 1-day return <= -2.0%, R2 = 5-day return <= -5.0%.

**Step 6**, six `wf-volscale` windows (reproduction check passed: unmasked +59539.41, -0.5% against the control):

| variant | risk-off dates / suppressed BUY | realized per window q1 q2 q3 q4 w2 w1 | sum realized | worst window max DD |
|---|---|---|---:|---:|
| none | 0 / 0 | 4002, 53, 34089, -12339, 2676, 31057 | +59539.41 | 3.56% |
| R1 | 27 / 168 | 4102, 53, 32505, -12339, 2705, 31645 | +58671.15 (98.5%) | 3.58% |
| R2 (secondary) | 27 / 318 | 6942, 53, 28046, -12339, 2676, 50807 | +76184.13 (128%) | 3.49% |

The registered primary (R1: realized at least 0.80 of control, worst-window drawdown at most 0.90 of control) fails on drawdown (ratio 1.006). R2 raises realized in two windows (w1 is 26k of the 28k gain) and lowers w1 max DD 3.05% -> 1.82%, but was not the registered primary.

**Step 10**, four unseen windows (2024-03-05..2025-03-04; models trained since 2021): control realized p1 -1062.31, p2 -2786.11, p3 -885.12, p4 -1347.74 (p4 fails the reproduction check and is excluded; its kill switch tripped, DD 8.74%). Risk-off dates R1 21, R2 17; suppressed BUY R1 7, R2 1. R2 is **not evaluable** (1 suppressed BUY across the valid windows; realized sum -4733.5 and worst DD 1.86% identical to the control). The 2024 models lose money in every window, so there is little long exposure to protect. No `internal/risk` pre-registration for an IMOEX regime gate; reopening needs a long-biased active book.

### 8.3 Cross-sectional ranking of OOS ensemble probabilities (step 7)

`cmd/rankspread -wf-dir wf-volscale`: six windows, 17 tickers, 7689 decisions, 458 dates, rank-weighted portfolio per date, excess over IMOEX from the next open, cost 0.15% per side (0.60% per tranche).

| h | dates | gross spread % | CI95 | net of 0.60% | net CI95 |
|---:|---:|---:|---|---:|---|
| 1 | 458 | +0.0143 | [-0.0865, +0.1214] | -0.5857 | [-0.6865, -0.4786] |
| 5 | 458 | +0.0629 | [-0.1953, +0.3483] | -0.5371 | [-0.7953, -0.2517] |
| 10 | 458 | -0.0132 | [-0.4230, +0.4238] | -0.6132 | [-1.0230, -0.1762] |

At h = 10 the bottom probability quintile earns -0.0649 [-0.9010, +0.7262] and the top -0.4831 [-1.0514, +0.1110]. The order of the deployed recipe's probabilities inside a date carries no cross-sectional information (consistent with section 5.2): criterion not met and the gross spread is indistinguishable from zero at every horizon; no rank-objective training is justified.

### 8.4 Cross-sectional factors on the whole board (part C, steps 18-21 and corrections)

Code `scripts/factors_wide.py`; `cmd/pricehist` and `cmd/tqbrsnap` (881385 daily snapshot rows of all 719 securities that traded on TQBR 2014-2026, including delisted). Costs 0.15% per side, borrow 0.005% per day on the short leg, entry at the next open, the 2022-02-25..03-24 halt excluded; one-day moves below -45% or above +80% are treated as splits. The first run used today's 133 most liquid names and price returns (survivorship and look-back liquidity bias). Three corrections, each found by inspection before any decision: total return with dividends from the T-Invest calendar (2152 events; the ISS endpoint is unavailable), removal of an availability look-ahead (names without an exit price carry the last close), and a **point-in-time universe** (252 bars of history and 60-day median traded value at least 30 million RUB on the date; 57.6 names per period). Known gaps: T-Invest resolves only 186 of 587 old tickers (dividends missing for most delisted names), dividends above 35% of the previous close dropped as unit mismatches, no corporate-action correction beyond the split rule.

**Step 18 on the point-in-time universe, total return** (monthly = 21 bars, 128 periods 2016-2026; ML from 2019). Registered rule: four variants, Bonferroni t >= 2.5, Sharpe >= 0.5, at least 60% positive years, positive after 2022-06.

| variant | net mean per period | NW t | net Sharpe | years positive | post 2022-06 mean (52 periods) | max drawdown | verdict |
|---|---|---|---|---|---|---|---|
| V1 composite long-short quintile | +0.01% | 0.01 | 0.00 | 7/11 | +1.23% | -72% | fail |
| V2 composite long minus universe | +0.40% | 1.54 | 0.47 | 6/11 | +0.89% | -22% | fail |
| V3 LightGBM long-short | +0.68% | 1.27 | 0.42 | 6/8 | +1.20% | -39% | fail |
| V4 LightGBM long minus universe | +0.58% | 1.94 | 0.64 | 6/8 | +0.82% | -17% | fail (t only) |
| V1 at 10-bar rebalancing | +0.12% | 0.48 | 0.15 | 7/11 | +0.40% | -52% | descriptive |
| V2 at 10-bar rebalancing | +0.14% | 0.99 | 0.32 | 6/11 | +0.25% | -38% | descriptive |

On the survivor-only panel the same variants gave higher t (V3 2.29, V4 2.25, V1 1.78, V2 1.69); the point-in-time universe lowers every t-statistic. Descriptive attribution (long-short quintile, net, full sample): mom_12_1 +1.24% per period (t 2.17), high52 +1.77% (t 2.99), low_vol -0.32% (t -0.55), low_max -0.85% (t -1.88), **rev_1m -2.69% (t -5.18, zero positive years of 11)**: one-month returns continue rather than reverse on MOEX, so the registered composite, which treats rev_1m as a reversal signal, worked against the strongest signal in the data.

**Step 19, preferred-common pairs** (6 pairs with both legs >= 1000 bars: MTLR, PMSB, RTKM, SBER, SNGS, TATN; 2016-2026): net mean +0.0058% per day, NW t 0.77, Sharpe 0.24, positive in 8 of 11 years, max drawdown -25.8%, total +14.8%; per-pair t 0.12, 1.23, 0.29, 0.31, -0.39, 1.32. Registered rule (t >= 2.0, Sharpe >= 0.7): fail; the spread converges too slowly for the 0.6% four-leg cost.

**Step 20, overnight against intraday** (point-in-time, total return): overnight +0.102% per day (NW t 12.1), intraday -0.058% (t -2.36); overnight is positive in every year, intraday negative since 2024. A buy-the-close/sell-the-open strategy needs a gross daily mean of at least 0.30%; not tradable at these costs.

### 8.5 Sequential test of a hand-set composite (step 23)

Rule fixed in `81b2112`: composite C2 = mean of z(mom_12_1), z(1-month momentum = -rev_1m) and z(high52); signs and the drop of low_vol and low_max chosen on 2016-2021 data of the survivor panel; test 2022-06-01..2026-10-18 (52 periods, point-in-time universe).

| arm | test net mean per period | NW t | Sharpe | net sum by year | verdict |
|---|---|---|---|---|---|
| C2 long-short | +1.17% | 1.42 | 0.56 | 2022 (Jun-) -27.7%, 2023 +0.2%, 2024 +47.4%, 2025 +15.2%, 2026 +26.0% | fail (t >= 2.0 not met) |
| C2 long minus universe | +0.83% | 1.94 | 0.77 | 2022 -4.8%, 2023 +1.7%, 2024 +25.3%, 2025 +15.7%, 2026 +5.0% | descriptive |

On the survivor panel the same rule gave t 3.67, a product of the universe. Reference period 2016-2021 (where the signs were chosen, not out of sample): +2.49% per period, t 3.10, Sharpe 1.31, 6 of 6 years positive. Funds excluded: figures unchanged. Long-only books beat the universe but not the key-rate carry in 2024-2026 (high52 top quintile 2024 +4.4% against universe -16.7% and carry +19.1%; 2025 +8.2%, -6.7%, +20.9%; 2026 to October -7.0%, -20.8%, +11.5%). Short-borrow sensitivity of C2 long-short (net, after 2022-06): 0.005% per day +1.17% (t 1.42), 0.03% per day (10.9% per year) +0.42% (t 0.51), 0.06% per day -0.49% (t -0.59); the long-short result needs a borrow cost below about 5% per year to be positive after 2022.

### 8.6 Ridge and MLP against LightGBM (step 25, 2026-10-19)

Same data, features (five signals and their cross-sectional ranks), target (cross-sectional rank of the next 21-bar return), expanding window from 2019-01 with refits every 63 days, 92 monthly periods to October 2026, costs and universe as in section 8.4, total return. Family of six models, registered threshold t >= 2.6.

| model | arm | net mean per period | NW t | net Sharpe | years positive | post 2022-06 mean | max drawdown | gross t |
|---|---|---|---|---|---|---|---|---|
| ridge (alpha 10) | long-short quintile | +1.73% | 2.54 | 0.88 | 7/8 | +1.74% | -38% | 3.16 |
| ridge | long minus universe | +1.07% | 3.35 | 1.11 | 6/8 | +1.19% | -16% | 3.81 |
| MLP (32-16, five seeds) | long-short quintile | +0.49% | 1.05 | 0.35 | 5/8 | +0.82% | -22% | 2.20 |
| MLP | long minus universe | +0.42% | 1.49 | 0.54 | 6/8 | +0.71% | -14% | 2.17 |
| LightGBM (V3 / V4, same period) | long-short / long minus universe | +0.68% / +0.58% | 1.27 / 1.94 | 0.42 / 0.64 | 6/8 | - | - | - |

**Ridge long minus universe passes all four registered conditions**; ridge long-short misses on t (2.54); both MLP variants and LightGBM fail. The linear model with weights learned only from past data learns the positive weight on one-month momentum that the hand-set composite had with the wrong sign (the existence of that inverted signal was known from the section 8.4 attribution, so the result is not fully clean). Net sums by calendar year, long minus universe: 2019 +4.6%, 2020 -1.6%, 2021 +26.5%, 2022 (Jun-) -6.0%, 2023 +18.7%, 2024 +20.9%, 2025 +18.4%, 2026 +16.7%; long-short: +3.0%, +3.0%, +45.3%, -25.4%, +31.9%, +49.9%, +16.2%, +35.5%.

Descriptive, computed after seeing the result: the ridge top quintile (long only, net, compounded) returned +180% over 2019-2026 against +1% for the equal-weight universe, but in 2024-2026 it beat the universe by 20 points per year while earning less than the key rate (2024 +4.6% vs carry 19.1%; 2025 +13.3% vs 20.9%; 2026 -3.8% vs 11.5%). The long book minus the IMOEX price-index return (a short future without basis, roll or contract-size effects) is +1.36% per period (t 3.99, Sharpe 1.32, positive every year, post-2022 +1.33%, t 2.84); the earlier real-futures test (section 5.4) found whole-contract sizing and beta mismatch destroyed a daily alpha on a 1M account. Ridge long-short under higher borrow: 0.005% per day t 2.54 (post-2022 1.65), 0.03% per day t 1.42, 0.06% per day t 0.07. Long turnover 93% of the long leg per month; about 65 eligible names per month, 13 in the top quintile; capacity, mid-cap preferred liquidity and execution at the open are not modelled. The ridge track was added to the forward recorder (`~/moex-shadow`) before its first rebalancing record; the registered forward criterion is in the plan (step 22 addenda).

### 8.7 Universe model: decomposition, lead-lag, regimes, macro (step 26)

Code `scripts/world_model.py`, data `scripts/world_fetch.py`; same panel and costs as section 8.6 (a first run had a data defect in the FX series after 2024-06-11 and was replaced).

**M1, out-of-sample decomposition of daily ticker returns** (walk-forward by year, 504 training days, test 2019..2026, 150-220 tickers per year), pooled R2:

| model | R2 2019-2026 | R2 2023-2026 |
|---|---|---|
| A market (IMOEX) | 0.116 | 0.107 |
| B market + 5 statistical factors | 0.245 | 0.283 |
| C market + macro (USDRUB/CNYRUB, Brent, VIX, UST10, RVI, key-rate change, SPX lag) | 0.091 | 0.092 |
| D B + C | 0.223 | 0.271 |

Macro adds nothing out of sample (incremental R2 negative every year, -0.005 to -0.083); the market explains 11-12% of daily variance, the common structure about 28% in recent years, about 72% is idiosyncratic. The five statistical factors do not map stably to named sectors.

**M2, drivers of IMOEX next-day return** (1947 test days): R2 against the expanding mean -1.0% (fail), sign hit rate 53.4% (one-sided p 0.0016, pass), NW t of the net excess over buy-and-hold 1.76 (needs 2.6, fail), post-2022-06 excess +0.05% per day (t 1.35, pass); Sharpe 0.07 against -0.50, max drawdown -29.5% vs -70.5%; 2022 dominates. **Fail** (2 of 4). **M3, two-state Markov-switching variance, hold IMOEX unless P(high variance) >= 0.5 else key-rate carry:** Sharpe -0.34 vs -0.51 (difference +0.17, needs 0.2), drawdown -48.9% vs -71.6% (ratio 0.68, pass), post-2022-06 difference -0.12 (fail). **Fail.** **M4, macro-conditioned stock signal added to ridge** (91 periods): ridge baseline reproduces +1.07% (t 3.35); ridge + macro +1.04% (t 3.26, Sharpe 1.10, 8 of 8 years positive); paired difference -0.011% (t -0.04, needs 2.0); macro alone -0.52% (t -1.71). **Fail.**

**M5, descriptive** (2021-2024, 134 tickers, 102642 ticker-days): a news item exists on 21.3% of ticker-days, on 40.2% of days with |z| > 3 (2032 events) and 19.6% of quiet days; 59.8% of large idiosyncratic moves have no archive item (the archive crawls a few publishers, so absence is not evidence of no cause).

Verdict: no tested model of the universe predicts out of sample beyond the existing ridge; the measurable result is the decomposition (market 12%, common structure up to 28%, macro 0 out of sample, about 72% idiosyncratic of which about 40% coincides with ticker news).

### 8.8 Gate-passing candidate search (step 27)

Replay: 17 tickers, 1M deposit, 15000 RUB per position, costs 0.0005 x 3, net caps 90000, borrow 0.00005 per day, thresholds 0.60/0.40, csvprob; ridge scores from `scripts/ridge_probs.py` (percentile r in the full eligible universe, p = 0.5 + (r - 0.5)/3, held 21 bars). The live sandbox profile reads this table (`model.score_table_path`, `data/ridge_scores.csv`).

- **27, ridge long-short**, realized net of borrow: q1 -2, q2 +1136, q3 +407, q4 +2113, w2 -274, w1 -2697, G +3916 (81 closed). Positive windows 3 of 6 (needs 4): fail; controls: bias variant G +2077, flipped sign G -4491 with a negative six-window sum.
- **27b, project recipe** (5 seeds, 18 tickers from 2024-01-01) with and without the ridge agreement filter: base six-window sum +1681 (seeds -13842..+17461), G -11884 (all seeds negative, -19440..-4291); filtered -6688 and G -3303. The recipe fails the gate window for every seed (consistent with the preflight error).
- **27c, 34 rolling 90-day windows every 15 days** (2025-03-05..2026-07-13, gate rule realized > 1 RUB with at least one closed trade): ridge long-short passes 20 of 34 (59%), sum +10171, min -4125, max +3324; flip passes 5 of 34 (15%), sum -31076. Windows starting 2026-05-14..06-28 are negative (-1366, -3615, -4125, -2677). Criterion (pass share >= 70%): 59% fail.

The ridge signal has a real sign (the flip loses systematically) and passes the current gate window, but at 15000 RUB per position its P&L is of the order of the +1 RUB threshold with a pass rate near 60%, so a green gate on a given day is close to a coin flip with a positive tilt. No candidate meets the registered bar; the ensemble artifact, gate and config are unchanged.

### 8.9 Report events and dividend history (step 28)

`cmd/reportsfetch` (T-Invest, history from 2014-03): 289 of 719 point-in-time tickers resolve (delisted names do not, so event studies are survivor-biased), 3811 report dates, 1811 dividend records; after eligibility, 2016+ and excluding 30 days around the 2022 halt, 1448 events. The AlgoPack key is rejected ("subscribers only"), so no order-book or tradestats tests.

- E1: mean abnormal return [0,+1] -0.20%; mean |abnormal| on report days 1.55% vs 1.24% (ratio 1.25).
- E2 premium [-2,+1]: -0.03%, clustered t -0.26, 4 of 9 years positive: fail.
- E3 reaction continuation (top minus bottom tercile of the [0,+1] reaction, drift [+2,+21]): +0.98% (top +0.20%, bottom -0.78%, 483 events each), clustered t 2.21, 5 of 9 years: fail (direction as in the literature).
- E4 ridge long book excluding names with a report in the next 21 bars: net +0.82% (t 2.14, Sharpe 0.75) against the unfiltered +1.07% (t 3.35, Sharpe 1.11); paired -0.25% (t -1.09): fail (the flag uses actual report dates, so any gain would be optimistic).
- E5 trailing dividend yield quintile long minus universe: -0.52% per period (t -1.87, 2 of 11 years): fail; ridge plus yield features +0.85% (t 2.50), paired -0.22% (t -1.44): fail.

All blocks fail: report timing and dividend yield add nothing to the ridge signal. Reaction-based drift (E3) stays a candidate once more events exist.

### 8.10 Futures open interest with the AlgoPack key (step 29)

`cmd/futoihist`: 25 futures series, snapshots 18:45 and 23:50, FIZ and YUR groups, 126204 rows, history from 2020-01 (the free ISS gave 2023-12 with a 14-day delay). F1 (MX retail net-position change, contrarian position on IMOEX, 5-day non-overlapping, 221 periods from 2022-06): difference vs buy-and-hold +0.06% per period, NW t 0.20, hit rate 48.9%, halves -0.26% / +0.38%: fail (23:50 snapshot t 1.32, hit 48.2%). F2 (stock futures, pooled panel of up to 17 names, abnormal return t..t+5 on the z of the 5-day change in retail net share, 3657 observations): coefficient -0.0004, clustered t -0.52, halves t 0.01 and -0.91: fail. Retail/institutional positioning in FORTS carries no usable 5-day forecast; do not redo with the same features.

### 8.11 Intraday candles, calendar effects and cycles (step 30, 2026-10-19)

Pre-registered (criteria fixed in `docs/plans/20261018-seven-hypotheses.md`, commit `320fffc`). Data: 10-minute ISS TQBR candles 2011-12-15..2026-10-16 for 29 companies (5186545 main-session bars, 100823 ticker-days), S&P 500 (^GSPC) 24814 daily closes since 1927-12-30, IMOEX 7277 days since 1997-09-22. Scripts `scripts/intraday_study.py`, `scripts/calendar_cycles.py`, `scripts/intraday_followup.py`.

**Block I** (t >= 2.6): I1 intraday momentum coefficient -0.016 (t -1.18), strategy -0.103% per day: fail. I2 overnight +0.074% per day (t 7.07), intraday -0.060% (t -3.10), difference +0.133% (t 6.90; halves t 2.21 and 7.04): existence criterion passed, but buy-at-close/sell-at-open nets -0.026% per day (t -2.50) after 0.05% per side: not traded. I3 time of day: slots 16:30 (-0.0096%, t -4.27) and 18:30 (+0.0120%, t +5.83), about 1 bp per bar. I4a signed turnover of the day against the next day's abnormal return: +0.0438% per sigma (t 6.48; halves t 5.13 and 4.00): passed, but with the same-day return controlled the slope falls to +0.0167% (t 2.15) while the daily-return slope is +0.0453% (t 4.39) (most of I4a is daily momentum); long-short on the top and bottom three of F: gross +0.170% per day (t 6.79), -0.030% (t -1.21) after 0.05% per side, -0.430% after 0.15%. I4b first-two-hours turnover to the rest of the day: +0.0024% (t 0.50): fail.

**Block S** (discovery |t| >= 3.0, confirmation |t| >= 2.0; US 1953-1989 / 1990-2026 / IMOEX 1997-2026): no effects found; the classic effects are strong in the discovery period and vanish after 1990: Monday -0.210% (t -7.86) / +0.010% (0.31) / IMOEX +0.087% (1.27); turn of the month +0.130% (5.50) / +0.041% (1.44) / +0.219% (3.11); pre-holiday +0.264% (6.84) / +0.054% (1.06) / +0.118% (0.87); January +0.934% (1.10) / -0.395% (-0.57) / +1.460% (0.77); Halloween +0.980% (2.88) / +0.549% (1.48) / +2.891% (2.60), threshold 3.0 not reached. Descriptive: S&P mean annual log return by cycle year (year % 4 = 0..3) 6.6%, 4.8%, 1.9%, 11.5%; IMOEX monthly means negative May-September (-1.72% in May) and +1.8..+3.2% December-April.

**Block Y**: Y1 periodogram S&P p 0.072 (period 4.8 days), IMOEX p 0.847: fail. Y2 fixed 40/48/108-month cycles: out-of-sample R2 negative in all six pairs (-0.0009 to -0.059): fail. Y3 variance ratios (S&P z over four 25-year subsamples; IMOEX): q = 2 [-1.01, +8.99, +1.20, -3.72], +1.71; q = 21 [+1.00, +3.92, -0.67, -1.72], +0.54; q = 252 [+0.83, +2.76, -0.73, -0.76], +0.48: strong momentum only in 1953-1977, reversed after 2002; no q passes. Y4 12-month time-series momentum: difference of means after a positive versus after a negative result S&P 1928-1989 +0.658% (t 1.33), 1990-2026 +0.689% (t 0.92), IMOEX -0.654% (t -0.52); descriptive 1990-2026 CAGR 8.50% vs 8.96% buy-and-hold, drawdown -20% vs -53%, Sharpe 0.71 vs 0.58. Y5 autocorrelation of absolute returns at lags 1/21/250: S&P 0.32/0.22/0.13, IMOEX 0.38/0.25/0.09.

Verdict: no cycles or seasonality; calendar effects significant before 1990 vanish on confirmation; the overnight premium and next-day signed turnover exist but neither survives 0.05% per side costs. The live model does not change.

### 8.12 Risk-free hurdle (step 21)

Key rate 14% on 2026-10-09 (between 14% and 21% since 2025-04). Compounded daily, 1,000,000 RUB earns 95,947 RUB over 2025-04-01..2025-09-16, 180,378 RUB over 2025-09-17..2026-10-18, 293,632 RUB over 2025-04-01..2026-10-18 and 12,347 RUB over 2026-09-17..10-18. The LQDT money-market fund returned +4.59% over 2026-06-22..2026-10-18 (118 days, about 14% annualised). The realized sums of sections 6 and 8 (single windows of thousands to tens of thousands of RUB on 1M) are an order of magnitude below this carry; the deployed strategy's live sandbox equity change was -25,395 RUB over 2026-09-17..10-18 against +12,347 RUB at the key rate.

### 8.13 Research-to-live gap attribution (H-R9, 2026-10-19)

Plan: `docs/plans/20261019-research-to-live-gap-hypotheses.md`. Tool: `cmd/gapattrib` (Go port of the step-25 portfolio on `internal/gapattrib`), inputs: the ai-box snapshot `tqbr_snap.csv` (881384 rows), `dividends_pit.jsonl` and the full-universe ridge percentiles written by `scripts/ridge_full_dump.py` (94 rebalance dates). Same rules as section 8.6: quintiles of the eligible universe (60-day median turnover >= 30M RUB, 252 bars of history), 21-bar hold from the next open, cost 0.0015 per unit of turnover, borrow 0.00005 per day. Carry is the actual key rate (`data/macro/keyrate.csv`) lagged one day, accrued on deployed capital. Rungs: R0 research quintiles; R1 quintiles inside the 17 live tickers; R1b the live thresholds r >= 0.8 / r <= 0.2 on the 17 tickers with equal weight per leg (two changes against R1 at once: selection by absolute threshold and an unbalanced book); R2 the same with equal weight per name; R3 R2 at 15000 RUB per name. Validation: R0 reproduces step 25 exactly (92 periods, net mean +1.733% per period, NW t 2.54).

| rung | periods | mean longs / shorts | mean net per period (R2 per name-unit, not comparable with the other rungs) | NW t of net per deployed capital | net per deployed capital | carry per deployed capital | excess per deployed capital |
|---|---|---|---|---|---|---|---|
| R0, 2019-01..2026-10 | 92 | 12.6 / 12.6 | +1.733% | 2.54 | +0.87% | +0.66% | +0.21% |
| R1, 2019-01..2026-10 | 92 | 2.3 / 2.3 | +1.320% | 1.84 | +0.66% | +0.66% | +0.00% |
| R1b, 2019-01..2026-10 | 92 | 3.6 / 1.7 | +1.075% | 1.18 | +0.61% | +0.59% | +0.01% |
| R2, 2019-01..2026-10 | 92 | 3.6 / 1.7 | +0.108% per name-unit | 0.41 | +0.02% | +0.68% | -0.66% |
| R0, 2025-03..2026-10 | 19 | 16.4 / 16.4 | +3.096% | 3.08 | +1.55% | +1.39% | +0.16% |
| R1, 2025-03..2026-10 | 19 | 2.6 / 2.6 | +3.500% | 2.86 | +1.75% | +1.39% | +0.36% |
| R1b, 2025-03..2026-10 | 19 | 4.2 / 1.0 | +4.202% | 2.16 | +2.49% | +1.36% | +1.13% |
| R2, 2025-03..2026-10 | 19 | 4.2 / 1.0 | +2.125% per name-unit | 0.82 | +0.41% | +1.38% | -0.97% |

R3 (15000 RUB per name, mean deployed capital 78424 RUB of the 1M deposit): +16 RUB per period, +1485 RUB over 92 periods; on 2025-03..2026-10 +319 RUB per period, +6056 RUB over 19 periods, the same order as the replay sums of step 27c (section 8.8). All per-capital columns are capital-weighted (sum of net over sum of deployed capital); an earlier draft averaged per-period ratios, which inflated R2, whose capital varies from 1 to 10 name-units, and is not used. Gross return per unit of leg capital over 2019-2026, capital-weighted: R1b long +1.12%, short +0.41%; R2 long +0.78%, short -1.10%, before costs and borrow.

Reading, descriptive and not a registered criterion. Restricting the universe to the 17 live tickers costs about a quarter of the net (R0 to R1: +1.73% to +1.32%, t 2.54 to 1.84). Moving from quintiles to the absolute live thresholds with an unbalanced book (3.6 long against 1.7 short names, R1b) leaves the net per deployed capital at the level of carry (+0.61% against +0.59%) with t 1.18. Weighting every name equally instead of every leg (R2) takes gross per deployed capital from +0.80% to +0.19% at the same cost per ruble (0.20% against 0.17%), so the costs of rebalancing do not explain the step. R1b and R2 hold the same positions; they differ only in how periods and names are weighted, so the step R1b to R2 says that the periods with many simultaneous signals (large books of 8-10 names) earn little per ruble and the periods with one or two names earn much, a heavy-tailed pattern on a small sample, not a defect proven in the live rule. The conclusion is therefore limited: the 15000 RUB scale does not explain the gap (R3 earns the same per ruble as R2) and the cost per ruble is flat across R1b and R2, while the composition of the live book (selection by absolute thresholds, balance and weighting) is where the gross return is lost. In the full sample every live-form rung is at or below the key-rate carry per deployed capital; in 2025-03..2026-10 R1 and R1b are above it and R2 below, on 19 periods. Limitations: R1b and R2 differ from the live replay (no 21-bar hold bookkeeping, deadband, caps, kill switch, partial fills); the rungs are cumulative, so the order of the steps matters for how the loss is split, and the step R1 to R1b mixes two changes (selection and balance); the step from 719 to the eligible universe is not in this table (R0 already uses the eligible universe of about 65 names); the Go port differs from `scripts/factors_wide.py` only in that it has no 2016 cut-off inside the portfolio (the window is set by `-from`) and computes forward returns for all columns, never using names outside the eligible set; 19 periods are about 6 independent 90-day windows.

Rungs 4 and 5 through the replay engine (`cmd/backtest`, signal source `csvprob` on `data/ridge_scores.csv`, thresholds 0.60 / 0.40, 17 tickers, 1M deposit, `target_notional` 15000, costs 0.0005 x 3, borrow 0.00005 per day, kill switch on). The base reproduces the step-27 gate window: 2026-07-20..2026-10-18 realized net of borrow +3916.09, 81 closed trades, maximum drawdown 0.89%. Realized net of borrow in RUB over the six non-overlapping 90-day windows from 2025-03-05 and over the whole of W2 (2025-03-05..2026-10-18):

| variant | w1 | w2 | w3 | w4 | w5 | w6 | sum of 6 | trades in 6 | W2 net | W2 trades | worst window | max drawdown |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| base: caps 90000, deadband 0.05 | 0 | +1101 | +765 | +2269 | -216 | -3579 | +340 | 239 | -2788 | 205 | -3579 | 3.15% |
| no caps | -855 | +1089 | +440 | +2331 | -277 | -3951 | -1224 | 328 | +7472 | 300 | -3951 | 5.60% |
| deadband 0 or 0.3 (either, caps 90000) | 0 | +1101 | +765 | +2269 | -216 | -3579 | +340 | 239 | -2788 | 205 | -3579 | 3.15% |

The deadband (`risk.rebalance_min_deviation_pct`) has no effect in the replay: the variants with 0 and 0.3 are identical to the base to the rouble, because the deadband lives in `internal/executor/target_position.go` and the replay makes one decision per day, so rung 4 cannot be measured on the replay engine and is not measured here. The caps (rung 5) change the result in both directions: removing them raises the whole-W2 result from -2788 to +7472 and the trades from 205 to 300, but lowers the sum of the six non-overlapping windows from +340 to -1224 and takes the maximum drawdown from 3.15% to 5.60%, above the 3% budget; the two readings disagree because positions held across window boundaries are counted differently, which is a limit of six windows and not a verdict on the caps. No variant is positive in the last window (-3579, -3951).

H-R45 (short-threshold ladder) and H-R7 (position age limit) on the same replay and windows. Realized net of borrow in RUB; the "realized net of borrow" line is absent from the replay report when no short was opened, then the realized figure of closed trades is used. Short threshold in `r` (`p <= 0.40`, 0.3667, 0.35 and none), borrow 0.00005 and 0.00047 per day:

| arm | borrow per day | six windows w1..w6 | sum of 6 | W2 net | closed trades in 6 / W2 | worst window | max drawdown |
|---|---|---|---|---|---|---|---|
| r <= 0.20 (live base) | 0.00005 | 0, +1101, +765, +2269, -216, -3579 | +340 | -2788 | 239 / 205 | -3579 | 3.15% |
| r <= 0.20 | 0.00047 | -549, +780, -233, +1031, -1427, -4308 | -4706 | -11953 | 239 / 205 | -4308 | 3.49% |
| r <= 0.10 | 0.00005 | +24, +1148, +213, +3206, +43, +14 | +4649 | +2695 | 162 / 92 | +14 | 3.39% |
| r <= 0.10 | 0.00047 | +24, +1013, -466, +2158, +43, +14 | +2786 | -2542 | 162 / 92 | -466 | 3.79% |
| r <= 0.05 | 0.00005 | +24, +1186, -48, +2732, +43, +14 | +3951 | +1764 | 145 / 42 | -48 | 4.00% |
| r <= 0.05 | 0.00047 | +24, +1186, -48, +2448, +43, +14 | +3668 | +61 | 145 / 42 | -48 | 4.11% |
| no short book | either | +24, +1186, -48, +2854, +43, +14 | +4073 | +2142 | 135 / 48 | -48 | 3.87% |

Position age limit (`-max-hold-bars`) on the live base at borrow 0.00005: 21 bars gives windows -7925, +4892, -3768, +11694, -159, -4729 (sum +5, W2 -10837, 393 closed trades in 6 windows, maximum drawdown 2.76%); 42 bars gives -5630, +2724, -3467, +11139, -4048, -11997 (sum -11279, W2 -1588, 308 trades, 2.43%). Both fail the H-R7 criterion (sum above base +340 with fewer trades): the trade count rises.

Reading, descriptive: the loss of the base is in the short leg. Every arm that shortens less is better than the base at both borrow rates over the six windows and over W2, and the last window turns from -3579 to between -48 and +14. At the REPO-estimate borrow (0.00047) the base loses 4706 over the six windows while the arms with no or a rare short are unaffected or barely affected. Caveats: realized P&L only (open positions at the cutoff are not included, so the long-only arms, which hold longer, understate their final result); the maximum drawdown of the fewer-short arms is 3.4-4.1%, above the 3% budget in this replay, so no arm satisfies the full criterion of the plan; the arm `r <= 0.05` has far fewer than 30 short entries per window, so by the plan's rule it is "untested". Nothing is changed in the config.

## 9. Evidence from live operation

### 9.1 Root cause of the September-October SELL drift (step 9)

Hypotheses P1 (feature extrapolation), P2 (staleness), P3 (base rate) were each rejected by their registered rules (P1 Spearman of weekly mean p with weekly mean mom_63d / dist_ma50_pct / rsi_14 -0.40 / -0.24 / -0.37 with 4-7% of observations above the training 95th percentile; P2 p <= 0.40 share 17.1% deployed vs 32.9% fresh model; P3 training base rate 0.451 vs window 0.487, a 3.6 pp difference). The cause was found afterwards (post hoc, verified on every day in the table): the logged live p equals the model applied to the **unmasked** ingest vector (`news_count` up to 144 against a training mean 0.10 and std 0.70) on every day from 09-20 to 10-09, and equals the model with `news_count` and `news_sentiment` masked to zero from 10-10.

| day | logged live p | model on unmasked vector | model on masked vector |
|---|---:|---:|---:|
| 09-30 | 0.2160 | 0.2160 | 0.4556 |
| 10-02 | 0.1903 | 0.1903 | 0.4759 |
| 10-08 | 0.1504 | 0.1504 | 0.4094 |
| 10-09 | 0.1752 | 0.1752 | 0.4191 |
| 10-10 | 0.4043 | 0.1679 | 0.4519 |
| 10-17 | 0.4572 | 0.1587 | 0.4572 |

Until 2026-10-09 the running trader evaluated the ensemble with `news_count` and `news_sentiment` the model never trained on (training value 0), because `maskUntrainedFeatures` (commit `f8f64a1`, 2026-10-10) was not in the running build (the ai-box binary is dated 2026-10-06). The growing live news archive pushed the logistic and tree members to SELL: live p was on average 0.18 below the replay p (gap 0.04 on 09-17 growing to 0.28 on 10-02); 81-95% of live decisions were SELL in ISO weeks 39-41 against 17.1% in the masked replay; indicator offsets of the live ingest vectors are worth only -0.007 in mean p. Consequently the September-October "model nearly always bearish while IMOEX rose" was this feed bug, not a model property; the 82 live SELL executions of 09-17..10-09 (against 4 in the replay of the masked model) came from it.

**Post-mortem of the masked model over 2026-07-20..10-18** (step 8, replay with the deployed artifact, gate shape): realized -1820.29 RUB on 117 closed trades (-0.18% of the deposit, max DD 1.05%, no kill switch); by 30-day period -914.60 (43 trades), +3151.92 (35), -3555.03 (23). By ticker PLZL -4508, CHMF -2331, POSI -1367, NVTK -992, ROSN -697, OZON +4952, MGNT +1444, GAZP +886; BUY +839 on 28 trades, SELL -2659 on 89. Beta over the period (IMOEX +22.3%): +3871 component (BUY +8475, SELL -4605), excess -5691: the short-beta explanation is not supported; dividends -14 RUB. Under the registered rule (a cause explains the period if removing it moves realized by at least 50% of its absolute value) only the three worst trades do (PLZL BUY 15 lots -4133, a 22.3% slide while IMOEX rose 6.6%; CHMF SELL -2274; LKOH SELL -1777; removing them gives +6364). The loss is small, idiosyncratic and inside the procedure noise of section 3.6.

### 9.2 Sell-bias diagnostic and the out-of-range guard (2026-10-18)

Data: `trader-sandbox.db` audit events 2026-09-16..10-18, about 38.3k decisions since the news-masking fix. After the fix about 3600-4200 HOLD and 460-1200 SELL per day, 0 BUY; hold reasons: `out_of_range` 50-60%, `model` about 30%, SELL about 14%; probability bins: < 0.40 about 31%, [0.40, 0.60] about 69%, >= 0.60 0.11% (42 decisions, all VTBR).

- **Guard finding.** 100% of post-fix `out_of_range` blocks are `event_dividend` with z = 14..38, although the deployed artifact carries mean 0, std 1, coefficient 0 for all eight event features and both topic signals and no tree uses them. The guard vetoed would-SELL 7138, would-BUY 42 and would-HOLD 14861. A fix (`ae6ffdd`: skip features with logistic coefficient 0 that no tree uses; test `TestEnsembleIgnoresUntrainedEventFeatures`) was written and then reverted (`dcb601d`, see below); the veto behaviour is live.
- **Reproduction offline.** Replay of the deployed model with the fixed guard over 2026-10-10..10-18 (136 decisions): 95 HOLD / 41 SELL / 0 BUY (SELL share 30.1% against 32.2% live pre-guard): the lean reproduces from data and model, not a serving defect. A fresh model (trained to 2026-09-16) gives 94 / 39 / 3 BUY (BUY share 2.2%): a small staleness component in the BUY tail.
- **Deployment attempt and correction.** Deploying the guard fix made the preflight gate refuse to start (90-day replay -4859.13 net of borrow, 98 trades, hit 41.8%); the fix was reverted on the belief that the accidental veto acted as a profitable regime filter. That attribution is **not supported**: two `cmd/backtest` builds with and without the guard change give identical replay output to the last digit (window ending 10-18: -2467.25, 117 trades; ending 10-17: +1209.23, 118 trades); the live gate printed the same -4859.13 with and without the fixed binary; the "+602.32 on the same window" was an earlier gate with a window start of 07-19. The gate result is set by the window start, not by new bars (replay ending 10-18, realized net of borrow by start date: 07-13 +20380, 07-14 +14584, 07-15 +11455, 07-16 +9382, 07-17 +9100, 07-18 +3435, 07-19 +1372, 07-20 -2467, 07-21 -5085, 07-22 -5959, 07-23 -2375, 07-24 -3034, 07-25 -3514, 07-26 -5536, 07-27 -784). Whether the live path should veto on `event_dividend` is an open owner decision independent of the gate (`docs/plans/20261018-gate-failure-remediation.md`).

### 9.3 Preflight concentration (2026-10-18)

Gate-shaped replays (90-day rolling, 17 tickers, 1M deposit, costs 0.0005 x 3 plus per-ticker spreads from `trader-sandbox.db`, borrow 0.00005 per day, caps 90000/90000, deployed artifact, no news) give realized gross +9843.33 (till 10-14), +3637.89 (till 10-16), -1990.32 (till 10-18), matching the gate's own decline (+7156 -> +602 -> -4859 net of borrow; the CLI lacks lot quantisation). ISS has no bars for 10-17/10-18, so the decline is the left boundary of the rolling window. Closed trades of the till-10-18 replay by close month: 2026-07 20 trades +414.93; 08 35 +2828.55; 09 37 -4821.86 (mean -130.32); 10 24 -411.94. By ticker: OZON +4952.20 (3), MGNT +1444.44 (1), GAZP +885.65 (10), SBER +594.66 (14), VTBR +237.61 (19), RUAL +78.45 (10), TATN +65.69 (10), T -32.16 (4), MTSS -48.65 (9), LKOH -100.52 (3), GMKN -171.25 (8), ROSN -697.07 (6), NVTK -992.47 (4), POSI -1367.08 (3), CHMF -2331.33 (5), PLZL -4508.49 (7). Positives are month-concentrated (July-August), September is deeply negative, and the rolling sum drifts further negative before stabilising (profitable August days drop out while September losses stay in until December). This is a model-quality problem, not a gate problem: do not loosen the threshold.

### 9.4 Manual retrain candidate (2026-10-18), rejected

Same recipe as the deployed ensemble, fresh export 2024-01-01..2026-10-18 (18 tickers, 14619-record news archive), `ENSEMBLE_TRAIN_TILL=2026-10-17`, embargo 10; both models replayed on the HEAD engine.

| window | deployed realized | candidate realized | deployed trades | candidate trades | deployed max DD | candidate max DD |
|---|---|---|---:|---:|---:|---:|
| hold30 (2026-09-17..10-17) | +583.21 | -3281.95 | 32 | 45 | 1.24% | 1.45% |
| pre90 (2026-07-20..10-17) | +10419.63 | -1508.68 | 137 | 164 | 1.69% | 2.27% |

The candidate fails the gate on both windows; `ensemble_model.json` stays. Retrain again only through the same gate after at least a quarter of new data from 2026-10-17.

### 9.5 Ticker circuit breaker in the gate replay (2026-10-18)

`backtest.Config.Breaker` / `cmd/backtest -breaker` feed the live `risk.TickerBreaker` (3 consecutive losing round trips or cumulative loss of 5% of the 15000 RUB target; a tripped ticker stays blocked). Gate-shaped replays, one run per cell:

| window (90d till) | breaker | closed trades | realized net of borrow | unrealized MTM |
|---|---|---|---|---|
| 2026-10-18 | off | 117 | -2455.72 | -2066.01 |
| 2026-10-18 | on | 96 | -3356.64 | +6139.47 |
| 2026-07-20 | off | 90 | +34737.78 | +189.38 |
| 2026-07-20 | on | 85 | +30484.28 | +3086.18 |
| 2026-04-21 | off | 34 | -3842.92 | +4610.19 |
| 2026-04-21 | on | 22 | -1183.71 | +3291.07 |

Sum of realized over the three windows: off +28439.15, on +25943.92. The breaker does not rescue the gate. `risk.Gate` rejects every order for a tripped ticker with `ticker_breaker_blocked`, including closing orders, so a tripped ticker's position is frozen (about 8.2k RUB moves from realized to unrealized in the 10-18 window); whether blocking exits is intended is an open owner question.

### 9.6 Nightly candidate gate aligned with the preflight (2026-10-19)

The nightly harness lacked the risk layer of the preflight. Deployed artifact, 17 tickers, 2026-07-20..2026-10-18, same window the preflight rejected with -4694.87:

| variant | flags beyond the old harness | realized | realized net of borrow | net with open | closed | max DD |
|---|---|---|---|---|---|---|
| A (old harness with `-news-history`) | none | +10857 | - | -5930 | 137 | 1.72% |
| B | `-max-net-short 90000 -max-net-long 90000 -borrow-pct-day 0.00005` | -2057 | -2704 | -4843 | 116 | 1.06% |
| C | B without `-news-history` | -2069 | -2716 | -4855 | 117 | 1.06% |
| preflight (live log) | spread table from audit DB, macro enricher | - | -4695 | -903 | 99 | 1.12% |

The net-exposure caps and stress borrow rate explain the whole gap between the old harness and the preflight sign. `-news-history` changes the result by 12 RUB (the live path masks news features); the remaining 2000 RUB gap is not decomposed (candidates: the audit-derived spread table, the market enricher). `scripts/daily-retrain-box.sh` now passes the caps and the borrow rate and `cmd/retrainreport` compares realized net of borrow (`TestParseMetricsPrefersRealizedNetOfBorrow`, `TestDailyScriptReplayMatchesPreflightRisk`). Earlier nightly verdicts (10-16, 10-17) were produced without the caps and are not recomputed.

### 9.7 Live sandbox P&L, 2026-09-17..2026-10-18

`cmd/livepnl` over the audit copy (executor events replayed with `internal/pnl`, marked to ISS closes; 148 fills, none between 09-24 and 10-06 or after 10-14):

| period | binary | realized | equity change incl. unrealized | net / gross exposure at end | IMOEX |
|---|---|---|---|---|---|
| 09-17..10-09 | no feature mask (bug) | -149 | -14073 | -226922 / 227104 | +6.97% |
| 10-10..10-18 | mask `f8f64a1` | -1771 | -11464 | -244191 / 244384 | +1.87% |
| total | | -1920 | -25395 | | +8.9% |

The book was entered as an almost purely short book (17 positions) during the unmasked period and stayed short after the fix because the fixed model mostly returned HOLD (4 BUY fills, all covering shorts). Beta check: -227k net short against +6.97% IMOEX is about -15.8k, the same order as the observed -14.1k, so the unmasked-period loss is market beta of a book the bug built. The masked period lost more than its beta (about -4.6k, observed -11.5k; three covered shorts, one -1650 realized on 10-14). Realized P&L alone hides the loss sitting in 17 open positions. Caveats: last close at or before the boundary, no borrow, dividends or financing; the Mac trader was in `error` after 38 restarts at the audit copy of 2026-10-19.

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

### 9.8 Execution-quality tracking (Task 16)

Live tracking of expected (bounded limit) versus actual fill price, broker rejection rate under `risk.max_slippage_pct` and actual short-borrow charges: a positive slippage in basis points means the fill was better than the cap. The trader logs each fill and emits a digest every 6 hours cross-referencing observed slippage against the per-ticker half-spread and margin-fee charges against the 0.005% per day stress rate (`internal/filltracking`, `internal/borrowcost.SumMarginFees`, `executionQualityReporter`). No fill-level numbers have been recorded yet.

## 10. Observability metrics and alerts

Prometheus metrics of the live trader (`internal/metrics/metrics.go`, wired through `internal/orchestrator` and `cmd/trader`; `moex_trader_inference_duration_seconds` was renamed from `llm_inference_duration_seconds` because the pipeline is a model, not an LLM):

| metric | type | labels | source |
|---|---|---|---|
| `moex_trader_inference_duration_seconds` | histogram | - | orchestrator signal generation |
| `signals_generated_total` | counter | - | orchestrator |
| `risk_rejections_total` | counter | `reason` | risk gate decision |
| `executor_skips_total` | counter | `reason` | approved decisions the executor skipped |
| `signal_probability` | histogram | - | model probability per poll |
| `candle_age_seconds` | gauge | `ticker` | newest ingested candle age |
| `feature_psi` | gauge | `feature` | drift monitor warnings |
| `position_notional` | gauge | `ticker` | signed exposure per ticker (RUB) |
| `gross_exposure` | gauge | - | sum of absolute per-ticker notionals (RUB) |
| `lease_held` | gauge | - | 1 while this process holds the trading lease |
| `var_95`, `var_99`, `es_95`, `es_99` | gauge | - | 1-day historical VaR/ES of the current book (RUB), 250-day trailing returns (`internal/risk/var.go`), monitoring only, also one line in the daily Telegram digest |
| `incident_shorts_total` | counter | `ticker` | SELL decisions emitted by the news gate during an incident (not every one becomes a fill) |
| `news_exit_total` | counter | `ticker`, `reason` | first exit decision for a news-opened position (`stop_loss`, `take_profit`, `time_exit`); each also writes an `audit_events` row, stage `news_exit`, with entry, peak and result for the parameter fit in `docs/plans/20261010-news-exit-tuning-and-buy-side.md` |

Telegram alerts (`internal/alert/telegram/watch.go`), each with a cooldown:

| alert | trigger | default threshold | default cooldown |
|---|---|---|---|
| probability collapse | more than 90% of the last N probabilities inside [0.45, 0.55] | N = 20, fraction 0.9, band 0.45-0.55 | 1h |
| stale candles | candle age above threshold during the trading session | 24h (daily candle pipeline) | 1h |
| PSI breach | drift monitor warning above the configured PSI threshold | `drift_psi_threshold` 0.2 | 1h |

Defaults come from `config.alerts` (`probability_collapse_window`, `stale_candle_age`, `cooldown`); negative values are rejected, and the senders are no-ops without Telegram credentials.

## 11. Synthesis

| question | result | section |
|---|---|---|
| Does the deployed recipe predict direction? | No: AUC 0.46-0.55, N95 noise 0.016, tail precision below the base rate | 3.1, 3.6 |
| Where does its P&L come from? | Mostly short IMOEX beta; alpha vanishes under a real futures hedge | 5.2-5.5 |
| Can a single change pass the gate? | No variant of A-F, depth, breadth, rank, volatility-unit, open-interest or TimesFM changes | 4, 6 |
| Is a longer horizon or more history better? | Horizon: no (10 days best). History: small paired gain at the noise floor, no PnL benefit | 6.1, 6.3, 6.6 |
| Does news carry a tradable signal? | AUC about 0.52 at best, net spread +0.263% with CI spanning zero, short leg only | 7.3-7.6 |
| Do LLM headline directions help? | No (AUC 0.511-0.526) | 7.7 |
| Do events, levels, bursts matter? | No tradable effect found | 7.8-7.10 |
| Are there calendar or cycle effects? | None after 1990 / after costs | 8.11 |
| Is there any cross-sectional edge? | Ridge long-minus-universe passes the registered rule; long-short needs borrow below about 5% per year; gate-candidate pass rate 59% | 8.6, 8.8 |
| What caused the autumn SELL drift? | Unmasked news features in the running build, not the model | 9.1 |
| What beats the hurdle? | Nothing recorded exceeds the 14% key-rate carry | 8.12 |

Open items: the real short-borrow rate (Task 11 cannot measure it from account history); the forward track of the ridge and C2 books (recorder running since 2026-10-19, first evaluation after at least 12 rebalancings); the `event_dividend` veto decision; whether the ticker breaker should block exits; a second fundamentals experiment after at least 120 daily snapshots.

## Appendix A. Concordance with the former section names

| former section (commit `82a124b`) | now |
|---|---|
| Tail-precision falsification (Task 1) | 3.1 |
| Dividend capture event-study (2) | 7.11 |
| Process rule (incident E4) | 0.5 |
| Momentum benchmark (Task 2) | 3.2 |
| Live/backtest feature parity fix (Task 3) | 2 |
| Portfolio risk gate in backtest (Task 4) | 2, 3.3 |
| Preflight hardening (Task 5) | 2, 3.4 |
| Attempt registry, DSR and PBO (Task 7); Period-return matrices and pre-registered attempts (Task 4) | 3.7 |
| Purged walk-forward baseline | 3.5 |
| Experiment A, B, C, D, E | 4.2 |
| Experiment F | 4.3 |
| Sell-bias diagnostic | 9.2 |
| Manual retrain candidate | 9.4 |
| Preflight-concentration analysis | 9.3 |
| Deployment decision 2026-10 (Task 13) | 4.1 |
| Gap-stress test (Task 8) | 5.1 |
| Beta / regime decomposition (Task 9) | 5.2 |
| OOS freeze (Task 10) | 5.9 |
| Short-borrow measurement and stress (Task 11) | 5.8 |
| Shadow reconciliation (Task 3) | 9.7 (marker block) |
| Sandbox/paper execution quality tracking (Task 16) | 9.8 |
| Negotiations/sanctions signal evidence gate (Task 6) | 6.9 |
| Backtest metrics: Sortino, Calmar, CAGR; HTML tearsheet and walk-forward harness | 0.1, 2 |
| News classifier recalibration attempt; rebuild from a fresh 9596-article backfill | 7.1 |
| Alpha-without-beta hedge test (0) | 5.3 |
| Real IMOEX-futures overlay | 5.4 |
| Market-neutral retraining; Phase 2b | 5.5 |
| Managed-beta leverage grid (1) | 5.6 |
| `max_net_exposure` on the unhedged book / on a 100% short book / semantics change | 5.7 |
| Where the 9596 news articles are actually lost; Same root cause reaches the PRICE model | 7.2, 6.2 |
| Observability metrics and alerts (Task 6) | 10 |
| Horizon sweep 9-40d and news-set features | 6.1 |
| Event causal study | 7.8 |
| Macro and dividend-yield features | 6.8 |
| Level-crossing event study (H20) and extension | 7.9 |
| H22: news burst | 7.10 |
| Data depth, step 1 / step 3 | 6.3 / 6.4 |
| Data depth, step 2 / 2b / 2c / 2g | 7.3 / 7.4 / 7.5 / 7.6 |
| Explainability of signals | 6.10 |
| H23 stage 0 | 8.1 |
| Ticker circuit breaker in the gate replay | 9.5 |
| Seven hypotheses, step 2 | 7.6 |
| Seven hypotheses, step 5 | 3.6 |
| Seven hypotheses, step 6 / step 10 | 8.2 |
| Seven hypotheses, step 7 | 8.3 |
| Seven hypotheses, step 4 | 6.6 |
| Seven hypotheses, step 8 / step 9 | 9.1 |
| Seven hypotheses, step 1 (INTERIM and FINAL) | 7.7 |
| Live sandbox P&L | 9.7 |
| Nightly candidate gate aligned with the preflight | 9.6 |
| Seven hypotheses, part B, steps 11-13 / 14-16 / Step 15 follow-up / Step 17 | 6.7 |
| Seven hypotheses, part C, steps 18-21 and corrections, step 23 | 8.4, 8.5, 8.12 |
| Step 24 / 24b | 6.5 |
| Part C addendum | 8.5 |
| Step 25 | 8.6 |
| Step 26 | 8.7 |
| Step 27 | 8.8 |
| Step 28 | 8.9 |
| Step 29 | 8.10 |
| Step 30 | 8.11 |
| H-R9, research-to-live gap attribution | 8.13 |
| Short-borrow rate: measurement and ridge ls sensitivity | 5.8 |
