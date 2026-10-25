# News impulse: attention and sentiment shocks as short-horizon trend signals (2026-10-25)

Status: REGISTERED 2026-10-25 (English only), before the run. Item 7 of the owner's order of work ("impulse trend models from news"). Continues the earlier news results of `docs/knowledge/model-quality-history.md` (the daily news score: gross rank spread +0.21% at 5 and +0.43% at 10 days, concentrated on the negative side, no net edge after 0.60% tranche cost).

## What is new compared with the earlier news score

The earlier score worked on a sentiment level. Here the signal is a shock in the event window around the decision, with the continuation question asked at horizons of one day (the tradable impulse), five and ten days, and a separate row for news that arrives after the close and is tradable only at the next open. The data are raw news headlines (not regenerated), so the price-based rows use 2022-01-03..2026-10-16, with a first half (2022-2024) and a second half (2025-plus); only the flow-interaction rows (which need A2) use the realtime window.

## Data and signals (causal, fixed ex ante)

`data/news_history.jsonl`: 155,601 ticker-tagged headlines, roughly 24-25 thousand a year in 2023-2024 and 40-43 thousand a year in 2025-2026, so the count signals are standardised against the ticker's own trailing 20 trading days. Matrix: the point-in-time top-100 (`work/matrix.csv`) with its side file for the next day's open-to-close return (`idy1`). Information cut: 18:30 Moscow; a headline belongs to the first trading date whose 18:30 is not earlier than its timestamp, so the day-d signal uses headlines between 18:30 of the previous trading day and 18:30 of d. Headlines after 18:30 of d and before 09:50 of the next trading day form the "evening" bucket of d.

- **Count shock** `zc`: (count - mean of the previous 20 dates) / sqrt(mean + 1). **Trust-weighted count shock** `zt`: the same with the `trust_weight`s summed.
- **Lexicon sentiment** `sent`: for each headline, (positive stems - negative stems) from a fixed Russian stem list (positive: growth, profit, record, dividend, raise, increase, buyback, contract, approval, expansion, improvement, purchase, strong; negative: fall, cut, loss, sanctions, fine, lawsuit, cancel, default, bankruptcy, investigation, suspension, reduction, downgrade, risk, accident, crash, loss; the exact stems are in `cmd/newsimpulse/news.go`), summed with the trust weights and divided by (count + 1). The list is fixed and crude on purpose: it is a lexicon, not a model; a better sentiment model (the local LLM scorer) is a separate, costlier step reserved for a lead.
- **Evening** signals `eveSent`, `eveZ` (count shock of the evening bucket against the trailing 20 dates of evening counts): outcome the next day's open-to-close return (an order sent at the open).

## Rows (17 plus 5 controls)

| Id | Row |
|---|---|
| N01 h1, h5, h10 | count shock `zc` against the abnormal return (return of the name minus the universe mean) over 1, 5, 10 days from the close of d |
| N02 h1, h5, h10 | trust-weighted count shock |
| N03 h1, h5, h10 | lexicon sentiment |
| N04 h1, h5, h10 | sentiment shock times A2 (product of the centred ranks; confirmation by order flow), realtime |
| N05 h1, h5, h10 | count shock times A2, realtime |
| N06 | evening sentiment against the next day's open-to-close return (cross-sectionally demeaned) |
| N07 | evening count shock against the same outcome |
| C01-C05 | controls: the N01 h1 signal permuted across names within each date (five seeds) |

Statistic: the daily rank (Spearman) correlation between the signal and the outcome across names (at least 30), the mean over dates with a Newey-West t (lags h + 5), the two halves. **Hit**: |t| at least 3.0 (the project floor), the same sign in both halves as the mean, and no placebo hit (one control hit voids all hits). The effective number of distinct signals is seven; the threshold is the floor, because the horizons of one signal are nearly the same hypothesis.

## Honest expectations

The lexicon is weak and headline-level news is mostly priced within minutes; the realistic prior of a day-1 continuation at the close-to-close horizon is low. The row with the best chance is the evening bucket (N06, N07): information that arrives while the exchange is closed and is partly reflected at the open, and an open-to-close outcome removes the gap that cannot be traded at the previous close. Prior of any hit: low; an IC of 0.02 with t above 3 over 1,200 dates is the size the data can detect.

## What this does not test

Intraday reaction (minutes after publication) is not tested: it is forbidden scalping territory for this project and needs timestamps accurate to the minute, which headline aggregators do not guarantee. The LLM sentiment is not used. Names outside the top-100 are not covered.

## Run order

1. Reviewer reads this file and `cmd/newsimpulse`.
2. One run on the ai-box (matrix and side file are there; the news file is copied once).
3. Numbers into `docs/metrics.md` 8.32 and `docs/knowledge/model-quality-history.md`.

## Review log

(pending the Reviewer's pre-run pass)

## Errata

(none)
