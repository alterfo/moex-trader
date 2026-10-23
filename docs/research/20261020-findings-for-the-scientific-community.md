# Findings of interest to the scientific community


Status: 2026-10-20. A selection from [../metrics.md](../metrics.md), which stays the single source of every number; section references below point there. Setting: the MOEX Trader research programme, a Russian equity market (MOEX TQBR board), daily and 10-minute data, 2014-2026, costs 0.05% per side per component unless stated. Every result below was judged against criteria fixed before the run; descriptive readings are labelled as such.

Selection rule: a finding is listed when it is (a) a replicable methodological result that changes how a comparable study should be run, (b) a well-powered negative result on a market that is thinly covered in the literature, or (c) a documented failure mode that other practitioners can hit. Parameter tuning of the trading system and operational incidents without a general lesson are left out.

## Caveats that apply to everything below

- One market, one broker's data, one author. Nothing has been independently replicated.
- The positive cross-sectional result (F6) was found after an inverted signal was already known from an earlier attribution, so it is not fully clean (section 8.6). Ridge windows and hyperparameters were chosen after seeing the data; the p-values are not corrected for that selection.
- Forward (post-freeze) evidence is not yet available for any result. The core rerun on refreshed data is pre-registered in [../plans/20261019-core-rerun.md](../plans/20261019-core-rerun.md); its first gate rerun (section 4.4) is the only completed part.
- Per-quarter models of the earliest runs were not saved, and results dated before 2026-09-17 predate pipeline fixes (section 0.4).

## A. Methodology

### F1. The evaluation procedure is noisier than the effects usually reported (section 3.6)

On 75 configurations of one recipe (5 seeds x 3 first-test shifts x 5 training starts) the pooled AUC has SD 0.0108 and range 0.4605-0.5009. The 95th percentile of the absolute difference between two configurations that differ only in seed or training start is 0.016 AUC. The dominant source is where the walk-forward fold boundaries fall (SD 0.0116), not the random seed (SD 0.0019). Consequences: any single-configuration AUC comparison below 0.016 is inconclusive; paired comparisons over at least 15 configurations are needed. The one difference that survived pairing (training from 2015 against 2024, 15 of 15 pairs, mean +0.020) sits at the noise floor with an absolute AUC at or below 0.52.

Related: holding out the last 20% of training rows for calibration moved the raw AUC of the same recipe by up to 0.08 (section 6.4), and calibrators fitted on those rows inverted the ranking (AUC drop of 0.07-0.19) because the most recent training window was anti-correlated with the outcome.

### F2. Survivorship and look-back liquidity bias change the verdict of a factor study (sections 8.4, 8.5)

The same factor rules were run on a survivor-only panel and on a point-in-time panel of 719 securities that traded on TQBR (881385 daily snapshot rows, delisted names included; universe defined on the date by 252 bars of history and 60-day median traded value of at least 30 million RUB). The point-in-time universe lowered every Newey-West t-statistic: LightGBM long-short 2.29 to 1.27, LightGBM long-minus-universe 2.25 to 1.94, composite long-short 1.78 to 0.01. A hand-set composite whose signs were chosen on 2016-2021 reached t = 3.67 on the survivor panel in the 2022-2026 test and 1.42 on the point-in-time panel. A reported t above 3 on a survivor universe is therefore not evidence of a signal.

### F3. Structural stability as an acceptance condition (sections 0.6, 5.7)

A conclusion drawn from one specification (one seed, split, cost level, universe) is treated as a hard-model conclusion and counts only if it survives a declared family of small perturbations; otherwise it is recorded as "no finding". Four unstable results are on record: the survivor/point-in-time swing above, the volatility-unit labels not confirmed on rerun, the 0.08 AUC swing from the calibration hold-out, and the sign change of the ridge long-short book under higher borrow. A parameter tuned to a historical optimum is flagged as a boundary optimum when a grid neighbour loses more than half of the realized P&L or changes its sign. Worked example: the aggregate exposure cap is discontinuous and non-monotonic (30000 per position: cap 60000 gives +81730, cap 75000 gives -6555 as closed trades collapse from 234 to 50, cap 90000 gives +144058), so the profitable region cannot be tuned into.

