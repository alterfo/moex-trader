# Metrics: market exposure, hedging, borrow (section 5)

Part of [metrics.md](../metrics.md) (the index, conventions and ledger live there). Numbers here are the single source of truth for this topic.

## 5. Market exposure, hedging and borrow

### 5.1 Overnight gap stress (generation 1)

Open portfolio at the end of the 2026-06-19..09-17 replay: 18 positions, gross 266573 RUB (long 43687, short 222887, net -179200). The kill switch blocks new entries and does not liquidate, so a gap hits equity in full first.

| scenario | P&L | % of deposit | breaches 3% DD | breaches 0.5% daily |
|---|---|---|---|---|
| worst historical day (2026-03-09) | -3827.21 | -0.38% | no | no |
| worst per-ticker composite | -6731.51 | -0.67% | no | yes |
| synthetic -10% | +17919.97 | +1.79% | no | no |
| synthetic -20% | +35839.93 | +3.58% | no | no |
| synthetic +10% | -17919.97 | -1.79% | no | yes |
| synthetic +20% | -35839.93 | -3.58% | yes | yes |

### 5.2 Beta and regime decomposition (generation 1)

One continuous replay of the deployed ensemble, 2025-04-01..2026-09-17, 18 tickers, costs 0.0005 x 3, decomposed net of IMOEX.

| strategy | realized | MTM | closed trades | return on trade notional | max DD |
|---|---|---|---|---|---|
| ensemble (deployed) | +194293.10 | +188225.76 | 862 | +4.77% | 2.11% |
| equal-weight 18 tickers | -673.13 | -68102.99 | 516 | -0.24% | 11.97% |
| momentum long-only | -20948.09 | -38708.67 | 500 | -1.27% | 4.68% |
| momentum long+short | -32082.28 | -26713.25 | 1151 | -0.80% | 3.20% |

| leg | trades | realized | notional | IMOEX component | excess | return on notional |
|---|---|---|---|---|---|---|
| long | 261 | +67261.43 | 1986656.57 | +28429.45 | +38831.98 | +3.39% |
| short | 601 | +127031.67 | 2090319.43 | +100892.32 | +26139.35 | +6.08% |
| total | 862 | +194293.10 | 4076976.00 | +129321.77 | +64971.33 | +4.77% |

The IMOEX (beta) component is about twice the excess component: the book is short-tilted (601 short vs 261 long trades) and earned much of its P&L from short beta in a declining index. Regression of P&L on alpha, equal-weight and momentum benchmarks with cluster-robust errors: per trade (862 trades, 18 ticker clusters) alpha +0.01056 (t 1.47), equal-weight beta -1.681 (t -6.19), momentum beta +1.070 (t 0.78), R2 0.111; per day (486 days, 6 quarter clusters) alpha +0.00031 (t 4.66), equal-weight beta -0.274 (t -2.64), momentum beta -0.197 (t -1.05), R2 0.233. Quarter regimes (IMOEX return, +-3% flat band): 2025Q2 -3.95% down, 2025Q3 -5.75% down, 2025Q4 +4.51% up, 2026Q1 +0.70% flat, 2026Q2 -15.39% down, 2026Q3 -2.55% flat. Only one of six quarters is a bull quarter; the short book earned in the three down quarters.

### 5.3 Alpha without beta: synthetic IMOEX overlay (2026-09-25)

Pre-registered (commit `d39da47`). Does the deployed ensemble's daily alpha survive execution-level beta neutralisation by a synthetic IMOEX overlay (no position sign or size changes)? HEAD rerun: 867 trades, +202640 realized (about 4% engine drift against section 5.2).

| overlay share | leg P&L | leg costs | daily alpha (cluster t, df = 5) | beta equal-weight | beta momentum | bootstrap CI at block 20 / 40 |
|---|---|---|---|---|---|---|
| 0.00 | 0 | 0 | +0.000325 (3.98) | -0.291 | -0.084 | excludes 0 / excludes 0 |
| 0.50 | -68502 | 3744 | +0.000216 (3.45) | -0.181 | -0.125 | excludes 0 / excludes 0 |
| 1.00 | -137004 | 7488 | +0.000101 (2.10) | -0.065 | -0.171 | [+0.000017, +0.000190] / [+0.000018, +0.000179] |

