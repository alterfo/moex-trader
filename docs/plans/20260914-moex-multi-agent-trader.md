# MOEX Multi-Agent Trader

## Overview

A Go trading system for MOEX instruments: ingests market data + news, builds a
`FeatureContext`, asks a local LLM (Ollama) for a structured `TradeSignal`, passes it
through a `RiskGate`, and executes it (paper first, then live micro-lots via Tinkoff).
Every step is recorded as an `AuditEvent` for later review.

Architecture is a **single-process modular monolith** (internal Go packages talking
through interfaces, no network hop between components) — per the source spec, this
trades "fashionable multi-agent" for reliability, latency and predictability. Redis
Streams (Phase 3) is an optional internal bus for later process-splitting, not a
requirement for the system to work.

There is no prior Phase 1 anywhere on disk — this plan designs it from scratch, then
builds Phases 2–4 as originally specified by the user. It lives in its own repo
(`~/dev/fin/moex-trader`), separate from the Python `finanalys` project, which remains
an unrelated, untouched reference for ideas (ticker list, news-source weights) only.

## Context (from discovery)

- Sibling project `finanalys` (`~/dev/fin/finanalys`) is a Python MOEX news-sentiment
  tool. Not reused as code — only its 19-instrument ticker list (`config.py`
  `INSTRUMENTS`) and RSS news-source list are ported as a starting point.
- Ollama runs on aibox at `192.168.88.193:11434` (RTX 4090, shared GPU — check
  `nvidia-smi` free VRAM before assuming headroom). Never run `ollama serve` locally.
- No existing Go code, no existing Tinkoff/Redis/Prometheus integration anywhere.
- `ralphex` CLI v1.6.1 is installed and will execute this plan task by task.

## Development Approach

- **Testing approach**: Regular (implementation first, tests immediately after, in the
  same task — not full TDD).
- Complete each task fully, with passing tests, before moving to the next.
- Single Go module, package-per-concern under `internal/`, thin `cmd/` entrypoints.
- All money/price values use `github.com/shopspring/decimal.Decimal` — never `float64`.
- SQLite (`modernc.org/sqlite`, pure Go, no cgo) opened with
  `?_journal_mode=WAL&_busy_timeout=5000`.
- Real external integrations (Tinkoff live orders, Redis, Prometheus, Telegram,
  running the system for days/weeks with real or paper money) are scoped into this
  plan's Phase 3–4 tasks as *code* (client, wiring, tests against mocks) — but
  obtaining credentials, provisioning infra, and actually running the system
  unattended are **Post-Completion** items, not checkboxes, since no agent can do
  those.
- **CRITICAL: every task MUST include new/updated tests.**
- **CRITICAL: all tests must pass before starting next task.**
- **CRITICAL: update this plan file when scope changes during implementation.**

## Testing Strategy

- Unit tests (`go test ./...`) for every task, table-driven where it fits Go idiom.
- External services (Ollama, MOEX ISS, Tinkoff, Redis, Telegram) are mocked via
  `httptest.Server` / in-memory fakes — no real network calls in tests.
- No UI, so no e2e/browser tests.

## Progress Tracking

- Mark completed items with `[x]` immediately when done.
- Add newly discovered tasks with ➕ prefix.
- Document issues/blockers with ⚠️ prefix.

## What Goes Where

- **Implementation Steps** (checkboxes): everything buildable and testable inside this
  repo without external accounts/infra.
- **Post-Completion** (no checkboxes): obtaining API tokens, provisioning Redis/
  Prometheus/Grafana/Telegram, running paper/live soak tests over real time, manual
  position-size ramp-up decisions.

## Implementation Steps

### Phase 1: Foundation

### Task 1: Project skeleton and config
- [x] `go.mod` (`github.com/olegsidorkin/moex-trader`), directories: `cmd/trader`,
      `internal/{config,domain,storage,ingestion,features,llm,risk,executor,audit,orchestrator}`
