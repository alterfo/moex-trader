# AlgoPack, third untouched-data plan: equity flow to futures outcomes, futures hi2, thin equities (2026-10-23)

Status: DONE 2026-10-23 (English only): scored once, numbers in `docs/metrics.md` 8.19, outputs in `artifacts/algopack-third/`. Roles as before (the Claude session proposes and writes code, the opencode pane reviews read-only). Nothing is scored. The owner asked a second time for "another 100 hypotheses"; this plan registers **42** and says why it is not 100.

## Why 42 and not 100

The previous two cycles (`docs/plans/20261023-algopack-100-hypotheses.md`, `20261023-algopack-forecast-improvement.md` with rounds 2-3, `20261023-algopack-futures-fx.md`) used up the powered predictor-outcome pairs. What is untouched and powered today is small: (a) the equity AlgoPack block as the predictor of futures and FX outcomes (the outcomes are untouched; the predictors are built), (b) the new `fo` and `fx` `hi2` datasets (concentration of participants per contract, downloaded 2026-10-23, realtime throughout the checked period), (c) thin equities (ranks 101-300) as outcomes. Everything else is either a variant of a closed family, on a spent outcome with a spent predictor, or blocked by the data vintage (the futures order-flow family waits on `orderstats`, realtime only from 2026-08, about 120 dates in 2027-02, ten rows already registered as deferred in the futures plan). Padding to 100 would only add noise and multiplicity; the demand is met instead by the matrix below, so that the next request can be answered by pointing at it.

## Vintage matrix and global test counter

| Predictor family | Source | Realtime from | Outcome family | Outcome realtime from | Status |
|---|---|---|---|---|---|
| Equity block (A2, A5, ticket, depth, spread, morning and auction trade imbalance) | equity AlgoPack | 2025-01 | futures returns (`fo` front contract) | 2025-05 | this plan, Book 1 |
| Equity block | equity AlgoPack | 2025-01 | `CNYRUB_TOM` spot | 2026-02 | this plan, Book 1 (second tier) |
| `fo` `hi2` (11 concentration metrics per contract) | `fo` `hi2` | whole checked period (2024-09 on, SYSTIME same day) | futures next-day open-to-close | 2025-05 | this plan, Book 1 |
| `fx` `hi2` | `fx` `hi2` | whole checked period | `CNYRUB_TOM` spot | 2026-02 | 2 rows registered, deferred |
| `fo` `orderstats` family (NOF, CI, OT) | `fo` `orderstats` | 2026-08 | futures | 2025-05 | 10 rows deferred in the futures plan, about 2027-02 |
| Equity block | equity AlgoPack | 2025-01 | thin equities rank 101-300 | 2025-01 | this plan, Book 2 (spent outcome population, leads only) |
| Equity block | equity AlgoPack | 2025-01 | top-100 equities | 2025-01 | closed (rounds 1-3), nothing more |
| `fo` and `fx` features | `fo`, `fx` | see futures plan | futures and FX | see futures plan | closed (futures plan, 62 tested, 1 artefact-prone pass) |

Global counter (written down so the reuse is visible, not as a retroactive threshold): equity-return outcomes about 250 tests before this plan; futures and FX outcomes 85 rows registered in the futures plan (62 tested) plus the 36 Book 1 rows here (34 scored, 2 deferred); thin-equity outcomes 6 rows here (the top-200 and top-300 A2 economics of plan 20261022 is the only earlier touch, besides the descriptive universe sensitivity of plan 20261022 that also covered the 101-200 band).

## Provenance (Step 0, outcome-free)

