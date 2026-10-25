# Ordinary and preferred share pairs: statistical arbitrage on MOEX (2026-10-25)

Status: DONE 2026-10-25 (English only; scored once, 0 of 20 under the registered rule, the reversion is real but not tradable), registered before the run. Item 2 of the owner's order of work ("also add arbitrage, maybe we can earn on arbitrage"). Continues `docs/plans/20261023-algopack-futures-fx.md` (futures-spot basis, closed: 1 of 62, a thin-spot artefact).

## What arbitrage is available to us

True riskless arbitrage on the exchange (cross-market, futures against cash, ETF against basket) is taken by participants with millisecond latency; a retail book can reach only statistical arbitrage. The cleanest statistical pair on MOEX is the ordinary and preferred share of the same company, which are claims on the same assets with different dividend and voting rights. The pairs with full daily history in the point-in-time top-100 are SBER/SBERP, TATN/TATNP, SNGS/SNGSP, MTLR/MTLRP, BANE/BANEP (1483 days) and RTKM/RTKMP (1485 days); LSNG/LSNGP has no ordinary-share history. Data not in the repo and therefore not tested: ETF/BPIF premium to the basket (a separate acquisition item), Moscow against SPB exchange quotes, currency triangles.

## Construction (causal, no fitted parameters)

For a pair the relative index S is the cumulative sum of the daily log-return difference ordinary minus preferred (`rb1`), counting a day only if both legs have a valid return and neither is flagged as an ex-dividend day (dividend gaps do not enter the index). Reversion signal z at date d: S(d) against the mean and standard deviation of the previous 60 observations of S, needing 60 observations. Momentum signal mz: the 20-observation change of S divided by the trailing standard deviation of the daily differences times sqrt(20). Flow signal fd: A2 of the ordinary share minus A2 of the preferred share, divided by its trailing 60-observation standard deviation. Outcome: the forward return of the ordinary leg minus that of the preferred leg over h trading days (h = 1, 5, 10), dividend-clean (windows crossing an ex-date are missing). Price-only rows (reversion, momentum) use the full sample 2020-04..2026-10 because prices are not affected by the regeneration of the flow aggregates; their pre-2025 and 2025-plus coefficients are reported separately. Flow rows use only the realtime window from 2025-01-03.

## Rows (20 plus 2 economic and 1 descriptive)

| Id | Row |
|---|---|
| PR01-PR06 | reversion of z for each of the six pairs, h = 5 |
| PR07-PR09 | pooled reversion of z (pair fixed effects), h = 1, 5, 10 |
| PM01-PM02 | pooled momentum of the spread, h = 5, 10 |
| PF01-PF06 | A2 difference for each pair, h = 1, realtime |
| PF07-PF08 | pooled A2 difference, h = 1 and h = 5, realtime |
| PX01 | pooled interaction z x A2 difference, h = 5, realtime (coefficient of the product, two-sided) |
| PN01 | descriptive: five same-sector pairs of different companies (SBER-VTBR, LKOH-ROSN, GAZP-NVTK, NLMK-MAGN, MTSS-RTKM), pooled reversion, h = 5: a comparison that tells whether the effect is specific to claims on the same company |
| PE01-PE02 | economic rule (below), full sample and realtime |

## Statistic and verdict (frozen)

Fixed-effects regression of the outcome on the signal with date-clustered Newey-West standard errors (lags h + 5), the t statistic of the signal coefficient (the product term for PX01). Circular-shift permutation: the signal within each pair is shifted by 120, 127, 134, ... observations (every offset at least 120 from the identity) and the t recomputed; p = (1 + the number of shifts with |t*| at least |t|) / (1 + the number of shifts). Family: the 20 rows PR, PM, PF, PX; threshold p at most 0.05 / M_eff with M_eff computed outcome-free by the Nyholt method from the correlations of the 20 signal panels (rows on disjoint pairs count as uncorrelated); the registered sign is negative for reversion rows (a high z followed by a lower ordinary-minus-preferred return), positive for momentum, positive for flow (two-sided for PX01). A row is a **hit** if its permutation p is at most the threshold, the sign agrees with the registered sign, and for the price-only rows the pre-2025 and 2025-plus coefficients have the same sign. PE01, PE02 and PN01 are not in the family.

