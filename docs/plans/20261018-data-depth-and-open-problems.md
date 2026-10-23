# More data and five open problems (2026-10-18)


Source of the five problems: the review by Darwish, Hassanien, Eissa (2026), the section on unsolved tasks. Here they are tied to data that was missing before. The constraints from `AGENTS.md` and `docs/plans/20261015-hypotheses-from-llm-review.md` apply unchanged: the LLM stays off the trading path, no automatic replacement of `ensemble_model.json` without separate consent of the owner, metrics go into `docs/metrics.md` in the same commit.

## What is known about the data as of 2026-10-18

- ISS serves TQBR from 2013 (about 3400 trading days for SBER). The default training window is two years (`trainmodel -from`), the `evalh.py` protocol used `-from 2024-01-01`. Depth by year was not tested; Experiment B (`docs/metrics.md`) only extended the ticker list and was rejected (AUC 0.5185, minus 0.0185).
- Macro series in `data/macro` start in 2023. `cmd/macrofetch -from 2013-01-01` gives imoex, cnyrub, keyrate from 2013 and brent from 2017.
- The dense news archive `data/news_history.jsonl` starts in April 2026 (about 1500-2600 records per month); before March 2026 there are 1-10 records per month. `Kasymkhan/RussianFinancialNews` (Apache 2.0): 92 thousand articles, 2020-06..2024-11, tickers in `tags`, the period December 2024 – March 2026 is not in it.
- The effective number of observations is set by the number of dates, not rows: a 10-day horizon and correlated tickers make added tickers a weak substitute for added years.

## Order

1. Price depth (this section, criteria below).
2. News from the 2020-2024 dataset for the classifier and features.
3. Calibration by horizons and regimes.
4. Long horizons (20-60 days) on deep history.
5. Explainability (SHAP, reasons for the signal in the alert).
6. The role of the LLM in the risk loop and localization: drawn up after the results of 2 and 5; criteria are written before the run of each step in a separate commit.

## Step 1. Pre-registration: depth of the training history (criteria fixed before the run)

Data: `exportdataset -from 2015-01-01 -till 2026-10-07 -split 2026-06-18 -horizon-days {10,20} -deadband-pct 0.5 -label-mode absolute`, 17 trading tickers (FX excluded), no news and no macro. One file per horizon, all arms are cut from it.

Protocol: `evalh.py` `wf()` unchanged (expanding purged walk-forward, folds of 63 days from 2025-04-01, `label_date < fold start`, LightGBM plus logistic regression, 15 price features). The test rows are identical in all arms; only the start of the training sample differs.

Arms:
- A: training from 2024-01-01 (baseline).
- B: from 2021-01-01.
- C: from 2018-01-01.
- D: from 2015-01-01.
- E: from 2015-01-01 without 2022-02-24..2022-12-31 (a check for a regime break).

Primary metric: pooled AUC; secondary: mean AUC over folds, number of folds with AUC > 0.5, Brier and 10-bin ECE.

Candidate criterion: an arm beats A by at least +0.010 pooled AUC at both horizons and has more than half of the folds above 0.5. A spread within ±0.005 counts as noise (as in `model-quality-history.md`). Weaker: the result is recorded as "depth does not help", and further steps are planned from the 2024+ window.

Secondary criterion for a weak positive effect (addendum 2026-10-18, fixed before the run): the primary criterion requires +0.010 at both horizons, which with 9 folds and a base near 0.53 has low power — a real effect of +0.005..0.008 will with high probability be recorded as "depth does not help", and then steps 3-5 would lose the deep history without sufficient grounds. So a weak but stable effect is registered separately, for each horizon: (1) the pooled AUC of arm D minus the pooled AUC of arm A is positive, and the lower bound of the block bootstrap CI by trading dates (2000 repetitions, blocks by dates, seed 42) is above −0.002; (2) D beats A in at least 7 of 9 folds. The differences B−A, C−A, D−A at both horizons are given descriptively (a by-eye check of monotonicity, not a formal test). If the conditions hold at least at one horizon, the "depth does not help" record is not made: the effect is registered as "weak positive, insufficient to replace the model", and steps 3-5 (calibration by horizons and regimes, long horizons, explainability) may use the deep history; replacing the model still requires only the primary criterion. If D−A ≤ 0 at both horizons, the conclusion "depth does not help" stands as recorded above. Arm E does not take part in monotonicity (it is a regime-break check, a different question): for it, E and D are compared descriptively.

