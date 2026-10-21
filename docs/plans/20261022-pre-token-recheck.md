# Re-check of hypotheses rejected before the AlgoPack and T-Invest tokens

Status: done 2026-10-22; criteria were fixed before the first run. R1 verdict: dividend capture rejected again on 1591 events (`docs/metrics.md` 7.11).

## What the tokens give (probed 2026-10-22)

- AlgoPack key (`MOEX_TRADER_ALGOPACK_TOKEN`): `futoi` with history from 2020 and without the 14-day delay, delayed trades and candles; `datashop/algopack/eq/tradestats`, `obstats`, `orderstats`, `hi2` still answer "information available only to subscribers". Order-flow hypotheses (H20 order flow at levels) therefore remain untestable on history; the forward trade recording on the ai-box (`cmd/tradesrecord`, since 2026-10-19) is the only route.
- T-Invest read-only token: `GetAssetReports` and `GetDividends` give history from 2014 for still-listed instruments (instrument uid); `GetAssetFundamentals` is a snapshot only.

## Inventory of earlier rejections

| Earlier result | Data limit at the time | Already re-tested with the tokens? | Action |
|---|---|---|---|
| Futures open interest, retail positioning (steps 14, `futoi`, `futoi14`) | free ISS: 2023-12.., 14-day lag | yes, step 29 (F1, F2), fail | none |
| Report timing, reaction drift, ridge report filter, dividend-yield factor (step 28 E1..E5) | none, first run with the tokens | this is the re-test | none |
| Dividend-adjusted labels (Experiment A) and dividend-yield feature (6.8) | calendar of 17 live tickers | partly: E5 on 289 tickers, fail | none |
| Dividend capture event study (7.11) | 65 events, 9 waves in 2024..2026, 17 tickers | no | **R1 below** |
| Fundamental ratios (step 15) | snapshot only | not possible before about 120 daily snapshots (recorder since 2026-10-19, earliest about 2027-02) | wait |
| Order flow at levels (H20 extension) | no order-book history | not possible, `obstats`/`orderstats` are subscriber-only | wait for forward recording |

## R1. Dividend capture on the wide calendar

Question: does buying before the last buy date and selling at the ex-date close earn a positive excess over IMOEX net of costs and tax, on a sample about 30 times larger than the 2026-09-25 rejection (65 events, 9 waves)?

Data: `dividends_pit.jsonl` (2152 records, point-in-time panel, T-Invest `GetDividends`, instrument uid), ISS daily candles. Protocol: unchanged from `docs/plans/completed/20260925-event-driven-dividend-drift.md` (`cmd/dividendstudy`): position 30000 RUB, costs 0.0005 x 3 per leg, offsets -1, -2, -3, -5, -10 trading days before the last buy date, exit at the ex-date close, excess over IMOEX, bootstrap over whole waves (seed 42, 2000 repeats), tax 0.13 and 0.00. Study window: last buy date 2014-01-01..2026-12-19. Excluded: events whose entry to exit span contains 2022-02-25..2022-03-28, and events with a daily close-to-close move beyond 45% in the span (splits and consolidations in unadjusted closes).

Criterion (the original, unchanged): the bootstrap CI of the mean excess excludes zero on a contiguous subset of offsets containing {-2, -3, -5} under both tax conventions, and the mean stays positive under leave-one-out by ticker, season, year and wave. Coverage bar: at least 30 events, at least 2 waves, at least 20% of events outside the largest wave. Descriptive without a criterion: the split 2014..2021 versus 2022..2026; the mean gap ratio.

Verdict is binary: accept (a record for forward observation, nothing goes live) or reject. A result of the old kind (ex-date drop almost fully priced, gap ratio near -1) is expected from the earlier run; a different outcome must hold in both sub-periods before it is believed.

## What we do not do

No change to the live path, config, models or `data/dividends.jsonl`. Nothing is retested for which the token gives no new data.