At share 1.00 the CI at block 5 touches zero ([-0.000002, +0.000209]); blocks 10/60 give [+0.000012, +0.000196] / [+0.000030, +0.000179]. All six leave-one-quarter-out drops keep alpha positive. Registered bar met (CI excludes 0 on contiguous {20, 40}; leave-one-out positive), with the caveat that the overlay leg is frictionless and that **all numbers were produced without short borrow**. Rerun with borrow 0.00005 per day: share 0 alpha 0.000316 (t 3.84), 0.5 alpha 0.000206 (t 3.25), 1.0 alpha 0.000090 (t 1.85, p 0.124), CI {20, 40} still excludes 0 (L10 touches 0); at twice the rate (0.00010) the condition fails (L20 [-0.000008, +0.000170], L40 [-0.000006, +0.000158]). The alpha is sensitive to the real borrow rate.

### 5.4 Real IMOEX-futures overlay (2026-10-02)

The preceding test replaced by the tradable instrument: MX front-month series from ISS FORTS (quarterly roll 3 calendar days before last trade), contract notional 230000 RUB (GO 27464 RUB, about 11.9%), fee 15.18 RUB per contract per side. HEAD rerun base: 778 trades, +184077 realized.

| overlay | share | avg contracts | leg P&L | leg costs | avg daily alpha (cluster t) | beta equal-weight | CI {20, 40} excludes 0 |
|---|---|---|---|---|---|---|---|
| none | 0.00 | - | 0 | 0 | +0.000300 (3.68) | -0.287 | yes |
| synthetic cash IMOEX | 0.50 | - | -66079 | 3622 | +0.000193 (3.18) | -0.179 | yes |
| synthetic cash IMOEX | 1.00 | - | -132158 | 7244 | +0.000080 (1.83) | -0.065 | yes |
| real MX, whole contracts | 0.50 | 0.03 | -19000 | 1681 | +0.000266 (3.12) | -0.284 | yes (hedge almost never fires) |
| real MX, whole contracts | 1.00 | 0.48 | -174800 | 16906 | -0.000036 (-0.49) | -0.053 | no |
| real MX, divisible (unit 0.01) | 1.00 | 46.7 | -144940 | 14314 | +0.000028 (0.39) | -0.054 | no |

One contract (230000 RUB) against an average beta exposure of about 107000 RUB makes the hedge bang-bang; the fully divisible variant loses significance too (about 12800 RUB of basis, roll and tracking separate it from the frictionless overlay). **The alpha-without-beta claim does not survive the real instrument**: realized P&L is predominantly a net-short IMOEX-beta bet. This supersedes section 5.3 and closes the beta-neutral income lever at the current book size (about 200k); reopening needs a much larger book or per-name single-stock futures.

### 5.5 Market-neutral retraining (2026-10-03)

Excess-to-IMOEX labels against absolute labels, otherwise identical (13438 rows; absolute 5398 pos / 6802 neg / 1238 hold, excess 4116 / 4179 / 5143 hold: the dead band swallows more rows). Validation AUC (2026-06-18..09-16), xgb / lgbm / logreg / ensemble: absolute 0.4895 / 0.4922 / 0.4634 / 0.4824; excess 0.5447 / 0.5237 / 0.4570 / 0.5226 (ensemble +0.040, outside the noise band; not reproduced in the walk-forward of section 4.3). Hedged books (2025-04-01..2026-09-17, models rebuilt here):

