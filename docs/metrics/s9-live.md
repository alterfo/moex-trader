# Metrics: evidence from live operation (section 9)

Part of [metrics.md](../metrics.md) (the index, conventions and ledger live there). Numbers here are the single source of truth for this topic.

## 9. Evidence from live operation

### 9.1 Root cause of the September-October SELL drift (step 9)

Hypotheses P1 (feature extrapolation), P2 (staleness), P3 (base rate) were each rejected by their registered rules (P1 Spearman of weekly mean p with weekly mean mom_63d / dist_ma50_pct / rsi_14 -0.40 / -0.24 / -0.37 with 4-7% of observations above the training 95th percentile; P2 p <= 0.40 share 17.1% deployed vs 32.9% fresh model; P3 training base rate 0.451 vs window 0.487, a 3.6 pp difference). The cause was found afterwards (post hoc, verified on every day in the table): the logged live p equals the model applied to the **unmasked** ingest vector (`news_count` up to 144 against a training mean 0.10 and std 0.70) on every day from 09-20 to 10-09, and equals the model with `news_count` and `news_sentiment` masked to zero from 10-10.

| day | logged live p | model on unmasked vector | model on masked vector |
|---|---:|---:|---:|
| 09-30 | 0.2160 | 0.2160 | 0.4556 |
| 10-02 | 0.1903 | 0.1903 | 0.4759 |
| 10-08 | 0.1504 | 0.1504 | 0.4094 |
| 10-09 | 0.1752 | 0.1752 | 0.4191 |
| 10-10 | 0.4043 | 0.1679 | 0.4519 |
| 10-17 | 0.4572 | 0.1587 | 0.4572 |

Until 2026-10-09 the running trader evaluated the ensemble with `news_count` and `news_sentiment` the model never trained on (training value 0), because `maskUntrainedFeatures` (commit `f8f64a1`, 2026-10-10) was not in the running build (the ai-box binary is dated 2026-10-06). The growing live news archive pushed the logistic and tree members to SELL: live p was on average 0.18 below the replay p (gap 0.04 on 09-17 growing to 0.28 on 10-02); 81-95% of live decisions were SELL in ISO weeks 39-41 against 17.1% in the masked replay; indicator offsets of the live ingest vectors are worth only -0.007 in mean p. Consequently the September-October "model nearly always bearish while IMOEX rose" was this feed bug, not a model property; the 82 live SELL executions of 09-17..10-09 (against 4 in the replay of the masked model) came from it.

**Post-mortem of the masked model over 2026-07-20..10-18** (step 8, replay with the deployed artifact, gate shape): realized -1820.29 RUB on 117 closed trades (-0.18% of the deposit, max DD 1.05%, no kill switch); by 30-day period -914.60 (43 trades), +3151.92 (35), -3555.03 (23). By ticker PLZL -4508, CHMF -2331, POSI -1367, NVTK -992, ROSN -697, OZON +4952, MGNT +1444, GAZP +886; BUY +839 on 28 trades, SELL -2659 on 89. Beta over the period (IMOEX +22.3%): +3871 component (BUY +8475, SELL -4605), excess -5691: the short-beta explanation is not supported; dividends -14 RUB. Under the registered rule (a cause explains the period if removing it moves realized by at least 50% of its absolute value) only the three worst trades do (PLZL BUY 15 lots -4133, a 22.3% slide while IMOEX rose 6.6%; CHMF SELL -2274; LKOH SELL -1777; removing them gives +6364). The loss is small, idiosyncratic and inside the procedure noise of section 3.6.

### 9.2 Sell-bias diagnostic and the out-of-range guard (2026-10-18)

Data: `trader-sandbox.db` audit events 2026-09-16..10-18, about 38.3k decisions since the news-masking fix. After the fix about 3600-4200 HOLD and 460-1200 SELL per day, 0 BUY; hold reasons: `out_of_range` 50-60%, `model` about 30%, SELL about 14%; probability bins: < 0.40 about 31%, [0.40, 0.60] about 69%, >= 0.60 0.11% (42 decisions, all VTBR).

