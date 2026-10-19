# Seven hypotheses to improve the forecast (2026-10-18)

Russian translation: [20261018-seven-hypotheses.ru.md](20261018-seven-hypotheses.ru.md). This file is canonical.

Order: 1 H2 (zero-shot LLM direction) and H3 (lookahead), 2 H1/H4 (decay of the reaction to news), 3 H6 (LLM scores as features), 4 history depth and 2021-2024 news in the ensemble, 5 variance of the evaluation procedure itself, 6 IMOEX regime filter, 7 cross-sectional ranking. The criteria of each step are fixed in a separate commit before it is run. The `AGENTS.md` constraints do not change: the LLM is offline only, `ensemble_model.json` is not touched, metrics go into `docs/metrics.md` in the same commit as the code.

## Step 1. Pre-registration: zero-shot LLM direction on Russian headlines (H2) and lookahead (H3)

Question: does qwen3.8 (digest `3f181bb5737e`, ai-box, temperature 0, no thinking) predict the sign of the excess return from a single headline better than chance.

Tool: a new command `cmd/llmdirection` (read-only, CSV). The headline and ticker go into the prompt, the model answers JSON `{"direction":"up|down|neutral","confidence":1..5}`; the score = the sign of the direction times confidence (up 1..5, neutral 0, down -1..-5). The prompt contains no prices, dates or dynamics.

Samples (seed 42, dedup by `ticker+article_id`, at most 5 articles per ticker-day, the 17 trading tickers of the config, publication before 2026-10-07 minus 12 trading days for the 10-day horizon):
- A (clean): 2026-04-01..2026-09-30, up to 1200 articles. The model was trained before these dates and could not have seen the outcomes.
- B (before the cutoff): 2023-01-01..2024-06-30, up to 1200 articles. The exact training cutoff of qwen3.8 is unknown; the sample is needed only for H3.

Label: the sign of the excess return versus IMOEX, entry at the open of the next trading day after publication, exit at the close after h trading days, no deadband (as in `BuildNewsLabels`). Horizons h = 1, 5, 10. For both h, additionally and descriptively: AUC with a 0.5% deadband and on the subsample without neutral.

Primary H2 verdict for sample A, separately by horizon:
- h = 1 ("short"): AUC >= 0.55 and the lower bound of the 95% CI of the block bootstrap by publication date (2000 repeats, seed 42) above 0.500, with >= 200 labelled events and >= 40 dates.
- h = 10 ("for the strategy"): the same threshold. The trading path holds a position for about 10 trading days, so only passing h = 10 opens the way to step 3 and beyond; passing h = 1 alone means "there is a signal, but it is shorter than the strategy horizon" (link to H14).
If both thresholds fail, the result is recorded as "zero-shot LLM direction on Russian headlines is no better than chance", and H6 and H9 are closed without being run.

Extrapolation control (H5 for direction): Spearman correlation of the LLM score with the ticker's return over the previous 5 trading days (close pub-5 .. close pub). If |rho| > 0.3, the AUC is considered confounded with momentum and the AUC after alignment is also reported: within terciles of the previous return.

H3 verdict (lookahead): AUC(B) - AUC(A) at h = 10 with a bootstrap by dates (difference of independent samples, 2000 repeats). A leak is acknowledged if the difference is >= +0.030 and the lower bound is above zero; then results on the old archive cannot be used for calibration or training. If AUC(B) is lower than or equal to AUC(A), no lookahead is detected.

Side reports: the share of neutral, the distribution of confidence, the share of unparseable answers (must be < 2%, otherwise the run is repeated with a fixed parser before AUC is read).

## Step 2. Pre-registration: decay of the reaction to news by horizon (H1) and liquidity/sign (H4)

Question: if the news score has an effect, at which horizons does it live and is it concentrated in illiquid names and on the negative side. Step 2c already showed AUC 0.4963 at the 1-day horizon and 0.5212 / 0.5234 at 5 / 10 days (no decay); here the same score is tested not by AUC but by the return of a portfolio built from ranks, to get the size of the effect by horizon and by cut. One primary test, the rest descriptive.

Data and score (no re-tuning): `data/news_history.jsonl` in its current revision; the classifier is retrained with `cmd/trainnewsmodel` using the same flags as step 2c (`-split 2025-01-01 -horizon-days 10 -vocab-size 3000 -min-doc-freq 3 -max-tickers-per-article 3 -min-turnover 10000000`, the "MOEX site news" source is excluded from the training file); test = pairs from 2025-03-01 to 2026-09-30, the same two exclusions, pairs whose headline is in the training set for the same ticker removed, liquidity on the publication date not below 10 mln RUB by the 20-day median turnover. The ticker's score on a date = the mean of the scores of its articles.

Labelling (new command `cmd/newsdecay`, internal package `internal/newsstudy`): entry at the open of the next trading day after the publication date in MSK, exit at the close after h trading days, excess return versus IMOEX. Only days with an IMOEX bar count as trading days (weekend stock bars are dropped; `newsvalidate` in step 2c loses part of the sample because of them, so the numbers will not match 2c exactly). Horizons h = 1, 2, 5, 10, 20.

Portfolio: on each date with at least 10 scored tickers the weights are w_i = (rank_i − mean rank) / Σ|rank − mean rank|, the sum of absolute weights is 1; the date's return = Σ w_i · excess_i(h). This is a cross-sectional spread without costs (gross). Per-day normalisation: spread_h / h.

Primary criterion (H1): the gross spread at h = 5 is strictly positive: the mean over dates > 0 and the lower bound of the 95% CI of the block bootstrap by dates (block of 10 dates, 5000 repeats, seed 20261018) > 0, with >= 100 dates. If it is not met, the result is recorded as "the reaction to news with a 5-day hold is indistinguishable from zero" and H1 is closed.

Descriptively: the spread for all h with CI; the per-day spread spread_h / h (decay, if the per-day value at h = 1 or 2 is more than twice that at h = 10); comparison of h = 1 with h = 10 (difference with bootstrap).

H4 (descriptive, one run per cut, no correction): tickers are split into terciles by the median turnover over the test period; for each tercile the h = 10 spread (on each date >= 5 tickers of the tercile, otherwise the date is skipped); the difference lower minus upper tercile with bootstrap. Sign: the mean excess return of the lower quintile and of the upper quintile of the score (h = 10) separately, and their asymmetry |lower| − upper. For H4, "significant" = the lower bound of the 95% CI of the difference above zero.

Consequence: H4 can only narrow the hypothesis for a future pre-registration; the step gives no trading conclusions (costs are already closed by step 2d).

## Status of steps 1 and 3 (2026-10-18)

Step 2 is done (`docs/metrics.md`, commit 4b19567). Step 1 (H2/H3) is started: `cmd/llmdirection` is ready and checked, 550 of the 1200 queries of sample A are collected, but Ollama on the ai-box is busy with someone else's load (an empty query waits 10-20 s even with the process stopped), and per `AGENTS.md` parallel jobs on qwen3.8 are forbidden. The run is stopped and resumable (the CSV is appended by the key `ticker|article_id`); the H2/H3 verdict and step 3 (H6, depends on H2) are deferred until Ollama is free. The execution order of steps 4-7 does not depend on the LLM, so they go before step 3.

## Step 5. Pre-registration: variance of the evaluation procedure itself (criteria fixed before the run)

Question: how large is the walk-forward AUC noise from choices unrelated to the hypothesis (LightGBM seed, fold boundary, training start, fold length). Depth step 3 showed a spread of up to 0.08 AUC from dropping the last 20% of training; all differences of 0.01-0.02 in steps 1-3 are not interpreted before this check.

Harness: `evalh.py` `wf()` and `fit_predict()` with no change in logic (LightGBM + logreg ensemble, 15 price features, `label_date < start`, 10-day horizon, `depth/h10.csv` on the ai-box, 17 tickers without FX), parametrised through `scripts/procvar.py` (a Python research script on top of the existing harness; the project has no Go LightGBM training). Base configuration: seed 42, first test 2025-04-01, fold 63 days, training start 2024-01-01.

Axes: seed {0..4}; shift of the first-test boundary {0, 27, 54} days; training start {2023-07-01, 2023-10-01, 2024-01-01, 2024-04-01, 2024-07-01}; fold length {42, 63, 84} with everything else at base.

