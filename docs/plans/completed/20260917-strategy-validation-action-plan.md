# Strategy validation plan: consensus of two agent sessions


Date: 2026-09-17. Status: agreed by the left (ensemble/model zone) and right (broker/sandbox zone)
opencode sessions after two rounds of adversarial review. This is a plan, not an implementation:
no code changes in this commit.

Inputs of the dispute (temporary, not in the repo): `critique-left.md`, `rebuttal-right.md`,
`plan-additions-right.md`. Where the sessions diverged, the agreed position is recorded below.

Scope: the deployed abs-10d ensemble (18 features, thresholds 0.60/0.40, 15000 RUB per position,
hold-until-flip, 18 tickers, T-Bank sandbox). The goal is to decide, by pre-registered criteria,
whether the system has a provable edge, and only then think about real money.

## Outcome of the dispute

- Both sessions agree: **do not put real money in today.** The sandbox keeps running
  as data collection and an infrastructure check.
- The left session's claim "AUC 0.52 ⇒ P&L is indistinguishable from luck" is narrowed: AUC ranks the whole
  label distribution, while the strategy trades only the probability tails; the headline Sharpe is an artifact
  of an almost flat daily curve (days without exposure). The correct wording: **the edge is not proven,
  and the burden of proof lies on the tails and on the beta/regime decomposition**, not "mathematically
  impossible".
- The right session's hypothesis "+110742 RUB may be beta caught by a trend filter in a single
  regime" is itself a narrative until measured. It becomes test 0.2/2.3.
- Blockers found in the code (confirmed by both sessions, with file:line): the live feature vector
  differs from the training one by construction; the kill switch/daily limit in the backtest is dead code,
  in live they are portfolio-level; preflight works on MTM, with zero costs and a 0 RUB threshold;
  the 5-minute poll + the target-position executor produce jitter.

## Step 0 — cheap falsification (1-2 days, before all expensive fixes)

These two tests are not invalidated by the blockers B1/B3/B4 (bugs of absolute P&L and execution) and can
close the strategy in half a day, before spending on parity code.

0.1 **Tail-precision — the headline test.** One pre-registered metric: the pooled precision of decisions
    `p ≥ 0.60` (BUY) and `p ≤ 0.40` (SELL), where "outcome in the direction of the signal": BUY → a move
    > +0.5%, SELL → a move < −0.5% over 10 trading days (symmetric with the training label). The base rate
    is computed on the same window. Do not multi-test
    bands/tickers — that is multiple testing again. p-value: cluster bootstrap by episodes/positions
    (the i.i.d. binomial is invalid — positions are held for weeks), right tail. Probabilities are taken from
    `-cache` (`cmd/backtest/main.go:78`, `ensemble.go:122`). Caveat: the first run is on the
    deployed artifact over the available window (~136 decisions), which is an approximation (per-quarter
    models were not saved, `backtest-results.md:109`); the exact version is on the saved models
    after 3.3. If the tail is also ~0.5, there is no edge, and there is no need to go further.

0.2 **Momentum benchmark.** Top-k by `mom_21d` (k=5, pre-registered), rebalanced once per 10 trading
    days, the same 18 tickers, the same commission/spread/slippage/kill-switch/deposit. Two variants:
    long-only and long+short. The model must beat the benchmark net of costs on the same window, otherwise
    "model = momentum + noise" regardless of the tail metric. The test is relative, so common
    engine biases (including the kill-switch bug) cancel out.

Output of step 0: a "continue / stop" decision. If both tests fail, the expensive parity fixes
are still needed for the engine and honest reporting, but they no longer "save the strategy".

## Phase 1 — Blockers: bring the numbers in line with live

