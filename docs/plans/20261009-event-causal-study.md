# Pre-registered protocol: causal event study of news topics (DiD / stacked DiD / TWFE / AIPW)

Status: EXECUTED 2026-10-09 (protocol written before the first run). Results: `docs/metrics.md` "Event causal study".
Tool: `cmd/eventstudy` (net-new). Result goes to `docs/metrics.md` in the same commit.

## 1. Question and non-goals

Do news topics that the repo already detects (`features.DetectEvents`, `features.DetectIncident`)
move the stock's market-adjusted return, causally, beyond what the AUC gate could resolve?
This closes the open item "revisit `negotiations_signal`/`sanctions_signal` after a richer news
backfill" and gives the live `incident` BUY-veto its first return-level evidence.
Non-goals: no change to `cmd/trader`, the model, the feature order or any config. IV is not run
(no valid instrument for a news topic on MOEX; any candidate violates the exclusion restriction).

## 2. Inputs (fixed)

- News: `data/news_history.jsonl` (`model.LoadFinanalysNewsHistory`), records with `trust_weight > 0`,
  ticker in the 17 `config.sandbox.yaml` tickers. Archive span 2026-04-01..2026-10-09.
- Prices: MOEX ISS daily candles via `backtest.NewISSSource`, incomplete trailing bar dropped,
  IMOEX as the market. Estimation window 2025-10-01..2026-03-31 (entirely before the archive);
  study window 2026-04-01..last closed bar.
- Topics: PRIMARY `sanctions`, `negotiations` (DetectEvents) and `incident` (DetectIncident).
  EXPLORATORY `dividend`, `report`, `buyback`, `mna`. IPO/delisting/default are reported as counts only.

## 3. Outcome and event definition (fixed)

- Abnormal return `AR_it = r_it - alpha_i - beta_i * r_IMOEX,t`, close-to-close, alpha/beta by OLS
  per ticker on the estimation window. Robustness: `r_it - r_IMOEX,t`.
- Publication time converted to Europe/Moscow; published at or after 19:00 MSK or on a non-trading
  day maps to the next trading day. Event day t0 = that trading day.
- Event = (ticker, topic) first t0 with at least one flagged article; any further event of the same
  ticker+topic within 5 trading days is dropped (de-clustering). Events need 10 clean prior days.
- Windows: PRIMARY CAR[0,+1] (t0 return includes intraday-priced news by construction).
  SECONDARY CAR[+1,+5] (post-news drift) and PRE-TREND CAR[-5,-1].
- Round-trip cost bar for tradability: 2 x 0.0015 = 0.003 (commission+spread+slippage per leg, as
  in `cmd/dividendstudy`).

## 4. Estimators (fixed)

1. Event-study: mean CAR per window, 95% CI from a bootstrap that resamples event DATES (2000 reps,
   seed 42), because many tickers share a date.
2. Stacked DiD (robust to staggered-timing bias of plain TWFE): for each event a stack of the treated
   ticker plus clean controls (same topic absent in [t0-10, t0+10]); DiD_e = mean_treated(post - pre)
   minus mean_controls(post - pre) over k in [-5,-1] vs [0,+1]; average over events, date-clustered
   bootstrap CI.
3. TWFE: panel ticker x day, `AR_it` on event-time dummies k in {-3..+3} with ticker and date fixed
   effects (alternating projections), SE clustered by date. Reported next to (2) to show the
   TWFE-vs-stacked gap; pre-event coefficients (k<0) are the pre-trend test.
4. Doubly robust AIPW for the ATT of the topic on CAR[0,+1] and CAR[+1,+5]: propensity = logistic on
   X, outcome model = OLS on controls; X = AR_{t-1}, 5d abnormal return, 20d AR volatility,
   IMOEX return_{t-1}, ticker news count over 5 days, weekday. Propensity clipped to [0.01, 0.99].
   Controls exclude ticker-days within +-5 trading days of an own-ticker same-topic event. SE by
   influence function aggregated per date.
5. Placebo: event dates shifted within ticker by a uniform draw in [20, 60] trading days (2000 reps);
   the empirical null of the DiD statistic gives a permutation p-value.
6. Market-level check for macro topics: IMOEX return on days with topic article count in the top
   decile of the study window, versus other days, date-bootstrap CI.

## 5. Acceptance gate (fixed before numbers)

A topic is "supported" only if ALL hold on PRIMARY CAR[0,+1]:
(a) N events >= 30 across >= 10 distinct dates;
(b) stacked DiD and AIPW have the same sign, each CI excludes 0;
(c) Holm-adjusted p < 0.05 across the three primary topics (permutation p from step 5);
(d) pre-trend CAR[-5,-1] CI includes 0;
(e) |effect| > 0.003 (round-trip cost) for it to count as tradable rather than merely detectable.
Failing (a) means "underpowered", reported with the minimum detectable effect (MDE at 80% power from
the observed AR volatility), never as "no effect". Exploratory topics are reported, not gated.

## 6. Known threats, stated up front

- The archive is 6 months, one regime; ticker attribution of generic news is noisy (VKCO is 41% of
  records, documented false positives), which biases effects toward 0.
- News timing: RSS timestamps can lag the first market reaction, so t0 returns may be partly
  pre-news; the SECONDARY [+1,+5] window is the one a strategy could actually trade.
- Sentiment sign is not used: topics are presence flags, so a symmetric effect averages to 0.
  A signed variant is allowed only as exploratory and labelled so.
