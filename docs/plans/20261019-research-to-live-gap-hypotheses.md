# Research-to-live gap: pre-registered hypotheses H-R2..H-R9

Plans are English-only (owner decision 2026-10-19): this file has no `.ru.md` twin.

Status: PLANNED. Criteria are fixed before any run. Nothing here touches the live path, `config.sandbox.yaml`, `ensemble_model.json`, `data/ridge_scores.csv` or `./trader`. Numbers go into `docs/metrics.md` in the same commit as the result (AGENTS.md rule 8).

Origin: a two-agent review on 2026-10-19 (Claude plus opencode, each claim checked against code and `docs/metrics.md`). The live signal since 2026-10-19 is the ridge long-short score table (`model.score_table_path`, `internal/model/scoretable.go`, 21-bar horizon, refit every 63 days). Earlier decisions (hold until flip, `target_notional` 15000, caps 90000, deadband 0.05) were made for the 10-day ensemble.

## Facts the plan stands on

1. `scripts/ridge_probs.py` writes `p = 0.5 + (r - 0.5) / 3`, so `p` lies in [0.333, 0.667] and the live thresholds `p <= 0.40` / `p >= 0.60` are `r <= 0.2` / `r >= 0.8`, where `r` is the percentile in the whole eligible universe.
2. Research (`docs/metrics.md` section 8.6, step 25): ridge long-short quintile, net +1.73% per monthly period, NW t 2.54, on the full universe with full deployment. Live replay (section 8.8, step 27): 17 tickers, 15000 RUB per position, caps 90000, realized sum of 34 windows +10171 RUB, about 300 RUB per window on a 1M deposit.
3. The key rate carry on 1M for 90 days is of the order of 34000 RUB (section 8.12). The book is smaller than carry by about two orders of magnitude, so "beats carry on the whole deposit" is decided by book size, not by the signal. The original H-R1 is withdrawn for this reason.
4. The 34 rolling 90-day windows of step 27c start every 15 days and overlap sixfold: about 6 are independent. Every window-share criterion and every date-bootstrap on them is invalid.

## Common rules

- Primary test: the non-overlapping 90-day windows of W2 (2025-03-05..2026-10-18, 6 windows). The 34 windows are descriptive only and carry a block bootstrap with block length >= 90 days. Low power at 6 windows is a stated limitation, not a defect to hide.
- Carry: the actual key rate by date from `data/macro/keyrate.csv`, forward-filled to the decision date with a one-day lag, `rate / 100 / 365` per calendar day. Carry is accrued on deployed capital for the comparison in H-R9 and on free cash only when a total-return figure is reported; one formula per report, stated in the report header.
- Borrow rates reported separately: 0.00005 per day (accepted stress) and 0.00047 per day (REPO estimate, about 17% a year, section 8.12). The tariff itself is not measured; this is a stated limitation. The verdict is taken at the worse of the two.
- Multiple comparisons: a Bonferroni correction on the number of arms of each hypothesis.
- Money is `decimal.Decimal`; new code is Go, stdlib `testing`, no new dependencies, no code comments.

## Order

H-R8 and H-R9 in parallel, then H-R45, then H-R2, then H-R3 (only on a rung where H-R9 shows a positive excess over carry), then H-R6 (only if H-R9 leaves a gap that truncation does not explain), H-R7 anywhere after H-R8.

## H-R8. Placebo check first

Defined in `20261019-placebo-check.md`; criteria there are unchanged. H-R3 does not start before its verdict.

## H-R9. Attribution of the research-to-live gap

Question: where does the roughly 20% a year of step 25 go on the way to the live book?

