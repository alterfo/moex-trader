# AlgoPack wave 3: realistic costs, longer horizons, next-open entry (2026-10-24)

Status: DONE 2026-10-24 (English only; scored once, primary gate failed narrowly), registered before any wave-3 number is computed. Continues `docs/plans/20261023-algopack-alphafit.md` (composite, partial-adjustment book) and the 80/20 map of `docs/plans/20261024-algopack-meta-wave2.md`.

## Why this plan (the vital few)

The composite predicts (realtime IC 0.096) and the partial-adjustment book nets +0.030% per day at 17% borrow with a bootstrap interval that includes zero. Three facts from the earlier waves say where the money is decided:

1. **The binding constraint is cost, not signal.** The daily-rebalanced book's gross (+0.282% per day) equals its costs (0.280%); the partial adjustment is what makes it alive. All books assumed a flat 0.15% per side for every name. The quoted spread says otherwise: the mean `SpBBO` is 3.6 bp in the most liquid third of the top-100 and 18.1 bp in the thinnest third, and the spread for a 1 million RUB order (`Sp1m`) is 11 bp against 144 bp. The flat cost overstates the price of liquid names and (for larger orders) understates the price of thin ones, and the composite's strongest IC is in the thinnest tercile (0.161 in the calm state).
2. **The signal is persistent.** The composite with every feature lagged by five dates keeps an IC of 0.089 against 0.114 (exploration), which means the ten-day target is not the natural horizon; a 20- or 40-day target needs far less turnover.
3. **Entry timing is unmeasured.** All returns are close to close of the main session; the decision uses bars up to 18:30. Whether the edge is in the next overnight gap (not capturable by an order sent after 18:30 at the open) is a one-line decomposition.

## Design (one chain, no grid)

Data: `cmd/flowscan -mode matrix` now also writes `matrix.csv.ext` (side file, the matrix itself is unchanged) with the 20- and 40-day forward returns and their universe means, and the next day's overnight (close to open) and intraday (open to close) log returns; the windows crossing a dividend ex-date are dropped by the same rule as the 10-day return (`spanClean`). Verified before use: the rebuilt matrix is byte-identical to the one of the alphafit verdict, or the difference is reported.