| model | overlay | trades / realized | leg P&L | alpha per day (cluster t) | beta unhedged / hedged | CI {20, 40} excludes 0 |
|---|---|---|---|---|---|---|
| absolute | none | 789 / +221316 | 0 | +0.000363 (6.02) | -0.302 / - | yes |
| absolute | synthetic 1.0 | - | -143051 | +0.000130 (4.43) | / -0.066 | yes |
| absolute | real MX whole 1.0 | - | -189900 | +0.000007 (0.12) | / -0.056 | no |
| absolute | real MX divisible 1.0 | - | -165809 | +0.000062 (1.21) | / -0.057 | no |
| excess | none | 392 / +81881 | 0 | +0.000175 (2.96) | -0.189 / - | yes |
| excess | synthetic 1.0 | - | -43117 | +0.000108 (3.46) | / -0.073 | yes |
| excess | real MX whole 1.0 | - | -33100 | +0.000125 (2.49, p 0.055) | / -0.029 | yes (hedge fires rarely) |
| excess | real MX divisible 1.0 | - | -48030 | +0.000085 (2.04, p 0.097) | / -0.079 | no |
| excess, borrow 5 bp/day | synthetic 1.0 | - | -43117 | +0.000102 (3.17) | / -0.073 | yes |

Excess labels work as intended (beta loading -0.189 vs -0.302, a real intercept) at 2.6x lower realized P&L. With a divisible hedge the excess alpha is 0.000085 per day (t 2.04, p 0.097), about 2.7x the dead absolute residual but short of the registered bar. Not a deployment-ready income source.

**Phase 2b, regime-balanced training (2026-10-03), rejected.** Inverse-frequency weights by trailing cross-sectional mean return (`ENSEMBLE_REGIME_BALANCE`, window 21 or 63, 2-4 buckets) lower validation AUC in every variant (ensemble 0.4964 / 0.4948 / 0.4804 / 0.4887 against 0.5226 unweighted); after a divisible hedge the residual alpha turns negative (rb21 b3 -0.000124, t -1.57; rb21 b2 -0.000165, t -1.94, CI entirely negative). Beta-residualised price features were deferred (cross-zone live/backtest parity risk, small expected gain).

### 5.6 Managed-beta leverage grid (2026-09-25)

Beta-neutral book (`-shares 1`), borrow 0.00005 per day, leverage = larger `-target-notional` on the same 1M deposit (no broker margin). Acceptance: max quarterly DD at most 3% and positive realized net at every multiplier.

| target notional | realized net of borrow | daily alpha (t) | max quarterly DD | result |
|---|---|---|---|---|
| 15000 (1x) | +197275 | +0.000090 (1.85) | 1.11% | pass (baseline) |
| 30000 (2x) | +395785 | +0.000195 (1.92) | 2.15% | pass |
| 45000 (3x) | +589992 | +0.000293 (2.01) | 2.56% | pass |
| 60000 (4x) | +753632 | +0.000364 (1.95) | 3.55% | fail |

The hedged book scaled 2-3x under the 3% budget; this lever depends on the hedge, which section 5.4 shows is not achievable at the current book size.

### 5.7 Aggregate exposure cap (`max_net_exposure`)

**Unhedged live book, HEAD rerun 2026-09-30** (signed-net semantics; window 2025-04-01..2026-09-17, borrow 0.00005 per day; the 2026-09-25 figures predate the 2026-09-17 feature fixes):

| target notional | cap | realized net of borrow | trades | max quarterly DD | result |
|---|---|---|---|---|---|
| 15000 (1x, live) | 0 | +197275 | 871 | 1.96% | pass |
| 15000 | 90000 | +87861 | 341 | 1.71% | pass (income -55%) |
| 30000 (2x) | 0 | +395785 | 1012 | 3.57% | fail |
| 30000 | 60000 | +81730 | 234 | 2.21% | pass (2.31% at twice the borrow) |
| 30000 | 75000 | -6555 | 50 | 2.52% | fail (income negative) |
| 30000 | 90000 | +144058 | 249 | 1.94% | pass |
| 45000 (3x) | 0 | +589992 | 1050 | 4.26% | fail |
| 45000 | 90000 | -19338 | 74 | 4.95% | fail (worse than uncapped on both axes) |
| 45000 | 120000 | +127923 | 188 | 3.85% | fail |

The only cap keeping every quarter under 3% (2x with 60k) earns less than half the live 1x. The cap is discontinuous and non-monotonic (60k to 75k turns +81730 into -6555 as trades collapse 234 to 50), so the profitable region cannot be tuned into: the book's income is its market beta and an aggregate cap removes the cross-name diversification. Stay at 1x; the cap ships as a fail-closed guardrail only.

