# Model quality and robustness

## Overview

- Make the validation honest before trying to improve anything: purge/embargo in
  walk-forward (the 10-day label currently overlaps train and validation) and
  archive period-return matrices so PBO becomes computable.
- Add the regression safety net the repo lacks: CI, a bit-identical golden
  backtest and a Go-vs-Python inference parity check.
- Make the live system observable: Prometheus metrics that match the current
  pipeline (no LLM), alerts for silent model death and stale data, VaR/ES
  monitoring.
- Run five pre-registered experiments (dividend-adjusted labels, wide-train /
  narrow-trade universe, probability calibration and ensemble-vs-members,
  volatility-scaled sizing, sector exposure cap) and **deploy the winner only if
  it passes the deployment gate in Technical Details**, which is fixed here,
  before any numbers exist.
- Out of scope (closed by evidence in `AGENTS.md`, do not reopen): new
  oscillator features, intraday, threshold tuning, leverage / aggregate
  exposure cap as sizing, futures hedge, regime balancing, news classifier
  retrain, GDELT, paid news, automatic retrain-and-swap, neural nets / RL,
  wiring `internal/bus`, migrating off SQLite, macro series as live features.

## Context (from discovery)

- `oosfreeze -action status` on 2026-10-04: `recipe unchanged: false`,
  `collection days: 0 / 183`. The freeze (2026-09-17) predates the model change
  of 2026-09-18 (`ae2a5fb`) and the config change of 2026-09-21 (`720529c`).
  The re-freeze is done once, after this plan's deployment decision
  (Post-Completion).
- Labels: `internal/model/dataset.go` (`LabelMode`, `buildSamples`,
  `forwardExitIndex`); no dividend adjustment, no purge/embargo anywhere in
  `internal/walkforward`, `cmd/walkforward`, `cmd/trainmodel`,
  `scripts/export_ensemble.py`.
- Walk-forward: `internal/walkforward` (`GenerateWindowSpecs`, `Recorder`
  persists raw probabilities, `Save`/`Load`, `MetricsFromResult`),
  `cmd/walkforward`, `cmd/backtest -wf-dir`.
- Validation stats: `internal/strategyvalidation` (`pbo.go`
  `ProbabilityOfBacktestOverfitting(matrix, s)`, `registry.go` attempt registry,
  DSR in `stats.go`), `cmd/strategyvalidation`. `docs/metrics.md` says PBO is
  "NOT computable" because no period-return matrices are archived; DSR 0.64
  against the 0.95 real-money bar.
- Dividends: `data/dividends.jsonl` (`ticker`, `last_buy_date`,
  `dividend_net`, ...), produced by `cmd/dividendscalendar`.
- Sizing: `internal/model/ensemble.go` `EnsembleSignalSource.TargetNotional`
  (lots = notional / price-per-lot); config `risk.target_notional`
  (`internal/config/config.go`). Risk gate: `internal/risk/gate.go`
  `HardenedGate.ApproveReason`, also used in backtest.
- Metrics: `internal/metrics/metrics.go` has only `LLMInferenceDuration`,
  `SignalsGenerated`, `RiskRejections`; used in
  `internal/orchestrator/orchestrator.go`. Drift PSI in `internal/drift`.
  Alerts: `internal/alert/telegram`. Daily digest: `internal/dailysummary`.
- `internal/model/calibration.go` is feature/return correlation, not
  probability calibration — reliability/Brier does not exist yet.
- No `.github/`, no `Makefile`. Remote `github.com:alterfo/moex-trader`.
- `config.sandbox.yaml` `risk.blackout_windows: []`.
- Sandbox audit: ~18.8k `executor`, ~46k `executor_skip`, ~81k `risk_gate`
  events since 2026-09-16.

## Development Approach

- **Testing approach**: Regular (code first, then tests in the same task).
- Complete each task fully before moving to the next; small, focused commits.
- **Every task MUST include new/updated tests** for its code changes, covering
  success and error paths. All tests must pass before the next task.
- Update this plan when scope changes.
- Project rules that apply to every task:
  - no comments in code (user rule);
  - money is `decimal.Decimal`, never `float64`;
  - every new or changed number goes into `docs/metrics.md` in the **same
    commit** (incident E4), and `TestRepositoryMetricsDocumentPassesLint` must
    pass;
  - shared working tree with another session: check `git status` before
    editing, commit promptly (AGENTS.md rule 7);
  - heavy compute (`cmd/exportdataset`, `cmd/trainmodel`, `cmd/walkforward`,
    `scripts/export_ensemble.py` training) runs on the ai-box
    (`ssh oleg@192.168.88.193`, repo `~/moex-trader`), never on the Mac;
  - new flags default to current behaviour; nothing in the live recipe changes
    until Task 13.
