# Metrics: options on MOEX FORTS, 11.1-11.3

Part of [metrics.md](../metrics.md) (the index, conventions and ledger live there). Numbers here are the single source of truth for this topic.

### 11.1 The data door and the census (2026-11-08)

MOEX ISS publishes the whole FORTS option market: a chain snapshot (`/iss/engines/futures/markets/options/securities.json`, 39448 series in 64 asset codes on 2026-11-08) and a per-date history (`/iss/history/engines/futures/markets/options/securities.json?date=D`, 6000-12000 rows per date of which 115-304 series traded). History reaches 2016; `from`/`till` are ignored on that endpoint and `date=` is required. Pulled by `cmd/optfetch` in `series` mode: 7263 BR option series plus 49 BR futures, expiries 2023-05-05..2026-11-06 (five families still pulling, see 11.4).

Option SECID is `<prefix><strike>B<month code><year digit><weekly suffix>`; call codes are A..L (January..December), put codes the same letter shifted by twelve (M..X). The underlying future is the family's future whose last trading date is the first at or after the option expiry.

Turnover census, one date per month 2021-2022: 0.12-2.4 bn RUB per day; **BR (Brent options) is 75-95% of it on every sample**, then CR (CNY options), GD (gold), GZ (Gazprom), LK (Lukoil), Eu (euro). Equity and index options are thin on these dates.

### 11.2 BR: the variance premium (2023-05..2026-11)

ATM implied volatility is Black-76 on the underlying future's settlement price (zero rate); realized volatility is the annualised standard deviation of the future's daily settlement returns from entry to expiry. Entry is the latest trading day within DTE..DTE+3 where the call and the put of the ATM strike both traded (5 contracts minimum per leg); the premium is the traded `WAPRICE`, never the settlement price.

| DTE | expiries | IV | RV | VRP | straddle %F | realized move %F | skew 25d | moneyness | realized DTE | net %F |
|---|---|---|---|---|---|---|---|---|---|---|
| 5 | 129 | 0.368 | 0.291 | +0.077 | 3.92 | 3.27 | +0.007 | 0.51% | 6.3 | +0.51 |
| 10 | 50 | 0.372 | 0.300 | +0.072 | 5.56 | 4.06 | -0.016 | 0.70% | 12.9 | +1.28 |
| 20 | 35 | 0.375 | 0.318 | +0.058 | 8.39 | 5.91 | -0.018 | 0.63% | 27.2 | +2.62 |

The straddle premium exceeds the realized move at every tenor, and the excess is not a cost artefact: the net column charges the frozen cost model (4 RUB per contract per side plus 1% of premium per side plus the CBR USD/RUB conversion of the entry date). The skew is close to zero: this is a level premium, not a put-skew harvest.

### 11.3 BR: four premium-selling books (2023-05..2026-11)

One observation per expiry, held to expiry, contractual payoff against the underlying future's settlement; capital = 15% of the underlying notional (a margin proxy, not a broker's figure). Pooled over DTE 5, 10 and 20 (Bonferroni bar for four strategies t = 2.64; bootstrap by calendar month, 2000 draws).

| book | n | mean RUB | median | sd | NW t | month-bootstrap 95% | worst | CVaR 5% | ann % on capital |
|---|---|---|---|---|---|---|---|---|---|
| S1 short ATM straddle | 219 | +771.5 | +889 | 3458 | 4.32 | [+338, +1187] | -14916 | -7381 | 245 |
| S2 short 25-delta strangle | 166 | +822.3 | +766 | 1494 | 6.14 | [+564, +1101] | -6975 | -2897 | 415 |
| S3 covered call (30-delta) | 191 | +690.8 | +900 | 3057 | 3.51 | [+124, +1258] | -13559 | -6535 | 328 |
| S4 delta-hedged straddle | 219 | +538.0 | +513 | 1712 | 5.07 | [+266, +797] | -4946 | -3670 | 169 |

By DTE (S1): 5 days +438.6 (t 2.48, n 133); 10 days +901.9 (t 2.41, n 51); 20 days +1846.6 (t 4.17, n 35). Both halves of the window (split 2025-07-01) are positive in every cell, for example S1 at DTE 5 +550.8 then +263.8, S2 at DTE 5 +569.7 then +1025.7.

