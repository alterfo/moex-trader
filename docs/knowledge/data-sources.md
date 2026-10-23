# Public data sources: availability checks (2026-10-19)

Russian translation: [data-sources.ru.md](data-sources.ru.md). This file is canonical.

Read before looking for new market data. Rows marked (verified) were confirmed by this repository's own tools; the others come from the assistant in the second terminal pane, which probed the endpoints without tokens on 2026-10-19 and are not independently verified.

| Source | URL | Fields | Delay | History from |
|---|---|---|---|---|
| MOEX ISS daily history of the whole TQBR board (verified, `cmd/tqbrsnap`) | `iss.moex.com/iss/history/engines/stock/markets/shares/boards/TQBR/securities.json?date=D&start=N` | open, high, low, close, volume, value, numtrades for every security that traded on the date, including delisted ones | after the close | 2014-01 (719 securities) |
| MOEX ISS daily candles per ticker (verified, `cmd/pricehist`) | `.../securities/<SEC>/candles.json` | OHLCV | after the close | 2013 for liquid names |
| MOEX ISS intraday candles | `.../securities/<SEC>/candles.json?interval=1|10|60` | minute / hour OHLCV | near real time | 2013 (reported); the old EQBR board is empty, use TQBR |
| MOEX ISS futures open interest (verified, `cmd/futoifetch`) | `iss.moex.com/iss/analyticalproducts/futoi/securities/<ticker>.json?from&till` | positions of individuals (FIZ) and legal entities (YUR): pos, long, short, counts, 5-minute bars | 14 days for free users ("Free users can't receive data for the last 14 days") | 2023-12 for MX and stock futures SR, LK, GZ, GK, RN, MG, VB; YD from 2024-07; TN from 2025-10 |
| MOEX ISS dividends endpoint | `iss.moex.com/iss/securities/<SEC>/dividends.json` | none | | not available (returns the security description); the dividend calendar comes from T-Invest `GetDividends` (`cmd/dividendscalendar`, 105 of the 133 liquid names, 401 of 587 older tickers unresolved) |
| T-Invest fundamentals and report calendar (verified, `cmd/fundsnap`) | `invest-public-api.tbank.ru/rest/...InstrumentsService/GetAssetFundamentals`, `GetAssetReports` | about 60 snapshot fields; report dates with `createdAt` | immediate | snapshot only, no history; recorded daily from 2026-10-19 in `data/fundamentals.jsonl` |
| MOEX RSS news | `moex.com/export/news.aspx?cat=200` | title, link, date, content | real time | about 7 weeks |
| Bank of Russia | `cbr.ru/scripts/XML_daily.asp`, `XML_dynamic.asp`, `cbr.ru/hd_base/KeyRate/`, `.../ruonia/` | currency rates, key rate, RUONIA, inflation | T+1 | years back |
| MOEX AlgoPack (paid key, verified 2026-10-22, `cmd/algopackfetch`) | `apim.moex.com/iss/datashop/algopack/eq/{tradestats,obstats,orderstats,hi2}.json?date=` | 5-minute bars: trades and buy/sell split, order-book imbalance and spreads, placed and cancelled orders, concentration; all tickers per date, pages of 1000 rows (`limit` ignored) | about 10 seconds after the bar | 2020-01-03 |
| e-disclosure.ru | | issuer disclosures | | behind a JavaScript anti-bot page, not reachable by scripts |
| Interfax RSS, fedstat.ru, data.gov.ru, opendata.moex.com, sberbank.com RSS | | | | blocked, timing out or 403 from this network |

The ISS candle `close` and `open` of TQBR are unadjusted for splits and consolidations (VTBR is quoted at about 0.05 RUB before its July 2024 5000:1 consolidation); `scripts/make_candles_csv.py` and `scripts/factors_wide.py` treat one-day moves below -45% or above +80% as corporate actions.
