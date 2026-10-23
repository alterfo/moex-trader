# AlgoPack meta-variables: does the macro and exogenous state modify the composite? (2026-10-24)

Status: DONE 2026-10-24 (English only; scored once, one lead), registered before any macro series is aligned with the composite's out-of-fold series. Design agreed with the Reviewer (opencode, read-only) in the meta-variable brainstorm of 2026-10-24. Roles as in `docs/plans/20261023-algopack-alphafit.md`.

## Why this plan

The owner asked to test "hyper-super-meta-parameters": fund holdings and volumes, the key rate and its cuts, inflation, GDP, aliens, Saturn and geomagnetic storms, as a further 100 hypotheses. The repo already holds a precedent (`docs/knowledge/model-quality-history.md`): macro series fed to the tree models as features failed (dividend yield -0.005 fold AUC; IMOEX, CNY-RUB, Brent returns and the key rate -0.041 mean fold AUC), with the written reason that a macro value is the same for every ticker on a date, so a model memorises the date. The market-timing channel (T) therefore has a low prior (the M family scored 0 of 6, best t 1.66), and the genuinely untested channels are:

- **R**, regime conditioning: does the daily out-of-fold IC of the frozen composite (`cmd/alphafit` blend) differ between two macro states;
- **S**, economics: does the net of the partial-adjustment book (rate 0.33, 17% borrow) differ between the same two states;
- **I**, stock-level interaction: does the cross-sectional payoff of a stock's rolling exposure to a macro factor move with the factor's recent change (Fama-MacBeth form).

## What is testable and what is not (the honest count)

A monthly series has about 22 realtime values and a quarterly one about 7: the effective sample is the number of regime changes, not 450 dates. The family is therefore about 50 rows, not 100, and 22 of every 100 asked items cannot be tested at all. Padding the count with grids of thresholds on the same few series would only manufacture false positives, so it is not done.

**Registered rows (50)**: 12 real states and 3 negative controls, each with one R, one S and one T row (45), plus 5 interaction rows.

**Not tested, with reasons (in the ledger of the result)**:

- GDP (7 quarterly values), inflation (12 monthly values, nearly collinear with the key rate; its information is inside the key-rate state), dividend season (one cycle), options expiry (redundant with the quarterly-expiry indicator of the 100-hypothesis plan), day-of-week (tested there), aliens (no data file exists; one is welcome from the owner and would run as one more control, with the same machinery).
- Acquisition items, no rows until the data lands with its own provenance step: BPIF fund share creation and redemption and fund holdings (the ISS history carries traded value only, not issued shares; the shares issued are not a daily history), CPI and GDP releases, OFZ curve slope, MOSPRIME, RUONIA, weather, Bitcoin. Fund trading value from ISS history is available and is registered as a state (F12, a proxy for fund activity, not for creations).

## States (all binary, all causal, no tuned parameters)

Every state at feature date d is computed from data through the close of d-1 (one trading day of lag, to avoid any same-day information). Dates of the fast series are the trading calendar of the matrix. "High" means above the trailing median of the same series over the previous 250 trading days (needs 120 observations). All series come from ISS or the CBR through `cmd/macrofetch` (`data/macro-long`) and `cmd/metascan fetch` (gold, RVI, fund value).

| Id | State = 1 when |
|---|---|
| F01 | key rate easing: key rate at d-1 below its value 63 trading days earlier |
| F02 | key-rate change window: a change day (cut or hike, from the daily step series) in the last 5 trading days |
| F03 | CNY-RUB 10-day realised volatility (daily log changes) high |
| F04 | Brent 20-day realised volatility high |
| F05 | IMOEX 10-day realised volatility high |
| F06 | IMOEX above its 100-day mean |
| F07 | gold in rubles (GLDRUB_TOM) 10-day realised volatility high |
| F08 | RVI close high |
| F09 | sanctions news: 5-day count of distinct articles in `data/news_history.jsonl` whose title contains the Russian stem of "sanction" (code points U+0441 U+0430 U+043D U+043A U+0446, case-insensitive) high |
| F10 | turn of month: last 2 or first 3 trading days of a calendar month |
| F11 | Brent 10-day return positive (direction state, as opposed to F04) |
| F12 | fund activity: 5-day sum of traded value of the equity funds TMOS, SBMX and EQMX (TQTF until 2026-06-19, TQBR after, one history query across boards) high |
| C01 | Saturn: geocentric ecliptic longitude (mean-element Kepler orbit, accuracy one degree is enough) in the first half circle |
| C02 | lunar phase: waxing (age under 14.77 days from the 2000-01-06 18:14 UTC new moon) |
| C03 | geomagnetic: maximum 3-hour Kp of day d-1 at least 5 (GFZ Potsdam definitive Kp) |

C01 to C03 are the negative controls and run through the identical machinery; they have no mechanism. A state needs at least 40 realtime dates on each side to be tested, otherwise the row is "not testable" (stated in the result, counted in the family).

## Statistics (identical for every row)

Out-of-fold series (fixed, from the realtime verdict of the alphafit plan; deterministic dump `cmd/alphafit -mode dump`, an unchanged rerun of the verdict code that only writes the daily series, no new evaluation): composite (blend) daily IC, A2 baseline daily IC, partial-adjustment composite net at 17% borrow, universe mean 10-day forward return `u10`. Realtime feature dates 2025-01-03..2026-10-16, halves split 2026-01-05.

