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

Capacity is the binding constraint, not the edge: the median number of contracts traded on the entry day per leg is 22 for S1 at DTE 5 and 9-15 for the other cells, i.e. about 0.7-1.7 mln RUB of notional per trade and therefore a strategy measured in tens of thousands of RUB a year on 1 mln RUB of capital, not millions.

### 11.5 Gold (GD) does not show the same premium

GD pulled to 1426 series over 2023-10..2026-11; gold options trade far less often than BR, so the sample is 9-15 expiries per tenor.

| DTE | expiries | IV | RV | VRP | straddle %F | move %F | net %F |
|---|---|---|---|---|---|---|---|
| 5 | 15 | 0.182 | 0.154 | +0.028 | 1.77 | 1.53 | -0.16 |
| 10 | 10 | 0.176 | 0.183 | -0.008 | 2.58 | 3.33 | -0.54 |
| 20 | 9 | 0.161 | 0.154 | +0.007 | 3.51 | 3.66 | -0.00 |

The four books pooled (DTE 5 and 20): S1 short straddle -1464.4 RUB per expiry (t -1.00, n 26), S4 delta-hedged -1029.2 (t -1.49), S3 covered call +1255.9 (t 1.52, n 12), S2 25-delta strangle +1616.8 (t 9.48, n 8). The straddle loses, the VRP is within a point of zero, and the only positive cell is an eight-observation strangle with no losing trade: **GD fails the gate for S1, S3 and S4 and the S2 cell is too small to read**. The premium is specific to Brent, not a property of MOEX options as such, which is what a real premium should look like.

### 11.4 Verdict

Against the registered gate of `docs/plans/20261108-options-premium.md`: criteria (1) t >= 2.64, (2) both halves positive, (3) net positive at a 2% half-spread and (4) a positive VRP all pass for **all four books on BR**. This is a lead for the forward log, never a deployment: the sample is one market, one to three years, four books and a heavy tail (the worst BR straddle lost 1.3% of notional, 19 times the mean gain, and the 5% CVaR is 7.4 thousand RUB against 10.3 thousand RUB of margin-proxy capital). The other five families (GD, CR, GZ, LK, Eu) were still downloading when this section was written and are reported when their pull completes.

Frozen forward rule (registered 2026-11-08, before any forward date exists): for every BR expiry from 2026-11-09, sell the ATM straddle at the traded `WAPRICE` on the latest trading day within DTE 5..DTE 8 before expiry where both legs traded at least 5 contracts, hold to expiry, mark the payoff against the underlying future's settlement, charge 4 RUB per contract per side and 1% of premium per side. The forward log is scored on the same pooled statistics with the same bar; a forward mean below zero or a forward month-bootstrap interval that contains zero closes the lead.