- **Guard finding.** 100% of post-fix `out_of_range` blocks are `event_dividend` with z = 14..38, although the deployed artifact carries mean 0, std 1, coefficient 0 for all eight event features and both topic signals and no tree uses them. The guard vetoed would-SELL 7138, would-BUY 42 and would-HOLD 14861. A fix (`ae6ffdd`: skip features with logistic coefficient 0 that no tree uses; test `TestEnsembleIgnoresUntrainedEventFeatures`) was written and then reverted (`dcb601d`, see below); the veto behaviour is live.
- **Reproduction offline.** Replay of the deployed model with the fixed guard over 2026-10-10..10-18 (136 decisions): 95 HOLD / 41 SELL / 0 BUY (SELL share 30.1% against 32.2% live pre-guard): the lean reproduces from data and model, not a serving defect. A fresh model (trained to 2026-09-16) gives 94 / 39 / 3 BUY (BUY share 2.2%): a small staleness component in the BUY tail.
- **Deployment attempt and correction.** Deploying the guard fix made the preflight gate refuse to start (90-day replay -4859.13 net of borrow, 98 trades, hit 41.8%); the fix was reverted on the belief that the accidental veto acted as a profitable regime filter. That attribution is **not supported**: two `cmd/backtest` builds with and without the guard change give identical replay output to the last digit (window ending 10-18: -2467.25, 117 trades; ending 10-17: +1209.23, 118 trades); the live gate printed the same -4859.13 with and without the fixed binary; the "+602.32 on the same window" was an earlier gate with a window start of 07-19. The gate result is set by the window start, not by new bars (replay ending 10-18, realized net of borrow by start date: 07-13 +20380, 07-14 +14584, 07-15 +11455, 07-16 +9382, 07-17 +9100, 07-18 +3435, 07-19 +1372, 07-20 -2467, 07-21 -5085, 07-22 -5959, 07-23 -2375, 07-24 -3034, 07-25 -3514, 07-26 -5536, 07-27 -784). Whether the live path should veto on `event_dividend` is an open owner decision independent of the gate (`docs/plans/20261018-gate-failure-remediation.md`).

### 9.3 Preflight concentration (2026-10-18)

Gate-shaped replays (90-day rolling, 17 tickers, 1M deposit, costs 0.0005 x 3 plus per-ticker spreads from `trader-sandbox.db`, borrow 0.00005 per day, caps 90000/90000, deployed artifact, no news) give realized gross +9843.33 (till 10-14), +3637.89 (till 10-16), -1990.32 (till 10-18), matching the gate's own decline (+7156 -> +602 -> -4859 net of borrow; the CLI lacks lot quantisation). ISS has no bars for 10-17/10-18, so the decline is the left boundary of the rolling window. Closed trades of the till-10-18 replay by close month: 2026-07 20 trades +414.93; 08 35 +2828.55; 09 37 -4821.86 (mean -130.32); 10 24 -411.94. By ticker: OZON +4952.20 (3), MGNT +1444.44 (1), GAZP +885.65 (10), SBER +594.66 (14), VTBR +237.61 (19), RUAL +78.45 (10), TATN +65.69 (10), T -32.16 (4), MTSS -48.65 (9), LKOH -100.52 (3), GMKN -171.25 (8), ROSN -697.07 (6), NVTK -992.47 (4), POSI -1367.08 (3), CHMF -2331.33 (5), PLZL -4508.49 (7). Positives are month-concentrated (July-August), September is deeply negative, and the rolling sum drifts further negative before stabilising (profitable August days drop out while September losses stay in until December). This is a model-quality problem, not a gate problem: do not loosen the threshold.

### 9.4 Manual retrain candidate (2026-10-18), rejected

Same recipe as the deployed ensemble, fresh export 2024-01-01..2026-10-18 (18 tickers, 14619-record news archive), `ENSEMBLE_TRAIN_TILL=2026-10-17`, embargo 10; both models replayed on the HEAD engine.

| window | deployed realized | candidate realized | deployed trades | candidate trades | deployed max DD | candidate max DD |
|---|---|---|---:|---:|---:|---:|
| hold30 (2026-09-17..10-17) | +583.21 | -3281.95 | 32 | 45 | 1.24% | 1.45% |
| pre90 (2026-07-20..10-17) | +10419.63 | -1508.68 | 137 | 164 | 1.69% | 2.27% |

The candidate fails the gate on both windows; `ensemble_model.json` stays. Retrain again only through the same gate after at least a quarter of new data from 2026-10-17.

### 9.5 Ticker circuit breaker in the gate replay (2026-10-18)

`backtest.Config.Breaker` / `cmd/backtest -breaker` feed the live `risk.TickerBreaker` (3 consecutive losing round trips or cumulative loss of 5% of the 15000 RUB target; a tripped ticker stays blocked). Gate-shaped replays, one run per cell:

