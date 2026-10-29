# Development and operations guide

Russian translation: [DEVELOPMENT.ru.md](DEVELOPMENT.ru.md). This file is canonical.

How to run, configure, test and change the project. System structure lives in [ARCHITECTURE.md](ARCHITECTURE.md); rules and invariants in [../AGENTS.md](../AGENTS.md).

## 1. Commands

```sh
go build ./...

go run ./cmd/trader -config config.yaml                 # paper
go run ./cmd/trader -config config.sandbox.yaml         # T-Bank sandbox
go run ./cmd/trader -reset-kill-switch                  # reset the kill switch and exit
go run ./cmd/sandboxcheck -config config.sandbox.yaml [-ticker SBER] [-place-order]

go run ./cmd/exportdataset -config config.yaml -from 2024-09-01 -split 2026-06-18 -out /tmp/moex-dataset.csv
DATASET=/tmp/moex-dataset.csv ENSEMBLE_OUT=ensemble_model.json python3 scripts/export_ensemble.py
go run ./cmd/trainmodel -config config.yaml             # fallback logistic regression -> model.json
go run ./cmd/calibrate -config config.yaml -out report-calibration.md
go run ./cmd/listtqbr -out data/train_tickers.json

go run ./cmd/backtest -signal-source=ensemble -ensemble-path ensemble_model.json
go run ./cmd/verifier -db trader.db -since 24h -interval 1h

go run ./cmd/tailprecision -config config.sandbox.yaml -out report-tailprecision.md
go run ./cmd/momentumbench -config config.sandbox.yaml -out report-momentumbench.md
go run ./cmd/betaregime -config config.sandbox.yaml -out report-betaregime.md
go run ./cmd/gapstress -config config.sandbox.yaml -portfolio artifacts/portfolio.json
go run ./cmd/borrowmeasure -config config.sandbox.yaml -out report-borrow.md
go run ./cmd/borrowterms -config config.sandbox.yaml -tickers "$(cat data/borrow_universe.txt)" -out data/borrow_terms.jsonl
go run ./cmd/oosfreeze -action status
go run ./cmd/strategyvalidation
```

Tests and lint: `make check` (`go test ./...` + `go vet ./...`) and `gofmt -l .`. CI (`.github/workflows/ci.yml`) runs `make check` on every push and PR with the Go version from `go.mod`. External services are mocked in tests (`httptest`, in-memory fakes); tests never use the network. `make check` and `gofmt -l .` are mandatory before a commit. `internal/docscheck` fails the build when a doc lacks its `.ru.md` twin, an English doc contains Cyrillic, a package is missing from `docs/ARCHITECTURE*.md`, or a relative link is broken.

## 2. Feature vector

The order is defined once in `internal/model/features.go` (`defaultFeatureOrder`, `ToVector`): 28 names, the models are trained on 18 of them.

```
return_pct, realized_volatility, news_sentiment, news_count, order_book_imbalance,
mom_5d, mom_21d, mom_63d, reversal_1d, rsi_14, dist_ma20_pct, dist_ma50_pct,
realized_vol_21d_annualized_pct, volume_zscore_20d,
macd_hist_pct, stoch_k_14, williams_r_14, alligator_spread_pct,
event_dividend, event_buyback, event_sanctions, event_ipo, event_report,
event_delisting, event_mna, event_default, negotiations_signal, sanctions_signal
```

Oscillators are computed in `internal/features/price_features.go`. The order must match in three places: `internal/model/features.go`, `FEATURES` in `scripts/export_ensemble.py` and in `scripts/compare_models.py`; `cmd/exportdataset` builds its CSV header and rows from `model.ToVector`. A mismatch is a hard failure at model load (`checkFeatureOrder`) and in preflight. In the deployed `ensemble_model.json` the `event_*`, `negotiations_signal` and `sanctions_signal` coefficients are zero. Inference invariants (float32 for XGBoost, `base_score` handling): [knowledge/feature-pipeline-and-inference.md](knowledge/feature-pipeline-and-inference.md).

## 3. Models

