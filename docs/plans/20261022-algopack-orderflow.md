# Order-flow hypotheses on AlgoPack history (Super Candles, 2020-2026)

Status: first run DONE 2026-10-22 (English only): one of 12 tests passed (A2, book imbalance), numbers in `docs/metrics.md` 8.15; follow-up F1 (hedged form) failed on 2026-10-22; follow-up F2 (frozen rule, forward evaluation about 2027-01-20) is running. Criteria were fixed before the full run; a short trial run on the first nine months of 2020 was printed before it and changed no definition.

## Why now

The paid AlgoPack key (`MOEX_TRADER_ALGOPACK_TOKEN`, valid until 2027-10-17) opens `datashop/algopack/eq/tradestats`, `obstats`, `orderstats` and `hi2` with history from 2020-01-03 (probed 2026-10-22). This removes the blocker recorded in `20261022-pre-token-recheck.md` ("order flow at levels: wait for forward recording") and in H20 (`20261015-hypotheses-from-llm-review.md`, re-test condition 1). The forward trade recording (`cmd/tradesrecord`) stays as a second, independent sample.

## Data

`cmd/algopackfetch` downloads one gzip CSV per dataset per day into `data/algopack/<dataset>/<date>.csv.gz` (gitignored, about 10 GB): 5-minute bars, all tickers the endpoint returns that day (about 250, delisted names included, so no survivorship in the universe), weekdays from 2020-01-03 plus the working Saturdays found in Step 0. Resume skips existing files; a holiday gives a header-only file.

| Dataset | Content | Columns used |
|---|---|---|
| `tradestats` | OHLC, volume, value, trades; buy/sell split of trades | `pr_close`, `val`, `val_b`, `val_s`, `vol_b`, `vol_s`, `disb`, `trades` |
| `obstats` | order-book state: spreads, depth, imbalance | `spread_bbo`, `spread_lv10`, `imbalance_val_bbo`, `imbalance_val`, `val_b`, `val_s` |
| `orderstats` | placed and cancelled orders by side | `put_val_b`, `put_val_s`, `cancel_val_b`, `cancel_val_s`, `put_val`, `cancel_val` |
| `hi2` | market concentration metrics (long format) | `metric`, `value` |

`tradetime` is the bar end (first bar 10:05, published about 10 seconds later). Features at decision time use only bars with `tradetime` up to 18:30; the bars of the closing auction are never used.

### Step 0. Data validation (no returns)

Descriptive only, no outcome variable is touched. Report: files per dataset against the ISS trading calendar (missing and extra days); rows per ticker-day by year; share of bars with a zero `val`; agreement of the last `pr_close` of the main session with the ISS daily close (median absolute deviation; days above 1% listed); one-day moves below -45% or above +80% treated as corporate actions, as in `docs/knowledge/data-sources.md`; the list of `hi2` metric names with their coverage. Failing validation (more than 2% of ticker-days unusable in any year) means the affected year is dropped, not repaired.

### Step 0 result (2026-10-22, `cmd/algopackcheck`, no return or outcome variable computed)

- Download: 1775 days per dataset, 146.06 million rows, 7.3 GB, 0 failed requests. Five working Saturdays (2021-02-20, 2024-04-27, 2024-11-02, 2024-12-28, 2025-11-01) were missing because the fetcher skipped weekends; fetched afterwards with `-dates`.
- Calendar against ISS IMOEX trading days (1708): after the Saturdays only 2022-01-07 has no rows (header-only file; the stock market was closed). Days with rows that ISS does not list: 2026-02-23, 2026-05-01 (14073 rows, session 10:00..19:00) and 2026-10-22 (the day of the download, partial). Decision: 2026-10-22 is cut from every test; the other two are kept, since they have a full session.
- Coverage by year (median tickers per day / median 5-minute bars per ticker-day in the 10:05..18:50 session): 2020 241/47, 2021 245/62, 2022 243/63, 2023 241/98, 2024 243/93, 2025 251/80, 2026 253/61. The low counts are thin names without trades in most bars; the admitted universe (top 100 by value) is not affected. No bar has a zero `val`.
- From 2022 the file also contains morning (from 07:00) and evening (to 23:50) bars. Every feature and return uses the 10:05..18:50 session only.
- `obstats` and `orderstats` have the same days as `tradestats`; their row counts differ from `tradestats` by more than 10% on almost every day (they carry bars without trades), and on about 90 days they have fewer than 90% of the tickers of `tradestats`. Features take only the ticker-bars that exist in all three datasets, and a ticker-day needs at least 20 such bars.
- `hi2` has 11 metrics, all present in every year; `hhi_agressive` (the spelling of the source) exists, so D stays testable.
- Prices are not adjusted for corporate actions (VTBR 0.043 in 2020, GMKN 20214 in 2020, before the 5000:1 consolidation and the 100:1 split). The ISS daily candle query used for the check returns values on a different scale for VTBR, GMKN and PLZL (apparently adjusted; not inspected further) (median deviation of the last bar close 99.98%, 9869% and 898%). For the other 13 sample tickers the median absolute deviation of the last session bar close from the ISS close is 0.18..0.33%, with 90..190 days above 1% and 2..9 days above 5%. Therefore: no mixing of the two sources, and the corporate-action rule (one-day moves below -45% or above +80% are dropped, as in `docs/knowledge/data-sources.md`) applies to the `tradestats` series.

