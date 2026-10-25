# AlgoPack thin universe: the composite on ranks 101-400 with real spreads (2026-10-25)

Status: REGISTERED 2026-10-25 (English only), before the run. Item 3 of the owner's order of work. Continues `docs/plans/20261024-algopack-wave3-costs-horizon.md` (the same chain on the top-100) and `docs/plans/20261023-algopack-third-untouched.md` (thin equities: the A2 lead t +7.47 with flat costs).

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

## Review log

(pending the Reviewer's pre-run pass)

## Errata

(none)
