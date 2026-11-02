# The 46 strategies on crypto markets, with crypto costs (2026-11-02)

Status: PLANNED and PARTLY RUN (registered 2026-11-02, before any crypto data was downloaded). S0 and S1 are done and the W1 battery has been run once on the frozen panel: **0 candidates, 2 leads** (order flow t 2.89, one-month momentum t 2.18, both below the registered |t| >= 3.3), three families closed negative, numbers in `docs/metrics/s10-crypto.md` 10.1-10.6. English only. The owner's request, 2026-11-02: "check all 46 strategies on crypto markets, but with the costs of those markets accounted for". Nothing here touches the live path: no order, no key with withdrawal rights, no change to `config.sandbox.yaml`, `ensemble_model.json` or `./trader`. The live programme stays MOEX; a crypto line is research until a candidate passes every gate in section 7.

Reviewer: none yet. No row may be called confirmed without a completed review pass (rule 10 of `20261029-closing-the-limitations.md`).

## 1. What "the 46" means

The programme registers one plan file per research line; the tree holds 46 of them (`docs/plans/*.md`, one per line, excluding this file and `README.md`). "The 46 strategies" is read here as those 46 registered lines, each mapped to one tradable crypto analogue, or marked not transferable with the reason. The count is a count of registered lines, not of independent bets: several lines are the same bet reshaped (momentum appears in C35, C37, C42; order flow in C11, C12, C18), which is why section 8 treats the 46 as ~12 families for multiplicity.

## 2. Why this is not a rerun

A strategy that is null on MOEX does not become positive because the venue changed. What changes is the cost structure and the payoff geography, and both move in the same direction for some families and against others:

| Fact | MOEX (as the programme models it) | Crypto (crypto perp as the reference tradable) |
|---|---|---|
| Commission | 0.05% a side, exchange fee on turnover | 0.02% maker / 0.05% taker a side at the retail tier; maker can be zero at high volume |
| Spread paid | 0.05% a side assumed flat | measured from the book: majors ~1 bp, top-20 ~2-5 bp, 21-100 ~5-20 bp, below that 20 bp+ |
| Borrow for shorts | 17% (base) to 29% a year; names differ, some unavailable | perp short needs no borrow; spot short does, 5-30% a year and often unavailable |
| Carry on a held position | none beyond borrow | funding every 1-8 h, sign varies, ~+9-11% a year median, stress to -22% a year |
| Sessions | 10:00-18:45 with an overnight gap | 24/7, no daily gap, weekly/CME gaps only |
| Survivorship | delisting is rare, point-in-time universe exists | delistings, token deaths, exchange failures are routine |
| Capacity | small book, high impact | deeper in majors, thin in alts, exchange risk priced nowhere |

Two consequences drive the whole plan. First, **crypto is cheaper per round trip than the MOEX model for majors** (about 12-14 bp all-in taker round trip against 30 bp charged on MOEX), so a signal that was killed only by turnover deserves one honest re-read. Second, **funding is not a fee and cannot be dodged by lowering turnover**: at ~11% a year it costs about 30 bp per 10 days and 120 bp per 40 days of notional, which is 3-10x the entire fee budget of a multi-week book. For the programme's usual 10-40 day horizons, funding, not commission, decides the verdict. A dollar-neutral book nets funding to approximately zero and is therefore the cheap structure; a directional book pays it.

## 3. Cost model (frozen before any result)

Reference venue: Binance, retail tier, checked 2026-11-02. Bybit and OKX are within about 10% of these numbers; the plan scores every book on at least two venues and reports the worse one as the verdict.

| Cost | Spot | USDⓈ-M perp | Source / rule |
|---|---|---|---|
| Maker fee | 0.10% | 0.02% | venue fee page, retail tier |
| Taker fee | 0.10% | 0.05% | same |
| Discount variant | 0.075% with BNB | 0.045% with BNB | reported as a variant, never as the base |
| Spread | from the book, both sides crossed | from the book | measured per pair per hour in stage S0; no assumed constant |
| Slippage / impact | participation-based | participation-based | charge 0.5x half-spread for a limit fill, 1.0x plus impact for a market fill |
| Funding | n/a | settled every 1-8 h per contract | realised funding history per venue, charged to the held side |
| Spot borrow (short) | 5-30% a year, availability varies | n/a | measured from the lending API, never assumed |
| Off-ramp | 0.1-0.5% fiat ramp, fixed withdrawal fees | n/a | charged once per capital cycle, not per trade |
| Exchange failure | not modelled as a fee | haircut parameter, base 0 | sensitivity at 1% and 5% a year of notional |

