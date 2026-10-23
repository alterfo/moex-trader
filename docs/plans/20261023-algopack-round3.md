# AlgoPack round 3: the economic gate on the top-100 universe (2026-10-23)

Status: REGISTERED 2026-10-23 (English only), design agreed with the Reviewer (opencode, read-only) in the pre-registration exchange; the Reviewer read the plan and the code before the first score (log below). Roles as in rounds 1 and 2 (`docs/plans/20261023-algopack-forecast-improvement.md`, `docs/plans/20261023-algopack-round2.md`).

## Why a third and last plan

Rounds 1 and 2 left one door: the abnormal-label cross-sectional content of the ten-column block, with corrected ΔAUC of +0.0186 (realtime-only) and +0.0196 (2020-plus) on 18 tickers and a positive ΔIC A-S in all four comparisons (t +1.7..+2.2), none of which passed the registered conjunction. Its problem is power (5000-7000 test rows). This plan asks the economic question on the full top-100 universe, where the rows are about six times as many. The A2 effect is already known to be statistically strong (`docs/metrics.md` 8.15, t +19), so any AUC above 0.5 on this panel is expected; therefore the AUC is context and the verdict is an economic gate.

## Stop rule (registered now)

A fail of the decile gate below closes the cross-sectional directional door for the registered definitions: no further plan on the same ten columns. A pass is a lead that waits for the forward window (data accruing from 2026-10-21), not a result; nothing enters the live path.

## Design

