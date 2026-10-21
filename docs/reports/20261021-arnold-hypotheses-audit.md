# Audit: Arnold's hypotheses vs moex-trader (2026-10-21)

Read-only audit against HEAD `a43f6c7`. No files modified for this report; all evidence points to committed files or local data. The tree has uncommitted work from another session (I did not touch it).

This report follows the six items requested: (1) run what already exists but was never executed, (2) PSI/drift, (3) champion/challenger, (4) threshold perturbation, (5) ticker-subset perturbation, (6) window-boundary/fresh window. Where code/tools exist I ran or inspected them; where nothing exists I state the gap explicitly.

## 1) Pass^k / window-consistency (strategyvalidation -consistency)

**Status: tool exists, never executed.**

- CLI: `cmd/strategyvalidation` has `-consistency <wf-dir>`, `-pass-horizon`, `-min-pass-k` (`go run ./cmd/strategyvalidation -h`).
- Code: `internal/strategyvalidation/consistency.go:20-55` computes Laplace-smoothed share `(pos+1)/(n+2)` and `Pass^k = share^k`; prints in `cmd/strategyvalidation/main.go:62-65,299-302`. `AGENTS.md` (Champion/challenger shadow) documents its use.
- Evidence in `docs/metrics.md`: only positive-window counts appear (e.g. §4.1 "2/6, 4/6, 1/6, 4/6, 3/6", §6.5, §6.7). **No Pass^k value recorded anywhere**.
- Data: searched entire repo for `period_returns.csv` — none exists. `internal/walkforward/persist.go:78-79` and `period_returns.go` define it, and plans say `cmd/walkforward`/`cmd/backtest -wf-dir` persist it, but there are no persisted wf dirs on disk (`artifacts/` only has `incident_checks_paper.jsonl`, `oos/`).

**Conclusion:** The mechanism is in place and tested by code structure, but there is nothing to run against. To actually compute Pass^k you need a wf-dir with per-window `period_returns.csv` files. The repo does not have them committed or on disk now.

## 2) PSI drift monitoring

**Status: fully wired, zero measurements recorded.**

- Implementation: `internal/drift/drift.go` (PSI against normal reference, Acklam quantile, once-per-episode warnings, minSamples 64, window caps). Wired in `cmd/trader/main.go:191-199,889-960` as `driftSignalSource`; skipped when `ScoreTablePath` set (current sandbox uses `data/ridge_scores.csv` so `newDriftMonitor` returns nil). Logs: `drift: feature %s PSI %.3f exceeds threshold %.3f`.
- Metrics/alerts: `internal/metrics/metrics.go:62,171` exposes `feature_psi` gauge; `internal/alert/telegram/watch.go` + tests handle PSI breaches with cooldown.
- Docs: `docs/metrics.md:953` lists `feature_psi` gauge; `docs/knowledge/validation-and-ops.md` describes it. **No PSI values ever recorded in `docs/metrics.md`.** `grep` shows only definitions/wiring/tests, no observed values.
- Binaries: `bin/trader-paper`, `./trader` contain drift strings (wired in binary).

**Conclusion:** Instrumentation is complete and alertable, but effectively off in the current sandbox configuration (score-table path) and has produced no recorded measurements. This is a gap in evidence, not a gap in code.

## 3) Champion/challenger shadow

**Status: implemented, never run.**

- Wiring: `cmd/trader/main.go:171-220` — wraps live source with `newChallengerSignalSource` only if `model.challenger_ensemble_path` set; logs to `artifacts/challenger/decisions.jsonl` (default), never reaches executor, errors swallowed.
- Comparison: `cmd/challengercompare` pairs with closed ISS bars (10d horizon default), date-bootstrap CI, refuses verdict below 200 pairs / 40 dates; verdict logic in `cmd/challengercompare/compare.go:182-193` (needs CI excludes 0).
- State: `artifacts/challenger/` does not exist; no records anywhere. `docs/metrics.md` has no challenger section. `AGENTS.md:52-55` documents the policy.

**Conclusion:** Ready to run if/when a challenger ensemble is configured, but there is no evidence of any shadow run to date.

## 4) Threshold perturbation of the deployed recipe

**Status: not done on the deployed artifact.**

