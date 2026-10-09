# Feature pipeline and model-to-Go inference

Read before touching `internal/model/features.go`, `scripts/export_ensemble.py`, `cmd/exportdataset`, or any XGBoost/LGBM export or Go inference code.

Moved verbatim from AGENTS.md on 2026-10-09; the rules in AGENTS.md itself still take precedence.

## Feature pipeline

- The feature vector order is defined once in `internal/model/features.go` (`defaultFeatureOrder`/`ToVector`) and must stay mirrored only in the Python side (`scripts/compare_models.py` `FEATURES`, `scripts/export_ensemble.py`); `cmd/exportdataset` derives its CSV header/rows from `model.ToVector` automatically. A silent mismatch here is exactly the failure mode in ownership rule 4 of AGENTS.md, and an ungated `TestDeployedModelArtifactsMatchFeatureOrder` now checks both tracked artifacts against `ToVector`.
- Current canonical order (28 names, as of 2026-09-18): the 18 price/flow features — original 14 plus `macd_hist_pct, stoch_k_14, williams_r_14, alligator_spread_pct` — followed by the 8 `event_*` flags, then `negotiations_signal, sanctions_signal`. The deployed `ensemble_model.json` trains on only the 18 price/flow columns; the remaining 10 names are zero-coefficient placeholders that preserve live feature-order parity.
- `order_book_imbalance` is structurally always 0 in `cmd/exportdataset`'s historical CSV export — no historical order-book snapshots exist. `news_sentiment` / `news_count` default to 0 too, but `cmd/exportdataset -news-history <path>` (added 2026-09-16, mirrors `cmd/trainmodel -news-history` and `cmd/calibrate -news-history`) will backfill real values from a finanalys-format `news_history.jsonl` (see `cmd/newsfetch`) via `model.LoadFinanalysNewsHistory`/`AggregateDailySentiment`/`AggregateDailyTopicSignals`, matched by ticker+date. Without that flag, or for dates/tickers missing from the archive, all three groups still fall back to 0 — a feature-importance run on a dataset built without `-news-history` will still show them as dead weight for that reason alone.
- `SBMM` (ETF "Первая – Фонд Сберегательный") is NOT tradable via the T-Invest API (`apiTradeAvailableFlag=false`) — dropped from the ticker list and replaced with `POSI`. Don't reintroduce it as an order target.

## XGBoost/LGBM → Go inference (solved — don't re-derive)

- Export with `booster.save_model("model.json")` (full native format). Never `dump_model()` (lossy, different node schema, no `base_score`) or `save_raw()` without `raw_format="json"` (defaults to a UBJSON binary in modern xgboost — feeding that to a JSON parser is what caused a parse hang during development).
- `learner.learner_model_param.base_score` in that JSON is a **string wrapped in brackets**, e.g. `"[4.7E-1]"` — strip the brackets before parsing as float.
- With `boost_from_average` (xgboost's default), `base_score` is stored in **probability space** (the label mean), not margin space — transform via `base_logit = log(bs/(1-bs))` before summing with tree outputs. This was independently re-derived and confirmed twice (both agent sessions, separately) against `predict_proba` — trust it without re-deriving again unless a fresh accuracy check disagrees.
- Leaf value = `split_conditions[node]` at a **leaf** node (`left_children[node] == -1`). `base_weights[node]` only equals it at leaves; at internal nodes it is neither the split threshold nor a usable leaf value.
- XGBoost's internal split comparisons are float32; LGBM's are float64. Quantize XGB tree inputs to float32 before comparing against split thresholds, or you'll see ~0.01-level mismatches at certain boundaries. Verified end-to-end accuracy vs Python `predict_proba`: 6.4e-8 max abs error on the full validation set.
- Final probability = `sigmoid(base_logit + Σ leaf values across all trees)` for `binary:logistic`.
- LGBM tree JSON recursive builder gotcha: append a `-1` placeholder for a child index **before** recursing, then overwrite it (`lc[i] = walk(...)`). Appending the recursive call's result directly (`lc.append(walk(...))`) executes children out of order and corrupts the flat array indices.
