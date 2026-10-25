# TimesFM 3 beyond the point forecast: wave A (2026-10-25)

Status: DONE 2026-10-25 (English only; run once, 0 of 11 realtime hits, 0 of 5 placebos; E0 calibration passes; nothing beyond the recorded momentum). Registered before any generation.

## What changed since the first plan

The window is spent and `mom256` now lives in it: a post-hoc lead of incremental slope t +3.4 over the composite. Anything rank-correlated with 12-month momentum will nominally hit on the spent window. Three guards therefore apply to every arm.

1. **Baseline with momentum.** The incremental slope is taken over the out-of-fold composite **plus the 120- and 256-observation momentum ranks as controls** (per-date OLS of the 10-day abnormal return on composite, mom120, mom256 and the candidate; the candidate's coefficient is the statistic). Every candidate reports its mean daily rank correlation with mom256.
2. **Mechanical twin.** Each arm has a twin of the same output shape from a trivial model; the arm must keep |t| of at least 3.0 with the twin added to the baseline. Five within-date permutations of the first candidate column are the placebos; one placebo hit voids the verdict of the run.
3. **Independent confirmation.** A realtime hit is a lead. The confirmation is price-based and uses dates 2022-04-01..2024-12-31, which the exchange regeneration does not touch: per-date OLS of the 10-day abnormal return on mom256 rank and the candidate rank, Newey-West t (lag 10) of the candidate slope at least 2.0, same sign in 2023 and in 2024. The confirmation is not available to arms whose inputs are AlgoPack flows (E4): there the registered check is flow-forecast skill (below) and nothing else.

## Generation (single Python script on the ai-box, inference only)

`scripts/timesfm_features_b.py`, one run over feature dates 2022-04-01..2026-10-16, names with at least 256 observations, context = abnormal daily return (as in the first plan), missing as 0, `use_znorm` on, `make_positive` off. Columns of `tfm3b-features.csv`:

- `u64`, `u128`, `u256`, `u512` (u512 only with at least 512 observations): sum of the ten forecast steps, univariate.
- `q10s1`, `q90s1`, `q10s5`, `q90s5`, `q10s10`, `q90s10`: quantiles of the single forecast steps 1, 5 and 10 (256 context); `sp`: mean over steps 1..10 of (q90 minus q10).
- `sh256`: the point forecast sum with the last 20 observations shuffled by a fixed seed (date and secid hash).
- `shapeA`, `shapeB`: from a horizon-20 forecast on the 256 context: mean of steps 1-3 minus mean of steps 4-10; mean of steps 11-20 minus mean of steps 1-10.
- `flA2`, `flLev`, `flSp`: sum of ten forecast steps of the 256-day series of A2, LevAsym, SpLv (univariate).

## Experiments, features and rules (frozen)

All features rank-scaled within date. Order of evaluation: E0 first.

- **E0 calibration (gate, not a signal).** From `q10/q90` at steps 1, 5, 10 against the realised abnormal return at d+1, d+5, d+10 (single-day abnormal return `ab`): coverage of the 80% interval, rate below q10, rate above q90, over all forecasts, and per year. Rule: coverage within 0.72-0.88 at every step means the spread is usable; otherwise E2 is reported but cannot be called a hit.
- **E1 lagged forecast surprise.** `s_d` = realised 10-day abnormal return over d-9..d minus `u256` stored 10 dates earlier. Causal; no overlap with the outcome. Twin: the same with the AR(1) forecast (coefficient fitted on the previous 256 observations of the name).
- **E2 quantile spread as moderator.** Composite IC (and incremental slope of the composite on the outcome) in the top vs the bottom tercile of `sp`, raw and stratified by composite conviction (|prediction| tercile), compared with the same split on `OwnVol20`. Hit: difference of tercile ICs with block-bootstrap interval excluding zero in both halves, and still excluded after stratification, and not equal to the OwnVol20 split (the `sp` split must add over OwnVol20 as a second stratifier).
- **E3 context disagreement.** Candidates `u512 - u64` (rank difference) and `u512 - u256`, `u256 - u64`. Twin: `mom256 - mom64` and `mom256 - mom120`.
- **E4 forecast the flow.** First a skill check outside the return test: daily rank IC of each flow forecast (`flA2`, `flLev`, `flSp`) against the realised mean of that flow over d+1..d+10, versus the naive twin (mean of the last 10 observations of the same flow) and an AR(1) twin; stop if the model does not beat the naive twin. If it does, the return test: candidates `flA2`, `flLev`, `flSp` as in the rule with the composite+momentum baseline and the twin as a control. Caveat: the flow history before 2025 is exchange-regenerated, so no by-year gate; a hit stays a lead for the forward log.
- **E6 term-structure shape.** Candidates `shapeA`, `shapeB`. Twin: the same shapes computed from a momentum extrapolation (mean of the last 3 observations minus mean of the previous 7; mean of the last 20 minus the previous 40) of the abnormal-return series.
- **E7 recent-block sensitivity.** Candidate `u256 - sh256` (the model weight on the last 20 observations). Twin: `mom20 - mom256 / 12.8` (the recent block against the proportional share of the full sum).

Candidate rule per arm: |t| of the candidate slope at least 3.0 in the realtime window with the baseline above, same sign in both halves and for the IC, no placebo hit, and the arm keeps |t| at least 3.0 with its twin in the baseline. A CONFIRMED arm in addition passes guard 3. Only a CONFIRMED arm is carried to an economic test (the wave-3 chain with the carry of 27-29%). Number of candidate columns in the return test: about 20; stated up front so that the nominal level is read against that count.

## Honest expectations

Prior of at least one CONFIRMED arm: low (about 5-10%); the most likely outcome is that every long-context feature re-expresses momentum and is absorbed by the controls, which closes the TimesFM 3 question cleanly. A null is reported as a null. No second look at the realtime window: each arm is scored once.

## Review log

Pre-run pass (opencode, 2026-10-25): no look-ahead in the generation; every column ends its context at d; the design is sound. Conditions and pins, all applied below as errata.

## Errata

Written 2026-10-25 after the Reviewer's pass, before the evaluation.

1. The return test has exactly 11 candidates (u256, surprise_lag, d512-64, d512-256, d256-64, flA2, flLev, flSp, shapeA, shapeB, drop) plus 5 placebos (permutations of u256) plus 8 twins; u64, u128, sp and the quantile columns are inputs, not scored candidates. The nominal level is read against 11; at |t| of 3.0 about 0.03 false hits are expected under the null with independent candidates, more under momentum contamination.
2. Confirmation (guard 3) controls mom256 **and the arm's twin** (per-date OLS of the 10-day abnormal rank on mom256 rank, twin rank, candidate rank); CONFIRMED needs the candidate slope |t| at least 2.0 with the twin in, same sign in 2023 and 2024. It controls linear mom256 only: a nonlinear momentum re-expression could pass, a property stated in the verdict. The window starts 2022-04-01 because the context needs 256 observations and the price history begins in 2021-04; it excludes 2021, the strongest momentum year, which is conservative for momentum-family arms.
3. E0 additionally reports both tail rates; a tail rate outside 0.05-0.15 at any step marks the spread unusable for E2 regardless of coverage. The last 10 dates have no realised outcome and are excluded automatically.
4. E2 has no composite out of fold before 2025, so it is **exempt from guard 3**: any E2 hit is a lead for the forward log only.
5. E4's skill check uses the realtime window only.
6. The realtime statistic uses only 2025-01-03..2026-10-16 (halves split 2026-01-05); 2022-2024 rows feed guard 3 only. Empty u512 cells are missing values; availability is per column.
7. The twin mapping is stated in the verdict: E6 twins use 3 vs 7 and 20 vs 40 observations as loose structural matches; E7's twin is mom20 minus mom256/12.8.
8. CSV checks before the evaluation: q10 < q90 on every row, sp non-negative, quantiles in abnormal-return units (about 1e-2), per-year row counts for 2022-2026. Smoke file already showed q10s1 -0.0185, q90s1 +0.0159 (units correct, shape (10, 9)).

## Result (scored once, 2026-10-25)

Numbers in `docs/metrics.md` 8.35, output `artifacts/algopack-timesfm3/tfm3b-verdict.txt`. 102,325 forecasts. **0 hits of 11, 0 of 5 placebos.** E0: coverage 0.80-0.82, tails 0.08-0.10, calibrated. With the composite plus mom120 and mom256 in the baseline the raw 256-context forecast has slope -0.029 (t -1.4); no other arm (surprise, disagreement, flow forecast, shape, sensitivity) exceeds |t| 2. E2 spread moderator t +1.3 raw, +0.75 after volatility stratification. E4: the model forecasts A2 and LevAsym flow better than the naive mean but the forecast adds nothing to returns. Guard 3 is reported but not applicable (no realtime hit). Wave B (E5, E8, E9) is not run: the channel that wave A tested (past-only inputs of the same daily series) is exhausted, and the intraday long-context idea needs its own plan if the owner wants it.
