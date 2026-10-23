# AlgoPack round 2: market component, market-level targets and the materiality bar (2026-10-23)

Status: REGISTERED 2026-10-23 after one Reviewer pass (10 amends applied, see review log; English only). Roles as in `docs/plans/20261023-algopack-forecast-improvement.md` (round 1): the Claude session proposes and writes code, the opencode pane reviews read-only. Round 1 results (scored 2026-10-23, outputs in `artifacts/algopack-forecast/`) are the reason for this plan.

## What round 1 showed (all of it known before this plan was written)

- Family A, primary arm (realtime-only training, folds 3-7, 5367 rows): baseline AUC 0.495, ΔAUC A-B +0.002 (95% [-0.017, +0.018]), placebo-corrected +0.0018, ΔIC t +0.2: fail, minimum detectable effect about 0.018.
- Family A, descriptive 2020-plus arm: ΔAUC A-B +0.032 (95% [+0.001, +0.066], 5 of 5 seeds, both halves), but the shuffled placebo S gains +0.013 over B and the corrected A-S is +0.019 (95% [-0.002, +0.040]); A-lin minus B-lin +0.039, ΔIC A-S t +0.9.
- Families B and C: 6 of 14 and 6 of 7 rows pass the registered statistic, with out-of-fold R-squared gains over the persistence controls of +0.0008 (h1) and +0.0021 (h5) for volatility (bases 0.44, 0.57) and +0.0002 for spread (base 0.99). Statistically present, materially nothing.
- Family D: three registered passes (D1 h10 t +5.6; D2-200 t +8.1; D2-300 t +8.6) with net +0.006..+0.046% per day, i.e. 3 to 6 percent a year before any hedge, below the key rate.
- Reading to test: the within-date shuffle keeps every date-level moment of every feature, so the placebo S still carries market-timing information and the 2020-plus gain is probably a market component learned from regenerated history. The realtime-only null is consistent with that component being a backfill-era artefact. This is a hypothesis, tested by Family R below, not a finding.

## Honest status of the data this plan reuses

The realtime window 2025-01-03..2026-10-16 is the verdict window of round 1 and is reused here. Round 2 rows were chosen after seeing round 1, so they are not independent of it: the market-level rows overlap the leads H079, H081, H082 of plan 20261023 (t +2.97, +3.50, +2.43 on this window) and the decomposition rows overlap the hist-arm numbers above. Consequences, fixed now: (i) every pass in this plan is called a lead and needs a forward read or a new window to count as a result; the only window not yet seen is the forward one (data accruing from 2026-10-21 through `scripts/algopack-daily.sh`); (ii) thresholds are not relaxed to compensate for the reuse.

## Materiality bar (the owner's question, fixed now)

A result for the owner is one of three things, each at least a lead and each with its own gate:

1. Production model: ΔAUC of at least 0.016 (the measured same-test N95 of `docs/knowledge/model-quality-history.md`) on the absolute label, placebo-corrected, in the realtime-only arm; plus a behavioural read on the same out-of-fold predictions at the deployed thresholds 0.60/0.40 (trade counts, trade-set overlap with the baseline, mean gross 10-day forward return of buys minus sells for each arm); the behavioural read is reported, not a pass condition, because the repo history holds AUC lifts that did not trade.
2. Non-return target: out-of-fold R-squared gain of at least 0.01 over the persistence control, and for a volatility forecast also a sizing-overlay read with a pinned base book and rule: the D1 h10 book as the base, weights divided by the predicted 5-day market volatility relative to its trailing mean and renormalised to mean 1, judged by H069's frozen criterion (information-ratio gain at least +0.1 with Newey-West t of the daily net difference at least 2.0), before it is called material. Spread forecasting is declared closed: with a base R-squared of 0.99 the bar is unreachable by construction, and the spread change is not forecastable at the registered size.
3. Directional: net return above the key rate of the capital tied up, after the beta-hedge cost of follow-up F1 of plan 20261022, at 0.15% per side, positive in both halves, annualised. By this bar family D of round 1 is statistically real and economically below the deposit; that sentence goes into the report and is not a re-verdict.

## Family R. Decomposition of the hist-arm gain (4 comparisons)

Same recipe as family A of round 1 (T18, deployed lgbm+xgb+logreg, 10-day label, deadband 0.5%, seven blocks, five seeds, label_date embargo 10 calendar days, bootstrap by date blocks), with three changes fixed here:

- Demeaned features: each of the ten block columns is replaced by its value minus the mean over the rows of the same date in the merged file (the T18 names present that date). Computed per date from that date's own rows, so it is leak-free under date-blocked folds. The placebo S-dm permutes the demeaned ten-column vector within date; since its date mean is zero by construction, S-dm carries no date-level information and is a clean negative control for the cross-sectional claim.
- Two labels: L-abs, the production absolute label; L-ab, the abnormal label: 1 if the 10-day simple close-to-close return minus the mean 10-day return of the T18 names of that date is above +0.5%, 0 if below -0.5%, otherwise the row is dropped (the production deadband applied to the abnormal return). The returns come from the `tradestats` closes of the block file, ten ticker rows ahead; a move below -45% or above +80% drops the row. `label_date` is the date ten rows ahead.
- Two training regimes: realtime-only (blocks 3-7, rows from 2025-01-03) and 2020-plus (all seven blocks, descriptive).