- Deployed: `scripts/export_ensemble.py:111,117` hardcodes `random_state=42`; `internal/model/scoretable.go` reads `data/ridge_scores.csv` produced by ridge with thresholds 0.60/0.40 (documented). `cmd/trainmodel` defaults 0.55/0.45 fallback.
- Evidence: `docs/metrics.md` §6.5 tested quantile thresholds (80th/20th) on a different recipe — mixed results. §6.7 step 12 varied sigma-label k ∈ {0.25,0.5,0.75} descriptively (non-monotonic). `internal/strategyvalidation/registry.go:45` has `thresholds_065_035` with `Value: 0` (placeholder). No grid around 0.60/0.40 on the deployed score-table/ridge recipe is recorded in `docs/metrics.md`.
- Plan explicitly warns about selection bias: `docs/plans/20261019-placebo-check.md:33` notes ridge hyperparameters and thresholds chosen after looking at data; `docs/plans/completed/20260917-strategy-validation-action-plan.md:168` says "No tuning of thresholds/notional on the same 6 windows — that is the winner's curse."

**Conclusion:** The deployed pair (0.60/0.40) has never been systematically perturbed. This is a real gap.

## 5) Ticker-subset perturbation (drop-one / universe changes)

**Status: partially tested on different recipes, not on deployed book.**

- Evidence: `docs/metrics.md` §4.2 experiment B (wider training universe 42 tickers) — +86k but mean val AUC 0.5185 (-0.0185, outside band), q1 max DD 4.38% → rejected. §6.5 tested breadth (172 securities) making recipe not profitable. `internal/strategyvalidation/registry.go:48` has `universe_42_tickers`. §8.4-8.6 discuss universe/eligibility.
- Gap: no drop-one-ticker, no leave-one-out, no liquidity-restricted subset test on the deployed 17-ticker book recorded in `docs/metrics.md`.

**Conclusion:** Universe-level changes tested elsewhere and rejected or harmful; per-ticker sensitivity of the deployed set is unmeasured.

## 6) Window-boundary perturbation / fresh window

**Status: core rerun exists and is in progress; gate still on selection windows.**

- Core rerun: `docs/research/20261019-core-rerun.md` (+ru), `docs/research/20261020-findings-for-the-scientific-community.md` (+ru). Commits `7a681c7`, `e5b2d95`, `a43f6c7` show R1/R2/frozen-model forward evaluation; cron on ai-box mentioned. `scripts/rerun-forward.sh`, `rerun-r2.sh` exist.
- Gate windows: `docs/metrics.md` §0.2 defines G = 2026-07-20..2026-10-18 as 90-day preflight window; §4.1 says gate evaluated against purged control on the same six windows; §0.2 notes W2 contains q1-q4/w2/w1. Window G overlaps w1.
- Shift grid: `scripts/procvar.py` tests shifts {0,27,54} days on research recipes (N95=0.0160 in §3.6), but not systematically on the gate's fixed window set as a published result.

**Conclusion:** Fresh/window work is underway (core rerun). The structural issue remains: the gate and many comparisons reuse selection windows; a truly untouched window would be stronger evidence.

## Benford (quick check)
- `data/fundamentals.jsonl` is nested (`fundamentals.fundamentals[]`), n=1692 magnitude values across tickers/fields: chi²=24.7 vs Benford (8 dof, crit~15.5). Distribution close but a bit high (1s a little over). Price levels/volumes/bounded probabilities are not valid targets (confirmed).
- No Benford code exists anywhere in repo.

## Placebo (in progress)
- `cmd/placebocheck` built and tested; full run launched in background (`/private/var/folders/66/vdbbt0cd07l4zhlsg2np0qtc0000gn/T/opencode/run-placebo-full.sh`, pid 33580). Probe (n=2) gave verdict "skill is indistinguishable from the book structure" on W2 at both borrow rates with current ridge_scores.csv; full n=200 will take time (W2 ~3-4h per pass estimate). Results will land in `/private/var/folders/66/vdbbt0cd07l4zhlsg2np0qtc0000gn/T/opencode/placebo-full/`.

## Concrete action items (highest leverage)
1. **Finish placebo** — let full run complete, copy `reports/<date>/placebo.*` into repo per plan, add to `docs/metrics.md` §8.x and knowledge docs.
2. **Compute Pass^k** — if wf dirs with `period_returns.csv` appear (core rerun outputs), run `strategyvalidation -consistency`; else note gap.
3. **Threshold sweep** — quick perturbation of score-table thresholds on W2/W1 (±0.02 steps) without changing training, record in metrics.
4. **Drop-one-ticker** — sensitivity on deployed book.
5. **PSI baseline** — enable/log PSI on a paper/sandbox run or record "no observations" explicitly.
6. **Fresh window validation** — use core rerun outputs when ready.

All of the above are feasible without changing live trading config/artifacts except reading and reporting.