### F4. Multiple-testing correction on a real research log (section 3.7)

All 41 documented attempts were registered (7 selected, 24 rejected, 10 controls) and the best six-quarter series was deflated. Probabilistic Sharpe against zero 0.987 becomes a deflated Sharpe of 0.643 (p = 0.357) and a conservative DSR of 0.233, against a pre-registered bar of 0.95. The raw series looked significant; the attempt count removed it. The registry pre-registers each new attempt before its numbers exist.

### F5. A stale-result check: a reported lead did not reproduce on a fresh control (section 4.4)

The logistic single member led the control by +81383 RUB over six windows in the original run. Rerun on refreshed data with a control retrained in the same run, it is level (-0.2% over seven windows). The cause is stack drift: library versions changed between runs and the archived artifact reproduces to about 93%. Comparisons against an older ledger number are therefore not used; each comparison needs a same-run control.

## B. Positive result (limited)

### F6. A linear cross-sectional model passes where gradient boosting and a neural net do not (section 8.6)

Five signals and their cross-sectional ranks, target the cross-sectional rank of the next 21-bar return, expanding window from 2019-01, refit every 63 days, 92 monthly periods, total return with dividends, 0.15% per side, borrow 0.005% per day. Registered rule: t >= 2.6 on a family of six models.

| model, arm | net mean per period | NW t | Sharpe |
|---|---|---|---|
| ridge (alpha 10), long minus universe | +1.07% | 3.35 | 1.11 |
| ridge, long-short quintile | +1.73% | 2.54 | 0.88 |
| MLP (32-16, five seeds), long minus universe | +0.42% | 1.49 | 0.54 |
| LightGBM, long minus universe | +0.58% | 1.94 | 0.64 |

Findings within it:

- One-month returns continue rather than reverse on MOEX: the long-short quintile on rev_1m earns -2.69% per period (t -5.18, zero positive years of 11). The registered composite treated it as a reversal signal and so worked against the strongest signal in the data; the ridge learned the correct positive weight from past data alone. This inverts the usual short-term reversal sign of developed markets.
- The long-short form depends on the short-borrow cost: t 2.54 at 0.005% per day, 1.42 at 0.03%, 0.07 at 0.06% (10.9% per year at 0.03%). The real borrow rate was never measurable from the account history.
- Net of key-rate carry nothing is left: the ridge top quintile beat the equal-weight universe by about 20 points per year in 2024-2026 while earning less than the 14-21% key rate (section 8.12). The edge is relative, not absolute.
- It does not transport to a small live book: restricting to the 17 traded names costs about a quarter of the net (t 2.54 to 1.84); an absolute-threshold, unbalanced live rule leaves net per deployed capital at the carry level (+0.61% against +0.59%, t 1.18); weighting every name equally instead of every leg takes gross per deployed capital from +0.80% to +0.19% (section 8.13). As a gate candidate it passes the rolling gate in 59% of 34 windows, below the 70% bar.

## C. Well-powered negative results

### F7. No predictive edge from price/flow features at the horizons tested (sections 3.1, 3.5, 3.6, 6)

Validation AUC 0.46-0.55 depending on recipe and window; tail decisions (p >= 0.60 or p <= 0.40) hit 42.4% against a 47.7% base rate (500 decisions, 83 episodes, right-tailed bootstrap p = 0.912). No tested variation passes the pre-registered gate: dividend-adjusted and excess-to-index labels, wide training universe (172 securities, 376553 rows), calibration, volatility scaling, rank features, deeper history, an open-interest feature set, and a foundation time-series model. Training-panel breadth reduces the loss by about 17000 RUB but does not make the recipe profitable (section 6.5).