Stability (pooled, DTE 5 and 20): at a 2% half-spread S1 +668.9 (t 3.40), S2 +741.0 (t 5.30), S3 +742.0 (t 3.57), S4 +430.2 (t 3.54); at a 5% half-spread S1 +458.3 (t 2.39), S2 +661.9 (t 4.98), S3 +716.1 (t 3.49), S4 +221.1 (t 1.98). With settlement-price entry and no trade filter (descriptive, not executable) the means are S1 +682.9, S2 +627.4, S3 +860.4, S4 +467.5: the premium is a property of the market, not of the tradability filter. Doubling the exchange fee changes a trade by 8 RUB against a mean of 771.

By calendar year (mean RUB per expiry) the premium is decaying, and the decay is clearest in the shortest book:

| book | 2023 | 2024 | 2025 | 2026 (to 11-07) |
|---|---|---|---|---|
| S1 at DTE 5 only | +965 (22) | +706 (40) | +182 (39) | +56 (32) |
| S1 pooled DTE 5-20 | +912 (28) | +1101 (50) | +484 (50) | +453 (40) |
| S2 pooled | +746 (24) | +777 (42) | +369 (41) | +1376 (27) |
| S4 pooled | +610 (28) | +633 (50) | +169 (50) | +647 (40) |

The shortest straddle, the book that would actually be run, went from +965 RUB per expiry in 2023 to +56 in 2026: the average premium over the realized move has compressed to the level of the cost charge, and the 2026 cell is inside its own noise (standard deviation about 2100 RUB). A strategy that is only statistically alive on pooled history is not a strategy.

Capacity is the binding constraint, not the edge: the median number of contracts traded on the entry day per leg is 22 for S1 at DTE 5 and 9-15 for the other cells, i.e. about 0.7-1.7 mln RUB of notional per trade and therefore a strategy measured in tens of thousands of RUB a year on 1 mln RUB of capital, not millions.

### 11.5 The other five families do not show the same premium

**Gold (GD)** pulled to 1426 series over 2023-10..2026-11; gold options trade far less often than BR, so the sample is 9-15 expiries per tenor.

GD pulled to 1426 series over 2023-10..2026-11; gold options trade far less often than BR, so the sample is 9-15 expiries per tenor.

| DTE | expiries | IV | RV | VRP | straddle %F | move %F | net %F |
|---|---|---|---|---|---|---|---|
| 5 | 15 | 0.182 | 0.154 | +0.028 | 1.77 | 1.53 | -0.16 |
| 10 | 10 | 0.176 | 0.183 | -0.008 | 2.58 | 3.33 | -0.54 |
| 20 | 9 | 0.161 | 0.154 | +0.007 | 3.51 | 3.66 | -0.00 |

The four books pooled (DTE 5 and 20): S1 short straddle -1464.4 RUB per expiry (t -1.00, n 26), S4 delta-hedged -1029.2 (t -1.49), S3 covered call +1255.9 (t 1.52, n 12), S2 25-delta strangle +1616.8 (t 9.48, n 8). The straddle loses, the VRP is within a point of zero, and the only positive cell is an eight-observation strangle with no losing trade: **GD fails the gate for S1, S3 and S4 and the S2 cell is too small to read**. The premium is specific to Brent, not a property of MOEX options as such, which is what a real premium should look like.

**CNY (CR)**, 508 series, 13 expiries at DTE 5: IV 0.173 against RV 0.133 (+4.0 points), straddle 2.27% of forward against a realized move of 2.40%, net +0.08% of notional; at DTE 20 there are four expiries and the net is -1.25%. **Inconclusive, close to zero, and too few observations for any verdict.** **GZ (Gazprom), LK (Lukoil) and Eu (euro)** produced 16, 0 and 0 series: the generator either found no listed series for the historical price levels or the family trades too rarely to generate them, so they are **not tested**, and a future plan that wants them needs the per-date whole-market pull (`cmd/optfetch -mode day`) instead of generated SECIDs.

### 11.6 Defined-risk variants: what the wings cost

Two protected books were added to the battery, both with one contract per leg and the same cost model: **S5 iron condor** (short 25-delta strangle, long 10-delta wings) and **S6 iron butterfly** (short ATM straddle, long 10-delta wings). Capital for these books is the true maximum loss of the structure, not the 15% proxy.

