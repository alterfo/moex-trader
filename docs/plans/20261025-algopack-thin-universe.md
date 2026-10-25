# AlgoPack thin universe: the composite on ranks 101-400 with real spreads (2026-10-25)

Status: DONE 2026-10-25 (English only; scored once, signal stronger than in the top-100, both economic gates fail), registered before the run. Item 3 of the owner's order of work. Continues `docs/plans/20261024-algopack-wave3-costs-horizon.md` (the same chain on the top-100) and `docs/plans/20261023-algopack-third-untouched.md` (thin equities: the A2 lead t +7.47 with flat costs).

## Why this plan

The 80/20 map says three things about where the composite works: calm regimes, the short leg, and thin names (the highest IC tercile, 0.161 in calm periods, was the least liquid third of the top-100). Funds cannot trade thin names at size, so a retail-size book there is the one niche where the absence of competition could be real. It was never evaluated economically with quoted spreads: the third plan used flat costs on the A2 feature alone. The question: does the composite, fitted on the thin universe, earn a net return after the quoted BBO spread and the broker's short carry, either as a long-short book or, because many thin names may not be shortable, as a long-only book measured against the equal-weight thin universe?

## Universe and data

Point-in-time ranks 101-400 by the 60-day median traded value of each date (`cmd/flowscan -mode matrix -universe 400 -rank-from 101 -rank-to 400`), the same SYSTIME rule as the other plans, 242,868 ticker-days; in the realtime window (from 2025-01-03) about 93 names per date, 337 distinct tickers, 82% of the rows in ranks 101-200. The quoted BBO spread there: median 36 bp, 25th percentile 21 bp, 75th 59 bp, 90th 89 bp (mean 39 bp in ranks 101-200, 72 bp in 201-300); the cost model is the wave-3 one (0.05% commission plus half the day's `SpBBO` per side per name, 0.15% where the spread is missing). The top-100 matrix is byte-identical to the verdict matrix on its first 102 columns (regression check of the new rank flags). The universe mean return `u1`, used by the long-only book as the benchmark, is the equal-weight mean of the 101-400 names of the date. No separate tradability filter is applied: the cost model prices the spreads, and a filter chosen after seeing results would be a grid.

## Chain (one verdict, as in wave 3)

1. **Exploration** (regenerated window 2022-07..2024-12, the only place anything is chosen): `explore` on the thin matrix at horizon 10 with the bbo costs selects the ridge and elastic-net penalties and the trade rate from the usual grids by the partial-adjustment net.
2. **Realtime verdict, run once** (feature dates 2025-01-03..2026-10-16, halves 2026-01-05, five placebo seeds, date-block bootstrap with block 10 and 5000 draws): the composite and the A2 baseline fitted on the thin universe, expanding walk-forward with the 11-date embargo. Books at the exploration trade rate with the bbo costs: **(L) the dollar-neutral long-short quintile book** with the broker carry (reported at 17% and 25% a year; the carry is linear in the short notional, so the net at 27-29% follows by extrapolation), **(O) the long-only top-quintile book** against the equal-weight thin universe (`-book longonly`; abnormal net per day, no carry).
3. **Frozen gate** (primary, as in wave 3): for (L) at 25% carry and for (O), the mean daily net with the bootstrap 95% interval above 0, positive in both halves, the placebo books' mean net not above 0. Both are reported; a pass of either is a lead, never a result (the window is the same spent one).
4. **Reported, not conditions**: the IC of the composite and its difference against the A2 baseline; the IC by liquidity third (descriptive, dump mode); the transfer arm (the frozen top-100 composite, trained through 2026-10-07, applied to the thin names with `-mode forward`: names it never saw, dates it partly did); the share of a sample of thin tickers that the broker marks short-enabled (`cmd/borrowmeasure`), as an execution caveat for (L).

## Honest expectations

The IC should be at least as high as in the top-100 (A2 was strongest in thin names). The costs are higher: a median round trip of about 0.5% a name against a gross of order 0.2-0.4% per day for the quintile spread, so the result turns on turnover control (the partial-adjustment rate). The long-short book carries 25-29% a year on the short notional and is expected to fail the gate; the long-only book has no carry and is the more realistic product, with the benchmark being the thin universe itself. The prior of passing (O) is low-medium, of passing (L) low.

## Run order

1. Reviewer reads this file and the code changes (`cmd/flowscan/matrix.go` rank flags, `cmd/alphafit` flag `-book longonly`, the relative liquidity thirds).
2. Exploration, one verdict (L and O), the dump for thirds, the transfer arm, the borrow terms sample.
3. Numbers into `docs/metrics.md` 8.30 and `docs/knowledge/model-quality-history.md`.

## Result (scored once, 2026-10-25)

Numbers in `docs/metrics.md` 8.30, outputs `artifacts/algopack-thin/`. **Signal: stronger than in the top-100.** Thin composite IC +0.1292 (Newey-West t +12.8, bootstrap 95% [+0.111, +0.150], halves +0.142 / +0.113), composite minus A2 baseline +0.036 [+0.017, +0.058], 5 of 5 placebos; the IC is flat across the three liquidity thirds of the thin universe (0.122 / 0.124 / 0.123), so the strongest-in-the-thinnest pattern of the top-100 does not extend. Transfer arm (frozen top-100 composite on names it never saw): IC +0.0496 (t +4.6) with the A2 baseline at +0.003: the composite generalises across names at half the in-universe strength. **Economics: both gates fail.** (L) long-short, rate 0.20, bbo costs: gross +0.194% per day, turnover 0.458, net +0.0407% per day at 17% carry [0.000%, +0.086%], halves +0.058% / +0.019%; **+0.0174% at 25% carry [-0.023%, +0.063%]** (gate fails; extrapolated linearly to the 27-29% broker carry about +0.009% per day, about 2% a year); placebo books -0.74% to -0.77% per day. (O) long-only top quintile against the equal-weight thin universe: gross abnormal +0.073% per day, turnover 0.264, **net +0.0184% per day [-0.004%, +0.045%], halves +0.033% / 0.000%** (gate fails; about +4.6% a year). The transfer arm's long-short books are negative (-0.029% at 17% carry, -0.057% at 25% [-0.095%, -0.007%]). Borrow-terms sample: 10 of 60 thin tickers short-enabled at the broker. Registered reading: the composite predicts the thin cross-section better than the top-100 one, and costs consume almost all of it; the long-only form has no carry but a gross of only 0.07% a day against the benchmark of the thin universe itself.

## Review log

Pre-run pass (opencode, 2026-10-25): the long-only benchmark arithmetic, the thin universe mean, the rank flags, the cost model, the absence of a tradability filter and the comparability with wave 3 verified; three pins applied (errata).

## Errata

Written 2026-10-25 after the Reviewer's pass, before the run.

1. The thin universe includes the production-18 names that fall in ranks 101-400 (the rank window does not exclude them, unlike the band-2 filter of the round-3 plan): the registered difference.
2. The long-only benchmark is the thin universe's own equal-weight mean, so "abnormal" is relative to the 101-400 names, not the top-100: the product-relevant benchmark, because a retail buyer of thin names chooses among them.
3. Basis of the numbers: the long-only net is per unit of long notional (weights sum to about 1), the long-short net is per unit of one leg (gross exposure 2): both on the same per-leg basis.
4. Exploration results (regenerated window, selection surface only): the thin composite IC 0.1314 (ridge 0.003, elastic net 0.0002), long-short partial-adjustment net +0.069% per day at rate 0.20 (gross 0.51%), long-only abnormal +0.036% per day at rate 0.20; the rate 0.20 is used for both books. Borrow-terms sample: of the 60 thin tickers with the lowest mean rank (present at least 200 days in the realtime window) 10 are marked short-enabled by the broker (APTK, DELI, ELFV, GEMC, KMAZ, MRKU, MSRS, NKHP, ROLO, ZAYM), 50 are not.