Mandatory rules:

1. **Retail tier only.** Volume discounts are a function of the book's success and are never assumed. VIP/BNB variants are printed beside the base and are descriptive.
2. **Two executions per book.** Every row is scored twice: (a) all-taker, (b) maker entry with a fill model (queue position and adverse selection charged as a probability of fill at the touch). A row passes only if the taker read passes; the maker read cannot rescue it.
3. **Funding is attributed, not netted silently.** Every book prints the funding paid and received separately, and the funding-neutral form of the same signal.
4. **Break-even first.** Each row prints acceptable cost per side = gross edge / turnover, next to the cost charged, before any net is interpreted. A row whose break-even is below the charged cost is closed at stage S1 without a backtest.
5. **No cost is updated after a result is seen.** A venue fee change mid-programme is a dated amendment, applied forward only.

## 4. Stage S1 kills most rows arithmetically

Turnover t (fraction of notional traded per side per rebalance) at all-in cost c per side needs a gross edge of 2·t·c. With c ≈ 7 bp per side for majors taker and 30 bp+ for mid-cap alts, a daily-turnover cross-section needs roughly 14 bp a day of gross edge in majors and 60 bp+ in alts. The programme's measured daily gross edges sat at 5-10 bp; that arithmetic, not another backtest, is why the gap-reversal family failed on MOEX. The same arithmetic is computed for every one of the 46 rows in stage S1 and printed before stage S2. A row is closed as "not transferable at these costs" when its required gross edge exceeds twice the gross edge the same signal family produced on MOEX, adjusted for the horizon.

## 5. Data and infrastructure (S0)

- **Universe, point-in-time.** Every pair listed on the reference venue with its listing and delisting date; a name enters only if it was listed and liquid at the decision date. Delisted names stay in the panel (survivorship is the single most common way a crypto backtest lies).
- **Venues.** At least two, so a result is not a venue artefact: Binance and Bybit (or OKX) public REST/WS. Public data only; no keys in stage S0-S3.
- **Series.** 1-minute klines plus aggregated trades, funding history, mark and index price, open interest, book snapshots at a fixed cadence (only if a row needs them), spot lending rates, token unlock and listing calendars, news and social timestamps.
- **Where it runs.** Download and panel construction on ai-box, not on the Mac; storage budgeted before the first fetch; a daily incremental update job, mirroring the `cmd/tradesrecord` pattern.
- **Code.** Go only: one tool for the panel and the books (working name `cmd/cryptofit`) and one shared cost package (`internal/cryptocost`) so every row is charged by the same code. New `cmd/` and `internal/` packages require the `docs/ARCHITECTURE.md` and `.ru.md` rows in the same commit (architecture law).
- **Frozen inputs.** Panel files and cost parameters are hashed in the plan or an errata before the first verdict run, exactly as the AlgoPack plans do.

## 6. The 46 rows

Columns: **row** = the registered line; **MOEX verdict** as recorded in its `Status:` line; **crypto analogue**; **wave** (W1 = first 60 days, W2 = second wave, W3 = only after a W1/W2 hit, Drop = no transfer).

