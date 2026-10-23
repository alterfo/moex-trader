# Metrics: baseline performance (section 3)

Part of [metrics.md](../metrics.md) (the index, conventions and ledger live there). Numbers here are the single source of truth for this topic.

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

