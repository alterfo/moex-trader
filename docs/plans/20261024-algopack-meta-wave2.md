# AlgoPack meta-variables, wave 2: the vital few (2026-10-24)

Status: DONE 2026-10-24 (English only; scored once, no new hit), registered before any wave-2 series is built or aligned. Design agreed with the Reviewer (opencode, read-only) in the wave-2 critique of 2026-10-24. Continues `docs/plans/20261024-algopack-meta-variables.md` (wave 1, numbers in `docs/metrics.md` 8.21).

## The 80/20 map (measured, used to aim the hypotheses)

The owner asked which 20% of the effects give 80% of the result, and to use that in generating hypotheses. Measured on the existing artifacts:

- **Signal: one family.** In the frozen composite the ridge coefficient mass is diffuse (16 of 78 inputs hold 52%, 35 hold 80%, the elastic net keeps 29), so the model is not an 80/20 object. The signal is: the A2 family (net-imbalance of the order flow) alone gives IC 0.059 of the composite's 0.096 in realtime (62%), and the single feature A2H2 gave 0.1105 of the exploration composite's 0.1335 (83%).
- **Time: one regime.** The composite's daily IC sums to 43.1 over the 450 realtime dates; the 244 dates with the RVI below its trailing median (54% of the dates) carry 33.9 (79%). The partial-adjustment net is +0.070% per day in that state and -0.018% in the other: the whole book economics (more than 100%) sits in the low-RVI state.
- **Leg: the short side.** In the round-3 decile gate the bottom decile earned -1.11% against the top decile +0.14% abnormal per 10 days: the short leg is 89% of the spread.
- **Cost: turnover.** The daily-rebalanced book's gross (+0.282% per day) is entirely absorbed by costs (0.280%); the partial adjustment is what makes the book alive.

Consequence for the hypothesis list: the 80% of wave-1 rows (key rate, CNY, Brent, IMOEX, news, calendar) are not extended further. Wave 2 drills into the vital few (the A2-flow family, the low-volatility regime, the short leg, thin names) and adds only series that are new dimensions. The owner's "expand the hypotheses" is answered by about twenty-five rows, not by another hundred: ten decompositions of the one real lead, new-series rows with verdicts, and more controls.

## Three statuses (the Reviewer's split)

The RVI lead (F08 R: composite IC 0.045 with RVI high against 0.139 low, p 0.0024) was seen on the realtime window. A refinement of it on the same window is the same hypothesis in other clothes and can never be a confirmatory test of it; the confirmation of a seen lead is new data (a second year of realtime, about 2027-10; the 60-date alphafit forward window is too short for a regime effect). The lead stays a lead.

**Status 1: descriptive diagnostics, no verdict, no p, not in the family.** They are reported next to the lead as a coherent-or-not picture. A decomposition that hangs together raises the prior for the forward confirmation, one that does not lowers it (stated before the run). No percentile or other threshold variant of the RVI split is tried (optimising a seen result is forbidden by the no-new-door rule).

| Id | Diagnostic (all split by the F08 state, mean in state 1 / state 0 and the count) |
|---|---|
| D01 | S decomposition: the partial-adjustment net, gross, and the on-off form (trade only when RVI is low; in-sample) |
| D02 | long leg and short leg of the partial-adjustment book separately (gross contribution of each) |
| D03 | the A2 baseline IC and the composite IC with the whole A2 family zeroed (A2, A2Late, A2Early, A2H1, A2H2, UnexpA2; same frozen hyperparameters, refit in the same walk-forward) |
| D04 | single-feature IC of the vital features (A2, A2Late, A2H2, ImbVol, LevAsym, TicketMed, SpLv, UnexpA1, NJ) |
| D05 | the composite IC within liquidity-rank terciles (rank 1-33, 34-66, 67-100) |
| D06 | alternative volatility states, each against the same outcomes: V1 cross-sectional dispersion of the previous 5 days' returns high, V2 top-100 equal-weight realised volatility (10 days) high, V3 RVI 10-day change positive, V4 RVI minus 100 x the annualised 10-day realised volatility of IMOEX high; plus the agreement (share of equal states) of each with F08 |