| # | Registered line | MOEX verdict | Crypto analogue | Wave |
|---|---|---|---|---|
| C01 | `20260927-moex-pulse` | all tasks unchecked | realtime edge to serving latency; infrastructure, not a strategy | Drop |
| C02 | `20261010-news-exit-tuning-and-buy-side` | planned, blocked by data | exit rules for shock shorts (take-profit, stop, time exit) | W3 |
| C03 | `20261015-hypotheses-from-llm-review` | registry H1-H22, part null | batch: intraday seasonality, vol regimes, calendar | W3 |
| C04 | `20261018-data-depth-and-open-problems` | depth/news/calibration null, H23 rejected | history length, calibration, explainability on crypto panels | W3 |
| C05 | `20261018-gate-failure-remediation` | protocol accepted, nothing run live | acceptance protocol; adopted as section 7 of this plan | Drop |
| C06 | `20261018-seven-hypotheses` | steps 1-21 registered | regime mask, cross-sectional ranking, news decay | W3 |
| C07 | `20261019-core-rerun` | draft, rerun on refreshed data | freeze-and-rerun discipline; reused, not re-tested | Drop |
| C08 | `20261019-placebo-check` | plan, criteria fixed | within-date permutation placebo for every passing row | W2 |
| C09 | `20261019-research-to-live-gap-hypotheses` | planned H-R2..H-R9 | quoted vs executed cost, latency, fill rate | W1 |
| C10 | `20261020-structural-stability-candidates` | draft | state-dependent sizing, ablation audit, first-digit check | W3 |
| C11 | `20261022-algopack-orderflow` | A2 passed, forward to 2027-01 | book imbalance / trade flow on L2 or aggTrades | W1 |
| C12 | `20261023-algopack-100-hypotheses` | 0 of 28 Tier 1 | a subset of the flow hypotheses that have a crypto column | W3 |
| C13 | `20261023-algopack-alphafit` | DONE, new model class | composite of flow features, partial-adjustment book, cost-aware entry | W1 |
| C14 | `20261023-algopack-forecast-improvement` | registered | does extra data improve a forecast, not a book | W3 |
| C15 | `20261023-algopack-futures-fx` | 101 rows, two books | perp-spot basis, funding curve, cross-venue basis | W2 |
| C16 | `20261023-algopack-round2` | registered | market component and market-level targets (BTC as the market) | W2 |
| C17 | `20261023-algopack-round3` | registered, economic gate | economic gate on the liquid universe, break-even per side | W1 |
| C18 | `20261023-algopack-third-untouched` | DONE, 0 of 42 | flow to a downstream outcome; thin names | W3 |
| C19 | `20261024-algopack-library-v2` | 0 of 100 | transforms of the vital few, increment over the composite | W3 |
| C20 | `20261024-algopack-meta-variables` | one lead | exogenous state conditioning: funding, open interest, stablecoin supply | W3 |
| C21 | `20261024-algopack-meta-wave2` | no new hit | the vital few of the above | W3 |
| C22 | `20261024-algopack-wave3-costs-horizon` | primary gate failed narrowly | realistic costs, horizons 1/5/10 days, next-open and mark-price entry | W1 |
| C23 | `20261024-algopack-wave3b-model-ideas` | no improvement | five model ideas from local LLMs | W3 |
| C24 | `20261025-algopack-deflation-neutrality` | not distinguishable after deflation | deflated Sharpe, neutrality to BTC and to sector baskets | W1 |
| C25 | `20261025-algopack-dividend-runup` | lead on the economic gate | front-running announced flows: token unlocks, vesting cliffs, listing and airdrop dates | W2 |
| C26 | `20261025-algopack-execution-timing` | avoiding the open is robust | cheapest hour and weekday to trade; spread and depth by hour; funding timestamps | W1 |
| C27 | `20261025-algopack-library-v3-moderators` | 2 nominal hits | stock-level moderators replaced by token-level: age, float, holder concentration | W3 |
| C28 | `20261025-algopack-news-impulse` | decaying reversal lead | attention shocks and their fade; social and listing news | W2 |
| C29 | `20261025-algopack-pairs-arbitrage` | reversion real, not tradable | BTC/ETH ratio, spot/perp, staked/underlying, cross-venue pairs | W2 |
| C30 | `20261025-algopack-thin-universe` | signal stronger, gates fail | ranks 21-100: stronger edges against wider spreads | W2 |
| C31 | `20261025-algopack-timesfm3-wave2` | 0 of 11 | foundation-model covariance, placebo-controlled | W3 |
| C32 | `20261025-algopack-timesfm3` | 0 of 3, proxies momentum | zero-shot forecasts as a feature, momentum control | W3 |
| C33 | `20261025-algopack-wave4-world-data` | 0 of 99 | the crypto version of world data: hash rate, on-chain flows, stablecoin supply, exchange reserves | W2 |
| C34 | `20261026-algopack-dividend-announcement` | rule v2 passes | point-in-time event calendars: unlocks, mainnet dates, delistings | W2 |
| C35 | `20261026-algopack-momentum-confirmation` | registered, confirmed on MOEX | 12-month momentum, the best-documented crypto factor | W1 |
| C36 | `20261026-algopack-news-llm-sentiment` | one hit blocked by guard | LLM-scored sentiment as a cross-sectional feature | W3 |
| C37 | `20261026-algopack-slow-momentum` | registered, no gate passed | low-turnover books: the structure that funding and taker fees reward | W1 |
| C38 | `20261026-calm-before-move` | 0 of 6 | quiet/low-volatility predicts direction | W3 |
| C39 | `20261027-algopack-audit-gaps` | diagnostics | noise, stale signal, total loss (exchange failure, token death), bound checks | W1 |
| C40 | `20261027-algopack-fund-playbook` | 0 candidates, 1 lead | fund-style strategy descriptions re-expressed on crypto | W3 |
| C41 | `20261027-algopack-gap-reversal-sleeve` | gate fails on costs | there is no daily gap in 24/7 markets; the analogue is a liquidation-cascade reversal | W2 |
| C42 | `20261027-algopack-lowbeta-momentum` | not replicated | momentum inside low-BTC-beta tokens | W2 |
| C43 | `20261027-algopack-owner-100-ind-batch` | 1 lead of 20 | dependence splits of the crypto results | W3 |
| C44 | `20261027-algopack-owner-100-triage` | triage only, no run | triage of the same 100 on crypto; no data work until W1 closes | Drop |
| C45 | `20261027-algopack-signal-speed-table` | descriptive | how fast the crypto cross-section is, by era, cycle and venue | W2 |
| C46 | `20261029-closing-the-limitations` | E1 null, limitations 1-2 closed | the same limitations apply, and E1 (value beyond 12-month momentum) is mandatory here too | W1 |

