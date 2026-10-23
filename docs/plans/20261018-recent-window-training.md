# Training on short recent windows: 10/30/90 days (2026-10-18)


The owner's question: does training only on recent data (logistic regression and the full ensemble) cure model staleness, with windows of 10, 30 and 90 days.

## Finding before the experiment

`export_ensemble.py` takes only rows with `split == "train"`, and `exportdataset -split D` marks as train the rows with `label_date < D`. The manual candidate of 2026-10-18 was exported with `-split 2026-06-18`, so it was actually trained on data up to 2026-06-06, and `ENSEMBLE_TRAIN_TILL=2026-10-17` extends nothing. The candidate contained no data after June, and the hold30/pre90 windows are entirely outside its training for it. The "Manual retrain candidate" row in `docs/metrics.md` describes training up to 10-17 incorrectly. The deployed model was trained on the same data up to June.

## Design (fixed before the run)

Dataset: `exportdataset` with `-from 2024-01-01 -till 2026-10-18 -split 2026-10-18`, 18 tickers, recipe unchanged (absolute-10d, 18 features, 0.60/0.40, embargo 10).

Test windows (outside training), as in run-f plus the latest one: q1 2025-03-05..06-05, q2 2025-06-06..09-05, q3 2025-09-06..12-03, q4 2025-12-04..2026-03-04, w2 2026-03-05..06-06, w1 2026-06-07..09-16, h 2026-09-17..10-17.

For a window starting at S: `ENSEMBLE_TRAIN_TILL=S-1`, embargo 10 as in the recipe (the last available row has `date < S-20`). Variant N takes only rows with `date >= (S-20)-N`; the base (all) takes everything from 2024-01-01. N = 10, 30, 90.

Models: the full ensemble and the logistic member only (`-ensemble-member logistic`). Replay: 17 tickers, deposit 1M, 15000 RUB per position, costs 0.0005×3, kill-switch.

## Criterion (before the run)

Variant N is better than the base of the same model class if all of these hold: (1) the sum of realized P&L over the 7 windows is higher than the base; (2) N beats the base in at least 5 of 7 windows; (3) the number of windows with negative realized is not greater than the base's. The windows and models are dependent, n=7, so passing only earns the right to become a candidate for the daily gate, not a replacement of the model. Realized and P&L with open positions are both printed; realized decides (AGENTS.md invariant).

## Result (2026-10-18)

Per-window summary: `20261018-recent-window-training-summary.csv`. Sum of realized over the 7 windows, RUB (in parentheses: wins over the base all / windows with negative realized):

| training window | ensemble | logistic |
|---|---|---|
| all (from 2024-01) | +67 916 (-/4) | +27 977 (-/5) |
| 90 days | +54 475 (5/7, 3) | +26 163 (4/7, 3) |
| 30 days | -10 906 (3/7, 3) | +8 302 (3/7, 4) |
| 10 days | -68 767 (3/7, 4) | -71 382 (5/7, 6) |

The criterion is not met by any variant: by sum the base all is better (condition 1 fails for all six variants). Short windows produce 2-3 times more trades (q4: 129 for all against 346-381 for 30/10) and worse P&L with open positions and drawdown (up to 7% for the ensemble on 10 days). Training rows: 30 days about 300-430, 10 days 72-134.