**Status 2: new hypotheses with verdicts, counted in an enlarged family.** The wave-1 rows keep their wave-1 verdicts (thresholds fixed in plan 20261024-algopack-meta-variables, not reopened); the wave-2 rows are judged at 0.05 / M_eff with M_eff recomputed outcome-free over the enlarged set of state and factor series, and their p are conditional on the wave-1 look (said once, here). Same machinery as wave 1: causal binary states from data through d-1, "high" means above the trailing 250-day median, circular-shift permutation p, sign in both halves, A2 baseline sign agreement for R and S.

| Id | State = 1 when |
|---|---|
| G01 | OFZ slope: the 10-day return of the RUGBITR10Y total-return index minus that of RUGBITR1Y is positive (long end outperforming) |
| G02 | USDRUB 10-day realised volatility high (USD000UTSTOM) |
| G03 | RTSI above its 100-day mean |

Each of G01 to G03 gets R, S and T rows (9 rows). Six exposure-interaction rows (the I form of wave 1: rolling 60-day beta of the stock's daily return to the sector index against the standardised lagged 10-day change of that index): J01 MOEXOG, J02 MOEXFN, J03 MOEXMM, J04 MOEXCH, J05 MOEXEU, J06 MOEXTL. Dropped with reasons: RUONIA and MOSPRIME (information inside the key-rate states F01, F02), RUONIA kept out even as a redundancy check.

**Status 3: negative controls (identical machinery).** The wave-1 set (Saturn, lunar phase, Kp) grows by C04 Mercury retrograde (geocentric longitude falling at d-1, mean-element ephemeris), C05 day-of-month parity (even calendar day of d), C06 Bitcoin 10-day realised volatility high (Binance BTCUSDT daily close; external control only, never a hypothesis): 9 more rows, 18 control rows in all. The rule is unchanged and sharper: one control hit voids the real hits of the same use.

## Statistics, additions to the diagnostic dump

`cmd/alphafit -mode dump` is extended with columns from the same walk-forward (no new fit except D03): the long and short gross of the partial-adjustment composite book, the composite IC inside the rank terciles, and the daily IC of the vital single features. The existing columns must reproduce wave 1 exactly (mean ic_blend 0.09571, mean net 0.02979% per day; run stops otherwise). The D03 refit uses `-zero` (the listed columns set to the missing value 0 after ranking, equivalent to removal); its IC series is dumped separately.

## Power and expectations (written before the run)

The wave-2 state rows have the same power limits as wave 1 (differences below about 0.08 IC invisible for slow states). Expected: zero hits among the nine new state rows and six interaction rows. The diagnostics are not tests; what they can do is change the prior of the forward confirmation, in the direction stated above.

## Run order

1. Reviewer reads this file and the code changes.
2. Fetch the new series (ISS indices and USDRUB, Binance BTC) into `data/macro-long`; extend and rerun the dump on the ai-box, and the `-zero` refit.
3. Run `cmd/metascan -mode wave2` once; numbers into `docs/metrics.md` 8.22 and `docs/knowledge/model-quality-history.md`.

## Result (scored once, 2026-10-24)

Numbers in `docs/metrics.md` 8.22, outputs `artifacts/algopack-meta/metascan-wave2.txt`. The wave-1 rows reproduced byte-identically (regression check on the extended dump). Enlarged M_eff 30.39, threshold 0.0016 for the new rows. Status 2: no hit among the nine G rows (G02 USDRUB not testable: USD000UTSTOM stopped trading in 2025, 38 usable dates on one side) and the six J rows; smallest new p: J02 financials exposure 0.17, G01 R 0.58. Status 3: six controls, 18 rows, zero control hits (smallest C04 Mercury R p 0.0597), so the calibration holds and the rule voids nothing. The wave-1 lead keeps its wave-1 verdict (threshold 0.0026); against the enlarged threshold 0.0016 the F08 R p of 0.0024 would not pass, which is stated here and does not change its status as a lead. Status 1 diagnostics are in the metrics section; the picture hangs together in sign and is partly informative: the regime is carried by the order-book-shape features, not by the A2 flow family.

## Review log

Wave-2 critique (opencode, 2026-10-24): split into three statuses; no post-hoc refinement of a seen lead in the family; the percentile split dropped; the implied-realised gap a diagnostic; OFZ slope, USDRUB, RTS and six sector exposures kept; RUONIA, MOSPRIME dropped; Mercury retrograde, parity and Bitcoin as controls; the count about 25. Applied.

## Errata

(none)