Runs:
1. Full grid 5 seeds x 3 shifts x 5 starts = 75 configurations of recipe A.
2. Recipe D (training from 2015-01-01) on a grid of 5 seeds x 3 shifts = 15 configurations, paired with A by seed and shift.
3. OFAT over fold length: 3 configurations.

Measures: pooled AUC and mean AUC over folds for each configuration.

Conclusions fixed in advance:
- SD of pooled AUC by seed (everything else at base), by training start, by boundary shift, and the joint SD over grid A.
- N95 = the 95th percentile of |difference of pooled AUC| between pairs of A configurations with the same boundary shift (the same test rows), different seeds and/or training starts. This is the noise threshold for comparing two recipes on the same test rows.
- Paired difference D minus A: mean, SD, min, max, share positive, over 15 pairs.
- Interpretation rule: if N95 > 0.010, all earlier results with |ΔAUC| < N95 (depth steps 1-3, experiments A-E of 2026-10-04 by AUC, macro features) are declared undetermined, and new experiments are compared only as the mean paired difference over at least 15 configurations with a CI. If N95 <= 0.005, the earlier noise band of +-0.005 is confirmed. Between 0.005 and 0.010 the band is updated to N95.
- The step has no pass/fail criterion: it is a calibration of the ruler.

## Step 6. Pre-registration: IMOEX regime filter as a risk gate (H11; criteria fixed before the run)

Question: if new buys are forbidden on days of a sharp IMOEX fall, does the maximum drawdown fall without losing most of the income. Macro as a model input was rejected (2026-10-10); here macro is only a decision mask.

Data: six purged walk-forward windows `~/moex-trader-task10/wf-volscale/*/window.json` (2025-03-05..2026-09-16, ensemble probabilities `ens-emb10-<window>.json` by dates and tickers; the probability does not depend on vol-scale). Replay: `cmd/backtest -signal-source csvprob -csv-prob-col p_ens -csv-buy-pct 0.60 -csv-sell-pct 0.40` with the Task 5/volscale run parameters (17 tickers, deposit 1M, `-max-lots 1000 -target-notional 15000 -lookback-days 0`, commission/spread/slippage 0.0005 each, default kill switch), per window.

Reproduction check (before any mask): the sum of realized over the six windows for csvprob without a mask must lie within ±10% of the Task 5 control (+59829.47 per `docs/metrics.md`). If not, the step is recorded as "a replay from decision logs does not reproduce the control", the comparison of the mask with the csvprob base stays descriptive and no verdict is issued.

Masks (new command `cmd/regimemask`, IMOEX from ISS, trading day = a day with an IMOEX bar; dates without an IMOEX bar, including weekend stock sessions, are not counted as risk-off):
- R1 (primary): IMOEX close-to-close return on the decision day d (known at the close of d, like the features) <= -2.0%.
- R2 (secondary): IMOEX return over 5 trading days up to the close of d <= -5.0%.
Action: decisions with probability >= 0.60 (BUY) on such dates are removed from the CSV (the signal becomes HOLD); SELL and HOLD are not touched, existing positions are not closed.

Primary criterion (R1 versus csvprob without a mask), all three conditions:
1. the sum of realized net over the six windows >= 0.80 of the base;
2. the worst-by-window maximum drawdown <= 0.90 of the base's worst drawdown;
3. informativeness: at least 10 risk-off dates in the windows and at least 5 removed BUYs.
The thresholds -2.0% and -5.0% are not tuned; variants -1.5% / -3.0% per day are descriptive. Consequence: passing gives the right to a pre-registration of building it into `internal/risk`, but not the building itself.

## Step 7. Pre-registration: cross-sectional ranking on the OOS probabilities of the ensemble (criteria fixed before the run)

Context that does not need rediscovering: the xsec/csvprob direction was closed by the owner's decision of 2026-09-17 (realized near zero, DD 4.26%), the excess-to-IMOEX label was rejected on 2026-10-18 (realized +9669 versus +67816, mean val AUC 0.483 versus 0.517). The condition for reopening from `docs/knowledge/model-quality-history.md`: the realized/MTM split and a spread proxy. No new training is run here; the only check is whether the order of probabilities of the deployed recipe within a date carries information and whether it survives costs.

Data: OOS probabilities of the six `wf-volscale` windows (2025-03-05..2026-09-16, 17 tickers, 7689 decisions). Label: excess return versus IMOEX over h trading days, entry at the open of the next trading day, exit at the close (`internal/newsstudy.BuildOutcomes`, days without an IMOEX bar dropped), h = 1, 5, 10.

Portfolio: on each date with at least 10 tickers with a probability the weights are w_i = (rank_i − mean rank) / Σ|rank − mean rank|; date return = Σ w_i · excess_i(h); one tranche per day (as in step 2d). Costs as in step 2d: 0.15% per side, i.e. 0.60% for a long-short tranche; for long-only top quintile 0.30%. Short borrow is not counted (optimistic). Block bootstrap by dates (block 10, 5000 repeats, seed 20261018).

Primary criterion: at h = 10 the mean net return of the long-short tranche (gross rank spread minus 0.60%) > 0 and the lower bound of the 95% CI > 0 with >= 100 dates. Descriptively: the gross spread and CI for h = 1, 5, 10; the mean excess return of the top and bottom quintiles; comparison of the gross spread by probability ranking with the gross spread by a moderate control (inverting the order gives a mirrored result, so a separate control is not needed).

Passing gives the right to a pre-registration of training for a rank target with the same protocol; failing closes the direction also for the ready recipe.

## Step 4. Pre-registration: deep history as a walk-forward candidate with costs (criteria fixed before the run)

Question: does the only robust positive result of step 5 (paired difference D minus A = +0.0203 pooled AUC, 15 of 15) survive a test by return with costs. A news feature is not added to the ensemble: the condition of the second stage of step 2 of the data-depth plan (val AUC of the news classifier >= 0.530) is not met, and in 2015-2024 training it would otherwise learn on a series that the live path zeroes (`untrainedFeatures`). Step 2 of this plan (`internal/newsstudy`) shows a weak gross spread of the score, but not a model input.

Data: `~/moex-research/depth/h10.csv` on the ai-box (17 tickers without FX, horizon 10, deadband 0.5, label absolute, 15 price columns; news and event columns are zero), with two added zero columns `negotiations_signal`, `sanctions_signal` so that the feature order matches the deployed one (28 names). Arms: A = rows from 2024-01-01, D = rows from 2015-01-01; the test windows and their training boundaries are the same as in Task 5 (`run-task5.sh`): q1 train_till 2025-03-04, q2 2025-06-05, q3 2025-09-05, q4 2025-12-03, w2 2026-03-04, w1 2026-06-06; `ENSEMBLE_EMBARGO_BARS=10`, 18 features, `scripts/export_ensemble.py` unchanged, stack as in `scripts/requirements-ensemble.txt`.

Replay: the `backtest` binary and `config.sandbox.yaml` of Task 5, Task 5/volscale run parameters (`-target-notional 15000 -max-lots 1000 -lookback-days 0`, commission/spread/slippage 0.0005 each, deposit 1M, kill switch on, no `-news-history`), one window per run. The control is arm A on the same machine and in the same run, never the numbers from the Task 5 archive (stack drift was recorded on 2026-10-18).

Candidate criterion (all five, arm D versus arm A):
1. the sum of realized net over the six windows D > A;
2. realized D >= A in at least 4 of 6 windows;
3. the worst-by-window maximum drawdown D <= the worst drawdown of A, and the number of windows with a triggered kill switch for D is no more than for A;
4. mean val AUC over windows D is not lower than A by more than 0.005;
5. the sum of realized D > 0.
One seed (42): step 5 showed an SD by seed of 0.0019, much smaller than the criterion.

Consequence: passing gives the right to ask the owner to change the training window in the daily candidate; D does not enter the deployed model automatically (the `AGENTS.md` constraint).

## Step 8. Pre-registration: analysis of September-October 2026 (why the models show a loss; the questions and attribution rules are fixed before looking at results)

Owner's request of 2026-10-18: understand what happened in September-October that made the forecast models "break" and give a loss. Only public data and our own logs, no search for closed information.

What is already known before the start (not a result of this step): the live sandbox since 2026-09-16 (the audit was reset that day) hardly trades after 09-18: a handful of trades per day (09-17..09-18 about 120 BUY/SELL decisions, then 0-2 per day); `docs/knowledge/model-quality-history.md` records that p >= 0.60 occurs in 0.11% of live decisions and the max p was 0.597 in the 10-10..10-18 replay; step 5 showed the pooled AUC of recipe A below 0.5 (0.483).