- **Universe and rows**: the point-in-time top-100 by 60-day median value of each date (flowscan `adm100`), realtime window feature dates 2025-01-03..2026-10-16, names with a full ten-column block that day, at least 30 such names per date. The realtime-only primary rule of round 1 (the KS trigger fired on T18 pooled) is extended to the top-100 universe by the same conservative rule, since a separate KS on the top-100 was not computed.
- **Features**: the ten block columns of round 1 (definitions unchanged), each demeaned by the date mean over the admitted names present that date (computed from that date's own rows, leak-free under date-blocked folds). No FEAT18 columns: the model sees the block only.
- **Label**: the 10-day simple close-to-close return minus the equal-weight mean 10-day return of the admitted names present that date, deadband 0.5% (label 1 above +0.5%, 0 below -0.5%, otherwise the row is not a training row), returns from the `tradestats` closes, moves below -45% or above +80% dropped, the tenth row ahead must be exactly ten trading days ahead on the calendar of the block file, rows whose end date is after 2026-10-21 dropped. The abnormal return itself (not only its sign) is kept for the gate and for deadband rows.
- **Model**: the deployed recipe (lgbm + xgb + logistic, `scripts/export_ensemble.py`, `ENSEMBLE_SEED`), five seeds 20261023..20261027, seven blocks of round 1, `label_date` embargo 10 calendar days; the primary arm trains on realtime rows only (blocks 3-7, 2025-07-01 on); the 2020-plus arm (all seven blocks) is descriptive.
- **Arms**: A (ten demeaned columns), S-dm (the same vector permuted within date after demeaning, seed 20261022, the negative control), and descriptively A2-dm (the single demeaned `ap_a2` column through the same recipe, to ask whether the ten-column block beats the already known A2).

## The verdict: the decile gate (frozen form)

On each test date with at least 30 names, rank the names by the seed-mean predicted probability of the arm; top decile long, bottom decile short (registered sign: long the top); the per-date gate value is the mean abnormal 10-day return of the top decile minus the mean of the bottom decile minus 0.30% (the round trip of the two legs, the hurdle of block B of plan 20261022). The date series is judged by:

- the date-block bootstrap (block 10 dates, 5000 draws, seed 20261022) 95% interval of its mean above 0, with the Newey-West t (lags 10) reported next to it;
- the mean positive in both halves (split 2026-01-05) on the primary arm;
- the sign of the mean agrees in at least four of five seeds when each seed's own predictions are used for the deciles (protocol item 7 of round 1);
- the negative control: the S-dm arm's own gate value has a point estimate at most 0 or an interval containing 0.

Pass only if every item holds on the primary (realtime-only) arm. The 2020-plus arm is reported next to it and cannot rescue the primary.

## Reported, not conditions

- AUC of A, S-dm and A2-dm on the labelled rows with the corrected A minus S difference and its bootstrap interval, halves and seeds (context; no AUC bar, the +0.016 of round 1 was measured for the T18 production recipe and does not transfer to a pooled top-100 design).
- The long-only companion: the mean abnormal 10-day return of the top decile minus 0.30% (round trip at 0.15% per side) with the same bootstrap, because the live constraint is no short book and decile-minus-decile is a hypothetical long-short.
- The same gate for the A2-dm arm.

## Power statement (written before the score)

About 330 test dates in the primary arm and a 10-day horizon overlapping across dates (effective dates fewer by the block bootstrap); the minimum detectable gate value is reported from the achieved bootstrap interval. If the first half is as weak as the abnormal-label realtime halves of round 2 (+0.002 corrected ΔAUC), the both-halves condition is the binding one, and a fail is the expected, registered outcome.

## Run order

1. Reviewer reads this file and the code (`cmd/algopackincremental` subcommands `panel` and `pscore`, `scripts/algopack-panel.sh`).
2. Build the panel files on the ai-box, train the three arms, score once.
3. Numbers into `docs/metrics.md` 8.17 together with rounds 1 and 2, `docs/knowledge/model-quality-history.md`, and the final verdict sentence.

## Result (scored once, 2026-10-23) and the forward rule

Primary arm (realtime-only, 326 dates, 30787 rows): gate mean +0.952% per date, date-block bootstrap 95% [+0.311%, +1.537%], Newey-West t +2.95, halves +0.564% / +1.213%, 5 of 5 seeds positive, S-dm gate mean -0.609% (interval below 0): **the registered decile gate passes; it is a lead.** Legs: top decile +0.14%, bottom decile -1.11% abnormal per 10 days; long-only top net -0.16%; AUC context A 0.524, S-dm 0.487, A2 0.514. 2020-plus arm (descriptive): +1.513%. Full numbers: `docs/metrics.md` 8.17, outputs `artifacts/algopack-forecast/`.

Caveats written with the result: the window was examined by rounds 1 and 2; the S-dm arm is itself significantly negative, a bias that inflates the A-S contrast (not the A gate); the Newey-West t is under 3; the long-only form is negative so the lead needs a short book whose real borrow rate is unmeasured (the gate stays positive at 17% and 25% a year: +0.48% and +0.26%).

**Forward rule (frozen 2026-10-23, before any forward date exists).** Arm A models trained with `scripts/algopack-panel.sh` regime `fwd` (fold 8: train start 2026-10-21 so `label_date` before 2026-10-11, five seeds 20261023..20261027, features the ten demeaned columns), SHA-256 of the five `f8.json` files of arm A: `d10133ba99bb2be0dadfde061c5b0254b626658dd92b2f3da198d4b4dce5c9a7`, `e6a3e603fa47dfe42884a8456cd6192824940c2898c2eb9b493597cf37888982`, `384a154e08722389b04460f19593946340b7d5609a0cc50d8963393f43dc64dc`, `d1811a4f9a62a34adc4ef4a98cd9b54a6a8d0636a93e989499d8d563f0063b35`, `9cfb88a3d7eb504190a34ffac110fec8d6a43d5cab30abafa2bcabd7dd151f55`. Evaluation once, after 60 forward feature dates (from 2026-10-21) whose 10-day outcomes have matured, about 2027-02-05, with `cmd/flowscan -mode export` and `cmd/algopackincremental panel` rebuilt on the refreshed data and `pscore -fold-from 8` (the refresh is the daily job `scripts/algopack-daily.sh`, launchd `com.oleg.moex-trader.algopackdaily`, 00:20 MSK, the last four days always refetched, the same job as the F2 read of plan 20261022; a forward date counts only if the rebuilt export produces rows for it, a missing or unusable day is not counted and is never backfilled after the evaluation date, the evaluation runs once on the first date on which 60 counted dates with matured outcomes exist and no day is added afterwards; the evaluation is run by the owner or by an agent on the owner's instruction, never by an automatic job): the same gate (seed-mean deciles, top minus bottom minus 0.30%), accepted as confirmed if the mean is positive with the date-block bootstrap 95% interval above 0 (one pre-specified rule), the sign agrees in at least four of five seeds, and the S-dm arm (trained the same way) has a mean at most 0 or an interval containing 0. The forward window is not extended or repeated to rescue it. Nothing enters the live path in either case without a separate owner decision, which would also need the real short-borrow rate.

## Review log

Pre-registration exchange (opencode, 2026-10-23): AUC conditions demoted to context and no +0.016 in round 3; the constant baseline dropped (S-dm is the null); seed rule four of five; the exact gate form above; demeaning and label pinned to the admitted universe; the one-line KS extension; the power statement; A2-dm and the long-only companion as descriptive reads; the stop rule. Applied. Code pass (opencode, 2026-10-23): no look-ahead; one real deviation (abnormal mean population) and two pins, all applied (errata 1-3).

## Errata

Written 2026-10-23 after the Reviewer's code pass, before any round 3 score; the models trained on the first panel build were discarded unscored and retrained on the corrected build.

1. The demean mean, the abnormal-return mean and the row set are all the admitted names with a full ten-column block and a valid 10-day return that date; corporate-action-dropped names (no valid return) and incomplete-block names are excluded from all three; deadband rows count toward the at-least-30 floor and enter the deciles, as registered. (Fixes a deviation of the first build, in which the abnormal mean also contained admitted names with an incomplete block.)
2. Ties in the decile ranking are broken by ticker ascending (stable sort on the descending seed-mean probability, input order date then ticker).
3. The gate bootstrap uses its own random source (seed 20261023), independent of the AUC context bootstrap (seed 20261022), so the gate interval does not depend on the AUC block.
