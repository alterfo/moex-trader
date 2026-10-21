# Dropping losing tickers: walk-forward and random-drop check, `cmd/tickerselect`

Russian translation: [20261022-ticker-selection-check.ru.md](20261022-ticker-selection-check.ru.md). This file is canonical.

Status: done 2026-10-22; criteria were fixed before the first run. Verdict: indistinguishable from random drops, filter not adopted (`docs/metrics.md` 8.14).

## Question

Does removing the tickers that lost money (or "behaved against the model") improve the live ridge book, once the selection uses only information available at the time?

## Design

The ridge table `data/ridge_scores.csv` (17 tickers) is replayed on W2 (2025-03-05..2026-10-18) through `backtest.NewEngine` with the same book parameters as `cmd/placebocheck`. W2 is cut into consecutive blocks of `-block-days` calendar days (default 91, close to the 63-trading-day ridge refit). Block 0 is used only for training; blocks 1..m are evaluated. Positions do not carry across blocks; every block is an independent engine run, and the statistic is the sum of realized P&L net of borrow over blocks 1..m.

Arms, each dropping `K` tickers (default 4, the bottom quartile of 17) per block:

- FULL: all tickers.
- WALK-FORWARD: drop the K tickers with the lowest per-ticker net P&L (sum of closed `Trade.NetPnl`) in block j-1, trade block j. This is the only honest arm.
- IN-SAMPLE (leak, descriptive): drop the K worst of block j itself, trade block j. It shows how much of the apparent gain is pure selection bias; it is never a result.
- RANDOM: N = 200 draws, each drops K random tickers independently per block; the draw's total is summed over blocks. Seed 20261022 + draw number.

Diagnostic without a criterion: the mean Spearman rank correlation of per-ticker P&L between block j-1 and block j (persistence of "losers"), and how often a dropped ticker was in the bottom K of the next block.

Borrow rates 0.00005 and 0.00047 per day, as in the placebo check.

## Criterion (before the run)

For each rate, one-sided empirical p = (1 + number of random-drop totals not below WALK-FORWARD) / (N + 1).
- "Ticker filter works": WALK-FORWARD > FULL and p <= 0.05 at 0.00047.
- "Limited by the rate": the same holds only at 0.00005.
- Otherwise: "dropping losers is indistinguishable from dropping random tickers"; the filter is not adopted.
Caveats: K, block length and the selection metric are chosen by the author before the run, once, and are not tuned afterwards; with m of about 5 blocks the power is low, so a negative result means "not shown", not "proved useless". The result is not a gate and does not touch the live path.

## What we do not do

No change to preflight, the live path, config, `ensemble_model.json` or `data/ridge_scores.csv`; no new dependencies; flags only.
