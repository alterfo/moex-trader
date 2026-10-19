# Approximate: project vision

Russian translation: [VISION.ru.md](VISION.ru.md). This file is canonical.

The main source of truth and direction of the project. The author's wording (2026-10-10), quoted verbatim in the Russian original. If code, a plan or a document contradicts this file, this file is right; the hard safety invariants from `AGENTS.md` still stand above any plan.

**Idea:** An algorithmic trader with a news analyzer that gives trade recommendations based on news analysis and market dynamics.

**Problem:** It is hard to manually analyze the large volume of information about the market and companies in order to make effective investment decisions for large investors.

**Technology:** The data-processing pipeline includes collecting market data, computing features, inference of ensemble models, applying a threshold and external vetoes, which results in concrete trades and signal alerts.

**Working name:** Approximate

## How to read this in repository terms

| Word from the vision | Where it lives in code |
|---|---|
| collecting market data | `internal/ingestion/*`, `internal/orchestrator` |
| computing features | `internal/features`, `internal/model/features.go` |
| inference of ensemble models | `internal/model/ensemble.go` (LightGBM + XGBoost + logreg); the live sandbox profile reads probabilities from the ridge score table, `internal/model/scoretable.go` |
| applying a threshold | `buy_threshold` / `sell_threshold` in `ensemble_model.json` (`0.60` / `0.40`; the same constants in `scoretable.go`) |
| external vetoes | news veto gate, incident detector, risk gate (`internal/risk`), kill switch |
| trades and signal alerts | `internal/executor`, `internal/alert/telegram` |
| news analyzer | `internal/model/newsclassifier.go`, `cmd/newsfetch`, `cmd/newsscore`, `cmd/trainnewsmodel` (developed in a separate session) |
