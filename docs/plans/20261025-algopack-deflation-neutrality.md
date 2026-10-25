# AlgoPack book: deflated Sharpe and neutrality to the market and sectors (2026-10-25)

Status: REGISTERED 2026-10-25 (English only), before the runs. Prompted by the owner's request to look at how funds work (courses, fund strategies) and to do both follow-ups: (a) correct the book's Sharpe ratio for the number of trials, (b) test whether the book survives neutralisation to the market and to sectors.

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

## Review log

(pending the Reviewer's pre-run pass)

## Errata

(none)
