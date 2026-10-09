# Income levers and hedging

Read before reopening leverage, `max_net_exposure`, dividend capture, futures hedging or market-neutral labels.

Moved verbatim from AGENTS.md on 2026-10-09; the rules in AGENTS.md itself still take precedence.

## Income levers closed, futures-hedge feasibility is the only remaining lever (2026-09-25)

Both income paths explored since 2026-09-16 are closed by evidence; the live
trader (PID 1764, `config.sandbox.yaml`, `target_notional 15000`, no cap) is
**the best-tested configuration and must NOT be changed** — every alternative
tested today (leverage 2-3x unhedged, `max_net_exposure`, dividend capture)
performs worse on realized P&L and/or DD. Treat it as the control point and
accumulate a 2-3-month proving period.

- **(2) dividend capture REJECTED** (commit `ebb13b9`): 123 events, 65 in the
  study window, gap ratio −0.97 (ex-date decline fully priced); primary
  offsets {−2,−3,−5} fail under both tax 0.13/0.00; only offset −1 at tax=0
  excludes 0 (outside the pre-registered subset). Report:
  `docs/dividendstudy-report.md`; calendar `data/dividends.jsonl`.
- **(1) managed-beta leverage closed on the unhedged book** (commits
  `bc84403`, `abe614d`): `risk.max_net_exposure` built and contains the 3%
  DD only at 2x+60k (+72545 realized, DD≤2.21%), which underperforms live 1x
  (+179045, DD 2.05%); the cap is cliff-like (60k→75k flips 2026-Q1 DD
  0.55%→3.46%) and 3x fails at every cap (concentration). Root cause: the
  unhedged book's income IS its market beta, and an aggregate cap destroys
  the cross-name diversification (124-249 trades vs 781-939 uncapped).
  Grid + verdict: `docs/metrics.md` "max_net_exposure on the unhedged book".
  `max_net_exposure` ships as defence-in-depth guardrail only, not as a
  sizing mechanism; backtest/alphahedge expose it via `-max-net-exposure`.
- **`max_net_exposure` means GROSS, not signed net (2026-10-01)**: the cap
  bounds `Σ_ticker |signed position notional|`. The old signed-net basis was
  blind to gross risk — long 500k SBER + short 500k OZON net to **zero** and
  passed a 60k cap carrying 1M of risk — and made *which* names filled depend
  on how much opposing exposure was booked first. Flag/config name
  (`-max-net-exposure`, `MaxNetExposure`) unchanged on purpose.
  Three invariants when touching this cap:
  (1) **Project per ticker, never book-wide addition.** The order replaces only
  its own ticker's leg; the bound is `|other names| + |this name after|`.
  Adding `|delta|` to total gross makes the cap reject *risk-reducing* orders —
  an intermediate implementation passed cap 0 and silently blocked every
  flatten-to-flat order (all-short grid: 2-6 closed trades → **0**, drawdown
  unchanged, result left as open-position MTM).
  (2) **`Signal.TargetLots` is NOT the exposure delta.** In the target-position
  path it is the absolute target that `recordFill` reconciles to; reading it as
  a delta double-counts the held position (a real pre-existing bug, fixed here).
  `risk.Request.ExposureDeltaLots` carries the signed delta; nil means
  "TargetLots is the order size" (live `EnsembleSignalSource`, incremental).
  (3) **Re-run the all-short grid** (`docs/metrics.md` "signed net → gross")
  after any cap change — net ≡ gross there, so every row must stay bit-identical;
  a changed row means the projection is wrong, not that the market moved.
  Live keeps `max_net_exposure: 0`, so this commit changed no live behavior.
- **Futures hedge MEASURED, and the whole beta/market-neutral line is CLOSED
  (2026-10-02/03).** The T-Invest feasibility probe (`cmd/tinvestprobe`) came
  back all-yes (MOEX index futures MX + margin + futures candles + sandbox), so
  the "engineering task, not a measurement" backlog was actually measured:
  - **Phase 1, real IMOEX-futures overlay** (`cmd/alphahedge/futures.go`,
    commit `10b26bb`): a real MX contract is 230000 RUB notional vs ~107k avg
    beta exposure, so the hedge is bang-bang (one contract ≈ 2x the book's
    beta); at share 1.0 the leg loses ~-175k and the residual alpha goes to
    t≈-0.49 (CI includes 0). Real hedge kills the alpha — the synthetic-leg
    claim does not survive the actual instrument. `docs/metrics.md`
    "Real IMOEX-futures overlay".
  - **Phase 2, excess-to-IMOEX labels** (commit `54daff4`): retraining on
    market-neutral labels raises val AUC **+0.040** (0.4824→0.5226) and halves
    the beta loading, a positive residual alpha — but after a realistic fine
    hedge it is only **t≈2.0, p≈0.10** (below the pre-registered bar) and still
    needs a finer instrument. Not deployable on its own. `docs/metrics.md`
    "Market-neutral retraining".
  - **Phase 2b, regime-balanced training REJECTED** (commit `620e669`):
    inverse-frequency reweighting by trailing market regime lowers val AUC in
    every variant (0.4804–0.4964 vs 0.5226) and turns the hedged residual alpha
    **negative**. Env-gated in `scripts/export_ensemble.py`, default off.
  - **Beta-residualized price features DECLINED by the user (2026-10-03)**:
    they need the IMOEX series inside the live feature path
    (`internal/orchestrator` ingest → `internal/features`), a cross-zone change
    with live/backtest parity risk, and the label change above already captures
    most of the market-neutrality — defer permanently unless a new reason
    appears.
  **Consequence: the income-lever search is exhausted under current constraints
  and the live 1x book (`config.sandbox.yaml`, `target_notional 15000`, no cap)
  stays the operating optimum. Do not reopen the beta/market-neutral line
  without a new, pre-registered reason (finer hedge instrument or a materially
  larger book).**
