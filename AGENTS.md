# Project rules

## Multi-session ownership (agreed 2026-09-16)

Two agent sessions share this repo. Zone boundaries:

1. **Ensemble + model zone:** ensemble lgbm+xgb+logreg @ buy/sell 0.60/0.40 and its Go inference, `internal/model/**`, `cmd/trainmodel`, `cmd/calibrate`, `cmd/exportdataset`, `internal/features/**`, `scripts/*.py`.
2. **Broker/sandbox zone:** `internal/broker/tinkoff/**`, `internal/ingestion/tinkoff/**`, `cmd/sandboxcheck/**`, `config.sandbox.yaml`, sandbox wiring in `cmd/trader` (`newBrokerRuntime`), Telegram alerting. Signal-source selection inside `cmd/trader` belongs to the ensemble/model zone.
3. The live/backtest signal source MUST implement both `orchestrator.SignalSource` (`Generate(ctx, feature) → BUY/SELL/HOLD`) and `backtest.SignalSource`, so it works in live sandbox and backtest without changes to executor/risk.
4. `cmd/trader` runs the preflight backtest gate before the live loop (`preflight.*` in the config): the current configuration is replayed over recent MOEX history and the trader refuses to start when **realized** P&L is below `preflight.min_net_pnl`, when closed trades are below `preflight.min_closed_trades`, or when history is unavailable for every ticker. The gate MUST use the same signal source as the live loop; do not bypass it or point it at a different source. The gate must also refuse to start (not silently "pass") when every decision in the replay errors out — a feature-count mismatch between `ensemble_model.json` and the current feature vector once made every decision fail, which looked like "0 trades, P&L 0, gate passed" instead of a hard failure.
5. Cross-session consultation goes through the user relaying messages (or agterm typing between panes) — headless `claude -p` returns 403 in this environment, there is no direct programmatic channel between the two agents.
6. **Failover zone:** `cmd/watchdog/**`, `cmd/witness/**`, `internal/failover/**`, `deploy/**`, `scripts/deploy-failover.sh`, `watchdog.yaml`/`watchdog.env` on the machines. Before touching election/witness logic run `go test ./internal/failover/ ./cmd/witness/ ./cmd/watchdog/`; the invariants below are load-bearing.
7. **Hard rule for all models — no concurrent code edits.** The two sessions share one working tree; only one may be writing code at a time. Before editing any tracked file, check `git status`/`git diff` and co-ordinate via the user that the other session isn't mid-change; concurrent edits tear the tree (the backtest realized/unrealized feature once landed uncommitted beside its own spec). Split work into read-only research vs. the single active writer, and commit promptly to shrink the dirty-window overlap.
8. **Metric-bearing changes are committed immediately (incident E4).** `docs/metrics.md` is the single source of truth for backtest/analysis numbers; any change that adds or edits a metric must land in the same commit. A session must only read metrics from `HEAD` or from an explicitly requested uncommitted file — an empty `git log -S` search does not prove absence when the working tree has uncommitted changes.

## Hard invariants