Cost models of `cmd/alphafit` (`-cost`): `flat` (0.15% a side, the old convention), `bbo` (0.05% commission plus half the day's `SpBBO` in basis points, primary, an order of ordinary retail size), `mio` (0.05% plus half of `Sp1m`, stress, an order of about one million RUB). A name without a spread falls back to 0.15%.

Chain, in this order:

1. **Exploration** (regenerated window 2022-07..2024-12, the only place anything is chosen): for each horizon H in {10, 20, 40} trading days, with the `bbo` cost, `explore` selects the ridge and elastic-net penalties and the trade rate from the usual grid (1, 0.5, 0.33, 0.2, 0.1, 0.05) by the exploration net. The target is the within-date rank of the abnormal H-day return; the purge and embargo are H+1 trading days. The horizon H* is the one with the highest exploration net of the partial-adjustment composite book. The exploration window is regenerated history: this selection is optimistic by construction and is not evidence.
2. **Verdict, run once on the realtime window** (feature dates 2025-01-03..2026-10-16, halves 2026-01-05, expanding walk-forward, five placebo seeds 20261023..27, date-block bootstrap with block 10 and 5000 draws): the composite and the A2 baseline at H*, partial-adjustment book at the exploration trade rate, `bbo` costs, borrow 17% a year. This is the third look at this window for the economics (after plans 20261023-alphafit and 20261024-meta-variables), so a pass is a lead, never a result.
3. **Frozen primary gate**: the mean daily net of the composite book has the bootstrap 95% interval above 0, the mean is positive in both halves, and the five placebo books have a mean net not above 0. Also reported next to it, not conditions: the same H* book under `flat` and `mio` costs, at 25% borrow, the H = 10 composite under `bbo` (to separate the cost-model effect from the horizon effect), the IC at H*, and the next-open entry (`-entry open`: the change in weights earns only the next day's intraday return, the held part earns the full day) for the same book.

## Stop rule and honest expectations

The three costs and horizon choices are one chain with a single verdict; if H* is 10 the horizon question closes as "no gain". A fail of the primary gate means that, at realistic retail-size costs, the composite book has no demonstrable economic edge on this window, and no further cost or horizon variant is tried on it. A pass is a lead with a frozen forward rule: models fixed at the training end of the verdict, evaluation after 60 forward dates, interval above 0 required (minimum detectable effect at 60 dates is large, about +0.2% per day, so the expectation is an inconclusive forward read). Prior of passing: low-medium. The expected value of this plan is the answer to "is the old flat cost hiding or inventing the edge", which is informative either way.

## Power statement (before the run)

450 dates. The verdict half-width of the daily net in the alphafit verdict was about 0.08% per day; a longer horizon with a quarter of the turnover reduces cost drag by roughly 0.1-0.2% per day of gross, which is within reach only if the gross at the longer horizon stays near 0.19%. The ten-day label overlaps across dates, a 40-day label overlaps four times more, so the effective number of independent dates falls with H; the bootstrap block is max(10, H) dates (20 at H = 20, 40 at H = 40), as coded in `bootBlock`.

## Run order

1. Reviewer reads this file and the code (`cmd/flowscan/matrix.go` side file, `cmd/alphafit/ext.go`, `eval.go` book step).
2. Rebuild the matrix on the ai-box with `cmd/flowscan -mode matrix -from 2020-01-01 -till 2026-10-21 -lag 30` and compare `sha256sum` with the verdict matrix (`c0580fbeace81ad530fc…`); if any row differs, the exploration and the verdict wait until the difference is explained (the chain reuses the frozen features and drift would change the composite silently). Then run the three explorations.
3. Run the verdict once for H*; numbers into `docs/metrics.md` 8.23 and `docs/knowledge/model-quality-history.md`.

## Result (scored once, 2026-10-24)

Numbers in `docs/metrics.md` 8.23, outputs `artifacts/algopack-wave3/`. Exploration (bbo costs, partial-adjustment composite net per day at the best trade rate): H = 10 +0.162% (rate 0.5), H = 20 +0.141% (0.5), H = 40 +0.097% (0.33): H* = 10, the horizon question closes as no gain (longer horizons lose gross faster than they save cost). Realtime verdict at H* = 10, bbo costs, rate 0.5, borrow 17%: composite IC +0.0957 (unchanged); partial-adjustment composite net **+0.0753% per day, bootstrap 95% [-0.005%, +0.163%], halves +0.066% / +0.087%**, placebo books -0.34..-0.40% per day; **primary gate: FAIL by the lower bound of the interval (-0.005% against the required above 0)**; +0.0445% per day at 25% borrow. Daily-rebalanced book at bbo costs +0.048% per day [-0.037%, +0.142%]. Stress variants (reported, not conditions): 1 mln RUB order costs (`mio`) partial-adjustment net -0.026% per day [-0.109%, +0.062%]; the old flat 0.15% convention at rate 0.5 +0.018% per day; next-open entry at bbo costs +0.031% per day [-0.057%, +0.122%] (gross of the daily book falls from 0.282% to 0.224%: about a fifth of the gross is the overnight gap). Registered reading: no pass; the economics are positive in sign and in both halves at retail order size but inside the noise, and vanish for a one-million order.

## Forward addendum (frozen 2026-10-24)

At the alphafit forward read (60 forward dates, about 2027-02-05) the partial-adjustment book at rate 0.5 under `bbo` costs is reported next to the frozen rate 0.33 flat-cost book (report only, same frozen models). Nothing else changes.

## Review log

Pre-run pass (opencode, 2026-10-24): no leakage in the horizon change; side-file alignment, cost accounting and `bootBlock` correct; the chain hides no grid; amendments applied (hash guard in the run order, three pins in the errata).

## Errata

Written 2026-10-24 after the Reviewer's pre-run pass, before any wave-3 number.

1. The matrix rebuild was compared before the exploration: the first rebuild with `-till 2026-10-20` differed in the last dates only (the forward windows of the final days), the rebuild with `-till 2026-10-21` is byte-identical (sha256 prefix `c0580fbeace81ad5`).
2. The open-entry trade pays the signal day's spread (day-d features), not the spread at the open of the trade day; approximate by design. The `mio` stress uses half of `Sp1m` plus the same 0.05% commission. For H = 20 and 40 only the training target is overwritten (the within-date rank, the at-least-30 floor and the dividend handling are those of H = 10); the book return `f1`, the placeholder columns `f1`, `f5`, `u1`, `u5` and the controls are untouched.
3. The exploration selection surface is 3 horizons times 6 ridge penalties times 6 trade rates (and 5 elastic-net penalties), about 108 combinations of the main choices; the one-shot realtime verdict is what bounds the selection bias. The Reviewer expects H* = 10 (cost and regenerated-window differences); that outcome closes the horizon question and is a result.