### F8. Zero-shot LLM direction on Russian headlines is not better than chance (section 7.7)

A local instruction-tuned model, temperature 0, 1200 clean articles over 157 dates and a 1114-article pre-cutoff sample for a look-ahead test. Direction against the sign of the excess return over IMOEX: AUC 0.511 (h = 1), 0.527 (h = 5), 0.526 (h = 10), confidence intervals containing 0.5; the registered bar (AUC >= 0.55 with lower bound above 0.5) fails. Part of the signal is trailing momentum (rank correlation 0.263 with the prior 5-day return). The look-ahead rule (+0.030 with lower bound above 0) is not met, yet the pre-cutoff sample alone is above chance (0.5524 at h = 10), so the older archive is not cleared as contamination-free.

### F9. Headline classifiers: a small effect that depends on the corpus and the quarter (sections 7.3-7.6)

On a 260-ticker archive (153151 pairs, training before 2025, test from 2025-03) the classifier reaches AUC 0.5234 at 10 days with a Bonferroni interval of [0.5022, 0.5424], above 0.5 in 4 of 6 quarters (0.564 to 0.477); the within-date permutation control gives 0.5021. The registered criterion (AUC >= 0.530) is not met. The effect is carried by Telegram posts, measured on non-independent pairs (reposted wires), and the net-of-cost top-minus-bottom quintile spread (+0.263% per tranche) has an interval spanning zero. A domain-shift hypothesis (the 2021-2024 archive not transferring to 2026 text) was not supported: a control classifier that never saw 2026 matched the one trained on it, and its monthly 10-day AUC ranged from 0.373 to 0.753, a window effect.

### F10. Calendar effects: strong before 1990, absent after (section 8.11)

Discovery on the S&P 500 1953-1989, confirmation on 1990-2026 and on IMOEX 1997-2026 (daily closes since 1927 and 1997). Monday effect -0.210% (t -7.86) then +0.010% (t 0.31) and +0.087% (t 1.27) on IMOEX; turn of the month +0.130% (5.50) then +0.041% (1.44); pre-holiday +0.264% (6.84) then +0.054% (1.06). January and Halloween do not reach the discovery threshold. Fixed 40/48/108-month cycles have negative out-of-sample R2 in all six pairs; variance ratios show momentum only in 1953-1977, reversed after 2002; the periodogram of the S&P has p = 0.072 and IMOEX p = 0.847. These are confirmatory tests with a discovery/confirmation split declared in advance (|t| >= 3.0 and >= 2.0), not scans.

### F11. Intraday: the overnight premium exists but is not harvestable (sections 8.4, 8.11)

On 10-minute candles of 29 companies (5186545 bars, 2011-2026): overnight +0.074% per day (t 7.07), intraday -0.060% (t -3.10), difference +0.133% (t 6.90), stable in halves. A buy-at-close, sell-at-open strategy nets -0.026% per day (t -2.50) after 0.05% per side. On the point-in-time panel the overnight return is positive in every year and the intraday return negative since 2024. Signed turnover of the day predicts the next day's abnormal return (+0.0438% per sigma, t 6.48), but with the same-day return controlled the slope falls to +0.0167% (t 2.15): most of it is daily momentum. A long-short on it is +0.170% gross per day (t 6.79) and -0.030% (t -1.21) after 0.05% per side.

### F12. A published portfolio-formation method failed on MOEX (section 8.1)