- **Score table** (`model.score_table_path`, live in the sandbox profile): daily per-ticker probability from `data/ridge_scores.csv` (`ticker,date,p`), produced offline by `scripts/ridge_probs.py`; pull to the Mac with `scripts/ridge-scores-pull.sh`. Max staleness 7 days; BUY at `p >= 0.60`, SELL at `p <= 0.40`.
- **Ensemble** (`model.ensemble_path`): LightGBM + XGBoost + logistic regression, `p = (p_lgb + p_xgb + p_lr) / 3`, thresholds in the artifact (`0.60`/`0.40`), `Confidence = |p - 0.5| * 2`. Training: `cmd/exportdataset` (label = excess return over IMOEX, 5-day horizon, 0.5% dead band; `-news-history` mixes in real news features) → `scripts/export_ensemble.py` → `ensemble_model.json`; compare with `scripts/compare_models.py`.
- **Fallback** (`model.path`): pooled logistic regression from `cmd/trainmodel` (pure Go).
- **Challenger**: `model.challenger_ensemble_path` scores every vector in shadow; see AGENTS.md.

Diagnostics that never touch the trading path: `cmd/calibrate`, `cmd/leadlag`, `cmd/tailprecision`, `cmd/momentumbench`, `cmd/betaregime`, `cmd/gapstress`, `cmd/borrowmeasure`, `cmd/borrowterms` (daily append-only snapshot, `scripts/borrow-terms-daily.sh`), `cmd/oosfreeze`, `cmd/strategyvalidation`, `cmd/newsfetch`.

## 4. Backtest and preflight

`internal/backtest/engine.go` is the live path without the network:

- daily MOEX ISS candles (`ISSSource`, correct paging; warm-up covers the full 300-bar feature lookback, 434 calendar days, so EMA/SMMA indicators converge as in live and training);
- on day `d` features are built from candles `[:d]`, entry at `candles[d+1].Open`, mark-to-market at `candles[d].Close`;
- the signal passes the same `risk.HardenedGate`; commission is charged on entry and exit;
- result: net/gross P&L, commissions, hit rate, Sharpe, max drawdown, decision count, hold reasons (including `error`/`timeout`), realized/unrealized split, borrow cost, per-ticker attribution.

`cmd/backtest -signal-source`: `model` (default, `model.json`), `ensemble` (needs `-ensemble-path`), `rule` (`-reversal-threshold`), `csvprob` (probabilities from CSV). Useful flags: `-commission-rate` (default `0.0005`), `-lookback-days`, `-min-confidence`, `-kill-switch`, `-cache`, `-out`, `-tearsheet`, `-spread-pct`/`-slippage-pct`, `-spread-db`/`-spread-min-obs`, `-borrow-pct-day`, `-max-net-exposure`/`-max-net-short`/`-max-net-long`, `-sector-caps`/`-sectors`, `-wf-dir`, `-ensemble-member lgb|xgb|logistic`. `-members-csv <path>` (with `-signal-source ensemble`) writes one row per closed trade with the member probabilities of the decision that opened it, plus `<path>.meta.json` with the unmatched count; `-members-window` and `-members-seed` label the rows and `cmd/memberagree -in <file|dir> [-out report.md]` turns them into the member-agreement report.

Other flags: `-embargo-bars N` (`cmd/walkforward`, `cmd/backtest -wf-dir`, `cmd/trainmodel`); `cmd/strategyvalidation -returns-dirs a,b,c` (PBO), `-reliability <wf-dir>`, `-consistency <wf-dir>`; `-label-mode absolute_tr` with `-dividends` (`cmd/exportdataset`, `cmd/trainmodel`); `-train-tickers`/`-train-min-turnover` (`cmd/exportdataset`); `-vol-scale*` (`cmd/backtest`, `cmd/walkforward`; in `cmd/walkforward` only together with `-target-notional`).

Preflight in `cmd/trader` runs the same engine over the last `preflight.days` (90) with `preflight.deposit`, `preflight.spread_pct`/`preflight.slippage_pct` (0.05% each by default) and the live base signal source. The threshold applies to **realized** P&L. Refusal conditions are listed in [ARCHITECTURE.md](ARCHITECTURE.md) section 2.4.

## 5. Execution profiles

**Paper** (`broker: paper`, `is_paper_trading: true`, default): virtual fills with commission `price x lots x commission.rate`; no account snapshot, so daily-loss and drawdown checks are off; lot limit and fat-finger stay on.

**T-Bank sandbox** (`broker: tinkoff`, `is_paper_trading: false`, `tinkoff.sandbox: true`) is the working contour. Template: `config.sandbox.yaml`.

```yaml
broker: "tinkoff"
is_paper_trading: false
commission:
  rate: "0.0005"
tinkoff:
  endpoint: "sandbox-invest-public-api.tbank.ru:443"
  sandbox: true
  account_id: ""
  pay_in: "1000000"
  order_type: "market"
```

