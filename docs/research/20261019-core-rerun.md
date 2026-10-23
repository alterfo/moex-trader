# Core rerun on refreshed data


Status: in progress, first version 2026-10-20. Plan and pre-registered rules: [../plans/20261019-core-rerun.md](../plans/20261019-core-rerun.md). Original results: [../metrics.md](../metrics.md). The paper is extended study by study; sections for studies not yet run say so.

## Abstract

The key core of the research programme is rerun on data refreshed to the freeze date D = 2026-10-16. The first finding concerns the data, not the models: most of the original core studies already used data up to 2026-10-16 (price snapshots, ridge table) or 2026-10-18 (news), so refreshing adds seven trading days to the depth studies and 33 news pairs, nothing to the cross-sectional studies. R1 (variance of the evaluation procedure) reproduces the original numbers exactly on the original file and, on the refreshed file, gives the same conclusions: the procedure noise N95 is 0.0148 AUC (original 0.0160), recipe A pooled AUC averages 0.4839 (below chance, original 0.4829), and the deep-history recipe D beats A in 15 of 15 paired configurations (+0.0173, original +0.0203). R2 (deployment gate) again passes nothing and removes the earlier C-logistic lead against a fresh control (+66494.64 against +68449.25 on the original six windows). R3-R6 are pending.

## 1. Data

All sources are the original ones, refreshed in place or rebuilt (the plan lists the paths in its data inventory).

| dataset | original (as cited in `docs/metrics.md`) | refreshed | difference |
|---|---|---|---|
| depth dataset `h10.csv` (15 price columns, horizon 10, dead band 0.5%, absolute labels) | 54017 rows, 21 tickers (17 trading names, DATA, 3 FX series), 2015-01-05..2026-10-07 | 45201 rows, 17 tickers, 2015-01-05..2026-10-16, 41441 labelled rows | seven more trading days; DATA and the FX series are not in the current config |
| TQBR daily snapshots | 881385 rows, 2014-2026-10-16 (section 8.4) | 881384 data rows, 2014-01-06..2026-10-16 | none |
| ridge score table | to 2026-10-16 | 6728 rows, 17 tickers, 2025-03-03..2026-10-16 | none |
| merged news archive | 153151 pairs (section 7.5) | 153184 lines, last record 2026-10-18 20:21 | +33 pairs |

Reproduction check of the rebuilt depth dataset against the original file: 45048 common (ticker, date) rows; every feature column is identical (maximum absolute difference below 1e-6, no mismatch of missing values); labels differ in 147 rows dated 2026-09-27..2026-10-05, which at the time of the original export had no complete 10-day forward window and now do. Everything the original file had is therefore reproduced; the refresh only extends it.

Limits: no price data after 2026-10-16 exists yet, so the forward window (rows dated 2026-10-17 and later) is empty; the 10-minute candles of the original step 30 were not kept and are outside the core rerun; the news sources keep the ToS restrictions recorded in `AGENTS.md`.

## 2. Method

The engine, cost model, windows, noise conventions and bootstrap settings are those of `docs/metrics.md` sections 0 and 2. For R1 the original script `scripts/procvar.py` (expanding purged walk-forward, 9 test folds of 63 days from 2025-04-01 plus shifts of 27 and 54 days, LightGBM + logistic average over 15 price columns; 75 configurations of recipe A, 15 of recipe D, 3 fold lengths) is run unchanged in a research directory on the ai-box (`~/moex-rerun/`), with the binary built from commit `40e94ca`. Two runs are made: the original file (reproduction) and the refreshed file. Summary statistics follow the definitions of `docs/metrics.md` section 3.6 and were computed with a throwaway script that is not committed (see Deviations). Inherited criteria: the N95 rule (single-configuration comparisons below N95 are inconclusive; new comparisons are paired over at least 15 configurations) and the registered structural-stability rule (rule 8 of the plan).

## 3. Studies and results

### 3.1 R1: variance of the evaluation procedure

| quantity | original | reproduction on the original file | refreshed |
|---|---|---|---|
| recipe A pooled AUC over 75 configurations: mean / SD / min / max | 0.4829 / 0.0108 / 0.4605 / 0.5009 | identical | 0.4839 / 0.0117 / 0.4611 / 0.5028 |
| SD over seed / training start / first-test shift | 0.0019 / 0.0059 / 0.0116 | identical | 0.0017 / 0.0060 / 0.0128 |
| N95 (900 pairs, same shift) / median | 0.0160 / 0.0055 | identical | 0.0148 / 0.0059 |
| recipe A (start 2024-01-01) pooled AUC by shift 0 / 27 / 54 | 0.4959 / 0.4765 / 0.4901 | identical | 0.4964 / 0.4731 / 0.4925 |
| recipe D pooled AUC by shift 0 / 27 / 54 | 0.5199 / 0.4917 / 0.5117 | identical | 0.5160 / 0.4923 / 0.5057 |
| D minus A over 15 paired configurations: mean / SD / min / max / positive | +0.0203 / 0.0052 / +0.0112 / +0.0272 / 15 of 15 | identical | +0.0173 / 0.0040 / +0.0107 / +0.0226 / 15 of 15 |
| fold length 42 / 63 / 84 days: pooled AUC | 0.4933 / 0.5005 / 0.5140 | identical | 0.4948 / 0.5003 / 0.5194 |
| SD of mean-fold AUC over the 75 A configurations | 0.0145 | identical | 0.0118 |
| median test rows per A configuration | 7652 | identical | 7362 |

