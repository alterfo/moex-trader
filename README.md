# MOEX Multi-Agent Trader

A Go trading system for MOEX instruments. It ingests market data and news, builds a
   feature context, asks a local LLM (Ollama) for a structured trade signal, passes the
   signal through a risk gate, and executes it — paper trades first, then live micro-lots
   via Tinkoff or Finam. Every step is recorded as an audit event for later review.

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
- Paper executor plus a live executor for Tinkoff and Finam; Tinkoff orders are
  idempotent via UUID v4 order IDs, while Finam order idempotency is pending Finam
  API confirmation
- Full audit trail in SQLite
- Prometheus metrics, optional Telegram alerting, and an hourly verifier report

## Requirements

- Go 1.26 or newer
- An Ollama server reachable from this process. The default host is
  `192.168.88.193:11434` with model `qwen3.8`. Do not run `ollama serve` on the trading
  machine itself; point the config at the shared GPU host.
- Broker credentials only when using live mode (Tinkoff or Finam; see below)

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
algopack_base_url: "https://apim.moex.com/iss/datashop"
storage:
  path: "./trader.db"
risk:
  max_lots: 1
commission:
  broker: "tinkoff"
  rate: "0.003"    # 0.3%, T-Bank "Инвестор" tariff — flat rate, no per-trade minimum
finam:
  base_url: "https://api.finam.ru"
broker: "paper"     # paper | tinkoff | finam
is_paper_trading: true
poll_interval: 5m
telegram:
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
| `algopack_base_url` | `MOEX_TRADER_ALGOPACK_BASE_URL` |
| `algopack_token` | `MOEX_TRADER_ALGOPACK_TOKEN` (secret, env-only) |
| `storage.path` | `MOEX_TRADER_STORAGE_PATH` |
| `risk.max_lots` | `MOEX_TRADER_RISK_MAX_LOTS` |
| `commission.broker` | `MOEX_TRADER_COMMISSION_BROKER` |
| `commission.rate` | `MOEX_TRADER_COMMISSION_RATE` |
| `finam.base_url` | `MOEX_TRADER_FINAM_BASE_URL` |
| `finam.secret_token` | `MOEX_TRADER_FINAM_SECRET_TOKEN` (secret, env-only) |
| `broker` | `MOEX_TRADER_BROKER` (`paper` / `tinkoff` / `finam`) |
| `poll_interval` | `MOEX_TRADER_POLL_INTERVAL` |
| `is_paper_trading` | `MOEX_TRADER_IS_PAPER_TRADING` |
| `telegram.bot_token` | `MOEX_TRADER_TELEGRAM_BOT_TOKEN` |
| `telegram.chat_id` | `MOEX_TRADER_TELEGRAM_CHAT_ID` |

## Paper vs live mode

Paper mode is the default (`is_paper_trading: true`). In paper mode, signals are
recorded as virtual fills in SQLite by `PaperExecutor`; no external order is sent.
Account-based risk limits (daily loss and drawdown kill switch) are disabled because
there is no live account snapshot source; the max-lot and fat-finger checks still apply.

Live order placement is implemented for both Tinkoff (`internal/executor/live.go`) and
Finam (`internal/executor/finam.go`), but the current `cmd/trader` entrypoint has no
orders/account wiring for either broker. It therefore refuses to start when
`broker: tinkoff` or `broker: finam` is selected (or `is_paper_trading: false`) rather
than running with silently bypassed live protections. Connect the broker credentials,
account snapshot, and order canceller before enabling real money, then follow
`docs/GO_LIVE_CHECKLIST.md`.

The Finam integration mirrors the Tinkoff client pattern. Its market-data and order
client (`internal/ingestion/finam`) authenticates by exchanging a long-lived secret
token for a JWT, caches the JWT, and refreshes it one minute before its 15-minute
expiry (or reactively on a 401). Finam publishes a demo account for exercising this
flow without real money; the secret token is obtained manually from the Finam tokens
portal and is never committed. The order-placement response does not include an
execution price, so the recorded fill price is the signal price used to place the
market order; treat Finam P&L as an estimate until real fills are reconciled.

Commission is estimated per fill as `price x lots x commission.rate`. The verifier
reports gross P&L, total commission, and net realized P&L; a trade that is
gross-positive can still be reported as a loss once commission is subtracted.

## Commands

- `cmd/trader` — runs the full ingest → features → LLM → risk → executor loop; use
  `-reset-kill-switch` to clear a persisted kill switch and exit
- `cmd/llmbench` — sends sample feature contexts to Ollama and reports success rate
  and latency percentiles; use `-host`, `-model`, `-timeout`, and `-n`
- `cmd/verifier` — reads audit events and produces a markdown report correlating
  signals with realized P&L; use `-db`, `-since`, `-interval`, and `-out`

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
  ingestion/  MOEX ISS, news, AlgoPack, Tinkoff, and Finam market-data clients
  llm/        Ollama client, prompt builder, JSON schema, decision engine
  risk/       risk gate and kill switch
  executor/   paper and live (Tinkoff and Finam) executors
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
5. Executor — `internal/executor` either records a paper fill or sends a real Tinkoff or
   Finam order.
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