`fo` `hi2` is one row per contract-metric at 18:50 on the trade date, 11 metrics, about 90-130 contracts a day; the share of rows with `SYSTIME` more than 30 days after the trade date is 0 in every month checked from 2024-09 to 2026-10 for both `fo` and `fx`. The equity block is the `cmd/flowscan -mode export` file rebuilt on 2026-10-23 with two new columns (morning-session trade imbalance and closing-auction 18:40-18:50 trade imbalance, the imbalance quantities are `tradestats` columns; the morning one is computed on morning bars joined with `obstats`, the auction one on `tradestats` bars alone) and with the names ranked 101-300 by 60-day median value added (`adm100` still marks the top 100). The `fo` outcome side keeps the futures plan rules (front contract by same-day value, one-contract returns, volatility scaling, caps, no explicit expiry rule), eligibility per dataset, and halves at the median of the outcome-free eligible-date list.

## Protocol (binding)

1. **Windows and halves**: feature dates 2025-01-03..2026-10-16, eligible days per row as in the futures plan (equity predictors from 2025-01, `fo` outcomes from 2025-05, `CNYRUB_TOM` outcomes from 2026-02, `hi2` rows from 2025-05 because of the outcome). Halves: the median of the row's outcome-free eligible-date list (predictor available and the outcome contract's `tradestats` realtime, no return or regression used). A row with fewer than 80 eligible dates is "not testable".
2. **Equity date-level predictors** (adm100 names present that date, at least 40 with the value, features up to 18:30 except the auction one): E01 mean A2; E02 share of positive A2; E03 mean A2 of SBER, GAZP, LKOH; E04 cross-sectional std of A2; E05 oil mean A2 (ROSN, LKOH, TATN, at least 2); E06 metals (GMKN, PLZL, CHMF, at least 2); E07 banks (SBER, VTBR, T, at least 2); E08 mean ticket-size z20; E09 mean cancellation intensity A5; E10 mean depth growth; E11 mean spread ratio; E12 mean morning-session (07:00-09:55) trade imbalance; E13 mean closing-auction (18:40-18:50) trade imbalance. Each is z-scored against its own previous 60 days (at least 40). Definitions of A2, A5, ticket, depth and spread are the executed `cmd/flowscan` ones. All signs are registered plus (a higher flow measure predicts a higher return), as everywhere in the AlgoPack plans; no sign is chosen after seeing futures data.
3. **Outcomes**: `fo` volatility-scaled h-day return of the front contract (as in the futures plan); for E13 and for all `hi2` rows the **next-day open-to-close** return of the same contract (the equity auction ends and `hi2` is stamped at 18:50, the futures close, so the decision is taken after the close and the outcome starts at the next open; the evening session and the overnight gap are excluded by design). `CNYRUB_TOM` spot likewise, h1, volatility-scaled.
4. **Time-series rows** (P, Q01-Q06): outcome on the predictor with the three return controls (scaled returns of day d, d-1, mean of d-2..d-5), Newey-West HAC with lags max(5, h). P22 (interaction) regresses on the product of the predictor and the quarterly-expiry-week indicator with the predictor and the indicator as controls and tests the product; the indicator is 1 from the Monday of the week of the third Thursday of March, June, September and December through that Thursday (ex ante from the calendar); the indicator weeks were verified against the observed front-change calendar in Step 0 before scoring (all 7 front changes of MIX, all 9 of Si and all 7 of RTS since 2025 fall inside the flagged weeks), so no replacement was needed. **Fama-MacBeth rows** (Q07, Q08): `hi2` rank on the next-day open-to-close abnormal volatility-scaled return over the liquid groups with the futures plan's controls, at least 20 groups. **Thin-equity rows** (T, Book 2): Fama-MacBeth over names ranked 101-300 with a feature and the three return controls, at least 40 names a date, the outcome the volatility-scaled h-day return minus its cross-sectional mean, dividend days dropped (ex-date minus 1 to plus 1); T04 adds the per-name indicator that the ex-date lies within the next 5 trading days and the dividend was already declared, and tests the rank-times-indicator coefficient.
5. **Threshold for Book 1**: `T` = max(3.0, the two-sided normal quantile at 0.05 / M_eff) with M_eff the Nyholt number of the 19 date-level z60 predictor series (13 equity series and the 6 `hi2` series) from their correlation matrix on the verdict window, outcome-free: `cmd/futscan prep3` gave M_eff 17.99 and `T` = 3.000 (the floor; the quantile is 2.99). A Book 1 row passes statistically if its registered-sign t is at least `T`, both half coefficients have the registered sign and the pooled one, and it survives Benjamini-Hochberg q = 0.05 over the 34 scored Book 1 rows (nearly redundant next to `T` = 3.0). Book 2: Benjamini-Hochberg q = 0.10 over the 6 rows, registered sign, leads only. A pass is a lead; a forward rule is written before any forward date exists. Nothing enters the live path.
6. **No new door after a result**: definitions, signs, windows, universes and thresholds are frozen with this file. The cost-based economic flag is not part of this plan (these are statistical time-series and Fama-MacBeth tests; an economic reading needs a separate plan with a measured futures cost).
7. **Honest expectation**: the equity market-level component vanished in realtime in rounds 1-3 (family M null, best t +1.66) and futures hi2 has never been tested; the prior for a pass is low-medium for the P rows on index futures (derivative-leads-spot is a standard channel), low elsewhere. A null is recorded in the same way as a pass. Power: with about 340-360 eligible dates a single time-series row needs a daily correlation of about 0.16 to reach t = 3.

