# Architecture

Russian translation: [ARCHITECTURE.ru.md](ARCHITECTURE.ru.md). This file is canonical.

Law: this document is updated in the same commit as any change that adds, removes or renames a `cmd/*` or `internal/*` package, or changes the live loop, the data flow, the config keys or an external integration. `internal/docscheck` fails the build when a package is missing from the maps below.

Direction and goals: [VISION.md](VISION.md). Hard invariants and ownership: [../AGENTS.md](../AGENTS.md). Numbers: [metrics.md](metrics.md).

## 1. System overview

A single-process modular monolith written in Go. Packages under `internal/*` talk through interfaces; there are no network hops between components. `internal/bus` (Redis Streams) is unused scaffolding for a future process split and is imported by nothing.

```
MOEX ISS / RSS + Telegram news / AlgoPack ──> orchestrator.MOEXIngestor
                                                   │
                                                   v
                                  features.Builder ──> domain.FeatureContext
                                                   │
                                                   v
        signal source chain (see 2.2): model | ensemble | score table
                                                   │  domain.TradeSignal
                                                   v
                      risk.HardenedGate ──> executor (paper | T-Bank sandbox | Finam)
                                                   │  Fill
                                                   v
         SQLite (audit_events, trade_signals, kill_switch) ──> cmd/verifier, cmd/livepnl
         Telegram alerts, Prometheus :9090
```

Offline side (research, training, validation) shares `internal/backtest`, `internal/features` and `internal/model` with the live path, so a backtest replays the same code the trader runs.

## 2. Live trader (`cmd/trader`)

### 2.1 Startup order

config → storage (SQLite) → optional `-reset-kill-switch` (then exit) → MOEX/news/AlgoPack/Telegram clients → broker runtime (`newBrokerRuntime`) → base signal source (`newModelSignalSource`) → wrappers (2.2) → **preflight gate** → risk gate with ticker circuit breaker (seeded from audit history by `seedTickerBreaker`) → Prometheus server → orchestrator loop (`orchestrator.Run`) plus background goroutines (exposure metrics, execution-quality tracking, daily summary).

### 2.2 Signal source selection and wrapper chain

Base source, first match wins (`newModelSignalSource`, `cmd/trader/main.go`):

1. `model.score_table_path` → `model.ScoreTableSource` (`internal/model/scoretable.go`): daily per-ticker probability `p` read from a CSV (`ticker,date,p`, currently `data/ridge_scores.csv`, produced offline by `scripts/ridge_probs.py` and pulled from the ai-box by `scripts/ridge-scores-pull.sh`); rows older than 7 days are stale; thresholds `p >= 0.60` BUY, `p <= 0.40` SELL.
2. `model.ensemble_path` → `model.EnsembleSignalSource` (`internal/model/ensemble.go`): LightGBM + XGBoost + logistic regression evaluated in pure Go; thresholds stored in the artifact.
3. otherwise `model.SignalSource` over `model.json` (pooled logistic regression, `internal/model/logistic.go`).

Wrappers, innermost first: lot-size resolver (`lotSizeSignalSource`) → champion/challenger shadow (`challenger.go`; the challenger never reaches the executor) → PSI drift monitor (`driftSignalSource`, warns only) → probability-collapse watch → two-poll signal hysteresis (`orchestrator.SignalHysteresisSource`) → news gate (`newsGateSignalSource`: incident veto/short-on-incident, news-position exit via `newsExitManager`) → alerting wrapper (Telegram on signal errors).

The preflight gate uses the base source before the shadow, drift and news wrappers (`newPreflight(cfg, modelSource, ...)`); it must use the same base source as the live loop.

### 2.3 Cycle (`orchestrator.RunOnce`, every `poll_interval`, default 5 min)

1. Reset the per-cycle news cache and prefetch AlgoPack order books.
2. Account snapshot (`AccountSource`): sandbox portfolio; none in paper mode.
3. For each ticker: kill-switch check → ingest (security lookup, quote, 10-day daily candles, news, order book) → `features.Builder.Build` → `SignalSource.Generate` → `risk.HardenedGate.Approve` → `Executor.Execute`. Each stage writes an `audit_events` row with stage names `ingest`, `llm` (historical name of the signal stage; no LLM is on the path), `risk_gate`, `executor`.
4. Observers (Telegram decision notifier, ticker breaker fill observer, fill-quality observer) see every decision.

