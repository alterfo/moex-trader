# News impulse: attention and sentiment shocks as short-horizon trend signals (2026-10-25)

Status: DONE 2026-10-25 (English only; scored once, a decaying reversal lead after news bursts, not an impulse), registered before the run. Item 7 of the owner's order of work ("impulse trend models from news"). Continues the earlier news results of `docs/knowledge/model-quality-history.md` (the daily news score: gross rank spread +0.21% at 5 and +0.43% at 10 days, concentrated on the negative side, no net edge after 0.60% tranche cost).

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

## Result (scored once, 2026-10-25)

Numbers in `docs/metrics.md` 8.32, output `artifacts/algopack-news/newsimpulse.txt`. **7 hits of 17 rows by the registered rule, 0 of 5 controls**, and the sign is the opposite of an impulse: a news-attention shock predicts REVERSAL. Count shock (N01) against the next-day abnormal return: mean IC -0.0245 (Newey-West t -6.3, 1198 dates), h5 -0.0214 (t -3.8), h10 -0.0251 (t -3.6); the trust-weighted version (N02) the same (t -6.5, -4.0, -3.8); both halves negative in every horizon but weaker in 2025-plus (h1 -0.034 then -0.009, h5 -0.033 then -0.003, h10 -0.039 then -0.003). N04 (sentiment x A2) at h5 -0.0193 (t -3.4, halves -0.014 / -0.026) is the seventh hit. Descriptive deciles of the count shock (added after the hits were seen, not a condition): the top decile earns an abnormal return of -0.127% over the next day (t -5.1), -0.209% over 5 days (t -3.8), -0.254% over 10 days (t -2.7); the bottom decile +0.039% / +0.076% / +0.173%; top minus bottom -0.17% / -0.29% / -0.43%. Not hits: lexicon sentiment (N03: IC +0.007 to +0.010, t +2.2, +2.2, +1.5; the lexicon is crude), count shock x A2 (N05: t +2.4, -0.4, -0.7), evening count shock (N07: t -1.1). N06, evening sentiment against the next open-to-close return, has IC +0.0223 (t +5.9) but flips sign between the halves (+0.040 in 2022-2024, -0.004 in 2025-plus), so it fails the registered both-halves condition. Registered reading: **a lead, decaying**: news-driven attention is followed by an abnormal return that is negative on average (reversal of the news burst, about -0.13% the next day for the top decile), strong in 2022-2024 and about a quarter of that size in 2025-plus. Economics are not tested: as a short it faces the broker carry (0.074% per calendar day) and spreads; as an avoid-filter on the long leg of the composite it is a candidate overlay, which would be a separate step.

## Review log

Pre-run pass (opencode, 2026-10-25): timestamp assignment, evening bucket, standardisation, A2 product, dividends, duplicates and outcome columns verified; the rule is fair; two amendments and four pins applied (errata).

## Errata

Written 2026-10-25 after the Reviewer's pass, before the run.

1. Lexicon noise: three stems are false-positive magnets: the negative stem for "fall" also matches the Russian words for "West" and "reserves", the negative stem for "loss" also matches "consumer", the positive stem for "growth" also matches "simple". A crude lexicon pushes the sentiment rows toward zero and cannot manufacture a hit; a null sentiment row is therefore evidence about this lexicon, not about sentiment, and a hit that survives it is stronger evidence.
2. N06 measures a level (the trust-weighted score per headline of the evening bucket) and N07 a shock (the evening count against its trailing 20 dates): they are different hypotheses, not one at two scales. The `sent` rows are also levels, not standardised.
3. Weekend headlines after Saturday 09:50 fall into Monday's day bucket: the registered semantics, not a leak. The cross-sectional demeaning of the open-to-close outcome is implicit in the rank IC (invariant to a per-date constant); the code uses the raw `idy1`.
4. N04 and N05 run on about 450 realtime dates, so the same floor needs a larger IC; their halves are cut at 2026-01-05.
