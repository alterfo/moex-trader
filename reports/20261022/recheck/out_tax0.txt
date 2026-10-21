# Dividend drift event-study (2) — 2026-10-21

- Calendar: dividends_pit.jsonl (2152 records), study window 2014-01-01 -> 2026-12-19
- Study events: 1591 (waves: AUTUMN:300, OTHER:1074, SPRING:217)
- Position: 30000 RUB notional, lot-quantized map[GAZP:10 GMKN:10 MTSS:10 RUAL:10], tax 0%, costs comm/spread/slip 0.0005/0.0005/0.0005 per leg

## Entry offset -10 (entry = close at LastBuyDate -10 TD, exit = ex-date close)

- n=1574: mean raw 0.015202, mean IMOEX-adjusted (excess) 0.015155, mean gap ratio 20.9676
- Wave-cluster bootstrap CI(mean excess): [0.008252, 0.022521] *
- Leave-one-out (mean excess > 0): ticker true | season true | year true | wave true

## Entry offset -5 (entry = close at LastBuyDate -5 TD, exit = ex-date close)

- n=1586: mean raw 0.004616, mean IMOEX-adjusted (excess) 0.004400, mean gap ratio 20.8104
- Wave-cluster bootstrap CI(mean excess): [0.000587, 0.008360] *
- Leave-one-out (mean excess > 0): ticker true | season true | year true | wave true

## Entry offset -3 (entry = close at LastBuyDate -3 TD, exit = ex-date close)

- n=1588: mean raw 0.000500, mean IMOEX-adjusted (excess) 0.002220, mean gap ratio 20.7842
- Wave-cluster bootstrap CI(mean excess): [-0.001601, 0.006254] 
- Leave-one-out (mean excess > 0): ticker false | season true | year true | wave true

## Entry offset -2 (entry = close at LastBuyDate -2 TD, exit = ex-date close)

- n=1589: mean raw 0.000202, mean IMOEX-adjusted (excess) 0.001895, mean gap ratio 20.7704
- Wave-cluster bootstrap CI(mean excess): [-0.001576, 0.005333] 
- Leave-one-out (mean excess > 0): ticker false | season true | year true | wave true

## Entry offset -1 (entry = close at LastBuyDate -1 TD, exit = ex-date close)

- n=1591: mean raw 0.001881, mean IMOEX-adjusted (excess) 0.003276, mean gap ratio 20.7024
- Wave-cluster bootstrap CI(mean excess): [0.000013, 0.006243] *
- Leave-one-out (mean excess > 0): ticker true | season true | year true | wave true

Coverage: events=1591 (>=30 true), waves=38 (>=2 true: 2014-AUTUMN, 2014-OTHER, 2015-AUTUMN, 2015-OTHER, 2015-SPRING, 2016-AUTUMN, 2016-OTHER, 2016-SPRING, 2017-AUTUMN, 2017-OTHER, 2017-SPRING, 2018-AUTUMN, 2018-OTHER, 2018-SPRING, 2019-AUTUMN, 2019-OTHER, 2019-SPRING, 2020-AUTUMN, 2020-OTHER, 2020-SPRING, 2021-AUTUMN, 2021-OTHER, 2021-SPRING, 2022-AUTUMN, 2022-OTHER, 2022-SPRING, 2023-AUTUMN, 2023-OTHER, 2023-SPRING, 2024-AUTUMN, 2024-OTHER, 2024-SPRING, 2025-AUTUMN, 2025-OTHER, 2025-SPRING, 2026-AUTUMN, 2026-OTHER, 2026-SPRING), outside-largest-wave share=93% (>=20% true)
