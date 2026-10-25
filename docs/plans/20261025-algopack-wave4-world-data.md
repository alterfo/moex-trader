# AlgoPack wave 4: the world outside the exchange (weather, commodities, news tone, a resource-equilibrium index) and the forward-only registry (2026-10-25)

Status: DONE 2026-10-25 (English only; scored once, 0 of 99), registered before any wave-4 series is aligned with the composite. Design from the owner's request to include the "crazy" hypotheses (abandoning the backtest, weather forecasts, a mathematical model of resources and market equilibrium, more world data, government procurement) and from the Reviewer's critique of 2026-10-25. Continues `docs/plans/20261024-algopack-meta-variables.md` and `docs/plans/20261024-algopack-meta-wave2.md`; the machinery is `cmd/metascan` (causal binary states at d-1, circular-shift permutation, Nyholt threshold over the enlarged family, the six controls).

## What is a hypothesis here and what is calibration (stated plainly)

Roughly seventy of the ninety-seven rows are series with no mechanism for the MOEX cross-section (ten cities of temperature, twelve commodity-direction states, four news series) run through the same machinery as extended calibration rows: they enlarge the family and test the machinery on data of the world's own structure, and a hit among them is far more likely to be an artefact than a discovery. The real hypotheses, each with a mechanism, are three: (1) commodity moves reach MOEX equities through stock-level exposure (the ten commodity interaction rows), (2) heating demand reaches the energy and utilities names (the heating-degree anomaly rows and its one frozen-list interaction), (3) the sanctions-news tone conditions the composite (the news-tone state). Everything else is labelled calibration.

## Series and states (all binary, all causal, no tuned parameters)

Every state at feature date d uses data dated d-1 or earlier. Series come from public free sources: Open-Meteo archive (weather), Yahoo chart API (futures and one ETF), GDELT doc API (news tone and volume), fetched by `cmd/metascan -mode fetch4` into `data/macro-long`.