- **R**: D = mean IC of the composite on state-1 dates minus on state-0 dates.
- **S**: D = mean daily net of the partial-adjustment composite book on state-1 minus state-0 dates.
- **T**: D = mean `u10` on state-1 minus state-0 dates.
- **I** (five rows): for each date the Spearman-style slope of the rank-scaled 10-day abnormal return on the rank-scaled 60-day rolling beta of the stock's daily return to the factor (Brent, CNY-RUB, gold in rubles, IMOEX, and RVI change against the IMOEX beta); the statistic is the OLS slope b of that date series on the lagged standardised 10-day factor change (standardised by the trailing 250-day standard deviation of 10-day changes), with Newey-West lags 10. The beta window ends at d-1 for returns and the factor change ends at d-1.
- **Calibration, the verdict statistic**: circular-shift permutation of the state (or factor-change) series over the realtime dates, every offset at least 20 dates from the identity (the autocorrelation and regime durations are preserved by construction); the two-sided permutation p is (1 + number of shifts with |D*| at least |D|) / (1 + number of shifts). With about 450 dates the p floor is about 0.0025 and it is stated per row (a slow state with few regime changes has an effective floor far above it, visible in the shift distribution and reported).

## The verdict (frozen form)

- Family: the 45 state rows plus the 5 interaction rows. M_eff is computed outcome-free by the Nyholt method from the correlation matrix of the 15 state series and the 5 factor-change series (not from the outcomes). Threshold on the permutation p: 0.05 / M_eff, also reported with Benjamini-Hochberg at 0.10 on the real rows.
- A real-state row is a **hit** only if (1) its permutation p is at most the threshold, (2) the sign of D agrees in both halves, (3) for R and S rows, the statistic is positive for the same state direction in the A2 baseline series too (consistency, not a size condition).
- **Calibration of the machinery (must hold first)**: the three controls (45 control rows would be a grid, so just C01 to C03 times R, S and T, 9 rows) must give zero hits at the threshold. One hit among them voids every real hit of the same use (R, S, T or I) and is the finding: the machinery over-rejects. Expected number of control hits at p 0.05/M_eff over 9 rows is about 0.02.
- A hit that survives is a **lead**, not a result: the 60-date forward window of the alphafit forward rule is too small to confirm a regime effect (about 3 regimes), so the lead waits for a second year of data; nothing enters the live path.
- The on-off form for S (trade only in the state with the positive D) is reported next to every S hit, and is described as in-sample.

## Power statement (written before the run)

450 dates with a 10-day overlap. For a binary state with a 50/50 split and daily IC volatility about 0.18 the standard error of D for R is about 0.017 before autocorrelation; a macro state that changes a few times a year inflates it two to three times. Differences smaller than about 0.08 IC (almost the whole IC of the composite, +0.096) cannot be detected for slow states. A null is the expected outcome and the prior that any of the 12 real states passes is low.

## Run order

1. Reviewer reads this file and the code (`cmd/metascan`, `cmd/alphafit` mode `dump`).
2. Fetch gold, RVI, fund value and Kp to `data/macro-long`; dump the series on the ai-box (`~/algopack-study`).
3. Run `cmd/metascan` once; numbers into `docs/metrics.md` 8.21, `docs/knowledge/model-quality-history.md`.

## Result (scored once, 2026-10-24)

Numbers in `docs/metrics.md` 8.21, outputs `artifacts/algopack-meta/`. Controls 0 hits of 9; one real hit, F08 R (RVI above its trailing median: composite IC +0.045 against +0.139, p 0.0024, threshold 0.0026); F08 S not significant (p 0.148); no T row, no interaction row passes. Registered reading: a lead. The descriptive state means printed after the verdict (not a condition) were added to metascan after the first run; the verdict lines are byte-identical before and after.

## Forward rule (frozen 2026-10-24)

At the alphafit forward read (60 forward dates, about 2027-02-05) the F08 split of the forward composite IC is reported (state defined identically, from the refreshed RVI series), with the date-block bootstrap interval of D. Descriptive: 60 dates hold few regime changes, so neither a confirmation nor a refutation is possible from it; the confirming read needs a second year of realtime data (about 2027-10). Nothing enters the live path.

## Review log

Meta-variable brainstorm (opencode, 2026-10-24): precedent of macro features, T prior; R and S as the untested channels; circular-shift permutation as the calibration and the machinery check; controls mandatory and identical; family counted at about 40-50 rows, not 100; GDP, inflation, dividend season, option expiry, day-of-week, weather, Bitcoin dropped or deferred; fund creations and redemptions a data-acquisition item; aliens have no data. Applied.

## Errata

Written 2026-10-24 after the Reviewer's code pass, before the run.

1. C03 uses the GFZ definitive Kp, published with a delay, so its d-1 value was not available at d in real time; the machinery is identical to the real rows, the data availability is not, which is immaterial for a no-mechanism control.
2. The I-row verdict is the permutation p; the Newey-West mention in the statistics section is vestigial and is not computed.
3. Run-order check added: metascan prints the means of the dumped series first and stops unless mean ic_blend is +0.0957 and mean net_gp_blend +0.0298% per day (the dump reproduced +0.09571 and +0.02979% before the run).
4. Floors: F01's effective sample is the number of easing-direction flips (a handful), so its permutation floor is high; `rankScale` is only called with at least 30 names.