In the sandbox the executor is wrapped in a market-hours guard; open positions are reconciled with the broker portfolio (`position_reconcile.go`).

### 2.4 Preflight gate

`cmd/trader` replays the current configuration over the last `preflight.days` (default 90) days with `internal/backtest` before the loop and refuses to start when realized P&L is below `preflight.min_net_pnl`, closed trades are below `preflight.min_closed_trades`, history is unavailable for every ticker, or every decision errored. The decision is logged with a SHA-256 of the gate-relevant configuration.

### 2.5 Risk

`risk.HardenedGate` (`internal/risk/gate.go`): lot limit, net/short/long exposure caps, sector caps, fat-finger price band, daily-loss limit, drawdown kill switch (persisted in SQLite, reset only with `-reset-kill-switch`, which needs equity ≥ 1.5x the largest open notional via `runbook.Decide`). `risk.TickerBreaker` stops a single name after consecutive or cumulative losses. `risk/var.go` provides 1-day VaR/ES for monitoring only.

### 2.6 Execution modes

- **paper** (`broker: paper`): `PaperExecutor`, virtual fills, no account checks.
- **T-Bank sandbox** (`broker: tinkoff`, `tinkoff.sandbox: true`): `internal/ingestion/tinkoff` client, `internal/broker/tinkoff` account bootstrap/snapshot/cancel-by-instrument, `executor.LiveExecutor`.
- **Finam** (`broker: finam`): `internal/ingestion/finam`, `internal/executor/finam.go`.
- **Real money**: intentionally disabled; `cmd/trader` refuses a non-sandbox T-Bank account. See [GO_LIVE_CHECKLIST.md](GO_LIVE_CHECKLIST.md).

## 3. Failover (two nodes, one sandbox account)

Mac (primary) and ai-box (standby) never trade simultaneously. The witness (`cmd/witness`, FreeBSD VPS) grants a 5-minute lease; `cmd/watchdog` on each node runs the trader only while holding it and fences it when the lease is lost (`internal/failover`: election, witness client, peer heartbeat, SQLite snapshot sync). The authoritative DB is chosen by `trader.last_start_at`, never by file mtime. Deploy: `scripts/deploy-failover.sh {witness|aibox|mac|start-aibox|status}`, templates in `deploy/`. Details and invariants: [../AGENTS.md](../AGENTS.md).

## 4. Offline pipeline

Data: `cmd/pricehist`, `cmd/tqbrsnap`, `cmd/intradayfetch`, `cmd/futoifetch`/`futoihist`, `cmd/macrofetch`, `cmd/fundsnap`, `cmd/reportsfetch`, `cmd/dividendscalendar`, `cmd/tradesrecord`, `cmd/algopackfetch`/`algopackcheck`/`orderflowstudy`, news tools. Sources and limits: [knowledge/data-sources.md](knowledge/data-sources.md).

Training: `cmd/exportdataset` (CSV from `model.ToVector`) → `scripts/export_ensemble.py` → `ensemble_model.json`; `cmd/trainmodel` → `model.json`; `cmd/trainnewsmodel` → `news_classifier.json`. Feature order is mirrored in `internal/model/features.go`, `scripts/export_ensemble.py` and `scripts/compare_models.py`; a mismatch is a hard load error (`checkFeatureOrder`) and a preflight failure. Details: [knowledge/feature-pipeline-and-inference.md](knowledge/feature-pipeline-and-inference.md).

Validation: `cmd/backtest`, `cmd/walkforward` (purged, embargo), `cmd/strategyvalidation` (deflated Sharpe, PBO, Pass^k), `cmd/oosfreeze`, `cmd/challengercompare`. Every number lands in [metrics.md](metrics.md) (lint: `internal/metricsdoc`). Auto-swap of the live model is forbidden; a daily CANDIDATE job is allowed ([../AGENTS.md](../AGENTS.md)).

Research lifecycle: pre-registered plan in [plans/](plans/README.md) → run → numbers into `metrics.md` → plan moved to `plans/completed/`.

## 5. Package map: `cmd/*`

Live and operations (in the live or failover path):

