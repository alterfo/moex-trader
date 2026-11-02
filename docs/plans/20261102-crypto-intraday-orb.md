# Crypto intraday: opening-range breakout, intraday momentum, level fade (2026-11-02)

Status: REGISTERED 2026-11-02, before any 5-minute price was read. English only. Owner's request of 2026-11-02, quoting three published strategies: (1) the 5-minute opening-range breakout on "stocks in play" (reported 1600% net, Sharpe 2.81, 36% annual alpha on 7000 US stocks 2016-2023), (2) Zarattini-style intraday momentum on SPY (reported 1985% net, 19.6% a year, Sharpe 1.33, 2007 to early 2024), (3) support and resistance levels combined with ATR for risk control. The question put to this file: have we tested these, and if not, test them properly on crypto.

Reviewer: none yet. No cell may be called confirmed without a completed review pass (rule 10 of `20261029-closing-the-limitations.md`).

## 1. What is already known, disclosed before the run

- **Intraday momentum was tested on MOEX and failed** (metrics 8.11, block I1, step 30): regression of the last 20 minutes on the first 20 minutes over 100823 ticker-days gave a coefficient of -0.016 with t -1.18, and the trading form -0.103% a day. The related overnight-versus-intraday asymmetry existed (+0.133% a day, t 6.90) but did not survive the 0.05% a side cost. This plan therefore starts from a prior of "null on equities, untested on crypto", not from the published results.
- **The opening is the most expensive part of the session**: the execution-timing study (metrics 8.31) found avoiding the open to be the single robust fact of that family. An opening-range breakout trades exactly there, twice, and this is priced below rather than argued away.
- **The one family that survived on MOEX is order flow** (A2 imbalance, t +19), and the crypto transfer reproduced it as an aggressive-flow lead with t 2.89 (metrics 10.3-10.6). The intraday cells here are therefore also a test of whether intraday breakouts carry anything the daily flow book does not.
- **A 10-minute MOEX pipeline reached the same AUC ceiling as daily bars** and intraday scalping was dropped by the owner on 2026-09-17 (`docs/knowledge/model-quality-history.md`). That decision was about the model, not about these rules; these rules are non-parametric and are being tested on a different market.
- **The published numbers above use selection** ("stocks in play") and report no capacity or borrow analysis. Every cell here is charged the venue's own retail fees, spread by liquidity tier and funding at the settlements that fall inside the hold.

## 2. Data

- `data.binance.vision` monthly `futures/um/klines` at interval **5m**, window 2023-01-01..2026-10-31, for the top 100 USDⓈ-M contracts by trailing 24-hour quote volume, classified `underlyingType = COIN` (errata 1 of `20261102-crypto-transfer-46` applies unchanged). A contract that was in the top 100 and later died stays in the sample until its data ends.
- New bytes only: prices already downloaded are daily. No 5-minute price has been examined before registration; the only thing read so far is the size of the bucket objects.
- Panel freeze with `chain_sha256` before the first scored run, exactly as in the daily stage.

## 3. Sessions and cells (frozen)

Two session definitions, both fixed here and not tuned:

- **S0, UTC day**: open 00:00 UTC, opening range = the first 30 minutes (bars 00:00, 00:05, ..., 00:25), session end 23:55 UTC.
- **S1, US cash**: open 13:30 UTC, opening range = 13:30-14:00, session end 20:00 UTC.

Three rules, each run in both sessions, which is six cells:

| cell | rule | entry | exit |
|---|---|---|---|
| O-ORB | long on a close above the opening range high, short on a close below the low, one position per contract per session, first breakout only, stop at the opposite boundary of the range, size = 1 unit of notional at the stop distance | first 5-minute close beyond the range | session end, or the stop, or 6 hours, whichever comes first |
| O-MOM | position = sign of the return over the opening range (close of the 30-minute window over its open), held as a dollar-neutral cross-section of the top and bottom quintile by that return | close of the opening-range window | session end |
| O-LVL | fade a level, or follow it if the same bar closes beyond it with volume above 1.5x the 20-bar median. Levels: previous session high and low, previous 7-session high and low, and the round-number grid within 10% of price for contracts priced below 100. A touch is a bar whose high or low comes within 0.25 ATR(14 bars of 5m) of the level. Stops at 1 ATR, exit at session end | the bar that touches | session end or stop |

