# Placebo check of the ridge table: `cmd/placebocheck` (rewritten 2026-10-19 instead of `cmd/synthcheck`)


Status: plan; criteria are fixed before implementation and before the first run. The implementation is carried out by one session (AGENTS.md rule 7: `git status` before editing).

## Why not synthetic prices

The first variant (`cmd/synthcheck`, block bootstrap of prices, a run of `EnsembleSignalSource`) would have tested the wrong model: since 2026-10-19 the live signal takes `ScoreTableSource` from `data/ridge_scores.csv` (`score_table_path`), and the table is tied to real dates and means nothing on synthetic prices. Recomputing ridge on a synthetic panel is possible only together with the whole point-in-time panel of 719 securities; that is a separate and expensive step (moved to the end). Besides, independent per-ticker paths destroyed the correlations between securities, and a bootstrap removes any serial structure, so an "outlier in either direction" said nothing about overfitting. Instead, the placebo works on real prices and the real book structure and breaks only the link between scores and future returns.

## What we test

Null hypothesis: the table scores are unrelated to subsequent returns; the result of the live book (longs and shorts, limits, costs, borrow) is explained by the book structure (net exposure, trade frequency, market drift). Statistic: realized P&L net of borrow on the replay.

Placebo A, cyclic shift. All tickers are shifted by a common offset of k trading days along the table's date axis (the tail wraps to the start). The score distribution, autocorrelation, simultaneous behaviour of securities and the number of trades are preserved; the link with time is broken.
Placebo B, cross-sectional permutation. On each date the scores are shuffled between tickers (deterministically from the seed, the run number and the date). The set of scores on each day and the total net exposure are preserved; the choice of specific securities is broken. It shows whether the P&L comes from selection rather than from the book's market direction.

For A the offsets |k| >= 21 (the 21-bar horizon) are enumerated deterministically, N = 200 distinct k (if fewer than 200 are admissible, all are taken). For B, N = 200 runs, seed = 20261019 + run number.

## Windows and regimes

- W2, primary: 2025-03-05..2026-10-18 (the whole accumulated table; the scores are out-of-sample by construction).
- W1, descriptive: the gate window 2026-07-20..2026-10-18.
- Daily borrow rate: 0.00005 (the accepted stress) and 0.00047 (REPO estimate of about 17% per year, see the section on the rate in `docs/metrics.md`). The other parameters as in preflight: 17 securities, commission 0.0005, spread 0.0005, slippage 0.0005, `target_notional` 15000, caps 90000 on net long and net short, thresholds 0.60/0.40.

## Criterion (before the run)

One-sided empirical p = (1 + number of placebos with P&L not below the real one) / (N + 1), on W2 for each (placebo, rate) pair.
- "Skill" is credited if p <= 0.05 in both placebos at the rate 0.00047.
- "Skill limited by the rate": p <= 0.05 at 0.00005 but not at 0.00047.
- Otherwise: "skill is indistinguishable from the book structure".
W1 and the percentiles of the real value within the placebo distribution are reported without a criterion. The result is not a gate and does not change the live path; exit code 0 on a successful run.
Caveat: the ridge windows and hyperparameters (alpha 10, recomputation every 63 days, thresholds 0.60/0.40) were chosen by the author after looking at this data, so the p-values are not corrected for selection and give only an upper bound of confidence.

## Implementation

1. `internal/placebo`: `Shift(table, k)`, `PermuteByDate(table, seed)` over the `internal/model.ScoreTable` structure (key ticker+date, `p`), with no disk writes. Invariants: a common offset for all tickers; the multiset of a ticker's scores is preserved under shift; the multiset of a date's scores is preserved under permutation; k = 0 gives the original table.
2. `cmd/placebocheck` modelled on `cmd/strategyvalidation`, flags: `-config`, `-score-table`, `-from`, `-till`, `-n` (200), `-borrow` (a list, default 0.00005,0.00047), `-seed`, `-workers`, `-out`. The candle history is loaded once through `backtest.NewISSSource` and put into an in-memory `HistoricalSource`; the run goes through the existing `backtest.NewEngine` with the same `ScoreTableSource` as the live path (it implements the `backtest.SignalSource` interface). Money only as `decimal.Decimal`. Parallelism is limited by `-workers`.
3. Report `reports/<date>/placebo.json` and `.md`: parameters, real P&L, placebo distribution (mean, sd, quantiles 5/50/95), p, the number of closed trades of the real book and of the placebos, verdict.
4. Tests (stdlib testing, fakes, determinism): invariants of `internal/placebo`; e2e on `T.TempDir` with a fake `HistoricalSource`: an oracle table built from future returns gives p close to the minimum 1/(N+1); a random table gives an unbiased p (checked over many seeds); a repeated run with the same seed gives the same report.
5. Before the main run, measure the time of one engine run on W2 and record it in the report; N = 200 for four combinations (placebo x rate) on two windows is about 1600 runs, to be computed on the ai-box if needed.
6. The numbers go into `docs/metrics.md` in the same commit (rule 8); prose and caveats into `docs/knowledge/validation-and-ops.md`, a row in the knowledge index and a mention next to `cmd/strategyvalidation`.

## What we do not do

We do not touch preflight and the live path; we do not add a config section (flags only); we do not write to `ensemble_model.json` or `data/ridge_scores.csv` and do not restart `./trader`; no new dependencies.

## Deferred stage: synthetic panel

If the owner wants a check for overfitting of ridge itself: a stationary block bootstrap of the panel's log-returns shared by all tickers (`icaport.StationaryIndices`, mean block 10 days, one set of indices for all securities), recomputation of the five signals and ridge on the synthetic panel, then a replay. It needs a prior measurement of the recomputation cost on 719 securities, N >= 200, and the criteria are fixed in a separate addendum before the run. Dividend gaps must not be carried along with the dates of repeated blocks: returns are taken in full (with dividends), as in step 25.