- `EnsureAccount` reuses the first account or opens one and tops it up by `pay_in`; pin the id in `tinkoff.account_id`.
- `Snapshot` gives the gate `Deposit = pay_in`, `DayStartEquity` and `CurrentEquity`.
- Orders are idempotent (UUID v4 `OrderID`); `market` fills at the last price.
- The kill switch cancels only orders on the bot's instruments (`TestCancelOpenOrdersLeavesForeignOrdersAlone`).
- The sandbox accrues no dividends or taxes and has no real slippage; P&L is approximate.
- The bot's current position is derived from its own executor events; manual positions on the same account are invisible to it (a SELL there opens a short).

**Live**: intentionally not wired. See [GO_LIVE_CHECKLIST.md](GO_LIVE_CHECKLIST.md).

Risk defaults (`risk.DefaultConfig()`): daily loss 0.5% of deposit, fat-finger 2%, drawdown 3% to kill switch. Per-ticker breaker (`risk.circuit_breaker_max_losses`, `_max_loss_pct`), PSI drift (`risk.drift_psi_*`, warn only), rebalance hysteresis (`risk.rebalance_min_deviation_pct`).

## 6. Storage and observability

SQLite (`modernc.org/sqlite`, WAL, busy timeout 5000 ms) at `storage.path`: `audit_events` (stage JSON), `trade_signals`, `kill_switch`. Prometheus on `:9090` (`-metrics-addr`); metric catalogue in [metrics.md](metrics.md). Telegram: signal errors, kill switch, executed decisions (filter `telegram.signal_tickers`), cooldown alerts (`internal/alert/telegram/watch.go`). Telegram needs `telegram.proxy` from this network (`socks5://127.0.0.1:3333`).

## 7. Configuration

YAML plus a `.env` next to it (real environment variables win). Secrets only from the environment. Templates: `config.example.yaml`, `config.paper.yaml`, `config.sandbox.yaml`; `config.yaml` and `.env` are gitignored; variable names in `.env.example`.

| Field | Env |
|---|---|
| `tickers` | `MOEX_TRADER_TICKERS` (comma-separated) |
| `model.path` | `MOEX_TRADER_MODEL_PATH` |
| `model.ensemble_path`, `model.score_table_path`, `model.challenger_*` | YAML only |
| `news.classifier_path` | `MOEX_TRADER_NEWS_CLASSIFIER_PATH` |
| `moex_iss_base_url` | `MOEX_TRADER_MOEX_ISS_URL` |
| `algopack_base_url` / `algopack_token` | `MOEX_TRADER_ALGOPACK_BASE_URL` / `_TOKEN` (secret) |
| `storage.path` | `MOEX_TRADER_STORAGE_PATH` |
| `risk.max_lots`, `risk.target_notional`, `risk.rebalance_min_deviation_pct` | `MOEX_TRADER_RISK_MAX_LOTS`, `_TARGET_NOTIONAL`, `_REBALANCE_MIN_DEVIATION_PCT` |
| `risk.circuit_breaker_*`, `risk.drift_psi_*`, `risk.max_slippage_pct`, `risk.no_trade_after_open_minutes`, `risk.blackout_windows`, `risk.max_net_*`, `risk.vol_scale`, `risk.sector_caps`, `risk.sectors` | YAML only |
| `commission.broker` / `rate` | `MOEX_TRADER_COMMISSION_BROKER` / `_RATE` |
| `tinkoff.endpoint` / `token` / `sandbox` / `account_id` / `pay_in` / `order_type` | `MOEX_TRADER_TINKOFF_*` (`_TOKEN` is a secret) |
| `finam.base_url` / `secret_token` | `MOEX_TRADER_FINAM_SECRET_TOKEN` (secret) |
| `preflight.enabled` / `days` / `deposit` / `min_net_pnl` | `MOEX_TRADER_PREFLIGHT_*` |
| `preflight.min_closed_trades` / `spread_pct` / `slippage_pct` | YAML only |
| `broker` (`paper` / `tinkoff` / `finam`) | `MOEX_TRADER_BROKER` |
| `poll_interval`, `is_paper_trading` | `MOEX_TRADER_POLL_INTERVAL`, `MOEX_TRADER_IS_PAPER_TRADING` |
| `telegram.bot_token` / `chat_id` / `signal_tickers` / `proxy` | `MOEX_TRADER_TELEGRAM_*` (`_BOT_TOKEN` is a secret) |

`alerts.*` (`probability_collapse_window`, `stale_candle_age`, `cooldown`) has active defaults (`20`, `24h`, `1h`) and is silent only without Telegram credentials. If `news.classifier_path` is empty or the file fails to load, the trader logs a warning and falls back to the keyword lexicon.

