# Revision plan for "When Alpha Survives Only on Paper"

Base reviewed: `alpha_on_paper_preprint-1.pdf`, 41 pages, 46 research directions.
Referee report: 10 November 2026, recommendation major revision, 38 numerical checks, 35 exact.
Draft in the repository at the time of this plan: 37 pages, 42 directions.
Supporting calculations: `paper/recalc-20261110.md` and `paper/recalc/main.go`.

**Status 10 November 2026: P0, P1 and P2 are applied to `paper/preprint` (version 4, 42 pages,
rebuilt).** The abstract, Section 1, the criteria block of Section 4, Section 5 including the
momentum table, Section 6 including the measured-carry, short-leg and cost-model paragraphs,
Section 7 including the borrow subsection, the equivalence formula, the power table and the second
real-time window, the limitations and the conclusion all carry the measured numbers. P2 additions:
the model-choice paragraph (ridge against the blend), the placebo criterion restated at the level of
the forecast, the dual cost model as a stated decision variable, the realtime-2 protocol
(Section 7) and its registration package (`paper/registration.md`). Remaining: the response letter,
the deposit itself, and the 561-date accrual.

## 0. Diagnosis

The referee is right about the numbers and right about the direction. The three defects that matter
are the power of the confirmation rule, the mismatch between the declared \|t\| >= 3.0 standard and
the gate the composite actually passed, and an economic result whose decisive inputs were assumed.
None of them is fatal, and none requires new data to fix.

There is a fourth fact the referee could not see, and it outranks all three. Since the reviewed
draft the programme has produced exactly the two measurements the report asks for:

- **E1, the momentum control, has been run and closed null** (`docs/metrics/s8b-portfolio.md` 8.46):
  composite minus a 256-day momentum book is +0.019 %/day at 25 % carry with an interval
  [-0.074, +0.097]; the residual keeps 91 % of the ranking power (IC 0.0869, t 7.33) but is not
  separable from momentum at book level; the composite IC (0.0957) is below mom256's (0.1072, t 3.95).
- **The borrow rate has been measured** (8.47): 72 of 99 admitted names resolved, Dshort median
  25.0 %, quartiles 22.0 % and 36.9 %, maximum 125 %, and **not one name at or below the 17 % base**
  the paper uses. At 25 % the book earns +0.038 %/day (t 0.89), at 30 % +0.020 %, at 36.9 % -0.006 %.

So the paper is behind its own evidence. The revision is therefore not a defence; it is a
re-anchoring. The thesis moves from

> order-flow statistics predict the 10-day cross-section, but monetization is not established

to

> order-flow statistics rank the 10-day cross-section **no better than 12-month momentum**, and
> neither the composite nor the momentum book survives quoted costs and the measured carry; what
> survives is a point-in-time audit of a commercial dataset and a documented provenance threat.

The second thesis is stronger, more publishable, and already supported by committed numbers.

## 1. P0 — bring the text up to E1 and 8.47 (no new computation, 1-2 days)

| # | Change | Where | Numbers to use |
|---|---|---|---|
| 1.1 | Replace the headline economics with the measured carry | abstract, §1, §6, §7 | 0.0383 %/day at 25 % (t 0.89, [-0.040, +0.122]); +0.0199 at 30 %; -0.006 at 36.9 %; the 17 % figure stops being a result |
| 1.2 | Turn "is the signal distinct from momentum?" into a result | §5, Box 2, abstract | E1 contrast +0.019 %/day [-0.074, +0.097]; residual 91 % of IC; mom256 IC 0.1072 (t 3.95) against composite 0.0957 |
| 1.3 | Move borrow from "assumed" to "measured" | §4, §7 table, §8 | Dshort median 25.0 %, p25 22.0 %, p75 36.9 %; 68 of 72 short-enabled; availability still assumed |
| 1.4 | Rewrite the carry sentence | §4 "Three cost models" | 17 % is below the measured minimum quartile; the base is 25 %, stress 30-37 % |
| 1.5 | Soften the causal claim of the provenance finding | Box 2, §1, §5.5 | "is associated with" rather than "inflates"; the confound with calendar time is real and cannot be broken by KS distances |
| 1.6 | Correct the ledger statuses | appendix C | the two directions that ran without a completed review (one of them the momentum confirmation); ledger #14 as exploratory |

