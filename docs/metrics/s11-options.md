# Metrics: options on MOEX FORTS, 11.1-11.10

Part of [metrics.md](../metrics.md) (the index, conventions and ledger live there). Numbers here are the single source of truth for this topic.

### 11.1 The data door and the census (2026-11-08)

MOEX ISS publishes the whole FORTS option market: a chain snapshot (`/iss/engines/futures/markets/options/securities.json`, 39448 series in 64 asset codes on 2026-11-08) and a per-date history (`/iss/history/engines/futures/markets/options/securities.json?date=D`, 6000-12000 rows per date of which 115-304 series traded). History reaches 2016; `from`/`till` are ignored on that endpoint and `date=` is required. Pulled by `cmd/optfetch` in `series` mode: 7277 BR option series plus 49 BR futures, expiries 2023-05-05..2026-11-06.

Option SECID is `<prefix><strike>B<month code><year digit><weekly suffix>`; call codes are A..L (January..December), put codes the same letter shifted by twelve (M..X). The underlying future is the family's future whose last trading date is the first at or after the option expiry.

Turnover census, one date per month 2021-2022: 0.12-2.4 bn RUB per day; **BR (Brent options) is 75-95% of it on every sample**, then CR (CNY options), GD (gold), GZ (Gazprom), LK (Lukoil), Eu (euro). Equity and index options are thin on these dates.

### 11.2 BR: the variance premium (2023-05..2026-11)

ATM implied volatility is Black-76 on the underlying future's settlement price (zero rate); realized volatility is the annualised standard deviation of the future's daily settlement returns from entry to expiry. Entry is the latest trading day within DTE..DTE+3 where the call and the put of the ATM strike both traded (5 contracts minimum per leg); the premium is the traded `WAPRICE`, never the settlement price. USD-quoted families are converted with the CBR USD/RUB rate of the entry date.

| DTE | expiries | IV | RV | VRP | straddle %F | realized move %F | skew 25d | moneyness | realized DTE | net %F |
|---|---|---|---|---|---|---|---|---|---|---|
| 5 | 126 | 0.360 | 0.289 | +0.072 | 3.84 | 3.28 | +0.005 | 0.49% | 6.3 | +0.39 |
| 10 | 50 | 0.368 | 0.298 | +0.070 | 5.50 | 4.48 | -0.017 | 0.70% | 12.8 | +0.78 |
| 20 | 34 | 0.357 | 0.312 | +0.045 | 7.97 | 5.85 | -0.016 | 0.66% | 27.1 | +2.27 |

The straddle premium exceeds the realized move at every tenor, but at DTE 5 only by 0.39% of notional after the frozen cost model; the skew is close to zero, so this is a level premium, not a put-skew harvest.

### 11.3 BR: the four premium-selling books (2023-05..2026-11)

One observation per expiry, held to expiry, contractual payoff against the underlying future's settlement; capital = 15% of the underlying notional (a margin proxy, not a broker's figure). Pooled over DTE 5, 10 and 20 (Bonferroni bar for four strategies t = 2.64; bootstrap by calendar month, 2000 draws).

| book | n | mean RUB | median | NW t | month-bootstrap 95% | worst | CVaR 5% | ann % on capital |
|---|---|---|---|---|---|---|---|---|
| S1 short ATM straddle | 215 | +612.3 | +889 | 2.86 | [+90, +1055] | -14916 | -8162 | 203 |
| S2 short 25-delta strangle | 160 | +783.9 | +787 | 5.94 | [+497, +1034] | -6975 | -3126 | 398 |
| S3 covered call (30-delta) | 188 | +709.1 | +1000 | 3.89 | [+120, +1204] | -13558 | -6535 | 328 |
| S4 delta-hedged straddle | 215 | +486.6 | +513 | 4.63 | [+176, +758] | -4946 | -3719 | 169 |

By DTE (S1): 5 days +362.3 (t 1.63, n 130); 10 days +580.8 (t 1.03, n 51); 20 days +1615.5 (t 4.25, n 34). S2 at DTE 5 is +708.4 (t 4.70, n 114).

Stability: at a 2% half-spread the pooled means are S1 +556.2 (t 2.57), S2 +709.2 (5.28), S3 +750.6 (3.79), S4 +378.5 (3.18); with settlement-price entry and no trade filter (descriptive, not executable) the means are within 10% of the traded-price ones, so the premium is a property of the market rather than of the tradability filter.

By calendar year (mean RUB per expiry) the premium decays:

