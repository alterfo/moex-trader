# Alpha-without-beta hedge test (0)

- Window: 2025-04-01 -> 2026-09-17
- Tickers: YDEX, OZON, SBER, LKOH, GAZP, GMKN, ROSN, NVTK, TATN, MTSS, MGNT, PLZL, CHMF, DATA, T, VTBR, RUAL, POSI
- Deposit 1000000, target notional 15000, costs comm/spread/slip 0.0005/0.0005/0.0005
- Ensemble realized / unrealized: 202639.66 / -6158.73 RUB, closed trades 867
- Per-name beta (log-return OLS vs IMOEX, window): CHMF=0.894, DATA=0.583, GAZP=1.008, GMKN=0.818, LKOH=0.905, MGNT=0.821, MTSS=0.602, NVTK=1.091, OZON=0.853, PLZL=0.748, POSI=0.881, ROSN=0.879, RUAL=1.015, SBER=0.536, T=0.760, TATN=1.057, VTBR=0.767, YDEX=0.852

## Overlay share 0.00

- Leg P&L: 0.00 RUB, leg costs: 0.00 RUB
- Daily regression (cluster by quarter): N=487, skipped=0, R2=0.2300
- alpha=0.000325 (cluster t=3.983, p=0.0105), beta1(eqw)=-0.2906, beta2(mom)=-0.0844
- Block bootstrap CI(alpha): L=5: [0.000159, 0.000523]*, L=10: [0.000165, 0.000512]*, L=20: [0.000156, 0.000497]*, L=40: [0.000157, 0.000492]*, L=60: [0.000161, 0.000490]* | CI excludes 0 on contiguous {20,40}: YES

| quarter | n | mean daily alpha |
|---|---|---|
| 2025-04-01 -> 2025-06-30 | 81 | 0.000364 |
| 2025-07-01 -> 2025-09-30 | 86 | 0.000495 |
| 2025-10-01 -> 2025-12-30 | 83 | 0.000238 |
| 2026-01-05 -> 2026-03-31 | 77 | -0.000036 |
| 2026-04-01 -> 2026-06-30 | 87 | 0.000417 |
| 2026-07-01 -> 2026-09-17 | 73 | 0.000452 |

Leave-one-quarter-out alpha: drop 2025-04-01: 0.000318; drop 2025-07-01: 0.000292; drop 2025-10-01: 0.000355; drop 2026-01-05: 0.000392; drop 2026-04-01: 0.000296; drop 2026-07-01: 0.000288

## Overlay share 0.50

- Leg P&L: -68501.93 RUB, leg costs: 3743.86 RUB
- Daily regression (cluster by quarter): N=487, skipped=0, R2=0.1369
- alpha=0.000216 (cluster t=3.447, p=0.0183), beta1(eqw)=-0.1808, beta2(mom)=-0.1251
- Block bootstrap CI(alpha): L=5: [0.000087, 0.000362]*, L=10: [0.000102, 0.000348]*, L=20: [0.000096, 0.000338]*, L=40: [0.000099, 0.000335]*, L=60: [0.000104, 0.000334]* | CI excludes 0 on contiguous {20,40}: YES

| quarter | n | mean daily alpha |
|---|---|---|
| 2025-04-01 -> 2025-06-30 | 81 | 0.000189 |
| 2025-07-01 -> 2025-09-30 | 86 | 0.000369 |
| 2025-10-01 -> 2025-12-30 | 83 | 0.000196 |
| 2026-01-05 -> 2026-03-31 | 77 | -0.000069 |
| 2026-04-01 -> 2026-06-30 | 87 | 0.000260 |
| 2026-07-01 -> 2026-09-17 | 73 | 0.000340 |

Leave-one-quarter-out alpha: drop 2025-04-01: 0.000221; drop 2025-07-01: 0.000186; drop 2025-10-01: 0.000230; drop 2026-01-05: 0.000269; drop 2026-04-01: 0.000200; drop 2026-07-01: 0.000187

## Overlay share 1.00

- Leg P&L: -137003.86 RUB, leg costs: 7487.73 RUB
- Daily regression (cluster by quarter): N=487, skipped=0, R2=0.0229
- alpha=0.000101 (cluster t=2.100, p=0.0898), beta1(eqw)=-0.0654, beta2(mom)=-0.1707
- Block bootstrap CI(alpha): L=5: [-0.000002, 0.000209], L=10: [0.000012, 0.000196]*, L=20: [0.000017, 0.000190]*, L=40: [0.000018, 0.000179]*, L=60: [0.000030, 0.000179]* | CI excludes 0 on contiguous {20,40}: YES

| quarter | n | mean daily alpha |
|---|---|---|
| 2025-04-01 -> 2025-06-30 | 81 | 0.000012 |
| 2025-07-01 -> 2025-09-30 | 86 | 0.000237 |
| 2025-10-01 -> 2025-12-30 | 83 | 0.000152 |
| 2026-01-05 -> 2026-03-31 | 77 | -0.000102 |
| 2026-04-01 -> 2026-06-30 | 87 | 0.000088 |
| 2026-07-01 -> 2026-09-17 | 73 | 0.000214 |

Leave-one-quarter-out alpha: drop 2025-04-01: 0.000117; drop 2025-07-01: 0.000072; drop 2025-10-01: 0.000098; drop 2026-01-05: 0.000138; drop 2026-04-01: 0.000100; drop 2026-07-01: 0.000080