- All money is `decimal.Decimal`, never `float64` (GO_LIVE_CHECKLIST #1).
- Real-money live execution stays intentionally disabled — `cmd/trader` refuses to start against a non-sandbox account. Only paper mode and the T-Bank sandbox run today. Don't wire real order placement without an explicit go-ahead and a full go-live checklist pass.
- Automatic daily retraining with atomic replacement of the live model artifact (cron → walk-forward gate → hot-swap, no human review) was explicitly declined by the user on 2026-09-16: current baseline AUC (~0.50-0.52) is too close to noise for an ungated auto-swap to be safe with real money. Feature-drift/PSI diagnostics and a per-ticker circuit breaker are fine to build; the atomic auto-swap step needs new explicit sign-off before it's implemented.
- Two nodes (Mac primary, ai-box standby) share one sandbox account, so **only one may run `cmd/trader` at a time**. The right to trade is a lease from the VPS witness (`cmd/witness`); a node must stop its trader (fence) the moment its lease expires or is lost, and must never start the trader without holding a lease. Do not "fix" a fencing node by bypassing the witness or starting the trader by hand — a manual `./trader` on the non-lease-holding machine is exactly the double-trading failure the witness exists to prevent.
- Never pick the authoritative `trader-sandbox.db` by file mtime: a freshly created empty DB has a newer mtime than the working one and will clobber it (this happened on 2026-09-16, wiping the sandbox audit history; the account was flat, so only statistics were lost). The source is the node whose trader started last (`trader.last_start_at` in the heartbeat); a DB accepted from the peer is marked synced and the local node reclaims ownership when its own trader starts. Downloads are validated as SQLite before replacing the local file.
- A node that fails to start its trader `trader.max_failures` times releases the lease and stays in `error` (not `standby`) while its `post_failure_cooldown` lasts — otherwise the peer sees a healthy-looking standby and never takes over. The cooldown (30 min) is deliberate anti-ping-pong for a globally broken model.
- The watchdog on the Mac runs `./trader` from the repo root; `scripts/deploy-failover.sh mac` rebuilds it. The ai-box runs its own linux/amd64 build with `SSL_CERT_FILE=certs/ca-bundle.pem` (system CAs + Russian Trusted Root/Sub CA, needed because T-Bank uses the Russian CA chain and Ubuntu does not ship it).

## Knowledge index (read the file BEFORE touching the area)

Detailed history, measured results and rejected ideas live in `docs/knowledge/`. They were moved verbatim out of this file on 2026-10-09 so the always-loaded rules stay short; the invariants above still take precedence. Numbers live only in `docs/metrics.md`.

| Before you touch... | Read |
|---|---|
| feature order, `ToVector`, XGBoost/LGBM export, Go inference | `docs/knowledge/feature-pipeline-and-inference.md` |
| news ingestion, news classifier, event/incident detectors, news gate | `docs/knowledge/news-and-events.md` |
| new features, labels, horizons, thresholds, sizing, retraining | `docs/knowledge/model-quality-history.md` |
| leverage, `max_net_exposure`, dividends, futures hedge, market-neutral | `docs/knowledge/income-levers.md` |
| validation tooling, shadow reconciler, metrics/alerts, execution safeguards, kill switch, Telegram | `docs/knowledge/validation-and-ops.md` |

## Champion/challenger shadow (2026-10-09)

- `model.challenger_ensemble_path` (+ `model.challenger_log_path`, default `artifacts/challenger/decisions.jsonl`) makes `cmd/trader` score every live feature vector with a second ensemble and append both signals as JSONL. The challenger NEVER reaches the executor: it is wrapped around the live model source only (`cmd/trader/challenger.go`), not around the preflight source, and its errors or panics are logged, not propagated. Leave the path empty to disable.
- `cmd/challengercompare` pairs the log with closed ISS bars (default 10-day forward return), reports agreement, signed return per decision and a date-bootstrap CI for challenger-minus-champion, and refuses a verdict below 200 pairs / 40 dates. A "challenger better" verdict is evidence only: promotion still requires the purged walk-forward gate and the explicit sign-off required by the auto-swap invariant above.
- `cmd/strategyvalidation -consistency <wf-dir> [-min-pass-k 0.5 -pass-horizon 4]` reports realized P&L per walk-forward window, positive-window count, worst window and a Laplace-smoothed Pass^k (probability the next k windows are all positive). Use it next to PBO/DSR: a sum over windows can hide one catastrophic window.

## Lessons and regression tests

- Every incident or post-mortem finding that can be expressed as code behaviour lands as a regression test in the same commit (existing examples: `TestCancelOpenOrdersLeavesForeignOrdersAlone`, the fencing and mtime-clobber tests, the Cyrillic `[а-яё]*` incident detector tests). A lesson that is only prose will be forgotten.
- Put the prose lesson in the matching `docs/knowledge/` file, not here; only add to this file what must be loaded in every session (ownership, hard invariants, the index).
- A reviewer or verifier judges independent evidence (committed metrics, test output), never the author's statement that something is done.
