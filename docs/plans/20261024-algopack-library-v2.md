# AlgoPack library v2: 100 transforms and combinations of the vital features, tested for increment over the composite (2026-10-24)

Status: DONE 2026-10-24 (English only; scored once, 0 of 100), registered before the realtime run. The exploration-window run in `-libwin explore` mode was a debugging run on regenerated history (it produced the table format and the timing, not evidence). Design from the 80/20 map of `docs/plans/20261024-algopack-meta-wave2.md` and the Reviewer's triage of the model brainstorm.

## Why this plan and why it is shaped this way

The owner asked for another 100 hypotheses. The honest situation after about 700 registered rows is that the data axes are exhausted on the spent realtime window, and the three local models returned about five testable ideas of seventy-five. What remains legitimate is a systematic, generated library on the one family that carries the signal: the 16 features with the largest ridge coefficients (the vital few: `LevAsym`, `SpLv`, `NJ`, `A2`, `A2H2`, `A2Late`, `ImbVol`, `TicketMed`, `UnexpA1`, `Idle`, `VwapDev`, `Trades`, `PutOrders`, `A5`, `Depth`, `ImbSlope`), asked the only question that matters for the composite: does a transformed or combined version add anything beyond the composite already in the model. The library is generated, not hand-picked, so there is no selection of the favourable form, and it answers the objection "you never tried the smoothed, standardised or interacted version of the strongest features".

## The 100 rows

- **80 transform rows** = 16 features times 5 transforms, per ticker on its own previous observations: `mean5` (mean of the last five available observations including today, at least three), `z20` (z-score of today against the previous 20 observations, at least ten), `chg5` (today minus the mean of the previous five, at least three), `tercileRank` (rank within the liquidity tercile of the day, ranks 1-33, 34-66, 67-100), `lag1` (the previous available observation).
- **20 combination rows** (centred rank values within the date): the products LevAsym x SpLv, LevAsym x NJ, LevAsym x A2, SpLv x NJ, SpLv x A2, NJ x A2, TicketMed x A2, Idle x A2, Trades x LevAsym, Idle x SpLv, TicketMed x LevAsym, A2H2 x LevAsym, A2 x rb1 rank, NJ x Idle; the differences A2 - A2Late, LevAsym - SpLv, A2 - rb5 rank, ImbVol - A2, Depth - SpLv, A5 - A2. The ids `L001`-`L100` follow the order of the generator in `cmd/alphafit/lib2.go` (transform rows in feature-major order, then the combinations).
- **5 placebo controls** through the identical pipeline: `LevAsym` permuted across names within each date (seeds 20261024..28).

Each candidate is rank-scaled within the date over the names that have it (at least 30), the same convention as the composite.

## Statistic (frozen)

Realtime window, feature dates 2025-01-03..2026-10-16 (450 dates), halves split 2026-01-05. The composite predictions are the out-of-fold blend of the wave-3 primary parameters (`artifacts/algopack-wave3/w3-frozen-h10.json`, expanding walk-forward, embargo 11 trading days). For each date the target (rank of the 10-day abnormal return) is regressed on [rank of the composite prediction, candidate]; the **incremental slope** of the candidate is the daily coefficient, its mean over dates the statistic, with a Newey-West (lag 10) t. The ordinary rank IC of the candidate is reported next to it.

- **Family**: 100 rows. M_eff by the Nyholt method from the correlation matrix of the 100 candidates (outcome-free: the mean over dates of the cross-sectional rank correlation of each pair). Threshold on the two-sided normal p of the incremental t: 0.05 / M_eff. Benjamini-Hochberg at 0.10 reported.
- **Hit**: p at most the threshold, the incremental slope has the same sign in both halves, and the ordinary IC has the same sign as the slope.
- **Calibration**: the five placebo controls must give zero hits; one placebo hit voids every real hit (the machinery over-rejects).
- A hit is a lead only (the window has been looked at four times); the next step for a hit is a frozen forward rule, not a deployment.

## Honest expectations (before the run)

The composite already contains each of the 16 features. A transform or a product of them can add only the part the ridge cannot represent (a smoothing across days, a standardisation, a nonlinearity). The expected number of hits is zero; the plan is worth running because a zero closes the "you did not try transforms" objection with a falsifiable count, and because a hit among 100 on the strongest family would be the most direct pointer to a better model class. Power: the standard error of the mean daily incremental slope is of the order 0.003-0.01 depending on the candidate; the composite's own IC is 0.096, so a candidate with an incremental slope of about 0.02 or more is detectable.

## Run order

1. Reviewer reads this file and `cmd/alphafit/lib2.go`.
2. One run on the ai-box: `cmd/alphafit -mode lib2 -frozen work/w3-frozen-primary.json -matrix work/matrix.csv` (realtime window, about 20 seconds).
3. Numbers into `docs/metrics.md` 8.25 and `docs/knowledge/model-quality-history.md`.

## Result (scored once, 2026-10-24)

Numbers in `docs/metrics.md` 8.25, output `artifacts/algopack-library-v2/lib2-verdict.txt`. M_eff 95.49, threshold p 0.00052; mean correlation within the 16 feature blocks 0.391 (below 0.5, so the Nyholt threshold stands), between blocks 0.107. **0 hits of 100; 0 of 5 placebo controls; no discovery by the block rule.** Smallest incremental p: L068 A5 chg5 (slope -0.0181, t -3.06, p 0.0022, IC -0.007, so it fails the IC-sign condition), L089 Trades x LevAsym (t -2.82, p 0.0047), L009 SpLv tercile rank (p 0.037), L005 LevAsym lag1 (slope +0.0265, t +2.06, p 0.040). 4 rows have p below 0.05 (5 expected by chance) and 18 below 0.10 (10 expected). Registered reading: no candidate adds to the composite; the 18 against 10 excess at p 0.10 is inside what block-correlated candidates produce by chance. The explore-mode debug hits were never examined.

## Review log

Pre-run pass (opencode, 2026-10-24): no look-ahead in the transforms; the incremental regression, the out-of-fold predictions, the tercile fix, the placebo and the hit conjunction are correct; two amendments applied (errata 1-3).

## Errata

Written 2026-10-24 after the Reviewer's pass, before the realtime run.

1. M_eff and the block structure: the 100 candidates are 16 blocks of five near-duplicate transforms, so the Nyholt M_eff (about 95 in the debug run) overstates the number of effective hypotheses about alpha. The run prints the mean correlation within blocks and between blocks; the registered threshold stays 0.05 / M_eff, and if the within-block mean is 0.7 or higher the result will state that the threshold is not conservative for the block structure.
2. Block-discovery rule: two or more hits among the five transforms of the same feature count as one discovery; any forward rule is written per feature, not per transform.
3. Pins: the z20 standard deviation uses the population formula (division by n); the two candidate hits seen in the explore-mode debug run were not examined, and the realtime run is the first read of any hit.
