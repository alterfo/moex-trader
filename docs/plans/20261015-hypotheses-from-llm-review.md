# Hypotheses to test from the LLM forecasting review (2026-10-15)

Russian translation: [20261015-hypotheses-from-llm-review.ru.md](20261015-hypotheses-from-llm-review.ru.md). This file is canonical.

Source: Shashkov K.A. A review of world experience in applying large language models to financial forecasting // ETAP, 2026, no. 3, pp. 81–96. This is a secondary review: the primary papers (Lopez-Lira & Tang, Chen–Green–Gulen–Zhou, LLMFactor, Quantifying Qualitative Insights, TrustTrade and others) were not read, and the references in the hypotheses must be checked against the originals before work starts.

Constraints that apply to all hypotheses:
- The LLM stays off the trading path, except the short veto (decision of 2026-09-15 and the exception of 2026-10-15). Hypotheses where an LLM produces an entry signal are tested offline only; enabling one in the trader requires new consent from the owner.
- The threshold and the metric are fixed before the run. Without 200 events and 40 dates there is no verdict (as in `cmd/challengercompare`).
- Metrics go into `docs/metrics.md` in the same commit.
- Model quality on Russian headlines does not transfer automatically from English results.

## Priority A: cheap and testable on the data we have

**H1. The reaction to news decays with the horizon.** In the review, LLM signals are strong at short horizons and weaken over days and weeks. Test: on the news archive scored by `cmd/newsscore`, compute the mean excess return against IMOEX at horizons of 1, 2, 5, 10 trading days after the first news item of an episode. Criterion: if the effect at the 5-day horizon is not above zero with a confidence interval, news positions must not be held longer than 2 days, and `exit_max_hold` must be reduced. Tool: `cmd/eventstudy`.

**H2. Zero signal on Russian headlines.** A zero-shot LLM direction forecast in Russian may be noise (our directional AUC of the ai-box model is about 0.5). Test: qwen3.8 zero-shot predicts the sign of the reaction from the headline; compare it with the realized next-day return. Criterion: AUC above 0.55 with the lower bootstrap bound above 0.5 on 200+ events. Otherwise the hypothesis is closed and the LLM stays a veto only.

**H3. Future leakage in qwen3.8 (lookahead).** The model may have seen the outcomes of old events. Test: split the archive into news before and after the qwen3.8 training cutoff date and compare the veto quality and H2 on the two parts. Criterion: if quality before the cutoff is noticeably higher, results on the old archive cannot be used for calibration.

**H4. The effect is stronger for small caps and negative news.** Lopez-Lira & Tang: predictability is higher for small companies and for negative news. Test: split the H1 events by liquidity (ticker turnover) and by sentiment sign. Criterion: the effect in the lowest turnover tercile and on negative news is significantly higher (difference with a bootstrap CI). This matches the fact that the trader shorts on an incident.

**H5. The LLM veto does not just copy recent price dynamics.** Chen–Green–Gulen–Zhou: LLMs overweight recent performance (extrapolation). Test: on the `incident check` verdict log, see whether `damage`/`no_damage` correlates with the ticker's `Mom5d` and `Mom21d`. Criterion: a correlation above 0.3 means the veto mixes in price information rather than the meaning of the news, and it must then be checked blind, without the price in the prompt (there is no price in the prompt now, but the wording of the headline may carry it).

## Priority B: need labeling or a new feature

**H6. Structured LLM scores as ensemble features.** LLMFactor and Quantifying Qualitative Insights: the LLM produces numeric factor scores (impact on the company, sector background, urgency) and a classical model uses them. Test: obtain 3–4 scores per archive headline offline, add them as features to `cmd/trainmodel`, compare walk-forward AUC and realized P&L with the baseline. Criterion: an AUC gain above 0.01 on all walk-forward windows, otherwise do not enable. Condition: the features are computed offline and cached, the LLM does not enter the live path, so live use would require new consent.

**H7. Text quality filter (duplicates, spam, reprints).** Chen et al. 2025: LLM filtering of comments raises the profit-to-loss ratio. Test: label Telegram sources and duplicates with an LLM classifier offline and see how many false `incident_min_sources` come from reprints of a single source. Criterion: the share of episodes where two sources are in fact one is above 20%. This is compatible with the veto, because the filter only removes.