- Backward compatibility: existing CLI flags, config keys, audit stages and
  walk-forward directory layouts keep working.

## Testing Strategy

- Unit tests for every task (table-driven where it fits the package style).
- Golden backtest (Task 1) is the regression gate for all later tasks: any
  task that changes its output without intending to is wrong.
- No UI, no e2e suite.

## Progress Tracking

- Mark completed items `[x]` immediately.
- New tasks get a ➕ prefix, blockers a ⚠️ prefix.

## What Goes Where

- Implementation Steps: code, tests, measurements runnable from this repo
  (including over ssh on the ai-box), docs.
- Post-Completion: restarting live traders, OOS re-freeze, anything needing the
  user's eyes.

## Implementation Steps

### Task 1: CI and golden backtest

- [x] add `Makefile` targets `test` (`go test ./...`), `vet` (`go vet ./...`), `check` (both)
- [x] add `.github/workflows/ci.yml` running `make check` on push and PR with the Go version from `go.mod`
- [x] add `internal/backtest/testdata/golden/` with a small committed candle fixture (3 liquid tickers + IMOEX, ~420 daily bars, enough for the 300-bar lookback) captured from ISS once
- [x] add `TestGoldenBacktest` in `internal/backtest` that replays the fixture through the deployed `ensemble_model.json` with the live config (15000 notional, 0.0005 commission/spread/slippage) and compares realized P&L, closed trades, commission and max DD bit-for-bit against `golden.json`; `-update` flag rewrites it
- [x] write test that the golden test fails on a perturbed fixture (one candle changed)
- [x] run `make check` — must pass before Task 2

### Task 2: Go-vs-Python inference parity fixture

- [x] add `scripts/dump_parity_fixture.py` that loads the source boosters behind `ensemble_model.json` and writes `internal/model/testdata/parity.json` (≈200 feature vectors from the golden fixture + `predict_proba` per member and ensemble)
- [x] generate the fixture on the ai-box and commit it
- [x] add `TestEnsembleMatchesPythonPredictProba` (max abs error ≤ 1e-6 per member and ensemble)
- [x] write test for fixture/feature-order mismatch producing a clear error, not a silent pass
- [x] run `make check` — must pass before Task 3

### Task 3: Purge and embargo in walk-forward and training splits

- [x] add `EmbargoBars int` to `walkforward.WindowSpec` generation (`GenerateWindowSpecs`) and purge training samples whose label exit date (`forwardExitIndex`) is on/after the test window start
- [x] add `-embargo-bars` to `cmd/walkforward`, `cmd/backtest -wf-dir`, `cmd/trainmodel` holdout split, default 0 (old behaviour)
- [x] mirror the purge in `scripts/export_ensemble.py` date split (env/flag, default off) so Go and Python splits agree
- [x] write tests: no training sample's exit index reaches the test window; embargo 0 reproduces the old split exactly; embargo larger than the window returns an error
- [x] run `make check` — must pass before Task 4

### Task 4: Period-return matrices and computable PBO

- [x] persist a daily realized-P&L series per window in the `-wf-dir` output (`period_returns.csv`, date + realized net), extend `walkforward.Window.Validate`
- [x] add `cmd/strategyvalidation -returns-dirs a,b,c` that joins variants by date into the matrix and calls `ProbabilityOfBacktestOverfitting` with `DefaultSplits`
- [x] register every attempt of this plan in `internal/strategyvalidation/registry.go` (control + Tasks 7–11 variants)
- [x] write tests: date join with gaps, mismatched calendars error, PBO on a synthetic matrix with a known answer
- [x] run `make check` — must pass before Task 5

### Task 5: Honest baseline (measurement)

- [ ] on the ai-box, rerun the deployed recipe (abs-10d, 18 features, 0.60/0.40, 15000, costs 0.0005×3) over the same 6 quarterly windows with `-embargo-bars 10`, persisting `-wf-dir`
- [ ] record val AUC per window, realized P&L per window, max DD, trades — embargo 0 vs 10 side by side — in `docs/metrics.md` "Purged walk-forward baseline" in the same commit
- [ ] this run is the **control** for the deployment gate; if purging drops realized P&L below 0, add ⚠️ here and stop before Task 7 to ask the user
- [ ] run `make check` (metricsdoc lint) — must pass before Task 6

### Task 6: Observability — metrics and alerts

