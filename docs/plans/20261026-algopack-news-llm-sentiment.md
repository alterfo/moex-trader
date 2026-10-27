# News sentiment scored by a local LLM: does a real reader beat the lexicon? (2026-10-26)

Status: DONE 2026-10-27 (run once; one nominal hit blocked by the 2025+ guard, 0 of 5 placebos). Registered 2026-10-26 before any scoring run (English only). Origin: the survey arXiv 2408.06361 (LLM agents in trading) was read at the owner's request; its only usable idea is an LLM-scored news sentiment used as a cross-sectional rank. The earlier lexicon sentiment (`20261025-algopack-news-impulse.md`, rows N03, N04, N06) was null, and its errata state that a null lexicon row is evidence about the lexicon, not about sentiment. This plan tests the sentiment itself.

## Data and scoring (frozen)

`data/news_history.jsonl`: 155,601 ticker-tagged headlines; the scoring window is 2022-01-03..2026-10-16 (about 145,000 headlines). One call per headline with the ticker, the title and the source; the model answers one of five labels (strong negative, negative, neutral, positive, strong positive) mapped to -2..+2; temperature 0, a fixed prompt stored in the output directory with its SHA-256, no price or date in the prompt (no look-ahead through the model's memory of outcomes is possible by construction only for the text; the model's own pre-2026 knowledge of events is a stated risk, tested by the 2022-2024 versus 2025-plus split and by the placebo below).

