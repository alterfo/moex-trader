# Dividend run-up: trading ahead of a predictable public flow (2026-10-25)

Status: DONE 2026-10-25 (English only; scored once, a lead: the pre-registered economic gate passes, the statistical rows do not), registered before the run. Item 8 of the owner's order of work ("front-running"). Only the legal reading is tested: anticipating flows that are announced in advance and visible in public data. Trading ahead of client orders one has privileged knowledge of, and manipulation of the order book, are not tested and not proposed.

## Hypothesis

The dividend calendar (the last-buy date, announced weeks ahead) creates a predictable demand: holders buy shares to be entitled to the dividend and sell after the ex-date. If the run-up is not fully priced, a long position taken about ten trading days before the last-buy date and closed the day before it earns an abnormal return that grows with the dividend yield; after the ex-date the opposite pressure may leave a drift. This is a calendar anticipation, tradable long-only (no borrow), the cleanest legal analogue of "front-running" a known flow.

## Data

`data/dividends.jsonl` (the broker dividend calendar, 123 records), `last_buy_date` and `dividend_net` (RUB per share), deduplicated by ticker and last-buy date, `YNDX` and `TCSG` mapped to their current tickers. Prices: the main-session close of the top-100 matrix (`work/matrix.csv`, columns `CloseDay` and `SpBBO`); an event needs the name in the top-100 at 11 trading days before and 1 day before the last-buy day. The yield is the net dividend over the close 11 trading days before the last-buy day (a price known at entry). The ex-date and the day around it are never inside a measured window: the run-up windows end at the close before the last-buy day (the last day with the entitlement is excluded to avoid the one-day dividend-drop flag), the post window starts at the close two days after the last-buy day.

## Rows

| Id | Row |
|---|---|
| DV1 | 10-day run-up (close 11 days before to close 1 day before the last-buy day) abnormal return against the equal-weight market, rank correlation with the yield across events |
| DV2 | the same for the 5-day run-up |
| DV3 | the post-ex-date drift (close two days after the last-buy day to close seven days after), rank correlation with the yield |
| DE | economic read of DV1: long-only, buy at the close 11 days before the last-buy day and sell at the close before it, abnormal against the market, net of the quoted BBO spread in and out (0.05% commission plus half the spread per side, 0.15% where missing), for yield thresholds 0, 3% and 6% |

Statistic: the Spearman correlation between the yield and the abnormal return over events with a cluster bootstrap by calendar month of the last-buy date (3000 draws, seed 20261025) and the mean abnormal return of the half of the events with higher yield against the lower half; DE with a bootstrap interval of the mean over events. **Hit** (DV1-DV3): the 95% bootstrap interval of the correlation excludes zero and the high-yield half's mean abnormal return has the registered sign (positive for DV1 and DV2, no sign registered for DV3). A row with fewer than 40 events is "not testable". DE passes if the interval of the net mean is above zero for the yield threshold chosen before the run at 3% (the other thresholds are shown for context, not as a search). Family of three rows plus one economic read; no multiplicity adjustment beyond the interval.

## Honest expectations

Only about 100 events can enter (the calendar file is short and the top-100 filter drops many), so the power is low: a correlation of 0.25 would be needed for the interval to exclude zero. The run-up before the last-buy date in Russia is a known anecdote with large events (SBER, LKOH, MTSS) and the net may be positive for the large-yield events; with 100 events a null is the expected statistical outcome and a negative result for the economics is likely at 0.15%+ spreads in and out. The result is not a claim about index-review or futures-expiry flows: the repo has no constituent-change data and the 26 quarterly review dates give no power, so those rows are not registered (an acquisition item).

## Run order

1. Reviewer reads this file and `cmd/divrunup`.
2. One run on the ai-box.
3. Numbers into `docs/metrics.md` 8.33 and `docs/knowledge/model-quality-history.md`.

## Result (scored once, 2026-10-25)