**Synthetic 100% short book (2026-09-30)**, SELL on every bar and BUY on the last: cap 0 +51381 (1581 trades, max DD 5.83%, 18 names); 30000 +4981 (2 trades, 0.86%); 45000 +5864 (3, 1.04%); 60000 +8831 (4, 0.87%); 90000 +11484 (6, 1.68%). A cap costs 78-90% of the income because `risk.Gate` is an order-level fail-closed check that never resizes: the first tickers fill the cap and the book freezes. The uncapped all-short control breaches the 3% drawdown on its own (5.83%), so the live loss of 2026-09-30 (17 shorts, net about -210k, -4.53 RUB realized over 60 trades) was not caused by a missing cap.

**Semantics change, signed net to gross (2026-10-01).** The cap now bounds the sum of `|signed position notional|`; the net cap is blind to gross risk (long 500k SBER and short 500k OZON net to zero). Per-ticker projection (the order replaces only its own leg) and an explicit `risk.Request.ExposureDeltaLots` are load-bearing: treating `TargetLots` as a delta double-counted held positions (a latent bug). Regression: the 100% short grid reproduces exactly (cap 0 +51381.37, 1581 trades, 5.83%; 30000 +4981.30, 2, 0.86%; 45000 +5863.69, 3, 1.04%; 60000 +8830.73, 4, 0.87%; 90000 +11483.63, 6, 1.68%); an intermediate book-wide-additive implementation collapsed the capped rows to 0 closed trades by blocking the final flatten-to-flat order (`TestHardenedGateMaxExposureAllowsReducingOrder`). Live keeps `max_net_exposure: 0`.

### 5.8 Short-borrow cost

**Measurement.** `cmd/borrowmeasure` (180-day lookback) shows 17 of 18 names shortable (DATA is not) and 0 margin-fee operations (0 RUB); the broker API exposes short margin risk rates but no borrow fee. The real T-Invest accounts (brokerage and investment piggy bank, 2020-2026, 271 and 2354 operations) contain no `MARGIN_FEE` operation and no short trade, and margin status is disabled on both (code 30051); the sandbox charges no rollover. Public terms: intraday shorts are free, overnight carry is charged per calendar day; a secondary source quotes the REPO rate at about 17% per year (unconfirmed by the account tariff). The project's stress rate of 0.005% per day (1.8% per year) is about ten times below this estimate. `-borrow-pct-day` charges the rate daily on short notional and reports `RealizedPnlNetBorrow`.

**Sensitivity of the ridge long-short replay** (section 8.8 recipe, 17 stocks, caps 90000), realized net of borrow, RUB:

| rate per day | window G | sum of six windows | positive windows | worst window |
|---|---|---|---|---|
| 0.00005 (1.8% per year) | +3916 | +684 | 3 of 6 | -2697 |
| 0.00023 (8.4%) | +3583 | -1611 | 2 of 6 | -3097 |
| 0.00047 (17.2%) | +3140 | -4672 | 2 of 6 | -3631 |
| 0.00070 (25.6%) | +2715 | -7605 | 2 of 6 | -4143 |

The gate window passes at any rate up to 25% per year, but at about 17% per year the six historical windows sum negative; if the real rate is close to REPO the short part of ridge long-short is loss-making on average.

### 5.9 Out-of-sample freeze (2026-09-17)

The abs-10d recipe is frozen for two quarters (minimum collection 183 days; real-money execution disabled): frozen at 2026-09-17T16:09:40Z, code SHA `30a422a9eb7193ab9300d00b4ec7c9190b98e94e`, recipe SHA256 `7d6e616c8973e8d2c32110e049b5a539c616ba367fab86732ac0aa850e51de9e`, model SHA256 `feb827494ba0bc943ab722d1dff888e0fe4c523bd170d463a5e3ca2fa11e6140`, config SHA256 `54c8d1949fd8c3b514c8a2b9c23a6c9048d7e20a433ad24038ab8eec6540b1ef`. `cmd/oosfreeze` recomputes the recipe hash from the current files and resets the collection count when it changes.

