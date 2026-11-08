# Options on MOEX FORTS: is there a premium that survives its costs? (2026-11-08)

Status: PARTLY RUN 2026-11-08: BR scored once (four books pass the gate, a lead; numbers in `docs/metrics/s11-options.md` 11.2-11.4), the other five families are still downloading. Registered 2026-11-08 before any run (English only); Reviewer: none yet, so no result of this plan may be recorded as confirmed until a read-only review pass has happened. Owner zone: model zone (new asset class, no live path).

## Question

The programme has closed every equity direction, news, calendar, cross-section and crypto family it tested (`docs/metrics.md`, sections 3-10). Options are the one large MOEX market that has never been touched: they carry a second, mechanical source of return (the variance risk premium, IV minus subsequent realized volatility) that does not depend on predicting direction. This plan asks one question, in two steps:

1. **Descriptive:** on the families that actually trade, is option-implied volatility above the volatility that follows, and by how much per unit of premium?
2. **Economic:** do four pre-registered premium-selling books earn a positive net after a frozen cost model, and does any of them clear a pre-registered bar?

A positive answer is a lead and a frozen forward rule, never a deployment.

## Data (verified 2026-11-08, this repository)

| Source | URL | Fields | Depth |
|---|---|---|---|
| Options chain (reference) | `iss.moex.com/iss/engines/futures/markets/options/securities.json` | SECID, SHORTNAME, ASSETCODE, LASTTRADEDATE, OPTIONTYPE, STRIKE, UNDERLYINGASSET | current listing only: 39448 series, 64 asset codes |
| Option description (per series) | `iss.moex.com/iss/securities/<SEC>.json` | STRIKE, OPTIONTYPE, LASTTRADEDATE, UNDERLYINGASSET, FACEUNIT | current plus delisted |
| Options history (whole market per date) | `iss.moex.com/iss/history/engines/futures/markets/options/securities.json?date=D&start=N` | OPEN, LOW, HIGH, CLOSE, WAPRICE, SETTLEPRICE, THEOR_PRICE, VOLUME, VALUE, NUMTRADES, OPENPOSITION | 2016 verified (2013 reported); `from`/`till` are ignored on this endpoint, `date` is required |
| Futures history (underlying) | `iss.moex.com/iss/history/engines/futures/markets/forts/securities/<SEC>.json` | CLOSE, SETTLEPRICE, VOLUME | 2013 reported |

Census of one trading day per month, 2021-2022 (whole-market pages, 100 rows each): 8000-12000 option rows per date, of which only 115-304 series traded; turnover 121 mln to 2.4 bn RUB per day and **BR (Brent options) is 75-95% of it** every single sample, followed by CR and GD (crude, gold), then GZ and LK (Gazprom, Lukoil options) and Eu/ED (FX). Equity-index and single-stock options on MOEX are, on these samples, a thin market; that is a finding of the census, not an assumption.

## Universe (frozen)

Families with at least 30 traded series on at least half of the census dates: **BR, CR, GD, GZ, LK, Eu**. Underlying future and strike come from the per-series description (`UNDERLYINGASSET`, `STRIKE`, `OPTIONTYPE`, `LASTTRADEDATE`), never from parsing the SECID. Window: entry dates 2024-01-01..2026-11-07; data pulled from 2023-10-01 so that no expiry is truncated.

Option contract unit: BR options are quoted in USD per future and the BR future is 10 barrels (`LOTSIZE` 10), so premium RUB = price x 10 x USDRUB; SR futures are 100 shares quoted in RUB. Every P&L is computed from `SETTLEPRICE` in the contract's own unit and converted with the CBR daily USDRUB rate for USD-quoted families (the conversion is a second-order term for a 1-3 week trade, and the plan reports it).

## Method (frozen)