| window (90d till) | breaker | closed trades | realized net of borrow | unrealized MTM |
|---|---|---|---|---|
| 2026-10-18 | off | 117 | -2455.72 | -2066.01 |
| 2026-10-18 | on | 96 | -3356.64 | +6139.47 |
| 2026-07-20 | off | 90 | +34737.78 | +189.38 |
| 2026-07-20 | on | 85 | +30484.28 | +3086.18 |
| 2026-04-21 | off | 34 | -3842.92 | +4610.19 |
| 2026-04-21 | on | 22 | -1183.71 | +3291.07 |

Sum of realized over the three windows: off +28439.15, on +25943.92. The breaker does not rescue the gate. `risk.Gate` rejects every order for a tripped ticker with `ticker_breaker_blocked`, including closing orders, so a tripped ticker's position is frozen (about 8.2k RUB moves from realized to unrealized in the 10-18 window); whether blocking exits is intended is an open owner question.

### 9.6 Nightly candidate gate aligned with the preflight (2026-10-19)

The nightly harness lacked the risk layer of the preflight. Deployed artifact, 17 tickers, 2026-07-20..2026-10-18, same window the preflight rejected with -4694.87:

| variant | flags beyond the old harness | realized | realized net of borrow | net with open | closed | max DD |
|---|---|---|---|---|---|---|
| A (old harness with `-news-history`) | none | +10857 | - | -5930 | 137 | 1.72% |
| B | `-max-net-short 90000 -max-net-long 90000 -borrow-pct-day 0.00005` | -2057 | -2704 | -4843 | 116 | 1.06% |
| C | B without `-news-history` | -2069 | -2716 | -4855 | 117 | 1.06% |
| preflight (live log) | spread table from audit DB, macro enricher | - | -4695 | -903 | 99 | 1.12% |

The net-exposure caps and stress borrow rate explain the whole gap between the old harness and the preflight sign. `-news-history` changes the result by 12 RUB (the live path masks news features); the remaining 2000 RUB gap is not decomposed (candidates: the audit-derived spread table, the market enricher). `scripts/daily-retrain-box.sh` now passes the caps and the borrow rate and `cmd/retrainreport` compares realized net of borrow (`TestParseMetricsPrefersRealizedNetOfBorrow`, `TestDailyScriptReplayMatchesPreflightRisk`). Earlier nightly verdicts (10-16, 10-17) were produced without the caps and are not recomputed.

### 9.7 Live sandbox P&L, 2026-09-17..2026-10-18

`cmd/livepnl` over the audit copy (executor events replayed with `internal/pnl`, marked to ISS closes; 148 fills, none between 09-24 and 10-06 or after 10-14):

| period | binary | realized | equity change incl. unrealized | net / gross exposure at end | IMOEX |
|---|---|---|---|---|---|
| 09-17..10-09 | no feature mask (bug) | -149 | -14073 | -226922 / 227104 | +6.97% |
| 10-10..10-18 | mask `f8f64a1` | -1771 | -11464 | -244191 / 244384 | +1.87% |
| total | | -1920 | -25395 | | +8.9% |

The book was entered as an almost purely short book (17 positions) during the unmasked period and stayed short after the fix because the fixed model mostly returned HOLD (4 BUY fills, all covering shorts). Beta check: -227k net short against +6.97% IMOEX is about -15.8k, the same order as the observed -14.1k, so the unmasked-period loss is market beta of a book the bug built. The masked period lost more than its beta (about -4.6k, observed -11.5k; three covered shorts, one -1650 realized on 10-14). Realized P&L alone hides the loss sitting in 17 open positions. Caveats: last close at or before the boundary, no borrow, dividends or financing; the Mac trader was in `error` after 38 restarts at the audit copy of 2026-10-19.

<!-- shadow-reconciliation:start -->

## Shadow reconciliation (Task 3)

Live decision vs replayed backtest decision for the same ticker and last closed
day. `FeatureMatch` compares candle-derived features (return_pct and
price/volume indicators); news, order-book and event fields are live-only and
excluded. `SignalMatch` compares the model action. Rows are appended by the
live trader when run with `--shadow-digest docs/metrics.md`; no live sandbox
rows are available yet at commit time.

| ticker | day | live | replay | feature match | max delta | signal match |
|---|---|---|---|---|---|---|

<!-- shadow-reconciliation:end -->

### 9.8 Execution-quality tracking (Task 16)

Live tracking of expected (bounded limit) versus actual fill price, broker rejection rate under `risk.max_slippage_pct` and actual short-borrow charges: a positive slippage in basis points means the fill was better than the cap. The trader logs each fill and emits a digest every 6 hours cross-referencing observed slippage against the per-ticker half-spread and margin-fee charges against the 0.005% per day stress rate (`internal/filltracking`, `internal/borrowcost.SumMarginFees`, `executionQualityReporter`). No fill-level numbers have been recorded yet.

