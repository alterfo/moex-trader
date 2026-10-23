# Project rules

Russian translation: [AGENTS.ru.md](AGENTS.ru.md). This file is canonical. Direction: `docs/VISION.md` (read first); the invariants below take precedence over any plan.

## Ownership (two sessions share one tree)

1. **Model zone:** ensemble lgbm+xgb+logreg (buy/sell 0.60/0.40) and Go inference, `internal/model/**`, `internal/features/**`, `cmd/trainmodel|calibrate|exportdataset`, `scripts/*.py`, signal-source selection in `cmd/trader`.
2. **Broker/sandbox zone:** `internal/broker/tinkoff/**`, `internal/ingestion/tinkoff/**`, `cmd/sandboxcheck/**`, `config.sandbox.yaml`, `newBrokerRuntime`, Telegram alerting.
3. **Failover zone:** `cmd/watchdog|witness`, `internal/failover/**`, `deploy/**`, `scripts/deploy-failover.sh`, `watchdog.*`. Before touching election logic run `go test ./internal/failover/ ./cmd/witness/ ./cmd/watchdog/`.
4. The signal source implements both `orchestrator.SignalSource` and `backtest.SignalSource`.
5. `cmd/trader` runs the preflight backtest gate (`preflight.*`) with the same signal source as the live loop and refuses to start on negative realized P&L, too few closed trades or no history; never bypass it.
6. Cross-session talk goes through the user (or agterm/peer-chat); no headless `claude -p` (403).
7. **No concurrent code edits.** One writer at a time; check `git status`/`git diff` first, commit promptly.
8. **Metric-bearing changes are committed immediately.** `docs/metrics.md` (index) and `docs/metrics/*.md` hold every backtest/analysis number; edit them in the same commit as the change. Read metrics from `HEAD` or a file the user named.

## Hard invariants

- Money is `decimal.Decimal`, never `float64`.
- Real-money execution stays disabled: `cmd/trader` refuses non-sandbox accounts. No real orders without explicit go-ahead and a full `docs/GO_LIVE_CHECKLIST.md` pass.
- No automatic hot-swap of the live model (declined 2026-09-16). Daily retraining of a CANDIDATE is approved (2026-10-18); the swap is manual; no job writes `ensemble_model.json`, restarts `./trader` or runs `deploy-failover.sh`. Details: `docs/knowledge/validation-and-ops.md`.
- Only one node runs `cmd/trader`: the right to trade is a lease from the VPS witness (`cmd/witness`). Fence on lease loss; never start the trader without a lease or by hand on the standby.
- Never pick `trader-sandbox.db` by mtime; a node failing `trader.max_failures` starts stays in `error` through the cooldown (details in the same knowledge file).

## Documentation

- English is canonical. Russian twins (`X.ru.md`) exist only for `AGENTS`, `README`, `docs/VISION`, `docs/ARCHITECTURE`, `docs/DEVELOPMENT`, `docs/GO_LIVE_CHECKLIST` (owner decision 2026-10-23); update them in the same commit as the English file. Agents read and edit English only. Everything else, including plans, is English-only.
- **Architecture law:** a commit that adds, removes or renames a `cmd/*` or `internal/*` package, or changes the live loop, data flow, config keys or an integration, updates `docs/ARCHITECTURE.md` and `.ru.md`. `internal/docscheck` (in `make check`) enforces coverage, twins, English-only files and links.
- Keep docs short: tables of result and verdict, no narrative; numbers only in `docs/metrics.md` and `docs/metrics/*.md`.

## Knowledge index (read BEFORE touching the area)

| Area | Read |
|---|---|
| packages, live loop, deploy topology | `docs/ARCHITECTURE.md` |
| commands, config keys, backtest flags | `docs/DEVELOPMENT.md` |
| feature order, `ToVector`, model export, Go inference | `docs/knowledge/feature-pipeline-and-inference.md` |
| news ingestion, classifier, event detectors | `docs/knowledge/news-and-events.md` |
| features, labels, horizons, thresholds, retraining | `docs/knowledge/model-quality-history.md` |
| leverage, exposure, dividends, hedge | `docs/knowledge/income-levers.md` |
| validation, shadow reconciler, alerts, kill switch, challenger, retraining job | `docs/knowledge/validation-and-ops.md` |
| optimizing tuned parameters (Arnold guard `scripts/hooks/arnold-guard.sh`) | `docs/knowledge/soft-models-and-optimization.md` |
| data sources and AlgoPack | `docs/knowledge/data-sources.md` |
| any number | `docs/metrics.md` index, then one `docs/metrics/s*.md` |

## Lessons

- An incident that can be code behaviour lands as a regression test in the same commit; prose lessons go to the matching `docs/knowledge/` file, not here.
- A reviewer judges independent evidence (committed metrics, test output), never the author's "done".