"In play" filter for O-ORB and O-LVL: the contract must be in the top decile of the cross-section by 24-hour quote volume, and its opening-range volume must be above its own trailing 20-session median for the same window. O-MOM is cross-sectional and needs no filter beyond the universe.

## 4. Costs (the same frozen model, no new numbers)

Fees, spread and impact come from `internal/cryptocost` at the retail tier with the per-name liquidity tier; funding is charged at every settlement inside the hold (S0 crosses 08:00 and 16:00 UTC; S1 crosses 16:00 and 20:00). Two executions per cell: all-taker (the verdict read) and limit entry at the range boundary with a 55% fill probability (descriptive).

Intraday turnover is the point of the exercise, so the S1 stage runs first, before any signal is computed:

| book | trades a day | turnover a side | required gross a day at 13 bps all-in |
|---|---|---|---|
| ORB, top-100 majors | 1 | 1.0 | 26 bps |
| ORB, mixed tiers | 1 | 1.0 | 26-52 bps |
| any cell with two entries a day | 2 | 2.0 | 52-104 bps |

A cell whose gross per trade is below its own break-even is closed at that point and reported as such, not as a null.

## 5. Gates (frozen, same convention as 10.3)

1. **Economic.** Net of fees, spread, impact and funding positive in both halves of the window, in the taker read.
2. **Statistical.** Newey-West t of at least **3.3** on the daily net of the cell (Bonferroni over 46 registered lines, not relaxed for six cells).
3. **Multiplicity.** Deflated Sharpe with the trial count of the family (two sessions x three rules x two parameter variants where applicable), and the Bonferroni bar over the six cells printed next to it.
4. **Placebo.** Entry shifted by plus and minus one bar, and the level and sign sets permuted within session; a cell must lose at least 80% of its net under the shift.
5. **Stability.** Sign survives the two sessions, the two venues' fee schedules, the maker and taker executions, and the 2023-2024 versus 2025-2026 halves.
6. **Execution realism.** The stop is assumed to fill at the stop price minus one tick of slippage, and a gap through the stop fills at the bar open; a cell whose net depends on a stop filling at the exact level is reported as such.

## 6. Kill criteria

The intraday line closes, with what was measured written down, if any of these holds:

1. No cell reaches a positive net in the taker read on the full window (the venue's costs exceed the family).
2. The best cell's net is positive only with the maker execution.
3. The best cell's gross per trade is below its own break-even per trade on the S1 table.
4. Any result requires data that cannot be obtained without a live key.

## 7. Deliverables

- `cmd/cryptofetch -mode klines -interval 5m` for the panel, the freeze manifest, and a new `cmd/cryptointraday` (Go, one binary, modes `orb`, `mom`, `lvl`) sharing `internal/cryptocost`.
- Numbers in `docs/metrics/s10-crypto.md` 10.8 onwards, in the same commit as the run.
- The report page `docs/reports/crypto-w1.html` gains the intraday section only after the numbers exist.

## 8. Honest prior

The published results are large because the samples are equities with an overnight session, a wide choice of "in play" filters and no capacity analysis. Crypto has no overnight session, no closing auction, a session boundary that is a convention rather than an event, and a cost structure that punishes 1-2 round trips a day far more than the daily books of 10.3. The realistic outcomes are: one cell closes arithmetically at the S1 table, one or two show a gross effect that costs remove, and at most one survives as a lead. Zero survivors is the prior I would put money on.
