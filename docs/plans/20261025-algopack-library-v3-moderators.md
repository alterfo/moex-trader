# AlgoPack library v3: where does the composite work? 105 stock-level moderators (2026-10-25)

Status: DONE 2026-10-25 (English only; scored once, 2 nominal hits explained by conviction, no discovery), registered before the realtime run. The explore-mode run of `-mode lib3 -libwin explore` was a smoke test on regenerated history; its output (two candidate hits) was not examined. Continues `docs/plans/20261024-algopack-library-v2.md` (additive transforms, 0 of 100) with a different question.

## Why this plan (the 80/20 map says: select where, not what)

Library v2 asked whether anything adds to the composite as a signal and found nothing. The map of the earlier waves says a different thing about the composite: its IC is concentrated (79% of the total IC in the half of the dates with a low RVI, 89% of the decile spread in the short leg, the highest IC in the thinnest liquidity tercile). If that concentration exists across stocks too, a rule "trade only the names where the composite works" would cut cost and noise without any new signal, and that rule is exactly what a retail product could use. So the question here is moderation, not addition: for which stock-level characteristics does the composite's daily IC differ between the top and the bottom third of the names?

## The 105 rows

The 100 candidates of library v2 (16 vital features times five transforms, 20 products and differences, same generator, same causal per-ticker construction) serve as moderators, plus five stock characteristics: `OwnVol20` (the ticker's 20-day return standard deviation), `LiquidityRank` (the position in the point-in-time top-100; the top third of the moderator = the thinnest names), `Ret1`, `Ret5`, `Ret20` (backward returns). Five within-date permutations of `LevAsym` are the placebo controls. The moderator is rank-scaled within the date over the names that have it (at least 30); the top third is the names at or above the 2/3 cut, the bottom third at or below the 1/3 cut.

## Statistic (frozen)

Realtime window, feature dates 2025-01-03..2026-10-16, halves split 2026-01-05, the out-of-fold blend predictions of the wave-3 primary parameters (identical to library v2). For each date the composite's rank IC against the 10-day abnormal rank is computed separately in the top third and in the bottom third (at least 15 names each); the statistic is the mean over dates of IC(top) minus IC(bottom), with a Newey-West (lag 10) t and the two-sided normal p.

- **Family**: 105 rows, M_eff by the Nyholt method from the outcome-free mean cross-sectional rank correlations of the 105 moderators; threshold p at most 0.05 / M_eff; Benjamini-Hochberg at 0.10 reported.
- **Hit**: p at most the threshold and the sign of the difference the same in both halves (two-sided: either direction is a finding; the direction is reported).
- **Calibration**: five placebo controls must give zero hits; one placebo hit voids every hit.
- **Block rule** (inherited): two or more hits among the five transforms of one feature count as one discovery, and any forward rule is written per feature.

## Consequence rules (written before the run)

A hit is a lead. If a moderator is a stock characteristic with a mechanism (`LiquidityRank`, `OwnVol20`), the lead becomes a frozen universe-restriction rule for the forward read; if it is a transformed feature, it is a description of where the composite works and needs the forward read before any restriction. Nothing enters the live path.

## Honest expectations (before the run)

`LiquidityRank` has a prior above zero: the wave-2 diagnostics showed the composite IC +0.059 / +0.008 / +0.072 in the liquid, middle and thin terciles in the high-RVI state and +0.109 / +0.122 / +0.161 in the low state (thin minus liquid +0.05 in the calm state, +0.01 in the volatile state); the tercile ICs are descriptive and were not tested. With a standard error of IC(top) minus IC(bottom) of the order 0.03 on 450 dates, a pooled difference of 0.03-0.05 is near the detection limit and the threshold (about 0.0005 in p) needs a t of about 3.5. The expected number of hits is zero or one (liquidity). The other 104 rows are the exploratory part and are expected to be null.

## Run order

1. Reviewer reads this file and `cmd/alphafit/lib2.go` (mode `lib3` shares the generator and the statistic scaffolding with `lib2`).
2. One run on the ai-box: `cmd/alphafit -mode lib3 -frozen work/w3-frozen-primary.json -matrix work/matrix.csv`; the `lib2` mode is re-run and must reproduce the library-v2 output byte for byte (done: identical).
3. Numbers into `docs/metrics.md` 8.26 and `docs/knowledge/model-quality-history.md`.

## Result (scored once, 2026-10-25)

Numbers in `docs/metrics.md` 8.26, output `artifacts/algopack-library-v3/lib3-verdict.txt`. M_eff 100.39, threshold p 0.00050. **2 nominal hits of 105, 0 of 5 placebos**: L083 (LevAsym x A2) and L092 (A2H2 x LevAsym), both the same moderator in two forms (agreement of book-depth asymmetry and order-flow imbalance): composite IC +0.149 in the top third against +0.060 in the bottom third, difference +0.088 (t +5.08) and +0.089 (t +5.33), halves +0.072/+0.110 and +0.077/+0.104. `LiquidityRank` (the prior-guided row): difference +0.028, t +1.30, p 0.19, not significant. Smallest other p: ImbVol - A2 0.0008 (t +3.34), TicketMed tercile rank 0.0021, SpLv x A2 0.0037, Idle lag 1 0.0042, Ret20 0.0043; 18 rows with p below 0.05 (5 expected), 29 below 0.10 (10 expected).

**Post-run diagnostic (added after the hits were seen, to test whether the moderation is real):** a Pearson IC computed inside a subgroup rises mechanically with the dispersion of the predictor in that subgroup, and a moderator built from the composite's own strongest inputs selects names where the prediction is extreme. Control moderator = the absolute value of the composite prediction itself: difference +0.142 (t +5.61), larger than either hit. Stratifying by terciles of the absolute prediction (the moderator effect within equal-conviction strata): L083 +0.0154 (t +0.81, halves +0.020/+0.008) and L092 +0.0156 (t +0.86, halves +0.025/+0.0004). **Registered reading: the two hits are explained by the composite's conviction, not a separate moderator; there is no discovery.** The excess of rows with small p (18 against 5) has the same explanation, because the moderators are transforms of the composite's own inputs. A statement about conviction itself (the IC is higher where the prediction is more extreme) is not a finding either: for a linear relation the correlation grows with the dispersion of the predictor; whether the payoff is nonlinear in conviction would need a slope comparison, not an IC comparison, and is not tested here.

## Review log

Pre-run pass (opencode, 2026-10-25): thirds, ties, the LiquidityRank sign convention, causality of `OwnVol20` and the lib2 regression are correct; one real layout bug found (errata 1) and one pin (errata 2).

## Errata

Written 2026-10-25 after the Reviewer's pass, before the realtime run.

1. Layout bug fixed: the first lib3 build wrote the five stock characteristics over the last five combination candidates (columns 95-99). The layout is now 100 candidates (columns 0-99), 5 characteristics (100-104, labelled X01-X05: OwnVol20, LiquidityRank, Ret1, Ret5, Ret20) and 5 placebos (105-109, labelled C01-C05, outside the family); the M_eff matrix covers the 105 moderators. The `lib2` mode was re-run after the fix and is byte-identical to the library-v2 output. The checks of the fixed layout printed the explore-mode (regenerated window) rows of L095-L100, X01-X05 and C01-C03; those values are not evidence and the realtime run is the first read of any realtime value.
2. The moderator values entering the thirds cut are the within-date ranks (every moderator is rank-scaled in the same pass, `LiquidityRank` re-ranked), so the cut positions refer to rank units.