Limit on conclusions: a model replacement candidate separately requires a purged walk-forward through `cmd/backtest` with costs and explicit consent of the owner; AUC here is only the admission to such a test.

## Step 2. Pre-registration: news classifier on the 2021-2024 archive (criteria fixed before the run)

Question: does training on Russian headlines of 2021-2024 (dataset `Kasymkhan/RussianFinancialNews`, Apache 2.0) give a directional signal on the independent period 2026-04-01..2026-10 where the previous classifier on 2855 samples gave val AUC 0.493-0.529 and was declared dead.

Data:
- Training: `cmd/newsimport` over the `train` and `test` parquet, only the 18 trading tickers, records dated before 2025-01-01 (18957 records, 15386 unique articles in 2021-2024; 97% without publication time).
- Validation: `data/news_history.jsonl`, records from 2026-04-01 for the 17 config tickers (7129 records, 6481 articles). Records of this file before 2026-04-01 do not enter training. The owner's decision of 2026-09-29 on Telegram and Google News stands: the slice without them (731 records) is given separately.
- No leakage: `BuildNewsLabels` enters at the open of the next trading day after the article date, so an article "without time" does not see its own day.

Method: `cmd/trainnewsmodel` unchanged (bag-of-words logistic, `-vocab-size 3000 -min-doc-freq 3`, label: direction of excess return against IMOEX), `-split 2026-04-01`. Sample scores are taken by the new command `cmd/newsvalidate` (read only, CSV).

Primary horizon: 10 trading days (as for the price model). Horizons 1 and 5 are given descriptively, no verdict is issued on them.

Criterion: val AUC at the primary horizon >= 0.530 and the lower bound of the 95% CI of the block bootstrap by publication dates (2000 repetitions, seed 42) above 0.500, provided at least 1000 validation samples and 40 dates. Additionally reported: AUC on the slice without Telegram and Google, on the slice with `-max-tickers-per-article 3`, accuracy against the majority share. If the criterion is not met, "the Russian 2021-2024 archive gives no directional signal" is recorded, and the news feature is not added to the price model in step 2.

Second stage of the step (only if the criterion is met): evaluating the classifier score as a feature of the price model under the step 1 protocol (arm D plus `news_sentiment`), the same threshold: +0.010 pooled AUC and a majority of folds above 0.5.

## Step 3. Pre-registration: calibration by horizons and regimes (criteria fixed before the run)

Question: can the ensemble probabilities be brought to calibrated ones (ECE and Brier lower than raw), stably across horizons and market regimes, without touching the ranking (AUC).

Data and protocol: the same datasets and folds as in step 1 (`h10.csv`, `h20.csv`, 9 folds of 63 days from 2025-04-01, 15 price features, LightGBM plus logistic regression). Additionally the dataset `h5.csv` with the same export for the 5-day horizon. Training windows: A (from 2024) and D (from 2015).

Calibrators (trained strictly before the start of the fold): raw (no calibration), Platt (logistic regression on the ensemble logit), isotonic regression. Calibration sample: the last 20% of training rows by time (purged: `label_date < fold start`), the main model is trained on the other 80%. This is the only calibration variant in the step, no others are tuned.

Regimes (by decision date, thresholds computed only from data before the start of the fold): IMOEX volatility over 21 days above or below the median of the training part; the key rate (from `data/macro`, rise or fall over 90 days). The regime report is descriptive.

Primary metric: ECE (10 bins) and Brier, pooled over 9 folds, at each horizon.

Success criterion: for each of the horizons 5, 10, 20 and both windows (A, D) the best calibrator lowers ECE by at least 20% relative to raw and does not worsen Brier, while pooled AUC changes by no more than ±0.005. Additionally: the ECE reduction must hold in a majority of folds (at least 6 of 9) and in both volatility regimes. If the criterion is not met on all six combinations, it is recorded on which it is met; the calibrator is not connected to the trader without a separate recomputation of the 0.60/0.40 thresholds and consent of the owner, because calibration shifts probabilities relative to the current thresholds.

## Step 5. Pre-registration: explainability of the signal (criteria fixed before implementation)

Method: attribution by replacing a feature with its baseline value (occlusion). For each feature of the vector the change in the ensemble probability is computed if that feature is replaced by the training sample mean (`logistic.mean` from `ensemble_model.json`, no new artifacts needed). These are not Shapley values: interactions between features are not divided, the remainder of nonlinearity is output separately. Exact TreeSHAP is unavailable: the artifact has no node covers for the trees. Features masked as untrained (`news_count`, `news_sentiment`) contribute nothing.