- [x] `internal/config`: load YAML (or env) config — ticker list (port the 19
      instruments from finanalys `config.py`), Ollama host/model (default
      `192.168.88.193:11434`, `qwen3.8`), MOEX ISS base URL, SQLite path, poll interval,
      `is_paper_trading` flag
- [x] `config.example.yaml` committed; real `config.yaml`/`.env` gitignored
- [x] write tests for config loading (valid file, missing file, malformed YAML,
      missing required field)
- [x] run tests — must pass before task 2

### Task 2: Domain types
- [x] `internal/domain`: `FeatureContext`, `TradeSignal` (Action enum
      BUY/SELL/HOLD, Confidence, Reasoning, TargetLots), `AuditEvent` — all
      price/money fields as `decimal.Decimal`
- [x] validation methods (`TradeSignal.Validate()`: confidence in [0,1], action is a
      known enum value, ticker non-empty)
- [x] write tests for validation (success + each invalid-field case)
- [x] run tests — must pass before task 3

### Task 3: SQLite storage layer
- [x] `internal/storage`: open DSN with `_journal_mode=WAL&_busy_timeout=5000` via
      `modernc.org/sqlite`
- [x] schema + migration for `audit_events` and `trade_signals` tables
- [x] `InsertAuditEvent`, `ListAuditEvents(since time.Time)`, `InsertTradeSignal`
- [x] write tests against a temp-file SQLite DB (insert/list round-trip, constraint
      violation, concurrent-write-under-WAL smoke test)
- [x] run tests — must pass before task 4

### Task 4: MOEX ISS data ingestion + news fetcher
- [x] `internal/ingestion/moex`: fetch last price / candles per ticker from the public
      MOEX ISS API
- [x] `internal/ingestion/news`: port the RSS source list + trust weights from
      finanalys `config.py`, fetch + parse RSS, match articles to tickers by alias
- [x] write tests with `httptest.Server` fixtures (valid response, HTTP error,
      malformed JSON/XML, timeout)
- [x] run tests — must pass before task 5

### Task 5: FeatureContext builder
- [x] `internal/features`: combine price data + matched news + basic technical
      features (return, realized volatility) into a `FeatureContext` per ticker
- [x] write tests with fixture inputs (full data, missing news, missing price data)
- [x] run tests — must pass before task 6

### Task 6: Paper executor and risk-gate stub
- [x] `internal/executor`: `Executor` interface + `PaperExecutor` (simulated fill at
      last known price, persisted via storage)
- [x] `internal/risk`: `Gate` interface + basic stub (rejects if `TargetLots` exceeds a
      configured max — hardened rules land in Task 17)
- [x] write tests: paper fill simulation, risk gate approve/reject on the stub rule
- [x] run tests — must pass before task 7

### Task 7: Orchestration loop
- [x] `internal/orchestrator`: ticker-driven loop — per configured instrument:
      ingest → build FeatureContext → signal source (Phase 1: simple rule-based stub,
      swapped for the LLM in Task 11) → RiskGate → Executor → AuditEvent
- [x] `cmd/trader/main.go`: wire config + all components, graceful shutdown on
      SIGINT/SIGTERM
- [x] write tests: run N loop ticks against fakes, assert audit events recorded and
      shutdown is clean
- [x] run full test suite — must pass before task 8

### Phase 2: LLM Integration

### Task 8: Ollama client wrapper
- [x] `internal/llm`: wrap the Ollama API client pointed at the configured host/model,
      `Chat` call using `Format: "json"`
- [x] JSON Schema for `TradeSignal` passed alongside the request
- [x] write tests against a mocked HTTP server (valid JSON reply, malformed JSON,
      request timeout)
- [x] run tests — must pass before task 9

### Task 9: Prompt template and few-shot examples
- [ ] system prompt builder taking a `FeatureContext`, explicit instruction "Отвечай
      ТОЛЬКО валидным JSON, без markdown-оберток и пояснений" (Go-Live checklist item)
- [ ] embed 3–5 few-shot examples (varied tickers/sentiment/actions) directly in the
      system prompt
- [ ] write tests verifying prompt assembly for several `FeatureContext` inputs
      (contains schema, contains few-shot block, contains the strict-JSON instruction)
