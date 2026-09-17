# Go-Live Checklist

This checklist maps the five source-spec go-live requirements to the implementation
and tests that satisfy them. The current status is verified against the repository at
the completion of Task 21.

## Source-spec requirements

| # | Requirement | Implementation | Tests |
|---|---|---|---|
| 1 | All price/money values use `decimal.Decimal`; never `float64` | `internal/domain/types.go`, `internal/features/builder.go`, `internal/risk/gate.go`, `internal/executor/executor.go`, `internal/executor/live.go`, `internal/ingestion/moex`, `internal/ingestion/tinkoff`, `internal/ingestion/algopack`, `internal/verifier` | `internal/domain/types_test.go`, `internal/features/builder_test.go`, `internal/risk/gate_test.go`, `internal/executor/live_test.go` (decimal boundary and `270.5` quotation assertions) |
| 2 | SQLite uses WAL and a 5000ms busy timeout | `internal/storage/storage.go` `buildDSN` appends `_journal_mode=WAL&_busy_timeout=5000`; DSN uses `modernc.org/sqlite` | `internal/storage/storage_test.go`: `TestOpenConfiguresWALBusyTimeoutAndSchema`, `TestConcurrentWALWrites` |
| 3 | Fat-finger check rejects an order price more than 2% from current best bid/ask | `internal/risk/gate.go` `fatFingerOK` calculates bid/ask deviation with `decimal.Decimal` and falls back to previous close when no live bid/ask is available | `internal/risk/gate_test.go`: `TestHardenedGateFatFingerBoundary` covers exactly 2.00% vs 2.01% above/below; `TestHardenedGateFatFingerUsesPrevCloseFallback`; `TestHardenedGateFatFingerRejectsNonPositiveOrderPrice` |
| 4 | Live executor uses UUID v4 `OrderID` and is idempotent | `internal/executor/live.go` generates `uuid.NewString`, parses and enforces `uuid.Version() == 4`, and atomically reserves in-flight/sent order IDs; unexecuted NEW orders record zero executed lots instead of a fake fill | `internal/executor/live_test.go`: `TestLiveExecutorPlacesOrder`, `TestLiveExecutorDuplicateOrderIDIsIdempotent`, `TestLiveExecutorNewOrderRecordsZeroLots` |
| 5 | Kill switch trips on >3% drawdown and blocks new signals until reset | `internal/risk/gate.go` `exceedsDrawdown`, `triggerKillSwitch`, `TripKillSwitch`, `ResetKillSwitchContext`; `internal/storage/storage.go` persists `kill_switch`; `cmd/trader` wires a store-backed gate and `-reset-kill-switch` | `internal/risk/gate_test.go`: `TestHardenedGateKillSwitchOnDrawdown`, `TestHardenedGateDrawdownBoundary`, `TestHardenedGateManualKillSwitch`, `TestHardenedGatePersistsAndReadsKillSwitch`, `TestHardenedGateAlertsOnKillSwitch`; `internal/orchestrator/orchestrator_test.go`: `TestOrchestratorKillSwitchBlocksNextCycle` |

## Phase 1-4 implementation verification

- Phase 1: config, domain, SQLite storage, MOEX/news ingestion, feature builder, paper
  executor, risk-gate stub, and orchestration loop are implemented and tested.
- Phase 3: Tinkoff market-data client, AlgoPack enrichment wired into the orchestrator
  as an async per-cycle worker, Redis Streams bus, Prometheus metrics, and Telegram
  alerting are implemented and tested.
- Phase 4: hardened risk gate, idempotent live executor, persisted kill switch, and
  verifier agent are implemented and tested. The running
  `cmd/trader` supports paper execution and Tinkoff sandbox execution; real-money
  mode is still refused until account snapshot, order cancellation, and
  broker-specific reconciliation are verified.

## Sandbox verification

- T-Bank sandbox execution is wired through `internal/broker/tinkoff` (account
  bootstrap, portfolio snapshot for the risk gate, open-order cancellation) and
  `cmd/trader` (`broker: tinkoff`, `is_paper_trading: false`, `tinkoff.sandbox: true`).
- The sandbox charges a flat 0.05% commission and executes market orders at the last
  trade price; set `commission.rate: "0.0005"` so recorded P&L matches the sandbox.
- Before real money: re-run the kill-switch drill against the sandbox account,
  confirm the sandbox portfolio snapshot matches the strategy's own position view,
  and verify order cancellation on kill switch.

## Broker-specific pre-live checks

- Confirm the configured `commission.rate` matches the real contracted tariff for the
  selected broker, and update the rate if the demo or live fill proves otherwise.

## Validation results

- `go test -count=1 ./...` passes for all packages.
- `go vet ./...` reports no issues.
- `gofmt -l .` reports no unformatted Go files.
- Coverage for the go-live-critical packages is reasonable: `internal/risk` 84.9%,
  `internal/executor` 73.8%.