Acceptance: no number in the abstract that is contradicted by `docs/metrics/s8b-portfolio.md` 8.46
or 8.47; every carry figure traceable to a committed artifact.

## 2. P1 — the four real defects (numbers already computed, 2-3 days)

**2.1 Power of the confirmation rule (M3, V4).** The 60-date rule has 14.8 % power against the
observed increment; MDE 0.0787 against 0.0368. Replace it with an explicit two-part rule:

- 60 forward dates can **falsify** (sign flip or an interval excluding the observed effect) but
  never confirm;
- confirmation requires 561 dates at the t > 1.96 rule, 1 056 at \|t\| >= 3.0, 1 317 at \|t\| >= 3.45;
- print the power table (n = 60 / 120 / 250 / 450) next to the rule, and the empirical block
  bootstrap that reproduces the standard error (0.0408 against 0.0402).

Cheapest honest fix: keep 60 dates, rename the reading "falsification check", and make the frozen
evaluation two-stage. Do not claim a confirmation horizon the power cannot support.

**2.2 One evidentiary standard (M2, M4, V3).** The composite passed a CI-above-zero gate while the
paper demands \|t\| >= 3.0 for single features. Either

- (a) state formally why the gate differs — the composite's inputs were fixed by family before the
  window was used, its weights are fitted out-of-fold, so the multiplicity of 41 trials does not
  apply to the increment; report the deflated signal probability anyway, or
- (b) lower the status of the signal claim to lead, consistently with rule 4.1(ii), and say so in
  the abstract.

Recommend (a) plus a sentence that the increment, t 2.51, does not clear the paper's own bar while
mom256's t 3.95 does — the asymmetry belongs in the open, not in §5.5.

**2.3 Equivalence arithmetic (m14, V9).** Print
`T = ((z_0.975 + z_0.80) sigma_LR / delta)^2` with `sigma_LR = 0.94` (autocorrelation-adjusted
daily volatility, against the plain 0.766): 1 231 dates for delta 0.0751 against the published
1 160, 2 215 for delta 0.056 against the published 2 300. The published pair is internally
consistent to 4-6 %; only the formula and sigma were missing.

**2.4 Short-leg economics (M5).**
- headline becomes the beta-hedged figure +0.0538 %/day (t 1.49) next to +0.0751 unhedged;
- carry sweep 17 / 20 / 25 / 27.5 / 30 / 37 % = +0.0751 / +0.0637 / +0.0446 / +0.0351 / +0.0256 / -0.0011;
- dividend compensation: bias = 0.96 · yield gap / 252, i.e. 0.0038 %/day per 1 pp of gap, worst
  case 0.0126 %/day (17 % of the net) if the long leg earned nothing — one paragraph, an upper bound,
  no new data;
- short leg share of gross 89.5 % (0.2002 against 0.0236), stated in §6;
- autocorrelation-consistent Sharpe 1.14-1.27 next to the 1.56 iid figure, with the volatility
  0.766 %/day and V' = 1.87 named in the text (m17).