- [ ] run tests — must pass before task 10

### Task 10: Response validation and bounded retry
- [ ] parse LLM response into `TradeSignal`, run `Validate()`; on invalid JSON/schema,
      retry up to 2 times; on exhausted retries, fall back to a HOLD signal and log an
      `AuditEvent` noting the failure
- [ ] write tests: valid on first try, invalid-then-valid on retry, exhausted retries
      → HOLD fallback
- [ ] run tests — must pass before task 11

### Task 11: Wire LLM into the orchestrator + latency benchmark
- [ ] replace the Phase 1 stub signal source in `internal/orchestrator` with the real
      LLM call (context timeout applied per call)
- [ ] add a lightweight latency measurement around the LLM call (logged; full
      Prometheus histogram is Task 15)
- [ ] `cmd/llmbench`: small CLI/test harness running N sample `FeatureContext`s through
      the LLM and reporting success rate + average/percentile latency
- [ ] write tests: orchestrator integration test with a mocked LLM client
- [ ] run full test suite — must pass before task 12

### Phase 3: Paper Trading Realism

### Task 12: Tinkoff Invest API client (market data)
- [ ] `internal/ingestion/tinkoff`: client using the Tinkoff Invest Go SDK for market
      data (quotes/candles), gated behind `is_paper_trading` config flag
- [ ] write tests with mocked gRPC/HTTP responses (valid data, auth error, stream
      disconnect/reconnect)
- [ ] run tests — must pass before task 13

### Task 13: MOEX AlgoPack enrichment worker
- [ ] `internal/ingestion/algopack`: async worker enriching `FeatureContext` with
      additional AlgoPack fields (e.g. order-book imbalance), fanning results into the
      feature builder from Task 5
- [ ] write tests with fixture AlgoPack payloads (success, partial data, worker
      timeout)
- [ ] run tests — must pass before task 14

### Task 14: Redis Streams signal bus (optional decoupling)
- [ ] `internal/bus`: publish `TradeSignal` + `AuditEvent` to a Redis Stream; consumer
      stub that can later run as a separate process
- [ ] write tests using an in-memory/fake Redis (e.g. `miniredis`)
- [ ] run tests — must pass before task 15

### Task 15: Prometheus metrics
- [ ] `internal/metrics`: `llm_inference_duration_seconds` (histogram),
      `signals_generated_total` (counter), `risk_rejections_total` (counter); expose
      `/metrics` over HTTP
- [ ] wire metric updates into the orchestrator (LLM call, signal generation, risk
      gate rejection)
- [ ] write tests asserting metrics register and increment correctly
- [ ] run tests — must pass before task 16

### Task 16: Telegram alerting
- [ ] `internal/alert/telegram`: send a message on LLM failure (exhausted retries) and
      on Kill Switch trigger; bot token/chat ID from config/env
- [ ] write tests with a mocked Telegram Bot API endpoint (success, API error, missing
      config → no-op instead of crash)
- [ ] run tests — must pass before task 17

### Phase 4: Live Micro-Lot Trading

### Task 17: Hardened risk gate
- [ ] extend `internal/risk.Gate` with the full Go-Live checklist rules: max position
      = 1 lot, daily loss limit = 0.5% of deposit, fat-finger check (order price within
      2% of current best bid/ask), kill switch on >3% drawdown (cancels all open orders,
      blocks new signals)
- [ ] write table-driven tests covering each rule's approve/reject boundary
      (e.g. exactly 2.0% vs 2.01% from best bid/ask)
- [ ] run tests — must pass before task 18

### Task 18: Idempotent live executor
- [ ] `internal/executor.LiveExecutor`: sends real orders via the Tinkoff orders API;
      `OrderID` generated as UUID v4 and passed through for idempotency; all
      quantities/prices as `decimal.Decimal`
- [ ] write tests with a mocked Tinkoff order endpoint (successful placement, duplicate
      `OrderID` is idempotent/no double-fill, rejected order surfaces an error)
- [ ] run tests — must pass before task 19