Data: (a) the audit of `trader-sandbox.db` (stage `llm` with p in `reasoning`, stage `executor` with executions); (b) a replay of the deployed `ensemble_model.json` in the shape of the preflight gate (17 tickers, deposit 1M, costs 0.0005 x 3 + spreads from `trader-sandbox.db`, borrow 0.00005/day, limits 90000/90000, kill switch, no news) for 2026-07-20..2026-10-18 and by month; (c) `data/news_history.jsonl`, `data/dividends.jsonl`; (d) IMOEX from ISS.

Questions and measures:
- Q1 Where is the loss in time and by ticker: realized by ticker and by week from the replay (b), long and short separately.
- Q2 Beta: IMOEX return over the period and the book's mean net position; decomposition of realized into an IMOEX component and an excess one (as Task 9). Hypothesis: a book with a short tilt lost beta income when IMOEX stopped falling.
- Q3 Dividends: for positions that crossed an ex-date in the period, the P&L difference with dividend-adjusted and unadjusted prices. Hypothesis: register-closing gaps give a loss to long positions.
- Q4 Events: for the ten worst closed trades (b) list the archive headlines (sources >= 2) in a window of +-1 trading day from entry and from exit, and dividend ex-dates. Descriptive, no causal conclusions.
- Q5 Signal availability: the share of decisions with p >= 0.60 and p <= 0.40 by week, live and in the replay; comparison with the share in the Task 5 test windows.
- Q6 Replay versus life: the number and direction of live trades 09-16..10-18 versus the replay for the same interval.

Attribution rule (so that the analysis does not turn into fitting an explanation): a cause is considered to explain the period's loss only if the counterfactual correction (remove dividend gaps; remove the beta component; remove the worst 3 trades) changes the period's realized by at least 50% of its magnitude. The rest is recorded as "does not explain". If no cause passes, the outcome is recorded as such.

## Step 9. Pre-registration: why live probabilities fell below 0.4 (criteria fixed before the run)

Fact from step 8: in weeks 39-41 the share of decisions with p <= 0.40 was 81-95%, and p >= 0.60 about zero, while IMOEX rose by 22%. The model `ensemble_model.json` was trained around 2026-09-16 (last commit 2026-09-18), i.e. it is about a month old.

Hypotheses and rules (one evaluation window 2026-09-16..2026-10-07, limits of the dataset `depth/h10.csv`):
- P1 (feature extrapolation): a replay of the deployed model gives probabilities by ticker-date (`cmd/backtest -wf-dir`); by week the mean p and the mean cross-sectional values of mom_63d, dist_ma50_pct, rsi_14 (dataset) are computed. P1 is accepted if the Spearman correlation of the weekly mean p with one of these features over the window 2026-07-20..2026-10-07 is >= 0.6 in absolute value with >= 8 weeks, and in the window the feature goes beyond the 95th percentile of the training range (2024-01-01..2026-09-16) in >= 25% of observations.
- P2 (ageing): a new model of recipe A with `train_till` 2026-09-16 (the same stack and procedure as step 4) is rescored on the same decisions; P2 is accepted if the share of p <= 0.40 of the fresh model in the window is no more than half of the deployed model's share in the replay.
- P3 (base-rate shift): the share of labels 1 (10-day rise) in the training sample versus the share in the window 2026-09-16..10-07 by labels available by 2026-10-07; P3 is accepted if the difference of absolute shares is >= 15 pp.
All three may hold simultaneously; if none holds, the outcome is recorded as such. New data on live probabilities: the audit of `trader-sandbox.db`, stage `llm`.

## Step 10. Pre-registration: R2 (IMOEX over 5 days <= -5%) on windows not used in step 6 (criteria fixed before the run)

Question: does the effect of variant R2, which gave +28% realized in two windows in step 6, repeat on independent windows. The primary variant here is R2 (the -5.0% threshold does not change), R1 is only descriptive.

Windows (purged walk-forward, four quarterly, before the Task 5 windows): p1 test 2024-03-05..2024-06-05 (train_till 2024-03-04), p2 2024-06-06..2024-09-05 (2024-06-05), p3 2024-09-06..2024-12-03 (2024-09-05), p4 2024-12-04..2025-03-04 (2024-12-03). Training: `scripts/export_ensemble.py` of Task 5, rows of `depth/h10.csv` from 2021-01-01, `ENSEMBLE_EMBARGO_BARS=10`, 18 features, 28-name order, seed 42 (as step 4). This is not a check of the recipe but a check of the mask on OOS probabilities.

Replay: the same as step 6: first `cmd/backtest -signal-source ensemble -wf-dir` on each window (probabilities), then `cmd/regimemask` and a `csvprob` replay without a mask (control) and with R2 (and R1 descriptively); 17 tickers, deposit 1M, `-target-notional 15000`, commission/spread/slippage 0.0005 each, kill switch on. Reproduction check: realized of csvprob without a mask on each window within 10% of the realized of the ensemble replay on the same window (otherwise the window is excluded and this is recorded).

Criterion (R2 versus csvprob without a mask), all three:
1. the sum of realized net over the four windows >= 0.80 of the base;
2. the worst-by-window maximum drawdown <= 0.90 of the base's worst drawdown;
3. informativeness: at least 10 risk-off dates in the windows and at least 5 removed BUYs.
Additionally, the combined result over ten windows (six from step 6 + four new), descriptively. Passing gives the right to a pre-registration of building it into `internal/risk`, but not the building itself.

## Part B (2026-10-19): six additional hypotheses in order, criteria fixed before the run

Owner's request: check in order feature normalisation, labels in volatility units, a meta-model, new public data, fundamental data from company cards, and TimesFM. Public data only.

Common protocol (steps 11-13 and 16):
- Dataset: `~/moex-trader-daily/work/data-daily.csv` on the ai-box (export of 2026-10-18, `exportdataset -from 2024-01-01 -horizon-days 10 -deadband-pct 0.5 -label-mode absolute`, 18 tickers, the nightly job recipe). Prices: `cmd/pricehist` (ISS, daily bars, 18 tickers and IMOEX). The row's forward return: close on `label_date` to the open of the next bar after `date`; reconciliation with the dataset labels (sign match in at least 99% of rows, otherwise the step stops).
- Base model: the mean of LightGBM(300, lr 0.03, 15 leaves, colsample 0.8, subsample 0.8) and logistic regression (C=1) on 15 price features (`PRICE` from `scripts/procvar.py`); new features are added to them.
- Purged walk-forward: test folds of 63 days, first test 2025-04-01, shifts 0/27/54 days, five seeds (0..4): 15 configurations; training on `label_date < fold start`. The same grid for base and variant; comparison pairwise by configuration.
- Evaluation noise (step 5): a difference of single AUCs below 0.016 is not informative; therefore the criterion below relies on pairwise differences over 15 configurations.
- A single evaluation target: the original dataset label (10-day direction, 0.5% dead zone), even when the variant is trained on a different label. Secondary metric: Spearman between p and the row's forward return.
- The decision "variant is better than base" (first stage): the mean pairwise difference of pooled AUC >= +0.005 and positive in at least 12 of 15 configurations (sign test p < 0.02). Second stage (only for those that passed the first): a `csvprob` replay in the same windows as step 4 (realized not below base, drawdown not above base by more than 0.5 pp). Passing gives the right to a pre-registration of building it in, but not the building itself nor replacing `ensemble_model.json`.

### Step 11. Feature normalisation by cross-section
Variant: each price feature is replaced by its rank within the date (the share of tickers with a smaller value, from 0 to 1) and features relative to IMOEX are added: mom_5d/mom_21d/mom_63d of the stock minus the same for IMOEX. Ranks are computed only from the data of the same date, with no future. The rest per the common protocol. Additionally, descriptively: the share of test-fold rows outside the training range at z > 12 for base and variant.

### Step 12. Labels in volatility units
Variant: the label is 1 / 0 if the forward return is above / below +/-0.5 sigma (sigma = standard deviation of the daily return over 20 days, scaled by sqrt(10), from bars before the decision date), rows inside the zone are dropped; base features. Training on the new label, evaluation on the original one (common target). Additionally, descriptively: the number of training rows and the share of class 1.

