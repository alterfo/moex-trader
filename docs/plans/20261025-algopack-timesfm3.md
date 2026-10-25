# TimesFM 3 zero-shot forecasts as a feature (2026-10-25)

Status: DONE 2026-10-25 (English only; scored once, 0 of 3, the covariate placebo equals the treatment; a post-hoc momentum lead found), registered before the features were evaluated. Item 9 of the owner's order of work ("can we fine-tune TimesFM, the latest one that predicts related series": TimesFM 3).

## What TimesFM 3 is (README of google-research/timesfm, read 2026-10-25)

Released in August 2026, 330 million parameters, native multivariate forecasting and past-only and past-and-future dynamic covariates, zero-shot, context up to 15,360 points, checkpoint `google/timesfm-3.0-pytorch`. The weights are under a non-commercial, non-production licence (commercial use only through Google Cloud services such as BigQuery ML): **research use only; the model cannot be part of a paid product**. The README gives a fine-tuning recipe only for TimesFM 2.5 (LoRA with HuggingFace Transformers and PEFT); for 3.0 there is no official recipe. Fine-tuning is therefore **not part of this plan**: a LoRA on the 3.0 weights would be custom code on an undocumented architecture, on about 450 honest dates, and the zero-shot test below decides whether any further effort has a point. A single independent review found no gain from the covariate and multivariate modes on calendar covariates; this plan tests the covariates that matter for us (order-flow features).

## Python exception

The project rule is Go or Rust only; TimesFM has only a Python runtime. By the owner's explicit request one isolated script, `scripts/timesfm_features.py`, runs inference in a separate virtual environment on the ai-box (`~/timesfm-study/venv`, outside the project's Python environment); it writes a CSV of forecasts and nothing else. All evaluation is in Go (`cmd/alphafit -mode libext`).

## Features (causal)

For each realtime feature date d in 2025-01-03..2026-10-16 and each top-100 name with at least 256 observations ending at d, the context is the last 256 observations of the name's abnormal daily return (its `rb1` minus the equal-weight mean of `rb1` over the names of that date; missing as 0), the horizon is 10, `use_znorm` on, `make_positive` off (the library default clips forecasts at zero), no stitching changes; the feature is the sum of the ten forecast steps.

- **tfm_uni**: zero-shot univariate forecast of the abnormal-return series.
- **tfm_cov**: the same with four past-only covariates over the same 256 observations: `A2`, `LevAsym`, `SpLv`, `ImbVol` (the strongest flow and book-shape features of the composite; missing as 0).
- **tfm_plc**: the same as tfm_cov but with the covariates of another ticker (the next name in alphabetical order on that date): a placebo for the covariate channel.

## Statistic and verdict (frozen)

`cmd/alphafit -mode libext -candfile <CSV> -frozen artifacts/algopack-wave3/w3-frozen-h10.json`: for each candidate, per date, the rank-scaled candidate against the 10-day abnormal rank: the ordinary IC and the incremental slope over the out-of-fold composite (the wave-3 primary composite, identical to library v2), mean over 450 dates with Newey-West lag 10 and the two halves split at 2026-01-05. Five within-date permutations of tfm_uni are the controls. **Hit**: |t| of the incremental slope at least 3.0, the same sign in both halves and for the IC; one control hit voids all. tfm_plc is reported as a candidate: if it reaches the same statistic as tfm_cov, the effect is generic and not due to the covariates. Three candidates, one verdict.

## Honest expectations

Foundation models for time series are trained on level and seasonality structure; daily abnormal returns are close to white noise. The expected result of tfm_uni is zero. tfm_cov can at most re-express, through a regression of the series on the covariates, information the composite already holds. A hit would be surprising; a null closes the question for 3.0 and removes the motivation for a fine-tuning attempt. Prior of any hit: low (a few per cent).

## Run order

1. Reviewer reads this file, `scripts/timesfm_features.py` and the `libext` mode.
2. One evaluation in Go on the ai-box.
3. Numbers into `docs/metrics.md` 8.34 and `docs/knowledge/model-quality-history.md`.

## Result (scored once, 2026-10-25)

Numbers in `docs/metrics.md` 8.34, outputs `artifacts/algopack-timesfm3/`. 40,851 forecasts (about 89 names on each of 457 dates). **0 hits of 3, 0 of 5 controls.** Ordinary IC and incremental slope over the composite: tfm_uni IC +0.0666 (t +2.46), slope +0.0376 (t +1.44); tfm_cov IC +0.0814 (t +3.22), slope +0.0561 (t +2.35, halves +0.056 / +0.056); **tfm_plc (the placebo with another name's covariates) IC +0.0825 (t +3.01), slope +0.0571 (t +2.14)**: the covariates of the name itself add nothing over the covariates of a different name, so the covariate channel is not what the model uses. Registered reading: no pass; with the placebo level equal to the treatment, there is no case for a fine-tuning attempt.

**Post-hoc diagnostic (added after seeing the positive IC of the univariate arm; not registered, labelled as such).** The univariate forecast of a 256-day context has rank correlation +0.65 (tfm_uni), +0.60 (tfm_cov) and +0.67 (tfm_plc) with the plain 256-observation sum of the same abnormal returns, i.e. with 12-month relative momentum. Tested through the same libext machinery, three momentum candidates: mom60 IC +0.032 (slope t +0.4), mom120 IC +0.071 (slope t +2.0), **mom256 IC +0.107 (t +3.95), incremental slope over the composite +0.0886 (t +3.41, halves +0.106 / +0.066), passing the rule (|t| at least 3.0, same sign in both halves and for the IC), with 0 of 5 placebo permutations hitting**. The composite contains only 1-, 5- and 20-day return ranks, so the 12-month momentum is a factor it lacks. Independent price sample (the daily IC of mom256 against the 10-day abnormal rank by calendar year, top-100 names with 256 observations): 2021 +0.095, 2022 +0.010, 2023 +0.008, 2024 +0.050, 2025 +0.121, 2026 +0.090: positive in every year, about +0.038 on average over 2021-2024 (919 dates) against +0.108 in the realtime window. This is a lead from a post-hoc look at a spent window with a theoretical basis (cross-sectional momentum) and an independent positive history; it is not confirmed. Pre-registered next step, not run here: add the 120- and 256-observation momentum ranks to the composite as controls (like the 20-day return rank), refit by the wave-3 chain, and test the extended composite against the frozen baseline with the economic gate and the carry of 27-29%; the Newey-West t of mom256 on 2021-2024 alone is the independent confirmation to compute first.

## Review log

Pre-run pass (opencode, 2026-10-25): the context is causal, the covariate placebo is a fair permutation of the covariate channel, the API calls and shapes match the plan, libext reuses the frozen composite and the lib2 statistic, the three arms differ only in the covariate channel; two pins applied (errata).

## Errata

Written 2026-10-25 after the Reviewer's pass, before the evaluation.

1. A shape mismatch in the forecast output fails the generation job loudly; it cannot silently produce zeros.
2. The covariate context is 256 observations (about one trading year): feature dates before about 2026-01-12 have part of the covariate context in the regenerated flow history of 2024, so for roughly the first 57% of the window the covariates carry regenerated values; the halves condition absorbs part of this and the result states the split.
3. The feature is the sum of ten daily forecast steps (d+1 to d+10) against a close-to-close 10-day outcome from the close of d: aligned up to one overnight boundary.