- [ ] in `internal/metrics/metrics.go` rename `LLMInferenceDuration` to `InferenceDuration` (metric `moex_trader_inference_duration_seconds`), update `internal/orchestrator/orchestrator.go`; grep `deploy/` and `cmd/pnlexporter` for the old metric name and update them
- [ ] add `risk_rejections_total{reason}` (from `risk.Decision.Reason`), `executor_skips_total{reason}`, `signal_probability` histogram, `candle_age_seconds{ticker}`, `feature_psi{feature}` (from `internal/drift`), `position_notional{ticker}`, `gross_exposure`, `lease_held`
- [ ] add Telegram alerts via `internal/alert/telegram`: probability collapse (>90% of the last N ensemble probabilities inside [0.45, 0.55]), stale candles (age > configurable threshold during the trading session), PSI breach already logged now also alerts; each with a cooldown to avoid spam
- [ ] write tests for every new metric's update path and for each alert's trigger and cooldown
- [ ] run `make check` — must pass before Task 7

### Task 7: Experiment A — dividend-adjusted (total-return) labels

- [ ] add `LabelModeAbsoluteTR` in `internal/model/dataset.go`: when an ex-date (session after `last_buy_date`) falls in (entry, exit], add `dividend_net` to the exit price before computing the return; load `data/dividends.jsonl` via the existing reader in `cmd/dividendscalendar`/`internal` (extract to a shared package if it lives in `cmd`)
- [ ] expose `-label-mode absolute_tr` in `cmd/exportdataset` and `cmd/trainmodel`
- [ ] write tests: label unchanged without a dividend in the window, dividend at exit day included, dividend at entry day excluded, missing calendar file is an error (not silently absolute)
- [ ] on the ai-box: export, train via `scripts/export_ensemble.py`, run the purged walk-forward; record AUC/realized/DD per window vs control in `docs/metrics.md` "Experiment A" in the same commit
- [ ] run `make check` — must pass before Task 8

### Task 8: Experiment B — wide training universe, unchanged trading universe

- [ ] add `-train-tickers` to `cmd/exportdataset` (separate from trading tickers), with point-in-time membership: a ticker contributes rows only on dates it traded with median 20-day turnover ≥ a threshold computed from data available on that date (no future liquidity); FX tickers rejected
- [ ] build the wide list from ISS TQBR shares (store the list with its fetch date in `data/`)
- [ ] write tests: rows before a ticker's eligibility are dropped, eligibility uses only past bars, FX in the list is an error
- [ ] on the ai-box: train on the wide set, trade/evaluate on the 17 config tickers with the purged walk-forward; record in `docs/metrics.md` "Experiment B" in the same commit
- [ ] run `make check` — must pass before Task 9

### Task 9: Experiment C — probability calibration and ensemble vs members

- [ ] add `internal/model/reliability.go`: Brier score, ECE (10 bins), reliability table from (probability, realized label) pairs
- [ ] add `cmd/strategyvalidation -reliability <wf-dir>` reading persisted decision probabilities and realized labels
- [ ] write tests for Brier/ECE on known inputs and empty/degenerate inputs
- [ ] on the ai-box: report reliability per member and for the ensemble; run purged walk-forward for each single member at 0.60/0.40 as registered variants; record in `docs/metrics.md` "Experiment C" in the same commit
- [ ] run `make check` — must pass before Task 10

### Task 10: Experiment D — volatility-scaled position size

- [ ] add `risk.vol_scale` config (`enabled`, `min_mult`, `max_mult`, default disabled) and apply in `EnsembleSignalSource` sizing: `notional × clamp(σ_median / σ_ticker, min, max)` using the existing `realized_volatility` feature and a cross-sectional median over the configured tickers from the same bar; all arithmetic in `decimal.Decimal`
- [ ] wire the same option into `cmd/backtest`/`cmd/walkforward` flags so live and backtest share one code path
- [ ] write tests: disabled reproduces the golden backtest bit-for-bit; clamp bounds; zero/missing volatility falls back to multiplier 1
- [ ] measure with `min 0.5 / max 1.5` (single pre-registered setting, no grid) on the purged walk-forward; record in `docs/metrics.md` "Experiment D" in the same commit
- [ ] run `make check` — must pass before Task 11

### Task 11: Experiment E — sector exposure cap