### Step 13. Meta-model (meta-labeling)
Primary signal: the base's p. The meta-model (logistic; features: |p-0.5|, realized_vol_21d_annualized_pct, mom_21d, IMOEX 5d, dist_ma50_pct) predicts whether the sign of (p-0.5) matched the original label; it is trained on the base's internal out-of-fold predictions in the training window (time split inside the window, purge 10 days). Comparison: the mean forward return in the signal direction on the 30% of rows with the highest meta-model confidence versus the 30% with the highest |p-0.5| of the base (the same number of rows, 15 configurations). The variant is better if the difference is >= +0.10 pp per trade on average and positive in at least 12 of 15 configurations.

### Step 14. New public data: open interest on the MOEX derivatives market
First an availability check: ISS `analyticalproducts/futoi` (positions of legal entities and individuals in futures) without a token, history not shorter than 2024-01-01 for the IMOEX future (MX) and for at least 5 of 18 stocks. If unavailable without a paid subscription (AlgoPack), the step is closed as "not verifiable on public data", with no workarounds. If available: features on date d from data of d-1 (the 5-day change of the net position of individuals and legal entities, normalised), common protocol. Index review dates are not taken into the check: the feature is common to all tickers and does not distinguish stocks (see the limitation of macro features in plan 20261010).

### Step 15. Fundamental data from company cards
A source check, not training. Questions: (1) which fields the T-Invest API gives (`GetAssetFundamentals` and the asset card) for 18 tickers; (2) whether the values have a publication date and a history sufficient for a point-in-time join from 2024-01-01. If there is no dated history, "as of today" values cannot be substituted into the past (leak); the result of the step: "not suitable for training", only a forward check on live data as a risk filter is possible. The token is read only by `cmd/tinvestprobe`; the token value is not printed.

### Step 16. TimesFM
Model: TimesFM (pretrained, no fine-tuning), a separate virtual environment on the ai-box (the pins of `scripts/requirements-ensemble.txt` do not change), CPU. Input: the series of closes of the stock up to and including the decision date over the last 512 bars (no future); output: the median forecast for 10 bars and quantiles. Features: the forecast 10-day return (median / last close - 1), the width of the quantile interval and the position of zero inside it.
- 16a (zero-shot): AUC of the forecast return against the original label and Spearman with the forward return on the test-fold rows of the common protocol, over 15 configurations (there is one model, only folds change); the signal is considered informative if pooled AUC >= 0.52 in 12 of 15 configurations and the 95% interval of the block bootstrap by dates (block 10) of the difference from 0.5 does not include 0.
- 16b (as a feature): base plus three features, common protocol and the first-stage criterion.
If the installation (package, weights) is unavailable from the ai-box network, the step is closed as "not run" with no workarounds.

Order: 11, 12, 13, 14, 15, 16. The results of each step go into `docs/metrics.md` in the same commit as the code.

### Step 17. Integration check of labels in volatility units (criteria fixed before the run, 2026-10-19)

Question: is the result of step 12 (AUC +0.0114, replay +11611 versus +4342 on a single seed) robust to the seed and does it hold on a window that did not take part in choosing the variant.

Data and model: as in step 12 (`scripts/hypo_wf.py`, the dataset and prices of steps 11-13, k = 0.5 sigma). Arms: base (label by the original 0.5% dead zone) and sigma (k = 0.5). Replay windows: the six Task 5 windows (q1, q2, q3, q4, w2, w1) and the holdout window h = 2026-09-17..2026-10-17 (not used in step 12; probabilities exist only for decisions whose label is known, i.e. roughly until the start of October, the other days of the replay are HOLD). Training of each window on rows with `label_date` before the window start, from 2024-01-01. Seeds 0, 1, 2, 3, 4 (affect LightGBM). The csvprob replay as in step 12: 0.60/0.40, 17 tickers, deposit 1M, 15000 RUB per position, costs 0.0005 x3, `max_net_short/long` limits 90000, borrow 0.00005, kill switch.

Criterion (sigma versus base, all four):
1. the sum of realized net over the six Task 5 windows, averaged over the five seeds, is higher for sigma than for base, and sigma is above base on this sum in at least 4 of 5 seeds;
2. the worst-by-window-and-seed maximum drawdown of sigma is no more than the base's worst plus 0.5 pp;
3. holdout window h: the seed-mean realized of sigma is not below base;
4. the seed-mean sum of realized of sigma over the six windows is positive.
Descriptively (not part of the criterion): k = 0.25 and k = 0.75 on seed 42 over the same seven windows; the number of windows where sigma is above base, per seed.

Passing gives the right to a pre-registration of training a candidate for manual replacement (the `export_ensemble.py` recipe with sigma labels, `exportdataset`, Go parity), but not the replacement itself: swapping `ensemble_model.json` stays a manual step of the owner.

## Part C (2026-10-19, overnight run at the owner's request): wide sample, pairs, carry. Criteria fixed before the run

Basis (literature, no invented details): Gu, Kelly, Xiu (Review of Financial Studies 2020, NBER w25398) and works on international markets (for example Machine learning goes global, Journal of Economic Dynamics and Control 2023) obtain a gain from ML on wide cross-sections and a monthly horizon, and the dominant signals there are momentum, liquidity and volatility. Our recipe works on 18 stocks with daily features and a 10-day horizon, i.e. on a small sample where such gains are not expected. Part C tests formulations that do not require forecasting the direction of a single stock on a small sample.

Constraints: public data only (MOEX ISS daily candles, the 130 most liquid TQBR stocks, from 2014-01-01, `cmd/pricehist`); the model is not replaced, the trader is not touched. Costs per side 0.15% (commission 0.05% + half-spread 0.05% + slippage 0.05%, as across the project), short borrow 0.005% per day on the short leg. The period 2022-02-25..2022-03-24 (stock trading halt) is excluded from the statistics in all steps and reported separately. Entry at the open of the day after the signal.

### Step 18. Cross-sectional factors on a wide sample
The universe on date t: stocks with at least 252 bars of history and a median (close x volume) over 60 bars of at least 30 mln RUB.
Signals at the close of t (all only from bars <= t): mom_12_1 = close[t-21]/close[t-252]-1; rev_1m = -(close[t]/close[t-21]-1); low_vol = -std of daily returns over 60 bars; high52 = close[t]/max(close[t-251..t]); low_max = -(maximum daily return over 21 bars).
Variants (four, multiplicity correction: threshold t >= 2.5):
- V1: a composite of five cross-sectional z-scores, equal weights, no training; long-short top minus bottom quintile, equal weights, rebalanced every 21 trading days.
- V2: the same composite, only long the top quintile minus the equal-weighted universe (excess).
- V3, V4: LightGBM regression (300 trees, lr 0.03, 15 leaves) on the same five signals plus their cross-sectional ranks, target = cross-sectional rank of the return over the next 21 bars (open t+1 .. close t+21), expanding window, retraining every 63 days, purge 21 bars, first test 2019-01-01; V3 long-short, V4 long minus universe.
Criterion for a variant (all four): (1) the mean net return per rebalancing period > 0 with a Newey-West t-statistic (3 lags) >= 2.5; (2) annual net Sharpe >= 0.5; (3) positive net return in at least 60% of calendar years (a year counts at >= 6 periods); (4) positive net return in the period 2022-06-01..2026-10-18. Descriptively: subperiods, turnover, gross return, a variant with rebalancing every 10 bars.

### Step 19. Common-preferred pairs
Pairs: all where both stocks trade on TQBR and have at least 1000 bars of history (the list is fixed before looking at results). Spread s = ln(P_common / P_preferred). z = (s - mean over 60 bars) / standard deviation over 60 bars, from bars <= t. Entry at the open of t+1 at |z| > 2 (buy the cheap, sell the expensive stock, equal amounts), exit at |z| < 0.5, or after 30 bars, or at |z| > 4 (stop). Capital is split equally between pairs. Costs 0.15% on each leg at entry and exit, borrow on the short leg.
Criterion (all four): net mean daily return per unit of capital > 0 with Newey-West t (5 lags) >= 2.0; annual Sharpe >= 0.7; positive in >= 60% of calendar years; positive in 2022-06-01..2026-10-18.

### Step 20. Night versus day
Decomposition of the daily return into night (close[t-1] -> open[t]) and day (open[t] -> close[t]) over the universe of step 18, by year. Rule: a night or day "buy every day" strategy can be a net profit only if the gross mean per day is >= 0.30% (round-trip costs 2 x 0.15%). Descriptive, no training.