Method: take the stored step-25 ridge scores of the full universe and restrict them without refitting. Rungs, in order: (1) 719 securities to the liquid point-in-time set of `cmd/tqbrsnap` (turnover >= 10M RUB per day, no survivors of today's list); (2) that set to the 17 live tickers; (3) full equal-weight deployment to a fixed 15000 RUB per ticker; (4) monthly rebalance at the bar close to the live rule with deadband 0.05; (5) no caps to caps of 90000. At each rung the report gives net return, excess over carry per ruble of deployed capital, closed trades and the share of the previous gap removed.

Result (2026-10-19): `docs/metrics.md` section 8.13; tool `cmd/gapattrib`. Rungs 1-3 are done in research form; rungs 4-5 (deadband, caps through the backtest engine) are still open.

Criterion: an attribution table, no pass or fail. A rung that removes more than half of the gap is the target of the next hypothesis. If rung (3) explains the gap, the loss is scale and not signal, and H-R3 moves ahead of H-R2 and H-R45. A gap not explained by truncation opens H-R6.

## H-R45. Conditional short

One axis: the short threshold in `r`. Arms `r <= 0.20` (live base), `0.10`, `0.05`, `0` (no short book). Nothing below 0.05.

Criterion, on the non-overlapping windows and on W2, at the worse of the two borrow rates: (a) at 17% a year an arm is not worse than the base in sum and in the worst window; (b) the 0.20 arm at 17% is not worse than the base at 1.8%; (c) only the direction "less short is better" is tested. Bonferroni on three comparisons (CI 98.33%). An arm with fewer than 30 short entries per window is "untested", not "failed". Outcome "short is partly working" is a result; nothing goes to the config without a separate owner decision.

## H-R2. Execution for a 21-bar signal

Two interventions, a 2x2 ablation: entry at the next bar open instead of the decision-bar close; exit when the score leaves the top or bottom third in addition to the 0.60/0.40 rule. The deadband 0.05 is the current base; 0.3 and 0.5 are separate single runs outside the ablation, with no joint verdict.

Criterion: paired difference per window against the base in each cell, CI 97.5% (two comparisons). A factor is adopted only if significant alone; the second factor then stays "not tested", not "rejected". A joint verdict needs both factors significant and in the same direction. Closed trades must not rise.

## H-R3. Risk budget and breakeven scale

Only on a rung where H-R9 shows a positive excess over carry per ruble of deployed capital. Grid `target_notional` 15k / 30k / 45k with the same 3% kill and caps. Criterion: at least one variant is not worse than the base in realized, has drawdown <= 3% and the kill never fires in three non-overlapping 90-day windows; the report also gives the breakeven scale (the multiple of the book at which the absolute gain over carry turns positive at drawdown <= 3%). If none qualifies, the answer is "the book is too small", which is a result.

## H-R6. Universe expansion after the capacity diagnosis

Only if H-R9 leaves a gap that truncation does not explain. The step-25 ladder (t >= 2.6, Sharpe >= 0.5, >= 60% positive years, post-2022-06 > 0) on the point-in-time universe of `cmd/tqbrsnap` with a turnover filter of >= 10M RUB per day.

## H-R7. Exit by decision, not only by threshold

`ScoreTableSource` returns HOLD for `0.40 < p < 0.60` and for a stale row (`scoreTableMaxStale` 7 days), and a held position is not recomputed on HOLD. Arms: a position age limit of 21 and of 42 bars; a stale-guard that flattens a position whose row is stale. Criterion on the non-overlapping windows: the realized sum is above the base, drawdown not higher, closed trades not more. Otherwise "the flip is a fact about the signal, not about execution".

## Closed, not to be repeated

Inverse-volatility sizing (Experiment D, H23 R1, H21 stage 0); a refusal meta-model (step 13); news as signal or exit (step 2g, H22, H1, H4); leverage and `max_net_exposure` (`docs/knowledge/income-levers.md`); take-profit, stops and partial exits (H20 and its extension); time exit on the 10-day model (2026-09-17); new features, macro, factors, pairs (macro, steps 18, 23, 24, 26, 28, 29); a short leg without the borrow rate (step 25: t 2.54 to 1.42 at 10.9% a year).