## Common protocol

- Universe: point-in-time, on each decision date the 100 tickers with the highest median daily value over the previous 60 trading days; sensitivity 50 and 200 (descriptive).
- Excluded: 2022-02-25..2022-03-28; ticker-days with fewer than 80% of the median number of bars of that ticker-year; days around corporate actions as above.
- Halves: 2020-01-03..2023-06-30 and 2023-07-01..2026-10-16; the same sign is required in both.
- Returns: log, abnormal = ticker return minus the equal-weighted return of the admitted universe of the same day. The daily close is the `pr_close` of the last `tradestats` bar with `tradetime` in 10:05..18:50; intraday returns use `tradestats` bar closes. Both come from one source (amended after Step 0, see below: ISS daily candles and `tradestats` disagree on corporate-action adjustment, so they are never mixed in one return).
- Features are ranked cross-sectionally within the date (rank scaled to [-0.5, 0.5]); no look at the distribution outside the training half for any scaling.
- Statistics: Fama-MacBeth by date with Newey-West (5 lags). Every test in blocks A-C carries the controls below; the criterion is on the controlled coefficient.
- Controls: the ticker's return of days t, t-1..t-5 (separately), log value turnover of day t against its 20-day mean, `spread_lv10` mean of day t. Without them, order-flow measures are mostly the same reversal and liquidity effects already known (compare the control of I4a in `docs/metrics.md`).
- Costs: 0.15% per side (the live tariff) is the verdict tariff; 0.05% per side is reported descriptively. Benchmark: the key rate of the day; a strategy is judged on its net return over the key rate of the capital it ties up.
- Multiple testing: the family is 12 tests (A 5, B 4, C 2, D 1); the threshold is |t| >= 3.0.
- No short book: the borrow cost is unmeasured. Trading forms are long top quintile against the equal-weighted admitted universe, i.e. long minus universe, as in step 25.

## Hypotheses

### A. Daily order-flow imbalance predicts the next day (5 tests)

Feature on day t from bars up to 18:30, one per test:
- A1 `tradestats` trade imbalance: sum(`val_b` - `val_s`) / sum(`val`).
- A2 `obstats` book imbalance: value-weighted mean of `imbalance_val` (10 levels).
- A3 `obstats` top-of-book imbalance: mean of `imbalance_val_bbo`.
- A4 `orderstats` net order flow: [(put_val_b - cancel_val_b) - (put_val_s - cancel_val_s)] / (put_val_b + put_val_s).
- A5 `orderstats` cancellation intensity: cancel_val / put_val (sign not fixed in advance, test is two-sided; the family threshold already covers it).

Outcome: abnormal return from the close of t to the close of t+1. Entry at the day-t close is feasible because the feature stops at 18:30 and the auction accepts orders until 18:50; the variant entering at the open of t+1 is reported descriptively.

Accept if all hold: the controlled coefficient has |t| >= 3.0; the same sign in both halves; in the trading form (long top quintile minus universe, daily, 0.15% per side) the net mean return per day is positive and exceeds the key-rate carry of the invested capital, with a positive net sum in both halves.

### B. Intraday flow predicts the rest of the day (4 tests)

Features at 13:00 from bars 10:00..13:00: B1 trade imbalance (A1 definition), B2 book imbalance (A2 definition). Outcome: abnormal return 13:00 -> 18:30. The same two features at 17:00 from bars 10:00..17:00 give B3 and B4, outcome 17:00 -> 18:30.