### Step 21. Risk-free income threshold
Descriptive: the return of the money-market fund LQDT (TQBR) for 2025-04-01..2026-10-18 on 1 mln RUB next to the sums of realized from steps 11-17; the key rate from `data/macro/key_rate.csv`. Any result of parts B and C is compared with this threshold, not with zero.

Order: 21, 18, 20, 19. The results go into `docs/metrics.md` in the same commit as the code.

### Step 18b. Correction before the re-run: total return with dividends (fixed 2026-10-19 before the run)

Basis: in step 18 the close prices are not adjusted for dividends, so the register-closing gap is counted as a loss for the long leg and a profit for the short leg without receiving the payment. This is a systematic error that depends on whether the signals lean towards dividend stocks. Calendar: `cmd/dividendscalendar` (T-Invest API, 1148 events for 105 tickers, 2013-2026; in the early years coverage is incomplete, 44-54 events a year until 2016). Ex-date = the first trading day after `last_buy_date`; the net dividend (after 13% tax) is added to the numerator of that day's daily return: (close + dividend_net) / prev_close - 1.
Rule: total return becomes the main series for steps 18 and 20; results on price return (commit d600732) stay on record as superseded. The criteria of step 18 do not change (the same four variants, threshold t >= 2.5). Added descriptively: the contribution of each of the five signals separately (long-short quintile, net return).

### Step 22. Direct forward test of the composite (observation, not trading)
If step 18b does not reject the composite (V1 or V2), the composite signal is recorded forward daily from closed ISS bars into a file without trading (`cmd/factorshadow`), and is evaluated no earlier than after 6 monotonic rebalancings (about 126 trading days) by the same cost-calculation rules. This is the only way to get data that is not in the test period. The forward evaluation criterion is fixed in a separate pre-registration before the first evaluation.

### Step 23. Sequential check: choice of signal signs on 2016-2021, test on 2022-06..2026 (fixed 2026-10-19 before the run)

What is visible in step 18b (dividends accounted for) and what is fixed before the run. Results of step 18b on the full sample: no variant passed the threshold t >= 2.5 (V3 t = 2.29, V4 2.25, V1 1.78, V2 1.69). A breakdown by signal showed that `rev_1m` has the opposite sign on MOEX (long-short by `rev_1m` in 2016-2021 gives -1.47% per period, t = -2.46; after 2022 -2.16%, t = -3.13), i.e. short-term momentum continues rather than reverses; `mom_12_1` and `high52` are positive, `low_vol` and `low_max` in 2016-2021 are negative (t -1.62 and -1.62).
Disclosure: I had already seen the numbers after 2022-06 before writing this rule, so the test below is not purely blind. The selection rule uses only 2016-2021 data: keep the sign that has |t| >= 1.6 in 2016-2021 (t_NW of the long-short quintile), flip the signal when t is negative, exclude `low_vol` and `low_max` when t is negative, keep `mom_12_1` and `high52`. The final composite C2 = the mean of the z-scores of {mom_12_1, mom_1m = -rev_1m, high52}, with no other tuning.
Test: long-short quintile and long minus universe, 21 bars, costs as in step 18, period 2022-06-01..2026-10-18 (43 periods), total return. Criterion (all three, one test): (1) net mean per period > 0 and Newey-West t (3 lags) >= 2.0; (2) net annual Sharpe >= 0.5; (3) a positive net sum in at least 3 of 4 calendar years (2023, 2024, 2025, 2026). Descriptively: the same composite in 2016-2021 (where the signs were selected).
Role of the result: even if it passes, it is not a basis for trading but a basis for running step 22 (forward observation) with this composite.

### Step 24. Sample width for the project recipe: training on a point-in-time wide panel (fixed 2026-10-19 before the run)

Basis: in the literature the gain of ML comes with a wide cross-section; the project recipe is trained on 13 thousand rows (18 stocks, 2024-2026), a wide panel gives two orders of magnitude more. Experiment B of 2026-10-04 widened training to 42 stocks, here the panel is point-in-time and includes the 172 stocks that ever passed the liquidity threshold (including those that disappeared), from 2014-06.
Data: `exportdataset -candles-csv candles_clean.csv` (a new option to read candles from CSV; candles from TQBR board snapshots adjusted only for splits) yields the same 15 price features as the live path; label = the sign of the forward return (open t+1 .. close t+10), for training the rows inside the 0.5% dead zone are dropped, as in the base. Test: the same rows of the 18 trading tickers and the same 15 configurations (5 seeds x shifts 0/27/54, folds of 63 days from 2025-04-01, purge by `label_date`) as in steps 11-13.
Arms: base18 (training on the 18 trading tickers from 2024-01-01, as before, but on features from this panel) and wide (training on all 172 tickers from 2014-06). Descriptively: wide from 2020-01 and wide with LightGBM only.
Stage 1 criterion: the mean pairwise difference of pooled AUC wide minus base18 >= +0.005 and positive in at least 12 of 15 configurations (evaluation on the original label). Stage 2 only on passing: a csvprob replay with the same protocol as step 17 (six Task 5 windows + holdout window h, seeds 0-4, criteria 1-4 of step 17 unchanged).

### Step 22 (clarification, fixed 2026-10-19 before the first record): direct forward test of the C2 composite