Comparisons: {rt, hist} x {L-abs, L-ab} = four. In each: A-dm minus B and S-dm minus B, and the corrected difference A-dm minus S-dm with the 95% bootstrap interval, the seed rule (the sign agrees in at least four of five seeds), the halves, and the ΔIC with Newey-West 10 lags. Acceptance of a cross-sectional claim in one comparison: corrected ΔAUC interval above 0, point estimate at least +0.016 for both labels; the label change does not make a smaller gain more material, the abnormal label's paired-difference noise is of the same order (same recipe, same deadband), and a lower bar for the scientific claim would open the exact soft-door the reuse paragraph forbids, both halves positive, the seed rule met, and the S-dm placebo's own gain over B not positive-and-significant. The reading rule, fixed now: (a) if in the hist/L-abs comparison A-dm minus S-dm is not above 0, the hist-arm gain was market timing and the sentence 'the order-flow block adds no cross-sectional forecast skill to the production model' is the registered conclusion, whatever L-ab shows; (b) if A-dm minus S-dm is above 0 but below the +0.016 bar in hist/L-abs, the conclusion is 'partly market timing, the cross-sectional remainder is below the material bar', recorded as not established; (c) if it clears the bar in hist but not in rt, the conclusion is 'present in regenerated history, absent in realtime', the backfill-artefact reading, and only the realtime arm counts; (d) if it clears in rt but not in hist, that contradicts the round 1 rt null and is a red flag: no conclusion is drawn and the round 1 rt arm is re-checked for a construction difference before any reading; (e) if it clears in both, the round 1 rt null must be reconciled first, same rule; (f) the L-ab comparisons decide whether any cross-sectional content is an abnormal-return signal rather than an artefact of the absolute label: L-ab clearing in hist supports the scientific claim, L-ab failing in both marks the content as label-definition-dependent, and L-ab is never a substitute for L-abs in the production sentence. The abnormal label uses the equal-weight mean of the T18 names present that date, not the top-100 mean of plan 20261022, because the rows are T18; this is the registered deviation. L-ab rows whose tenth row ahead falls after 2026-10-21 are dropped, so the L-ab fold 7 coverage ends about ten dates before the L-abs coverage, as in the round 1 errata. The A-lin and S-lin arms of round 1 are reported descriptively in R (same model JSONs, no new training) because the linear-versus-tree split was the market-component clue; they are not a pass condition.

Family size 4, no pooling; the per-comparison criterion is the one above.

## Family M. Market returns from date-level flow (6 rows)

Date-level series built from the admitted top-100 universe of each date (point-in-time, as in plan 20261022), features at 18:30 of day d:

- F1: universe mean of A2 (the daily value-weighted `imbalance_val`), z-scored against its own trailing 60 days (at least 40); the trailing 60 days of the first about 60 feature dates of 2025 reach into regenerated 2024 history, which affects only the first-quarter dates of the first half, is accepted, and is reported in the coverage table (z60 and not z250 on purpose: a z250 at the start of the verdict window would draw about 90% of its window from the regenerated era that the KS finding says differs).
- F2: breadth, the share of admitted tickers with A2 > 0, z-scored the same way.
- F3: mean A2 of the three highest-median-value names of the admitted universe, z-scored the same way.

Targets: the equal-weighted admitted-universe log return from the close of d to the close of d+1 (h1) and the sum over d+1..d+5 (h5). 3 features x 2 horizons = 6 rows. Test: time-series OLS of the target on the feature, Newey-West standard error with lags max(5, h), sign registered as + (a higher A2 predicts a higher return, the sign of A2 everywhere else), threshold pooled t at least `T_M` (written below from the outcome-free correlation matrix, never below 3.0), the coefficient estimated separately on each half (split 2026-01-05) has the registered sign and each half's coefficient has the same sign as the pooled one (the exact conjunction of the round 1 implementation: half coefficient 0 times half coefficient 1 above 0, and half coefficient 0 times the pooled coefficient above 0; the half estimates and their Newey-West t are reported), and Benjamini-Hochberg q = 0.05 over the 6 pooled two-sided p-values. No trading form (a market-timing position on the equal-weighted universe is a hedge decision, assessed under bar 3 only if a row passes). Window: feature dates 2025-01-03..2026-10-16, the equal-weighted universe return skips the 2022 window and the dividend days like the other flowscan rows.

The hedge-return target (universe minus IMOEX) proposed in the review is not included: `cmd/flowscan` has no IMOEX source and the `D` result of plan 20261022 already failed on IMOEX; a new plan may add it.

## Family V. Market volatility and dispersion from date-level flow (9 rows)

