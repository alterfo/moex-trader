# Negotiations/Sanctions Signal Feature

## Overview

The market thesis driving this plan: the RF market currently trades on the
sanctions-discount/Ukraine-negotiation news flow, not oil price or the CBR key rate.
Signal map: (a) substantive negotiation progress → strong risk-on; (b) real enforcement of
100% tariffs against buyers of Russian oil → stress; (c) base case — sideways on
dividends/rate cuts, with volatility spiking on every Witkoff-related headline.

Neither "oil" nor "rate" is currently an explicit feature in this model (there is nothing
to demote), and neither is "negotiations" (there is nothing to boost yet) — see Technical
Details for what actually exists today. This plan adds a **signed negotiations/sanctions
signal feature**, fixes the live/train sentiment mismatch that currently blocks it from
working correctly live, and only keeps the result in the deployed ensemble if it clears
the same AUC/backtest evidence bar the repo already applies to every other feature
addition.

## Context (from discovery)

- `news_classifier.json` (repo root) is a flat bag-of-words logistic regression over
  headline tokens (`internal/model/newsclassifier.go`) — every token, including
  `санкции`, `нефть`, `ставка`, `переговоры`, has one independent scalar coefficient
  feeding a single sigmoid that predicts forward IMOEX-excess-return direction. There is
  no topic/category grouping inside it.
- Two disconnected sentiment paths exist today:
  - **Live/backtest**: `internal/features/builder.go` (`aggregateNewsSentiment`/
    `articlePolarity`) scores headlines with a hardcoded keyword lexicon
    (`positiveKeywords`/`negativeKeywords`). It never loads `news_classifier.json`.
    The lexicon hardcodes `санкц` as always-negative regardless of context — a
    sanctions-*relief* headline would score negative today, backwards from this plan's
    signal map.
  - **Offline/training**: `cmd/newsscore -method=model` scores historical
    `news_history.jsonl` with `news_classifier.json` → `model.LoadFinanalysNewsHistory` →
    `AggregateDailySentiment` (`internal/model/newsimport.go`) → `news_sentiment`/
    `news_count` feature slots, consumed by `cmd/exportdataset -news-history`.
  - Per AGENTS.md (2026-09-17 news-aware retrain entry), the deployed `ensemble_model.json`
    was trained on classifier-scored history, but live inference still uses the keyword
    lexicon — a known live/train mismatch this plan resolves as a prerequisite.
- The only topic-like structure that exists is `event_sanctions`, one of 8 binary regex
  flags in `internal/features/events.go` (`DetectEvents`). Per AGENTS.md, these 8
  `event_*` features are currently *excluded* from the deployed 18-feature model: with
  `colsample_bytree`/`feature_fraction` 0.8 they dilute the sampled columns and degrade the
  walk-forward result (18 columns = +110741₽; 26 columns = +80716₽; colsample 1.0 =
  +87823₽). No `event_negotiations` exists.
- Feature vector order is defined once in `internal/model/features.go`
  (`defaultFeatureOrder`/`ToVector`) and must stay mirrored in `cmd/exportdataset/main.go`,
  `scripts/export_ensemble.py` (`FEATURES`), and `scripts/compare_models.py` (`FEATURES`) —
  update all four together (AGENTS.md feature-pipeline rule).
- **Concurrent-session note**: this plan and the currently-running
  `docs/plans/20260917-strategy-validation-implementation-plan.md` ralphex execution both
  touch `internal/features/builder.go`, `internal/model/features.go`, and
  `cmd/exportdataset`. Per AGENTS.md's no-concurrent-edit rule, **do not start ralphex on
  this plan while that run is still active** — confirm it has finished (check
  `.ralphex/progress/progress-20260917-strategy-validation-implementation-plan.txt` and
  `git log`) and the tree is clean first.
