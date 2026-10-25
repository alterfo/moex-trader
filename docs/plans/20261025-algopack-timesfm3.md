# TimesFM 3 zero-shot forecasts as a feature (2026-10-25)

Status: REGISTERED 2026-10-25 (English only), before the features were evaluated. Item 9 of the owner's order of work ("can we fine-tune TimesFM, the latest one that predicts related series": TimesFM 3).

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

## Review log

(pending the Reviewer's pre-run pass)

## Errata

(none)