Features: V1 the cross-sectional mean of the absolute per-ticker z60 of daily A2 (each ticker's A2 z-scored against its own trailing 60 days, at least 40, as F1 does for the universe mean); V2 breadth F2 (absolute value of its z-score); V3 the cross-sectional standard deviation of the same per-ticker z60 values. Targets: the log of the cross-sectional mean realised volatility (mean of `bRV`) at d+1 (h1) and the mean over d+1..d+5 (h5), and the log of the cross-sectional standard deviation of the h1 log returns of the admitted names (dispersion, h1 only). 3 x 3 = 9 rows. Controls: the same-target value of day d, its 5-day and 20-day means, and log mean turnover of the universe (so a feature has to beat persistence). Statistic: time-series OLS, Newey-West lags max(5, h), threshold pooled absolute t at least `T_M`, the half-coefficient conjunction of family M, BH q = 0.05 over the 9 pooled two-sided p-values, and for a row that passes the out-of-fold R-squared gain over the control-only regression (the block edges of round 1, expanding window with the horizon embargo) must be at least 0.01 to be called material; a pass below 0.01 is recorded as "statistically present, not material", as in round 1. The control-only out-of-fold R-squared is reported per target before the gain is read; if any target's control-only R-squared exceeds 0.95 the +0.01 gate for that target is recorded as unreachable by construction, exactly as spread was, and is not re-specified after the number.

## Frozen threshold `T_M` (computed outcome-free)

`T_M` is `max(3.0, the two-sided normal quantile at 0.05 / M_eff)` where M_eff is the Nyholt effective number of the five distinct date-level features (F1, F2, F3, V1, V3; V2 is a function of F2) computed from their correlation matrix on the verdict window 2025-01-03..2026-10-16 (features only, no return or target touched, which the outcome-free rule permits) and also on the exploration window 2023-12-01..2024-12-31 descriptively; `T_M` uses the realtime number, so that it is not a function of the regenerated era. The value is written below before any outcome is computed.

`T_M` and M_eff (`cmd/flowscan -mode mktprep`, run 2026-10-23 before any round 2 outcome): verdict window 2025-01-03..2026-10-16, M = 5, Nyholt M_eff 3.799, `T_M` = max(3.0, quantile at 0.05 / 3.799 = 2.48) = **3.000**; exploration window 2023-12-01..2024-12-30 descriptively: M_eff 3.970, 3.000. Correlations on the verdict window: F1-F2 +0.968, F1-F3 +0.672, V1-V3 +0.750, F/V cross-pairs +0.22..+0.38.

## Deliberately not in round 2

- Meta-labeling and a standalone GBM search: closed (round 1 and `docs/knowledge/model-quality-history.md`).
- Spread forecasting: closed as above. Evening-session rows: 2027. Event windows: block C of plan 20261022.
- Items 6-10 of the Reviewer's round-2 list (D1 regime split, D1 sizing overlay, A2 at h20, `hi2` market series, ticket-size proxy as a separate row): each is a low-prior variant of a closed family; they are queued for a third plan only if a row of R, M or V gives a lead, so that the family size stays honest.
- The full behavioural replay through `cmd/backtest` of the production model with the block columns: needs the block columns in the live feature path (an architectural change); the out-of-fold behavioural read of bar 1 stands in for it.

## Verdicts and the stop rule

Each family reports by its own conjunction. If R shows that the hist-arm gain is market timing, if M and V give no lead, and if the realtime-only A stays at the round 1 null, the registered answer to the owner is: "AlgoPack order-flow columns carry no cross-sectional forecast skill that improves the production model or any directional strategy net of costs in the realtime window; they carry a statistically stable but economically negligible improvement of volatility and spread forecasts; the stable A2 effect pays 3 to 6 percent a year before hedge, below the deposit; the remaining open item is the frozen A2 rule's forward read about 2027-01-20", with the power statement of each family. If a row gives a lead, the next step is a forward confirmation plan that copies the row verbatim; nothing enters the live path without an owner decision.

## Run order

1. Reviewer pass on this file.
2. Implement `merge -demean`, `merge -abnormal-label` and the behavioural read in `cmd/algopackincremental`; `mktprep` and `mkt` modes in `cmd/flowscan`; unit tests; outcome-free `mktprep` writes `T_M` here.
3. Family R on the ai-box (four comparisons), then M and V.
4. Numbers into `docs/metrics.md` 8.17, `docs/knowledge/model-quality-history.md`, `docs/ARCHITECTURE.md` and its twin in the same commit.

## Review log

Reviewer pass 1 (opencode, read-only, 2026-10-23): 0 drop, 10 amend, 4 keep; claims checked against the repo, no false claim, no leakage found. Applied: L-ab materiality equals 0.016 (the +0.010 proposal is withdrawn), the complete reading rule (a)-(f) of family R, the L-ab pins and the descriptive linear arms, the z60 justification and its accepted overlap with 2024, `T_M` from the realtime window, pinned V1 and V3, the half-coefficient conjunction, two-sided BH p-values, the sizing overlay with a base book and a rule, and the guard against an unreachable R-squared gate.

## Errata

(none)