**H8. A veto without LLM hallucinations: consensus of independent sources (TrustTrade).** The weight of a signal depends on the independence and agreement of the sources, not on their count. Test: replace the `incident_min_sources` count with a weighted one (source duplicates count zero, a conflict between headlines lowers the weight). Criterion: fewer false entries on refuted incidents (the list from SBER Balakovo and similar) without losing true ones.

**H9. Prior from the ensemble plus a news shift (Market-Conditioned Prompting).** Give the LLM the model's prior probability explicitly and ask it to update it from the text. Test: offline, as H6; compare calibration (Brier) with the ensemble without news. Criterion: Brier improves significantly. It must not be used in the trader without a new decision of the owner.

## Priority C: ideas for later, low odds

**H10. Volatility sets the look-back threshold.** Implemented as `news.lookback_vol_mult` (off). Test: tune the multiplier on an event study with thresholds set in advance. Criterion: the share of played-out moves where the price kept going after entry is significantly lower at the same number of trades.

**H11. Emergency mode on a sharp market drop (HedgeAgents, "extreme conference").** When IMOEX falls by more than a set threshold in a day, reduce position size and forbid new buys. Test: backtest on 2022 and 2024–2026. Criterion: a smaller drawdown with at most 20% loss of return.

**H12. Signal decay as LLMs spread.** Lopez-Lira & Tang: returns of LLM strategies decline over time. Test: compute the quarterly rolling quality of the news entry and alert if it falls. This is not a return hypothesis but monitoring: it is needed before any LLM signal is enabled.

**H13. Time series as tokens (StockTime, foundation models).** Low priority: the overfitting risk is high and the gain is unclear on daily data for 20 tickers. Take it only after H1–H5, if a reason appears.

## Order

1. H1 and H4 together (one event sample), then H5 and H3.
2. H2, if H1 gives an effect.
3. H7 and H8 as improvements to the veto.
4. H6 and H9 only if H2 shows that the text carries a directional signal.

The article "Comparative analysis of AR-GARCH and p-adic methods" (2019) did not make this list: its conclusions are insignificant and were not checked out of sample.

## Addendum: the review by Darwish, Hassanien, Eissa (2026-10-15)

Source: Darwish M., Hassanien E. E., Eissa A. H. B. Stock Market Forecasting: From Traditional Predictive Models to Large Language Models // Computational Economics, 2026, 67, 4553–4597. This is also a secondary review: the results of individual systems (FinMem, FinAgent, FinGPT, BloombergGPT, "+36% profit" for FinAgent) are taken from it, the primary papers were not read, and most of the described works have no costs and no future-leakage check. We take only section 6 (directions) and what can be tested on our data. The constraints from the top of the file apply here too.

Already done as a result of the same checks: the `newssent` dashboard shows the accuracy of LLM forecasts separately for the 1h, 4h and 24h horizons (before, the horizons were mixed and the tile understated the result). Rule for the future: any direction metric names its horizon and matches the horizon at which the strategy actually holds the position.

### Priority A

**H14. Holding for 18 hours may be harmful (newssent, paper signals).** Measuring news against random moments of the same hour (`cmd/impact` in `newssent`) showed that the excess move after news is concentrated at short horizons and almost vanishes by a day, and the LLM direction at 24 h is no better than random. Paper signals are held for 18 h. Test: from `data/shadow/outcomes.jsonl`, compare the z in the signal direction at 4 h and at 18 h, including skipped ones. Criterion: if the difference z18 − z4 is negative with a bootstrap CI by dates, shorten the holding. The threshold and the metric are fixed before the run, the verdict only with 200+ events and 40+ dates. This is the `newssent` repository; do not touch `moex-trader`.

**H15. Disagreement of ensemble members as a reason to abstain** (section 6.2, uncertainty-aware prediction). Today the decision is made on the ensemble probability 0.60/0.40. Test: on walk-forward forecasts, split the trades into segments by agreement of lgbm, xgb and logreg and compare realized (not MTM) P&L per trade across segments. Criterion: the disagreement segment is worse than the agreement segment with a bootstrap CI by dates on 200+ trades. If there is no difference, close the idea. This is the ensemble and model zone; agree with the owner before editing.

**H16. Transfer to tickers the model has not seen** (section 6.1, cross-market generalization at our scale). Test: purged walk-forward where the ensemble is trained on some tickers and tested on the others, against the usual split by time. Criterion: if the AUC on held-out tickers is not lower than the usual one, tickers can be added without retraining for each; if it falls to the noise level, the model memorizes the ticker, and the list cannot be extended without retraining. The same zone and agreement procedure as H15.

### Priority B

