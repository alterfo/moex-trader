# Metrics: portfolio construction and cross-sectional signals, 8.1-8.10

Part of [metrics.md](../metrics.md) (the index, conventions and ledger live there). Numbers here are the single source of truth for this topic.

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