| book | n | mean RUB | NW t | month-bootstrap 95% | worst | CVaR 5% | capital (max loss) | ann % |
|---|---|---|---|---|---|---|---|---|
| S5 condor 25/10 (pooled) | 87 | +218.9 | 2.88 | [+50, +378] | -2408 | -1927 | 6212 | 194 |
| S5 condor 25/10, DTE 5 | 85 | +206.7 | 2.70 | | -2408 | -1927 | 6212 | 194 |
| S6 iron fly 10 (pooled) | 90 | +90.9 | 0.53 | [-350, +438] | -5233 | -3837 | 3030 | 58 |
| S2 naked strangle, DTE 5 (reference) | 119 | +746.0 | 4.88 | | -5096 | -2654 | 10411 | 415 |

The condor costs 72% of the naked strangle's mean and buys a 53% smaller worst trade and a 27% smaller CVaR, at roughly half the capital; return on capital falls from 415% to 194% but the capital is now bounded and real. The iron butterfly is the wrong structure on this market: the wings are close enough to the money that they eat the whole premium (t 0.53). A capped-risk version of the BR premium is therefore possible, but it is a different, weaker trade, not a free tail hedge.

### 11.7 Capacity and the forward log

Median contracts traded on the entry day per leg: 22 for the straddle, 12 for the strangle, 13 for the condor. At a participation rate of 10/25/50% of that volume the books would run 2/6/11, 1/3/6 and 1/3/6 contracts; with the full-sample means (which 11.3 shows are optimistic for the recent regime) the annual income is:

| book | 10% | 25% | 50% | capital at 25% |
|---|---|---|---|---|
| S1 straddle DTE 5 | 50.5k RUB | 151.5k RUB | 277.8k RUB | 61.9k RUB |
| S2 strangle DTE 5 | 43.2k RUB | 129.6k RUB | 259.2k RUB | 31.2k RUB |
| S5 condor DTE 5 | 12.1k RUB | 36.2k RUB | 72.5k RUB | 18.6k RUB |

The strategy is capital-efficient and volume-bound: 50-150 thousand RUB a year at a defensible participation rate, against 148 thousand RUB a year of key-rate carry on 1 mln RUB. It cannot absorb a 1 mln RUB book, which is the honest reason it is a lead and not the answer to the risk-free hurdle.

The forward log started on 2026-11-08: `cmd/optscan -mode fwd` writes a daily snapshot of every BR expiry between DTE 1 and 10 (strike, traded prices, volumes, forward) to `data/options/forward-snapshots.jsonl`, and `-mode fwdsettle -target 5` scores the DTE-5 entry once the expiry has passed, writing `artifacts/options/forward-results.csv`. Both run from `scripts/options-daily.sh` in the nightly launchd job, so the frozen rule of 11.4 accrues evidence without anyone remembering to run it.

### 11.4 Verdict

Against the registered gate of `docs/plans/20261108-options-premium.md`: criteria (1) t >= 2.64, (2) both halves positive, (3) net positive at a 2% half-spread and (4) a positive VRP all pass for **all four books on BR**. The yearly decay (11.3) is a second reason to treat it as a lead and not a deployment: the pooled t-statistic is carried by 2023-2024. This is a lead for the forward log, never a deployment: the sample is one market, one to three years, four books and a heavy tail (the worst BR straddle lost 1.3% of notional, 19 times the mean gain, and the 5% CVaR is 7.4 thousand RUB against 10.3 thousand RUB of margin-proxy capital). The other five families (GD, CR, GZ, LK, Eu) were still downloading when this section was written and are reported when their pull completes.

Frozen forward rule (registered 2026-11-08, before any forward date exists): for every BR expiry from 2026-11-09, sell the ATM straddle at the traded `WAPRICE` on the latest trading day within DTE 5..DTE 8 before expiry where both legs traded at least 5 contracts, hold to expiry, mark the payoff against the underlying future's settlement, charge 4 RUB per contract per side and 1% of premium per side. The forward log is scored on the same pooled statistics with the same bar; a forward mean below zero or a forward month-bootstrap interval that contains zero closes the lead.
