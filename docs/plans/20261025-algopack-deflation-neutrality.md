# AlgoPack book: deflated Sharpe and neutrality to the market and sectors (2026-10-25)

Status: DONE 2026-10-25 (English only; scored once, book not distinguishable from zero after deflation), registered before the runs. Prompted by the owner's request to look at how funds work (courses, fund strategies) and to do both follow-ups: (a) correct the book's Sharpe ratio for the number of trials, (b) test whether the book survives neutralisation to the market and to sectors.

## Why

What the funds do that this study did not: they control the number of trials (the Deflated Sharpe Ratio of Bailey and Lopez de Prado asks how likely the observed Sharpe is the maximum of N zero-skill trials), and they build books neutral to common risk factors so that the P&L is alpha and not market or sector exposure. Our books are dollar-neutral with inverse-volatility legs but not beta-neutral and not sector-neutral. The best book (`docs/metrics.md` 8.23: partial adjustment, bbo costs, rate 0.5, +0.0753% per day, bootstrap interval [-0.005%, +0.163%]) has therefore never been tested against either objection.

## (a) Deflated Sharpe

`cmd/alphafit -mode dsr -base <dump CSV> -col net_gp_blend`: the daily Sharpe of the net series (457 days), its skew and kurtosis, the probabilistic Sharpe ratio against zero, and the deflated Sharpe probability for N = 1, 10, 50, 100, 300, 800 trials with the Bailey-Lopez de Prado expected-maximum formula (variance of the trial Sharpe ratios taken as 1/T, the null value, which is stated as an assumption). The number of independent trials that the book really survived is not knowable; about 800 rows were registered across the plans and a few dozen model and book variants (horizon, cost, rate); the table makes the dependence visible and no value of N is chosen after seeing the result. Descriptive; the reading rule written now: if the deflated probability is below 0.95 already at N = 10 the book is not distinguishable from the best of ten zero-skill books.

## (b) Neutrality

1. **Market**: regression of the daily net on the universe mean 1-day return (`u1`), beta, and the beta-hedged residual with Newey-West t and bootstrap interval. Free in the dump (a new `u1` column), no new model.
2. **Sector**: the composite prediction is demeaned within frozen sector lists before the quintile book is built (`-neutral sector`; lists `fin`, `oilgas`, `metals`, `power` in `cmd/alphafit/neutral.go`, everything else one remaining group; at least three names in a group). One realtime verdict run of the sector-neutral composite with the same frozen parameters, bbo costs, rate 0.5, borrow 17%, five placebos. The registered gate is that of wave 3: the net interval above 0, positive in both halves, placebo books not above 0. This is the sixth look at the economics on a spent window; a pass is a lead only, a fail closes the neutrality question for this composite.

## Honest expectations

The deflation is expected to put the book near or below the 0.95 line at any N above 10 (an interval that touches zero over 457 days). The market regression is expected to show a small beta (a dollar-neutral book) and an unchanged residual; the sector-neutral book is expected to lose a part of the gross if the signal partly is a sector rotation (LevAsym and SpLv, book-shape features, can be sector-level liquidity traits) and to keep it if it is stock-specific. Either answer is informative for the product question: a sector-neutral edge is a more defensible claim.

## Run order

1. Reviewer reads this file and `cmd/alphafit/neutral.go`.
2. `dump` of the base book (new `u1` column, otherwise byte-identical) and `dsr` on it; `verdict -neutral sector` once and `dump -neutral sector` with `dsr`.
3. Numbers into `docs/metrics.md` 8.28.

## Result (scored once, 2026-10-25)

Numbers in `docs/metrics.md` 8.28, outputs `artifacts/algopack-deflation/`. **(a) Deflated Sharpe of the primary book** (bbo costs, rate 0.5, borrow 17%; 457 days, daily Sharpe +0.098, annualised +1.56, skew -1.52, kurtosis 16.4): probabilistic Sharpe against zero 0.973; deflated probability with V = 1/T: 0.685 (10 trials), 0.435 (50), 0.346 (100), 0.233 (300), 0.158 (800); with the autocorrelation-adjusted variance V' (factor 1.87): 0.480, 0.176, 0.106, 0.044, 0.019. By the rule written before the run (below 0.95 already at 10 trials) **the book is not distinguishable from the best of ten zero-skill books.** **(b1) Market:** beta of the book on the universe mean return -0.269 (a net short market exposure; the sample window had a falling universe, mean about -0.08% per day), beta-hedged residual +0.0538% per day (NW t +1.49, boot95 [-0.012%, +0.125%]) against +0.0751% unhedged. **(b2) Sector-neutral composite:** IC +0.0819 (-14% against 0.0957, interval [+0.056, +0.113], 5 of 5 placebos); partial-adjustment net +0.0446% per day [-0.027%, +0.126%], NW t +1.12, halves +0.031% / +0.062%, +0.0140% at 25% borrow, placebo books -0.34..-0.41%; **the wave-3 gate fails** (interval touches zero); deflated probability 0.399 at 10 trials; beta unchanged (-0.265), hedged residual +0.0235% per day (t +0.73). Registered reading: neutralising the sectors costs about 40% of the net and 14% of the IC, does not neutralise the market, and leaves the book statistically indistinguishable from zero; the composite is not sector rotation but it is not a deflated-proof edge either.

## Review log

Pre-run pass (opencode, 2026-10-25): the deflated Sharpe formula verified term by term, V = 1/T defensible, u1 aligned with the book P&L, sector lists acceptable, one non-grid neutral run; amendments applied (errata 1-3).

## Errata

Written 2026-10-25 after the Reviewer's pass, before the sector-neutral run.

1. A sensitivity is added to the deflation: the null variance V' = (1 + 2 times the sum of the first ten empirical autocorrelations of the net series) divided by T (floored at 1/T) is reported next to V = 1/T; the reading rule is unchanged.
2. The beta regression is also run on the sector-neutral dump. The deflation applies to the primary book only (bbo costs, rate 0.5, borrow 17%); the 25% borrow stress is reported undeflated and the two are never mixed.
3. The sector lists are frozen judgment calls: RASP (coal) and ENPG (an energy-plus-metals holding) sit in `metals`; the remaining group holds about two thirds of the names and is a weak control.
