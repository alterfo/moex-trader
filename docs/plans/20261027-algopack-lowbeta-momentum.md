# Momentum inside low-beta names: replication and a sleeve (2026-10-27)

Status: REGISTERED 2026-10-27 before any run (English only). Follows `20261027-algopack-owner-100-ind-batch.md`, whose one lead is that 256-observation momentum lives in low-beta names (IC +0.108 in the lowest beta tercile against +0.025 in the highest on 2021-04..2024-12, difference -0.082, level 0.0083, both halves, placebo clean; beta-neutral momentum has the same IC as raw momentum). The finding was made on 2021-2024, so the realtime window 2025-01-03..2026-10-16 has not been used to evaluate the *conditioning on beta* and is the replication sample. Momentum itself was seen in that window earlier (`20261026-algopack-momentum-confirmation.md`), so only the beta split and the sleeve built on it are new there.

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

None yet.