## Rows

Status "test" is scored; "deferred" is registered and not scored (fx tradestats vintage); "lead" is Book 2.

| ID | Status | Kind | Predictor and target | Sign | Horizon | Definition |
|---|---|---|---|---|---|---|
| P01 | test | e2f | E01 MIX | +1 | h1 | universe-mean A2 (z60) -> MIX futures vol-scaled return h1 |
| P02 | test | e2f | E02 MIX | +1 | h1 | breadth of positive A2 (z60) -> MIX futures vol-scaled return h1 |
| P03 | test | e2f | E03 MIX | +1 | h1 | mean A2 of SBER, GAZP, LKOH (z60) -> MIX futures vol-scaled return h1 |
| P04 | test | e2f | E04 MIX | +1 | h1 | cross-sectional std of A2 (z60) -> MIX futures vol-scaled return h1 |
| P05 | test | e2f | E08 MIX | +1 | h1 | mean ticket-size z (z60) -> MIX futures vol-scaled return h1 |
| P06 | test | e2f | E09 MIX | +1 | h1 | mean cancellation intensity A5 (z60) -> MIX futures vol-scaled return h1 |
| P07 | test | e2f | E10 MIX | +1 | h1 | mean depth growth (z60) -> MIX futures vol-scaled return h1 |
| P08 | test | e2f | E11 MIX | +1 | h1 | mean spread ratio (z60) -> MIX futures vol-scaled return h1 |
| P09 | test | e2f | E12 MIX | +1 | h1 | mean morning-session trade imbalance (07:00-09:55) (z60) -> MIX futures vol-scaled return h1 |
| P10 | test | e2f | E13 MIX | +1 | h1 | mean closing-auction trade imbalance (18:40-18:50) (z60) -> MIX futures vol-scaled return h1, outcome next-day open-to-close of the same contract |
| P11 | test | e2f | E01 MIX | +1 | h5 | universe-mean A2 (z60) -> MIX futures vol-scaled return h5 |
| P12 | test | e2f | E11 MIX | +1 | h5 | mean spread ratio (z60) -> MIX futures vol-scaled return h5 |
| P13 | test | e2f | E01 RTS | +1 | h1 | universe-mean A2 (z60) -> RTS futures vol-scaled return h1 |
| P14 | test | e2f | E01 Si | +1 | h1 | universe-mean A2 (z60) -> Si futures vol-scaled return h1 |
| P15 | test | e2f | E07 Si | +1 | h1 | mean A2 of SBER, VTBR, T (z60) -> Si futures vol-scaled return h1 |
| P16 | test | e2f | E01 CNY | +1 | h1 | universe-mean A2 (z60) -> CNY futures vol-scaled return h1 |
| P17 | test | e2f | E01 BR | +1 | h1 | universe-mean A2 (z60) -> BR futures vol-scaled return h1 |
| P18 | test | e2f | E05 BR | +1 | h1 | mean A2 of ROSN, LKOH, TATN (z60) -> BR futures vol-scaled return h1 |
| P19 | test | e2f | E01 GOLD | +1 | h1 | universe-mean A2 (z60) -> GOLD futures vol-scaled return h1 |
| P20 | test | e2f | E06 GOLD | +1 | h1 | mean A2 of GMKN, PLZL, CHMF (z60) -> GOLD futures vol-scaled return h1 |
| P21 | test | e2f | E01 NG | +1 | h1 | universe-mean A2 (z60) -> NG futures vol-scaled return h1 |
| P22 | test | e2f | E01 MIX | +1 | h1 | universe-mean A2 (z60) x quarterly-expiry-week indicator -> MIX futures vol-scaled return h1 (interaction coefficient) |
| P23 | test | e2fx | E01 CNYRUB_TOM | +1 | h1 | universe-mean A2 (z60) -> CNYRUB_TOM spot vol-scaled return h1 |
| P24 | test | e2fx | E02 CNYRUB_TOM | +1 | h1 | breadth of positive A2 (z60) -> CNYRUB_TOM spot vol-scaled return h1 |
| P25 | test | e2fx | E07 CNYRUB_TOM | +1 | h1 | mean A2 of SBER, VTBR, T (z60) -> CNYRUB_TOM spot vol-scaled return h1 |
| P26 | test | e2fx | E09 CNYRUB_TOM | +1 | h1 | mean cancellation intensity A5 (z60) -> CNYRUB_TOM spot vol-scaled return h1 |
| Q01 | test | hi2ts | hhi_agressive MIX | -1 | h1 | fo hi2 hhi_agressive of MIX (z60) -> MIX next-day open-to-close, vol-scaled |
| Q02 | test | hi2ts | hhi_passive MIX | -1 | h1 | fo hi2 hhi_passive of MIX (z60) -> MIX next-day open-to-close, vol-scaled |
| Q03 | test | hi2ts | agr_diff MIX | +1 | h1 | fo hi2 agr_diff of MIX (z60) -> MIX next-day open-to-close, vol-scaled |
| Q04 | test | hi2ts | net_diff MIX | +1 | h1 | fo hi2 net_diff of MIX (z60) -> MIX next-day open-to-close, vol-scaled |
| Q05 | test | hi2ts | hhi_agressive Si | -1 | h1 | fo hi2 hhi_agressive of Si (z60) -> Si next-day open-to-close, vol-scaled |
| Q06 | test | hi2ts | agr_diff Si | +1 | h1 | fo hi2 agr_diff of Si (z60) -> Si next-day open-to-close, vol-scaled |
| Q07 | test | hi2fm | hhi_agressive  | -1 | h1 | fo hi2 hhi_agressive (rank) -> next-day open-to-close abnormal return, FM over liquid groups |
| Q08 | test | hi2fm | hhi_passive  | -1 | h1 | fo hi2 hhi_passive (rank) -> next-day open-to-close abnormal return, FM over liquid groups |
| Q09 | deferred | deferred | hhi_agressive CNYRUB_TOM | -1 | h1 | fx hi2 hhi_agressive of CNYRUB_TOM -> own next-day open-to-close (deferred: fx tradestats vintage) |
| Q10 | deferred | deferred | agr_diff CNYRUB_TOM | +1 | h1 | fx hi2 aggressive buy-minus-sell concentration of CNYRUB_TOM -> own next-day open-to-close (deferred: fx tradestats vintage) |
| T01 | lead | thin | a2  | +1 | h1 | A2 -> thin-name (rank 101-300) vol-scaled abnormal return h1, FM |
| T02 | lead | thin | a2  | +1 | h5 | A2 -> thin-name abnormal return h5, FM |
| T03 | lead | thin | a2m5  | +1 | h5 | A2 5-day mean -> thin-name abnormal return h5, FM |
| T04 | lead | thin | a2  | +1 | h1 | A2 x ex-date-week indicator -> thin-name abnormal return h1, FM interaction coefficient |
| T05 | lead | thin | tz  | +1 | h1 | ticket-size z20 -> thin-name abnormal return h1, FM |
| T06 | lead | thin | sr  | +1 | h5 | spread ratio -> thin-name abnormal return h5, FM |

