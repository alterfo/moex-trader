# Experiment F: excess labels through the Task 13 gate (2026-10-18)

Russian translation: [20261018-excess-label-walkforward.ru.md](20261018-excess-label-walkforward.ru.md). This file is canonical.

Pre-registration before the run. The owner authorized work in the ensemble/model zone on 2026-10-18 ("I give permission, do all 4": this experiment is item 1 of four).

## Prior

The only large measured shift in predictive power in the project's history: excess labels (excess return over IMOEX) give val AUC +0.040 (0.5226 against 0.4824 for absolute), a beta loading of −0.189 against −0.302 and a positive residual alpha t≈2.0 after a realistic hedge (`docs/metrics.md`, "Market-neutral retraining: excess-to-IMOEX labels…", 2026-10-03). It was measured on a single validation window through `cmd/alphahedge`, not through the standard Task 5 protocol (6 purged windows, embargo 10).

The old excess run of 2026-09-16 (registry: `excess_label_baseline`, realized +235, 2/6 windows) was on the old pipeline before the fixes of 2026-09-17/18 and with different data — it does not cancel the measurement of 2026-10-03, but obliges us to check the transfer of the AUC effect into walk-forward.

## What is measured

The same protocol as the control `purged_walkforward_control` (Task 5); only `-label-mode` changes: `absolute` → `excess`. Two branches of one run:

- **F-control (abs)**: a fresh absolute control on HEAD, with the same code and news archive as F. The gate is computed against it (protection against engine drift — a lesson from the "100% short book" section in metrics.md: comparison only against a fresh control of the same run, the historical control is context).
- **F (exc)**: the same, with `-label-mode excess`.

## Protocol (fixed before the run)

Windows, training and flags are from `run-task5.sh` (workspace `~/moex-trader-task5` on the ai-box):

- Windows: q1 (train_till 2025-03-04, backtest 2025-03-05..2025-06-05), q2 (2025-06-05 / 2025-06-06..2025-09-05), q3 (2025-09-05 / 2025-09-06..2025-12-03), q4 (2025-12-03 / 2025-12-04..2026-03-04), w2 (2026-03-04 / 2026-03-05..2026-06-06), w1 (2026-06-06 / 2026-06-07..2026-09-16).
- Data (both branches, ai-box): `exportdataset -config config.sandbox.yaml -tickers <17 tickers from run-task5.sh> -from 2024-09-01 -till 2026-09-17 -split 2026-06-18 -horizon-days 10 -deadband-pct 0.5 -label-mode {absolute|excess} -news-history news_history.jsonl -out data-17f-{abs,exc}-full.csv`.
- `news_history.jsonl` = the archive from the Task 5 workspace (2026-09-18, 1193 records) — the same as in the historical control, so that training rows do not differ because of news. At inference the news features are zeroed (`untrainedFeatures`), so the archive affects only training.
- Reproducibility verification BEFORE the training runs: the absolute export must reproduce the Task 5 dataset structure (first rows 2024-09-02, YDEX 2024-10-22, SBER 641 rows, 10744 rows in total). A discrepancy of more than ±10 rows per ticker — stop and tell the owner; a smaller discrepancy is recorded in metrics.md.
- Training per window: `scripts/export_ensemble.py` with `ENSEMBLE_FEATURES14` = the 18 price/flow features from `run-task5.sh`, `ENSEMBLE_FEATURE_ORDER` = 28 names, `ENSEMBLE_TRAIN_TILL`/`ENSEMBLE_EMBARGO_SPLIT` = the window's train_till, `ENSEMBLE_EMBARGO_BARS=10`, venv `/home/oleg/moex-venv/bin/python`. The model is written to `ens-{abs,exc}-emb10-<window>.json`; val AUC is printed on the fixed window 2026-06-18..2026-09-16 (as in Task 5).
- Backtest per window: `./backtest -config config.sandbox.yaml -tickers <17> -from <bt_from> -till <bt_till> -deposit 1000000 -max-lots 1000 -commission-rate 0.0005 -spread-pct 0.0005 -slippage-pct 0.0005 -target-notional 15000 -lookback-days 0 -signal-source ensemble -ensemble-path <window model> -news-history news_history.jsonl -wf-dir wf/<abs17|exc17> -embargo-bars 10 -out bt-<variant>-17-<window>.md`. The `exportdataset`/`backtest` binaries are cross-compiled from the Mac to linux/amd64 (`CGO_ENABLED=0 GOOS=linux GOARCH=amd64`), as in `scripts/deploy-failover.sh`; on the ai-box they are placed in a separate workspace `~/moex-trader-task12`, nothing live is touched.
- Thresholds 0.60/0.40, hold until flip, kill-switch on — unchanged.