1.1 **Feature parity live ≡ backtest** (top-1 by impact). Live builds features from a candle array
    that includes the current unclosed daily bar, and the intraday `LastPrice` (`ingest.go:75-94`,
    `builder.go:76-102`), while training/backtest use only closed bars with
    `LastPrice = prev close` (`engine.go:378-386`). Fix: the model always receives a snapshot
    of the last CLOSED daily bar — then live ≡ backtest by construction. Plus
    shadow reconciliation: live decisions and the replayed backtest are compared, the digest goes to metrics.md.

1.2 **Portfolio risk loop in the backtest** (`engine.go:442-452`, `gate.go:217-235`). Currently
    the equity of a single ticker is passed and `DayStartEquity` is not set → neither the 3% drawdown nor the 0.5%
    daily limit ever fires. Fix: portfolio `CurrentEquity` (the aggregate already exists,
    `aggregateCurves` `engine.go:707-723`) + `DayStartEquity`, signals are checked against the
    portfolio curve. After the fix, rerun the 6 windows and report how many days a live-equivalent
    gate would have frozen trading. Separately decide and document: does the kill switch only block
    new entries (the current live behaviour) or liquidate positions; if "only blocks" —
    the compensating control is the gap stress 2.2.

1.3 **Preflight hardening** (`cmd/trader/main.go:770-822`). Currently: spread/slippage are not passed,
    `min_net_pnl: "0"`, and `NetPnl` is MTM (`FinalEquity − Deposit`, `engine.go:696`), which
    contradicts the AGENTS.md invariant "judge by realized". Fix: pass SpreadPct/SlippagePct
    from the config, a threshold on **realized** P&L, `min_net_pnl` set explicitly and > 0, a minimum number
    of closed trades, the config hash in the gate log. After the fix, recompute the preflight numbers.

1.4 **Anti-jitter** (`orchestrator.go:165-210`, the target-position executor). Rebalance hysteresis:
    |delta| ≥ 1 lot AND deviation from the target > X% (X pre-registered, for example 5%
    of notional). Signal hysteresis: leaving the BUY/HOLD/SELL band only after 2 consecutive
    polls. Not to be confused with the rejected confidence sizing — this is about frequency, not size. Verify
    on live sandbox fills before/after.

Output of Phase 1: the 6-window and 90-day numbers reproduced with a live-equivalent gate and
realistic costs; the live feature path is byte-identical to the backtest on a replayed day.

## Phase 2 — Formal statistics of the edge

2.1 **Registry of attempts + DSR/PBO.** Record all configurations tried (≥20 documented
    in AGENTS.md and `docs/habr-ai-article/backtest-results.md`, plus ~20 in this dispute). Compute
    the deflated Sharpe (Bailey & López de Prado) and the probability of backtest overfitting. The
    multiple-testing bar is t ≈ 3–4 (Harvey & Liu), not t=2. Without the registry DSR degenerates.
    Refs: arXiv:1408.1159, arXiv:2608.23808, Bailey & López de Prado 2014, Arnott-Harvey-Markowitz
    2019, Harvey & Liu 2015.

2.2 **Gap stress.** Hold-until-flip without stops; the only protection is the sticky kill switch, which
    does not liquidate positions. Scenario: the worst historical overnight gaps + synthetic −10%/−20%
    on the current portfolio, a report of the equity effect against the 3% floor.

2.3 **Beta/regime decomposition.** P&L net of IMOEX; long/short legs separately; return on gross
    exposure (not "% of the 1M deposit"); the "equal-weighted 18 tickers" benchmark (the null context of
    the universe) and the momentum benchmark from 0.2. Per-trade/per-day regression: P&L ~ alpha + β1·bench
    + β2·momentum, cluster SE. Label the regime of each OOS quarter (trend/flat/fall): two
    bull quarters ≠ alpha. The first cheap shot is to decompose per-trade P&L into the IMOEX component
    from the data available right now, without waiting for collection.

2.4 **OOS freeze.** Freeze the recipe and the code SHA, run on the next two quarters without
    edits; any change restarts the count from zero. Timeline: ≥2 quarters = ≥6 months
    of collection. While it accumulates, no real-money actions.