Accept: |t| >= 3.0, same sign in both halves, and the gross mean of the top-decile minus bottom-decile pair per day above the round-trip cost of the live tariff (0.30% for a long and a short leg, 0.15% for the long leg alone against the universe). Gross edge below that is recorded as "statistical, not tradable" (the outcome of I2 and I4a in `docs/metrics.md`, step 30).

### C. Order flow at price levels (H20 re-test, 2 tests)

Events: exactly the H20 extension events (real levels: 20-day extreme by closes and round number; placebo as in H20), recomputed on the admitted universe of this plan and the period 2020-2026 instead of 16 tickers and 2024-11..2026-10. Features measured over the three 5-minute bars before the crossing: C1 `obstats` `imbalance_val` in the direction of the move; C2 `orderstats` cancellation intensity on the side of the level. Outcome: signed excess return at 1 hour, as in H20. Test: the difference of the outcome between the top and bottom tercile of the feature, bootstrap by dates, for real-level events only; the placebo events serve as a negative control (the same difference must not appear there).

Accept: confidence interval without zero at the 99.6% level (family of 12), same sign in both halves, at least 200 events and 40 dates in each tercile, the placebo difference not significant, and the mean signed excess return of the top tercile above 30 bp.

### D. Market concentration (`hi2`) and the index (1 test)

One series: the metric `hhi_agressive` (level at 18:40, z-score over an expanding window of at least 250 days). Target: IMOEX close of t -> t+5, non-overlapping five-day periods, position = -sign(z) (concentration of aggressive orders as a crowding signal; the sign is fixed now and the opposite sign is not tried), costs 0.03% per position change, test period 2022-06-01..2026-10-16. Accept: Newey-West t (2 lags) of the net return in excess of the key rate minus buy-and-hold >= 3.0, hit rate >= 54%, positive mean in each of the two halves. If the metric name is absent in the data (Step 0), D is closed as "not testable", the metric is not replaced.

## Implementation notes fixed before the first run (2026-10-22, `cmd/orderflowstudy`)

These settle points the sections above left open; no outcome had been computed when they were written.

- Joined bars: a bar counts for a feature only if it exists in `tradestats`, `obstats` and `orderstats`; a ticker-day needs at least 20 such bars up to 18:30 (A), 10 up to 13:00 (B1, B2), 20 up to 17:00 (B3, B4). Book imbalance is weighted by the bar's `tradestats` value.
- Lags and the 2022 window: a return is dropped when either of its two days lies in 2022-02-25..2022-03-28, so controls reach about ten days past the window; rows with any dropped return are skipped.
- Universe: top 100 by median value of the previous 60 trading days (at least 40 observations), recomputed every day; 50 and 200 are descriptive. The last file (2026-10-22, partial) is not used.
- A trading form: each day long the top quintile by the feature (sign of the full-sample coefficient, which must agree with both halves) against the equal-weighted admitted universe, weights equal, cost 0.15% (and 0.05% descriptively) times the sum of absolute weight changes. The condition is a positive net excess over the universe in both halves; the key rate cancels in a long-minus-universe comparison, so it is not part of the condition. The absolute return of the long book against the key rate is reported in `docs/metrics.md` when the run is recorded, as context for any later decision.
- A, entry at the next open: outcome from the next open to the next close, abnormal against the universe, coefficient and t only.
- B: price at the checkpoint is the last bar close up to 13:00 or 17:00 (not older than 30 minutes), end price the last bar close up to 18:30 (not before 18:00). The accept condition uses the top-decile minus bottom-decile mean of the abnormal outcome per day, more than 0.30%, positive in both halves.
- C: the bars are 5-minute (AlgoPack) instead of the 10-minute bars of the original H20; a crossing needs the previous bar of the same day within 15 minutes, the crossing bar at 10:10..17:30 so that the 1-hour outcome ends by 18:30, the first crossing per day of each level type and direction. Levels: 20-day extreme by closes and its 4th-extreme placebo, round number with step 10^(floor(log10 price) - 1) and the half-step placebo; real events pool both level types, placebo events pool both placebos. Outcome: direction times the 1-hour log return minus the equal-weighted 1-hour return of the admitted universe, in fractions (not normalised by sigma, so it is directly comparable with the 30 bp hurdle). Features over the three bars before the crossing bar, at least two joined: C1 is the direction times the mean `imbalance_val`; C2 is the cancelled over placed value of the side opposite to the move (sell side for an upward crossing). Terciles are cut on the feature of the pooled real events only; the placebo gets its own cuts.
- D: IMOEX closes come from ISS (`data/macro/imoex.csv` starts only in 2023), the `MARKET` row of `hi2` at 18:40 is the series, the z-score uses the day's own value and all earlier days (at least 250). Periods start at the first IMOEX trading day from 2022-06-01 and step five trading days; halves split at 2024-07-31; the last period must end by 2026-10-21. The statistic is the strategy net of 0.03% costs minus buy-and-hold, so the key rate cancels as in F1 of step 29.
- Overlapping events: the extreme and the round-number crossings of one bar are separate events; the bootstrap resamples whole dates, so their dependence is accounted for.
- Seed of every bootstrap: 20261022; 2000 draws; the interval for C is the 0.2%..99.8% percentile range.

