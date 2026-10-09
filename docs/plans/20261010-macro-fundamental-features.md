# Plan: macro and fundamental features in the model, position/PnL awareness in the risk gate

Status: EXECUTED 2026-10-10. Part C and the feature plumbing shipped; the pre-registered evaluation found no AUC lift (docs/metrics.md "Macro and dividend-yield features"), so no model was retrained for deployment. Decided with the user: fundamentals and macro become model features; account positions and PnL go to the risk gate only, never into the model.

## 1. Why this split

- Macro and fundamentals have history, so the ensemble can be trained and walk-forward validated on them. Account positions and PnL have no history before the sandbox account existed, and training on the bot's own fills leaks the label (position follows past signals). They are constraints, not predictors.
- The 2026-10-10 incident (`docs/knowledge/model-quality-history.md`, "Live news_count train/serve skew") shows the failure mode of any feature that exists live but not in training. Every new feature below must be present in `exportdataset` history, in the live Builder and in backtest, produced by the same code path.

## 2. Part A: macro features (model)

Candidates, each as a daily series available point-in-time:

| Feature | Source (verify before building) | Note |
|---|---|---|
| IMOEX return 1d/5d/21d, IMOEX vs MA50 | MOEX ISS index history | already fetched for excess-return labels |
| USD/RUB (or CNY/RUB) return 5d/21d | MOEX ISS currency market | verify the board and history depth |
| Brent return 5d/21d | MOEX ISS FORTS continuous contract or an index | verify; fallback is dropping it |
| CBR key rate level and last change | cbr.ru | verify reachability from this network first; step series, small |

Rules: a macro value on date d uses only data published by the close of d-1 (no look-ahead). Macro features are cross-sectionally identical on a given day, so they can only shift the base rate and interact with ticker features in the trees; do not expect them to rank tickers. Report that limit in the result.

## 3. Part B: fundamental features (model)

- Candidates: dividend yield (existing `internal/dividends` calendar plus price), P/E, P/B, net debt / EBITDA.
- Open blocker: no verified point-in-time fundamentals source. Check T-Invest API asset fundamentals and any MOEX endpoint first. Fundamentals that are restated or looked up as of today leak the future when joined to old dates; use report-date-stamped values only, otherwise drop the feature.
- Dividend yield from the existing calendar is the only candidate that needs no new source; build it first.

## 4. Part C: position and PnL in the risk gate (not the model)

Already present: gross exposure cap (`risk.max_net_exposure`, `sector_caps`), daily loss, drawdown kill switch, per-ticker breaker. Missing and motivated by the ≈-219k net-short book that grew while IMOEX rose:

- Net directional cap: reject orders that push signed net exposure beyond `risk.max_net_short` / `risk.max_net_long` (RUB, 0 = disabled).
- Macro-regime guard (optional, config-gated): block new shorts while IMOEX is above its MA50 and rising, mirror for longs. Evaluate in backtest before enabling.
- Expose current net exposure and day PnL in the `risk_gate` audit payload for diagnosis.

## 5. Order of work and gates

1. Part C net directional cap first: no retrain needed, immediately limits damage. Regression test reproducing the all-SELL book.
2. Dividend yield feature (no new source), then macro series. Add to `ToVector` in a single change with `exportdataset`, `scripts/export_ensemble.py`, `scripts/compare_models.py` `FEATURES` and the parity fixture.
3. Retrain on the ai-box, then purged walk-forward (`-embargo-bars 10`), compare AUC and realized P&L with `cmd/strategyvalidation`, record every number in `docs/metrics.md` in the same commit.
4. Run the new ensemble as `model.challenger_ensemble_path` (shadow) first; `cmd/challengercompare` is evidence only.
5. Promotion to live needs the purged walk-forward gate and the user's explicit sign-off (no auto-swap, see AGENTS.md).

## 6. Constraints

- Zone: Part A/B touch `internal/features/**` and `internal/model/**` (ensemble/model zone); Part C touches `internal/risk/**`. The news analyzer is being built in another session: do not start code until `git status` is clean and the user confirms that session is not editing the same files (AGENTS.md rule 7).
- Pre-register the feature list and the success criterion (walk-forward OOS AUC and realized P&L vs current ensemble, with the date-bootstrap CI) before the first training run.

## 7. Implementation notes (2026-10-10)

- Part C shipped as `risk.max_net_short` / `risk.max_net_long` (`net_direction_exceeded`), set to 90000 in `config.sandbox.yaml`.
- Features: `internal/macro` (ISS + cbr.ru fetch, CSV store in `data/macro`, point-in-time `At(day)` using only closes strictly before the decision day), `features.NewMarketEnricher` (adds `FeatureContext.Extra`), model vectors by name via `model.ToVectorFor(feature, model.FeatureOrder)`. A model whose `feature_order` needs a feature that is missing fails the decision loudly instead of reading zero.
- USD/RUB was replaced by CNY/RUB: `USD000UTSTOM` has a 20-month hole on ISS (2024-06..2026-02), which would corrupt 5d/21d returns. Brent is a front-month chained index from `BR*` futures (roll gaps removed). Fundamentals other than dividend yield are NOT built: no point-in-time source was found.
- Feature names: `div_yield_ttm_pct`, `imoex_ret_5d`, `imoex_ret_21d`, `imoex_dist_ma50_pct`, `cnyrub_ret_5d`, `cnyrub_ret_21d`, `brent_ret_5d`, `brent_ret_21d`, `key_rate_pct`, `key_rate_chg_90d`.

## 8. Pre-registered evaluation (fixed before the first run)

- Data: `exportdataset -from 2024-01-01 -till 2026-10-09 -split 2026-06-18 -horizon-days 10 -deadband-pct 0.5 -label-mode absolute -macro-dir data/macro`, no news history, FX tickers excluded, run on the ai-box (`~/moex-research`).
- Protocol: the existing `evalh.py` `wf()` (expanding-window purged walk-forward, 63-day test folds from 2025-04-01, `label_date < fold start`, LightGBM + logistic average).
- Arms (all on the same rows): A = 15 price/flow columns (the baseline of `docs/knowledge/model-quality-history.md`), B = A + `div_yield_ttm_pct`, C = A + 9 macro columns, D = A + B + C.
- Primary metric: pooled AUC; secondary: mean fold AUC and number of folds with AUC > 0.5.
- Success criterion to consider a swap candidate: an arm beats A by at least +0.010 pooled AUC AND has a majority of folds > 0.5, AND realized P&L through `cmd/backtest -signal-source=ensemble -macro-dir data/macro` on the same windows does not fall below the deployed ensemble. Anything weaker is reported as no effect. Passing this is evidence only: deployment needs the user's explicit sign-off.
