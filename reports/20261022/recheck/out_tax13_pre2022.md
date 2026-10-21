# Dividend drift event-study (2) — 2026-10-21

- Calendar: dividends_pit.jsonl (2152 records), study window 2014-01-01 -> 2021-12-31
- Study events: 844 (waves: AUTUMN:156, OTHER:582, SPRING:106)
- Position: 30000 RUB notional, lot-quantized map[GAZP:10 GMKN:10 MTSS:10 RUAL:10], tax 13%, costs comm/spread/slip 0.0005/0.0005/0.0005 per leg

## Entry offset -10 (entry = close at LastBuyDate -10 TD, exit = ex-date close)

- n=832: mean raw 0.018663, mean IMOEX-adjusted (excess) 0.010266, mean gap ratio -10.4749
- Wave-cluster bootstrap CI(mean excess): [0.004765, 0.015493] *
- Leave-one-out (mean excess > 0): ticker true | season true | year true | wave true

## Entry offset -5 (entry = close at LastBuyDate -5 TD, exit = ex-date close)

- n=839: mean raw 0.004398, mean IMOEX-adjusted (excess) -0.000623, mean gap ratio -10.3867
- Wave-cluster bootstrap CI(mean excess): [-0.005012, 0.003980] 
- Leave-one-out (mean excess > 0): ticker false | season false | year false | wave false

## Entry offset -3 (entry = close at LastBuyDate -3 TD, exit = ex-date close)

- n=841: mean raw -0.002192, mean IMOEX-adjusted (excess) -0.003673, mean gap ratio -10.3621
- Wave-cluster bootstrap CI(mean excess): [-0.007516, -0.000030] 
- Leave-one-out (mean excess > 0): ticker false | season false | year false | wave false

## Entry offset -2 (entry = close at LastBuyDate -2 TD, exit = ex-date close)

- n=842: mean raw -0.003227, mean IMOEX-adjusted (excess) -0.004415, mean gap ratio -10.3511
- Wave-cluster bootstrap CI(mean excess): [-0.007974, -0.001082] 
- Leave-one-out (mean excess > 0): ticker false | season false | year false | wave false

## Entry offset -1 (entry = close at LastBuyDate -1 TD, exit = ex-date close)

- n=844: mean raw -0.003694, mean IMOEX-adjusted (excess) -0.004432, mean gap ratio -10.4056
- Wave-cluster bootstrap CI(mean excess): [-0.007831, -0.001301] 
- Leave-one-out (mean excess > 0): ticker false | season false | year false | wave false

Coverage: events=844 (>=30 true), waves=23 (>=2 true: 2014-AUTUMN, 2014-OTHER, 2015-AUTUMN, 2015-OTHER, 2015-SPRING, 2016-AUTUMN, 2016-OTHER, 2016-SPRING, 2017-AUTUMN, 2017-OTHER, 2017-SPRING, 2018-AUTUMN, 2018-OTHER, 2018-SPRING, 2019-AUTUMN, 2019-OTHER, 2019-SPRING, 2020-AUTUMN, 2020-OTHER, 2020-SPRING, 2021-AUTUMN, 2021-OTHER, 2021-SPRING), outside-largest-wave share=89% (>=20% true)