## What this plan does not do

It does not rerun any closed family, does not test the `fo` order-flow family before its data matures, does not use evening-session `fo` bars as features (they sit inside the registered outcome window), does not touch the live path, and does not force the row count.

## Result (scored once, 2026-10-23, `cmd/futscan scan3`)

- **Book 1 (futures and FX outcomes, `T` = 3.000): 0 of 34 pass.** Largest statistics: P26 (mean cancellation intensity of the equity block to `CNYRUB_TOM`, n = 201) t +2.46 with halves t +0.90 / +2.30; Q01 (`fo` `hi2` `hhi_agressive` of MIX, contrarian, to MIX next-day open-to-close) t -2.11 (registered sign, halves -1.57 / -2.21); P12 (mean spread ratio to MIX h5) t -1.94; P10 (closing-auction imbalance to MIX next-day open-to-close) t +1.89; P22 (universe-mean A2 times the quarterly expiry week to MIX) t -1.69 (second half -3.43, opposite to the registered sign). The headline pair, universe-mean A2 to MIX (P01) t -0.01, breadth P02 t +0.04, RTS P13 t +1.02, Si P14 t -0.64, CNY P16 t -0.93, BR P17 t -1.61, GOLD P19 t +0.03, NG P21 t +0.57. None of the `hi2` time-series or Fama-MacBeth rows passes (best |t| 2.11).
- **Deferred:** Q09, Q10 (`fx` `hi2`), not scored.
- **Book 2 (thin equities, ranks 101-300, leads only, BH q = 0.10): 3 leads of 6.** T01 (A2 to the volatility-scaled abnormal return h1) t +7.47, halves +6.07 / +4.63; T02 (h5) t +5.65, halves +6.34 / +2.28; T03 (A2 5-day mean, h5) t +2.89, halves +4.75 / +0.20; T05 (ticket size) t +0.09; T06 (spread ratio) t +0.28; T04 (A2 times the ex-date week) is not testable (the regression returned no estimate on 80 dates; the likely reason is too few declared ex-dates inside the band, not diagnosed). The three leads are the known A2 effect in a less liquid band, where it is statistically stronger than in the top 100 (compare plan 20261022 top-200 and top-300); they carry no economic claim, since the plan does not test costs and the earlier D2 economics at 200 and 300 names was below the deposit.
- **Reading.** The reverse lead-lag (equity order flow predicting index, currency, commodity and spot futures) and the futures concentration index do not forecast untouched futures outcomes at the registered threshold; the only statistically large rows are the old A2 effect on thin equities, a spent outcome population.

## Review log

Reviewer pass 1 (opencode, read-only, 2026-10-23): no look-ahead found in `e2f.go`, `e2frun.go`, `hi2.go`, `retOC`/`sretOC` or the flowscan export changes. Applied: the expiry-week verification in Step 0 (done, aligned), the ex-date convention of the thin-equity indicator aligned with the dividend drop (first trading day after `last_buy_date`), the dead `|| true` removed, the tradestats/obstats sentence, the counter precision, and the pins below.

## Errata

1. Pins written before scoring: (a) the 18:30-18:50 move of the equity block against the MIX close at 18:50 is inside the outcome window (the first 20 minutes of signal decay are not captured, conservative); (b) the close-to-close volatility scaler applied to open-to-close outcomes is a conservative normalization (open-to-close volatility is smaller); (c) a new front contract starts with few participants, so a roll-day HHI spike in the `hi2` z60 is expected and registered, it is not treated as a finding; (d) the thin universe is ranks 101-300 excluding the production 18 names (Step 0: 336 distinct names ever in the band); (e) the correlation window of `prep3` is the intersection with the MIX eligible days, outcome-free.
