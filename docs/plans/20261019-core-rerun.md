# Core rerun on refreshed data (pre-registration)

Russian translation: [20261019-core-rerun.ru.md](20261019-core-rerun.ru.md). This file is canonical.

Status: draft pre-registration, written 2026-10-19 before any rerun. Owner decisions (2026-10-19): the same data sources refreshed up to the run date; the key core only; the result is a separate paper in `docs/research/` (English plus Russian twin), while `docs/metrics.md` stays the registry of numbers and links to it.

## Purpose

Check whether the core findings of [../metrics.md](../metrics.md) hold on more data, and measure them on a genuine out-of-sample period that did not exist when the criteria were set. The rerun repeats procedures; it does not search for new ones.

## Rules fixed in advance

1. **Freeze date.** A single data freeze date `D` is chosen after the data inventory of the ai-box and recorded in the paper before the first run. No datum after `D` enters any fit, selection or threshold. Rows dated after 2026-10-18 form the **forward window**: they are evaluated once, never used to tune, and reported separately from the refreshed in-sample windows.
2. **Criteria are inherited verbatim** from the original studies (sections cited below); no threshold, horizon, dead band, cost or seed is changed. Where an original criterion needs a window that no longer exists, the nearest equivalent is stated here before the run, not after.
3. **Side-by-side reporting.** Every result is printed next to the number it replicates (section of `docs/metrics.md`), with the data difference listed (rows added, tickers added, period extended). A difference is judged against the measured noise of the procedure (N95 = 0.016 AUC for single-configuration AUC comparisons, section 3.6; seed-to-seed ranges for replays); paired comparisons use at least 15 configurations.
4. **Fresh controls.** Every comparison uses a control retrained in the same run, never a number from the ledger (stack drift, section 0.3).
5. **Multiple testing.** Each rerun study is added to the attempt registry before its numbers exist; the paper reports the deflated Sharpe and PBO of the rerun set.
6. **Deviations.** Any departure from this plan is listed in the paper under "Deviations", with the reason and the effect.
7. **Safety invariants unchanged.** Research runs on the ai-box write only to research directories; no job writes `ensemble_model.json`, restarts `./trader` or touches the live path; real-money execution stays disabled.
8. **Structural stability.** Following the hard/soft-model rule recorded in [../knowledge/soft-models-and-optimization.md](../knowledge/soft-models-and-optimization.md): every study R1-R5 declares a perturbation family before its run, and a positive verdict counts only if it is unchanged across the whole family. Families: costs per side at 0.5x, 1x and 2x (borrow at 0.00005, 0.00023 and 0.00047 per day where shorts matter); first-test shifts 0, 27 and 54 days; at least five seeds; the universe definition (survivor panel against point-in-time) where a panel is used; for threshold-based books the fixed 0.60/0.40 pair against the 80th/20th quantile rule. Each study is reported as stable-pass, stable-fail or unstable; an unstable result is recorded as "no finding" and never as a pass. The perturbation values above are fixed here, not tuned afterwards.
9. **Boundary optima.** Any selected parameter (threshold, cap, notional, horizon) is reported together with its two grid neighbours. If realized P&L at either neighbour is less than half of the chosen value, or changes sign, the choice is flagged as a boundary optimum and is not eligible for a deployment recommendation; the neighbour grid is declared before the run.

## Studies (key core)

| id | study | original | criteria inherited from |
|---|---|---|---|
| R1 | Purged walk-forward baseline of the recipe, embargo 10, windows extended to `D`; evaluation-procedure variance (75 configurations of recipe A, 15 of D) | 3.5, 3.6 | gate definition 4.1, N95 rule 3.6 |
| R2 | Gate on the control and on the variants that came closest (C-logistic, B, E) and on excess labels (F), fresh same-run controls | 4.1-4.3 | the six gate criteria of 4.1 |
| R3 | News: classifier on the expanded archive (2c), score net of costs (2g), zero-shot LLM direction (7.7) on samples extended to `D` | 7.5, 7.6, 7.7 | AUC and CI rules of 7.5, the net-spread rule of 7.6, the H2 rule of 7.7 |
| R4 | Cross-sectional signals on the point-in-time panel: composite and LightGBM (step 18), sequential composite (step 23), ridge and MLP (step 25), borrow-cost sensitivity | 8.4-8.6 | registered rules of 8.4 and 8.6 (t >= 2.6, Sharpe >= 0.5, positive years, positive after 2022-06) |
| R5 | Gate-candidate search for the ridge book (rolling 90-day windows every 15 days) | 8.8 | pass share >= 70%, positive sum, flip gap >= 20 points |
| R6 | Live evidence: sandbox P&L since 2026-10-19, forward tracks of the ridge, C2 and high52 books | 9.7, 8.6 | the forward criterion of plan step 22 |

Studies outside the core (calendar effects, cycles, ICA portfolio, level crossings, FUTOI, report events, the sell-bias and retrain diagnostics) are not rerun here; their records stay as written.

## Data and method sections the paper must contain

- **Data**: for each source the path or command, coverage dates before and after refresh, row counts, tickers, known gaps (delisted names, weekend sessions, unresolved tickers, free-tier delays), licence notes (news sources are ToS-restricted; see `AGENTS.md`).
- **Method**: engine and cost model (section 0 and 2 of `docs/metrics.md`), label and feature definitions, windows, seeds, bootstrap settings, the criteria above, the reproduction check of each study against its original number.
- **Studies and results**: per study a table with columns original / refreshed / forward, the verdict against the inherited criterion, and a one-paragraph interpretation.
- **Limitations and deviations**.

## Deliverables

`docs/research/20261019-core-rerun.md` and its `.ru.md` twin; a new section in `docs/metrics.md` (and its twin) with the numbers in the same commit; this plan moved to `docs/plans/completed/` when the paper lands.

## Data inventory on the ai-box (verified 2026-10-19)

| dataset | path on the ai-box | last data point | action for the rerun |
|---|---|---|---|
| depth dataset `h10.csv` | `~/moex-research/depth/h10.csv` | 2026-10-07 | rebuild with `exportdataset` to `D` |
| TQBR snapshots | `~/moex-shadow/tqbr_snap.csv` | 2026-10-16 | extend with `cmd/tqbrsnap`; a half-written `tqbr_snap.csv.new` is a leftover to ignore |
| ridge score table | `~/moex-shadow/out/ridge_table.csv` | 2026-10-16 | regenerate; a cron job rewrites it daily at 16:50 box time, so read a frozen copy |
| news archive (daily merged) | `~/moex-trader-daily/work/news_history.jsonl` | 2026-10-18 | use this one (the copy in `~/moex-research/data/` stops at 2026-09-29) |
| futures open interest | `~/moex-research/hypo/futoi_all.csv` | 2026-10-02 | not part of the core rerun |
| 10-minute candles | not kept | - | not part of the core rerun; would be refetched with `cmd/intradayfetch` |

Resources: the ai-box GPU is nearly full (the local qwen3.8 model is resident, 22693 of 24564 MiB used), so R3's zero-shot LLM study cannot run in parallel with other GPU jobs; the sandbox trader stack and cron jobs (`factor_shadow.py`, `ridge_probs.py`, trade recorder) are live and must not be disturbed. Research output goes to a new directory outside the deploy tree.

## Open items to settle before the first run

The freeze date `D` (the latest common data point is 2026-10-16 for prices, 2026-10-18 for news); which of the closest variants in R2 are rerun if time is short (priority: control, C-logistic, F); the seeds of R1 (seed 42 plus the five seeds of step 17).
