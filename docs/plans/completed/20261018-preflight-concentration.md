# Analysis of the preflight refusal: P&L concentration by day (2026-10-18)

Russian translation: [20261018-preflight-concentration.ru.md](20261018-preflight-concentration.ru.md). This file is canonical.

The owner chose "stop and investigate". The trader is stopped by the gate, the thresholds are not touched. Hypotheses and criteria are fixed before the measurements run.

## What is known (already measured, numbers in `docs/metrics.md` "Sell-bias diagnostic")

- The preflight gate refuses deterministically: the 90-day replay (without news, i.e. without the veto) gives realized net of borrow −4859.13 against a threshold of 1 RUB. Earlier passes: +7156.33 (10-15 07:19), +602.32 (10-17 15:09). The gate fix is not the cause (both versions of the binary give a bit-for-bit identical refusal).
- My CLI replays WITHOUT net caps show the same trend: realized gross by till: 10-14 +33823, 10-15 +34374, 10-16 +33198, 10-17 +26492, 10-18 +10412. ISS has no bars for 10-17/10-18 — the shift of the result is created by the left edge of the sliding window (profitable days leave the window), not by new bars.
- The spread table from the DB is irrelevant: median half-spreads are 0.0004–0.05% by ticker.

## Hypotheses (fixed before the run)

- **I1. Concentration by day.** The realized P&L of the gate replay is concentrated in a small number of days; rolling these days out of the window turns the result negative. Measurement: the daily realized series (period_returns.csv) of the gate-form replay; the share of the top-10 days' P&L; the rolling 90-day sum by till.
- **I2. Degradation of fresh trades.** The average realized P&L per closed trade by closing month falls towards October. Measurement: mean/median over closed trades grouped by month, with a date bootstrap (descriptive).
- **I3. Expectation of self-recovery.** If I1 is confirmed and the profitable days have already left the window, the gate will not "pass again" by itself — checked by computing the rolling sum forward on the known series (where data exist) and by forecasting the boundary: will the window rise above the threshold at the current pace of fresh days.

## Protocol

- To reproduce the gate, `cmd/backtest` gets the flags `-max-net-short`/`-max-net-long` (a pass-through into the existing engine config `MaxNetShort`/`MaxNetLong` that preflight already passes); a flag parsing test in the same commit.
- Replays: `./backtest -config config.sandbox.yaml -tickers <17> -from <till−90d> -till <till> -deposit 1000000 -max-lots 1000 -commission-rate 0.0005 -spread-pct 0.0005 -spread-db trader-sandbox.db -slippage-pct 0.0005 -borrow-pct-day 0.00005 -target-notional 15000 -max-net-short 90000 -max-net-long 90000 -lookback-days 0 -signal-source ensemble -ensemble-path ensemble_model.json -wf-dir <dir>` for till ∈ {10-14, 10-15, 10-16, 10-17, 10-18}. Without `-news-history` (as in the gate).
- Limitation: lot quantisation (GAZP/GMKN/MTSS/RUAL lot=10) is not reproduced in the CLI; the difference is ~5–13% of notional on 4 tickers, recorded as a caveat. Also the gate takes till = the current moment, the CLI — a date; for daily bars the difference is immaterial.
- The numbers go into `docs/metrics.md` in the same commit. No changes to thresholds or to the live recipe.

## Outcome (2026-10-18)

Numbers in `docs/metrics.md`, section "Preflight-concentration analysis". The gate form was reproduced (`-max-net-short/-max-net-long` added to `cmd/backtest`): realized gross +9843 (till 10-14) → +3638 (10-16) → −1990 (10-18); ISS has no bars for 10-17/10-18, the decline is created by the left edge of the window. By closing month: July +415 (20 trades), August +2829 (35), September **−4822 (37, mean −130.32 per trade)**, October −412 (24). Concentration by ticker: OZON +4952, MGNT +1444 against PLZL −4508, CHMF −2331, POSI −1367, NVTK −992. I1 is partly confirmed, I2 is confirmed, I3 — there is no self-recovery in the coming weeks (the profitable August leaves the window, the loss-making September stays until December). The live path was protected by the veto (live has news and event features; the replay does not). The problem is the model quality in the September regime, not the gate; do not touch the thresholds.
