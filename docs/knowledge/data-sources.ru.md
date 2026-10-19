> Перевод [data-sources.md](data-sources.md). При расхождении прав английский оригинал.

# Публичные источники данных: проверки доступности (2026-10-19)

Прочитать перед поиском новых рыночных данных. Строки с пометкой (verified) подтверждены инструментами этого репозитория; остальные получены от ассистента во второй панели терминала, который проверял эндпоинты без токенов 2026-10-19, и независимо не проверены.

| Источник | URL | Поля | Задержка | История с |
|---|---|---|---|---|
| Дневная история MOEX ISS по всей доске TQBR (verified, `cmd/tqbrsnap`) | `iss.moex.com/iss/history/engines/stock/markets/shares/boards/TQBR/securities.json?date=D&start=N` | open, high, low, close, volume, value, numtrades по каждой бумаге, торговавшейся в эту дату, включая делистингованные | после закрытия | 2014-01 (719 бумаг) |
| Дневные свечи MOEX ISS по тикеру (verified, `cmd/pricehist`) | `.../securities/<SEC>/candles.json` | OHLCV | после закрытия | 2013 для ликвидных бумаг |
| Внутридневные свечи MOEX ISS | `.../securities/<SEC>/candles.json?interval=1|10|60` | минутные / часовые OHLCV | почти в реальном времени | 2013 (заявлено); старая доска EQBR пуста, использовать TQBR |
| Открытый интерес по фьючерсам MOEX ISS (verified, `cmd/futoifetch`) | `iss.moex.com/iss/analyticalproducts/futoi/securities/<ticker>.json?from&till` | позиции физических (FIZ) и юридических лиц (YUR): pos, long, short, количества, 5-минутные бары | 14 дней для бесплатных пользователей («Free users can't receive data for the last 14 days») | 2023-12 для MX и акционных фьючерсов SR, LK, GZ, GK, RN, MG, VB; YD с 2024-07; TN с 2025-10 |
| Эндпоинт дивидендов MOEX ISS | `iss.moex.com/iss/securities/<SEC>/dividends.json` | нет | | недоступен (возвращает описание бумаги); календарь дивидендов берётся из T-Invest `GetDividends` (`cmd/dividendscalendar`, 105 из 133 ликвидных имён, 401 из 587 старых тикеров не разрешены) |
| Фундаментальные данные и календарь отчётности T-Invest (verified, `cmd/fundsnap`) | `invest-public-api.tbank.ru/rest/...InstrumentsService/GetAssetFundamentals`, `GetAssetReports` | около 60 полей снимка; даты отчётов с `createdAt` | сразу | только снимок, без истории; с 2026-10-19 записывается ежедневно в `data/fundamentals.jsonl` |
| Новости MOEX RSS | `moex.com/export/news.aspx?cat=200` | заголовок, ссылка, дата, содержимое | реальное время | около 7 недель |
| Банк России | `cbr.ru/scripts/XML_daily.asp`, `XML_dynamic.asp`, `cbr.ru/hd_base/KeyRate/`, `.../ruonia/` | курсы валют, ключевая ставка, RUONIA, инфляция | T+1 | годы назад |
| MOEX AlgoPack | `apim.moex.com` | стакан и tradestats | | 401 без платного токена |
| e-disclosure.ru | | раскрытие информации эмитентами | | за JavaScript-антиботом, скриптами недоступен |
| Interfax RSS, fedstat.ru, data.gov.ru, opendata.moex.com, sberbank.com RSS | | | | заблокированы, таймаут или 403 из этой сети |

Цены `close` и `open` свечей ISS на TQBR не скорректированы на сплиты и консолидации (VTBR котировался около 0.05 RUB до консолидации 5000:1 в июле 2024); `scripts/make_candles_csv.py` и `scripts/factors_wide.py` считают дневные движения ниже -45% или выше +80% корпоративными действиями.