VAR-ICA-MGARCHSK (Mikhaylov et al., Finance: Theory and Practice 2026) reimplemented on 16 tickers, 99 rebalances 2019-2026, 750-day window, long-only, with a stationary block bootstrap. Equal weight: Sharpe 0.15, drawdown 53.8%. Full model: Sharpe -0.09, drawdown 74.3%, CVaR95 ratio to equal weight 1.044 [0.976, 1.121]. The noisy VAR(1) mean term drives the weights (turnover about 100% per rebalance against 7%); with the mean switched off the ICA variance forecast is indistinguishable from sample-covariance minimum variance (CVaR ratio 1.000 [0.979, 1.023]), and adding higher moments worsens tail risk (drawdown ratio 1.25). The registered criterion (15% lower CVaR, 15% lower drawdown) is not met.

### F13. Event, level and dividend studies find no tradable effect (sections 7.8-7.11)

Sanctions and negotiation events, stop and take-profit level crossings, news bursts as a risk regime, and dividend capture were each tested against registered gates. Dividend capture: the mean gap ratio is -0.97, i.e. the ex-date open sits about one full dividend below the last cum-dividend close, so the market prices the drop essentially exhaustively; the primary offset has an excess over IMOEX indistinguishable from zero under both tax conventions, and the one offset with an interval excluding zero lies outside the registered acceptance subset. Rejected with 65 events in 9 waves.

### F14. A beta-neutral overlay removes the apparent alpha once the hedge is real (sections 5.3-5.5)

The deployed book's realized P&L (+194293 RUB over 2025-04..2026-09) is about two thirds IMOEX beta (a short tilt in a falling index). With a frictionless synthetic IMOEX leg a residual daily alpha survives (cluster t 3.98, block-bootstrap interval excluding zero, positive in 5 of 6 quarters). With the real MX front-month future (roll 3 days before expiry, margin and fees modelled, whole-contract sizing) it disappears: t -0.49 with whole contracts and +0.39 with fractional ones, confidence intervals including zero; basis, roll and close-versus-index drag worth about 12800 RUB were omitted by the synthetic leg. A hedge test on a synthetic benchmark can therefore return a false positive, and at a book size of about 200k RUB one contract is coarse (230000 RUB notional) against a mean beta exposure of about 107000 RUB.

## D. Failure modes of applied ML in trading

### F15. Train/serve skew produced a persistent directional bias (section 9.1)

For three weeks (2026-09-17..10-09) the running system sold almost exclusively (81-95% SELL decisions in ISO weeks 39-41 against 17.1% in the masked replay) while the index rose. Three pre-registered explanations (feature extrapolation, model staleness, base-rate shift) were each rejected by their rules. The cause was found afterwards, and checked on every day in the table: the live process fed `news_count` (up to 144, against a training mean 0.10 and SD 0.70) and `news_sentiment` to a model trained on zeros for them, because the masking commit was not in the running binary. The logged live probability equals the model on the unmasked vector to four decimals on each day. The case is a clean example of how a feature that is always zero in training but live in production acts as a silent out-of-distribution input, and of a diagnostic (reproducing the logged output from candidate inputs) that identifies it exactly.

### F16. Position-sizing and risk-cap interactions are discontinuous (sections 5.7, 8.13)

An order-level fail-closed exposure cap that never resizes freezes the book after the first tickers fill it: on a synthetic always-short book a cap costs 78-90% of the income. A first implementation that treated the target lot count as a delta double-counted held positions, and an intermediate book-wide variant blocked the final flatten-to-flat order and collapsed the capped rows to zero closed trades. In the replay, removing the caps raises the whole-window P&L from -2788 to +7472 while lowering the sum of six non-overlapping windows from +340 to -1224 and lifting the maximum drawdown from 3.15% to 5.60%: the two readings disagree because positions carried across window boundaries are counted differently, a limit of short windows rather than a verdict on the caps.

## E. Open items that would make these findings stronger

- A measured short-borrow rate (F6 depends on it).
- The forward track of the ridge and composite books, first evaluated after at least 12 rebalancings (recorder running since 2026-10-19).
- The remaining core rerun studies R1 and R3-R6 on refreshed data, with the structural-stability families of the rerun plan applied.
- Independent replication on another emerging-market board with a point-in-time universe.
