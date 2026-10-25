# Ordinary and preferred share pairs: statistical arbitrage on MOEX (2026-10-25)

Status: REGISTERED 2026-10-25 (English only), before the run. Item 2 of the owner's order of work ("also add arbitrage, maybe we can earn on arbitrage"). Continues `docs/plans/20261023-algopack-futures-fx.md` (futures-spot basis, closed: 1 of 62, a thin-spot artefact).

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

## Review log

(pending the Reviewer's pre-run pass)

## Errata

(none)
