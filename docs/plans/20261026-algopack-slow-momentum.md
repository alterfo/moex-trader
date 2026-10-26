# Low-turnover momentum books (2026-10-26)

Status: REGISTERED 2026-10-26 before any run (English only). Follows `20261026-algopack-momentum-confirmation.md`: momentum is a confirmed ranking signal (2021-2024 prices, mom256 IC +0.038, NW t +2.56) that adds +0.0105 IC to the composite, but the composite book at rate 0.5 earned no net from it (the permuted-momentum placebo moved identically). The owner asked for a low-turnover form. Designs and priors come from a Reviewer brainstorm (opencode, 2026-10-26); seven were proposed, the first, second, third and fifth are registered here, the others (vol-scaled momentum, no-trade band, sector-neutral momentum) are a diagnostic family left for later.

## The structural advantage of this study

A momentum sleeve is price-only, so it can be evaluated on an **independent sample, from the first date with at least 40 names holding 256 observations (about 2021-05) to 2024-12-31** (see erratum 9; prices and returns are not regenerated, spreads for bbo costs are the AlgoPack history of those years and are an approximation). Every free parameter is frozen on that sample (halves of the exploration sample split at 2023-07-01); the spent realtime window (2025-01-03..2026-10-16) is scored once.

## Books

Signals (rank within date, top 100 names, missing when fewer than 256 observations): `mom256` = sum of the last 256 daily abnormal returns (abnormal = `rb1` minus the same-day cross-sectional mean); `mom121` = the same sum over observations 21..256 (skips the last 20, the 12-1 form), used only if confirmed (below).

Each book uses the existing machinery (`aimWeights`: top and bottom quintile, inverse-volatility legs; partial-adjustment book with rate r; bbo cost model; carry 17% / 25% and the 27.5% extrapolation).

- **S1 long-only slow:** top quintile of the signal, no short leg, measured against the equal-weight universe one-day return (as `-book longonly`), no borrow. Rate r.
- **S2 long-short slow, blended:** the long-short quintile book at rate r with carry on the short notional, held as a second sleeve next to the wave-3b composite sleeve (the fast book of record); blend = 50% composite sleeve, 50% momentum sleeve, fixed in advance (not tuned). Net of the blend per day = 0.5 × composite net + 0.5 × momentum net; costs and carry are the sum of the sleeves' own (netting of opposite positions between sleeves is ignored, which is conservative).
- **S3 momentum inside the composite at a low rate:** the step-2 chain (`-mom on`) with the book rate 0.10 and with 0.05, instead of 0.5.

## Frozen parameters (exploration sample 2022-04..2024-12)

- **Rate r for S1 and S2** chosen from {0.10, 0.05, 0.03} by the highest mean net per day on the exploration sample (S1: net against the universe; S2: net at 25% carry). Ties go to the lower rate (the code compares with >= over the rates 0.10, 0.05, 0.03).
- **S3** rates are fixed at 0.10 and 0.05 (two arms), hyperparameters of the model from `work/w6-frozen.json` except the rate.
- **12-1:** run `momdiag` pooled on 2021-2024 for mom121; the 12-1 arm exists only if its NW t is at least 2.0 with both sub-periods positive (the registered rule of the confirmation plan), otherwise it is dropped and reported.

## Verdicts (one run each; realtime window 2025-01-03..2026-10-16, halves split 2026-01-05)

Placebos in every arm: the signal permuted within date, five seeds (20261026..30), same book, same rate.

- **S1 gate:** mean net per day (against the universe, bbo costs, no carry) with the date-block bootstrap 95% interval (block 10, 5000 draws) above 0 in the realtime window, positive in both halves, **and** positive with Newey-West t at least 2.0 on the exploration sample; the five placebo books' mean net not above 0. Also reported: net per year against the key-rate hurdle on deployed capital (the long book ties up 100% of capital; the excess over the universe is not compared with the key rate, the absolute return of the book is).
- **S2 gate:** the blend net at 25% carry above the composite sleeve alone with the bootstrap interval of the daily difference above 0, positive in both halves; the blend of the permuted-signal sleeve minus the composite sleeve not above 0 (placebo-corrected: the statistic that counts is the treatment difference minus the mean placebo difference, interval excluding zero); the momentum sleeve alone positive at 25% carry on the exploration sample.
- **S3 gate:** the placebo-corrected difference (extended minus baseline) minus (placebo minus baseline) at 25% carry with the interval excluding zero, and the extended book's own net at 25% above 0 with the interval above 0.

A pass of any gate is a **lead for the forward log**, never a result: the window is spent and the signal was chosen after looking at it. Family size: up to 3 designs × 2 signals, plus the S3 rates: nine verdicts. The nominal level is read against nine; the exploration-sample agreement and the placebos are the real filters.

## Honest expectations

Reviewer priors: S1 and S2 30-40% for a positive net, S3 moderate. On the exploration sample the same rule gives the honest picture: the premium there is about +0.04 IC against +0.107 in the realtime window, so a realtime pass with an exploration miss is read as a regime result. S1 is a fund-style product (a slow long book against the universe) and carries no borrow; its weakness is that 2025-2026 was the strongest momentum regime in six years.

## Review log

