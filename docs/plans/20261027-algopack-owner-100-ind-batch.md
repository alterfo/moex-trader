# Owner's second hundred: the independent batch (2026-10-27)

Status: DONE 2026-10-27 (run once: 1 lead of 20, 2 not testable). Registered 2026-10-27 before any run (English only). Executes the IND class of `20261027-algopack-owner-100-triage.md` that is still open after the gap-reversal result: 20 dependence tests in four studies. Each is a split of a signal or a book that already exists; none is a new signal. All run on samples that exclude the spent AlgoPack window (price-only 2021-04..2024-12, news and dividends before 2025, pairs 2020-2024), and the 2025-2026 window is not read for any verdict here.

## Common rule (frozen)

A **split** compares a statistic between two or three groups (terciles of a moderator measured on the date, or regimes defined from data up to the previous day). The reported number is the difference (top minus bottom tercile; regime on minus off) of the group means of the daily rank IC (stock-level studies) or of the net per trade (pairs, dividends). Inference: block bootstrap over dates (block 20 days; month clusters for events) with the **Bonferroni-adjusted two-sided level 0.05 / k**, k the number of tests in the study; a split is a **hit** when that interval excludes zero, the sign is the same in both halves (split by date), and a within-date (or within-event) permutation of the moderator, five seeds, does not reproduce a hit. A hit is a LEAD, not a result; it never reopens a closed gate. Nulls are reported as nulls with the group means, so each question gets a number.

## Study 1: momentum (items 63, 64, 68; k = 6)

Signal `M256` (256-observation sum of abnormal daily returns, the confirmed momentum), outcome the 10-day abnormal rank `y10`, dates 2021-04-08..2024-12-17, from the playbook's feature file (SHA-256 299a7f9b…). Tests: (63) moderators D02 log turnover (liquidity), C04 volatility, C01 beta, top-minus-bottom tercile within date; (64) regime on/off: IMOEX below its own 200-day mean, RVI above its 60-day median (both known at the previous close); (68) beta-neutral: IC of the part of `M256` orthogonal to C01 (cross-sectional residual of the ranks) minus the raw IC, paired by date. Six tests.

## Study 2: attention shocks (items 21-24, 26-28, 30; k = 9)

Data `data/news_history.jsonl` (155,601 headlines, not regenerated), 2022-01-03..2024-12-17, prices from the matrix; the baseline signal is the count shock `zc` of `20261025-algopack-news-impulse.md` (known reversal), outcome the next-day abnormal return rank and the 5-day rank. `cmd/newsimpulse -dump` writes `zc` and its variants by date and name; `cmd/alphafit -mode hetero` does the splits. Tests: (21) size: mean next-day abnormal return of names with positive shocks, top tercile of `zc` minus bottom tercile (of the positive part); (22) source: IC of `zc` built from high-trust headlines (`trust_weight` at or above the median) minus IC of `zc` from low-trust ones, paired by date; (23) liquidity: IC by D02 tercile; (24) regime: IMOEX 200-day, RVI 60-day median (two tests); (26) company versus aggregated: IC of `zc` from headlines tagged to at most two tickers minus IC of `zc` from headlines tagged to three or more; (30) type: IC of `zc` built from keyword classes (dividend, sanctions and restrictions, results and profit; fixed stem lists in the code) minus the overall IC, per class (three tests). Nine tests. **Items 27 and 28 (short availability and execution costs)** are economics, not splits: a long-short quintile book on `-zc` through the `momsleeve` machinery (new signal kind `cand:-zc` from the dump), rates {1.0, 0.5, 0.3}, flat 0.15% and bbo costs, carry 17% and 25%, on 2022-01..2024-12; the break-even cost per side is the reported number; the gate is net positive with Newey-West t of at least 2.64 and positive halves.

## Study 3: pairs (items 73-77; k = 5)