## 10. Observability metrics and alerts

Prometheus metrics of the live trader (`internal/metrics/metrics.go`, wired through `internal/orchestrator` and `cmd/trader`; `moex_trader_inference_duration_seconds` was renamed from `llm_inference_duration_seconds` because the pipeline is a model, not an LLM):

| metric | type | labels | source |
|---|---|---|---|
| `moex_trader_inference_duration_seconds` | histogram | - | orchestrator signal generation |
| `signals_generated_total` | counter | - | orchestrator |
| `risk_rejections_total` | counter | `reason` | risk gate decision |
| `executor_skips_total` | counter | `reason` | approved decisions the executor skipped |
| `signal_probability` | histogram | - | model probability per poll |
| `candle_age_seconds` | gauge | `ticker` | newest ingested candle age |
| `feature_psi` | gauge | `feature` | drift monitor warnings |
| `position_notional` | gauge | `ticker` | signed exposure per ticker (RUB) |
| `gross_exposure` | gauge | - | sum of absolute per-ticker notionals (RUB) |
| `lease_held` | gauge | - | 1 while this process holds the trading lease |
| `var_95`, `var_99`, `es_95`, `es_99` | gauge | - | 1-day historical VaR/ES of the current book (RUB), 250-day trailing returns (`internal/risk/var.go`), monitoring only, also one line in the daily Telegram digest |
| `incident_shorts_total` | counter | `ticker` | SELL decisions emitted by the news gate during an incident (not every one becomes a fill) |
| `news_exit_total` | counter | `ticker`, `reason` | first exit decision for a news-opened position (`stop_loss`, `take_profit`, `time_exit`); each also writes an `audit_events` row, stage `news_exit`, with entry, peak and result for the parameter fit in `docs/plans/20261010-news-exit-tuning-and-buy-side.md` |

Telegram alerts (`internal/alert/telegram/watch.go`), each with a cooldown:

| alert | trigger | default threshold | default cooldown |
|---|---|---|---|
| probability collapse | more than 90% of the last N probabilities inside [0.45, 0.55] | N = 20, fraction 0.9, band 0.45-0.55 | 1h |
| stale candles | candle age above threshold during the trading session | 24h (daily candle pipeline) | 1h |
| PSI breach | drift monitor warning above the configured PSI threshold | `drift_psi_threshold` 0.2 | 1h |

Defaults come from `config.alerts` (`probability_collapse_window`, `stale_candle_age`, `cooldown`); negative values are rejected, and the senders are no-ops without Telegram credentials.

## 11. Synthesis

| question | result | section |
|---|---|---|
| Does the deployed recipe predict direction? | No: AUC 0.46-0.55, N95 noise 0.016, tail precision below the base rate | 3.1, 3.6 |
| Where does its P&L come from? | Mostly short IMOEX beta; alpha vanishes under a real futures hedge | 5.2-5.5 |
| Can a single change pass the gate? | No variant of A-F, depth, breadth, rank, volatility-unit, open-interest or TimesFM changes | 4, 6 |
| Is a longer horizon or more history better? | Horizon: no (10 days best). History: small paired gain at the noise floor, no PnL benefit | 6.1, 6.3, 6.6 |
| Does news carry a tradable signal? | AUC about 0.52 at best, net spread +0.263% with CI spanning zero, short leg only | 7.3-7.6 |
| Do LLM headline directions help? | No (AUC 0.511-0.526) | 7.7 |
| Do events, levels, bursts matter? | No tradable effect found | 7.8-7.10 |
| Are there calendar or cycle effects? | None after 1990 / after costs | 8.11 |
| Is there any cross-sectional edge? | Ridge long-minus-universe passes the registered rule; long-short needs borrow below about 5% per year; gate-candidate pass rate 59% | 8.6, 8.8 |
| What caused the autumn SELL drift? | Unmasked news features in the running build, not the model | 9.1 |
| What beats the hurdle? | Nothing recorded exceeds the 14% key-rate carry | 8.12 |

Open items: the real short-borrow rate (Task 11 cannot measure it from account history); the forward track of the ridge and C2 books (recorder running since 2026-10-19, first evaluation after at least 12 rebalancings); the `event_dividend` veto decision; whether the ticker breaker should block exits; a second fundamentals experiment after at least 120 daily snapshots.

