# Gap reversal: a feasibility bound for the one lead of the fund playbook (2026-10-27)

Status: DONE 2026-10-27 (run once; the gate fails at condition 1 for both signals; break-even 0.4-1.2 bp a side against 15 bp charged); registered before any run (English only); amended by the errata below after the Reviewer's fifteen conditions. Follows `20261027-algopack-fund-playbook.md`, whose single lead is the 1-2 day reversal family led by the opening gap (B05: incremental slope -0.0234, t -3.65 on 2025-01..2026-10; blocked from candidacy only by the stage-I floor, stage-I IC -0.0184). This plan asks one narrow question: **can a standalone, daily-turnover book on that signal pay its costs?** It is a feasibility bound, not a test of the incremental claim.

## Signals (frozen, no search)

- **GAP** = minus the rank of B05, the opening gap `OpenDay / previous CloseDay - 1` of the day, rank-scaled within date over the point-in-time top-100; dividend-drop days are excluded from the cross-section (NaN). Long names that gapped down, short names that gapped up.
- **BLEND** = minus the mean rank of B05, B04 (volume-weighted 1-day abnormal return) and B12 (day range times the sign of the day's abnormal return). A name with any one member missing is excluded. The three members are the strongest same-signed slopes of the playbook; they were chosen on that run, which includes the exploration window below (errata 5).
- Both are read from the playbook's feature file (`work/fund/fund-features.csv`, SHA-256 required to equal `299a7f9b73751d5c8ecbeaab1eba4d598351fb0a14bab65662a46323adab0c70`, otherwise the signal is not the lead's signal). The decision time is the close of day d; the gap was known at the open, so the signal is a full session old at entry (a positive result is a lower bound on a tradable open-entry form).

## Rows (frozen)

