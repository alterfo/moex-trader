# MOEX Trader (Approximate)

Russian translation: [README.ru.md](README.ru.md). This file is canonical.

> Source of truth and direction: [docs/VISION.md](docs/VISION.md).

A Go trader for MOEX instruments. It collects market data and news, builds a 28-feature vector, gets a `BUY`/`SELL`/`HOLD` signal from a pure-Go model (ridge score table, LightGBM + XGBoost + logistic ensemble, or pooled logistic regression), passes it through a risk gate and executes it, first in paper mode, then in the T-Bank sandbox. Every step is written to SQLite as an audit event. Before the live loop starts, `cmd/trader` replays the **same model and configuration** in a preflight backtest and refuses to start on a loss or when history is unavailable.

Real money is intentionally disabled: `cmd/trader` refuses a non-sandbox account.

## Quick start

```sh
go build ./...
make check                                              # go test + go vet
go run ./cmd/trader -config config.sandbox.yaml         # T-Bank sandbox (needs .env tokens)
go run ./cmd/backtest -signal-source=ensemble -ensemble-path ensemble_model.json
```

Full command list, configuration table and change recipes: [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md).

## Documentation map

Every document exists in two languages: `X.md` (English, canonical, the only one agents read) and `X.ru.md` (Russian translation for people). A change updates both in the same commit.

| Document | What it is |
|---|---|
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | system structure, live loop, package map of every `cmd/*` and `internal/*`, scripts, deploy topology |
| [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) | commands, features, models, backtest, execution profiles, configuration, failover ops, recipes |
| [docs/VISION.md](docs/VISION.md) | project vision (owner's source of truth) |
| [AGENTS.md](AGENTS.md) | project rules: session ownership, hard invariants, knowledge index |
| [docs/GO_LIVE_CHECKLIST.md](docs/GO_LIVE_CHECKLIST.md) | checklist before real money |
| [docs/metrics.md](docs/metrics.md) | the single source of every backtest and analysis number (format linted by `internal/metricsdoc`) |
| [docs/knowledge/](docs/knowledge/) | decision history and rejected ideas by area |
| [docs/reports/](docs/reports/) | standalone study reports |
| [docs/research/](docs/research/) | research papers (data, method, studies, results), e.g. the core rerun on refreshed data |
| [docs/plans/](docs/plans/README.md) | pre-registered plans and hypothesis registries (`completed/` for finished ones) |

## License

Apache License 2.0, see [LICENSE](LICENSE). Copyright 2026 Oleg Sidorkin.

The project uses MOEX ISS, T-Invest API and other external services under their own licences; nothing here is investment advice.
