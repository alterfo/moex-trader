# Momentum inside low-beta names: replication and a sleeve (2026-10-27)

Status: DONE 2026-10-27 (Step 1 not replicated; Step 2 not run for a verdict). Registered 2026-10-27 before any run (English only). Follows `20261027-algopack-owner-100-ind-batch.md`, whose one lead is that 256-observation momentum lives in low-beta names (IC +0.108 in the lowest beta tercile against +0.025 in the highest on 2021-04..2024-12, difference -0.082, level 0.0083, both halves, placebo clean; beta-neutral momentum has the same IC as raw momentum). The finding was made on 2021-2024. On the realtime window 2025-01-03..2026-10-16 the interaction of momentum and beta has not been evaluated, but the constituents have: M256's own IC there (8.43; `20261026-algopack-momentum-confirmation.md`) and the beta column C01 through the playbook (one of its 94 columns, inside the K02 and K05 blends). Only the interaction and the sleeve are new on that window; it is the replication sample, not an untouched one.

## Step 1: replication of the conditioning (one test)

Signal `M256`, outcome the 10-day abnormal rank, the lowest versus the highest beta tercile (C01, 250-day beta to the equal-weight universe, within date), mean daily rank IC of each, on 2025-01-03..2026-10-16 (the last ten dates lose their labels). Difference of the two IC means with a 95% two-sided block bootstrap (block 20 days), halves split 2026-01-05, a within-date permutation of the moderator, five seeds. **Replicated** if the difference is negative (low-beta stronger), the interval excludes zero and both halves are negative. Otherwise the lead is closed as a feature of 2021-2024 and Step 2 is not run for a verdict.

## Step 2: the sleeve (only if Step 1 replicates)

Signal: `M256` inside the group of the **lowest half** of beta on the date (breadth: a tercile holds about 25 names, a quintile leg of five; the half holds about 38; the tercile is run as a descriptive variant with a lower minimum). Outside the group the signal is missing. Books of the `momsleeve` machinery, slow rates {0.10, 0.05, 0.03}, **frozen on the exploration sample 2021-04-08..2024-12-30** by the highest net, ties to the lower rate:
- **S1 long-only against the group mean** (the equal-weight mean return of the same low-beta group, so the book earns momentum selection and not the low-beta premium or the market), flat 0.15% a side costs, no carry. Primary.
- **S2 long-short inside the group** at the carry of 25% (and 17%, 27.5%), descriptive.
- Placebo: five random rankings inside the same group through the same sleeve; the corrected net is reported.
- Descriptive: the low-beta group's own return against the universe (the low-beta premium), bbo costs, the tercile variant.

**Gate (realtime, one run):** Step 1 replicated, and the S1 net on 2025-01-03..2026-10-16 at the frozen rate is positive with Newey-West t of at least 2.0 (one primary book, one frozen rate) and positive in both halves (split 2026-01-05), and the placebo-corrected net is positive with its 95% interval above zero. Pass = a lead for the forward log, never a result: momentum was previously seen in this window.

## Prior and the cost arithmetic

A momentum book at a rate of 0.10 turns over about 0.04 a day, so 0.15% a side costs about 0.6 bp a day; the earlier slow-momentum long-only book earned +0.083% a day against the universe in this window (t +2.40) and failed only its exploration condition. If the beta conditioning is real, selecting within low-beta names should not lower the gross. The prior of Step 1 replicating is about 35% (a single test, but a conditioning found as one of six and of small size), of the S1 gate passing given Step 1 about 40%; overall about 15%.

## What a pass or a null does not tell

Top-100 only; the group is defined by a beta measured on 250 days of the universe; the low-beta premium itself is not the question (the benchmark removes it). A null reads "not as a within-low-beta momentum book at these costs on 2025-2026".

## Review log

Pre-run Reviewer pass requested 2026-10-27; conditions are recorded as errata before the run.

## Errata

Written after the Reviewer's sixteen conditions, before any run.

