# MOEX Multi-Agent Trader

A Go trading system for MOEX instruments. It ingests market data and news, builds a
feature context, asks a local LLM (Ollama) for a structured trade signal, passes the
signal through a risk gate, and executes it — paper trades first, then live micro-lots
via Tinkoff. Every step is recorded as an audit event for later review.

The system is a single-process modular monolith: internal Go packages talk through
interfaces, with no network hop between components. Redis Streams is an optional
internal bus for later process splitting, not a requirement for the system to work.

## Features

- MOEX ISS market data plus RSS news ingestion
- Feature-context building: price change, realized volatility, news sentiment,
  order-book imbalance
- Structured Ollama LLM decisions (`BUY` / `SELL` / `HOLD`) with strict JSON output
  and bounded retry
- Hardened risk gate: max position size, fat-finger price check, and persisted kill
  switch; daily-loss and drawdown limits require a live account snapshot (not wired yet)
- Paper executor and an idempotent Tinkoff live executor using UUID v4 order IDs
- Full audit trail in SQLite
- Prometheus metrics, optional Telegram alerting, and an hourly verifier report

## Requirements

- Go 1.26 or newer
- An Ollama server reachable from this process. The default host is
  `192.168.88.193:11434` with model `qwen3.8`. Do not run `ollama serve` on the trading
  machine itself; point the config at the shared GPU host.
- Tinkoff credentials only when using live mode (see below)

## Quick start

```sh
cp config.example.yaml config.yaml
# edit config.yaml to set tickers, Ollama host/model, and storage path
go build ./...
go run ./cmd/trader -config config.yaml
```

The trader starts an orchestration loop immediately, then repeats on
`poll_interval` (default `5m`). It also serves Prometheus metrics on port `9090`
(`-metrics-addr`, default `:9090`), reachable at `http://localhost:9090/metrics`.

Stop it with `Ctrl+C` (SIGINT/SIGTERM).

## Configuration

Configuration is YAML, with defaults applied for missing values. See
`config.example.yaml` for the full template. Real `config.yaml` and `.env` files are
gitignored.

Key fields:

```yaml
tickers: [YDEX, OZON, SBER, ...]
ollama:
  host: "192.168.88.193:11434"
  model: "qwen3.8"
  timeout: 10s
moex_iss_base_url: "https://iss.moex.com/iss"
storage:
  path: "./trader.db"
risk:
  max_lots: 1
is_paper_trading: true
poll_interval: 5m
telegram:
  bot_token: ""
  chat_id: ""
```

Every value can also be overridden with an environment variable, which is applied
after the YAML is parsed:

| Config field | Environment variable |
|---|---|
| `tickers` | `MOEX_TRADER_TICKERS` (comma separated) |
| `ollama.host` | `MOEX_TRADER_OLLAMA_HOST` |
| `ollama.model` | `MOEX_TRADER_OLLAMA_MODEL` |
| `ollama.timeout` | `MOEX_TRADER_OLLAMA_TIMEOUT` |
| `moex_iss_base_url` | `MOEX_TRADER_MOEX_ISS_URL` |
| `storage.path` | `MOEX_TRADER_STORAGE_PATH` |
| `risk.max_lots` | `MOEX_TRADER_RISK_MAX_LOTS` |
| `poll_interval` | `MOEX_TRADER_POLL_INTERVAL` |
| `is_paper_trading` | `MOEX_TRADER_IS_PAPER_TRADING` |
| `telegram.bot_token` | `MOEX_TRADER_TELEGRAM_BOT_TOKEN` |
| `telegram.chat_id` | `MOEX_TRADER_TELEGRAM_CHAT_ID` |

## Paper vs live mode

Paper mode is the default (`is_paper_trading: true`). In paper mode, signals are
recorded as virtual fills in SQLite by `PaperExecutor`; no external order is sent.
Account-based risk limits (daily loss and drawdown kill switch) are disabled because
there is no live account snapshot source; the max-lot and fat-finger checks still apply.

Live order placement is implemented in `internal/executor/live.go`, but the current
`cmd/trader` entrypoint has no Tinkoff orders/account wiring. It therefore refuses to
start when `is_paper_trading: false` (or `MOEX_TRADER_IS_PAPER_TRADING=false`) rather
than running with silently bypassed live protections. Connect the Tinkoff credentials,
account snapshot, and order canceller before enabling real money, then follow
`docs/GO_LIVE_CHECKLIST.md`.

## Commands

- `cmd/trader` — runs the full ingest → features → LLM → risk → executor loop; use
  `-reset-kill-switch` to clear a persisted kill switch and exit
- `cmd/llmbench` — sends sample feature contexts to Ollama and reports success rate
  and latency percentiles
- `cmd/verifier` — reads audit events and produces a markdown report correlating
  signals with realized P&L

Example:

```sh
go run ./cmd/llmbench -host 192.168.88.193:11434 -model qwen3.8 -n 100
go run ./cmd/verifier -db trader.db -since 1h -interval 1h
```

## Package layout

```text
cmd/
  trader/     orchestrator entrypoint
  llmbench/   Ollama latency benchmark
  verifier/   audit-based trade verifier
internal/
  config/     YAML + env config loading and defaults
  domain/     FeatureContext, TradeSignal, AuditEvent
  features/   feature-context builder
  ingestion/  MOEX ISS, news, AlgoPack, and Tinkoff market-data clients
  llm/        Ollama client, prompt builder, JSON schema, decision engine
  risk/       risk gate and kill switch
  executor/   paper and live (Tinkoff) executors
  storage/    SQLite store with WAL and audit/signal persistence
  orchestrator/ cycle loop and signal source adapters
  bus/        optional Redis Streams signal bus
  metrics/    Prometheus metrics
  alert/telegram/ Telegram alert client
  verifier/   markdown trade-adequacy reports
```

## Signal pipeline

1. Ingest — `internal/ingestion` fetches MOEX security data, candles, and RSS news,
   then matches articles to tickers.
2. Features — `internal/features.Builder` turns the ingested input into a
   `domain.FeatureContext` (return, volatility, sentiment, order-book imbalance).
3. LLM — `internal/llm.DecisionEngine` sends the feature context to Ollama with a
   strict-JSON prompt, retries on parse failure, and falls back to `HOLD`.
4. Risk gate — `internal/risk.Gate` validates the signal and rejects it if it exceeds
   the max position or fat-finger limits; with a live account snapshot it also enforces
   daily-loss and drawdown limits and trips the kill switch.
5. Executor — `internal/executor` either records a paper fill or sends a real Tinkoff
   order.
6. Audit — every stage is written to `audit_events` in SQLite for the verifier and
   debugging.

## Testing

```sh
go test ./...
go vet ./...
gofmt -l .
```

All external services are mocked in tests (`httptest.Server`, in-memory fakes,
`miniredis`); tests do not make real network calls.

## Go-live

Before running real money, follow `docs/GO_LIVE_CHECKLIST.md`. External credentials,
infrastructure provisioning, and real-time soak tests are post-completion tasks and
must be done manually.
