# Strategy Validation Implementation Plan

## Overview

The deployed abs-10d ensemble (18 features, buy/sell thresholds 0.60/0.40, 15000₽ per
position, hold-until-flip, 18 tickers, T-Bank sandbox) has never been validated by a
proper live-equivalent gate or formal statistics. This plan implements the two-session
opencode consensus (adopted from `docs/plans/20260917-strategy-validation-action-plan.md`,
which is not modified by this plan) for deciding, by pre-registered criteria, whether the
system has a provable edge before any real-money decision is considered.

Work runs in four groups, in order: cheap falsification tests that can kill the strategy
in half a day; engine blockers that make live and backtest numbers diverge; formal
statistics on edge (deflated Sharpe, PBO, regime decomposition, OOS freeze); and cost/risk
infrastructure (borrow, per-ticker spread, walk-forward persistence, attribution, drift
monitoring, unified metrics). Go/no-go criteria for real money are listed in
Post-Completion — they require a human decision, not an automated check, and are
deliberately not encoded as Task checkboxes.

## Context

- Both opencode sessions agree: **no real money today.** The sandbox keeps running as
  data collection and infrastructure validation.
- The left session's claim "AUC 0.52 => P&L indistinguishable from luck" is narrowed: AUC
  ranks the whole label distribution, while the strategy only trades the tails of the
  probability distribution, and the headline Sharpe is partly an artifact of a nearly flat
  daily curve (many no-exposure days). Corrected framing: **edge is not disproven, and the
  burden of proof rests on the tails and on a beta/regime decomposition** — not "provably
  impossible."
- The right session's hypothesis "+110742₽ could be beta caught by a trend filter in one
  regime" is itself just a narrative until measured — that becomes Task 9 (2.3).
- Blockers found in the code (confirmed by both sessions, with file:line references): the
  live feature vector is built differently from the training/backtest vector; the
  backtest's kill-switch/daily-loss limit is dead code while live's is per-ticker instead
  of portfolio-level; preflight judges by MTM with zero costs and a 0₽ threshold; the
  5-minute poll plus target-position executor causes rebalance churn.
- Applies the AGENTS.md invariant: judge strategies by **realized** P&L, not MTM.
- Source debate artifacts (`critique-left.md`, `rebuttal-right.md`, `plan-additions-right.md`)
  were temporary working files, not committed to the repo.

## Development Approach

- Testing approach: regular (write tests alongside each task, no strict TDD requirement).
- Complete each task fully — including its tests and metrics-doc update — before moving to
  the next.
- **Pause point after Task 2** (the Step 0 falsification tests): review the tail-precision
  and momentum-benchmark results with the user before continuing. Task 3 onward (Phase 1
  engine fixes) are worth doing regardless of the Step 0 outcome per the source debate, but
  continuing into Phase 2/3 (formal statistics, cost infrastructure) should follow a
  considered decision, not run automatically if Step 0 falsifies the strategy.
- Update this plan if implementation reveals the scope needs to change.

## Testing Strategy

- Unit tests required for every code-changing task (new computations, fixes, and report
  generation alike).
- Run the project's full test suite after each task before proceeding to the next.
- Where a task produces a report/metric rather than new production code, add tests for the
  computation logic (e.g. bootstrap p-value, DSR/PBO, regression) rather than the report
  text itself.

## Progress Tracking

- Mark completed items with `[x]` immediately when done.
- Record every backtest/analysis number produced by a task into `docs/metrics.md` (created
  in Task 17) so results are never only in chat or in an uncommitted file.
- Update this plan if implementation deviates from the original scope.

## Technical Details

Known code blockers and pointers (from the source debate, confirmed by both sessions):

- **Feature parity**: live builds features from a candle array that includes the current
  unclosed daily bar and an intraday `LastPrice` (`ingest.go:75-94`, `builder.go:76-102`);
  training/backtest use only closed bars with `LastPrice = prev close` (`engine.go:378-386`).
- **Portfolio risk gate**: backtest passes single-ticker equity and never sets
  `DayStartEquity` (`engine.go:442-452`, `gate.go:217-235`), so the 3% drawdown and 0.5%
  daily-loss limits never trigger in backtest even though live checks them portfolio-wide.
  An aggregate curve already exists (`aggregateCurves`, `engine.go:707-723`).
- **Preflight**: spread/slippage are not passed into the preflight backtest
  (`cmd/trader/main.go:770-822`); `min_net_pnl` defaults to `"0"`; `NetPnl` is MTM
  (`FinalEquity - Deposit`, `engine.go:696`), which contradicts the AGENTS.md
  realized-vs-MTM invariant.