**H17. Versioned prompt variants** (section 6.10 and Fatouros et al. 2023: zero-shot against template examples). There is no single prompt standard, and LLM answers to the same text are unstable. Test: 3 variants (zero-shot, with template phrases, JSON with a key sentence), the prompt version is written to the logs, comparison by the sign of the 4 h reaction and calibration on labeled `newssent` pairs. Criterion: a gain in sign accuracy with the lower bootstrap bound above zero on 200+ pairs. The metric names its horizon (see above).

### Priority C

**H18. Dynamic ensemble weights by market regime** (section 6.8). High overfitting risk with 18 tickers and a base AUC near noise. Test only with nested walk-forward, regime weights chosen on the inner part. Do not enable without a clear gain.

**H19. A regime-shift detector as an extension of H12.** Feature drift (PSI) and the per-ticker circuit breaker are allowed by `AGENTS.md`. Add an online shift detector for the distribution of the reaction to news (the share of significant moves against placebo) as an alert, not as a trading signal.

### Not taken

- Agent systems FinMem, FinAgent, FinGPT, BloombergGPT and RL/MARL: the claimed results have no costs and no leakage check, and fine-tuning a financial LLM runs into the shared GPU on the ai-box, which ComfyUI and TTS also use.
- Multimodal charts as input: there is no evidence that they add anything to price features on daily bars.

Conditions for revisiting this list: agent systems and RL return to consideration if a reproducible evaluation with costs and without leakage on the Russian market appears, or the GPU is freed for a separate task. Multimodal charts return if price features on daily bars begin to give a stable AUC above 0.55 and the question remains what to strengthen it with.

## Owner decisions

**GNN, LSTM and graph networks are not taken as the basis of the model (2026-10-15).** The occasion was a question about the LSTM and graph networks claimed by Sber (78% accuracy without a definition of the metric, horizon, costs and sample), and an article on graph diffusion on a Riemannian manifold (tested only on EEG, with no financial data and no trading evaluation). Reasons: the base AUC of the ensemble is near noise, and model complexity does not cure the absence of signal; there is little data (18 tickers, daily bars), the overfitting risk is high; links between instruments were already tried with simpler methods (cross-sectional ranking closed by the decision of 2026-09-17, there are `cmd/leadlag` and `betaregime`); training needs a GPU, which the ai-box shares between ComfyUI and TTS.

Condition for revisiting: an external source appears with a defined metric, costs and spread, or the challenger in shadow mode (`model.challenger_ensemble_path`, `cmd/challengercompare`) shows a realized, not MTM, gain with the lower CI bound above zero at 200+ pairs and 40+ dates. Without that, do not open the direction.

## H20. Stops and take-profits at levels (event study, 2026-10-15)

Idea: stop and take-profit orders accumulate at conspicuous levels (round numbers, recent extremes), so the price behavior after crossing such a level should differ from crossing an "ordinary" level at the same distance. We cannot see the orders, so only the footprint in prices is tested. The study runs offline in `newssent/cmd/levelstudy` (ISS 10-minute bars, cache `data/candles2025`); nothing enters the trading path. Results are written to `docs/metrics.md`.

Definitions fixed before the run:
- Universe: 16 tickers of the `market.BasketTickers` basket, period 2024-11-15..2026-10-11, events only in the main session, the post-event window entirely within the same day.
- Event: the first crossing of a level in a day by the close of a 10-minute bar, with the previous close of the same day on the other side (gaps across the level excluded). Direction d = +1 up, −1 down.
- Real levels: (a) the extreme of the previous 20 trading days by closes (maximum for an upward break, minimum for a downward one); (b) a round number with step 10^(floor(log10 price) − 1).
- Placebo at the same distance and the same frequency: (a) the fourth-largest extreme of the same 20 days; (b) a grid shifted by half a step.
- Outcome: signed excess return against the basket (beta over the whole sample), normalized by the hourly sigma, at horizons of 10 minutes, 1 hour and 4 hours. A positive value means continuation, a negative one means a return. Additionally the share of returns through the level within 30 minutes.
- There are two primary tests: (a) and (b) at the 1-hour horizon, both directions together, the difference between real and placebo, bootstrap confidence interval by dates 97.5% (correction for two tests). The other horizons and cuts are descriptive.
- The effect is accepted if the 1-hour difference has a confidence interval without zero, the same sign in the first and second half of the period, at least 200 events in each branch and at least 40 distinct dates.
- Economic threshold: the mean signed excess return of the real level must be above 30 bp (full round trip, i.e. entry and exit, with commission, spread and slippage of 0.05% per side). An effect below the threshold is recorded as statistical but unusable for trading and is good only as a protective filter.