## Follow-up 1: the A2 pass (criteria fixed 2026-10-22 night, before any code of this part ran)

A2 is the only pass of the first run (`docs/metrics.md` 8.15). Two questions are left and no other feature, window or universe is opened.

### F1. Hedged form (offline, once)

Same A2 feature, same top-100 universe, same top quintile by A2, equal weights, rebalanced daily at the close. Hedge: a short position in IMOEX of the same notional as the long leg (beta fixed at 1, not estimated), standing in for the IMOEX future. Daily net return = mean simple return of the top-quintile stocks, minus the simple IMOEX return of the same close-to-close day, minus 0.15% times the sum of absolute weight changes of the stock leg, minus a flat 0.01% per day for the futures leg (roll, basis and slippage allowance; the notional does not change, so no per-trade fee applies). IMOEX closes come from ISS; days without an IMOEX close are skipped. Descriptively: the same book without the hedge, the cost at 0.05% per side, the beta of the top quintile to IMOEX.

Criterion, all required: the Newey-West t (5 lags) of the daily net return is at least 3.0; the mean is positive in each of the two halves (split 2023-06-30). Economic reading, reported separately: the annualised mean (times 252) against the mean key rate on the days where the rate is known. If the hedged net is below the key rate the verdict is "statistically present, not worth more than the deposit"; the book is then not built.

### F2. Frozen rule and forward evaluation

The rule is frozen as it is in `cmd/orderflowstudy` at the end of this follow-up: A2 from `obstats` bars up to 18:30 joined with the other two datasets, top-100 universe by the 60-day median value, top quintile, equal weights, long minus the equal-weighted universe, daily, 0.15% per side. Nothing in the rule, the universe or the costs changes after this point. The first forward date is 2026-10-21 (the first feature date whose next-day outcome the first run could not see; its feature dates ended on 2026-10-20). Data accrue through the daily job `scripts/algopack-daily.sh` (the last three days are always refetched, because a day fetched before the close is partial).

Evaluation happens once, after 60 forward feature dates with an outcome (about 2027-01-20), with `cmd/orderflowstudy -forward-from 2026-10-21`. Accept as confirmed if the mean daily net excess over the universe at 0.15% per side is positive and its Newey-West t (5 lags) is at least 2.0 (one pre-specified rule, so a lower threshold than the family of 12), and the A2 Fama-MacBeth coefficient over the same dates is positive. A weaker result is reported as it is; the forward window is not extended or repeated to rescue it. Nothing enters the live path in either case without a separate owner decision.

## Verdicts and consequences

- A block passes only when every one of its conditions holds. A pass is a record for forward observation (a shadow book next to the C2 and high52 books, criteria for it written before it starts), never a change of the live path, config or models.
- A fail is final for the same feature definitions: no changed windows, no extra controls, no sign flips after the fact. Variants are a new plan with a new family size.
- The honest prior is low: earlier daily-direction ideas failed, key-rate carry (14-21% a year) dwarfs every strategy result, and flow signals shrink fast after 0.15% per side. A null outcome is an expected result and is recorded in the same way as a pass.

## Run order

1. Finish the download (`cmd/algopackfetch`); record the file counts in this plan.
2. Step 0 validation report.
3. Implement `cmd/orderflowstudy` (feature build, blocks A-D, seed-free), unit tests for the feature definitions on a hand-made day.
4. One run on all blocks. Numbers into `docs/metrics.md` (section 9, order flow) and `docs/knowledge/model-quality-history.md` in the same commit; `docs/knowledge/data-sources.md` gets the AlgoPack row corrected.

## What this plan does not do

It does not touch the live path, `data/dividends.jsonl`, the ridge tables or any model. It does not use `data/trades` (forward recording) before 120 trading days accumulate; it does not buy or request any other dataset.