- **Entry price** is `WAPRICE` on the entry date: the trade must have happened that day, so the price exists and a trade was possible. A series without `WAPRICE > 0` on the entry date is not tradable and is skipped, never filled with the settlement or theoretical price. `SETTLEPRICE` is used only for the underlying mark and for the exit payoff, which is contractual.
- **Implied volatility** is Black-76 on the underlying future, solved from `SETTLEPRICE` (not `WAPRICE`) and the future's `SETTLEPRICE` the same day, with the actual days to expiry on the futures calendar; zero-rate is used for the discount and stated.
- **ATM strike** is the strike whose distance to the entry-day future settle is smallest among strikes that have both a call and a put with `WAPRICE > 0` on the entry date.
- **Realized volatility** for the descriptive row is the annualised standard deviation of daily log returns of the underlying future's `SETTLEPRICE` from entry to expiry (r = 1 weighting, no overlap).
- **Entry dates**: (a) DTE 5 trading days, (b) DTE 20 trading days. The nominal entry day is that many trading days before expiry; if the straddle's two legs did not both trade that day, the entry moves one trading day closer to expiry, up to three times, and the realized DTE is recorded per trade. An expiry with no such day is skipped (errata 3).
- **Exit** is the contractual payoff at expiry against the underlying future's settle price on the option's expiry date: a short straddle pays `premium - |S - K|`, a short strangle pays `premium - max(S-Kc,0) - max(Kp-S,0)`, a covered call pays `premium - max(S-K,0) + (S - S_entry_future)`.

## Strategies (frozen, four)

- **S1 short ATM straddle**, held to expiry, no hedge.
- **S2 short 25-delta strangle** (call delta 0.25, put delta -0.25 from the same surface), held to expiry.
- **S3 covered call**: long the underlying future, short the 30-delta call, entry DTE 20, closed at expiry. Two legs, so the underlying leg pays its own cost.
- **S4 delta-hedged short ATM straddle**: short the straddle, hedge the delta daily at the future's settle, hedge cost charged per side. The hedge is a diagnostic: it isolates whether any edge is variance or just directional exposure.

Each trade is one observation; the mean, the median, the standard deviation, Newey-West t with lag 5, the worst trade, the 5% CVaR, and the annualised return on capital are reported per strategy and per family, and pooled.

## Cost model (frozen)