- **Rebalance/signal churn**: the target-position executor (`orchestrator.go:165-210`) has
  no hysteresis on rebalance size or signal-band exit.
- **Spread model**: a half-spread model already exists (`engine.go:481-490`) and can be
  reused for the per-ticker spread work.
- Probabilities for the tail-precision test come from the existing `-cache` flag
  (`cmd/backtest/main.go:78`, `ensemble.go:122`).

Pre-registered thresholds carried over from the source debate (used by later tasks, not
re-derived): tail-precision/momentum test window ≈136 decisions (approximation pending
Task 13's persisted per-quarter models); rebalance hysteresis ≥1 lot and >5% notional
deviation (exact X% pre-registered before Task 6 runs); multiple-testing threshold t≈3-4
(Harvey & Liu) for Task 7; gap-stress shocks of -10%/-20% for Task 8; short-borrow stress
of 0.005%/day for Task 11; kill-switch reset check of equity ≥1.5x max_notional for
Task 18.

## Implementation Steps

### Task 1: Tail-precision falsification test

- [x] Compute pooled precision of decisions with `p >= 0.60` (BUY) and `p <= 0.40` (SELL)
      using cached probabilities (`-cache`, `cmd/backtest/main.go:78`, `ensemble.go:122`);
      "outcome in the signal's direction" = BUY -> move > +0.5%, SELL -> move < -0.5% over
      10 trading days (matches the training label's horizon/deadband)
- [x] Compute the base rate over the same window for comparison
- [x] Report one single pre-registered pooled metric — do not multi-test across
      bands/tickers (avoids multiple-testing inflation)
- [x] Compute a p-value via bootstrap over episodes/positions (not i.i.d. binomial —
      positions are held for weeks), right-tailed
- [x] Note in the report that this first run uses the deployed artifact over the
      available window (~136 decisions) as an approximation, since per-quarter models were
      not saved (see `backtest-results.md:109`); flag that the exact version depends on
      Task 13's persisted per-quarter models
- [x] Record the result in `docs/metrics.md` (create a minimal version now if Task 17
      hasn't run yet; Task 17 will formalize its structure)
- [x] write tests for the pooled-precision and bootstrap p-value computation
- [x] run project tests - must pass before next task

### Task 2: Momentum benchmark

- [x] Implement a top-k momentum benchmark on `mom_21d` (k=5, pre-registered), rebalanced
      every 10 trading days, on the same 18 tickers with the same commission/spread/
      slippage/kill-switch/deposit as the live config
- [x] Support both long-only and long+short variants
- [x] Compare the deployed ensemble against this benchmark net of costs over the same
      window; the ensemble must beat it net of costs, otherwise treat it as
      "momentum + noise" regardless of the Task 1 result (engine-wide biases, including the
      kill-switch bug, cancel out in this relative comparison)
- [x] Record results in `docs/metrics.md`
- [x] write tests for the momentum benchmark signal source
- [x] run project tests - must pass before next task

### Task 3: Live/backtest feature parity fix

- [x] Fix live feature construction to always use a snapshot of the last CLOSED daily bar
      (matching backtest's `LastPrice = prev close`, `engine.go:378-386`), instead of the
      current unclosed-bar-plus-intraday-`LastPrice` behavior (`ingest.go:75-94`,
      `builder.go:76-102`) — after this fix, live and backtest are feature-identical by
      construction
- [x] Add shadow reconciliation: compare live decisions against a replayed backtest
      decision for the same ticker/day and write a digest into `docs/metrics.md`
- [x] write tests covering the closed-bar snapshot and the shadow-reconciliation digest
- [x] run project tests - must pass before next task

### Task 4: Portfolio risk gate in backtest

- [x] Fix `engine.go:442-452` / `gate.go:217-235` to pass portfolio-level `CurrentEquity`
      (via the existing `aggregateCurves`, `engine.go:707-723`) instead of single-ticker
      equity, and to set `DayStartEquity`, so the 3% drawdown and 0.5% daily-loss limits can
      trigger against the portfolio curve
- [x] Re-run the 6 walk-forward windows after the fix and report how many days the
      live-equivalent gate would have frozen trading
- [x] Decide and document whether the kill switch should only block new entries (current
      live behavior) or also liquidate open positions; if "block only," cross-reference
      Task 8's gap-stress test as the compensating control
- [x] write tests for the portfolio-equity/day-start-equity wiring
- [x] run project tests - must pass before next task

### Task 5: Preflight hardening

- [x] Pass `SpreadPct`/`SlippagePct` from config into the preflight backtest
      (`cmd/trader/main.go:770-822`)
- [x] Change the preflight pass/fail threshold to use realized P&L instead of MTM
      (`FinalEquity - Deposit`, `engine.go:696`), matching the AGENTS.md
      realized-vs-MTM invariant
- [x] Set `min_net_pnl` explicitly and greater than 0 (currently `"0"`)
- [x] Add a minimum-closed-trades requirement to the gate
- [x] Log a config hash alongside the gate decision
- [x] Recompute preflight numbers after the fix
- [x] write tests for the realized-P&L threshold and the minimum-trades check
- [x] run project tests - must pass before next task

### Task 6: Rebalance and signal hysteresis (anti-churn)

- [x] Add rebalance hysteresis in the target-position executor
      (`orchestrator.go:165-210`): only rebalance when `|delta| >= 1 lot` AND the deviation
      from target exceeds a pre-registered X% of notional (e.g. 5%)
- [x] Add signal hysteresis: only exit the BUY/HOLD/SELL band after 2 consecutive polls
      agree (distinct from confidence-sizing — this is about signal frequency, not position
      size)
- [x] Validate against live sandbox fills before/after the change (manual test —
      skipped, not automatable)
- [x] write tests for both hysteresis mechanisms
- [x] run project tests - must pass before next task

### Task 7: Attempt registry and DSR/PBO

- [x] Build a registry recording every configuration tried so far (the >=20 already
      documented in AGENTS.md and `docs/habr-ai-article/backtest-results.md`, plus the
      ~20 from the source debate)
- [x] Compute the deflated Sharpe ratio (Bailey & López de Prado) and probability of
      backtest overfitting (PBO) from the registry
- [x] Use a multiple-testing threshold of t ≈ 3-4 (Harvey & Liu), not t=2
- [x] write tests for the DSR/PBO computation
- [x] run project tests - must pass before next task

### Task 8: Gap-stress test

- [x] Model the worst historical overnight gaps plus synthetic -10%/-20% shocks against
      the current portfolio (hold-until-flip has no stop-loss; the only protection is the
      sticky kill switch, which does not liquidate positions)
- [x] Report the equity impact against the 3% drawdown floor
- [x] write tests for the gap-stress scenario runner
- [x] run project tests - must pass before next task

### Task 9: Beta/regime decomposition

- [x] Decompose P&L net of IMOEX; report long and short legs separately; report return on
      gross exposure (not "% of the 1M deposit")
- [x] Add an equal-weighted 18-ticker benchmark (null-universe context) alongside the
      momentum benchmark from Task 2
- [x] Run a per-trade/per-day regression: `P&L ~ alpha + beta1*bench + beta2*momentum`,
      with clustered standard errors
- [x] Label each OOS quarter's regime (trend/flat/decline) — two bull quarters is not alpha
- [x] Start with the cheapest version first: decompose per-trade P&L into its IMOEX
      component using data already available, without waiting for new data collection
- [x] write tests for the regression/decomposition code
- [x] run project tests - must pass before next task

### Task 10: OOS freeze

- [x] Freeze the recipe and record the code's SHA
- [x] Run it unmodified on the next two quarters (>=6 months of collection); any change
      restarts the count from zero (future manual operation - not automatable)
- [x] Document that no real-money actions happen while this data accumulates
- [x] write tests for the freeze/SHA-recording mechanism
- [x] run project tests - must pass before next task

### Task 11: Short-borrow measurement and stress

- [ ] Measure real short-borrow terms (rate, holding limits) for all 18 names in the
      T-Bank sandbox
- [ ] If it cannot be measured, apply a 0.005%/day stress on short-leg notional instead
- [ ] Use only the stress scenario in the go/no-go decision, not an optimistic estimate
- [ ] write tests for the borrow-cost stress calculation
- [ ] run project tests - must pass before next task

### Task 12: Per-ticker spread from AlgoPack

- [ ] Replace the global spread constant with a per-ticker spread derived from
      accumulated AlgoPack order-book data (already logged in `audit_events` by live)
- [ ] Reuse the existing half-spread model (`engine.go:481-490`)
- [ ] write tests for the per-ticker spread derivation
- [ ] run project tests - must pass before next task

### Task 13: Walk-forward persistence

- [ ] Persist per-quarter models and decision logs (id, config, probability, feature
      hash) under `artifacts/wf/<window>/`
- [ ] write tests for the persistence format and round-trip loading
- [ ] run project tests - must pass before next task

### Task 14: Attribution reporting

- [ ] Add realized P&L by ticker, top-N trade concentration, and a realized/unrealized
      split to every backtest report
- [ ] write tests for the attribution computation
- [ ] run project tests - must pass before next task

### Task 15: Drift monitoring and circuit breaker

- [ ] Add a PSI (population stability index) check for features against the training
      distribution, emitting a warning (not a hard stop)
- [ ] Add a per-ticker circuit breaker that stops trading a name after N losses or a
      cumulative loss greater than X% of notional
- [ ] Do not add auto-retraining (already declined 2026-09-16) — this task is diagnostics
      and circuit-breaker only
- [ ] write tests for the PSI calculation and circuit breaker triggers
- [ ] run project tests - must pass before next task

### Task 16: Sandbox/paper parallel tracking

- [ ] Log expected vs. actual fill price, rejection rate under the 0.3% marketable-limit
      cap, and actual borrow charges
- [ ] Cross-reference this against Task 11/12's measurements — this is the direct
      real-world measurement of those gaps
- [ ] write tests for the expected-vs-actual logging
- [ ] run project tests - must pass before next task

### Task 17: Unified metrics document

- [ ] Create `docs/metrics.md` as the single source of truth, with columns: window |
      realized vs MTM | artifact (commit) | costs | date
- [ ] Migrate all previously reported numbers into it (+110742, +79453, +75028, +63212,
      +56379/+40102, +25699)
- [ ] Apply the process rule from incident E4 (two-session race): commit metric-bearing
      changes immediately; the other session should only read HEAD or an explicitly
      requested uncommitted file — an empty `git log -S` search does not prove absence when
      the working tree has uncommitted changes
- [ ] write tests for the metrics-file writer/format, or a format-lint check if the file
      is hand-maintained
- [ ] run project tests - must pass before next task

### Task 18: Kill-switch runbook check

- [ ] Before any manual kill-switch reset, require a check that equity >= 1.5x
      max_notional across all open positions, so a frozen position cannot be reset into a
      skewed portfolio
- [ ] write tests for the runbook check
- [ ] run project tests - must pass before next task

### Task 19: Verify acceptance criteria

- [ ] verify all requirements from Overview are implemented
- [ ] run full project test suite
- [ ] run project linter - all issues must be fixed

## Post-Completion

*Items requiring manual intervention and human judgment — no checkboxes, informational
only. None of these should be auto-decided by an execution agent.*

**Pre-registered go/no-go criteria for real money** (set now, before any new runs;
loosening them requires a new explicit decision):

1. Phase 1 exit criteria met (live ≡ backtest, portfolio gate, hardened preflight — Tasks
   3-6).
2. Step 0 passed: tail-precision significantly beats the base rate, and the model beats
   the momentum benchmark net of costs (Tasks 1-2).
3. >=2 quarters of frozen OOS with **realized** P&L > 0 under conservative costs
   (spread+slippage 0.05%+0.05%, borrow under stress).
4. DSR > 0.95, PBO < 0.2 (attempt counts from the Task 7 registry).
5. Positive alpha with t > 3 after subtracting the IMOEX and momentum benchmarks; the
   regression specification and SE formula are fixed in advance, and the number of
   specifications tried is recorded in the registry.
6. At least one non-bull OOS quarter is present (otherwise the result is flagged as beta
   risk and does not pass).
7. Live-vs-backtest divergence stays within a pre-set tolerance over >=1 month of sandbox
   operation.
8. Full pass of `docs/GO_LIVE_CHECKLIST.md`, including a kill-switch drill in the sandbox.
9. All money remains `decimal.Decimal` (GO_LIVE_CHECKLIST #1); `float` only in
   statistics/plotting code.

**Explicit non-goals:**

- No new features until the data-breadth problem is solved (oscillators already showed
  only ±0.005 AUC).
- No threshold/notional tuning on the same 6 windows — that is a winner's-curse setup.
- No auto-retraining or hot-swap of the model (declined 2026-09-16; unchanged).
- No real execution without a full checklist pass and a new explicit sign-off.

**Open questions:**

- Whether to liquidate positions on kill-switch trip (live currently does not) — decided
  in Task 4 (1.2); if the answer stays "no," the gap-stress test (Task 8) is the
  compensating control.
- The exact borrow rate, and whether the sandbox account charges it at all — Task 11 (3.1)
  measures this rather than guessing.
- Whether tail-precision beats the base rate is unknown until Task 1 (0.1) runs — the
  source debate calls this the most informative experiment in the plan.