- [ ] add `risk.sector_caps` (sector → max gross notional) and `risk.sectors` (ticker → sector) to config, check in `HardenedGate.ApproveReason` with per-ticker projection (replace only this ticker's leg, as `max_net_exposure` does), reject reason `sector_cap`
- [ ] default sector map: banks SBER/VTBR/T; oil&gas LKOH/ROSN/TATN/NVTK/GAZP; metals GMKN/PLZL/CHMF/RUAL; other unassigned (no cap); default caps off
- [ ] write tests: risk-reducing orders always pass, flatten-to-flat passes at cap 0, unassigned tickers never blocked, reason recorded in audit
- [ ] measure with one pre-registered cap (45000 per sector = 3 positions) on the purged walk-forward; record in `docs/metrics.md` "Experiment E" in the same commit
- [ ] run `make check` — must pass before Task 12

### Task 12: VaR/ES monitoring and blackout dates

- [ ] add historical 1-day VaR and ES (95%, 99%) of the current book from 250-day trailing returns in `internal/risk/var.go` (decimal results), export as gauges and add one line to `internal/dailysummary`; monitoring only, not a gate
- [ ] fill `risk.blackout_windows` in `config.sandbox.yaml` for the remaining 2026 Bank of Russia key-rate decisions (announcement 13:30 MSK, window 13:15–14:30), dates fetched from cbr.ru with the source URL in the commit message; if the page cannot be verified, leave empty and add ⚠️
- [ ] write tests for VaR/ES on a known return series, empty book, short history
- [ ] run `make check` — must pass before Task 13

### Task 13: Deployment decision against the pre-registered gate

- [ ] evaluate each of Experiments A–E against the gate in Technical Details using `cmd/strategyvalidation` (realized split, DSR, PBO over the full registry)
- [ ] if two or more pass, register and run their combination as one more attempt; the candidate is the single best passing variant or combination
- [ ] if a candidate passes: produce the artifact/config on the ai-box, run the 90-day preflight with it, replace `ensemble_model.json` and/or `config.sandbox.yaml`, update the golden backtest and parity fixtures, record everything in `docs/metrics.md` "Deployment decision 2026-10" in the same commit
- [ ] if nothing passes: record the negative result in `docs/metrics.md` and leave the live recipe untouched
- [ ] run `make check` — must pass before Task 14

### Task 14: Verify acceptance criteria

- [ ] all Overview items implemented or explicitly recorded as negative results
- [ ] every new flag defaults to old behaviour (golden backtest unchanged unless Task 13 deployed)
- [ ] `go test ./...`, `go vet ./...`, `go test ./internal/failover/ ./cmd/witness/ ./cmd/watchdog/` all pass
- [ ] `TestDeployedModelArtifactsMatchFeatureOrder` and `TestRepositoryMetricsDocumentPassesLint` pass

### Task 15: [Final] Update documentation

- [ ] `AGENTS.md`: purged walk-forward is the standard validation; PBO computable; experiment verdicts (one bullet each, with metrics.md section names); new alerts and metrics
- [ ] `README.md`: CI, `make check`, new flags, metrics list

## Technical Details

### Deployment gate (pre-registered 2026-10-04, do not edit after Task 5 starts)

A variant is deployable only if **all** hold, against the Task 5 control, on the
same 6 quarterly purged windows (`-embargo-bars 10`), 1M deposit, costs
0.0005 commission + 0.0005 spread + 0.0005 slippage, kill switch on:

1. total **realized** P&L > control (MTM never counts);
2. realized P&L ≥ control in at least 4 of 6 windows;
3. max drawdown ≤ 3% in every window and the kill switch never trips;
4. for label/training changes (A, B, C): mean val AUC not below control by
   more than 0.005;
5. PBO over the full registry of this plan's attempts < 0.5;
6. 90-day preflight passes with the candidate.

DSR is reported for every variant but is **not** a deployment criterion here —
DSR > 0.95 remains the real-money go-live bar, unchanged.

### Formats

- `period_returns.csv`: `date,realized_net` (decimal string), one row per
  trading day of the window.
- Volatility multiplier: `clamp(median_σ / σ_i, min_mult, max_mult)`, σ from the
  `realized_volatility` feature of the same bar.
- Sector check: `Σ_{t∈sector, t≠ticker} |notional_t| + |notional_ticker after
  order| ≤ cap`.

## Post-Completion

*Manual, no checkboxes.*

- If Task 13 deployed a candidate: `scripts/deploy-failover.sh mac` and the
  ai-box linux/amd64 build; restart only through the watchdog (lease holder),
  never a manual `./trader`.
- Re-freeze OOS (`cmd/oosfreeze -action freeze`) right after the deployment
  decision, whether or not anything was deployed — this starts the 183-day
  clock; any later recipe change resets it.
- Watch the new Telegram alerts for a week and tune thresholds/cooldowns if
  they are noisy.
- Enable GitHub Actions on the repo if it is disabled.