`cmd/pairscan` economics (entry |z| at least 1.5, hold 5 trading days, non-overlapping, both legs at the day's BBO spread) on 2020-2024. Tests: (73) liquidity of the pair: tercile of the mean of the two legs' spread, wide minus tight; (74) regime: IMOEX 200-day, RVI 60-day median; (75) extremeness: |z| in [1.5, 2) versus 2.5 and above; (76) holding period: net at 3, 5 and 10 days (difference of 10 versus 5 and 3 versus 5, one test with the larger |t|); (77) executable prices: net with BBO costs ×1.5 (the close spread is wider than the day mean) and with a flat 0.15% a side; reported as net levels with the break-even, no hit rule. Month-cluster bootstrap on trades.

## Study 4: dividends (items 34, 38, 39; k = 3; 33 is already printed by the yield table and rank correlation of the original run)

`cmd/divrunup` events (115, 66 with yield of at least 3%), net of spread costs, buy 11 days before the last-buy day, sell the day before it. Tests: (34) liquidity: tercile of the entry spread, wide minus tight; (38) gap size: realised ex-date gap divided by the dividend (a post-event description, not tradable), high minus low half; (39) news after the announcement: events with a headline on the name between the announcement date (rule v2) and the entry day versus none. Month-cluster bootstrap; with 66-115 events the power is low and is stated in the output.

## Honest expectations

Dependence tests have low power: a tercile difference of an IC of 0.03 needs a difference of about 0.04-0.05 to clear a Bonferroni-adjusted interval on 900 dates, and event studies with 66-115 events cannot detect a moderate difference. The prior of any hit is about 5% per study; the economics rows are expected to fail (the news reversal sits on the short leg, and the gap result showed what daily-turnover books cost). The value is the numbers.

## What these tests do not cover

The EXP class (splits that exist only on the spent window), the FWD class, and the five gap-reversal splits (not run). No result here changes the live system.

## Review log

Pre-run Reviewer pass requested 2026-10-27; conditions are recorded as errata before any run.

## Errata

None yet.

### Implementation notes (written before any run, from the code as built)

1. Pairs: the matrix carries 1-, 5- and 10-day labels only, so test 76 reports holding periods 1, 5 and 10 days as net levels (no hit rule); 77 reports three cost treatments as levels; the pairs hit rule applies to 73, 74a, 74b and 75, so k = 4 and the level is 0.05 / 4.
2. Pairs trades are the original economics trades (|z| at least 1.5, non-overlapping, 2020-2024, broker carry on the short leg scaled by the holding period); the halves split is 2022-07-01.
3. Dividends: events with a yield of at least 3% and a last-buy day before 2025-01-01; net is the original run-up minus spread costs; the gap of test 38 is (close on the last-buy day minus close two sessions later) divided by the dividend, a post-event description.
4. Attention: `cmd/newsimpulse -dump` writes `zc` (identical definition to N01), the high- and low-trust, company and aggregated (three or more tickers per article) and keyword-class variants, and `zcneg` for the book; the economics rows use `cmd/alphafit -mode momsleeve -sig cand:zcneg -stage explore` with flat costs as the gate and bbo as description, on 2022-01..2024-12 only.
5. Momentum and attention splits run in `cmd/alphafit -mode hetero -hstudy momentum|attention` with `-hk 6` and `-hk 9`.
6. Frozen artefacts (hashes of `hetero.go`, `splits.go` in pairscan and divrunup, `dump.go`, the playbook feature file and the news dump) are recorded below before the first run.

Frozen before the first run (the Reviewer's pre-run answer had not arrived when the owner told the run to start; recorded as such): `hetero.go` SHA-256 ba60932bc927c880b8fda09609d6242dcbe6a66c2afd88afbb3c8d65d76f5a19; `pairscan/splits.go` b775e2b4467af82ca6fc7df1f01e41b27605a65d56aff8997d381f406ecba54f; `divrunup/splits.go` e31a7d54a0db32b34400e855a7c234e5bbdda85fbbe22cb5a5094dcfb0dd779c; `newsimpulse/dump.go` b33330dbcbfcb24469d6e8a8a75a8dd7cf675eb6dd9e3e45c6307745eef80da4; `momsleeve.go` 925d4619f8e22b56cbad298fffaf22d4000352d08bbaae49192da66609fd2c61; playbook feature file 299a7f9b73751d5c8ecbeaab1eba4d598351fb0a14bab65662a46323adab0c70; news dump 669b08bdc08dfe580236c1badc63ae07aa7f21208ce3545d56cdbcc49ce33675. Any change after the first output is a new plan.

## Result (scored once, 2026-10-27)

Numbers in `docs/metrics.md` 8.44, outputs `artifacts/algopack-owner-ind/`. 1 hit of 20 (momentum is stronger in low-beta names: IC +0.108 against +0.025, difference -0.082, level 0.0083, both halves, placebo clean), 2 not testable (size of positive shocks: 99 dates; source: the median trust weight is 1), 17 nulls. The attention reversal book is the closest standalone book so far (break-even 13 bp a side against 15 bp flat; bbo net +0.016% a day, t +0.92) but fails the flat-cost gate and the signal fades from 2025. Pairs and dividends splits are nulls. Errata: the dividend sample is 46 events (last-buy day before 2025), not 66; the pairs holding-period and executable-price rows are levels.