W1 is 11 rows (C09, C11, C13, C17, C22, C24, C26, C35, C37, C39, C46); W2 is 13; W3 is 18; Drop is 4. A W3 row is opened only if a W1 or W2 row passes its gates, and then only as a family member of the passing row.

## 7. Gates (frozen)

A row is a **candidate** (forward log only, never a result) if all of the following hold on data the row has never seen:

1. **Economic.** Net of the cost model in section 3 at the retail taker tier, in a dollar-neutral form and in a directional form, with positive net in both halves of the out-of-sample window.
2. **Statistical.** Newey-West t of at least **3.3** on the net, which is Bonferroni over 46 at the 5% level two-sided, before any family reduction; family-level M_eff may only raise the bar, never lower it.
3. **Multiplicity.** Deflated Sharpe with the number of trials actually run for that family (seeds, horizons, venues, mappings all count), reported next to the raw Sharpe.
4. **Robustness.** The sign survives (a) both venues, (b) the maker and taker executions, (c) two mappings out of the pre-registered family (equal weight, inverse volatility, quintile, top-k, long-only, long-short), (d) the fee-tier and funding stress cases (fees +50%, funding at the 30-day high and at the stress floor).
5. **Placebo.** Within-date permutation of the signal through the identical book fails to reproduce at least 80% of the net, five permutations.
6. **Execution.** Break-even cost per side is at least twice the charged cost, and the fill model on maker entries retains at least half of the taker net.
7. **Forward.** The frozen rule, run live-in-paper on one venue for at least 8 weeks with no parameter change and no re-fit, keeps the sign and a point estimate inside the backtest interval.

Gates 1-4 need a completed Reviewer pass; gate 7 needs the forward record. A failure at gate 1 in the directional form but a pass in the neutral form is recorded as "neutral form only", never as a pass.

## 8. Multiplicity and honesty

The 46 rows are ~12 families: momentum and its variants, low-turnover books, order flow, cross-sectional flow composites, news and attention, event calendars (unlocks, dividends analogies), pairs and basis, market/regime conditioning, thin names, model-class additions (foundation models, LLM features), execution and cost, and methodology rows that are not bets. Within a family, rows share data and signals, so 46 registered rows are worth far fewer independent tests. The plan therefore pre-registers that the expected number of false candidates at the 5% level is 5% of the independent families that reach stage S3, and that no family is declared to have an edge on one row alone.

The prior, stated before any run: the MOEX programme's honest conclusion is that the deployed ensemble has no demonstrable edge and that its leads do not survive costs; crypto's settled facts (momentum, funding carry, basis) are known to everyone and are the market's most crowded trades. The realistic outcome of this plan is 1-3 candidates from 46, most probably from C35/C37 (momentum and low turnover) and C25/C34 (announced-flow events), with the rest closed arithmetically at S1. A plan that returns zero candidates and a printed break-even table for all 46 is a success, not a failure: it would say the crypto venue does not rescue the programme's nulls.

## 9. Kill criteria for the whole plan

The crypto line is stopped, with what has been measured written into the metrics files, if any of these happens:

