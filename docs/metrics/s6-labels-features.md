# Metrics: labels, horizon, training data, features (section 6)

Part of [metrics.md](../metrics.md) (the index, conventions and ledger live there). Numbers here are the single source of truth for this topic.

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

