# Closing the limitations of the research programme (2026-10-29)

Status: registered 2026-10-29 before the first run of E1; item 1 RUN the same day and closed as null (metrics 8.46). English only. This file is the registry of the limitations stated in the programme's conclusion and the action that removes each one. A limitation is removed only by a measurement or a design change committed with its numbers in `docs/metrics.md`; the ones that no measurement can remove are listed as scope facts with a mitigation, not as hedges.

## Registry

| # | Limitation as stated | Action | Criterion / output | Status |
|---|---|---|---|---|
| 1 | value of the flow/composite signal beyond 12-month momentum is not established (E1 never run) | E1, this file, section "E1" | composite book minus momentum book and residual book, same machinery, costs and carry; direction and interval | DONE 2026-10-29: **null**, composite minus momentum +0.019% a day at 25% carry with the interval containing zero, residual keeps 91% of the IC but is not separated from zero; metrics 8.46 |
| 2 | short-side economics rest on unmeasured inputs while the profit is concentrated in the short leg | `cmd/borrowmeasure` on the T-Bank sandbox for every admitted name (rate, limit, margin), plus a daily append-only availability snapshot job; economics re-read at the measured rate; short-leg share of net reported per book | measured rate replaces 17% and 25% as the primary carry; availability is recorded, never assumed | QUEUED |
| 3 | the realtime window has been reused many times | designate realtime-2 (feature dates from 2026-10-19, after the last run's boundary) as untouched; one evaluation of the frozen rule at a pre-set date; no other look before it | one number, one date, no re-reads; the old window is called "spent" in every later table | QUEUED |
| 4 | one market, one author, no independent replication | frozen specification (files, hashes, commands, expected columns) published for a second executor; every plan records who reviewed it | a replication package that a third party can run without our notes | MITIGATION (cannot be measured away) |
| 5 | short window: 457 realtime dates, about 45 non-overlapping 10-day periods, two quarters almost without signal | power statement per verdict (minimum detectable effect from the same bootstrap); forward accrual to at least 700 dates; signal counts per quarter reported | a verdict is not read below the power floor; the forward test of the frozen rule is the only confirmation path | QUEUED |
| 6 | regeneration and time confound (AlgoPack history is regenerated before 2025) | every verdict on realtime-only dates; regenerated dates used only for falsification; year and era fixed effects reported for the composite IC | realtime-only numbers in the verdict rows; regenerated rows labelled as falsification | DONE for existing rows (8.16-8.39 use 2025-01-03..2026-10-16); E1 follows |
| 7 | costs are quoted (BBO) rather than executed | executed-cost model: measured session spread by liquidity third (`cmd/exectiming`), realised fill prices from `cmd/tradesrecord`, participation impact; economics re-read with it next to the quoted read | both reads reported; the executed read is the one that decides | PARTIAL: quoted BBO against the flat live tariff measured in 8.46 (the sign of the E1 contrast depends on which is used); the executed-fill model is still queued |
| 8 | a single portfolio mapping | pre-registered mapping family (equal weight, inverse volatility, quintile, top-k, long-only, long-short) under one criterion set, with the family size stated | the signal survives at least two mappings or the mapping is named as the driver | QUEUED (R0-R3 of 8.x cover part of it) |
| 9 | hyperparameters tuned on regenerated history | nested selection inside the realtime window (rolling folds), reported selection path and the frozen-versus-nested degradation | nested read within the interval of the frozen read, or the degradation is stated | QUEUED |
| 10 | pre-registration is internal and the reviewer is a language model; two studies ran without a completed review | reviewer status is a mandatory line in every plan; review artefacts kept next to the numbers; a plan without a completed review is never called confirmed | no "confirmed" verdict without a completed review line | PROCESS (adopted now; the two gaps stay in history) |

## E1. Value of the composite beyond 12-month momentum

Fixed inputs: matrix `cmd/flowscan -mode matrix` (1644 dates 2020-01..2026-10-20, sha256 `c0580fbe`), frozen models `alphafit-models-20261023.json` (sha256 `793bf125`), frozen hyperparameters `alphafit-hyper-20261023.json` (trade rate 0.33), tool `cmd/alphafit -mode momctrl` (new mode, same book machinery as `-mode dump` and `-mode momsleeve`). Window: 2025-01-03..2026-10-16 (457 realtime feature dates), halves split 2026-01-05. Horizon 10 days, target rank of the 10-day abnormal return, top-100 point-in-time universe.

Signals:
- **composite** — the frozen ridge-plus-elastic-net blend, the same prediction the published base book trades.
- **momentum** — `mom256`, the sum of the last 256 daily abnormal returns of the name (the confirmed 12-month form of 8.36), missing below 256 observations.
- **residual** — per date, the rank-scaled composite minus its ordinary least squares projection on the rank-scaled `mom256` over the names where both exist, i.e. the part of the composite the momentum does not span (centred, zero mean each date).

Books: all three signals go through the identical `aimWeights` quintile book (top quintile long, bottom quintile short, inverse-volatility weights) and the identical partial-adjustment step at the frozen trade rate 0.33, with bbo cost and carry at 17% and 25%; rate 0.50 is reported as a secondary read for comparability with 8.20-8.28. No retraining, no re-selection, one run.

Registered criteria, all on net at 25% carry (the binding carry of the published book):
1. **Primary.** mean daily net of composite minus mean daily net of momentum > 0, date-block bootstrap 95% interval above zero, and the same sign in both halves.
2. **Secondary.** mean daily net of residual minus momentum > 0 with the interval above zero, and the residual keeps at least half of the composite's net.
3. **Reported, not a criterion.** IC of each signal against the 10-day abnormal rank, the increment of the composite over `mom256` per date, turnover and break-even cost per side of each book, short-leg carry paid.

Verdict: **established** if 1 and 2 hold (the flow signal carries value the momentum does not); **partial** if only 1 holds (the composite beats a momentum book but not a momentum-adjusted composite); **null** otherwise, in which case the economic sentence of the programme is reduced to "the 5-minute flow statistics rank the 10-day cross-section like a 12-month momentum does, and neither converts into net return after costs and carry", and no economic claim survives.

Power note written before the run: the published base read has a bootstrap half-width of about 0.08% per day at 457 dates, so a composite-minus-momentum difference below about 0.08% per day cannot be resolved here; a difference inside that band is reported as "no established difference", never as equality. About 45 non-overlapping 10-day periods carry the full weight of the window, which is why criteria 1 and 2 need the same sign in both halves as well as the interval.