Output: the signal's `Reasoning` gets ` drivers=<name>:<±contribution in pp>,...` after `p=` for the three largest contributions by absolute value. The parser `probabilityFromReasoning` and the alert prefix truncation carry this through; regression tests for both sides are part of the implementation.

Acceptance criteria (before the run):
1. For the logistic member, the attribution in logit space matches the exact linear one (`coef*(x-mean)/std`) to 1e-9; for a vector equal to the baseline all contributions are zero; the result is deterministic.
2. A faithfulness test on the dataset `h10.csv`, rows from 2025-04-01, 17 tickers, the deployed artifact `ensemble_model.json`: replacing the three most important features by attribution with their baselines changes the probability in absolute value on average at least 1.5 times more strongly than replacing three random features (seed 42, 20 random draws per row, averaged).
3. Sign agreement: in rows where the probability leaves the 0.40-0.60 zone, the sign of the contribution of the top driver matches the sign of the deviation of the probability from 0.5 in at least 70% of cases.
If any of the three criteria is not met, the `drivers` format is not connected to the trader, and the result is recorded in `docs/metrics.md`.

## H23. AR-ICA-MGARCHSK for portfolio construction (pre-registration, 2026-10-18)

What it is (from the primary sources found, without access to the original article): an AR filter of the means, then a decomposition of the covariance by independent component analysis (ICA, the GO-GARCH family), for each independent component a univariate GARCH with dynamic skewness and kurtosis (GARCHSK, for the GJR variant called GO-GJRSK); the conditional covariance and higher moments of the assets are restored by the inverse linear map and fed into a portfolio optimizer that accounts for co-kurtosis and co-skewness. Literature: Lassance, DeMiguel, Vrins, ICA as a way to reduce the dimension of comoment tensors; Mathematics 2020, 8(11), 1990 (GO-GJRSK). There is no ready implementation in Go, and `gonum` is not among the dependencies.

Why this is not an obvious gain for us: the current scheme issues positions of an equal amount of 15000 RUB, and the simplest volatility scaling (Experiment D) gave P&L -20 RUB (-0.03%), so the expected effect is also small, and on 17 names with 250 observations higher moments are estimated noisily. So the hypothesis is tested as a reduction of tail risk, not as a growth of return.

Protocol (the Experiment D method): the portfolio weights are applied as a multiplier to the position size in `EnsembleSignalSource` on top of the existing signals, without retraining, on the same six windows and `wf-volscale` artifacts. Compared: control (equal positions), inverse volatility (already rejected, as a lower bar), covariance minimum variance (sample covariance), ICA with GARCH(1,1) on the components (GO-GARCH, no SK), ICA with GARCHSK. Each next step justifies the added complexity only if it beats the previous one.

Criterion (before the run): a step is accepted if the total realized P&L is not worse than control by more than 3%, and the maximum drawdown across windows and the 95% CVaR of the portfolio daily return are lower than control by at least 15%, with the kill switch firing in no window earlier than for control. Otherwise: rejected, recorded in `docs/metrics.md`. The implementation (FastICA, GARCH, GARCHSK) is written in Go as the offline tool `cmd/icaportfolio` and is not connected to the trader until a positive result and separate consent.

### H23, clarification from the original (2026-10-18, before implementation; replaces the method description above)

Source: Mikhailov A.Yu., Yusif N.B., Sotskov Yu.N., Khomyakova L.I. Construction of dynamic high-order portfolio models based on VAR-ICA-MGARCH // Finance: Theory and Practice, 2026, 30(4), pp. 19-37, DOI 10.26794/2587-5671-2026-30-4-19-37 (read in full).

The specification of the paper:
- Returns: VAR(1) (order by AIC) on log returns, the residuals go into ICA: `IC_t = U * eps_t`.
- For each independent component a univariate GARCHSK recursion: `xi^H_t = w + a*IC^2_{t-1} + b*xi^H_{t-1}`, the same for `IC^3` (skewness) and `IC^4` (kurtosis). The matrices are returned by the inverse map: `H = U^-1 Xi^H U^-T`, `S`, `K` through Kronecker powers of `U`.
- Utility: CARA, `U(W) = -exp(-rho*W)`, `rho = 1`, Taylor expansion to the 4th order: `max -exp(-rho(1+mu_p)) * (1 + rho^2/2 * sigma_p^2 - rho^3/6 * s_p^3 + rho^4/24 * k_p^4)` with `sum w = 1`, `w >= 0`. Solved by a genetic algorithm with non-dominated sorting and elitism.
- Data: a simulation of a 4-dimensional model in R on 1000 observations; empirics on 6 Chinese stocks (2014-01-03..2019-04-19, 1304 observations).

