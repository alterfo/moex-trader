# Execution timing analytics: when is a ticker cheapest to trade (2026-10-25)

Status: REGISTERED 2026-10-25 (English only), before the run. Item 4 of the owner's order of work; the product-side question behind "can we sell a subscription for 1000 roubles instead of 610": not an alpha claim but a measurable, deterministic saving for anyone who trades.

## Question

Every earlier result says that cost, not signal, decides the outcome (the quoted half-spread is 1.8 bp for the tightest third of the top-100 and 9-45 bp in thin names, per side, on top of 0.05% commission). The intraday spread has a shape (wide at the open, tight in the middle of the day, widening toward the auction). Hypothesis: the shape is stable enough per ticker that the window predicted to be cheapest from the previous 60 days is cheaper the next day than a naive choice (the open, the close, or an average time), and the saving is a few basis points per side for liquid names and more for thin ones. This is a measurement of a costless improvement in execution; it makes no statement about direction.

## Data and definitions

AlgoPack `obstats` 5-minute bars (`spread_bbo`, basis points of the quoted best bid-ask spread), realtime rows only (SYSTIME within 30 days of the trade date), feature dates 2025-01-03..2026-10-23, main-session bars labelled 10:05 to 18:30 grouped into 17 consecutive 30-minute windows from 10:05; a window needs at least 3 bars and a ticker-day at least 14 valid windows. A ticker needs at least 300 such days. Liquidity thirds by the ticker's median daily-mean spread over the sample (a stratification label, not an outcome). The predicted-best window of day d is the window with the lowest median spread over the previous 60 ticker-days (at least 30 observations per window), computed only from days before d. Savings are half the difference in spread (one side) between the benchmark window and the predicted-best window on day d itself, in basis points of price.

## Rows (16 plus a control)

| Id | Row |
|---|---|
| E1 (all, T1, T2, T3) | persistence: Spearman correlation between the trailing 60-day median profile and the day's profile, averaged across tickers each day, mean over days |
| E2 (all, T1, T2, T3) | half-spread saved per side by the predicted-best window against the session average |
| E3 (all, T1, T2, T3) | the same against the naive opening window (10:05-10:35) |
| E4 (all, T1, T2, T3) | the same against the naive closing window (18:05-18:35) |
| C1 (all, T1, T2, T3) | control: the best window predicted from another ticker's trailing profile (the next ticker in the liquidity order), against the session average |

Statistic: the daily cross-ticker mean, then the mean over days with a Newey-West (lag 5) t and the two halves of the sample. **Verdict**: E1 positive, E2-E4 positive with |t| above 3.0 in each of the four groups and positive in both halves; the control C1 must be at most zero or indistinguishable from zero in every group (a positive control saving would mean the saving comes from the average shape of all profiles rather than from the ticker's own history, which would make the "personal" window no better than a universal time rule, and the plan then reports the universal rule as the product). Family of 16 rows, effects expected to be large relative to noise, so no multiplicity adjustment beyond the 3.0 floor.

## Honest expectations

Spread shapes are known to be stable (wide at the open and the close, tight in the middle), so the persistence is expected to be strongly positive and the saving against the open and the close large; against the session average the saving is expected to be small (a few tenths of a basis point for the liquid names), and the control, which borrows another ticker's shape, is expected to also be positive because the common intraday shape is shared. The informative number is the personal window minus the universal window, not the saving against the open. An honest product statement needs both.

## What this cannot show

Quoted spread is not the whole cost of execution: price impact of the order, queue position and the fact that an order cannot choose the time of the signal. It applies to flexible-timing orders (the next-day rebalancing of a daily book is such an order); it is not a claim about alpha, and the savings are in basis points, to be set against the 0.4-1% round-trip costs of the strategies tested in this study.

## Run order

1. Reviewer reads this file and `cmd/exectiming`.
2. One run on the ai-box over the obstats files.
3. Numbers into `docs/metrics.md` 8.31 and `docs/knowledge/model-quality-history.md`.

## Review log

(pending the Reviewer's pre-run pass)

## Errata

(none)