| Package | Role |
|---|---|
| `cmd/trader` | live loop (section 2) |
| `cmd/watchdog` | per-node supervisor: lease, fencing, DB sync, alerts |
| `cmd/witness` | lease arbiter on the VPS |
| `cmd/sandboxcheck` | T-Bank sandbox smoke test (`-place-order` optional) |
| `cmd/verifier` | markdown P&L report from audit events |
| `cmd/pnlexporter` | Prometheus P&L exporter from SQLite |
| `cmd/algopackgrafana` | daily AlgoPack and IMOEX SQLite (`data/grafana/algopack.db`) for the Grafana dashboard `deploy/grafana/dashboards/algopack.json`; rolling alpha and beta are computed in SQL on the fly; the table `ridge` is loaded from `data/ridge_scores.csv` (pulled by `scripts/ridge-scores-pull.sh`), and the dashboard shows the cumulative A2 top-quintile and ridge top-third spreads and the latest ridge ranking; synced to the ai-box by `scripts/algopack-daily.sh` |
| `cmd/livepnl` | live P&L over a date range from the SQLite audit |
| `cmd/retrainreport` | Telegram report of the daily candidate retrain (no model swap) |
| `cmd/tradesrecord` | AlgoPack delayed-trades recorder (ai-box cron) |
| `cmd/algopackfetch` | AlgoPack Super Candles history (`tradestats`, `obstats`, `orderstats`, `hi2`) into `data/algopack/<dataset>/<date>.csv.gz`, resumable; daily refetch of the last four days by `scripts/algopack-daily.sh` (launchd, 00:20 MSK) |
| `cmd/algopackcheck` | validation of the downloaded AlgoPack history (calendar, coverage, hi2 metrics, price scale against ISS); no returns |
| `cmd/orderflowstudy` | pre-registered order-flow study on the AlgoPack history (blocks A-D, `-diag` post-hoc checks), offline, reads `data/algopack` |
| `cmd/flowscan` | pre-registered scan of 100 AlgoPack hypotheses (plan `20261023-algopack-100-hypotheses`): modes `prep` (outcome-free M_eff, threshold, control lists), `explore`, `verdict`; plus the forecast-improvement modes of plans `20261023-algopack-forecast-improvement` and `20261023-algopack-round2`: `export` (daily ten-column AlgoPack block with forward closes), `volfc` and `spreadfc` (Fama-MacBeth volatility and spread forecasts with out-of-fold R-squared gain), `econ` (A2 economic variants, universe size flag), `mktprep` and `mkt` (date-level market return and volatility families); Fama-MacBeth plus net trading form per row, offline, reads `data/algopack` and `data/dividends.jsonl` |
| `cmd/algopackincremental` | paired incremental test of the AlgoPack block on the deployed ensemble recipe (plans `20261023-algopack-forecast-improvement` and `20261023-algopack-round2`): `merge` joins the `exportdataset` CSV with the `flowscan export` block (optional demeaning, abnormal label, within-date placebo permutation), `score` scores per-fold ensemble JSONs through `internal/model` and reports paired ΔAUC and ΔIC with a block bootstrap, the placebo-corrected difference and a behavioural read, `ks` runs the regenerated-versus-realtime distribution check; offline, no live path |
| `cmd/futscan` | pre-registered scan of AlgoPack futures (`fo`) and currency (`fx`) hypotheses (plan `20261023-algopack-futures-fx`): `panel` builds the daily per-contract feature panel from the three datasets with per-dataset realtime flags, `step0` (outcome-free provenance, contract and distribution checks), `prep` (outcome-free effective test count and threshold), `scan` (Book 1: futures and FX outcomes, Fama-MacBeth and time-series rows) and `scan2` (Book 2: equity and IMOEX outcomes, leads only); offline, no live path |
| `cmd/alphafit` | composite-alpha and cost-aware-execution study on the `cmd/flowscan -mode matrix` file (plan `20261023-algopack-alphafit`): rank features, purged expanding walk-forward ridge and elastic net in Go, shuffled-feature placebo, dollar-neutral long-short quintile book with costs and borrow, Garleanu-Pedersen partial-adjustment execution; modes `explore` (hyperparameters and trade rate on the regenerated window), `verdict` (realtime window, run once), `freeze`, `forward` and `dump`; flags `-ext`, `-horizon`, `-cost` and `-entry` select the target horizon, the spread-based cost model and the next-open entry (plan `20261024-algopack-wave3-costs-horizon`); `-with3b` keeps the four wave-3b base columns as features, mode `shadow` writes point-in-time scores of the frozen models (plan `20261023-algopack-alphafit`), `-div` adds the `DivFlow` and `A2PR` derived columns, `-aucinc` keeps `AucA1` for next-open entry, mode `compare` differences two dumped series (plan `20261024-algopack-wave3b-model-ideas`), mode `lib2` tests 100 generated transforms of the strongest features for increment over the composite (plan `20261024-algopack-library-v2`), mode `libext` tests candidate columns from a CSV (for example TimesFM 3 forecasts, plan `20261025-algopack-timesfm3`) for increment over the composite, mode `lib3` tests the same candidates and five stock characteristics as moderators of the composite IC with a conviction-stratified diagnostic (plan `20261025-algopack-library-v3-moderators`), flag `-book longonly` builds the top-quintile book against the equal-weight universe (plan `20261025-algopack-thin-universe`), mode `dsr` reports the deflated Sharpe ratio and the market-beta regression of a dumped book series and `-neutral sector` demeans predictions within frozen sector lists (plan `20261025-algopack-deflation-neutrality`) (daily out-of-fold series for the meta-variable study); offline, no live path; modes `fundgen`, `fundcover`, `fundmeff` and `fundtest` (plan `20261027-algopack-fund-playbook`) generate 94 fund-playbook columns on the panel date index, report coverage and redundancy, compute the Nyholt bar and run the staged tests and six book exposure rules; `momsleeve` also takes `-sig gap|blend` (opening-gap reversal books, plan `20261027-algopack-gap-reversal-sleeve`), `-stage profile` and `-eqw`; modes `hetero` (moderator splits of a signal's IC with a Bonferroni-adjusted block bootstrap, plan `20261027-algopack-owner-100-ind-batch`) and `speed` (signal strength and market-speed proxies by year, plan `20261027-algopack-signal-speed-table`) |
| `cmd/pairscan` | ordinary/preferred share pairs (plan `20261025-algopack-pairs-arbitrage`): a causal relative-performance index with dividend gaps removed, reversion, momentum and A2-difference rows with fixed-effects regressions, circular-shift permutation p and a cost-and-carry economic rule on the `cmd/flowscan -mode matrix` file; offline, no live path; `-splits` runs the dependence splits of plan `20261027-algopack-owner-100-ind-batch` |
| `cmd/exectiming` | execution-timing analytics (plan `20261025-algopack-execution-timing`): persistence of the intraday quoted-spread profile per ticker and the saving of the predicted cheapest 30-minute window against the open, the close and the session average, from AlgoPack `obstats`; offline |
| `cmd/newsimpulse` | news impulse study (plan `20261025-algopack-news-impulse`): attention and lexicon-sentiment shocks from the ticker-tagged headline file against the abnormal return at 1, 5 and 10 days and against the next open-to-close return for after-close news, rank IC with Newey-West and placebo controls on the `cmd/flowscan -mode matrix` file; offline; `-dump` writes the count-shock variants by date and name for `alphafit -mode hetero` and the news books |
| `cmd/newsllm` | LLM headline scorer for the news-sentiment study (plan `20261026-algopack-news-llm-sentiment`): `pilot` mode (500-headline throughput, label histogram, lexicon correlation, ticker-redacted twin) and resumable `score` mode through a local Ollama model, writes `scores.csv` and a `meta.json` with the model digest and prompt hash; `cmd/newsimpulse -llm` then runs rows L1-L5 and the placebos; offline |
| `cmd/divrunup` | dividend run-up study (plan `20261025-algopack-dividend-runup`): abnormal return before and after the last-buy date against the dividend yield, cluster bootstrap and a long-only net-of-spread read on the `cmd/flowscan -mode matrix` closes and the dividend calendar; offline; `-splits` runs the dependence splits of plan `20261027-algopack-owner-100-ind-batch` |
| `cmd/calmscan` | calm-state study (plan `20261026-calm-before-move`): does a quiet stock (low volatility ratio or flat price box) rank its own 10-day abnormal return better than the rest; six pre-registered cells, placebo, replication window |
| `cmd/metascan` | meta-variable study (plan `20261024-algopack-meta-variables`): causal binary macro and exogenous states (key rate, CNY-RUB, Brent, IMOEX, gold, RVI, sanctions news, fund activity, calendar) plus Saturn, lunar and Kp negative controls, split of the composite daily IC and partial-adjustment net by state, stock-level exposure interactions, circular-shift permutation p and Nyholt M_eff threshold; modes `fetch`, `fetch2` and `fetch4` (ISS, GFZ, Binance, Open-Meteo, Yahoo and GDELT series into `data/macro-long`), `run` (wave 1), `wave2` (wave 2 rows and the descriptive diagnostics) and `wave4` (the world-data rows of plan `20261025-algopack-wave4-world-data`); read the `cmd/alphafit -mode dump` series; offline, no live path |
| `cmd/tinvestprobe` | futures order probe in the T-Bank sandbox |
| `cmd/borrowmeasure` | short-borrow terms and margin-fee measurement in the sandbox (report, no history) |
| `cmd/borrowterms` | append-only daily snapshot of the contractual short terms per ticker (`data/borrow_terms.jsonl`: `short_enabled`, `Kshort`, `Dshort`, `DshortMin`), driven by `scripts/borrow-terms-daily.sh` over `data/borrow_universe.txt` |
| `cmd/cryptofetch` | monthly bulk loader for the crypto research line (plan `20261102-crypto-transfer-46`): modes `symbols` (enumerates every USDⓈ-M perpetual that ever traded from the `data.binance.vision` bucket listing, with first and last month, so delisted contracts stay in the panel), `klines` and `funding` (resumable monthly download, unzip and normalise into `data/crypto/binance-um/`), `inventory` (coverage, last date, funding rows); offline, no keys |
| `cmd/cryptofit` | crypto transfer study (plan `20261102-crypto-transfer-46`): `costs` prints the frozen cost model per venue, execution and liquidity tier with the required gross per side, `s1` prints the break-even table of all 46 registered rows at their pre-registered turnover, `panel` reports coverage of the point-in-time contract panel (delisted names included, tokenised TradFi excluded), `book` runs one cross-sectional long-short book (signals `mom`, `rev`, `taker`, `funding`) with partial adjustment, per-name tiered taker or maker costs, realised funding and within-date permutation placebos, and `battery` runs the pre-registered W1 grid with Newey-West t, deflated Sharpe over the trial count and the halves; offline, no live path |

Training and model tooling:

| Package | Role |
|---|---|
| `cmd/exportdataset` | historical feature/label CSV for Python training |
| `cmd/trainmodel` | pooled logistic regression → `model.json` |
| `cmd/trainnewsmodel` | news sentiment classifier → `news_classifier.json` |
| `cmd/calibrate` | label diagnostics across horizons |
| `cmd/explaincheck` | check that occlusion drivers of `internal/model/explain.go` explain the probability |
| `cmd/featureparity` | Go vs Python feature-vector parity check |
| `cmd/listtqbr` | TQBR ticker universe list |

Validation and backtest:

| Package | Role |
|---|---|
| `cmd/backtest` | historical replay with `-signal-source=model\|ensemble\|rule\|csvprob`; `-members-csv` writes one row per closed ensemble trade (member probabilities of the opening decision) for the member-agreement analysis |
| `cmd/memberagree` | member-agreement analysis of the `-members-csv` files (H15, metrics 6.11): unanimous versus split trades, ISO-week cluster bootstrap, `|p-0.5|` terciles, label placebo, per-seed and per-window tables; offline, no live path |
| `cmd/walkforward` | purged walk-forward with per-window artifacts |
| `cmd/strategyvalidation` | attempts registry, DSR, PBO, reliability, consistency |
| `cmd/oosfreeze` | OOS recipe freeze (`-action record\|status`) |
| `cmd/challengercompare` | challenger vs champion from the shadow log |
| `cmd/tailprecision`, `cmd/momentumbench`, `cmd/betaregime`, `cmd/gapstress`, `cmd/alphahedge` | strategy-validation studies (see [reports/](reports/)) |

Data collection:

| Package | Role |
|---|---|
| `cmd/pricehist`, `cmd/tqbrsnap`, `cmd/intradayfetch` | MOEX ISS daily, TQBR board and 10-minute candles |
| `cmd/futoifetch`, `cmd/futoihist` | futures open interest |
| `cmd/macrofetch` | macro series (Brent, CNY/RUB, key rate, IMOEX) |
| `cmd/fundsnap`, `cmd/reportsfetch` | T-Invest fundamentals and report calendar |
| `cmd/dividendscalendar` | dividend calendar → `data/dividends.jsonl` |
| `cmd/newsfetch`, `cmd/newsimport`, `cmd/newsclean`, `cmd/newsrematch`, `cmd/tgarchive` | news archive: fetch, import, clean, re-match, Telegram archive |
| `cmd/newsscore` | score news with lexicon/regex/model |

Research studies (not in the trading path): (placebo check of the score table: `cmd/placebocheck` and `internal/placebo`, H-R8, replays cyclic-shift and per-date permuted tables through the backtest engine) and `cmd/tickerselect` and `internal/tickersel` (walk-forward and random-drop check of dropping losing tickers on the replay), `cmd/gapattrib` and `internal/gapattrib` (attribution of the research-to-live gap of the ridge book, H-R9; reads the full-universe percentiles written by `scripts/ridge_full_dump.py`), `cmd/eventstudy` (news event study), `cmd/dividendstudy` (dividend drift), `cmd/newsdecay` (reaction decay by horizon), `cmd/newsvalidate` and `cmd/newsportfolio` (news classifier validation and portfolio), `cmd/llmdirection` (zero-shot LLM direction on headlines, offline only), `cmd/leadlag` (lead-lag between tickers), `cmd/rankspread` and `cmd/xsecrank` (cross-sectional ranking on persisted OOS probabilities), `cmd/regimemask` (IMOEX regime filter over persisted walk-forward probabilities), `cmd/icaportfolio` (VAR-ICA-MGARCH portfolio). Results and verdicts: [metrics.md](metrics.md) and [plans/](plans/README.md).

## 6. Package map: `internal/*`

| Package | Role | Path |
|---|---|---|
| `internal/domain` | `FeatureContext`, `TradeSignal`, `AuditEvent`, decimal helpers | live |
| `internal/config` | YAML + `.env` loading, tickers, sectors, durations | live |
| `internal/ingestion` | `moex` (ISS), `news` (RSS/Telegram/spool, matcher, industry), `algopack` (order book, trades), `tinkoff`, `finam` clients | live |
| `internal/features` | `Builder` and price/market/event/incident/sentiment features | live |
| `internal/model` | ensemble, logistic, score table, news classifier, volatility scaling, explain, calibration, reliability | live |
| `internal/orchestrator` | ingest, cycle, signal hysteresis, shadow reconciler | live |
| `internal/risk` | hardened gate, ticker breaker, VaR/ES | live |
| `internal/executor` | paper/live/Finam executors, recovery, rejection cooldown, target position | live |
| `internal/broker` | `tinkoff` sandbox account bootstrap, snapshot, cancel | live |
| `internal/storage` | SQLite WAL: audit, signals, kill switch | live |
| `internal/alert` | `telegram` client and probability/stale-candle/PSI watch | live |
| `internal/metrics` | Prometheus metrics | live |
| `internal/proxyroute` | proxy selector for SOCKS-proxied HTTP clients: loopback, private and link-local hosts go direct, not through the tunnel | live |
| `internal/drift` | PSI feature drift | live |
| `internal/dailysummary`, `internal/pnl` | daily Telegram summary, P&L computation | live |
| `internal/filltracking`, `internal/spread`, `internal/borrowcost` | execution-quality, per-ticker spreads from audit, borrow cost | live/analysis |
| `internal/incidentcheck` | incident detector audit/check | live |
| `internal/dividends`, `internal/macro` | dividend calendar and macro series providers | live/training |
| `internal/runbook` | kill-switch reset equity rule | live |
| `internal/failover` | election, witness, peer, snapshot, heartbeat | failover |
| `internal/bus` | Redis Streams scaffolding (unused) | none |
| `internal/cryptocost` | frozen crypto cost model of the transfer plan (retail maker/taker per side, spread, impact, maker fill probability, funding over the holding period, borrow and exchange haircut, break-even and required-gross arithmetic); shared by every crypto row so no row is charged differently | analysis |
| `internal/backtest` | replay engine, ISS source, decision cache, attribution, reports | offline + preflight |
| `internal/walkforward`, `internal/strategyvalidation`, `internal/oosfreeze`, `internal/trainrun`, `internal/verifier`, `internal/tearsheet` | validation, training run, P&L report, HTML tearsheet | offline |
| `internal/metricsdoc` | lint of `docs/metrics.md` | offline |
| `internal/betaregime`, `internal/momentum`, `internal/equalweight`, `internal/gapstress`, `internal/tailprecision`, `internal/newsstudy`, `internal/icaport`, `internal/memberagree` | research libraries behind the study CLIs | research |

## 7. Scripts

| Script | Role | Runs on |
|---|---|---|
| `deploy-failover.sh`, `sync-aibox.sh` | failover deploy, ai-box sync | Mac |
| `export_ensemble.py`, `compare_models.py`, `combos.py`, `dump_parity_fixture.py` | ensemble training/export, comparison, parity fixture | Mac / ai-box |
| `timesfm_features.py` | TimesFM 3 zero-shot inference to a CSV of forecasts (research only, non-commercial weights; the single Python exception, run in an isolated virtual environment on the ai-box; plan `20261025-algopack-timesfm3`) | ai-box |
| `timesfm_features_b.py` | TimesFM 3 wave A inference (multi-length contexts, quantiles, shuffled-tail ablation, horizon 20, flow forecasts) to one CSV; same isolated-Python exception; plan `20261025-algopack-timesfm3-wave2` | ai-box |
| `daily-retrain.sh`, `daily-retrain-box.sh`, `recent-window-experiment.sh` | daily CANDIDATE retrain (report only) | Mac / ai-box cron |
| `ridge_probs.py`, `ridge_full_dump.py`, `factors_wide.py`, `ridge-scores-pull.sh`, `factor_shadow*.py`, `quantile_probs.py` | ridge score table and factor research | ai-box → Mac |
| `breadth*.sh/py`, `hypo*.sh/py`, `depth-candidate.sh`, `rerun-r2.sh`, `rerun-forward.sh`, `calendar_cycles.py`, `intraday_*.py`, `futoi_study.py`, `events_study.py`, `procvar.py`, `world_*.py`, `timesfm_features.py`, `make_candles_csv.py` | hypothesis studies | ai-box |
| `news-daily.sh`, `rebuild-news-archive.sh`, `score_news.py`, `tgarchive-live.sh`, `fundsnap-daily.sh`, `regime-fresh.sh`, `trades-record-deploy.sh` | data collection cron | Mac / ai-box |
| `borrow-terms-daily.sh` | daily short-term snapshot (`cmd/borrowterms`) | Mac |
| `launchd/` | macOS launchd units | Mac |

New code is Go; the Python scripts are legacy research tooling.

## 8. Configuration and deploy files

`config.example.yaml` is the template; `config.paper.yaml` and `config.sandbox.yaml` are ready profiles; `config.yaml`, `.env`, `watchdog.yaml`, `watchdog.env` are local and gitignored. Secrets come only from the environment (`MOEX_TRADER_TINKOFF_TOKEN`, `MOEX_TRADER_ALGOPACK_TOKEN`, Telegram token). Config struct: `internal/config/config.go`. Top-level sections: `tickers`, `model`, `moex_iss_base_url`, `algopack_base_url`, `storage`, `risk`, `commission`, `finam`, `tinkoff`, `preflight`, `broker`, `is_paper_trading`, `poll_interval`, `telegram`, `alerts`, `news`.

Artifacts in the repo root: `ensemble_model.json`, `model.json`, `news_classifier.json`, `data/ridge_scores.csv` (untracked), `artifacts/oos/freeze.json`. Failover templates and CA bundle: `deploy/`.

## 9. Observability

Prometheus `:9090`: inference duration, `signals_generated_total`, `risk_rejections_total{reason}`, `executor_skips_total{reason}`, `signal_probability`, `candle_age_seconds`, `feature_psi`, `position_notional`, `gross_exposure`, `lease_held`, `var_95/99`, `es_95/99`. Catalogue: [metrics.md](metrics.md) section 10 "Observability metrics and alerts". Telegram: signal errors, kill switch, executed decisions, cooldown alerts.