What in the paper's evaluation does not allow accepting its conclusions as is (noted before the run so as not to transfer them automatically):
- The comparison is by utility computed from the model's own conditional moments, not by realized out-of-sample returns. No costs, no realized return, Sharpe, CVaR, confidence intervals, no comparison with equal weights and with simple covariance estimators. The simulation is generated from the same family of models.
- The "traditional" portfolio is static (weights 0.44/0.33/0.15/0.07/0/0), the dynamic one is recomputed daily; the utility gain is partly a gain from recomputation.
- The estimates of conditional skewness and kurtosis are unstable (figures 5-6: up to 1442 and 1631, drift up to 150 for one component; in table 4 `alpha^S` = -0.5174, `alpha^K` = 0.0000), the parameter estimation method is not described.
- Utility depends on units: returns in percent, `rho = 1`; in fractions the same `rho` makes the higher-order terms negligible.
- The definition CEL% `(U(V*) - U(V')) / U(V')` with negative utilities gives a negative number, table 6 gives positive ones (25-47%); the sign convention is not disclosed. The dating of the sample is contradictory (abstract and section 3: until 2019-04-19; conclusion: until 2024-04-19).
- The variance `H` and kurtosis `K` in formulas (6) for independent components: the diagonal tensor `K` describes cumulants, while the utility expansion needs the fourth central moment `k_p^4 = sum a^4 kappa4 + 3 (sum a^2 kappa2)^2`; direct application of `W^T K (W (x) W (x) W)` misses the cross terms.

Implementation (deviations from the paper are fixed in advance):
- Estimation of the recursion parameters for `IC^2`, `IC^3`, `IC^4`: least squares over the recursion `xi_t = w + a * x_{t-1} + b * xi_{t-1}` (Nelder-Mead, constraints `w` positive for even powers, `a, b >= 0`, `a + b < 1`), since the paper does not state the method. Conditional cumulants: `kappa2 = xi^H`, `kappa3 = xi^S`, `kappa4 = xi^K - 3 (xi^H)^2`; the values are capped at the 99% quantile of the training sample so as not to repeat the instability of figures 5-6.
- Portfolio moments for independent components: `a = U^-T w`, `sigma^2 = sum a^2 kappa2`, `s^3 = sum a^3 kappa3`, `k^4 = sum a^4 kappa4 + 3 sigma^4`.
- Optimizer: a deterministic projected gradient on the simplex with several starts (seed 42) instead of a genetic algorithm: the objective is a smooth polynomial in `w`, reproducibility is needed.
- ICA: FastICA (deflation, `tanh`), white stabilization via Jacobi decomposition; returns in percent, `rho = 1` as in the paper, plus a descriptive variant `rho = 5`.

Stage 0 protocol (no signals; goal: test the portfolio construction method itself on our market before integration into the trader):
- Universe: stocks from `config.yaml` (no FX), ISS daily closes; at each rebalance tickers with at least 3 years of history are admitted.
- Estimation window 750 trading days, re-estimation and rebalancing every 21 trading days, test 2019-01-02..2026-10-07 (including 2020 and 2022), costs 0.05% per side of turnover, long-only.
- Steps: R0 equal weights (control), R1 inverse volatility over 63 days, R2 minimum variance on the sample covariance, R3 VAR-ICA-GARCH (second moment only, `s = 0`, `k = 3 sigma^4`), R4 full VAR-ICA-MGARCHSK per the paper's specification (`rho = 1`), descriptively R4 with `rho = 5` and R4 with `mu = 0`.
- Metrics: net annual return, annual volatility, 95% CVaR of the daily return, maximum drawdown, turnover, Sharpe; confidence intervals by stationary block bootstrap (block 20 days, 2000 repetitions, seed 42).
- Criterion for step R3 or R4: relative to R0 the 95% CVaR is lower by at least 15% and the upper bound of the 95% CI of the CVaR ratio is below 1.0; the maximum drawdown is lower by at least 15%; the net annual return is not below control by more than 1 pp. Additionally R4 is accepted as justified complexity only if it beats R3 by at least 5% on CVaR and beats R2. If R3 and R4 do not pass, no repeat on the `wf-volscale` artifacts is made, and the result is recorded in `docs/metrics.md`.
- Limitation: any step remains an offline tool (`cmd/icaportfolio`); connecting it to `EnsembleSignalSource` or the trader requires a separate recomputation on signals (long and short positions) and consent of the owner.