Model: the local model that fits next to the foreign GPU process (about 19 GB held by another user's job, leaving about 5 GB). A throughput pilot on 500 headlines decides the model: it must finish the whole set in under 36 hours; otherwise only the 2023-2026 headlines are scored and this is recorded. Heavy compute on the ai-box only.

Aggregation per name and day (fixed ex ante): information cut 18:30 Moscow as in the news-impulse plan; `llmS` = trust-weighted mean score of the headlines in the day bucket (missing when the bucket is empty); `llmN` = count; `llmShock` = `llmS` minus the name's mean over the previous 20 trading days with headlines.

## Rows (frozen, scored once)

| Id | Row |
|---|---|
| L1 h1, h5, h10 | `llmS` rank against the abnormal return (name minus universe mean) from the close of d, daily rank correlation over names with a bucket (at least 30), Newey-West t (lags h + 5), two halves (2022-2024, 2025-plus) |
| L2 h1, h5, h10 | `llmShock` |
| L3 | evening bucket `llmS` against the next day's open-to-close return |
| L4 | `llmS` incremental slope over the news-count shock `zc` (the known reversal) and the lexicon `sent` (per-date OLS, slope t) |
| L5 | agreement: `llmS` minus `sent` ranks (does the model add where the lexicon is blind) |
| C1-C5 | controls: `llmS` permuted across names within date, five seeds |

**Hit**: |t| of at least 3.0, same sign in both halves, no control hit (one control hit voids every hit). Distinct hypotheses: five. A hit is a lead; the confirmation is the 2022-2024 half alone (t of at least 2.0 with the same sign), since the headlines are not regenerated and the halves are independent samples. A confirmed hit goes to a sleeve test with the wave-3 chain, real shorts carry 25-29% and the bbo cost model; the news reversal is concentrated on the negative side, which is the short leg, so the carry decides.

## Honest expectations

Headline news is priced fast and the tradable horizon from a daily close is mostly consumed; the lexicon and count rows already show a decaying reversal after bursts, not a continuation. Prior of any hit: low (about 10%); the most likely finding is that a better reader moves the sentiment rows from about zero to a small, insignificant IC, which closes the news-sentiment question for daily horizons. A null is reported as a null.

## Review log

Pre-run Reviewer pass requested 2026-10-26 (see the peer-chat log); pins and amendments are added as errata before the scoring run.

## Errata

Written 2026-10-26 after the Reviewer's pre-run pass (eleven conditions), before the pilot and the scoring run.

1. **Model pin.** Name, quantisation and the Ollama digest are recorded in the output directory before the pilot; no moving tag. The pilot decides coverage only, never the model. Temperature 0, one call per headline, no retry except for an unparseable label, which is excluded from the buckets (share reported).
2. **Numeric pilot rule.** Random 500 headlines (fixed seed 20261026, drawn from the full window), the same concurrency and GPU conditions as the full run. Predicted total = measured seconds per headline x 145,000 x 1.2. Fallback to 2023-2026 if and only if the prediction exceeds 36 hours. Fallback definitions, fixed now: halves 2023-2024 / 2025-plus; confirmation = 2023-2024 t of at least 2.0 with the same sign; `llmShock` uses the trailing 20 headline-days available (fewer than 20 at the start of 2023 are allowed down to 10, otherwise missing). Re-measure if the VRAM guard fires or the foreign GPU job changes; paused time does not count toward the 36 hours; no local-LLM load while ComfyUI is mid-job.
3. **Pilot sanity gates.** Label histogram: at most 80% in one class; unparseable share at most 5%; |Spearman(llmS, lexicon sent)| of at least 0.4 on the pilot. If a gate fails the run proceeds and a null is reported as a model-quality null, not a sentiment null.
4. **Aggregation pinned.** `llmS` = sum(trust x score) / (count + 1), the same denominator as the lexicon `sent`. `llmShock` = `llmS` minus the mean of `llmS` over the previous 20 trading days with headlines. Evening bucket: `published_ts` after 18:30 of d and before 09:50 of the next trading day; L3's outcome is the raw next open-to-close `idy1`, the cross-sectional demeaning being implicit in the rank IC.
5. **Look-ahead caveats.** The day-d bucket assumes the headline was public before the close of d (`published_ts` against the 18:30 cut); the nightly fetch makes the day-d signal an upper bound, and a live deployment would lag one day. Inherited from the news-impulse convention.
6. **Memory reading corrected.** The 2022-2024 / 2025-plus split tests stability, not model memory; 2022-2024 is the half the model knows best. The memory control is the within-date permutation placebo: a name-specific memory score dies under it, and date-level memory cannot inflate a within-date rank IC. Guard: a lead proceeds to a sleeve only if the 2025-plus half also has t of at least 2.0 with the same sign; the 2022-2024 confirmation stays as written.
7. **Memory twin on the pilot.** The same 500 headlines are scored with the ticker redacted (title and source only); the label agreement is reported. If labels flip en masse, the full run carries a memory-channel caveat.
8. **Hit rule.** A multi-horizon row (L1, L2) hits on its primary horizon h1; h5 and h10 are context. A control hit is the same |t| of at least 3.0 rule on the permuted rows. Family accounting: the per-row threshold 3.0 is kept and the family is five hypotheses (about ten t-statistics are shown).
9. **Twin condition.** A hit in L1, L2 or L3 must keep the same-sign incremental slope over `zc` and `sent` at its horizon (L4's controls, extended) before it goes to a sleeve; otherwise it is the known count reversal under a new label.
10. **L5 pinned.** The daily IC (Spearman, names with a bucket) of rank(llmS) minus rank(sent) against the h1 abnormal return, mean with Newey-West t and the two halves; the mean per-date rank correlation of llmS and sent is reported next to it.
11. **Power floor.** Qualifying dates (at least 30 names with a bucket) are reported per half; a half with fewer than 100 qualifying dates reports its null as underpowered, not as evidence of absence.
12. **Run notes.** The scoring ran in three segments (the Ollama server stalled once under memory pressure and the run was resumed; one segment used a 2048-token context for about 11,000 headlines, then the default context again). Prompt, temperature 0 and seed are identical in all segments. 143,088 headlines scored, 0 unparseable.

## Result (scored once, 2026-10-27)

Numbers in `docs/metrics.md` 8.40, outputs `artifacts/algopack-news-llm/`. Pilot: throughput fine, quality gate failed (|Spearman with the lexicon| 0.265), so the null is a model-quality null. L1 h1 mean IC +0.0162 (t +2.73); L2 nothing; L3 evening: t +4.95 pooled, 2022-24 +7.25, 2025+ +0.14 (nominal hit, blocked by the 2025+ guard); L5 nothing; 0 of 5 controls hit. Not a lead.