| book | 2023 | 2024 | 2025 | 2026 (to 11-07) |
|---|---|---|---|---|
| S1 pooled DTE 5-20 | +1208 (36) | +1076 (63) | +476 (68) | **-228 (48)** |
| S2 pooled | +922 (30) | +862 (53) | +294 (52) | +1470 (25) |
| S5 condor | +447 (11) | +192 (35) | +23 (29) | +318 (14) |
| S3 covered call | +591 (31) | +643 (60) | +238 (62) | +1761 (35) |
| S4 delta-hedged | +749 (36) | +664 (63) | +213 (68) | +456 (48) |

The straddle book is carried entirely by 2023-2024 and turns negative in 2026; the strangle, covered call and delta-hedged cells are positive in 2026 on 14-48 observations each, i.e. inside their own noise. The pooled t-statistics are a statement about the whole window, not about the current regime.

### 11.5 The other five families do not show the same premium

**Gold (GD)** pulled to 1426 series; gold options trade far less often than BR, so the sample is 9-15 expiries per tenor.

| DTE | expiries | IV | RV | VRP | straddle %F | move %F | net %F |
|---|---|---|---|---|---|---|---|
| 5 | 15 | 0.182 | 0.154 | +0.028 | 1.77 | 1.53 | -0.16 |
| 10 | 10 | 0.176 | 0.183 | -0.008 | 2.58 | 3.33 | -0.54 |
| 20 | 9 | 0.161 | 0.154 | +0.007 | 3.51 | 3.66 | -0.00 |

The four books pooled (DTE 5 and 20): S1 -1464.4 RUB per expiry (t -1.00, n 26), S4 -1029.2 (t -1.49), S3 +1255.9 (t 1.52, n 12), S2 +1616.8 (t 9.48, n 8). **GD fails the gate for S1, S3 and S4**, and the S2 cell is too small to read.

**Gazprom (GZ)** pulled to 1874 series and does show the premium in implied volatility (IV 0.346 against RV 0.269 at DTE 5, +7.7 points) but the realized move matches the premium (3.38% of forward against a 3.62% straddle), so the net is -0.17% of notional at DTE 5 and +0.28% at DTE 20. The books are negative: S1 -1605.7 RUB per expiry (t -0.99, n 44) with a worst trade of -104416 RUB, S2 -1809.9 (n 20), S4 -261.2. A real VRP that does not survive the realized move is exactly the case the net rows exist to catch.

**CNY (CR)**, 508 series, 13 expiries at DTE 5: IV 0.173 against RV 0.133 (+4.0 points), straddle 2.27% against a realized move of 2.40%, net +0.08% of notional; at DTE 20 there are four expiries and the net is -1.25%. Inconclusive.

**Lukoil (LK)** produced 594 series but one usable expiry; **euro (Eu)** 580 series with zero and three: both are **not tested** for lack of observations. The whole-market daily file (`cmd/optfetch -mode day`, pulled nightly) is the door for them.

### 11.6 Defined-risk variants: what the wings cost

**S5 iron condor** (short 25-delta strangle, long 10-delta wings) and **S6 iron butterfly** (short ATM straddle, long 10-delta wings); capital for these books is the true maximum loss of the structure.

| book | n | mean RUB | NW t | month-bootstrap 95% | worst | CVaR 5% | capital (max loss, DTE 5) | ann % |
|---|---|---|---|---|---|---|---|---|
| S5 condor 25/10 (pooled) | 89 | +188.1 | 2.39 | [+24, +361] | -2408 | -1927 | 5929 | 185 |
| S6 iron fly 10 (pooled) | 95 | +41.1 | 0.22 | [-320, +377] | -5233 | -3837 | 2903 | 58 |
| S2 naked strangle, DTE 5 (reference) | 114 | +708.4 | 4.70 | | -5096 | -2717 | 10368 | 398 |

The condor costs 74% of the naked strangle's mean and buys a 53% smaller worst trade at roughly 57% of the capital; at a 2% half-spread its t falls to 1.64, below the four-book bar. The iron butterfly does not work: the wings eat the premium (t 0.22). A capped-risk version of the BR premium exists but it is a weaker, separate trade.

### 11.7 Capacity

Median contracts traded on the entry day per leg: 21 for the straddle (DTE 5), 13 for the strangle, 14 for the condor. At 10/25/50% participation, with the full-sample means (optimistic for the recent regime):

| book | 10% | 25% | 50% | capital at 25% |
|---|---|---|---|---|
| S1 straddle DTE 5 | 48k RUB | 145k RUB | 290k RUB | 62k RUB |
| S2 strangle DTE 5 | 43k RUB | 129k RUB | 259k RUB | 31k RUB |
| S5 condor DTE 5 | 12k RUB | 36k RUB | 72k RUB | 19k RUB |