Limitations: no order book and no order history; levels by closes, not by High/Low (the cache has only Open and Close); gaps excluded; beta and hourly sigma estimated over the whole sample; the basket is liquid, on mid-cap names the effect may be stronger but the spread eats it.

What to do with the result: if confirmed, use it only as protection (do not place our own stops and entries right next to round levels and recent extremes, do not enter at market right before a break) and check where the stops are in `internal/executor` and `internal/risk`. Do not use it as an entry signal source without a separate decision of the owner. The link with the reversal after news (a move through an extreme or a round number against one without) is to be checked only after that and in a separate run.

Result of H20 (2026-10-15): closed as a trading signal, numbers in `docs/metrics.md`, section "Level-crossing event study". The primary test on extremes formally passed (confidence interval without zero, both periods of the same sign), but the mean return of the real level is an order of magnitude below the economic threshold, and the difference comes from a negative placebo rather than from the level itself; no effect for round numbers; no sign of a stop run (the share of returns through the level). A protective filter around our own stops and entries is not supported by the data. The link with the reversal after news was not run. Not tested: order book and order history, fine roundness of levels, the 200-day moving average, illiquid names.

### H20, extension (criteria fixed before the run, 2026-10-15)

Order book and order history: there is no historical data. The AlgoPack token (`MOEX_TRADER_ALGOPACK_TOKEN`) is not set, public ISS gives only the current snapshot, and the T-Bank broker order book is streaming without history. The item is not tested until an AlgoPack subscription (`obstats`, `orderstats`) appears or our own order-book snapshots are collected.

New tests, a single protocol: the same definitions of event, outcome and placebo as above. A family of six primary tests (multiplicity correction, 99.2% confidence interval): fine rounding, the 200-day average, three cuts on illiquid names (extreme, round number, fine rounding), the link with the reversal after news. The other cuts are descriptive only.
- Fine rounding (basket): grid step 10^(floor(log10 price) − 2) (about 0.1–1% of the price), placebo shifted by half a step. Primary horizon 1 hour.
- 200-day moving average (basket): ISS daily closes, the SMA level over the previous days. Real SMA200, placebo SMA173 (no "popular" length). A second descriptive set SMA50 against SMA37. Primary horizon 1 hour.
- Illiquid names: stocks from the candle cache outside the basket with at least 300 trading days of history, ranked by the share of 10-minute bars with zero return; the lowest liquidity tercile. The same level definitions as for the basket (20-day extreme, round number and fine rounding), adjacent bars no more than 20 minutes apart. Economic threshold 100 bp instead of 30 (the spread of illiquid names is 0.5–2%).
- Link with the reversal after news: the first news item of a ticker (the next one no earlier than 24 hours later), a significant move within the hour `|z1h| ≥ 2`. Label: within that hour the price crossed the 20-day extreme in the direction of the move or not. Outcome: continuation in the direction of the move from the end of the first hour to 4 hours, normalized by the sigma of the 4-hour horizon; comparison "crossed the extreme" against "did not cross" adjusted for the size of the move (strata `|z1h|` 2–3, 3–5, 5+), bootstrap by dates. Criterion: interval without zero, at least 100 events in the "crossed" branch and 40 distinct dates, economic threshold 30 bp.

Result of the H20 extension (2026-10-15): none of the six primary tests rejects the null hypothesis, the economic thresholds were not reached, numbers in `docs/metrics.md`, section "H20 extension". Fine rounding, the 200-day moving average and illiquid names showed no effect; the general reversal on illiquid names is the same for the real and the placebo level and is probably last-trade noise. The test of the link with the reversal after news is inconclusive: 93 events with an extreme crossing against the required 100, the interval is very wide; there is no unconditional reversal after the first hour. Order book and order history remain untested until an AlgoPack token or our own order-book snapshots; it makes sense to do this only if a separate reason appears to expect an effect from the order flow. H20 is closed entirely as a signal source and as a basis for a protective filter.

Conditions for re-testing H20: (1) order book and order history: an AlgoPack token appears (`obstats`, `orderstats`) or at least 60 trading days of our own order-book snapshots are accumulated; then only the order flow at levels is tested by the same criteria, 200+ events and 40+ dates. (2) 200-day moving average: if the sample grows from 326 to 1000+ events (more tickers or a longer period), not by repeating the same data. (3) Link with the reversal after news: 100+ events with an extreme crossing are collected on new news after 2026-10-15; do not rerun the same data. (4) High/low instead of closes: High and Low appear in the 10-minute cache. If none of the conditions is met, do not open H20.