**Economic rule (PE)**: enter at the close when |z| is at least 1.5 (short the ordinary and long the preferred when z is positive, the opposite when negative), hold 5 trading days, one position per pair at a time; gross = the signed 5-day return difference; cost = entry and exit in both legs at the day's quoted BBO spread (0.05% commission plus half of `SpBBO` per side, 2 x (ordinary + preferred)); carry = the broker's 0.074% per calendar day on the short leg for 7 calendar days (the Investor tariff at a band top, `docs/knowledge/model-quality-history.md`, 2026-10-25), also shown without carry (as if the short leg were free). Mean net per trade per unit of one leg's notional with a bootstrap interval over trades. Gate: the interval above 0 with the carry; reported without carry for illustration only.

## Honest expectations (before the run)

A pair spread of 2-5% with a 5-day reversion of 20-30% of a 1.5-sigma deviation can yield about 0.3-1% gross per trade; the carry alone is 0.52% and four sides of costs about 0.3-0.5%, so the expected net is around zero or negative with the carry and positive without. The statistical test (reversion exists) is expected to be positive for price-only rows because the ordinary/preferred spread is a known mean-reverting object; whether it survives the costs is the question. Short-selling the preferred shares may also be limited by the broker (not checked).

## Run order

1. Reviewer reads this file and `cmd/pairscan`.
2. One run on the ai-box (`~/algopack-study/work/matrix.csv`).
3. Numbers into `docs/metrics.md` 8.29 and `docs/knowledge/model-quality-history.md`.

## Result (scored once, 2026-10-25)

Numbers in `docs/metrics.md` 8.29, output `artifacts/algopack-pairs/pairscan-verdict.txt`. **0 hits of 20 under the registered rule**, because the rule could not be met by construction: M_eff 14.22 gave the permutation threshold p at most 0.0035, but the permutation has about 180 shifts (offsets 120 to 1400 in steps of 7, fewer for the shorter pairs), so its smallest attainable p is about 0.0054 (reached by PR02, PR07 and PR08). This is a design flaw of mine, not a feature of the data, and it is recorded here rather than repaired by a rerun with a finer grid. What the data say: the price-only reversion is present and consistent: pooled reversion z coefficient -0.0004 (h1, t -3.22), -0.0015 (h5, t -3.18), -0.0024 (h10, t -2.68), the same sign before and after 2025 (t -2.41 and -2.72 at h5), five of six pairs with the registered sign (TATN t -4.15, RTKM -2.84, SNGS -2.14, SBER -2.05; MTLR +0.19, BANE -1.54); the same-sector pairs of different companies show nothing (PN01 t +0.60), so the reversion is specific to claims on the same company. Momentum (PM) and flow differences (PF, PX01) are null (smallest p 0.054, 0.076). **Economics (PE): the reversion is too small for the costs.** 785 trades 2020-2026: mean gross +0.366% per trade, mean net **-0.470%** with the broker carry on the short leg (bootstrap [-0.70%, -0.24%], 35% winning trades, every year negative), and **+0.048%** [-0.19%, +0.28%] if the short leg were free (2023 -0.34%, 2025 +0.26%); realtime only: gross +0.460%, net -0.362% [-0.72%, -0.01%] with carry, +0.156% [-0.20%, +0.51%] without. Registered reading: a real but small mean reversion of the ordinary-preferred spread (about 0.4% gross per 5-day trade at |z| 1.5), not tradable after four sides of spread and the carry of the short leg; no gate pass.

## Review log

Pre-run pass (opencode, 2026-10-25): no look-ahead; ex-dividend handling, survivorship, fixed effects with date-clustered Newey-West, permutation, interaction, economic simulation and the Nyholt computation (implemented in `family.go`) all check out; five pins and one amendment applied (errata).

## Errata

Written 2026-10-25 after the Reviewer's pass, before the run.

1. `mz` pairs a numerator that includes the current 20-observation change (known at the close) with a standard deviation from the 60 previous daily differences: a defensible asymmetry, not a leak.
2. The 60-observation windows are observation-based, so a day skipped because a leg is absent stretches the window in calendar time; this is the honest behaviour for a gap.
3. The cost uses the signal day's BBO spread (the approximation of the wave-3 book) and the 5-day outcome is a difference of log returns treated as a return, adequate at these magnitudes.
4. PX01's M_eff entry uses its z panel as the proxy for the correlation structure of the interaction product: a stated approximation.
5. The result table quotes the per-year net next to the trade count, so a single lucky year cannot pass as a strategy, and the availability of the short leg (the preferred shares may not be shortable at the broker, not checked) is an execution caveat of any pass, not a footnote.