## Gate (Task 13 criteria, against F-control)

1. the total realized P&L of F > F-control (MTM does not count);
2. realized P&L of F ≥ F-control in at least 4 of 6 windows;
3. max DD ≤ 3% in each window and the kill-switch does not fire;
4. the mean val AUC of F is not lower than F-control by more than 0.005 (F is a label change, the criterion is applicable);
5. PBO over the matrix {F-control, F} through `cmd/strategyvalidation -returns-dirs`, `DefaultSplits`, < 0.5 — with two candidates PBO degenerates, the value is given descriptively;
6. a 90-day preflight with the candidate — only if 1–4 pass.

Hypotheses fixed before the run:

- **H_predictability** (reported, does not gate): the mean val AUC of F ≥ the mean val AUC of F-control + 0.005. A negative result (the effect of 2026-10-03 does not transfer into walk-forward) weakens the grounds for the challenger step (item 2 of four).
- **H_income** (gate 1–4): failure is expected, because excess deliberately removes the beta income that was ~2/3 of the realized P&L of the absolute book; criteria 1–2 will fix the price of the beta income by window. Passing all criteria 1–4 makes a replacement candidate, but no replacement happens without a purged walk-forward through `cmd/backtest` with costs and the owner's explicit consent (AGENTS.md invariant; auto-replacement is forbidden). The live recipe (`ensemble_model.json` + `config.sandbox.yaml`) does not change under any outcome of this experiment.

## Registration

- Registry: `experiment_f_excess_labels`, metric `realized_pnl_6q_purged`, value 0, status `pending` — added by this commit before the run (like experiments A–E of 2026-10-04).
- Results: the section "Experiment F: excess-to-IMOEX labels (2026-10-18)" in `docs/metrics.md` in the same commit as the numbers; realized P&L by window, trades, max DD, kill, val AUC, PBO, verdict on criteria 1–5.
- `make check` (`go test ./...`, `go vet ./...`, metrics.md lint) must pass in every commit.

## Outcome of F (2026-10-18)

Rejected on both hypotheses. Numbers in `docs/metrics.md`, section "Experiment F: excess-to-IMOEX labels (2026-10-18)".

- Reproducibility: the fresh absolute export matches the Task 5 dataset in all fields on all common rows (SBER check, 0 differences; the only difference is one trailing row of 2026-09-17 per ticker, within tolerance).
- Stack drift, not engine drift: with identical data, script and environment variables the trained models differ from Task 5 — the venv package versions changed (xgb 3.4.1, lgb 4.7.0, sklearn 1.9.1, pandas 3.0.5, numpy 2.5.3; the Task 5 versions were not recorded). The historical artifact `ens-emb10-q1.json` on the HEAD engine reproduces the Task 5 result by ~93% (+4002/74/3.56% against +4301/66/3.57%), so the engine is not the cause. The gate is computed against the fresh same-run control, as pre-registered.
- Gate: criteria 1 (total realized +9669.22 against +67815.78), 2 (1/6 windows not worse than the control) and 4 (mean val AUC 0.4830 against 0.5170, −0.034) are not passed; criterion 3 is passed (max DD ≤ 2.35%, kill never); PBO 0.1242 (degenerate with 2 candidates, descriptive).
- H_predictability is rejected: the AUC advantage of 2026-10-03 (+0.040 on a single window) does not transfer into walk-forward; on the current stack excess is worse than absolute on average, q1 excess AUC 0.4559 is below random.
- H_income is rejected. The expected low-beta profile was confirmed: DD ≤ 2.35% everywhere, the total realized is positive (+9669.22).
- Consequences: the excess model is NOT connected as a challenger (the precondition of item 2 was a positive H_predictability); item 2 of four is reconsidered with the owner. The live recipe is untouched.
- Conditions for re-checking: (1) the same experiment on a stack with pinned package versions (a requirements file, a frozen venv) — then comparison with the historical control is direct again; (2) an excess candidate through `cmd/alphahedge` on the fresh engine with a realized decomposition by window. Without at least one of the conditions F is not reopened.
