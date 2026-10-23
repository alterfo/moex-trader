# AlgoPack wave 3b: five ideas from the local models, one chain (2026-10-24)

Status: DONE 2026-10-24 (English only; scored once, no improvement), registered before any wave-3b number. Design from the triage by the Reviewer (opencode, read-only) of the brainstorm of three local models (T-pro, Cydonia 24B, Mistral-Small-3.2 abliterated; outputs in `artifacts/algopack-brainstorm/`).

## Where the ideas come from, and what the triage kept

The owner asked for the local models to brainstorm. 75 items came back; the triage sorted them into usable, duplicate and nonsense (the log is in the Review log below). About a third were duplicates of tested rows (cancel asymmetry, Kyle, ex-date week, basis, hi2), a quarter were nonsense (a 00:00-07:00 session and a midnight auction do not exist, `avg_time_to_fill` does not exist, a fractal dimension of 450 daily points), several were forbidden intraday scalping or post-hoc refinements of the seen RVI lead, and Mistral looped after 12 items. Five usable ideas were picked, plus a shelf of deferred ones behind data conditions. The honest yield of three models is five testable ideas, not seventy-five.

## The five ideas and how each is built (deviations from the model wording are stated)

All are features for the existing composite (`cmd/alphafit`, rank-scaled within date, missing = 0); the target, walk-forward, embargo and the cost model are those of wave 3 (`docs/plans/20261024-algopack-wave3-costs-horizon.md`).

1. **Ex-dividend flow asymmetry (`DivFlow`)**, Cydonia 9. For a name with a dividend whose declared date is not after the feature date and whose last-buy date is 1 to 5 trading days ahead, the mean of the unexpected A2 (z-score against its own previous 20 days) of the last three feature dates; missing otherwise. Ex ante: the calendar is declared before the event.
2. **A2-correlation PageRank (`A2PR`)**, Cydonia 2. For each date the 20-day windows of A2 of the names with at least 15 valid days, pairwise correlation (at least 12 common days), weights the absolute correlation, PageRank with damping 0.85 and 40 iterations, scaled by the number of names; at least 40 names. Uses data through the feature date only.
3. **Intraday profile shape (`ProfP2`, `ProfP3`)**, Cydonia 6. Deviation: instead of principal components fitted on the data, fixed Legendre polynomial coefficients (orders 2 and 3) of the cumulative signed value imbalance profile over the bars up to 18:30 on a time axis scaled to [-1, 1]. The first-order coefficient is the existing `ImbSlope`. A fixed basis cannot leak and needs no fitting.
4. **Auction imbalance to the next open (`AucA1`)**, Cydonia 13. The closing-auction imbalance (18:40-18:50) was excluded as a feature because it is not implementable at the close; it is implementable for an order sent at the next open. It enters only the open-entry arm (`-aucinc -entry open`), never the close-entry composite.
5. **Compression/entropy channel (`VolEnt`, `DepEnt`)**, T-pro 1 and Cydonia 7. Deviation: the book levels are not in the data (the obstats columns are aggregates), so the entropy of the depth distribution over price levels cannot be built. Instead: the normalised Shannon entropy of the 5-minute traded-value shares over the bars up to 18:30 (`VolEnt`), and the normalised Shannon entropy of the 5-minute book-depth shares (bid plus ask volume) over the same bars (`DepEnt`). At least 20 bars.

## Chain (one verdict, no grid)

1. **Build**: rebuild the matrix with the four new base columns (`ProfP2`, `ProfP3`, `VolEnt`, `DepEnt`); the old 75 feature columns and the first 14 columns must be byte-identical to the verdict matrix (hard check, else stop). `DivFlow` and `A2PR` are derived in `cmd/alphafit` (`-div data/dividends.jsonl`). The extended set has 81 features plus the three return-rank controls.
2. **Exploration** (regenerated window 2022-07..2024-12, the only selection surface): `explore` re-fits the ridge and elastic-net penalties and the trade rate on the extended set (bbo costs, close entry); the wave-3 frozen parameters belong to the 75-column set. The AucA1 arm is explored separately with open entry.
3. **Realtime verdict, run once** (feature dates 2025-01-03..2026-10-16, halves 2026-01-05, five placebo seeds, date-block bootstrap, 5000 draws): the extended composite against the wave-3 baseline (the 75-feature composite with the wave-3 primary parameters, regenerated with the same dump code). Statistics: (a) **signal**: mean daily IC of the extended composite minus the baseline positive with the bootstrap 95% interval above 0 and both halves positive, and the extended composite above the placebo in at least four of five seeds with the difference interval above 0; (b) **economics**: the extended partial-adjustment book at bbo costs and 17% borrow with the same gate as wave 3 (interval above 0, both halves positive, placebo books not above 0); (c) the net difference extended minus baseline, reported. The AucA1 arm is one more row: its open-entry book net minus the wave-3 open-entry baseline (`+0.031%` per day), interval above 0.
4. This is the fourth look at the realtime window; a pass of (a) with (b) failing is an information finding, a pass of both is a lead with a frozen forward rule; nothing is confirmed on a spent window.

