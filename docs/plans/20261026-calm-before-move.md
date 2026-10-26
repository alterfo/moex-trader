# Calm before the move: does a quiet stock predict its own direction? (2026-10-26)

Status: DONE 2026-10-26, 0 of 6 cells pass (metrics 8.39). Registered before any run (English only). The owner's recollection of an investor's advice: buy and sell an asset not while it rises or falls but while it is quiet and flat, predicting the coming move.

## What is testable

Two separate claims hide in the advice.

- **Magnitude (known, descriptive only).** Volatility clusters: a quiet stock tends to stay quiet. A compressed range is therefore not by itself a forecast of a large move; the test reports the future range relative to the stock's long-run range for calm names and does not gate anything.
- **Direction (the real hypothesis).** Inside the calm state, does a pre-stated direction predictor rank the next 10-day abnormal return better than it does outside the calm state? Without a direction rule "calm" gives no trade, so the claim stands or falls here.

## Data and definitions

Price-only columns of the study matrix (`work/matrix.csv`, top-100 liquid names per date, 2021-04..2026-10): `rb1` (daily abnormal return: own return minus the same-day cross-sectional mean), `f10`, `u10`, `div_drop`. Outcome: `y = f10 - u10`, 10-day abnormal forward return, rows with `div_drop` removed (as in `cmd/alphafit`). Price history before 2025 is not exchange-regenerated, so 2021-2024 is an independent sample; 2025-01-03..2026-10-16 is the replication window.

Per stock, using only observations up to and including day t, with at least 120 consecutive observations:

- `sd10` and `sd120`: standard deviation of `rb1` over the last 10 and 120 observations.
- `box20`: maximum minus minimum of the cumulative `rb1` over the last 20 observations (the price box).
- `mom60`, `mom20`: sums of `rb1` over the last 60 and 20 observations. `rb5` is the existing 5-day abnormal return column.

Calm definitions (state is the bottom quintile on the date, by cross-sectional rank among names that have the statistic):

- **C1 (quiet vol):** `sd10 / sd120`.
- **C2 (flat box):** `box20`.

Direction predictors (cross-sectional rank on the date, sign fixed in advance): **D1 = mom60** (continuation), **D2 = mom20** (continuation), **D3 = -rb5** (short-term reversal).

## Frozen gate

Six cells (2 calm definitions x 3 predictors). For each: daily Spearman-style IC (Pearson of ranks) of the predictor with `y` among calm names on the date (dates with at least 10 calm names), and the same IC among the other 80%.

A cell **passes** only if all hold on 2021-04..2024-12:
1. calm-state mean IC with Newey-West (lag 10) t of at least 2.6 (Bonferroni for six cells at about 1%);
2. calm IC minus non-calm IC (daily difference): mean positive with NW t of at least 2.0;
3. calm IC positive in both 2021-2022 and 2023-2024;
4. calm IC positive on the replication window 2025-01-03..2026-10-16 with NW t of at least 1.65;
5. the placebo (calm label permuted within date across names, 5 seeds) gives a calm-minus-non-calm difference at most 1/3 of the real one in absolute value.

Reported but not gated: the long-minus-universe gross spread of the top predictor tercile inside the calm state, the magnitude table, IC by year. Any pass is a lead for the forward log, never a result: no short-borrow cost is measured, and the key rate of 14-21% a year beats every strategy found so far.

## Honest expectations

Prior of a pass is low (about 10%). Momentum alone is a confirmed but unmonetisable signal (metrics 8.36); a calm filter on top of it mostly halves the sample. The most likely reading: the investor's advice is about patience and entry price, not about a measurable prediction from quiet alone.

## Review log

Written before the run. Reviewer pass: not held at registration time (sole writer; peer pane review optional after the result).

## Result (scored once, 2026-10-26)

Numbers in `docs/metrics.md` 8.39, output `artifacts/calm/`. **0 of 6 cells pass.** Quiet stocks stay quiet (scaled next move 0.59 against 0.72 for C1). Closest cell C2-D2: calm IC +0.093 (t +4.58) but calm minus other +0.037 (t +1.75), inconsistent across 2021-22 and 2023-24. C1 flips sign between samples. Errata: first run included 2020 and used overlapping placebo seeds; fixed and rerun, verdict unchanged. Not a lead.