- **Alignment rules (Reviewer's pin).** A US series is indexed by its own trading date; a US bar dated D settles after the MOEX decision of D (the book is formed after 18:30 Moscow, the futures settle at about 21:00 UTC), so the state at MOEX date d uses only US bars dated at most d-1. A GDELT timeline value dated D covers the whole day D, so the state at d uses values dated at most d-1. Weather of day D is observed on D, so the state at d uses days up to d-1.
- **W01-W10 (calibration)**: the mean temperature of the last three days (d-3..d-1) minus the mean of the same calendar days of the previous five years is above zero, for Moscow, Saint Petersburg, Novosibirsk, Yekaterinburg, Kazan, Nizhny Novgorod, Samara, Krasnoyarsk, Rostov-on-Don, Khabarovsk. They are nearly one factor; the block M_eff and eigenvalues are printed.
- **W11 (mechanism: heating demand)**: the 7-day sum of Moscow heating degrees (max(18 - T, 0)) minus the median of the same window of the previous five years is above zero. **W12 (calibration)**: Rostov-on-Don 7-day precipitation sum above the median of the same window of the previous five years.
- **K01-K12 (calibration for T, S, R; mechanism through Q)**: the 10-observation return of natural gas (NG), TTF gas, copper, aluminium, steel HRC, silver, platinum, palladium, wheat, corn, soybeans, uranium ETF (URA) is above zero.
- **N01-N04**: GDELT 7-day mean tone of "russia sanctions" (**N01, the mechanism row**), "russia oil gas energy" (N02), "russia economy" (N03) below its trailing 250-observation median (the pessimistic state), and the daily article volume of "russia" (N04) above its trailing median.
- **E01 (the resource-equilibrium index)**: the sum over Brent, NG, TTF, copper, aluminium, HRC, platinum, palladium and wheat of each commodity's 10-observation log return divided by its trailing 250-observation standard deviation (at least seven commodities present) is positive. It is a function of the K states and the correlation matrix charges it as nearly the same hypothesis. This is the "mathematical model of resources and market equilibrium" in its testable, parameter-free form: a terms-of-trade impulse for the resource-exporting index, with no fitted weights.

Each of the 29 states gets R (composite daily IC by state), S (partial-adjustment net by state) and T (universe 10-day return by state) rows, 87 rows. Ten interaction rows **Q01-Q10** (the wave-1 I form: the rolling 60-day beta of the stock's daily return to the commodity series against the standardised lagged 10-day commodity change): NG, TTF, copper, aluminium, HRC, silver, platinum, palladium, wheat, corn. Two interaction rows with frozen membership lists and a weather z (no other weather interactions are registered; a sector-list grid over the twelve weather states would be a hidden grid): **Q11** energy and utilities list (IRAO, HYDR, MSNG, UPRO, FEES, OGKB, TGKA, TGKN, MRKP, MRKC, MRKV, MRKU, MRKZ, GAZP, NVTK) against the standardised heating-degree anomaly; **Q12** agri list (PHOR, RAGR, NKHP) against the standardised Rostov precipitation anomaly (three names: very low power, a calibration row). Total **99 rows** (87 + 12); the six wave-1/2 controls (Saturn, Moon, Kp, Mercury, parity, Bitcoin) run through the identical machinery in the same output.

## Statistic, family and verdict (frozen)

As in `docs/plans/20261024-algopack-meta-variables.md`: realtime dates 2025-01-03..2026-10-16, the alphafit out-of-fold series (dump check stops the run if the means do not reproduce), circular-shift permutation p (every offset at least 20 dates from the identity), halves, A2-baseline sign agreement for R and S, Nyholt M_eff from the outcome-free state and z series of all rows, threshold 0.05 / M_eff for the wave-4 rows (the wave-1 and wave-2 rows keep their registered verdicts). The Reviewer expects an M_eff of about 30-50 for the enlarged family (the ten temperatures are one factor, the twelve commodities two or three); the run prints the block M_eff and the largest eigenvalues of the weather, commodity, news and equilibrium blocks, so that the collapse of the effective count is visible. A hit needs the permutation p at most the threshold and the same sign in both halves; one control hit voids every real hit of the same use. A hit is a lead only (the window has been looked at five times).

## Government procurement (acquisition item, probe recorded)

`zakupki.gov.ru` and its open-data pages time out from the Mac and from the ai-box (curl status 000 after 15 seconds on 2026-10-25); `clearspending.ru` answers (200) but is an aggregator of unknown completeness and currency. No row is registered. A procurement test would need (a) an accessible source, (b) a map from supplier tax numbers to listed groups (a manual table of a few dozen subsidiaries), (c) a contract-award date feed that is not revised; none exists in the repo.

## Abandoning the backtest: the forward-only registry

The owner's phrase is answered by stopping the historical fishing, not by a new test: the leads that survived are frozen and will be read only on dates that do not exist yet. The registry (rule definitions in the cited plans; each is read once, never rescored on history, never extended):

| Rule | Plan | Read | Criterion (pre-registered) |
|---|---|---|---|
| A2 frozen rule | `20261022-algopack-orderflow` | about 2027-01-20 (60 forward dates from 2026-10-21) | the F2 criterion of that plan |
| Round-3 decile gate | `20261023-algopack-round3` | about 2027-02-05 | mean top-minus-bottom decile gate positive, date-block bootstrap interval above 0, sign in at least 4 of 5 seeds, S-dm control not positive |
| Composite alpha | `20261023-algopack-alphafit` | about 2027-02-05 | composite-minus-baseline IC interval above 0 (minimum detectable about +0.07); the bbo-cost book at rate 0.5 reported next to the frozen book |
| RVI regime split | `20261024-algopack-meta-variables` | about 2027-02-05 for the report, about 2027-10 for confirmation | the forward composite IC split by the F08 state, bootstrap interval of the difference; a second year of data decides |
| Deferred fo rows | `20261023-algopack-futures-fx` | about 2027-02 (120 realtime dates) | T = 3.0 per the plan; the early descriptive read of 2026-10-24 is non-binding |
| Shadow books ridge and high52 | `20261019-core-rerun` | after 12 rebalances | the plan's criteria |

The live model's preflight backtest gate is a safety check of the deployed model, not a hypothesis test, and stays.

## Power and expectations (before the run)

The expected number of hits among the 99 rows is zero. The interaction rows (Q) are the only ones with a plausible stock-level channel; the state rows use at best one fresh degree of freedom each. The run is worth doing because it closes the "you did not look at the world" objection with a falsifiable count, and because a hit would be found against a family whose effective size the run prints.

## Run order

1. Reviewer reads this file and `cmd/metascan` (`fetch4.go`, `states4.go`, the wave-4 branches of `run.go`); wave-1 and wave-2 outputs must reproduce byte for byte (done: both identical).
2. One run on the ai-box: `cmd/metascan -mode wave4 -series work/alphafit-series2.csv`.
3. Numbers into `docs/metrics.md` 8.27 and `docs/knowledge/model-quality-history.md`.

## Result (scored once, 2026-10-25)

Numbers in `docs/metrics.md` 8.27, output `artifacts/algopack-wave4/metascan-wave4.txt`. The wave-1 and wave-2 rows reproduced byte for byte. Enlarged M_eff **70.76** (the Reviewer expected 30-50; the weather block has M_eff 10.84 of 12 states, largest eigenvalue 4.29, the commodity block 11.39 of 12, largest 3.17, the news block 3.52 of 4), threshold p 0.0007. **0 hits among the 99 wave-4 rows; 0 hits among the 18 control rows.** Smallest p: N01 T 0.0239 (sanctions-news tone pessimistic: the next 10-day universe return higher by 2.3%, both halves positive), Q11 energy and utilities list against the heating-degree anomaly 0.0406 (difference -0.057), K12 T 0.062, Q10 0.076; 2 rows below 0.05 (about 5 expected) and 6 below 0.10 (about 10 expected). The resource-equilibrium index E01: R p 0.97, S p 0.21, T p 0.24. Registered reading: nothing in the weather, commodity, news-tone or equilibrium families conditions the composite, times the 10-day market, or enters through stock exposures at the family threshold; the mechanism rows (commodity interactions Q01-Q10, heating demand W11 and Q11, sanctions tone N01 R) all have p above 0.04.

## Review log

World-data critique (opencode, 2026-10-25): alignment pins for the US and GDELT series; ten cities as one factor (calibration, not hypotheses); weather interactions capped at two frozen-list rows; E01 charged by the correlation matrix; expected M_eff 30-50 with the block eigenstructure printed; about twenty states labelled calibration and three mechanisms named; the backtest answer is the registry table with dates and criteria; procurement an acquisition item. Applied. The Reviewer put the calibration share at about twenty rows; the plan above puts it nearer seventy once the T and S uses of the commodity and news states are counted, and says so.

## Errata

Written 2026-10-25 after the Reviewer's code pass, before the run.

1. Threshold accounting: the wave-4 run re-prints the wave-1 and wave-2 rows for reference only. Their registered verdicts are those of `docs/metrics.md` 8.21 and 8.22, and the calibration rule (one control hit voids the real hits of the same use) is evaluated on the registered verdicts of the earlier waves plus the wave-4 hits, not on statuses re-judged at the enlarged threshold. The same holds for the wave-2 controls.
2. When the GDELT feed has a gap the state is absent (missing), never extrapolated; the staleness guard (7 days) applies to the raw series, and the trailing median of the 7-day mean skips missing days.