- `docs/metrics.md` (created by the other plan's Task 17, if it has run first) is the
  shared single source of truth for backtest numbers — append to it rather than creating a
  competing file.

## Development Approach

- Testing approach: regular (code first, then tests).
- Complete each task fully, including its tests, before moving to the next.
- **Evidence gate (from Overview)**: the new features are only kept in the deployed
  ensemble if Task 6's with-vs-without comparison shows a measurable improvement in
  validation AUC and/or walk-forward realized P&L — matching the discipline already
  documented in AGENTS.md for the existing 8 event features. Do not skip this comparison.
- Update this plan if implementation deviates from the original scope.

## Testing Strategy

- Unit tests required for every task: new regex detectors, new aggregation code, new
  classifier-backed live scoring, and the feature-order consistency check.
- Run the project's full test suite after each task before proceeding.
- The retrain/comparison step (Task 6) is validated by its AUC/backtest numbers, not by a
  new unit test suite — but any new Go comparison tooling written for it still needs tests
  for its computation logic.

## Progress Tracking

- Mark completed items with `[x]` immediately when done.
- Record the Task 6 with-vs-without comparison numbers in `docs/metrics.md`.
- Update this plan if implementation deviates from the original scope.

## Technical Details

- **Negotiations topic detection**: new regex in `internal/features/events.go`, alongside
  the existing `event_sanctions` pattern (`санкц\w*|ограничен\w*\s*торг|блокиров\w*`).
  Candidate terms: `переговор\w*`, `уиткофф`, `witkoff`, `мирн\w*\s*(план|соглашен\w*)`,
  `прекращен\w*\s*огня`, `урегулирован\w*`. This is topic detection only (is the headline
  about negotiations at all) — it does not need to encode direction itself.
- **Direction comes from the classifier, not from keyword sign**: for any headline that
  matches the negotiations regex or the sanctions regex, take `news_classifier.json`'s own
  polarity score for that headline (already sigmoid-mapped to `[-1,1]`) instead of a
  hardcoded lexicon sign. This requires live inference to actually load and call the
  classifier (`model.NewsClassifier.Scorer()`), which it does not do today — wiring that in
  is Task 2, and it incidentally fixes the "`санкц` always negative" bug as a side effect,
  since the classifier's learned coefficients replace the fixed lexicon sign entirely for
  these two new features.
- **New feature fields**: `negotiations_signal`, `sanctions_signal` — daily mean of the
  classifier score over headlines matching each topic's regex that day, `0` when no
  matching headline exists. Append both as new names to the feature order (do not disturb
  the existing 26-name order or the existing binary `event_sanctions` slot).
- **Historical/offline aggregation**: mirror `AggregateDailySentiment`
  (`internal/model/newsimport.go`) with a topic-regex filter, so `cmd/exportdataset
  -news-history` can backfill both new signals from `news_history.jsonl`.
- **Sparsity caveat carried over from AGENTS.md**: the existing 8 event features are
  sparse (541 of 15187 rows non-zero in the last check) and still underperformed the
  18-feature set even with partial real event data. Expect `negotiations_signal` to be
  similarly sparse until more Ukraine-negotiation headline volume accumulates in
  `news_history.jsonl` — this is exactly why Task 6's evidence gate exists.

## Implementation Steps

### Task 1: Negotiations topic detector

- [x] add a negotiations regex to `internal/features/events.go` (see Technical Details
      for candidate terms) and wire it into `DetectEvents` alongside the existing
      `event_sanctions` detector
- [x] write tests for the new regex: positive matches (negotiation-progress headlines,
      "Уиткофф"-style mentions), negative matches (unrelated headlines), and boundary cases
      (partial word matches that should NOT trigger)
- [x] run project tests - must pass before next task

### Task 2: Wire live inference to the ML news classifier

- [x] load `news_classifier.json` at `cmd/trader` startup (mirroring how the ensemble
      model is loaded) and inject it into `internal/features.Builder`
- [x] replace the hardcoded keyword-lexicon scoring path
      (`aggregateNewsSentiment`/`articlePolarity` in `builder.go`) with
      `model.NewsClassifier.Scorer()` for computing per-headline polarity
- [x] decide and document the fallback behavior if the classifier file is missing/fails
      to load (e.g. fall back to the keyword lexicon vs. fail startup) — do not silently
      produce zero-value sentiment without logging
- [x] write tests for classifier-backed live sentiment scoring, including the fallback
      path
- [x] run project tests - must pass before next task

Fallback decision: if `news.classifier_path` is empty or `news_classifier.json` is
missing/fails to load, the trader falls back to the existing keyword-lexicon scoring and
logs a prominent warning; it does not fail startup. This keeps the live loop running with
the pre-existing behavior while making the degraded path visible. The classifier is scored
on each headline's `Title` only (matching `cmd/newsscore -method=model` and training),
while the lexicon fallback keeps scoring `Title + Description`.

### Task 3: Signed negotiations/sanctions signal computation

- [x] add `NegotiationsSignal`, `SanctionsSignal` fields to `domain.FeatureContext`
- [x] compute both live (in `internal/features/builder.go`, using Task 1's regex plus
      Task 2's classifier scoring) as the daily mean classifier score over topic-matching
      headlines, `0` if none match that day
- [x] add the historical/offline equivalent in `internal/model/newsimport.go` (mirroring
      `AggregateDailySentiment` but topic-filtered) so `cmd/exportdataset -news-history` can
      backfill both signals
- [x] write tests for both the live and historical aggregation paths (including the
      no-matching-headline zero case)
- [x] run project tests - must pass before next task

### Task 4: Thread new features through the feature vector order

- [x] append `negotiations_signal`, `sanctions_signal` to `defaultFeatureOrder`/
      `ToVector` in `internal/model/features.go`
- [x] update the CSV header/row sizing in `cmd/exportdataset/main.go`
- [x] update `FEATURES` in `scripts/export_ensemble.py` and `scripts/compare_models.py`
- [x] write/update the feature-order consistency test (`checkFeatureOrder` or equivalent)
      to cover the two new names
- [x] run project tests - must pass before next task

### Task 5: Backfill dataset with the new signals

- [x] re-run `cmd/exportdataset -news-history <path>` against the existing
      `news_history.jsonl` archive to produce a dataset CSV that includes
      `negotiations_signal`/`sanctions_signal`
- [x] spot-check the output: confirm non-zero values appear on known
      negotiation/sanctions headline dates and zero elsewhere
- [x] write tests covering the new columns' presence/shape in the exported CSV
- [x] run project tests - must pass before next task

### Task 6: Retrain and evidence-gate comparison

- [x] train two ensemble variants via `scripts/export_ensemble.py`: the current
      18-feature baseline, and baseline + `negotiations_signal` + `sanctions_signal`
- [x] compare validation AUC between the two variants
- [x] run the same 6-quarter walk-forward backtest (18 tickers, realistic
      spread+slippage, realized P&L not MTM) for both variants and compare
- [x] record both comparisons in `docs/metrics.md`
- [x] write tests for any new Go comparison/reporting code added for this task — no new
      comparison/reporting code; the backtest topic-signal injection added so the
      20-feature variant sees real values is covered by
      `TestLoadNewsOverridesTopicSignals`/`...NoMatches`
- [x] run project tests - must pass before next task

### Task 7: Deploy decision and documentation

- [ ] if Task 6 shows a measurable improvement: replace the deployed
      `ensemble_model.json` with the new variant and update `config.sandbox.yaml`/
      `config.example.yaml` references if the feature set name changed
- [ ] if Task 6 does NOT show improvement: leave `ensemble_model.json` unchanged and
      document the negative result (matching the existing "Rejected after walk-forward"
      style already used in AGENTS.md) so this isn't re-tried blindly later
- [ ] add a dated AGENTS.md entry under "Model quality / current state" summarizing the
      outcome either way
- [ ] write tests if any config-loading code changed as part of the deploy decision
- [ ] run project tests - must pass before next task

### Task 8: Verify acceptance criteria

- [ ] verify all requirements from Overview are implemented
- [ ] verify the evidence gate was actually applied (Task 6 ran and Task 7's decision
      matches its result)
- [ ] run full project test suite
- [ ] run project linter - all issues must be fixed

## Post-Completion

*Items requiring manual intervention or external systems — no checkboxes, informational
only.*

**Manual verification:**

- Qualitative spot-check: pull a handful of real Witkoff/negotiation headlines from the
  live news feed during an active news cycle and confirm `negotiations_signal` moves in
  the expected direction (progress → positive, breakdown → negative).
- If Task 7 deploys a new ensemble, monitor the first few days of sandbox live decisions
  for any regression versus the pre-change baseline before trusting it unattended.

**Data availability caveat:**

- If `news_history.jsonl` has too few negotiation/sanctions headlines for Task 6 to reach
  a confident comparison, backfilling more historical Telegram news
  (`cmd/newsfetch -retro-from`) is a prerequisite outside this plan's scope — flag this to
  the user rather than shipping a comparison on too few rows.