Record: `scripts/factor_shadow.py` on the ai-box (`~/moex-shadow`, cron 16:30 UTC on weekdays) updates the TQBR board snapshot (`cmd/tqbrsnap`), computes the composite C2 = the mean of z{mom_12_1, 1-month momentum, high52} over the point-in-time universe (60-day median turnover >= 30 mln RUB, 252 bars of history) and writes the long and short quintiles to `shadow.jsonl`. Anchor 2026-10-19; rebalancing every 21 trading days; no trades and no orders. Evaluation: `scripts/factor_shadow_eval.py` (entry at the next day's open, exit at the close after 21 bars, costs 0.15% per side, borrow 0.005% per day on the short leg, total return with the dividend calendar as of the evaluation time).
Criterion (both legs, evaluation no earlier than 12 rebalancings, about 2027-10-19): net mean per period > 0, Newey-West t (3 lags) >= 2.0, annual Sharpe >= 0.5; the "long minus universe" and "long-short" legs are evaluated separately. An interim look after 6 rebalancings is descriptive, no decisions are made on it. These rules do not change after the first record. Caveat: the universe includes funds (ETFs/BPIFs) and preferred shares, as in the study; the study results were obtained under the same conditions.
Addition to step 22 of 2026-10-19, before the first rebalancing record: a third track `long_h52` is added to the record (long the top quintile by high52 alone, minus the universe; "H52-LO"). Reason: in the descriptive breakdown by signals (point-in-time universe, 2016-2026, 30 combinations of signals and regimes, none was a pre-registered decision) it was the most robust (net mean +1.05% per period, t 3.59, positive in 10 of 11 years, t 2.82 after 2022-06). For the three tracks (C2 long-short, C2 long minus universe, H52-LO) the t threshold is raised to 2.4 (three comparisons), the other conditions are the same.

### Step 24b. Quantile thresholds instead of 0.60/0.40 (fixed 2026-10-19 before the calculation; basis: the result of stage 2 of step 24 is informative only formally)

A fact known before this step: the stage 2 replay with thresholds 0.60/0.40 gave the wide arm 4-5 closed trades over six windows (base18 about 490, wide2020 11-17), i.e. the wide model hardly crosses the thresholds, like the deep model in step 4. Comparing sums of realized on 5 trades is uninformative, and formally passing the stage 2 criteria is not accepted as a result.
Variant: the signal is determined from the model's own distribution of probabilities. For each arm and seed on day t: BUY if p >= the 80th percentile of p over the previous 60 calendar days (all tickers, only past days), SELL if p <= the 20th percentile, otherwise HOLD; with fewer than 200 values in the history window the signal is HOLD. Applied to the arms base18, wide, wide2020 identically. A csvprob replay as in steps 17/24 (threshold 0.60/0.40 over the transformed values 0.9/0.1/0.5), the same windows, seeds 0-4, the same conditions.
Criterion (wide versus base18): (1) the seed-mean of the sum of realized over the six windows is higher and higher in at least 4 of 5 seeds; (2) the worst drawdown is no more than the base + 0.5 pp; (3) holdout window h: the seed-mean is no worse; (4) the sum is positive; (5) informativeness: at least 150 closed trades over the six windows on average over seeds. All five are required.

### Step 25. Linear model and neural network instead of LightGBM on the point-in-time panel (fixed 2026-10-19 before the run)

Basis: in Gu, Kelly, Xiu (2020) neural networks give the best result among methods on a wide panel, linear models serve as the lower bound. Step 18 tested only LightGBM (V3, V4).
Variants on the same data, features (the five signals of step 18 and their cross-sectional ranks), target (rank of the return over the next 21 bars), expanding window, retraining every 63 days and costs as V3/V4: V5 ridge regression (alpha = 10, feature standardisation), V6 an ensemble of five MLPRegressor (hidden layers 32 and 16, alpha = 1e-3, early stopping, five seeds, mean). For each model the long-short quintile and long minus universe, the point-in-time universe and the total return of step 18.
Criterion (as for V1-V4, but a family of six models): Newey-West t >= 2.6 (correction for six comparisons), net annual Sharpe >= 0.5, positive net return in >= 60% of years, positive after 2022-06. The result is recorded regardless of the outcome.
Addition to step 22 of 2026-10-19, before the first rebalancing record: a fourth track `long_ridge` is added (long the top quintile by the ridge model of step 25, minus the universe; "RIDGE-LO"), because in step 25 it is the only variant that passed all four pre-registered criteria (net mean +1.07% per period, t 3.35, Sharpe 1.11, 6 of 8 years, +1.19% after 2022-06). For the four tracks the t threshold is raised to 2.5, the other conditions are the same. Hedging with an index future is not evaluated in this record (descriptively, the track's long minus the IMOEX index return is evaluated).

### Step 26. Model of the MOEX universe: structural decomposition, lag links with external drivers, regimes, macro-conditional signal (fixed 2026-10-19 before the run)

Basis: the owner asked for a "model of the universe that will explain the behaviour of the market and understand the causes of moves", allowing even an LLM on the critical path. The honest formulation: causality is not proven from observational data, so "cause" here means a time-ordered link (the driver is known before the response) and out-of-sample predictive power. The model consists of four blocks; block M1 is descriptive, M2-M4 have criteria. The LLM is not connected to the critical path: in steps 1-25 no news/language input gave a verifiable gain, and adding an unproven input to the live loop contradicts the project invariants; this decision is revisited only based on the results of the blocks below.

Data: `scripts/world_fetch.py` collects daily series for 2013-2026: IMOEX and MOEX sector indices, RTSI, RVI (ISS), USDRUB (ISS, CETS USD000UTSTOM), the key rate (cbr.ru), Brent, S&P 500 (from 2016-10), VIX, UST10, DXY, EURUSD, Henry Hub (FRED). Stocks: the point-in-time panel of step 25 (`tqbr_snap.csv`, dividends `dividends_pit.jsonl`), total return, the same rules (exclusion of 2022-02-25..03-24, costs 0.15% per side). Macro series are aligned to the MOEX calendar via ffill of levels and the difference between MOEX trading days. A decision "at the close of day t" uses only values dated no later than t.

M1. Decomposition (descriptive, no criterion). For each year Y=2019..2026 parameters are estimated on the 504 days before 1 January of Y and applied to the days of Y. Nested models of a stock's daily return: A market (IMOEX), B = A + 5 statistical factors (principal components of A's residuals, estimated on training), C = A + macro (USDRUB, Brent, VIX, UST10, RVI, SPX with a 1-day lag, key rate change), D = B + C. Report: out-of-sample R² (overall and by periods before/after 2022-06), variance shares (market, statistical factors, macro, idiosyncratic). Goal: to show which part of the moves is explained by common drivers and which is not.

M2. Lag link of external drivers with the index. Target: the IMOEX return from the close of t to the close of t+1. Features at the close of t: SPX_t, the change of VIX, Brent, USDRUB, DXY, UST10, RVI, IMOEX's own lags (1, 5, 21 days), 21-day realised volatility, the key rate level. Ridge (alpha 10, standardisation), expanding window, retraining every 63 days, first test value 2019-01-01. Strategy: long IMOEX when the forecast > 0, otherwise cash at the key rate; costs 0.03% per unit of position change. Criteria (all four): (1) out-of-sample R² relative to the expanding mean > 0; (2) share of correct signs >= 53% and one-sided binomial p < 0.01; (3) Newey-West t (5 lags) of the net return of the strategy in excess of the key rate minus the buy-and-hold excess >= 2.6 (correction for three tests); (4) the same sum is positive after 2022-06. Separately, descriptively: who carries the signal (by absolute weight), and the result without SPX.

M3. Regimes. A two-regime Markov variance-switching model (statsmodels MarkovRegression, k=2) on the daily IMOEX return, parameters trained on an expanding window every 63 days, the regime is taken from the filtered probability (no smoothing over the future). Strategy: hold IMOEX if P(high variance) < 0.5, otherwise cash at the key rate; costs as in M2. Criteria (all three): (1) the Sharpe of the excess over the key rate is higher than buy-and-hold by at least 0.2 over 2019-01..2026-10; (2) the maximum drawdown is no more than 75% of the buy-and-hold drawdown; (3) after 2022-06 the Sharpe difference is positive.

M4. Macro-conditional signal for stocks. To the five signals of step 25 and their ranks, 4 features are added (USDRUB, Brent, VIX, UST10): the cross-sectional z-score of the product of the rolling (252 days) beta of the stock's relative return (stock minus IMOEX) to the factor, times the factor's volatility, by the factor's 63-day trend normalised by its volatility. Then as step 25: ridge, rank target over 21 bars, expanding window, rebalancing every 21 bars, long the top quintile minus the universe. Criteria (all four): (1) Newey-West t (3 lags) of net return >= 2.6; (2) the pairwise difference of net returns against the base ridge (step 25, the same dates) has Newey-West t >= 2.0; (3) positive net return in at least 6 of 8 years; (4) positive mean after 2022-06.

Verdict of the step: a block passes or not independently. No pass is a basis for trading or changing the live model; a pass of M2/M3 leads to a forward record, a pass of M4 to adding a track to `factor_shadow.py`. If blocks M2-M4 do not pass, the outcome remains M1: a measured decomposition of the explainable and unexplainable, i.e. the boundary of what can be known about the causes of moves.

Addition to step 26 of 2026-10-19, after running M1-M4 and before reading their conclusions in the final report: (a) a data defect was found before counting the verdicts: the USDRUB_TOM pair stopped trading on 2024-06-11, the series broke off and in the first run of M1/M2 the macro column fx was zeros from mid-2024 and the year 2025 dropped out of M2; fix: factor fx = log change of USDRUB until 2024-06-11 and of CNYRUB_TOM after, both blocks were rerun, the first run was replaced; (b) descriptive M5 was added (the share of large idiosyncratic moves |z|>3 that coincided with news on the stock on the same trading day; 2021-2024, where the archive is dense) and a comparison of the M1 statistical factors with MOEX sector indices; they have no criteria, they were added after viewing M1-M4.

### Step 27. A candidate passing the preflight gate on an honest out-of-sample window (fixed 2026-10-19 before the run)

Owner's goal: a model that passes the preflight gate. The gate checks the last 90 days, so fitting to one window proves nothing; the honest form: a candidate is considered passing if the gate is passed on a window that the model did not see in training, and on the previous windows as well. Signal: the ridge model of step 25 (trained on the point-in-time wide panel, expanding window, retraining every 63 days), scores at the close of t, rebalancing every 21 bars, the score is held until the next one. For the 17 trading stocks the score is converted into a probability via the percentile r in the full admitted universe: p = 0.5 + (r - 0.5)/3, so BUY at r >= 0.8 and SELL at r <= 0.2 with the usual thresholds 0.60/0.40. There is one main variant (ls); the variants bias (SELL only at r <= 0.1) and flip (reversed sign, control) are computed and reported but do not become the main one. The replay is the same as the gate's: 17 tickers, deposit 1 mln, 15000 RUB per position, costs 0.0005x3, net limits 90000, borrow 0.00005 per day, no news.
Windows: q1 q2 q3 q4 w2 w1 (as before) and G = 2026-07-20..2026-10-18 (the gate window). Passing criterion (all): (1) realized on G > 1 RUB with at least 1 closed trade (the gate's own condition); (2) the sum of realized over the six windows > 0; (3) at least four positive windows out of six; (4) the worst window not below -10000 RUB. Comparison with the base (the project recipe, five seeds): the same windows from `bt17/summary.csv`. A pass gives no right to replace `ensemble_model.json`: replacement stays manual under the AGENTS.md rules; a pass only means that the candidate can be assembled as a signal source and followed in observation.

Result of the main variant of step 27 (fixed before step 27b): ls passes criteria 1 (G +3916 RUB), 2 (sum of six windows +684 RUB) and 4 (worst -2697 RUB), but not 3 (positive windows 3 of 6: q2, q3, q4; q1 -1.8 RUB): formally not passed. The flip control is negative on G (-4491) and in the sum of windows.

### Step 27b. Filtering the project recipe with the ridge score (fixed 2026-10-19 before the run)
Basis: the project's daily recipe gives frequent trades (noise), ridge gives rare ones. Hypothesis: the direction of a recipe trade is allowed only if it agrees with the ridge percentile r (BUY at r >= 0.5, SELL at r <= 0.5), otherwise p = 0.5. Base: the project recipe, five seeds (base@0..4), training on 18 tickers from 2024-01-01, window G gets a separate training up to 2026-07-19. The replay and windows as in step 27 (q1 q2 q3 q4 w2 w1 G). Criterion (by the mean of five seeds): the same four as in step 27, plus (5) the seed-mean of the sum of six windows is higher than the base without the filter and higher in at least 4 of 5 seeds. One main variant; other r thresholds are not tried.

Result of step 27b (fixed before step 27c): the filter does not pass. The project recipe without the filter on window G is negative for all five seeds (mean -11884 RUB, range -19440..-4291), the sum of six windows by seed from -13842 to +17461 (mean +1681); with the filter G mean -3303 (negative for all seeds), the sum of six windows mean -6688, below the base: criteria 1, 2 and 5 are violated.

### Step 27c. Gate pass rate on rolling 90-day windows for ridge ls (fixed 2026-10-19 before the run)
Basis: the gate on a single window G is too small to distinguish skill from luck. Windows: 90 calendar days, starts every 15 days from 2025-03-05 to 2026-07-20 (end no later than 2026-10-18); the signal is the same (p_ridge_ls, the main variant of step 27, nothing is tuned); the replay as in step 27. Criterion: the share of windows with realized > 1 RUB and at least 1 closed trade is not below 70%, and the sum of realized over the windows is positive; the flip control (reversed sign) must show a noticeably lower share (lower by 20 pp or more). The windows overlap, so this is an estimate of frequency, not independent trials.

### Step 28. Reporting events and T-Invest dividend history (fixed 2026-10-19 before the analysis)

What changed in the data. With a "view only" T-Invest token and the instrument uid (not the asset uid), the methods `GetAssetReports` and `GetDividends` return history: report publication dates (`reportDate`, period) and dividends with announcement dates, register-closing dates and yield. The early code of step 18 wanted `assetUid`, so it got code 3. Fundamental ratios (`GetAssetFundamentals`) remain a snapshot as of a date, their history is not given: the experiment with them runs on the accumulated `data/fundamentals.jsonl` no earlier than after ~120 daily snapshots. The AlgoPack token was rejected by the server as unsubscribed (`iss_permissions=0`, the reply "information available only to subscribers"), so hypotheses on order flows and tradestats are not tested here.
Data: `cmd/reportsfetch` collects, for all 719 tickers of the step 25 panel (where the instrument is still found by ShareBy), reporting events for 2014-2027 and dividends. Survivorship: disappeared instruments without records are absent, so events are taken only for those found; for event tests this biases the sample towards survivors, but does not create look-ahead inside the event window; the caveat is entered into the metrics. The analysis uses only events with a known publication date; future dates from the schedule are used only as known in advance.

Blocks (total return and the point-in-time panel, exclusion of 2022-02-25..03-24, no costs, abnormal return = the stock's return minus the equal-weighted return of the admitted universe of the same day):
E1. Reaction: the mean abnormal return of the window [0,+1] trading days around reportDate (close t-1 to close t+1) and the ratio of the mean absolute value to an ordinary day. Descriptive.
E2. Reporting premium: the mean abnormal return of the window [-2,+1]. Criterion: t clustered by date >= 2.6 and a positive mean in >= 6 of the 9 years 2017-2025.
E3. Reaction continuation (PEAD by reaction): events are split into terciles by the abnormal return [0,+1]; drift = abnormal return of the window [+2,+21]; criterion: top tercile minus bottom > 0 with t (clustered by date) >= 2.6 and a positive difference in >= 6 of 9 years.
E4. Filter for ridge: for the long book of step 25 (top quintile by ridge), at each rebalancing mark the stocks with an expected publication within the next 21 trading days (known from the schedule). Criterion: excluding such stocks from the book gives a net long-minus-universe return with Newey-West t >= 2.6 and a pairwise difference against the book without the filter with t >= 2.0.
E5. Dividend yield factor: the rolling sum of dividends with a register-closing date (lastBuyDate) over the last 365 days divided by the close price; quintile long minus universe (21 bars, costs 0.15% per side). Criterion: Newey-West t >= 2.6, Sharpe >= 0.5, >= 60% positive years, positive mean after 2022-06. Additionally: the feature is added to ridge (as in step 26 M4), pairwise difference criterion t >= 2.0.
Family: E2, E3, E4, E5 (the threshold 2.6 accounts for multiple comparisons). Passing a block gives only a record for forward observation; the live model does not change.

### Step 29. FUTOI with an AlgoPack key: positions of individuals and legal entities since 2020 (fixed 2026-10-19 before the analysis)

Data: the AlgoPack key (starter package) opens `apim.moex.com/iss/analyticalproducts/futoi/securities/<series>.json` without the 14-day delay and with history since 2020 (the free ISS gave 2023-12 and later with a delay); candles and trades with a 15-minute delay are opened by the same key, while `datashop/algopack/eq/*` (tradestats, obstats, hi2) stays "subscribers only". `cmd/futoihist` collects the 18:45 (stock close) and 23:50 snapshots for 25 series (index, currency, commodity and single-stock futures) by groups FIZ/YUR. Earlier futoi steps (futoi, futoi14) used 2023-12.. and lagged dates and showed no effect; here the sample is two to three times longer and without lag.
Feature: the share of the net position of individuals r = pos_FIZ / (pos_long_FIZ + |pos_short_FIZ|) at the 18:45 snapshot of day t, its 5-trading-day change dr, the z-score of dr over an expanding window (at least 250 days).
F1. Index: series MX (the IMOEX future), target IMOEX t -> t+5 (non-overlapping five-day periods, decision at the close of t, costs 0.03% per position change). Position = -sign(z) (contrarian to individuals; the sign is fixed before the run, the opposite sign is not tried as the main one); test 2022-06-01..2026-10-16. Criteria (all): Newey-West t (2 lags) of the strategy's net return in excess of the key rate minus buy-and-hold >= 2.6; share of correct signs >= 54%; a positive mean in each of the two halves of the test.
F2. Stocks: for the single-stock series (SR, GZ, LK, GK, RN, TN, MG, VB, YD, AF, AL, MT, NM, NV, MM, X5, HY, SN, PZ) a regression of the abnormal return of the underlying stock t -> t+5 (minus the equal-weighted universe) on z(dr) of the stock in a stock-date panel; standard errors clustered by date. Criteria (all): the coefficient is negative (contrarian) with |t| >= 2.6 and the same sign in both halves of the sample 2020-2023 and 2024-2026.
Family F1, F2 (the threshold 2.6 accounts for two tests). Descriptively, with no criterion: the same features at the 23:50 snapshot and the level r instead of dr. Passing a block gives only a record for forward observation.

### Step 30. Intraday candles, calendar effects and cycles on long series (fixed 2026-10-19 before the analysis)

Basis: the owner asks to test hypotheses on intraday candles and trades and to look for cyclicality and seasonality on data "for 100 years". Data: (1) ISS TQBR 10-minute candles from 2011-12-15 to 2026-10-16 for 30 SECIDs (SBER, SBERP, LKOH, GAZP, GMKN, ROSN, NVTK, TATN, TATNP, SNGS, SNGSP, MTSS, MGNT, PLZL, CHMF, MAGN, NLMK, VTBR, AFLT, ALRS, MOEX, PIKK, IRAO, HYDR, FEES, PHOR, RUAL, AFKS, YNDX, YDEX; YNDX and YDEX are glued as one company) via `cmd/intradayfetch`; (2) S&P 500 daily closes from 1927-12-30 (Yahoo, ^GSPC, a price index without dividends, a sample of 90 stocks before 1957; Saturdays were traded before 1952); (3) IMOEX daily closes from 1997-09-22 (ISS history, price index). Historical trades (tradestats, anonymised trades) are unavailable: ISS gives only the current day, the AlgoPack section is closed to the owner's key; therefore "trades" in this step are replaced by signed turnover from 10-minute candles (sign(close-open)*value), and recording trades forward is outside the step. Everything is out of sample relative to what the author has seen: before the run none of these series was looked at.

General rules. Returns are logarithmic by close, date gaps longer than 7 calendar days are zeroed (excluded). The date pairs 2022-02-25..2022-03-28 for stocks are excluded. t by Newey-West with 5 lags on a series aggregated by date, or Fama-MacBeth with 5 lags. Halves of the intraday sample: up to 2018-12-31 and from 2019-01-01. Costs: index strategies 0.03% per position change; intraday stocks 0.05% per side (an optimistic tariff for liquid stocks; for the live loop with 0.15% per side the verdict is described separately and is not applied at the entry to the live model). Ticker days are dropped if in the main session 10:00-18:30 there are fewer than 80% of the expected 10-minute bars (the expected number is computed as the median per ticker-year). Dividend gaps: for the overnight of I2 two trading days after each register-closing date (lastBuyDate from T-Invest) and |overnight| > 25% are excluded.

Block I (intraday, 10-minute candles; a family of five tests, threshold t >= 2.6):
I1. Intraday momentum. A trade per ticker-day: r_first = log(close of the 10:20 bar / open of the 10:00 bar), r_last = log(close of the 18:20 bar / open of the 18:00 bar). A pool of ticker-days, regression of r_last on r_first, SEs clustered by date. Criterion: coefficient > 0, t >= 2.6, positive sign in both halves. Trading form: an equal-weighted-across-tickers daily position sign(r_first) in the last 30 minutes; net mean daily return after 0.05% per side > 0.
I2. Overnight versus intraday. Per ticker-day overnight = log(open of the 10:00 bar of day t / close of the last bar of the main session of day t-1), intraday = log(close of the last bar of the main session / open of the 10:00 bar). Equal-weighted means by date. Criterion: mean overnight minus mean intraday > 0, t >= 2.6, the sign in both halves; trading form: buy at the close, sell at the open, net mean per day after 0.05% per side > 0.
I3. Time of day. The mean return of each 10-minute slot of the main session (equal-weighted across tickers, a series by dates). Criterion for a slot: |t| >= 3.5 (correction for ~52 slots) and the same sign in both halves. Descriptively also: the profile of volatility and turnover by slot.
I4a. Signed turnover to the next day. F = sum of sign(close-open)*value over the bars of the main session / sum of value, per ticker-day; the z-score of F across the day's tickers; a Fama-MacBeth regression of the next day's abnormal return (close-to-close of the ticker minus the equal-weighted mean of tickers) on z. Criterion: the mean slope, |t| >= 2.6, the sign the same in both halves.
I4b. Signed turnover to the rest of the day. F over the bars 10:00-11:50 (first 12 bars), target: log(close of the last bar / open of the 12:00 bar) of the same day, abnormal relative to the mean across tickers; Fama-MacBeth, criterion as I4a.

Block S (calendar effects; a family of ten tests, threshold |t| >= 3.0 on the discovery sample). Samples: US discovery 1953-01-01..1989-12-31 (Saturdays before 1952), US confirmation 1990-01-01..2026-10-14, IMOEX replication 1997-09-22..2026-10-16. For each effect, the difference of the mean returns of "effect" and other days/months, t by Newey-West.
S1 day of the week: five tests (each day against the others).
S2 turn of the month: the last trading day and the first three trading days of the month against other days.
S3 January against other months (monthly returns).
S4 Halloween: the months November-April against May-October (monthly).
S5 pre-holiday day: the last trading day before a break that is not a weekend, against other days.
S6 persistence of the best month: the month with the highest mean return on the discovery sample; a test of its mean minus the mean of the other months on the confirmation sample.
An effect is considered found if all hold: |t| >= 3.0 on discovery, the same sign and |t| >= 2.0 on confirmation, the same sign and |t| >= 2.0 on IMOEX. It is considered tradable if on confirmation and on IMOEX the size of the effect per period exceeds twice the costs (0.06%) for daily and (0.06%) for monthly effects. Descriptively with no criterion: the US presidential cycle (mean return by cycle year), the mean by months of the year.

Block Y (cycles and persistence; a family of six tests, threshold p < 0.01 with correction where indicated):
Y1 periodogram. Daily returns of GSPC and IMOEX are divided by the trailing 63-day standard deviation, a periodogram at frequencies with periods from 2 to 4000 days, Fisher's g-statistic, p from 2000 permutations of the standardised returns. Criterion (per series): p < 0.005 for the maximum and power at the found frequency above the 95th percentile of the permutation distribution in both halves of the series.
Y2 fixed cycles. On monthly log returns of GSPC and IMOEX a regression on sin/cos of periods 40 months (Kitchin), 48 months (presidential/4 years) and 108 months (Juglar); expanding window (at least 30 years for GSPC, 10 years for IMOEX), a forecast of the next month by extrapolating the cycle; OOS R^2 per Campbell-Thompson relative to the historical mean and p by Clark-West. Criterion: OOS R^2 > 0 and p < 0.0083 (six period-series pairs).
Y3 variance ratios. VR(q) for q = 2, 5, 10, 21, 63, 252 days on GSPC over four subsamples of ~25 years and on IMOEX; z by Lo-MacKinlay robust to heteroskedasticity. Criterion: |z| >= 3 with one sign in >= 3 of 4 GSPC subsamples and the same sign with |z| >= 2 on IMOEX (for each q; a q that passed the condition is considered found).
Y4 time-series momentum. The sign of the 12-month return at the end of the month (without skipping the last), the next month's position: long on a plus, otherwise cash (zero return). Criterion: the mean return of months after a plus minus after a minus, t >= 2.6 on discovery (1928-1989), t >= 2.0 on confirmation (1990-2026) and on IMOEX; tradability: the strategy's net annual return with costs 0.03% per change is above buy-and-hold on confirmation and on IMOEX, or not lower with a smaller drawdown.
Y5 (descriptive, no criterion): autocorrelation of |r| and r^2 up to 250 lags, an estimate of the period of the dominant volatility component, the mean return in September and by months.

Verdict of the step: each block is evaluated independently. A pass gives only a record for forward observation, the live model does not change. Any change of criteria after the run is made as a separate addition marked "after the run".

Result of step 30 (fixed 2026-10-19): found in the sense of the existence criterion are only I2 (overnight +0.074% versus -0.060% intraday, t of the difference 6.90) and I4a (slope +0.0438% per 1 σ, t 6.48), I3 gives two slots of about 1 bp; none is tradable after 0.05% per side (I2 -0.026% per day, long-short I4a -0.030%); I1, I4b, all calendar effects (S1-S6), periodograms, fixed cycles, variance ratios and time-series momentum did not pass. Calendar effects before 1990 are significant (Monday t -7.86, turn of the month t 5.50, pre-holiday day t 6.84) and disappear in 1990-2026 and on IMOEX. Details in docs/metrics.md. The addition after the run (control of I4a by daily return and long-short with costs) is marked in metrics.md as not part of the criteria.

### Forward recording of AlgoPack trades (deployed 2026-10-19, owner: "set up trade recording on the ai-box")

`cmd/tradesrecord` saves anonymised TQBR trades (AlgoPack starter package, 15-minute delay) for 33 stocks into `~/moex-trades/data/<date>/<ticker>.csv.gz` on the ai-box (columns tradeno, time, price, qty, value, buysell, period). Cron schedule (UTC, Mon-Fri): 16:15, 20:55 and 21:10; each run rereads the whole current day and replaces the file only if it has no fewer rows than the saved one. History backwards is unavailable by the key, so hypotheses about trade flows can be tested no earlier than about 120 trading days accumulate (~April 2027); the criteria will be fixed before the first analysis. Deployment: `scripts/trades-record-deploy.sh`.
