# Commission Accounting and Finam Broker Support

## Overview

Two related gaps found while reviewing the first build:

1. **No commission/fee accounting anywhere.** `executor.Fill` has no commission
   field, `PaperExecutor` and `internal/executor/live.go` (Tinkoff) both record fills
   at raw price, and `internal/verifier` reconstructs P&L from raw fill prices. Paper
   trading (Phase 3) and the "50+ profitable trades" gate (Phase 4) are both
   meaningless without this — a trade that's a paper "win" on raw price can be a real
   loss after fees.
2. **Second broker: Finam Trade API**, chosen over Alfa-Investments after checking
   what's actually publicly documented. Finam publishes a real API spec
   (`api.finam.ru` / `tradeapi.finam.ru`): REST + gRPC + WebSocket, two-step
   token→JWT auth (JWT valid 15 min, refreshed via an API call), order placement
   (market/limit/stop/stop-loss/take-profit), account/portfolio endpoints, real-time
   and historical market data, a demo account, and a published rate limit (200
   req/min per method). Alfa's public docs are ambiguous (mostly a general banking
   partner API, not clearly a retail trading API), so building against it risked
   producing a plausible-looking but non-functional client. Finam's "Единый дневной"
   tariff is also cheaper (from 0.01%) than Alfa's "Трейдер" (from 0.014%).

This plan adds commission accounting (broker-agnostic) and a Finam market-data +
execution integration, following the same pattern already used for Tinkoff.

## Context (from discovery)

- `internal/executor/executor.go`: `Executor` interface
  (`Execute(ctx, signal, price) (Fill, error)`), `Fill` struct, `PaperExecutor`.
- `internal/executor/live.go`: Tinkoff live executor — the pattern to mirror for
  Finam (same interface, its own client, its own tests against mocks).
- `internal/ingestion/tinkoff/client.go`: `Config`, `Client`, `Quote`, `Candle`,
  `OrderBookLevel`, `OrderBook`, `New(ctx, cfg) (*Client, error)` — the shape to
  mirror for `internal/ingestion/finam`.
- `internal/verifier`: reads `audit_events`, correlates signals with fills, produces
  markdown P&L reports — currently ignores fees entirely.
- `cmd/trader`: currently refuses to start when `is_paper_trading: false` because
  Tinkoff live wiring (account snapshot, order cancellation) isn't connected. The
  same honest-refusal behavior must apply to Finam live mode until it has equivalent
  wiring — no silently bypassing risk protections.
- No existing commission/fee code anywhere in the repo (confirmed by grep).

## Development Approach

- **Testing approach**: Regular (implementation first, tests in the same task) —
  consistent with the first plan.
- All money fields stay `decimal.Decimal`.
- Finam auth (secret token → JWT, 15 min expiry) needs a refresh-before-expiry code
  path — treat this like Tinkoff's token handling, tested with a mocked clock/expiry.
- **CRITICAL: every task MUST include new/updated tests.**
- **CRITICAL: all tests must pass before starting next task.**
- **CRITICAL: update this plan file when scope changes during implementation.**

## Testing Strategy

- Unit tests for every task; external HTTP/gRPC calls mocked via `httptest.Server`
  or in-memory fakes — no real network calls, no real Finam token needed to pass
  tests.
- Auth-refresh logic gets explicit tests: token near expiry triggers refresh, expired
  token on a request triggers refresh-and-retry, refresh failure surfaces an error.

## Progress Tracking

- Mark completed items with `[x]` immediately when done.
- Add newly discovered tasks with ➕ prefix.
- Document issues/blockers with ⚠️ prefix.

## What Goes Where

- **Implementation Steps**: commission accounting, Finam client/executor code, tests,
  docs — all buildable/testable without a real Finam account.
- **Post-Completion**: obtaining a real Finam secret token, exercising the demo
  account, verifying the exact commission actually charged, and confirming whether
  Finam's order-placement API supports a client-supplied idempotency key (not
  confirmed in the public getting-started docs — needs checking against the full API
  reference before going live).

## Implementation Steps

### Task 1: Commission field on Fill and config
- [x] add `Commission decimal.Decimal` to `executor.Fill` in
      `internal/executor/executor.go`
- [x] add a `Commission` section to `internal/config` (`Broker string`,
      `Rate decimal.Decimal`), with a documented default matching Finam's "Единый
      дневной" tariff (`0.0001` = 0.01%) and an env override
      (`MOEX_TRADER_COMMISSION_RATE`)
- [x] update `config.example.yaml` with the new section
- [x] write tests for config loading/defaults/env override (valid rate, missing
      section falls back to default, invalid decimal string)
- [x] run tests — must pass before task 2

### Task 2: Apply commission in both executors
- [x] `PaperExecutor.Execute`: compute `commission = price * lots * rate`, set it on
      the returned `Fill`
