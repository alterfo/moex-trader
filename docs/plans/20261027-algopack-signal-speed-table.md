# Signal speed and market speed by year: a descriptive table (2026-10-27)

Status: DONE 2026-10-27 (descriptive; only the news shock fades). Registered 2026-10-27 before any run (English only). Owner's question: do we account for the market getting faster over time? Until now only through half-sample comparisons. This is a descriptive table, not a hypothesis test: it fixes which signals have lost strength or horizon over 2021-2026 and whether simple proxies of market speed moved. No gate, no verdict; any statement of a trend is read together with its standard error and the number of years.

## Table A: signal strength by year and horizon

For each calendar year 2021 (from April) to 2026 (to 2026-10-16) and each horizon of 1, 5 and 10 days: the mean daily rank IC of the signal against the abnormal return over the horizon (name minus the universe mean, dividend-drop days excluded, at least 30 names a day), with the number of dates. Signals (frozen, signs as defined in the earlier plans): **M256** 256-observation momentum; **GAP** minus the rank of the opening gap (B05); **ZC** the news count shock (known reversal: negative IC expected), 2022-2026 only because the news file starts in volume in 2021. A **trend slope** is reported per signal and horizon: the OLS slope of the daily IC on time in years with a Newey-West (lag horizon + 5) t, over the whole sample. The composite is not in the table: its out-of-fold history exists only for 2025-2026.

## Table B: market-speed proxies by year

From the matrix, by year: median stock turnover per day (ValDay, from the candle), median closing spread in basis points (SpBBO, the day's mean spread), median number of trades per day (Trades), the Amihud illiquidity, and the **one-day reversal IC** (rank of the day's return against the next day's abnormal return, a direct measure of how much of a day's move is undone next day). **Caveat stated in the output:** SpBBO and Trades come from AlgoPack columns that the exchange re-emitted for history before 2025, so their year-to-year path mixes real change with the regeneration; turnover, Amihud and the reversal IC come from candles and returns and are not affected. The 2022 hole (2022-02-25..2022-03-28) is excluded everywhere.

## Reading rules

A fall of a signal's IC or a shortening of its horizon is called out only if the slope's |t| is at least 2.0 and the yearly means move in the same direction in at least four of six years; otherwise the table is read as no detectable change, with the standard error. Tests of the one-day reversal IC trend are the same. Nothing here changes the gates of other plans. The table is single-sample and descriptive.

## Review log

Reviewer pass requested together with the independent batch; conditions are added as errata before the run.

## Errata

None yet.

## Result (scored once, 2026-10-27)

Numbers in `docs/metrics.md` 8.43, output `artifacts/algopack-speed/speed.txt`. M256 and GAP show no decay in strength or horizon (trend t -0.16 and +0.84); the news count shock fades since 2025 (1-day IC -0.040 in 2024 to -0.004 in 2026; trend t +2.06 at 1 day, +2.60 at 5 days) in a hump shape that coincides with the doubling of the headline file. Turnover and trades doubled and spreads halved between 2022 and 2024, partly retracing in 2026. Not a gate; no other plan changes.