Implementation details of H23, stage 0 (fixed before the first run, 2026-10-18):
- Conditional cumulants are forecast for the holding period: the mean over the 21 forecast days of the recursion `xi_{T+h} = mu + rho^(h-1) (xi_{T+1} - mu)` is taken, not the one-step forecast.
- The mean forecast `mu = Phi * y_T` (VAR(1) without an intercept on demeaned returns), as in the paper without a separate mean term; the variant `R4_mu0` zeroes `mu`.
- Log returns in percent go into the model, simple returns go into the portfolio simulation; weights drift between rebalances; costs of 0.05% of turnover at each rebalance, including the initial purchase.
- Ticker admission at a rebalance: a quote exists on the rebalance day and at least 740 trading days out of the 750 window; below 3 admitted tickers the run stops with an error.
- If the fit of step R3 or R4 does not converge on a window, the previous weights are kept at that rebalance (or equal ones at the very start); the number of such cases is not hidden.
- Code: `internal/icaport` (9 tests: Jacobi decomposition, matrix inverse, FastICA source recovery, recursions, moments against sample ones, gradient against finite differences, simplex, metrics, determinism) and `cmd/icaportfolio`.

## Step 2b. Pre-registration: news domain shift (criteria fixed before the run, 2026-10-18)

Question: is the null result of step 2 (AUC 0.486-0.528) explained by a domain shift (training on long BCS/Finam texts of 2021-2024, validation on 90% short Telegram and Google News alerts of 2026) or by the absence of any signal in headlines. The archive composition is measured in `docs/knowledge/news-and-events.md` (Archive composition).

Data: `data/news_history.jsonl`, records from 2026-04-01 for the 17 tickers from `config.yaml` (7257 pairs at the time of pre-registration, 7054 after deduplication by the pair ticker plus normalized headline). Training: records 2026-04-01..2026-07-31; a gap of 14 calendar days (labels at 10 trading days do not overlap the test); test: records from 2026-08-15. Pairs whose normalized headline (lowercase, letters and digits only) occurs in training for the same ticker are removed from the test; the number removed is given. Source approval: the owner's decision of 2026-09-29 on Telegram and Google News stands.

Arms:
- A: training on 2026 only (April-July), the same classifier (`cmd/trainnewsmodel`, bag-of-words logistic, `-vocab-size 1000 -min-doc-freq 3`, the rest default, label of the direction of excess return against IMOEX, entry at the open of the next trading day).
- B: the classifier of step 2 (trained only on 2021-2024), evaluated on the same test without further training.
- C: training on 2021-2024 plus 2026 April-July (`-vocab-size 3000 -min-doc-freq 3`).

Horizons 1, 5, 10 trading days. Metric: AUC of the headline score on the test; confidence intervals by block bootstrap by publication dates (2000 repetitions, seed 42) with a Bonferroni correction for 6 tests (arms A and C at 3 horizons, level 1 - 0.05/6 = 99.17%).

Criterion: an arm (A or C) at a horizon counts as showing a directional signal if AUC >= 0.530 and the lower bound of the corrected CI is above 0.500, with a test of at least 1000 samples and 40 dates. Arm B is used as a control: if A or C show a signal and B does not, the domain shift is confirmed; if no arm shows a signal, the conclusion "there is no stable directional signal in headlines" does not depend on the domain. Additionally given: the slice without Telegram and Google (descriptive) and the share of duplicates removed from the test.

Consequences: if a signal is confirmed, the news feature is not added to the price model without a separate pre-registration of the evaluation under the step 1 protocol; if there is no signal, the news classifier stays off and direction 2 is exhausted.

## Step 2v. Pre-registration: news classifier on the extended archive (criteria fixed before the run, 2026-10-18)

Question: steps 2 and 2b worked on 17 tickers and 1427-2817 test samples over 38-49 dates, and the monthly AUC jumped from 0.37 to 0.75. Now the universe is 260 stocks and tens of thousands of headlines of 2025-2026 (RSS, Telegram, 13 new channels): does training on 2021-2024 give a directional signal on this sample, and is it stable over time. The archive was obtained by merging and cleaning on 2026-10-18 (`docs/knowledge/news-and-events.md`, sections "Archive merge", "Archive cleaning", "More Telegram channels"), the composition is fixed before the run.