**2.5 The m-list.** m1 (457 book dates against 450 IC dates: horizon plus the 20 October cut),
m15 (MDE 0.063, not 0.07), m16 (thin universe deserves its paragraph), m18 (sector-neutral DSR
0.399 / 0.120 reproduces exactly in `w5-verdict-sector.txt` — the referee's "approximate" is exact),
quarter count 7 of 8, not 7 of 7: 2026Q4 to 16 October is -0.266.

## 3. P2 — what makes it accepted rather than merely corrected (weeks)

1. **Realtime-2 and one frozen reading** (limitations registry item 3). New feature dates from
   2026-10-19; a written evaluation date; no intermediate looks; the spent window called "spent"
   in every table. Without this the referee's M4 objection returns in any resubmission.
2. **External registration.** OSF or equivalent, at least a timestamped hash of the protocol. The
   paper's own appendix F admits the pre-registration is internal.
3. **Both cost models as a stated decision variable.** E1 shows the contrast between composite and
   momentum flips sign between quoted BBO and the live flat 0.15 % tariff. That is a finding, not
   a footnote: report both, state which one the live account pays, and let the reader see the sign
   depend on it.
4. **Cost-matched placebos** (M8ii). The placebo books lose 0.34-0.60 %/day mostly through turnover.
   Report the gross placebo comparison and the cost-matched net; the current criterion is close to
   mechanical.
5. **Ensemble or ridge** (M8i). Ridge alone has IC 0.0970 against the blend's 0.0957. Either justify
   the blend on the fitting sample or report ridge as the model and the blend as a variant.
6. **Sign tests under overlap** (M7). 45 non-overlapping 10-date blocks: 38 positive, p < 0.0001;
   monthly, 18 of 22 with an effective 15.4 months, p = 0.0037. One table row, removes the objection.
7. **Reproducibility appendix.** Seed and commit hash printed for every table-producing result, in
   the paper and not only in the repository, plus a line in the QFRS checklist for the power of
   every confirmation test — the line whose absence produced M3.

## 4. Points to dispute, with evidence

| Referee point | Position | Evidence |
|---|---|---|
| "about 2 300 days is not reproducible" (m14) | the rule reproduces both published numbers within 6 %; the text omitted sigma_LR, not the arithmetic | `paper/recalc-20261110.md` §2 |
| Sharpe 0.13 against the policy rate (M5e) | a category error for a financed dollar-neutral book: financing cancels across the legs, so 1.56 is an information ratio, not an excess return. Report an IR and, separately, the excess return over the key rate — do not replace one with the other | §4.5 of the paper, and the referee's own hedge identity |
| M1's t = 3.41 (V1) | the number is real but comes from a regression slope in `docs/metrics/s8b-portfolio.md` 8.34, not from the preprint, which reports an IC increment of t 2.39. Both statistics exist and measure different things; the paper must report both, and the referee's §4 claim that all inputs came from the text is wrong on this point | 8.34 against sec5.tex:48 |
| the composite is a momentum proxy (III, M1i) | it is not: the frozen score correlates 0.26 with the 256-day momentum rank, against 0.60-0.67 the paper reports for TimesFM. The book-level tie is about cost and carry, not about ranking content | `paper/recalc-20261110.md` §7; indicative only, the score path is a reimplementation |

## 5. Verification run of 10 November 2026

- `go build ./...` clean; `go test ./...` green on 95 packages, zero failures, including the
  failover, witness and watchdog suites the repository rules require before election changes.
- Matrix rebuilt from `data/algopack` with the published flags: 163 610 rows, 1 644 dates, one date
  short of the archived build.
- m13 closed: 163 610 matrix rows, 159 726 of them with a mature 10-day outcome against the 159 708
  training rows — 18 rows, i.e. the 4 001-row gap was the missing outcomes, not a filter defect.
- V1 read indicatively at 0.26; the exact figure needs the forecast dump, which lives in the model
  zone and was not touched.

## 6. Sequence and effort

| Wave | Content | Effort | Blocking |
|---|---|---|---|
| 1 | P0 entirely, plus 2.3, 2.4, 2.5 text edits | 1-2 days | none, all numbers exist |
| 2 | 2.1 and 2.2 rewritten rules, the power table, the response letter, the version note (41 pp / 46 directions against 37 pp / 42) | 2-3 days | wave 1 |
| 3 | P2 items 1-3: realtime-2 designation, registration, dual cost model | 1 week of setup, then accrual | owner decision |
| 4 | realtime-2 reading | 561 trading dates, about 2.2 years from 2026-10 | wave 3 |

Wave 1 and 2 alone make the paper internally consistent, honest about the null, and answerable to
every mandatory item of the report. Wave 3 is what makes the confirmation claim checkable by
someone else.

## 7. Risks

- **Do not defend the 0.075 % headline.** It rests on a carry that the measured distribution rules
  out for every admitted name. Defending it converts a strong paper into a weak one.
- **Do not extend or re-read the spent window.** Any new look costs the pre-registration argument
  that is the paper's main asset.
- **Do not let the momentum parity be buried.** It is the most interesting empirical fact in the
  programme: 75 order-flow features rank the cross-section at least as well as a 12-month price
  rule, and 91 % of that ranking survives the momentum span, yet neither book pays after cost and
  carry. That sentence is the paper.
- **Version confusion.** The referee reviewed 41 pages and 46 directions; the repository holds 37
  and 42. The response letter must open with the mapping, or every point about counts looks evasive.