## H21. Entry regime filter on a volatility forecast (2026-10-17)

Source: Lysenok, "Which model families pay?" (IJCISIM 18(17s), 2026, doi 10.70917/ijcisim-2026-4759), 17 MOEX stocks, 10-minute bars, 2014–2026. The paper's claim: the same improvement in volatility forecast accuracy (28% by QLIKE) gives a Sharpe gain of +0.79 and +0.85 if the forecast decides whether to enter a trade (regime filter) and with what size (volatility targeting), and +0.04 (p=0.52) if the forecast only scales the stop and take-profit. This matches H20: stops and take-profits as a channel do not work. Caveats about the paper: the economic effect tables are taken from an earlier work of the same author and not recomputed; the strategies are textbook ones (Bollinger, Donchian, Supertrend) with market exposure of 7–16% of the time; commission 0.05–0.06% per side; a period with a 16–21% rate and free capital in the money market. The paper's figures are a guide for us, not proof.

What we already have and what limits expectations:
- Volatility targeting as a size channel was already tested crudely: experiment D (`risk.vol_scale`, a multiplier from the ticker's `realized_volatility` to the median) was rejected, P&L-neutral; see `docs/metrics.md`, "Experiment D". This is not a volatility forecast but the current realized one, so the size channel with a real forecast remains untested, but the expectation is low.
- HAR on our data is no better than the production sigma (`docs/metrics.md`, comparison of HAR and sigma). The paper's gain (28%) was obtained by a HAR-J hybrid with boosting on 234 features, not by HAR itself.

Hypothesis: a filter that forbids new model entries when the forecast of the next day's realized volatility falls into the upper quantile of the ticker's own history raises realized P&L per trade or lowers the maximum drawdown without losing realized P&L. The direction of the filter is fixed in advance: exclude the high regime (in the paper the gain comes from lower volatility and drawdown, not from higher return). The opposite direction (allow only the high regime) is not tested, otherwise the family of tests would double.

Protocol, two stages, the second runs only if the first passes.

Stage 0, no code in the trading loop. Closed trades of walk-forward runs (six purged windows, `-wf-dir` artifacts) and a daily HAR-J forecast on 10-minute bars, computed only from data before entry, are taken. Trades are split into the upper forecast quantile (threshold: the 80th percentile of the ticker's history, fixed in advance) and the rest. The realized result per trade is compared, bootstrap by dates, 95% interval. Pass criterion: the interval of the difference "rest minus upper quantile" is above zero, or the maximum drawdown on the "rest" trades is lower without losing more than 5% of total realized P&L; at least 200 trades and 40 distinct dates in the upper quantile. If there are fewer trades in the upper quantile, there is no conclusion and stage 1 does not run.

Stage 1, a regime filter in `EnsembleSignalSource` modeled on `risk.vol_scale` (option off by default, a common path for live and backtest). A run on the same window artifacts without retraining. Criteria in advance: realized P&L not lower than the control by more than 5%, maximum drawdown not higher than the control and the kill switch does not fire more often, positive windows not fewer than the control. The filter is not enabled by default under any outcome: enabling requires a separate decision of the owner, as for other changes to entry thresholds.

Pre-registration: the quantile threshold (80), the forecast horizon (1 day), the model type (HAR-J on 10-minute RV, no boosting), the filter direction, the criteria. They must not be changed after seeing the result; a new version counts as a new hypothesis with a multiplicity correction.

Conditions for re-testing after a rejection: (1) stage 0 failed but there were fewer than 200 trades in the upper quantile: repeat when 200+ trades over 40+ dates have accumulated; (2) boosting over HAR-J on our features gives a QLIKE better than HAR-J by at least 10% with rolling retraining on a held-out period: then test the filter with that forecast; (3) a size channel with a forecast (not realized) volatility: only if stage 1 passes for the filter. If none of the conditions is met, do not open H21.

### Read in this pass (2026-10-17)

- Chen et al., review "Lightweight Multimodal Fusion… Reversal Prediction of S&P 500": a literature review without own results, nothing applicable.
- Polozhentsev et al., "A multilevel forecasting system" (2026): evaluated only by MAE in rubles in a simulation, without costs and without comparison with a naive forecast. Not taken.
- Closed, only abstracts read. Makeeva et al., "News sentiment index and its predictive power for the Russian stock market" (Voprosy Ekonomiki, 2026): FinBERT, Telegram, 925 securities 2015–2024; negative sentiment acts more strongly than positive; the event analysis shows no significant market reaction after a sentiment shock and shows a pre-trend, i.e. sentiment is reactive, not a cause. Babiy and Borisenko, "Assessing the impact of news sentiment on stock returns in the Russian market" (Finance and Credit, 2026): the 50 largest issuers, minute data, 2020–2025; the asymmetry of negative news is strongest at the short horizon, no dependence on capitalization, a cross-security effect of news of large issuers on small ones at medium and daily horizons. Both agree with our result (reaction in the first 1–4 hours, no effect by a day, `docs/metrics.md`). A new testable idea from the second: the effect of news of large issuers on small ones. It is not opened as a separate hypothesis until there is a full text with the definition of the event and the horizons; condition: obtain the full text.

## H22. News as a risk regime, not a direction (criteria fixed before the run, 2026-10-17)

Basis: a directional sentiment signal is confirmed neither by our H20 nor by the abstracts of Makeeva et al. (sentiment is reactive, pre-trend), but news increases the size of the reaction in the first 1–4 hours. Hypothesis: model decisions made after a news burst on a ticker differ in outcome and in adverse excursion from the rest, and then news is suitable as an input to a regime filter (link with H21), not as a buy signal.

Data: BUY and SELL decisions of the six walk-forward windows (`wf-volscale`, the actions do not depend on size scaling) from 2026-04-01; before that date `data/news_history.jsonl` has a handful of news per month (since April 2026 about 1500–2500 per month), so earlier periods cannot be evaluated. 1492 decisions, 159 dates.

Definitions (without looking at outcomes):
- News burst: at least 3 articles on the ticker in a 24-hour window ending at 10:00 MSK of the decision date. Fits 304 decisions on 113 dates (at threshold 1: 816 on 154 dates, at 2: 489, at 5: 137). Threshold 3 was chosen by the number of decisions, not by outcomes. Threshold 1 is descriptive only.
- Outcome 1 (primary): signed return from the close of the decision date to the close 10 trading days later (BUY plus, SELL minus), as in `cmd/challengercompare`.
- Outcome 2 (primary): the worst signed level by daily closes over those 10 days (adverse excursion). There are no intraday High/Low in the cache, so this is a lower bound on risk.
- Comparison: mean outcome "burst" minus "no burst"; bootstrap by ISO weeks (consecutive decisions on one ticker are strongly correlated), 2000 repetitions. Two primary tests, Bonferroni correction, 97.5% interval.
- Minimum for a conclusion: 200 decisions in the "burst" and 40 distinct dates; otherwise there is no conclusion.
- BUY and SELL cuts, threshold 1, threshold 5: descriptive only.

Criterion: the interval of the difference in outcome 1 does not contain zero, or the interval of the difference in outcome 2 does not contain zero. Economic threshold: the difference of at least 30 bp over 10 days in absolute value. The sign of the difference sets the use: worse after a burst means a candidate for a protective entry filter (a separate test under the H21 stage 1 protocol); better means the news carries information that the model does not yet use, and this is a basis for studying a feature, not a filter. Any outcome other than the listed ones closes H22.

Limitations fixed in advance: the test window is 5.5 months, one market regime; the news has no sentiment score (count, not sign); overlap with H21 is possible because a news burst comes with volatility, and they cannot be separated here; the test itself does not prove causality.

Result of H22 (2026-10-17): not confirmed. Both primary tests gave intervals containing zero (difference in outcome 1 -0.41%, outcome 2 -0.37%), the required 200 decisions and 40 dates in the burst were reached (304 and 113). The descriptive cuts (thresholds 1, 2, 5, BUY only, SELL only) also have no significant differences. Numbers in `docs/metrics.md`, section "H22". Do not introduce a protective entry filter on a news burst; the "news as a regime" feature remains only as part of a possible H21 input.

Conditions for re-testing H22: (1) the sample grows at least twofold through new decisions after 2026-10-17 (not by repeating the same ones), with the same threshold 3 and the same criteria; (2) a sentiment score of news per ticker appears, then the agreement of the news sign and the decision direction is tested as a separate hypothesis; (3) High and Low appear in the 10-minute cache, then outcome 2 is computed from them. If none of the conditions is met, do not open H22.