Data and filters (before the run):
- Archive: `data/news_history.jsonl` after the final rebuild by `scripts/rebuild-news-archive.sh` (with the raw files `data/news_raw_tg*.jsonl`); the number of pairs and the composition are recorded before the run.
- Training: pairs published before 2025-01-01; the source "MOEX site news" (administrative notices, not news texts) and pairs of articles linked to 4 or more tickers (`-max-tickers-per-article 3`, BCS digests) are excluded.
- Test: pairs from 2025-03-01 (a gap of two months, more than any horizon); the same two exclusions; pairs whose normalized headline occurs in training for the same ticker are removed from the test.
- Tickers: stocks with a 20-day median turnover of at least 10 million RUB on the decision date, like `-train-min-turnover` in `trainmodel`; labels are computed by `BuildNewsLabels` (entry at the open of the next trading day, excess return against IMOEX). The filter is applied identically to training and test.
- Model: `cmd/trainnewsmodel` unchanged (bag-of-words logistic, `-vocab-size 3000 -min-doc-freq 3`), horizons 1, 5, 10 trading days.

Metric and criterion: AUC of the score on the test; CI by block bootstrap by publication dates (2000 repetitions, seed 42) with a Bonferroni correction for 3 horizons (level 98.33%). There is a signal at a horizon if AUC >= 0.530 and the lower CI bound is above 0.500 with at least 1000 test samples and 40 dates, and in addition the AUC is positive (>= 0.500) in at least 4 of 6 calendar quarters of the test (a stability requirement: earlier tests showed a window-dependent result). Leakage control: the same AUC under permutation of scores within each date (seed 42) must lie in [0.48, 0.52]. Descriptively given: AUC by source groups (RSS and agencies, Telegram, Google News, specialized sites), by quarters and among the 30 most discussed tickers.

Consequences: if the criterion is met at least at one horizon, the news feature may be evaluated as an input of the price model only by a separate pre-registration under the step 1 protocol; otherwise the direction of news as a signal stays closed (the decision of steps 2 and 2b), now on a sample many times larger.

## Step 2g. Pre-registration: news score after costs (criteria fixed before the run, 2026-10-18)

Question: does the weak AUC 0.523 (step 2v, 10-day horizon) turn into return after costs. One primary test, the rest descriptive, no multiplicity correction needed.

- **Data:** the scores of step 2v (`scores_h10.csv`, training until 2025-03-01, test 2025-03-16..2026-10-05, 19 085 pairs, 6 823 ticker-date pairs). No retuning: the classifier, the liquidity filter (10 million RUB) and the horizon are fixed by step 2v.
- **Aggregation:** a ticker's score on a date = the mean of the scores of its articles for that date. Label = 10-day excess return to the index (from the step 2v label).
- **Portfolio:** on each date with at least 10 scored tickers the top and bottom quintile are taken (k = floor(n/5)), equal weights, holding 10 trading days, a tranche opens every day independently. Tranche return = mean excess return of the top minus mean of the bottom.
- **Costs:** 0.15% per side of a trade (commission 0.05% + half-spread 0.05% + slippage 0.05%, as in preflight), i.e. 0.30% per round trip per leg and 0.60% per tranche (two legs). Short borrow is not counted (optimistic for the short leg).
- **Primary criterion:** the mean net tranche return > 0 and the lower bound of the 95% interval of the date block bootstrap (block 10 dates, 5000 repetitions) > 0.
- **Descriptive:** gross spread, break-even costs, returns by the five quintiles (monotonicity), net return of the long leg only (top quintile minus 0.30%), breakdown by quarters, sensitivity to costs of 0.10% and 0.20% per side.
- **If the criterion is not met:** news stays closed as a trading signal, the branch is closed; revisiting only with a new independent data period.
- **If met:** this is grounds for a shadow run, not for trading. An independent test on the period after 2026-10-05 and accounting for short borrow and liquidity limits on the number of securities are required.

**Result of step 2g (2026-10-18):** the criterion is not met, numbers in `docs/metrics.md` (section step 2g). Net spread +0.263% per tranche, CI95 [-0.305, +0.930]; the long leg after costs gives +0.045%. The news branch is closed as a trading signal.