Red-team pass (opencode, 2026-10-26) against the code: the definitions hang together; 21 items, two defects (tie-break direction, the false statement about the first date with 256 observations) fixed, the rest pinned in the errata below.

## Errata

1. 12-1 confirmation run before any book (price only, pooled 2021-2024): mom121 IC +0.0258, NW t +1.83 (2021-22 +0.0417, 2023-24 +0.0131): below the registered 2.0, so **the 12-1 arm is dropped**; the family is S1, S2 and the two S3 rates (four verdicts), all on mom256.
2. Implementation: `cmd/alphafit -mode momsleeve` (exploration and realtime in one run; the rate for S1 and S2 is the best of {0.10, 0.05, 0.03} on the exploration sample); the S3 arms use the existing chain (`-mom on` and `-mom placebo` with a frozen JSON at rate 0.10 and 0.05).
3. **Definitions (items 1-3, 5, 6).** S1 is measured against the raw equal-weight universe one-day return `u1` (not the abnormal return), the fund-product definition; a missing `u1` gives a zero benchmark (inherited). The quintile is over the names with a non-NaN signal (about 90 of 100); the legs are inverse-volatility (`sig`, the 20-observation standard deviation of `rb1`); the aim is empty under 40 names. A name absent from a day's rows is charged its decay turnover at the flat 0.0015 (inherited). The 256 observations are the last 256 appearances of the name in the top-100 matrix (days with `rb1` missing count as zero abnormal), which can span more than 12 calendar months for names that enter and leave; this is identical to the `-mom` features. `mom121` indices: observations p-255..p-20 (236 observations, skipping the 20 most recent including the same day); the arm is dropped (erratum 1).
4. **Costs on the exploration sample.** The regenerated spreads make the bbo net there an approximation. The `explore` stage prints the share of exploration rows with missing or non-positive `SpBBO`; **rule: if it exceeds 30%, the rates are frozen with `-cost flat` and the realtime verdict still uses bbo.** The freeze is hyperparameter selection, not evidence; the fully independent quantities on the exploration sample are the signal and the gross.
5. **Tie-break and the exploration condition (items 8, 10).** Ties go to the lower rate, implemented with >= in the descending order 0.10, 0.05, 0.03. The exploration sign condition of every gate is stricter than selected-best: the exploration net must be **positive at all three rates**, and the chosen rate must have NW t at least 2.0 (S1 on the net against the universe, S2 on the sleeve net at 25% carry); otherwise the condition is reported as failed.
6. **Seeds and bootstrap (items 12, 19).** One bootstrap seed, 20261026 (block 10, 5000 draws), everywhere in this plan. The sleeve placebos use seeds 20261027..31, distinct from the `-mom placebo` seed 20261026 of step 2. The S3 placebo is the `-mom placebo` run (one fixed within-date permutation of the two momentum features, seed 20261026); it is a single draw, not five, stated as a weakness.
7. **S2 blend (items 13-15).** The blend is 0.5 × composite sleeve + 0.5 × momentum sleeve; the gate statistic, the daily blend minus composite alone, equals 0.5 × (momentum sleeve net - composite sleeve net) so the 50/50 weight scales the level and is not a tuned parameter. The composite sleeve is the wave-3b baseline WITHOUT momentum: `artifacts/algopack-wave3b/w3b-base.csv` (SHA-256 `ca67248356be291a122b46106aac71b36e275226ba1b31abec40d65150c0934b`), columns `net_gp_blend` (17%) and `net_gp25_blend` (25%), made with `w3b-frozen.json` (ridge 0.003, elastic-net 0.0002, rate 0.5, bbo, close entry). The blend sums the sleeves' nets with no credit for netting; each sleeve pays costs on its own turnover (conservative).
8. **S3 (items 16-18).** Frozen files in `artifacts/algopack-momentum/`: `w6-frozen-r0.10.json` and `w6-frozen-r0.05.json` (ridge 0.003, elastic-net 0.0005 from the step-2 explore, alpha 0.5, rate 0.10 / 0.05; SHA-256 prefixes b0d387f37f43dc9b and 12346913214434d8). Because the elastic-net lambda differs from wave 3b, each rate needs three new dumps with the same JSON: no momentum (baseline), `-mom on`, `-mom placebo` (flags `-matrix work/matrix.csv -ext work/matrix.csv.ext -horizon 10 -cost bbo`). The statistic is computed by `alphafit -mode s3diff -base BASE -extdump EXT -plcdump PLC`: per date (extended - baseline) - (placebo - baseline) at net_gp25_blend and net_gp_blend, the block-10 bootstrap interval and the halves split at 2026-01-05, plus the extended book's own net. Gate as above.
9. **Count (item 21).** Realtime verdicts: S1, S2, S3 at 0.10, S3 at 0.05 = 4 (the 12-1 arm is dropped, its confirmation run was a price-only step). The nominal level is read against 4; no formal correction, but a lead needs the exploration conditions of erratum 5 as well.
10. **Wiring (item 20).** `alphafit -mode momsleeve -stage explore` (freezes the rates, no realtime numbers) then `-stage verdict -ratel R -rates R -base artifacts/algopack-wave3b/w3b-base.csv`; unit test `TestSumRangeAndOLS` covers the 256 and 121 sum indices.