- [x] `internal/executor/live.go` (Tinkoff): same commission computation on its
      `Fill`
- [x] write tests: commission computed correctly for both executors (several
      price/lots/rate combinations, zero-rate edge case)
- [x] run tests — must pass before task 3

### Task 3: Net P&L in the verifier
- [x] update `internal/verifier` P&L reconstruction to subtract `Fill.Commission`
      from gross P&L per trade, and report both gross and net in the markdown output
- [x] write tests with fixture audit logs showing a trade that's gross-positive but
      net-negative after commission (must be flagged as a loss in the report)
- [x] run tests — must pass before task 4

### Task 4: Finam market-data client
- [x] `internal/ingestion/finam/client.go`: `Config{BaseURL, SecretToken}`,
      `Client`, `Quote`, `Candle`, `New(ctx, cfg) (*Client, error)` mirroring the
      Tinkoff client shape
- [x] implement the two-step auth: exchange `SecretToken` for a JWT, cache it, and
      transparently refresh it before/after the 15-minute expiry
- [x] implement quote/candle fetch methods over REST
- [x] write tests with `httptest.Server` mocks: successful auth + fetch, JWT expiry
      triggers refresh-and-retry, auth failure, malformed response
- [x] run tests — must pass before task 5

### Task 5: Finam executor
- [ ] `internal/executor/finam.go`: `FinamExecutor` implementing the `Executor`
      interface, placing market orders via the Finam Trade API using the client from
      Task 4
- [ ] populate `Fill.Commission` using the config rate from Task 1 (Finam doesn't
      return commission synchronously in the order-placement response per the public
      docs, so this stays the config-rate estimate, not an exchange-confirmed figure
      — note this explicitly in a doc comment)
- [ ] write tests with mocked order endpoint (successful placement, rejected order,
      JWT refresh mid-request)
- [ ] run tests — must pass before task 6

### Task 6: Wire broker selection into cmd/trader
- [ ] extend config with a `broker` field (`paper` / `tinkoff` / `finam`) alongside
      the existing `is_paper_trading` flag
- [ ] `cmd/trader` selects the matching `Executor`; for `tinkoff` and `finam` in
      non-paper mode, keep the existing honest-refusal behavior (refuse to start
      without account-snapshot/order-cancellation wiring) rather than silently
      running with incomplete risk protections
- [ ] write tests: broker selection picks the right executor, live-mode refusal
      still triggers for both Tinkoff and Finam
- [ ] run full test suite — must pass before task 7

### Task 7: Verify acceptance criteria
- [ ] verify commission is applied consistently across paper/Tinkoff/Finam fills and
      reflected in verifier net P&L
- [ ] run full test suite (`go test ./...`)
- [ ] run `go vet ./...` and `gofmt -l .` — fix any issues
- [ ] verify test coverage for `executor`, `verifier`, and the new `finam` packages

### Task 8: [Final] Documentation
- [ ] update `README.md`: add Finam to the broker/commands section, note the demo
      account and the 15-minute JWT refresh behavior
- [ ] update `docs/GO_LIVE_CHECKLIST.md`: add "commission rate matches the real
      contracted tariff" and "confirm whether Finam order placement supports a
      client-supplied idempotency key before enabling live orders"

## Technical Details

**Fill with commission:**
```go
type Fill struct {
    ID         string
    Ticker     string
    Action     domain.Action
    Lots       int
    Price      decimal.Decimal
    Commission decimal.Decimal
    ExecutedAt time.Time
}
```

**Finam auth flow (per public docs):**
1. Obtain a long-lived secret token from the Finam tokens portal (manual,
   Post-Completion).
2. Exchange it for a JWT via the API's `Auth` method; JWT is valid 15 minutes.
3. Attach the JWT as `Authorization` header on every request; refresh proactively
   before expiry or reactively on a 401.

**Config additions:**
```yaml
commission:
  broker: "finam"
  rate: "0.0001"   # 0.01%, Finam "Единый дневной"
broker: "finam"     # paper | tinkoff | finam
finam:
  base_url: "https://api.finam.ru"
  secret_token: ""  # from env / .env, never committed
```

## Post-Completion

- Obtain a real Finam secret token from the tokens portal; open a demo account
  (expires in 2 weeks, 183-day cooldown before reactivation) and exercise
  Task 4/5 code against it manually.
- Confirm the exact commission Finam actually charges on a real/demo fill and
  compare against the configured rate; adjust `commission.rate` if it doesn't match.
- Check the full Finam API reference (beyond the getting-started guide) for
  order-placement idempotency support; if absent, consider client-side
  dedup/state-tracking on retries before enabling live orders.
- Re-run `cmd/llmbench`-style validation isn't needed here, but before enabling
  Finam live mode, follow the same manual kill-switch drill already required for
  Tinkoff in `docs/GO_LIVE_CHECKLIST.md`.
