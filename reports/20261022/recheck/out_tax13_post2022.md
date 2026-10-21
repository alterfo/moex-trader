# Dividend drift event-study (2) — 2026-10-21

- Calendar: dividends_pit.jsonl (2152 records), study window 2022-04-01 -> 2026-12-19
- Study events: 740 (waves: AUTUMN:144, OTHER:485, SPRING:111)
- Position: 30000 RUB notional, lot-quantized map[GAZP:10 GMKN:10 MTSS:10 RUAL:10], tax 13%, costs comm/spread/slip 0.0005/0.0005/0.0005 per leg

## Entry offset -10 (entry = close at LastBuyDate -10 TD, exit = ex-date close)

- n=735: mean raw -0.003623, mean IMOEX-adjusted (excess) 0.005367, mean gap ratio 56.6357
- Wave-cluster bootstrap CI(mean excess): [-0.005764, 0.018221] 
- Leave-one-out (mean excess > 0): ticker true | season true | year false | wave false

## Entry offset -5 (entry = close at LastBuyDate -5 TD, exit = ex-date close)

- n=740: mean raw -0.009573, mean IMOEX-adjusted (excess) -0.004817, mean gap ratio 56.2553
- Wave-cluster bootstrap CI(mean excess): [-0.009796, 0.000690] 
- Leave-one-out (mean excess > 0): ticker false | season false | year false | wave false

## Entry offset -3 (entry = close at LastBuyDate -3 TD, exit = ex-date close)

- n=740: mean raw -0.010921, mean IMOEX-adjusted (excess) -0.005884, mean gap ratio 56.2553
- Wave-cluster bootstrap CI(mean excess): [-0.011640, 0.000883] 
- Leave-one-out (mean excess > 0): ticker false | season false | year false | wave false

## Entry offset -2 (entry = close at LastBuyDate -2 TD, exit = ex-date close)

- n=740: mean raw -0.010421, mean IMOEX-adjusted (excess) -0.005790, mean gap ratio 56.2553
- Wave-cluster bootstrap CI(mean excess): [-0.010796, -0.000101] 
- Leave-one-out (mean excess > 0): ticker false | season false | year false | wave false

## Entry offset -1 (entry = close at LastBuyDate -1 TD, exit = ex-date close)

- n=740: mean raw -0.006451, mean IMOEX-adjusted (excess) -0.003019, mean gap ratio 56.2553
- Wave-cluster bootstrap CI(mean excess): [-0.007188, 0.001260] 
- Leave-one-out (mean excess > 0): ticker false | season false | year false | wave false

Coverage: events=740 (>=30 true), waves=15 (>=2 true: 2022-AUTUMN, 2022-OTHER, 2022-SPRING, 2023-AUTUMN, 2023-OTHER, 2023-SPRING, 2024-AUTUMN, 2024-OTHER, 2024-SPRING, 2025-AUTUMN, 2025-OTHER, 2025-SPRING, 2026-AUTUMN, 2026-OTHER, 2026-SPRING), outside-largest-wave share=85% (>=20% true)