## Phase 3 — Costs, risk, infrastructure

3.1 **Short borrow.** Not modelled anywhere. Measure the real short conditions in the T-Bank sandbox
    (rate/holding term for the 18 names); if it cannot be measured, a stress of 0.005%/day on the
    short leg notional. In go/no-go only the stress scenario counts.

3.2 **Per-ticker spread.** Instead of a global constant, a per-ticker spread from the accumulated
    AlgoPack order book (live already writes audit_events); the half-spread model already exists
    (`engine.go:481-490`).

3.3 **Walk-forward persistence.** Save per-quarter models and decision logs (id,
    config, p, feature hash) in `artifacts/wf/<window>/` — they do not exist now, and a repeated honest
    OOS statistic is impossible.

3.4 **Attribution.** Realized P&L by ticker, concentration of the top-N trades, a realized/unrealized
    split in every report. Without this a few lucky trades are indistinguishable from an edge.

3.5 **Drift and circuit breaker.** PSI of features against the training distribution (warn), a per-ticker
    circuit breaker (stop a name after N losses or a cumulative loss > X% of notional).
    Do not enable auto-retraining (rejected 2026-09-16).

3.6 **Sandbox/paper parallel.** Log expected vs actual fill price, the rejection rate
    under the 0.3% marketable-limit cap, actual borrow charges. This is a direct measurement of the gaps
    from 3.1/3.2.

3.7 **A single source of metrics** — `docs/metrics.md`: window | realized vs MTM | artifact (commit) |
    costs | date. All numbers (+110742, +79453, +75028, +63212, +56379/+40102, +25699) go there.
    Process rule from incident E4 (a race of two sessions): commit metric-bearing edits
    immediately; the second session reads only HEAD or an explicitly requested uncommitted file. "git log -S
    is empty" does not prove absence when the tree is uncommitted.

3.8 **Kill-switch runbook:** before a manual reset, a check "equity ≥ 1.5 × max_notional across all
    open positions" (a frozen position must not unblock into a skewed portfolio).

## Pre-registered go/no-go criteria (real money)

Formulated now, before new runs. They may be softened only by a new explicit decision:

1. The Phase 1 output is done (live ≡ backtest, portfolio gate, tightened preflight).
2. Step 0 is passed: tail-precision is significantly better than the base rate, the model beats the momentum benchmark net of costs.
3. ≥2 quarters of frozen OOS with **realized** P&L > 0 at conservative costs
   (spread+slippage 0.05%+0.05%, borrow at the stress).
4. DSR > 0.95, PBO < 0.2 (the attempt counts come from the registry 2.1).
5. Positive alpha with t > 3 after subtracting IMOEX and the momentum benchmark; the regression specification
   and SE formulas are fixed in advance, the number of specifications tried goes to the registry.
6. Among the OOS quarters there is at least one non-bull segment (otherwise the result is marked
   as beta risk and does not pass).
7. Live-vs-backtest divergence within a pre-set tolerance over ≥1 month of the sandbox.
8. A full pass of the GO_LIVE_CHECKLIST, including a kill-switch drill on the sandbox.
9. All money is `decimal.Decimal` (GO_LIVE_CHECKLIST #1); float only in statistics/charts.

## Explicit non-goals

- No new features until the data-breadth problem is solved (oscillators already gave ±0.005 AUC).
- No tuning of thresholds/notional on the same 6 windows — that is the winner's curse.
- No auto-retraining and hot-swap of the model (rejected 2026-09-16; unchanged).
- No real execution without the full checklist and a new explicit sign-off.

## Open questions

- Whether to liquidate positions on the kill switch (live currently does not). Decided in 1.2; if "no" —
  the compensation is the gap stress 2.2.
- The exact borrow rate and whether the sandbox account charges it at all: 3.1 measures, does not guess.
- Whether tail-precision beats the base rate: unknown until 0.1 — this is the most informative
  experiment of the plan.