50-150 thousand RUB a year at a defensible participation rate, against 148 thousand RUB a year of key-rate carry on 1 mln RUB: capital-efficient, volume-bound, and not able to absorb a 1 mln RUB book.

### 11.8 An ex-ante filter: is the premium worth selling only when it is large?

The signal is `IV_ATM - RV20`, where RV20 is the realised volatility of the underlying future over the 20 trading days ending on the entry day: it is known at entry, unlike the realised volatility to expiry. On BR at DTE 5 (123 observations) the correlation between the signal and the trade's P&L is +0.17 and a one-point increase in the signal is worth about 3027 RUB of mean P&L.

| threshold (vol points) | n | mean RUB | NW t | halves | 2025 | 2026 |
|---|---|---|---|---|---|---|
| none (all entries) | 123 | +456.7 | | | +182 (39) | +131 (25) |
| 0.00 | 102 | +458.0 | 2.41 | 443 / 522 | -114 (28) | +522 (20) |
| 0.02 | 92 | +566.8 | 2.99 | 579 / 522 | +238 (23) | +522 (20) |
| 0.04 | 80 | +605.0 | 2.76 | 647 / 470 | +245 (18) | +470 (19) |
| **0.06** | 66 | **+834.0** | **4.42** | **820 / 870** | **+454 (13)** | **+870 (18)** |
| 0.08 | 54 | +868.3 | 3.83 | 822 / 988 | +284 (8) | +988 (15) |

The threshold is monotone in the mean and in both halves, and at six points it turns the 2025 and 2026 cells positive: the decay in 11.3 is a decay of the *average* premium, and the recent years still pay when the premium is large at entry. The same exercise at DTE 20 does not work (signal correlation -0.12, thresholding reduces the mean), so the filter is a short-dated phenomenon.

Caveat: the thresholds were read on the same sample as the result. The monotonicity and the two independent halves are the evidence; a forward test at the 0.06 threshold is what would make it a rule.

### 11.9 Multi-strike ladder: how much more the market can absorb

Selling three straddles per expiry (the strikes with both legs traded, within 6% of the forward) instead of one, at DTE 5 (87 expiries where at least two such strikes exist):

| | single ATM | three strikes |
|---|---|---|
| mean RUB per expiry | +303.9 | +407.7 |
| contracts available per leg | 43 | 83 |
| NW t | | 0.64 |
| 2023 / 2024 / 2025 / 2026 | | +2231 / +1445 / +279 / **-1454** |

The ladder roughly doubles the volume that can be sold but adds only a third of the mean, and it deepens the 2026 loss: the extra strikes are further from the money, where the premium per unit of risk is worse. Capacity is not bought by widening the strike range.

### 11.4 Verdict

Against the registered gate of `docs/plans/20261108-options-premium.md`: criteria (1) t >= 2.64, (2) both halves positive, (3) net positive at a 2% half-spread and (4) a positive VRP pass for **S2, S3 and S4, and for S1 at the pooled level (t 2.86, 2.57 at 2% spread)**. The condor, added later, does not clear the bar at 2% (t 1.64). The yearly split is the reason to treat the whole thing as a lead and not a deployment: the straddle book is negative in 2026 and the positive 2026 cells of the other books rest on 14-48 observations. The ex-ante filter of 11.8 is the only construction that is positive in both halves and in the last two years, and it is still an in-sample threshold.

Frozen forward rule (registered 2026-11-08, before any forward date exists): for every BR expiry from 2026-11-09, sell the ATM straddle at the traded `WAPRICE` on the latest trading day within DTE 5..DTE 8 before expiry where both legs traded at least 5 contracts, hold to expiry, mark the payoff against the underlying future's settlement, charge 4 RUB per contract per side and 1% of premium per side, **and take the trade only when `IV_ATM - RV20 >= 0.06`**. The forward log is scored on the same pooled statistics with the same bar; a forward mean below zero or a forward month-bootstrap interval that contains zero closes the lead.

### 11.10 Correction (2026-11-09)

The numbers committed on 2026-11-08 (S1 +771.5, t 4.32; yearly +965/+706/+182/+56; condor +218.9) were computed while the CBR USD/RUB file held only 2026 dates: the nightly `options-daily.sh` called `-mode cbr -from <current year>` and `cbrMode` **overwrote** the file instead of merging, so the USD/RUB multiplier of every BR future that ended before 2026 was zero and its P&L collapsed to the 8-RUB fee. `cbrMode` now merges with the existing file, the full 2023-06..2026-11 rate path is restored (855 rows), and every number in 11.2-11.9 is recomputed from scratch. The direction of the finding survives (the premium is real, S2/S3/S4 clear the bar), the magnitude is smaller (S1 +612 against +771) and the 2026 straddle is negative rather than +56.
