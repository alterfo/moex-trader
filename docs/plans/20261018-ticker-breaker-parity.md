# Per-ticker circuit breaker in the gate replay (pre-registration, 2026-10-18, before the run)

Russian translation: [20261018-ticker-breaker-parity.ru.md](20261018-ticker-breaker-parity.ru.md). This file is canonical.

## Facts

- The live `cmd/trader` already runs with `risk.TickerBreaker` (`risk.circuit_breaker_*` are not set in `config.sandbox.yaml`, so the defaults apply: 3 losing round trips in a row or a cumulative loss of 5% of `target_notional`=15000, that is 750 RUB; the block holds until a manual `Reset`).
- The preflight replay (`internal/backtest`) did not contain the breaker: the gate evaluated a configuration without a protection that exists in live.
- This step does not tune thresholds. They are taken exactly from the live defaults.

## Change (code)

- `backtest.Config.Breaker *risk.TickerBreakerConfig` (nil = disabled, behaviour unchanged). The engine feeds the breaker with every execution (entry, add, partial and full close, flip, time-exit) and passes it to the risk gate. A test in the same commit.
- `cmd/backtest -breaker`: enables the breaker with the defaults and `-target-notional` as Notional.
- The breaker is NOT wired into the `cmd/trader` preflight in this step. Wiring it changes the gate input and is done only after an explicit "yes" from the owner.

## Experiment (one run, no tuning)

The gate form from `docs/plans/completed/20261018-preflight-concentration.md` (17 tickers, 90 days, deposit 1M, costs 0.0005×3 and spreads from `trader-sandbox.db`, borrow 0.00005/day, limits 90000/90000, ensemble `ensemble_model.json`, no news), with and without `-breaker`, for till ∈ {2026-10-18, 2026-07-20, 2026-04-21}. Three non-overlapping 90-day windows: one "bad" (the latest) and two controls.

## Criteria

- **Primary:** with the breaker, the realized P&L of the till=2026-10-18 window is not below 1 RUB (the gate threshold) AND the sum of realized P&L over the three windows with the breaker is not below the sum without the breaker.
- If the 10-18 window passes but the sum over three windows is lower: this is "protection at the price of forgone profit", not a fix of the forecast; the conclusion is worded exactly so.
- If the 10-18 window does not pass: the breaker does not solve the gate problem; thresholds are not changed, the branch is closed.
- The outcome of either branch is only descriptive; enabling it in preflight requires an explicit owner decision.

## Result (2026-10-18)

The criterion is not met on either part; the numbers are in `docs/metrics.md` (section "Ticker circuit breaker in the gate replay"). The 10-18 window with the breaker: realized P&L net of borrow -3356.64 (without: -2455.72); the sum over three windows is +25943.92 against +28439.15. The branch is closed, thresholds were not changed, nothing was wired into preflight. Side finding: the gate also rejects closing orders for a ticker that has tripped, so the position is frozen.
