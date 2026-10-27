# Owner's second hundred: triage before any run (2026-10-27)

Status: REGISTERED 2026-10-27 before any run (English only). The owner pasted one hundred dependence and robustness hypotheses ("X depends on Y") over ten studies already run. They are not new signals; they are splits, costs and replications of results the programme already holds. Running all hundred against the spent realtime window would repeat the mistake the programme exists to avoid: every realtime verdict is run once. This plan sorts them first.

## Classes (frozen)

- **DONE** answered by a committed run; the pointer is given and nothing is re-run.
- **CHECK** probably answered; the existing output is read first (no new compute) and the item is re-classed before any run.
- **IND** new and testable on a sample independent of the one that produced the result (price panel 2021-04..2024-12, or news and dividends before 2025). Each IND item gets a frozen rule, a placebo and a count in the family size.
- **EXP** new but only testable on the spent AlgoPack window (2025-01..2026-10); exploration grade only, can never confirm, reported with that label and not counted as evidence.
- **FWD** forward only; waits for the named read or for data to accumulate.
- **NA** not testable with our data, or not a hypothesis.

## Mapping (item numbers follow the owner's list)

| Items | Study | Class and pointer |
|---|---|---|
| 1 | A2 stability | FWD, frozen A2 rule read about 2027-01-20 |
| 2, 3, 4 | A2 by liquidity, spread, volatility | DONE, library v3 moderators (`docs/metrics.md` 8.31) |
| 5 | A2 by RVI | DONE, meta-variables wave 1 (8.27) |
| 6 | A2 by time of day | DONE, A2Early/A2Late/A2H1/A2H2 are inputs of the 100-hypothesis battery and the composite |
| 7, 10 | A2 by imbalance sign, by ticker group | EXP |
| 8, 9 | A2 after costs, with execution delay | DONE, wave 3 costs and next-open entry |
| 11, 20 | composite with RVI and for thin names on future data | FWD, round-3 read about 2027-02-05 |
| 12 | composite without order-book-shape features | EXP (needs a refit on the spent window) |
| 13, 14, 15, 16, 18, 19 | composite without momentum, at lower turnover, other rebalance rates, other carry, beta hedge, sector neutral | DONE (8.36, 8.37, wave 3 and 3b, 8.33) |
| 17 | composite long-only against long-short | EXP |
| 21, 22, 23, 24, 26, 28, 30 | attention shock by size, source, liquidity, regime, company versus aggregated, execution cost, announcement type | IND (news 2022-2024 is not regenerated), announcement type by frozen keyword classes |
| 25, 29 | attention shock horizons, after 2025 | DONE (8.32) |
| 27 | attention shock with real short availability | IND (carry scenarios 17% and 25%) |
| 31 | dividend entry timing | CHECK (rule v2 and the divrunup outputs) |
| 32, 35, 37 | dividend: how early before the announcement, by year, net versus gross | DONE (8.38) |
| 33, 34, 38, 39 | dividend by yield, liquidity, gap size, news after the announcement | IND |
| 36, 40 | broker calendar, replication on future data | FWD, 15 events and the daily calendar snapshot |
| 43, 44, 47, 49, 50 | opening-gap reversal: costs, borrow, holding period, independent periods, execution time | DONE, `20261027-algopack-gap-reversal-sleeve.md` (8.42: gross 0.5-2 bp a day, break-even 0.4-4.5 bp a side against 10-15 bp; rates 1.0 / 0.5 / 0.3; 2021-2024; open entry) |
| 41, 42, 45, 46, 48 | opening-gap reversal: direction, size, liquidity, volatility, market-adjusted | IND, not run: after 8.42 a split of a book that does not pay its costs cannot change the economics; a descriptive IC split on 2021-2024 is possible on request |
| 51, 52, 55 | timing by spread width, per-ticker windows, open versus later | DONE (8.34, tercile table) |
| 53, 58, 59 | timing by volatility, in stress, buy versus sell | EXP |
| 54, 57 | timing by order size, by liquidity | CHECK (the 1 mln run and the spread thirds) |
| 56 | timing with actual fills | NA until the sandbox fills accumulate; then FWD |
| 60 | timing in forward shadow | FWD |
| 61, 62, 65, 66, 67, 69, 70 | momentum on independent periods, lower turnover, horizons, long-only, borrow, in the composite, placebo | DONE (8.35-8.37) |
| 63, 64, 68 | momentum by liquidity, regime, after beta neutralisation | IND (price-only 2021-2024) |
| 71, 78 | pairs with pair-specific costs, by year | CHECK (`pairscan` output) |
| 72, 79 | pairs without borrow, short availability | DONE (8.30, +0.048% without borrow) |
| 73, 74, 75, 76, 77 | pairs by liquidity, regime, spread extremeness, holding period, executable prices | IND |
| 80 | pairs on future data | FWD |
| 81, 84, 85, 86, 87, 88 | thin names by liquidity level, turnover, regime, sector, rank normalisation, delay | EXP |
| 82, 83, 90 | thin names long-only after costs, long-short and borrow, short availability | DONE (8.29) |
| 89 | thin names on future data | FWD |
| 91, 92, 93, 94, 95 | fundamentals quality, size, value, added to the composite, point-in-time snapshots | FWD, daily snapshot since 2026-10-25 |
| 96, 98 | RVI rule and evening news sentiment on new data | FWD |
| 97 | RVI rule against the confidence control | EXP |
| 99 | calibration across regimes | EXP |
| 100 | conclusions reproduce under a pre-registered plan | NA, this is the programme's procedure, already applied to every plan |

## What is run

1. **CHECK first (no compute):** items 31, 54, 57, 71, 78 are read from the existing outputs and re-classed.
2. **IND batch, one tool per study, one frozen rule per item:** the opening-gap items run inside the gap plan; a generic moderator split (`cmd/alphafit -mode hetero`) handles momentum (63, 64, 68) with signal IC split by terciles of a moderator measured on the date (liquidity, volatility, beta, index trend), block-bootstrap interval of the difference, halves, a within-date permutation placebo, on 2021-04..2024-12 only; attention (21-24, 26-28, 30), dividend (33, 34, 38, 39) and pairs (73-77) use the same split inside `cmd/newsimpulse`, `cmd/divrunup` and `cmd/pairscan` respectively, each with its own plan addendum before its run.
3. **EXP items are not run for a verdict.** They may be listed with their numbers as exploration, labelled so, after the IND batch and only if the owner asks.

## Gate and multiplicity

A split hit needs the difference of the two tercile statistics with a 95% block-bootstrap interval excluding zero, the same sign in both halves, a placebo that does not reproduce it, and the Nyholt-adjusted bar for the number of IND items run in that study (stated in each addendum). A hit is a LEAD; it never reopens a closed result. Counts of classes: see the table (the owner's one hundred are 38 DONE, 5 CHECK, 2 NA, 15 FWD, 15 EXP and 25 IND, of which the five gap-reversal splits are not run).

## Honest expectations

Dependence hypotheses have low power: a tercile difference of an IC of 0.03 needs about 0.04-0.05 to clear a stable bootstrap interval on 900 dates, and most of the IND items inherit signals that did not clear a net-of-cost gate. The prior of any IND hit surviving the bar is about 5% per study. The value of the exercise is mostly to close the questions with numbers instead of leaving them open.

## Review log

Reviewer pass requested 2026-10-27; conditions are added as errata before any run.

## Errata

None yet.