Not tested individually: the six new columns enter as a block (testing them one at a time would be a grid of six). The coefficients of the extended ridge on the realtime fit are printed for description only.

## Deferred shelf (behind data or token conditions, no rows now)

`vwap_b` minus `vwap_s` asymmetry, a discretised mutual-information column, per-ticker GARCH volatility z, the CNY-IMOEX rolling-correlation breakdown, the IMOEX rebalance-day flow, the FUTOI retail-versus-institutional row (usable only if the paid `apim` key lifts the 14-day delay of the ISS futoi tier; one curl with the key decides, outcome-free), OFZ-based items (no OFZ flow data), sector-flow spillover (sector indices only as prices).

## Power and expectations (before the run)

Six added columns on a 78-column ridge with an IC of 0.096: the composite already contains the A2 family, the profile columns (`ImbSlope`, `ProfTilt`) and 75 others. The previous baseline-versus-composite IC difference needed +0.037 to be detected; an increment from six columns is likely smaller than the interval half-width of about 0.03, so the expected result is a null. The interest of the chain is that it settles whether the three models' best ideas add anything, and the null is a result.

## Run order

1. Reviewer reads this file and the code (`cmd/alphafit/netfeat.go`, `cmd/flowscan/base.go` profile and entropy columns, `compare` mode).
2. Rebuild the matrix on the ai-box, check the old columns, run explorations, dump the baseline and the extended arm, run the verdict and `compare` once.
3. Numbers into `docs/metrics.md` 8.24 and `docs/knowledge/model-quality-history.md`.

## Result (scored once, 2026-10-24)

Numbers in `docs/metrics.md` 8.24, outputs `artifacts/algopack-wave3b/`. Matrix rebuild: the 102 old columns byte-identical to the verdict matrix (sha256 prefix `c0580fbeace81ad5`). Exploration chose the same penalties as the baseline (ridge 0.003, elastic net 0.0002) and trade rate 0.5. **(a) Signal: not met.** Extended composite IC +0.0973 against the baseline +0.0957; difference +0.0016, bootstrap 95% [-0.0020, +0.0057], halves +0.0021 / +0.0011 (the interval includes 0); the extended composite beats the five placebos (5 of 5, differences +0.093..+0.100). **(b) Economics: a knife-edge pass.** Extended partial-adjustment book at bbo costs and 17% borrow: +0.0817% per day, bootstrap 95% [+0.006%, +0.166%], halves +0.074% / +0.091%, placebo books -0.36..-0.40%; +0.0511% per day at 25% borrow. The lower bound is +0.006% against the baseline's -0.005%, but the net difference extended minus baseline is +0.007% [-0.011%, +0.025%], so the crossing is not attributable to the new columns. **(c) AucA1 arm (open entry):** IC difference -0.0001 [-0.0003, +0.0001]; the exploration chose trade rate 1.0 (baseline open entry 0.5, so the net comparison is confounded by the rate): partial-adjustment net -0.003% against +0.031% per day, difference -0.034% [-0.065%, -0.003%]. Registered reading: the six added columns and AucA1 do not improve the composite (condition (a) fails, the IC increment is +0.0016 of a composite that already has 0.096); the marginal economics pass is the baseline's noise crossing zero, not an effect of the ideas. The brainstorm's best five ideas add nothing measurable.

## Review log

Triage (opencode, 2026-10-24), summary: T-pro: usable 1 (entropy), 6 (half, rebalance calendar), 10 (PC form), 15 (vwap asymmetry); the rest duplicates of PlaceAgg/CanDiff/CanOrdRatio, ex-date indicator, H016, VwapDev/CLV, RetLast/RetEarly, the E01-E03 basis rows, or nonsense (invented night session and midnight auction, `avg_time_to_fill`, a Black Monday superstition) or forbidden scalping (pre-news VPIN, dead-time volatility, five-mention headline pop). Cydonia: usable 2, 3, 6, 7, 9, 13, 15, 20, 22, borderline 10, conditional 12; duplicates 4, 5, 11, 14, 16, 17, 18, 19, 21, 23, 25; unusable now 1 (persistent homology), 8, 24. Mistral: 12 unique items, the rest a verbatim loop; all duplicates, post-hoc RVI refinements, scalping or nonsense. Picks and the depth-entropy and PC-score deviations above are the Reviewer's chain with the data corrections stated here.

## Errata

(none)