## 8. Failover operations

Topology and rules: [ARCHITECTURE.md](ARCHITECTURE.md) section 3 and AGENTS.md.

```sh
./scripts/deploy-failover.sh witness     # VPS: freebsd/amd64 build + rc.d
./scripts/deploy-failover.sh aibox       # ai-box: binaries, configs, CA, tunnel key
./scripts/deploy-failover.sh mac         # Mac: launchd unit + fresh trader
./scripts/deploy-failover.sh start-aibox # enable the standby unit
./scripts/deploy-failover.sh status      # /healthz of all three nodes
```

Tokens `MOEX_WITNESS_TOKEN` and `MOEX_FAILOVER_TOKEN` live in `watchdog.env` (0600, gitignored) on both machines and in `/usr/local/etc/moex-witness.token` on the VPS. The ai-box needs the Russian root CA: `certs/ca-bundle.pem` (system CAs + `deploy/certs/russian-trusted-ca.pem`) via `SSL_CERT_FILE`. Templates in `deploy/` contain placeholders (`<aibox-lan-ip>`, `<mac-lan-ip>`, `<witness-host>`, `<vps-host>`, `<telegram-chat-id>`) and `/Users/you/...`; replace them or override with `AIBOX`, `AIBOX_DIR`, `VPS` env variables. The preflight gate behaves identically on both nodes: if the model fails the gate nobody trades, which is correct behaviour, not a failover fault.

## 9. Change process

Plans live in `docs/plans/YYYYMMDD-<topic>.md` (`Overview` → `Context` → `Development Approach` → `Tasks` with `- [ ]`); examples in `docs/plans/completed/`. Execution: `ralphex --tasks-only --max-iterations N <plan>` (one conventional commit per task). Review: revmux review-fix loop until a round without gating findings. Finalisation: the plan moves to `docs/plans/completed/` in the same commit that puts its numbers into `docs/metrics.md`. Small edits (docs, config, one function) may go directly; the invariants in AGENTS.md always apply.

Every change updates documentation in the same commit: English `X.md` first, then its `X.ru.md`; structural code changes update `docs/ARCHITECTURE.md` and `docs/ARCHITECTURE.ru.md`.

### Recipes

**Add a feature.** (1) field in `domain.FeatureContext`; (2) computation in `internal/features`; (3) `ToVector`/`defaultFeatureOrder`; (4) the CSV header follows `ToVector` automatically; (5) `FEATURES` in `scripts/export_ensemble.py` and `scripts/compare_models.py`; (6) retrain and compare validation AUC (past oscillator features moved AUC by +-0.005, which is noise: data first, features second).

**New signal source.** Implement both `orchestrator.SignalSource` (`Generate(ctx, feature)` → BUY/SELL/HOLD) and `backtest.SignalSource`; wire it in `newModelSignalSource` (`cmd/trader`) and `buildSignalSource` (`cmd/backtest`). Preflight must use the same source.

**New broker.** Client in `internal/ingestion/<broker>`, executor in `internal/executor`, a branch in `newBrokerRuntime`, a check in `cmd/sandboxcheck`, then the GO_LIVE_CHECKLIST items (account snapshot, order cancellation, idempotency, commission).

**Retrain the model.** Manual only: export → train → compare AUC and backtest → replace the artifact → restart `cmd/trader` (preflight re-verifies). Steps: [plans/completed/20261018-manual-retrain.md](plans/completed/20261018-manual-retrain.md). No cron auto-swap.

### Known pitfalls

- XGBoost/LGBM export: only `save_model`/`save_raw(raw_format="json")`, never `dump_model()`.
- LGBM converter: put the `-1` child placeholder first, then overwrite (`lc[i] = walk(...)`), otherwise flat-array indices get mixed up.
- Finmarket RSS is Windows-1251 (`charset.NewReaderLabel`).
- AlgoPack silently turns off with one log line when `MOEX_TRADER_ALGOPACK_TOKEN` is missing.
- `SBMM` is not tradable through the T-Invest API (`api_trade_available_flag=false`).
- `internal/bus` is wired nowhere; do not look for live logic in it.
- ISS caps pages at 500 rows: pagers must stop on an empty page, not a short one.

## 10. Model quality in one paragraph

Validation AUC of all ensemble members is about 0.50-0.52, at the edge of noise; the bottleneck is data volume and breadth, not features. Order of priority: more tickers, more history, only then features and thresholds. Current numbers and verdicts: [metrics.md](metrics.md); history of rejected ideas: [knowledge/model-quality-history.md](knowledge/model-quality-history.md).
