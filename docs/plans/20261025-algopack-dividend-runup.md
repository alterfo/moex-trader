# Dividend run-up: trading ahead of a predictable public flow (2026-10-25)

Status: REGISTERED 2026-10-25 (English only), before the run. Item 8 of the owner's order of work ("front-running"). Only the legal reading is tested: anticipating flows that are announced in advance and visible in public data. Trading ahead of client orders one has privileged knowledge of, and manipulation of the order book, are not tested and not proposed.

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

## Review log

Pre-run pass (opencode, 2026-10-25): no look-ahead; the ex-date cannot enter a window; the test is a meaningful registered measurement of limited power; three pins applied (errata).

## Errata

Written 2026-10-25 after the Reviewer's pass, before the run.

1. The effective ex-date is the index of the last-buy day plus 1 in both cases (also when the last-buy date is not a trading day and the index steps back one trading day); the post window starts at that index plus 2; the entitlement day is never measured.
2. The economics charges the entry day's quoted spread for both legs (in and out) and no hedge cost on the market leg: the market is a benchmark, not a traded hedge.
3. The effective sample of the bootstrap interval is the number of month clusters (about sixty), not the roughly hundred events; the power statement (a correlation of about 0.25 needed) reads against that.