Numbers in `docs/metrics.md` 8.33, output `artifacts/algopack-dividend/divrunup.txt`. 115 events (median yield 3.3%). **Statistical rows: none is a hit** (the bootstrap intervals include zero): DV1 rank correlation of the yield with the 10-day abnormal run-up +0.138 [-0.076, +0.338] (mean abnormal +2.33% in the higher-yield half, +0.59% in the lower, +1.47% all), DV2 5-day +0.013 [-0.193, +0.219] (+0.85%, +0.13%, +0.49%), DV3 post-ex-date drift -0.177 [-0.395, +0.054] (-0.92%, +0.18%, -0.38%). **The pre-registered economic read passes:** buying at the close 11 trading days before the last-buy day and selling at the close before it, long only against the market, yield 3% or more: 66 events, gross +2.26%, **net of the quoted spread both ways +2.13%, bootstrap 95% [+1.14%, +3.13%]**; yield 0%: +1.34% [+0.40%, +2.19%] (115 events); yield 6%: +2.38% [+0.83%, +3.88%] (31 events). Robustness added after the first read (not conditions): the month-cluster bootstrap for the 3% rule gives [+1.13%, +3.10%]; by year +2.33% (2021, 14 events), +1.35% (2022, 8), +3.85% (2023, 9), +2.73% (2024, 15), +0.69% (2025, 11), +1.55% (2026, 9): positive in each of the six years, weakest in 2025. Registered reading: **a lead** (the economic gate passes with small, positive, year-stable numbers, while the yield does not rank the run-up within the sample: every event, not only the high-yield ones, gains in the ten days before the last-buy day).

**Caveat that bounds the lead:** the calendar file holds the final record of each dividend; `declared_date` in it is the record/ex date, not the announcement date, so the plan's assumption that the last-buy date is known 11 trading days (about 15 calendar days) earlier cannot be verified from this file for any event (large companies announce weeks ahead, interim payers sometimes later); the result is an upper bound conditional on that foreknowledge. Other caveats: only dividends that were paid appear in the calendar; 115 events is a small sample; the long position is unhedged against the market (dividend payers are low-beta names); capacity of a calendar trade is limited to a few names a month.

## Forward rule (frozen 2026-10-25, before any forward event matures)

Point-in-time capture: `scripts/dividends-snapshot.sh` (launchd template `com.oleg.moex-trader.dividendssnapshot.plist`, 06:10 Moscow, **not loaded**) writes the broker dividend calendar for the top-100 names as of each run date into `data/dividends-snapshots/`; the first snapshot (2026-10-25) holds one upcoming event (LKOH, last-buy day 2026-12-08, net 535 RUB). An event qualifies if it first appears in a snapshot at least 11 trading days before its last-buy day and its yield (net dividend over the close 11 trading days before the last-buy day) is 3% or more. Rule: buy at that close, sell at the close before the last-buy day, long only, abnormal against the equal-weight top-100, net of the entry-day quoted spread both ways. Read once, when 15 qualifying events have matured (not extended, not repeated): the mean net abnormal return with a month-cluster bootstrap interval above zero. Nothing enters the live path.

## Review log

Pre-run pass (opencode, 2026-10-25): no look-ahead; the ex-date cannot enter a window; the test is a meaningful registered measurement of limited power; three pins applied (errata).

## Errata

Written 2026-10-25 after the Reviewer's pass, before the run.

1. The effective ex-date is the index of the last-buy day plus 1 in both cases (also when the last-buy date is not a trading day and the index steps back one trading day); the post window starts at that index plus 2; the entitlement day is never measured.
2. The economics charges the entry day's quoted spread for both legs (in and out) and no hedge cost on the market leg: the market is a benchmark, not a traded hedge.
3. The effective sample of the bootstrap interval is the number of month clusters (about sixty), not the roughly hundred events; the power statement (a correlation of about 0.25 needed) reads against that.
