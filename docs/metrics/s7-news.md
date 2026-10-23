# Metrics: news and event information (section 7)

Part of [metrics.md](../metrics.md) (the index, conventions and ledger live there). Numbers here are the single source of truth for this topic.

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

Protocol `plan 20261009-event-causal-study (git history)`. Panel: 17 tickers x 140 trading days (2026-04-01..10-09), 10213 archive records, abnormal return against IMOEX (market model beta fitted on 2025-10-01..2026-03-31; `excess` mode as robustness), 2000 date-bootstrap and 1000 placebo draws, seed 42. No P&L is involved.

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

Pre-registered protocol `plan 20260925-event-driven-dividend-drift (git history)`; `cmd/dividendstudy`; calendar `data/dividends.jsonl` (123 events 2021-2027 from T-Invest `GetDividends`, 65 in the study window 2024-01-01..2026-12-19, 9 waves). Position 30000 RUB, costs 0.05% per leg, exit at the ex-date close; mean excess return over IMOEX by entry offset (trading days before the last buy date), bootstrap over whole waves (seed 42, 2000 repeats), holding tax 0.13 (worst case) or 0.00:

| offset | excess (tax 0.13) | CI (tax 0.13) | excess (tax 0.00) | CI (tax 0.00) |
|---|---|---|---|---|
| -10 | +0.0050 | [-0.0149, +0.0198] | +0.0114 | [-0.0083, +0.0263] |
| -5 | -0.0027 | [-0.0165, +0.0104] | +0.0037 | [-0.0102, +0.0168] |
| -3 (primary) | -0.0063 | [-0.0223, +0.0097] | +0.0001 | [-0.0165, +0.0165] |
| -2 | -0.0034 | [-0.0124, +0.0067] | +0.0030 | [-0.0066, +0.0137] |
| -1 | +0.0006 | [-0.0052, +0.0079] | +0.0071 | [+0.0008, +0.0151] |

The mean gap ratio is -0.97: the ex-date open sits about one full dividend below the last-buy close, so the drop is almost fully priced. Acceptance needed a contiguous subset containing {-2, -3, -5} with the CI excluding zero and leave-one-out stability under both tax conventions; all fail, and only offset -1 (tax 0.00) excludes zero, outside the registered subset. **Rejected**: dividend capture is not deployable; the income lever reverts to position scaling under an exposure cap, which sections 5.6-5.7 then closed. Detail: `docs/reports/dividendstudy-report.md`.

**Re-test on the wide calendar (2026-10-22, R1 of `plan 20261022-pre-token-recheck (git history)`), rejected again.** Calendar `dividends_pit.jsonl` (2152 T-Invest records, point-in-time panel), last buy dates 2014-01-01..2026-12-19, 1591 events in 38 waves (65 events, 9 waves before), same protocol and acceptance rule; 2022-02-25..03-28 and daily moves above 45% excluded. Mean excess over IMOEX by offset (trading days before the last buy date):

| offset | excess (tax 0.13) | CI (tax 0.13) | excess (tax 0.00) | CI (tax 0.00) |
|---|---|---|---|---|
| -10 | +0.0081 | [+0.0015, +0.0150] | +0.0152 | [+0.0083, +0.0225] |
| -5 | -0.0026 | [-0.0061, +0.0011] | +0.0044 | [+0.0006, +0.0084] |
| -3 (primary) | -0.0047 | [-0.0082, -0.0011] | +0.0022 | [-0.0016, +0.0063] |
| -2 | -0.0050 | [-0.0082, -0.0020] | +0.0019 | [-0.0016, +0.0053] |
| -1 | -0.0037 | [-0.0065, -0.0010] | +0.0033 | [+0.0000, +0.0062] |

The required subset {-2, -3, -5} has no positive CI under either tax convention, and leave-one-out fails under tax 0.13 for every offset from -5 to -1; at tax 0.13 the offsets -3, -2 and -1 are significantly negative. Split by period at tax 0.13: 2014..2021 (844 events) -3: -0.0037, -2: -0.0044; 2022-04..2026 (740 events) -3: -0.0059, -2: -0.0058, the same sign in both. Only the early entry -10 is positive, outside the registered subset and for ex-dividend runs that the market prices in advance. The reported mean gap ratio (about 20) is dominated by outliers in `dividend_net` and is not interpretable; it was descriptive and did not enter the verdict. Reports: `reports/20261022/recheck/`. Dividend capture is closed for good: the larger sample confirms the rejection.