### Task 19: Kill switch integration and Ollama timeout watchdog
- [ ] persist kill-switch state in storage; once triggered, orchestrator blocks new
      signals until manually reset
- [ ] wrap every Ollama call with a 10s `context.WithTimeout`; on timeout, skip the
      cycle for that ticker instead of blocking the loop
- [ ] write tests: simulated >3% drawdown trips the kill switch and blocks the next
      cycle; simulated LLM timeout skips the cycle without hanging or crashing
- [ ] run tests — must pass before task 20

### Task 20: Verifier agent
- [ ] `cmd/verifier` (or `internal/verifier`): hourly job reading `AuditEvent` history,
      correlating signals with realized P&L, producing a markdown report per losing
      trade ("was the LLM signal adequate?")
- [ ] write tests with a fixture audit log producing the expected report sections
- [ ] run tests — must pass before task 21

### Task 21: Verify acceptance criteria and Go-Live checklist
- [ ] write `docs/GO_LIVE_CHECKLIST.md` mapping each of the 7 checklist items from the
      source spec to the code/test that satisfies it (decimal usage, SQLite WAL params,
      strict-JSON prompt instruction, fat-finger check, UUID v4 idempotency, kill-switch
      test, Ollama timeout watchdog)
- [ ] verify all Phase 1–4 requirements from Overview are implemented
- [ ] run full test suite (`go test ./...`)
- [ ] run `go vet ./...` and `gofmt -l .` — fix any issues
- [ ] verify test coverage is reasonable for risk/executor/llm packages

### Task 22: [Final] Documentation
- [ ] `README.md`: what the system does, how to configure and run it, how paper vs
      live mode is selected
- [ ] document the package layout and the signal pipeline (ingest → features → LLM →
      risk gate → executor → audit)

## Technical Details

**Core domain (sketch):**
```go
type Action string
const (
    ActionBuy  Action = "BUY"
    ActionSell Action = "SELL"
    ActionHold Action = "HOLD"
)

type TradeSignal struct {
    Ticker     string
    Action     Action
    Confidence decimal.Decimal
    TargetLots int
    Reasoning  string
    GeneratedAt time.Time
}

type AuditEvent struct {
    ID        string // uuid v4
    Ticker    string
    Stage     string // "ingest" | "llm" | "risk_gate" | "executor"
    Payload   string // JSON blob
    CreatedAt time.Time
}
```

**SQLite DSN:** `file:trader.db?_journal_mode=WAL&_busy_timeout=5000`

**Config (YAML sketch):**
```yaml
tickers: [SBER, YDEX, OZON, T, ...]   # ported from finanalys config.py
ollama:
  host: "192.168.88.193:11434"
  model: "qwen3.8"
  timeout: 10s
storage:
  path: "./trader.db"
is_paper_trading: true
poll_interval: 5m
```

## Post-Completion

**External accounts / credentials (manual, before Phase 3–4 code can run for real):**
- Obtain a Tinkoff Invest API token — sandbox token first, production token only once
  Phase 4 is ready to go live.
- Create a Telegram bot via BotFather, obtain bot token + chat ID.
- If splitting processes: provision a Redis instance and Prometheus + Grafana, wire
  dashboards for `llm_inference_duration_seconds`, `signals_generated_total`,
  `risk_rejections_total`.
- Keep all tokens in `.env` / local config, never committed (already gitignored).

**Manual verification / soak tests (real time, real observation — not automatable):**
- Phase 2 success criterion: 100/100 requests to the LLM return parseable JSON,
  average latency < 4s on the RTX 4090 — run `cmd/llmbench` and eyeball the report.
- Phase 3 success criterion: run the system for 1 week in paper mode, confirm signals
  are generated and "filled" virtually, P&L is written to the DB.
- Phase 4: run for 1 month with real micro-lots, confirm risk limits hold and no
  manual intervention was required.
- Manually test the kill switch end-to-end against a simulated >3% drawdown before
  ever enabling `LiveExecutor` with real money.
- Only increase position size after 50+ real trades show positive expectancy — this
  is a human trading decision, not a code change.