- **R0 horizon profile (descriptive):** mean daily rank IC of GAP and BLEND against the abnormal return over 1, 5 and 10 days on both windows, with halves (the matrix has no 2-day label).
- **R1 long-only against the universe (no short carry), R2 dollar-neutral long-short** with real short carry (17%, 25% and the linear 27.5%), partial-adjustment book, adjustment rate frozen on the exploration sample from {1.0, 0.5, 0.3} separately per book. **Primary books, pre-committed: GAP is read through R1, BLEND through R2.** The other book of each signal is reported and is descriptive.
- **Costs.** The gate uses the flat run (0.15% a side). The bbo run is reported beside it and is descriptive: SpBBO is the day's mean spread, while the trade happens at the close on the names that gapped, where the spread is widest, and in 2021-2024 it is an exchange-regenerated column, so the bbo run is optimistically biased.
- **Printed for every signal, rate and book:** gross mean per day, cost mean, turnover, **break-even cost per side = gross / turnover** next to the cost charged, the net with t and halves at **all three rates** (not only the chosen one), the r = 1.0 gross, and the number of thin days (fewer than 30 signal names or 40 aimed names: the aim is empty, the day's net is set to NaN and counted).
- **Descriptive variants, not part of the gate:** equal weights inside each leg (`-eqw`; inverse-volatility sizing gives the largest gaps the smallest weight), next-open entry (`-entry open -ext`, the tradable form when the gap is revealed), bbo costs.
- **Placebo.** Five within-date permutations of the signal through the same sleeve (same costs, same turnover) on the exploration window, and the corrected net (real minus the mean of the five) with its interval; the same on the realtime window as a mechanism check.

## Prior, derived before the run

Turnover is about 2r per leg per day (positions replaced at the rate r), so at r = 1.0 and 0.15% a side a long-only book pays about 30 bp a day and a long-short book about 60 bp (76% and 151% a year). At a daily IC of 0.018 and 2% cross-sectional daily dispersion the gross is about 5 bp a day long-only and about 10 bp long-short, so the break-even cost is about 2-3 bp a side against 15 bp charged. Lowering the rate cuts cost and alpha together, so the break-even moves by a factor of two at best, not the factor of five that is needed. A pass would need a daily IC of about 0.10, thirteen standard errors above the measured 0.018. **The prior of a pass is under 1%.** The value of the run is the printed break-even number: the family, if real, needs market-making costs, not a directional book.

## Gate (frozen)

**Pass = a lead for the forward log, never a result; the screen can fail the lead and cannot establish it.** All of: (1) the primary book's net on the exploration window under flat costs (S1 without carry for GAP, S2 at 25% carry for BLEND) is positive with Newey-West t of at least **2.64** (Bonferroni over 2 signals x 3 rates) and positive in both halves (split 2023-07-01); (2) the placebo-corrected exploration net is positive with the 95% interval above zero; (3) the realtime net of the same primary book has the same sign and a positive point estimate. Failing (1) closes the lead as a gross effect without a net.

## What a null does not test

Only the top-100 (the matrix is truncated; gap reversals are usually stronger in smaller names); only close entry, a full session after the gap (the open-entry row is descriptive); only a daily-rebalanced standalone book with 1/vol sizing (the incremental form of the lead stays open: its evidence was a slope over the composite plus momentum, not a standalone IC); not market making. A null reads "not as a standalone directional book at these costs", not "not at all".

## Commands (exact, ai-box, `cd ~/algopack-study`, binary `bin/alphafit-rev`)

- Profile: `bin/alphafit-rev -mode momsleeve -stage profile -sig gap|blend -matrix work/matrix.csv -candfile work/fund/fund-features.csv -frozen work/w3-frozen-h10.json -cost flat`
- Exploration: the same with `-stage explore -base artifacts/algopack-wave3b/w3b-base.csv`; the variants add `-eqw`, `-cost bbo`, or `-entry open -ext work/matrix.csv.ext`.
- Verdict (after the rates are read from the exploration output): `-stage verdict -ratel R1 -rates R2 -base artifacts/algopack-wave3b/w3b-base.csv` with the frozen rates; the stage stops unless both rates are in {1.0, 0.5, 0.3}.

## Honest expectations

A null closes the standalone reversal book at the daily horizon in the top-100 and is reported as a null with the break-even table. The sample is one market and one author.

## Review log

Reviewer pass (opencode, 2026-10-27): fifteen conditions, timing confirmed clean (f1 is the return from the close of d; the gap is a full session old at entry); all adopted below.

## Errata

1. The gap is excluded from the cross-section on dividend-drop days (NaN), not zero; the "zero" of the playbook describes its own convention.
2. Gate (2) is computed on the exploration window (permutations added to the explore stage); the placebo is a random ranking through the same sleeve, not the equal-weight universe.
3. The verdict stage stops unless the rates are in {1.0, 0.5, 0.3} and a base file is given (`artifacts/algopack-wave3b/w3b-base.csv`, 2025-01-03..2026-10-16).
4. Family and bar: 2 signals x 3 rates with one primary book per signal, Bonferroni 2.64; the other books are descriptive.
5. The exploration window is the playbook's stage-I window, on which B05 printed IC -0.0184 (t -3.12) and the members were chosen; sign, family and member list come from data that includes it. Both windows have been looked at: this is a consistency screen on data used for selection. The only clean evidence is forward data from 2026-10-16.
6. Gate on flat costs; bbo descriptive. 7. Break-even printed. 8. Incremental form out of scope (above). 9. Universe limit stated. 10. Open-entry row descriptive. 11. All rates printed. 12. Equal-weight variant. 13. Thin days counted and set to NaN. 14. Exploration ends on the last trading date of 2024 (its f1 is the return into 2025-01-03, so no return is used twice). 15. SHA-256 of `momsleeve.go`, `revprofile.go`, `eval.go` and the candidate file are recorded below before the first run; tests cover the sign, the BLEND exclusion of a name with one missing member, the permutation (marginal kept, cross-section destroyed) and the rate guard.

Frozen before the first run: `momsleeve.go` SHA-256 ad0b25868b8e60ceebfb4d9116deeaeafc142a771f91f1910f9c6eb767801246; `revprofile.go` 9eb8c896a19bbaccdae5b5e1d97998ba6015fe4423d824d38dbffb0fef07bba3; `eval.go` 661663eb70c7f0d8ee8d8e06f4a6f717b08b1b74472e33eec56b893e9925da0d; candidate file `fund-features.csv` 299a7f9b73751d5c8ecbeaab1eba4d598351fb0a14bab65662a46323adab0c70 (equal to the playbook's); base sleeve `w3b-base.csv` ca67248356be291a122b46106aac71b36e275226ba1b31abec40d65150c0934b (on the ai-box at `work/w3b-base.csv`, in the repo at `artifacts/algopack-wave3b/`). Any change after the first output is a new plan.

## Result (scored once, 2026-10-27)

Numbers in `docs/metrics.md` 8.42, outputs `artifacts/algopack-gap-reversal/`. GAP's daily rank IC is real and short-lived (1 day +0.0288, t +6.26; 10 days +0.0174, t +2.78), but gross is about 0.5-2 bp a day against 15 bp charged on 0.5-3 turns: **break-even cost per side 0.4-1.2 bp** (4.5 bp at best with equal weights or open entry). All exploration nets are negative at every rate (t -6 to -20), the placebo-corrected net is indistinguishable from zero (GAP) or negative (BLEND). Gate condition 1 fails for both primary books; the lead is closed as a standalone directional book. The incremental form stays open, untested here.