1. Stage S0 cannot deliver a point-in-time universe with delisted names and realised funding at two venues (the data is not free and the budget is not approved).
2. Fewer than 3 of the 11 W1 rows survive S1 (nothing to test).
3. Every W1 row fails gate 1 at the taker tier and at the family's best break-even (the venue's costs exceed the whole signal family).
4. Any result requires a live key with withdrawal rights to be reproduced.

## 10. Deliverables and timeline

| Stage | Output | Time |
|---|---|---|
| S0 | crypto panel, funding history, cost measurements by pair and hour, hashed inputs | 3 weeks |
| S1 | break-even table for all 46 rows, closure notes for the closed ones | 1 week |
| S2-S3 | W1 books, verdict rows, placebo and deflation readings | 6 weeks |
| S4 | fill model and executed-cost re-read of the survivors | 3 weeks |
| S5 | paper forward record, 8 weeks minimum, one venue | 8 weeks |
| W2 | opened only after W1 closes | 8 weeks |

Numbers go into a new `docs/metrics/s10-crypto.md` with an index row in `docs/metrics.md`, in the same commit as the run (rule 8). Code goes into `cmd/cryptofit` and `internal/cryptocost` with their `docs/ARCHITECTURE.md` and `.ru.md` rows in the same commit. Data and outputs live in `artifacts/crypto/`, never in git.

## 11. Errata (dated, applied forward)

- **2026-11-02, errata 1, universe filter, written before any row is scored.** The exchange lists tokenized equities, commodities, indices and pre-market contracts next to crypto contracts (234 of the live USDⓈ-M contracts on 2026-11-02 are not `underlyingType = COIN`). These are excluded: the panel is USDⓈ-M perpetuals with `underlyingType = COIN` quoted in USDT. Rationale: a tokenized NVDA or SOXL contract reproduces an equity factor this programme already knows, and mixing it into a cross-section would attribute equity beta to a crypto signal. Exclusion is by the venue's own classification, applied to every row, never pair-by-pair.
- **2026-11-02, errata 2, contract life.** Source of the universe is the bucket listing of monthly kline files, so a contract that ever traded enters the panel from its first month and leaves at its last; `exchangeInfo` gives the live set and its classification, not the history. Renamed contracts (a delisted symbol re-listed under a new name) are treated as separate names; the panel therefore overstates the number of independent assets and never understates it.
- **2026-11-02, errata 3, funding bookkeeping.** Funding is charged as the sum of settlements falling inside the holding window, per contract, from the venue's own funding history, and is reported separately for paid and received; a book whose two legs carry different funding rates is not funding-neutral and the difference is printed, not hidden.

- **2026-11-02, errata 4, frozen evaluation window and disclosure of a prior look.** The W1 evaluation is fixed before the full panel exists: window 2024-01-01..2026-10-31, universe = the top 100 contracts by trailing 20-day median quote volume, costs at the retail taker tier with per-name liquidity tiers, partial-adjustment rates 0.10 and 0.33 negotiated per side per day, IC label horizon 5 days, 10 cells (five signals x two rates): 12-month momentum (lookback 365), one-month momentum (30), one-day reversal, taker-buy share (20-day), and funding carry (7-day). A diagnostic run happened on a partially downloaded panel (207 contracts, the top-volume subset, same window) before this freeze, and its numbers are disclosed here so the freeze is not a fresh start: one-month momentum net +21.3 and +26.0 bps a day at rates 0.10 and 0.33 with Newey-West t +2.18 and +2.13, deflated Sharpe 0.688, both halves negative in 12-month momentum and strongly positive in one-month; one-day reversal net -8.2 and -18.1 bps; taker flow net +1.6 and -10.3; funding carry net -8.7 and -12.0; the Bonferroni bar for 10 cells is |t| >= 2.81, which no cell reached. The full-panel run repeats the identical grid once and its numbers are the record.

## 12. What this plan does not claimIt does not claim crypto is easier; the same null signals are being re-read, not replaced. It does not compare venues as investments, does not model custody or counterparty risk beyond the haircut parameter, and does not test market making (which is where the cost structure points for high-turnover signals). It inherits every limitation 1-10 of `20261029-closing-the-limitations.md`, adds two of its own (fee tiers and funding regimes change within a year; a second exchange is a proxy for independence, not independence), and its first executor is one author with one Reviewer, exactly like the MOEX programme.
