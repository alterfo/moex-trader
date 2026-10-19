# Beta / regime decomposition (Task 9)

- Window: 2025-04-01 -> 2026-09-17
- Tickers: 18 (YDEX, OZON, SBER, LKOH, GAZP, GMKN, ROSN, NVTK, TATN, MTSS, MGNT, PLZL, CHMF, DATA, T, VTBR, RUAL, POSI)
- Ensemble artifact: ensemble_model.json
- Costs: commission 0.0005, spread 0.0005, slippage 0.0005 (fractions of price)
- Deposit: 1000000 RUB, target notional: 15000 RUB/position, k=5, momentum rebalance every 10 trading days
- Regime flat band: |IMOEX quarter return| <= 3% is flat

## Results (net of costs)

| strategy | realized P&L | MTM P&L | closed trades | return on trade notional | max DD |
|---|---|---|---|---|---|
| ensemble (deployed) | 194293.1 | 188225.76 | 862 | +4.77% | 2.11% |
| equal-weight 18-ticker | -673.13 | -68102.99 | 516 | -0.24% | 11.97% |
| momentum long-only | -20948.09 | -38708.67 | 500 | -1.27% | 4.68% |
| momentum long+short | -32082.28 | -26713.25 | 1151 | -0.80% | 3.20% |

## Realized P&L decomposition net of IMOEX

- Samples: 862 closed trades
- Total realized P&L: 194293.1 RUB
- Gross exposure (sum of closed-trade notionals): 4076976 RUB
- Return on gross exposure: +4.77%
- IMOEX (beta) component: 129321.77 RUB
- Excess (alpha) component: 64971.33 RUB

| leg | trades | realized P&L | notional | IMOEX component | excess | return on notional |
|---|---|---|---|---|---|---|
| long | 261 | 67261.43 | 1986656.57 | 28429.45 | 38831.98 | +3.39% |
| short | 601 | 127031.67 | 2090319.43 | 100892.32 | 26139.35 | +6.08% |

## Regression: P&L ~ alpha + beta1*equal-weight + beta2*momentum

### Per-trade

Dependent variable is the realized return on entry notional for each closed trade; the independent variables are the equal-weight and momentum long+short benchmark returns over the same holding window. Standard errors are one-way clustered by ticker.

- N=862, parameters=3, clusters=18, df=17, R2=0.1112

| term | estimate | cluster SE | t-stat | p-value |
|---|---|---|---|---|
| alpha | 0.010559 | 0.007188 | 1.469 | 0.1601 |
| beta1 (equal-weight) | -1.680993 | 0.271760 | -6.186 | 0.0000 |
| beta2 (momentum) | 1.070396 | 1.379604 | 0.776 | 0.4485 |

### Per-day

Dependent variable is the daily portfolio return; standard errors are one-way clustered by OOS quarter. 0 daily observations fell outside the OOS windows or lacked a benchmark return and were dropped.

- N=486, parameters=3, clusters=6, df=5, R2=0.2334

| term | estimate | cluster SE | t-stat | p-value |
|---|---|---|---|---|
| alpha | 0.000308 | 0.000066 | 4.657 | 0.0055 |
| beta1 (equal-weight) | -0.274478 | 0.103980 | -2.640 | 0.0460 |
| beta2 (momentum) | -0.196772 | 0.186688 | -1.054 | 0.3401 |

## OOS quarter regimes (IMOEX)

| window | IMOEX return | regime |
|---|---|---|
| 2025-04-01 -> 2025-06-30 | -3.95% | trend-down |
| 2025-07-01 -> 2025-09-30 | -5.75% | trend-down |
| 2025-10-01 -> 2025-12-30 | +4.51% | trend-up |
| 2026-01-05 -> 2026-03-31 | +0.70% | flat |
| 2026-04-01 -> 2026-06-30 | -15.39% | trend-down |
| 2026-07-01 -> 2026-09-17 | -2.55% | flat |