1. **Independence** softened above (the interaction is new on 2025-2026; the constituents are not).
2. **Power, stated.** The finding's interval was [-0.177, -0.003] with the bound nearest zero at -0.003 and half 1 at -0.039. The replication has about 430 testable dates against about 930, so the standard error of the difference is roughly 0.06-0.09 and excluding zero needs an observed difference of about -0.12 or larger; power at the observed -0.082 is about one half. The prior of Step 1 replicating is therefore about 35-45%, and the overall prior about 15%.
3. **Step 1 is a single test at the two-sided 95% level** (not 0.0083, which was the batch's 0.05/6). It is the only inferential test on 2025-2026 in this plan; everything else there is descriptive and is labelled so in the output: the tercile variant, bbo costs, equal weights, S2 and carries, the low-beta premium line.
4. **The five moderator permutations are support**, not tests: five clean permutations mean the relation is cross-sectional; they are not added to the family.
5. **R1 is bit-identical to test 63c of the batch** (kind tercile, signal M256, moderator C01, horizon 10). SHA-256 of `hetero.go` and `momsleeve.go` are recorded below before the run; the batch's earlier hash of `hetero.go` no longer matches because the `lbrep` study was added.
6. **Effective end.** The last ten sessions lose their 10-day labels, so the testable replication window is 2025-01-03..about 2026-10-02. The moderator is the 250-day beta through the current close (not the previous close); M256 is also through the current close; both are known at the close of d, the decision time.
7. **Verdict machinery** for the low-beta kinds no longer reads the wave-3b dump or prints composite blends; it stops without `-candfile`, requires the candidate file's SHA-256 to equal the playbook's (299a7f9b…), and the placebo-corrected S1 net with its interval, the cost arithmetic (turnover, cost, break-even per side) at the frozen rate, the observed range of names available per date, the thin-day count and the universe mean next to the group mean are printed. The gate t is computed on non-thin days only (a thin day has fewer than 48 available names for the half).
8. **Scope of a pass or a null.** The gate group is the half; the finding's group is the tercile (run as descriptive, legs of about five names); the finding is an equal-weight rank IC, the book weights by inverse volatility (equal weights run as descriptive; the gate keeps inverse volatility because that is the book's construction); the finding is a 10-day IC, the book earns 1-day returns. A null therefore bounds none of: the tercile group, the equal-weight book, the 10-day IC claim.
9. **Family.** One primary book at one frozen rate: Newey-West t of at least 2.0 is a single test at the 0.05 level; S2, carries, tercile, bbo and equal weights stay descriptive or the bar grows.
10. **Boundary.** The last exploration date is 2024-12-30 (the matrix has no 2024-12-31 session); its f1 earns the return into 2025-01-03, so no return is used twice.

## Commands (exact, ai-box, `cd ~/algopack-study`, binary `bin/alphafit-lb`, features `work/fund/fund-features.csv`)

- Step 1: `bin/alphafit-lb -mode hetero -hstudy lbrep -hk 1 -matrix work/matrix.csv -frozen work/w3-frozen-h10.json -candfile work/fund/fund-features.csv`
- Step 2 exploration (freezes the rate): `... -mode momsleeve -stage explore -sig lbmom -cost flat -matrix work/matrix.csv -frozen work/w3-frozen-h10.json -candfile work/fund/fund-features.csv`
- Step 2 verdict: the same with `-stage verdict -ratel R -rates R` (R from the exploration output, in {0.10, 0.05, 0.03}); descriptive variants add `-sig lbmom3`, `-eqw`, `-cost bbo`.

Frozen before the first run: `hetero.go` SHA-256 d8c7bfd725b29bf2421bb4b182ac0a369dd0418aec92bb5343d20734b1be1e77; `momsleeve.go` 456a1bbc1e1eeeab2879982c766b87dca18120327125fa1f6413cc442d4d9529; `guard.go` 655edbc6cdbbaa42d078ad197824d06ce85a3c5d4b22db034af2bd415bc7bbab; `eval.go` 03851951debb674fd6e6bed3c6d26c375a5a5be1df48f08c95ebb502e604ec5e; feature file 299a7f9b73751d5c8ecbeaab1eba4d598351fb0a14bab65662a46323adab0c70. Any change after the first output is a new plan.

## Result (scored once, 2026-10-27)

Numbers in `docs/metrics.md` 8.45, outputs `artifacts/algopack-lowbeta/`. Step 1: difference -0.0355 [-0.112, +0.026], halves -0.075 / +0.016, one placebo of five excluded zero: not replicated. Step 2 was not run for a verdict by the registered rule; the exploration stage was run as description only (net +0.05% a day at turnover 0.02-0.03 in 2021-2024, where the lead was found).
