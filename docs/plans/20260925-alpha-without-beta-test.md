# Pre-registered protocol: does alpha survive beta-neutral execution? (0)

Status: PROTOCOL ONLY. No numbers exist yet for this test. Acceptance bar is
fixed below BEFORE the first run; this document is the read-before-numbers
gate for both sessions (reviewed by the peer session before results are
reported).

RESULT (2026-09-25): see `docs/alphahedge-report.md` and the metrics.md
section "Alpha-without-beta hedge test (0)". Acceptance A and B both PASSED at
share=1.0; bifurcation consequence (1) is authorized.

## 1. Context (non-goal framing)

`docs/betaregime-report.md` shows the deployed ensemble's 18-month realized
P&L is dominated by IMOEX beta: +194293 RUB total, +129322 RUB beta
component, +64971 RUB excess; per-trade alpha t=1.469 (p=0.16, df=17),
portfolio structural beta1 (equal-weight bench) t=-6.186. The strategy today
is best described as a net-short IMOEX bet that happened to face 5/6 down-or-
flat OOS quarters, i.e. undistinguishable per-trade alpha on top.

This test asks only: **if the same 18 signals are traded with the net
exposure neutralized (execution-level), does a statistically robust daily
alpha remain?** It does NOT retune, retrain, or change the signal. Result is
binary for the (0)->bifurcation gate.

## 2. Reproducible inputs (fixed)

- Artifact: `ensemble_model.json` @ HEAD (deployed abs-10d recipe).
- Tickers: the 18 from `docs/betaregime-report.md` (YDEX..POSI).
- Window: 2025-04-01 -> 2026-09-17, single continuous backtest (no per-window
  retraining — no seed drift), deposit 1000000, target notional 15000,
  max_lots 1000, warmup standard, kill-switch on.
- Costs (all three, fractions of price): commission 0.0005, spread 0.0005,
  slippage 0.0005 — identical to the report.
- Inputs: MOEX ISS candles (tickers + IMOEX), same as `cmd/betaregime`.
- Benchmarks (same as report): equal-weight 18-ticker daily return; momentum
  long+short benchmark (as measured in the report, re-normalized identically).

## 3. (0a) Overlay-hedge — PRIMARY method

- Per-name beta bi: OLS of each ticker's daily log close-close returns on
  IMOEX daily log returns, full-window (2025-04-01 -> 2026-09-17). Used as a
  measurement, not an inference.
- Daily aggregate exposure N(t) = sum over OPEN positions of beta_i * entry
  notional(at repricing: position_notional_i). Positions unchanged by the
  overlay: the overlay never modifies a ticker position's sign or size.
- Overlay leg: synthetic IMOEX position sized to -share * N(t), share in
  {0, 0.5, 1.0} (share=0 -> unhedged baseline = reproduces the report).
  Leg rebalanced daily at close.
- Overlay costs: commission 0.0005 + spread 0.0005 applied to overlay-leg
  notional turnover at each daily rebalance (conservative; same as legs).
- Daily overlay P&L = -share * N(t) * imoex_daily_return - leg_costs.
- Invariant: NOTHING in the 18 tickers changes; the overlay is strictly
  additive. No reweighting in the primary method.

## 4. (0a) Secondary — reweighting (sensitivity only)

Solution of sum(beta_i * w_i) = 0 over the 18 signals. Run EXCLUDED from all
acceptance conclusions; additionally, if any reweighted position flips sign
against the model's BUY/SELL, the run is marked NON_ACCEPTANCE and reported
only as context.

## 5. (0b) Significance — the deciding test

NOTE (fixed before run): the overlay is a separate leg, so per-trade alpha is
IDENTICAL hedged vs unhedged by construction. Per-trade regression is
therefore descriptive only and is NOT an acceptance condition. Only the daily
series can discriminate.

- Daily alpha series e_d (per share level s): residual of the full-window
  daily regression r_p ~ alpha + beta1*equal_weight_bench + beta2*momentum on
  the overlay-adjusted daily portfolio returns (r_p includes overlay P&L, and
  the leg costs are in r_p).
- Stationary block bootstrap with geometric block lengths, mean block L in
  {5, 10, 20, 40, 60} trading days, N = 2000 replicates, percentile
  CI(alpha_boot) = [2.5%, 97.5%]. Rationale for block range: hold-until-flip
  creates dependence of order of the holding period (variable, up to tens of
  days); a single block length cannot be trusted.
- Leave-one-quarter-out: re-fit the daily regression and re-estimate mean
  alpha dropping each of the 6 OOS quarters in turn. Explicitly includes the
  2026-Q2 (-15.39%) drop. Supertest of "the alpha is one quarter".
- Quarterly alpha breakdown reported (descriptive; how Q2's share of alpha
  changes from unhedged to share=1.0 is a key diagnostic).

## 6. Acceptance bar (FIXED, pre-run)

With share = 1.0 (fully neutralized):

- (A) Bootstrap CI excludes 0 on a CONTIGUOUS run of block lengths that
  INCLUDES {20, 40} (significance at a single length is not enough).
- (B) Leave-one-quarter-out: mean alpha keeps sign across all six drops.
- (C) Agreement is not required with unhedged series; unhedged serves to
  decompose, hedged decides.

A AND B both hold  -> alpha survives beta-neutral execution -> proceed to
                      (1) managed-beta variant (inverse-vol sizing and a
                      config'd max_net_exposure as SEPARATE axes; never
                      conflated).
Either fails          -> alpha is residual beta/noise -> bifurcation: product
                      decision on a managed beta-short vs redirect resources
                      to (2) event-drift research with event-study +
                      spread-proxy gate.
Secondary reweighting (section 4) never counts toward acceptance.

## 7. Metrics discipline

Numbers land in `docs/metrics.md` in the SAME commit as the executor code.
No metric adds/edits before the run.