Reproduction: every statistic on the original file equals the recorded value, so the pipeline and environment are unchanged. Refreshed: the differences are small and inside the measured noise (all |differences| of the means are below the original N95 of 0.016). The three claims of section 3.6 are all confirmed on the refreshed data: the procedure noise is of the order of 0.015 AUC, dominated by where the fold boundaries fall (SD over shift 0.0128 against 0.0017 over seed); recipe A is below chance on average; D beats A in every paired configuration (stable across both data versions, so a stable-pass under rule 8 for the paired depth comparison and a stable-fail for absolute predictability of A). The test sets are smaller on the refreshed file (7362 against 7652 median rows) because DATA and the FX series are no longer in it; this is a universe difference, not a data-quality one.

### 3.2 R2: deployment gate on control, C-logistic and F

Run by `scripts/rerun-r2.sh` at D = 2026-10-16 (datasets exported with `exportdataset` from 2024-09-01, news archive of 153565 records, purged walk-forward with embargo 10, seed as in the original script). The six original windows are rerun, and a seventh window x (train to 2026-09-16, test 2026-09-17..2026-10-16) is added; this is where the refresh gives new out-of-sample days. The full per-window table is in `docs/metrics.md` section 4.4.

| quantity | original (section 4.1 / 4.3) | refreshed |
|---|---|---|
| control total realized, six windows | +67815.78 (4.3) | +68449.25 |
| control total realized, seven windows | - | +65973.38 |
| C-logistic total realized, six windows | +81382.67 (4.1, against control +59829.47) | +66494.64 |
| C-logistic total realized, seven windows | - | +65817.78 |
| F total realized, six windows | +9669.22 (4.3) | +1910.64 |
| F total realized, seven windows | - | -2437.13 |
| mean validation AUC, control / C-logistic member / F (seven windows) | 0.5170 / - / 0.4830 (six windows, 4.3) | 0.5105 / 0.5269 / 0.4859 |
| worst max drawdown, control / C-logistic / F | 3.57% / 3.69% / 2.35% | 3.52% / 4.35% (kill trip) / 2.24% |
| windows at or above control, C-logistic / F | 4 of 6 / 1 of 6 | 3 of 7 / 2 of 7 |

Verdict by the inherited gate: nothing passes, as in the original. C-logistic fails criteria 1 (by 155.60 RUB), 2 and 3; F fails 1, 2 and 4. Two readings. First, the control reproduces within 1% in total (per-window differences up to 6000 RUB come from the larger news archive and library drift, as recorded in section 4.3), and F's loss pattern is the same. Second, the earlier C-logistic lead (+36% over control) vanishes against a fresh same-run control and the member now also trips the kill switch in w1: under rule 8 the lead was not structurally stable, and the pre-registered reading is that the original result was inside evaluation noise (single-window and single-total comparisons, N95 of R1). The new window x adds 21 trading days, too few to change any verdict.

### 3.3 Forward window (accruing)

`scripts/rerun-forward.sh` runs on the ai-box cron (`# moex-rerun-forward`, Mon-Fri 17:20 UTC, after the session and the ridge job). Models of the control, C-logistic member and F are trained once on data up to D = 2026-10-16 (`~/moex-rerun/forward/ens-abs.json`, `ens-exc.json`) and never retrained; each run replays rows from 2026-10-17 to the run date with the section 4.1 costs and appends cumulative realized, net MTM, trades and max drawdown per variant to `~/moex-rerun/forward/forward.csv`. First check, 2026-10-17..2026-10-19 (one trading day, 2026-10-19): control -0.77 RUB realized (1 trade), C-logistic 0 (no trades), F +4.88 (2 trades). Nothing is concluded from this; the forward results enter the paper and `docs/metrics.md` only after at least 60 trading days.

### 3.4 R3-R6

Not yet run. Because refreshing adds nothing to the price snapshots and the ridge table, R3-R5 are reproduction checks of the original code and numbers rather than tests on new information. The forward window starts empty and accrues daily.

## 4. Limitations and deviations

- Summary statistics of R1 were computed with an inline script that was not committed (research tooling, not a project component); the definitions are those of section 3.6.
- The refreshed dataset has 17 tickers, the original 21 (DATA and three FX series present). Test-row counts differ accordingly; comparisons of AUC use the same procedure on both and the original file is reproduced exactly.
- Only R1 and R2 have been run so far. In R2, criteria 5 (PBO) and 6 (preflight) were not evaluated, and the B and E variants were not rerun (lower priority in the plan).
