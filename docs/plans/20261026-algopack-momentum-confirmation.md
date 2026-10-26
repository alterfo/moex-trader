# 12-month momentum: confirmation and composite refit (2026-10-26)

Status: REGISTERED 2026-10-26 before any run (English only). The pre-registered next step of `20261025-algopack-timesfm3.md`: the TimesFM forecasts proxy 12-month relative momentum, and the 256-observation sum of abnormal returns (`mom256`) has IC +0.107 and incremental slope +0.089 (t +3.4) over the composite in the spent window, found post hoc.

## Why it is a real candidate and not only a window artefact

Cross-sectional momentum has a theory (under-reaction, Jegadeesh-Titman) and a long literature, and the composite lacks it: its return controls are the 1-, 5- and 20-day ranks only. The independent price sample (the by-year daily IC of mom256 against the 10-day abnormal rank): 2021 +0.095, 2022 +0.010, 2023 +0.008, 2024 +0.050, 2025 +0.121, 2026 +0.090. Positive in every year, but only about +0.038 on average before the window.

## Step 1: independent confirmation (price only, dates before 2025)

`cmd/alphafit -mode momdiag` prints, in addition to the by-year table, the pooled daily IC of mom60, mom120 and mom256 for 2021-04..2024-12 with Newey-West (lag 10) t, the date-block bootstrap interval (block 10, 5000 draws) and the two sub-periods 2021-2022 / 2023-2024. **Confirmed** if mom256 has NW t of at least 2.0 on 2021-2024 pooled with the same sign in both sub-periods; otherwise the window IC is read as a regime of the window and step 2 is reported as an exploration only. Price history is not exchange-regenerated, so this sample is independent of the window.

## Step 2: composite with momentum (wave-3 chain)

The two features `Mom120` and `Mom256` (sums of the last 120 and 256 observations of the daily abnormal return, abnormal = `rb1` minus the same-day cross-sectional mean, missing until 120 / 256 observations exist; rank-scaled within date like every feature) are added to the feature set by `-mom`. Everything else follows wave 3b exactly: horizon 10, bbo costs, close entry, partial-adjustment book, hyperparameters from the `explore` mode on the regenerated window (not evidence), models refitted by the expanding walk-forward with the 11-date embargo, `dump` for the baseline and for the extended run, `compare` (extended minus baseline, bootstrap 95% interval, halves).

**Frozen gate (primary), one run.** For the extended book at the carry of 25% a year (the nearest dump column to the real 27-29%; the net at 27-29% follows linearly from the 17% and 25% columns and is reported): mean daily net with the bootstrap 95% interval above 0, positive in both halves (split 2026-01-05); the extended-minus-baseline difference of the daily net positive with an interval that excludes zero; the placebo books (five permutations of the Mom256 column) not above 0. The IC increment and the long-minus-short split of the gain are reported. Passing is a lead for the forward log, never a result: the window is spent, and the feature was chosen after looking at it.

## Honest expectations

Momentum in the composite is mostly cheap: it moves slowly, so it adds IC without adding turnover, which is the combination the net result is most sensitive to. A positive IC increment of +0.01-0.02 and a positive net difference are plausible; the prior of passing the primary gate at 25% carry is moderate (about 25%), because the base book sits at about zero net at the real carry. If step 1 fails, a step-2 pass is not read as support.

## Review log

Pre-run Reviewer pass NOT held: the opencode pane did not accept messages (composer prompt not recognisable, five attempts); the owner said to run without review (2026-10-26). Self-checks: momentum sums include the same-day abnormal return and the outcome starts at the close of the day; missing momentum rank is the neutral 0 like every missing feature.

## Errata

Written before the run: step 1 was run first and printed (price only, pooled 2021-2024 mom256 IC +0.0380, NW t +2.56, 2021-22 +0.0494, 2023-24 +0.0288): CONFIRMED by the registered rule. Hyperparameters from explore on the regenerated window (ridge 0.003, EN 0.0005, rate 0.5; explore IC 0.1296 against 0.1343 without momentum, not evidence). Placebo = the extended run with Mom120/Mom256 permuted within date (-mom placebo), compared to the baseline with the same interval rule.

## Result (scored once, 2026-10-26)

Numbers in `docs/metrics.md` 8.36, outputs `artifacts/algopack-momentum/`. **Step 1 confirmed** (price only, 2021-2024 pooled mom256 IC +0.038, NW t +2.56, both sub-periods positive). **Step 2: IC gain real, net gain absent; the primary gate fails.** Adding Mom120 and Mom256 raises the composite IC from 0.0957 to 0.1062 (difference +0.0105, NW t +2.39, interval [+0.0031, +0.0196], both halves positive) while the permuted-momentum placebo gives +0.0009 [-0.0003, +0.0021]. The net of the partial-adjustment book moves from +0.0751% to +0.0834% a day at 17% carry and from +0.0445% to +0.0526% at 25% (about +0.043% at 27.5%) but the placebo run moves identically (+0.0835% / +0.0529%): the net difference of +0.008% a day (naive t +0.65, interval at 17% [-0.016%, +0.037%]) is not due to momentum. The gate required a difference interval excluding zero: failed. Reading: momentum is a real, confirmed ranking signal that the books cannot monetise, because it is slow and sits on the same names the composite already holds; no lead for the forward log beyond recording.
