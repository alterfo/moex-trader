# Structural-stability candidates (draft pre-registration)

Status: draft, written 2026-10-20 before any run. English only (plans have no Russian twin, owner decision 2026-10-19). Source of the ideas: [../knowledge/soft-models-and-optimization.md](../knowledge/soft-models-and-optimization.md) (Arnold, "Hard and soft mathematical models"); the carry-over to this repo is an interpretation, not a claim made by the book. Numbers cited come from [../metrics.md](../metrics.md).

Common rules: criteria below are fixed before the first run; the data freeze date `D` and the fresh-control, side-by-side and deviation rules of [20261019-core-rerun.md](20261019-core-rerun.md) apply; research runs write only to research directories; nothing here touches the live path or `ensemble_model.json`. Each candidate is added to the attempt registry before its numbers exist.

## C1. State-dependent sizing ("differential quota")

Idea: a hard plan (a fixed 15000 RUB per position, fixed exposure caps) sits on the boundary of its stability region; a rule that reads the current state is stable at the same average yield. The volatility-scaled size of experiment D (section 4.2) read the instrument's volatility, not the state of the book, and was P&L-neutral (-20.00 RUB).

Test: replay the Task 5 windows with the gate shape of section 3.5 and the target notional replaced by `N_t = clamp(N_0 * (1 - DD_t / DD_max), 0.25 N_0, N_0)`, where `DD_t` is the current drawdown of the book and `DD_max` the 3% kill level; the baseline is the fixed `N_0 = 15000` rerun in the same run. Seeds, shifts and costs as in the core rerun.

Criteria (risk test, not an alpha claim): (1) worst-window max drawdown not above the baseline's; (2) the number of kill-switch trips not above the baseline's; (3) total realized P&L not below 0.8 of the baseline; (4) the three conditions hold in at least 4 of 5 seeds. All four required; otherwise rejected. A pass authorises a further pre-registration only.

## C2. Ablation audit of the decision chain

Idea: every layer between the model and the executor adds interaction risk, and two interactions are already on record: the `event_dividend` out-of-range veto (section 9.2) and the ticker breaker blocking closing orders (section 9.5).

Test: with the gate-shaped replay of section 9.3 (17 tickers, 90-day rolling windows every 15 days from 2025-03-05), measure realized net of borrow for the full chain and with each removable layer switched off in turn (signal hysteresis, news gate, news exit, ticker breaker, net-exposure caps, lot resolver where applicable). Output: a table layer x effect on realized, trades, drawdown and the share of gate passes, added to `docs/ARCHITECTURE.md` as a per-layer effect note.

Criterion: a layer is flagged as an interaction risk when switching it off changes the gate pass share by 15 points or more, or changes the sign of the three-window sum; flagged layers go to the owner with the evidence. No layer is removed by this plan.

## C3. First-digit data-quality diagnostic

Idea: the first digits of positive quantities spread over several orders of magnitude follow `lg(1 + 1/i)` (the first digit 1 about 30%, 9 about 5%); a gross departure hints at wrong units, filtering or duplication. MOEX prices are neither geometric progressions nor scale-free, so the diagnostic applies only to turnover, volume, capitalisation and dividend-per-share magnitudes, never to prices.

Test: compute the first-digit shares of daily value traded, volume and dividend amounts per ticker class on the refreshed data, and compare with the expected shares by a chi-square statistic with a pre-declared threshold of the 99th percentile (8 degrees of freedom). Run it on the datasets of the core rerun before they are used, and on the known case (VTBR before and after the share consolidation) as a positive control.

Criterion: the diagnostic is useful only if it flags the positive control and flags at most 5% of the other series; otherwise it is dropped. It never blocks a pipeline automatically.

## Out of scope

The perestroika laws and the multi-stage control model are recorded as design heuristics in the knowledge note and are not tested here.