- Option leg: exchange fee **4 RUB per contract per side** (FORTS options tariff; to be checked against the published MOEX tariff before the run and recorded in the plan's errata if it differs) plus **half-spread 1.0% of premium per side** as the base case, with **0.5%** (optimistic) and **2.0%** (pessimistic) as reported variants. The exchange fee is charged in RUB on the RUB value of the contract, so on small premium contracts it is material and is charged as such.
- Underlying future leg: **0.02% of notional + 1 tick** per side.
- Financing and margin: capital per short straddle is fixed at **15% of the underlying notional** (a conservative MOEX guarantee-collateral proxy), and the annualised return is quoted against it; the 14% key rate is the hurdle. The premium is treated as received at entry, which flatters a margined position; the sensitivity of the verdict to that convention is reported, not hidden.
- P&L is gross of the RUB/USD conversion for USD-quoted families and is converted at the entry-date CBR rate; the conversion sensitivity is reported beside the result.

## Rows

- **R1 (descriptive, no gate):** ATM IV minus realized vol, in vol points and in premium points, by family and DTE bucket (5, 20); IV term structure by DTE; 25-delta put minus 25-delta call IV (skew) by family. This is the number that decides whether the whole topic is worth the code.
- **R2 (gate):** the four strategies, base costs, per family and pooled.
- **R3 (stability):** the same four under the 0.5% and 2.0% half-spread variants and under the no-hedge/stress conventions; a strategy whose sign flips between the base and the 2.0% variant is unstable and is recorded as no finding (rule 0.6 of `docs/metrics.md`).

## Gate (frozen)

**Pass = a lead for the forward log.** All of, on the pooled R2 result of that strategy: (1) mean net per trade positive with Newey-West t of at least **2.64** (Bonferroni over four strategies); (2) positive in both halves of the window, split 2025-07-01; (3) positive net under the 2.0% half-spread variant; (4) R1's VRP on the same family is positive. A strategy failing (1) is closed at these costs. A pass freezes a forward rule before any forward date exists.

## What a null does not test

Only six families, only one to three weeks to expiry, only settlement-based marks, only the daily calendar; it does not test intraday option trading, market making, dispersion or cross-family vol arbitrage, longer-dated LEAPS-style series (thin on MOEX), or the American early-exercise premium (ignored, and it only ever favours the seller of an option that the buyer may exercise). It also does not test a broker's actual option margin, only the 15%-of-notional proxy.

## Commands (exact, local, `bin/`)

- `go run ./cmd/optfetch -mode chain`
- `go run ./cmd/optfetch -mode cbr -from 2023-06-01 -till 2026-11-08` (`data/options/usdrub.csv`)
- `go run ./cmd/optfetch -mode series -prefix BR,GD,CR,GZ,LK,Eu -from 2023-10-01 -till 2026-11-07 -workers 3`
- `go run ./cmd/optfetch -mode meta -workers 3` (futures lot size and unit, `data/options/futures-meta.jsonl`)
- `go run ./cmd/optscan -mode vrp -families BR,GD,CR,GZ,LK,Eu -dte 5,20`
- `go run ./cmd/optscan -mode battery -families BR,GD,CR,GZ,LK,Eu -dte 5,20` (costs: `-fee 4 -spread 0.01`; variants `-spread 0.005` and `-spread 0.02`)

Artifacts: `artifacts/options/` (CSV per row), numbers in `docs/metrics/s11-options.md`.

## Honest expectation

The prior is that R1 shows a large positive VRP on BR and GD weeklies (short-dated commodity options usually do) and that the four economic books lose after the 1% half-spread plus the 4-RUB fee, because the premium on a 10-40 USD contract is 200-1500 RUB and two round trips eat 3-8% of it while the tail is fat. The value of the run is the printed break-even spread: if the premium is real and large, the answer is a market-making or a wide-strike variant, not a directional book, and that is what the next plan would test.

## Result (BR, scored once 2026-11-08)

Numbers in `docs/metrics/s11-options.md` sections 11.2-11.4, artifacts `artifacts/options/`. BR 2023-05..2026-11, 219 pooled observations per book at DTE 5, 10 and 20. The variance premium is +5.8 to +7.7 volatility points and the ATM straddle's premium exceeds the realized move at every tenor. All four books pass the gate: S1 short ATM straddle +771.5 RUB per expiry (NW t 4.32, month-bootstrap [+338, +1187]), S2 25-delta strangle +822.3 (t 6.14), S3 covered call +690.8 (t 3.51), S4 delta-hedged straddle +538.0 (t 5.07); both halves of the window are positive in every cell and the sign survives a 2% and a 5% half-spread (S1 at 5% falls to t 2.39, S2 holds). The settlement-entry control without the tradability filter gives the same means, so the premium is not an artefact of the trade filter. The tail is the risk: the worst straddle lost 14916 RUB against a 771 RUB mean and the 5% CVaR is 7381 RUB against 10323 RUB of margin-proxy capital, and capacity is 9-22 contracts a leg. Lead, not deployment; the frozen forward rule is in 11.4. The yearly split is the caveat: the S1 book at DTE 5 earns +965, +706, +182 and +56 RUB per expiry in 2023, 2024, 2025 and 2026, so the pooled significance is carried by the early window and the current level is inside its own noise. GD was scored second and fails: the straddle loses -1464.4 RUB per expiry (t -1.00) and the VRP is within a point of zero (`docs/metrics/s11-options.md` 11.5), so the premium is a Brent property, not a MOEX-options property. CR shows +0.08% net on 13 expiries (inconclusive) and GZ, LK and Eu produced no usable series, so they are not tested; the data door for those three has to be the per-date whole-market pull, not generated SECIDs.

## Review log

none yet: this plan is registered but unconfirmed. A read-only review pass (opencode) must happen before any number from it is cited as evidence.

## Errata

1. The 4-RUB option fee is a placeholder until the MOEX tariff is checked; the check happens before the first scoring run and is recorded here.
2. `from`/`till` are ignored by the whole-market options history endpoint; the puller pages `date=` instead. Discovered 2026-11-08.
3. An expiry whose straddle legs did not both trade on the nominal entry day uses the latest trading day within three days closer to expiry, and the realized DTE is reported; registered before the first scoring run, because the census showed that a monthly BR series often trades on single-digit numbers of days.
4. Option SECID format is `<prefix><strike>B<code><year digit><weekly suffix>`, where the code is the month letter A..L for a call and the same letter shifted by twelve (M..X) for a put; the future is `UNDERLYINGASSET` from the description, i.e. the family's future whose last trading date is the first at or after the option expiry. The first generator emitted call-only ids and was corrected before any scoring (facts verified against `BR90BV3`, a put expiring 2023-10-26 with 21 traded days).
5. BR is quoted in USD per barrel with ten barrels per contract, so its RUB multiplier is 10 x the CBR USD/RUB rate of the entry date; equity and index futures are quoted in RUB per lot and have multiplier one.
