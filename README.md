# MOEX Trader

Go-трейдер для инструментов MOEX: собирает рыночные данные и новости, строит вектор
из 18 признаков, получает сигнал `BUY`/`SELL`/`HOLD` от ансамбля **lgbm + xgb + logreg**,
прогоняет его через риск-гейт и исполняет — сначала в paper-режиме, затем в песочнице
T-Банка. Каждый шаг пишется в SQLite как аудит-событие. Перед стартом живого цикла
`cmd/trader` прогоняет preflight-бэктест **той же модели и конфигурации** и отказывается
стартовать при убытке или при недоступной истории.

Реальные деньги намеренно отключены: `cmd/trader` отказывается работать с
не-песочным счётом. LLM/Ollama в проекте больше не используется — сигнал строит
только ансамбль моделей.

## Содержание

1. [Архитектура](#архитектура)
2. [Живой цикл трейдера](#живой-цикл-трейдера)
3. [Модель и признаки](#модель-и-признаки)
4. [Бэктест и preflight](#бэктест-и-preflight)
5. [Исполнение: paper / sandbox / live](#исполнение-paper--sandbox--live)
6. [Хранилище, аудит, наблюдаемость](#хранилище-аудит-наблюдаемость)
7. [Конфигурация](#конфигурация)
8. [Команды](#команды)
9. [Отказоустойчивость: основной Mac + резервный ai-box](#отказоустойчивость-основной-mac--резервный-ai-box)
10. [SDLC: как это менять и дорабатывать](#sdlc-как-это-менять-и-дорабатывать)
11. [Текущее качество модели и приоритеты](#текущее-качество-модели-и-приоритеты)

## Архитектура

Один процесс, модульный монолит: пакеты `internal/*` общаются через интерфейсы, сетевых
хопов между компонентами нет. `internal/bus` (Redis Streams) — задел на будущее
разделение процессов, сейчас нигде не подключён.

```
                    cmd/trader (main)
                          │
   MOEX ISS ──┐           │
   RSS news ──┼──> orchestrator.MOEXIngestor ──> features.Builder ──> domain.FeatureContext
   AlgoPack ──┘        (котировки, свечи,            (18 признаков)          │
                        новости, стакан)                                     │
                                                                             v
                                              model.EnsembleSignalSource (или model.SignalSource)
                                                          │  domain.TradeSignal
                                                          v
                          risk.HardenedGate ──> executor.Executor (paper | Tinkoff sandbox)
                                                          │  executor.Fill
                                                          v
                    SQLite audit_events ──> cmd/verifier (P&L-отчёт)
                    Telegram (сигналы, kill switch) + Prometheus :9090
```

Перед запуском цикла:

```
config.yaml ──> backtest.Engine (та же SignalSource, тот же риск-гейт, те же комиссии)
                    │
                    ├─ NetPnl < preflight.min_net_pnl        -> отказ стартовать
                    ├─ 0 решений (нет истории)               -> отказ стартовать
                    └─ все решения упали с ошибкой           -> отказ стартовать
```

### Карта пакетов

| Путь | Что там |
|---|---|
| `cmd/trader` | живой цикл: сборка всех компонентов, preflight, market-hours guard, Telegram-нотификатор |
| `cmd/sandboxcheck` | проверка песочницы T-Банка: авторизация, счёт, портфель, торговый статус, `-place-order` |
| `cmd/trainmodel` | обучение чисто-Go логистической регрессии + OOS-валидация, пишет `model.json` |
| `cmd/calibrate` | диагностика разметки: распределение forward-return по горизонтам (`-horizons 1,3,5`) |
| `cmd/exportdataset` | выгрузка исторического датасета в CSV для Python-обучения ансамбля |
| `cmd/leadlag` | диагностика lead-lag между тикерами (кросс-корреляция + Granger, `-robustness`) |
| `cmd/backtest` | исторический прогон: `-signal-source=model\|ensemble\|rule\|csvprob` |
| `cmd/verifier` | markdown-отчёт по аудиту: сигналы vs сделки, P&L за вычетом комиссий |
| `internal/model` | ансамбль, логистическая регрессия, фичи, датасет, калибровка, lead-lag |
| `internal/features` | `Builder` — `FeatureContext` из свечей/котировок/новостей/стакана |
| `internal/backtest` | движок реплея, `ISSSource` (дневные свечи с пейджингом), кэш решений |
| `internal/risk` | `HardenedGate`: лимит лотов/позиции, fat-finger, дневной убыток, drawdown, kill switch |
| `internal/executor` | `PaperExecutor`, `LiveExecutor` (Tinkoff), Finam-исполнитель (не подключён к `cmd/trader`) |
| `internal/broker/tinkoff` | песочница: bootstrap счёта, снапшот портфеля, отмена ордеров, торговый статус |
| `internal/ingestion` | клиенты MOEX ISS, RSS-новости, AlgoPack (стакан), Tinkoff, Finam |
| `internal/orchestrator` | цикл `RunOnce`, ingest |
| `internal/storage` | SQLite (WAL): `audit_events`, `trade_signals`, `kill_switch`, миграции |
| `internal/domain` | `FeatureContext`, `TradeSignal`, `AuditEvent` |
| `internal/metrics` | Prometheus: `signals_generated_total`, `risk_rejections_total`, тайминг инференса |
| `internal/alert/telegram` | алерты: ошибка сигнала, kill switch, сделки с результатом |
| `internal/verifier` | сборка P&L-отчёта из аудит-событий |
| `scripts/*.py` | обучение/сравнение ансамбля: `export_ensemble.py`, `compare_models.py`, `combos.py` |

### Живой цикл трейдера

`cmd/trader` при старте: config → storage → опциональный `-reset-kill-switch` → клиенты
(MOEX, RSS, AlgoPack, Telegram) → загрузка модели → preflight → broker runtime → риск-гейт →
Prometheus-сервер → `orchestrator.Run(ctx)`.

Дальше каждые `poll_interval` (по умолчанию 5m) выполняется `RunOnce`:

1. `ResetCycle`/`PrepareCycle` — сброс кэша новостей и параллельная предзагрузка стаканов
   AlgoPack (2 воркера) на весь список тикеров.
2. Снапшот счёта (`AccountSource`): в песочнице — портфель T-Банка, в paper — отсутствует.
3. Для каждого тикера:
   - проверка kill switch (активен → новые сигналы не идут);
   - ingest: `LookupSecurity` → `Quote` → дневные свечи за 10 дней → новости (кэш на цикл) →
     стакан AlgoPack;
   - `features.Builder.Build` → `FeatureContext`, аудит-событие стадии `ingest`;
   - `SignalSource.Generate` → `TradeSignal`, аудит стадии `llm` (имя стадии историческое);
   - `risk.HardenedGate.Approve`, аудит `risk_gate`; отклонённый сигнал не исполняется;
   - `Executor.Execute` → `Fill`, аудит `executor`; Telegram-уведомление об исполненной заявке
     (риск-гейт, ошибки исполнения и пропуски из-за закрытой биржи в Telegram не идут — только
     аудит-лог).
4. Следующий цикл.

В песочнице исполнение дополнительно обёрнуто в market-hours guard: перед ордером
проверяется торговый статус инструмента, и вне сессии ордер не отправляется (одна строка
лога на тикер в день вместо ошибки `30079`).

## Модель и признаки

### Вектор признаков (18)

Порядок задаётся один раз в `internal/model/features.go` (`defaultFeatureOrder`/`ToVector`):

```
return_pct, realized_volatility, news_sentiment, news_count, order_book_imbalance,
mom_5d, mom_21d, mom_63d, reversal_1d, rsi_14, dist_ma20_pct, dist_ma50_pct,
realized_vol_21d_annualized_pct, volume_zscore_20d,
macd_hist_pct, stoch_k_14, williams_r_14, alligator_spread_pct
```

Осилилляторы (MACD, Stochastic, Williams %R, Alligator) считаются в
`internal/features/price_features.go`. Порядок обязан совпадать в четырёх местах:
`internal/model/features.go`, заголовок CSV в `cmd/exportdataset/main.go`,
`FEATURES` в `scripts/export_ensemble.py` и `scripts/compare_models.py`. Рассинхрон
ловится на загрузке модели (`checkFeatureOrder`) и в preflight — это не молчаливый баг,
а жёсткий отказ.

### Живой источник сигнала: ансамбль

- Артефакт: `ensemble_model.json` (сейчас в корне репозитория), конфиг `model.ensemble_path`
  (перекрывает `model.path`, если задан).
- Состав: LightGBM + XGBoost + логистическая регрессия; итоговая вероятность —
  среднее трёх: `p = (p_lgb + p_xgb + p_lr) / 3`.
- Пороги хранятся в артефакте: `buy_threshold: 0.60`, `sell_threshold: 0.40`.
  `p >= 0.60` → BUY, `p <= 0.40` → SELL, иначе HOLD (`HoldReasonModel`).
  `TargetLots = risk.max_lots`, `Confidence = |p - 0.5| * 2`.
- Инференс: `internal/model/ensemble.go` (`EnsembleSignalSource`), дерево обходится в Go
  без внешних зависимостей.

Инварианты инференса (выведены и перепроверены против Python `predict_proba`, не
переоткрывать заново):

- LGBM-деревья сравниваются во `float64`, XGBoost — во `float32`; вход для XGB-деревьев
  надо квантовать в `float32`, иначе расхождения ~0.01 на границах сплитов.
- `base_score` у XGBoost хранится в вероятностном пространстве (при `boost_from_average`),
  перед суммированием с листьями — `log(bs/(1-bs))`; в JSON это строка в скобках, `"[4.7E-1]"`.
- Значение листа — `split_conditions[node]` при `left_children[node] == -1`.
- Итог: `sigmoid(base_logit + Σ листьев)`.

### Как обучается ансамбль (офлайн, Python)

```sh
# 1. Датасет: разметка = excess return над IMOEX, горизонт 5 дней, дедбенд 0.5%
go run ./cmd/exportdataset -config config.yaml \
  -from 2024-09-01 -split 2026-06-18 -out /tmp/moex-dataset.csv

# 2. Обучение и экспорт в JSON для Go (нужны xgboost, lightgbm, scikit-learn, pandas, numpy)
DATASET=/tmp/moex-dataset.csv ENSEMBLE_OUT=ensemble_model.json python3 scripts/export_ensemble.py

# 3. Сравнение моделей (val AUC/accuracy) и исследование комбинаций
DATASET=/tmp/moex-dataset.csv PREDS=/tmp/moex-preds.csv python3 scripts/compare_models.py
```

`export_ensemble.py` обучает xgb/lgbm/logreg, конвертирует деревья в плоские массивы
(`si/sc/lc/rc/dl`) и пишет `ensemble_model.json`. По умолчанию `news_sentiment`,
`news_count`, `order_book_imbalance` в историческом CSV структурно равны нулю —
`cmd/exportdataset` не подключает исторические новости и стаканы сам по себе. Если есть
finanalys-формата `news_history.jsonl` (см. `cmd/newsfetch`), передай его через
`-news-history` — `cmd/exportdataset` подмешает реальный `news_sentiment`/`news_count`
по дате и тикеру (то же самое умеет `cmd/trainmodel -news-history` для чисто-Go модели).
`order_book_imbalance` пока всё равно всегда 0 — исторических стаканов нет.

### Резервный источник: логистическая регрессия

Если `model.ensemble_path` пуст, трейдер использует `model.json` от `cmd/trainmodel` —
чисто-Go обучение на двух годах истории, горизонт 5 дней, дедбенд 0.5%, последние 90 дней
на OOS-валидацию. `cmd/trainmodel` печатает markdown-отчёт (Sharpe, hit rate, max drawdown)
и пишет артефакт с метаданными обучения.

### Диагностика (в торговый путь не входит)

- `cmd/calibrate` — как ведёт себя forward-return на разных горизонтах и с каким stride;
  может подмешать реальную историю новостей (`-news-history`) вместо нулевого сентимента.
- `cmd/newsfetch` — архивирует новости в finanalys-формате `news_history.jsonl`, который
  затем читают `cmd/calibrate -news-history`, `cmd/trainmodel -news-history` и
  `cmd/exportdataset -news-history`.
- `cmd/leadlag` — кто кого опережает: кросс-корреляции с лагами, Granger-тест, поправка
  Бонферрони; `-robustness -candidate X -target Y` проверяет пару на непересекающихся окнах.

## Бэктест и preflight

Движок `internal/backtest/engine.go` — это тот же путь, что в лайве, без сети:

- дневные свечи MOEX ISS (`ISSSource`, корректный пейджинг, warmup 100 дней);
- на дне `d` фичи строятся из свечей `[:d]` (без заглядывания вперёд), вход — по
  `candles[d+1].Open`, mark-to-market — по `candles[d].Close`;
- сигнал проходит тот же `risk.HardenedGate`; комиссия берётся с входа и выхода;
- позиции ведутся по тикерам независимо, эквити агрегируется как депозит + Σ P&L;
- результат: net/gross P&L, комиссии, hit rate, Sharpe, max drawdown, число решений и
  разбивка hold-причин (включая `error`/`timeout`).

Источники сигнала в `cmd/backtest`:

| `-signal-source` | Что это |
|---|---|
| `model` (по умолчанию) | `model.json` — логистическая регрессия |
| `ensemble` | `ensemble_model.json` (то, что в лайве), нужен `-ensemble-path` |
| `rule` | правило на `reversal_1d`, нужен `-reversal-threshold` |
| `csvprob` | вероятности из CSV (результат Python-исследований) |

Полезные флаги: `-commission-rate` (для реального тарифа Т-Банка «Трейдер» — `0.0005`),
`-lookback-days` (ограничить число решений на тикер), `-min-confidence`, `-kill-switch`,
`-cache` (кэш решений), `-out` (markdown-отчёт).

Preflight в `cmd/trader` — это тот же движок на последних `preflight.days` (90) днях с
`preflight.deposit` и тем же сигнал-сорсом, что и лайв. Отказ стартовать, если:

- net P&L ниже `preflight.min_net_pnl` (по умолчанию `0` — любой убыток блокирует старт);
- не получено ни одного решения (история недоступна по всем тикерам);
- **все** решения завершились ошибкой/таймаутом (защита от «0 сделок, P&L 0, гейт прошёл»).

## Исполнение: paper / sandbox / live

**Paper** (`broker: paper`, `is_paper_trading: true`) — по умолчанию. `PaperExecutor`
пишет виртуальные филлы в SQLite с комиссией `price × lots × commission.rate`.
Счётного снапшота нет, поэтому дневной убыток и drawdown не проверяются; лимит лотов и
fat-finger работают.

**Sandbox T-Банка** (`broker: tinkoff`, `is_paper_trading: false`, `tinkoff.sandbox: true`) —
рекомендуемый боевой контур:

```yaml
broker: "tinkoff"
is_paper_trading: false
commission:
  rate: "0.0005"          # плоские 0.05% в песочнице
tinkoff:
  endpoint: "sandbox-invest-public-api.tbank.ru:443"
  sandbox: true
  account_id: ""          # пусто: переиспользует первый счёт или откроет новый
  pay_in: "1000000"       # пополнение и база для drawdown
  order_type: "market"
```

Готовый шаблон — `config.sandbox.yaml`. Проверка контура:

```sh
go run ./cmd/sandboxcheck -config config.sandbox.yaml
go run ./cmd/sandboxcheck -config config.sandbox.yaml -ticker SBER -place-order
go run ./cmd/trader -config config.sandbox.yaml
```

Особенности песочницы:

- `EnsureAccount` переиспользует первый счёт или открывает новый и пополняет его на
  `pay_in`; id счёта стоит закрепить в `tinkoff.account_id` для повторных запусков;
- `Snapshot` даёт риск-гейту `Deposit = pay_in`, `DayStartEquity` (первый снапшот дня) и
  `CurrentEquity` из портфеля;
- ордера идемпотентны (UUID v4 `OrderID`), `market` исполняется по последней цене;
- kill switch отменяет только ордера по инструментам бота — ручные ордера человека на том
  же счёте не трогаются (регрессионный тест `TestCancelOpenOrdersLeavesForeignOrdersAlone`);
- песочница не начисляет дивиденды/налоги и не отражает реальное проскальзывание — P&L
  приблизительный.

**Live** — намеренно не подключён: при `tinkoff.sandbox: false` и для Finam-live
`cmd/trader` возвращает ошибку, а не работает с обойдёнными защитами. Порядок включения —
`docs/GO_LIVE_CHECKLIST.md`.

### Риск-гейт

`risk.DefaultConfig()`: `max_lots` (из конфига), дневной убыток 0.5% депозита,
fat-finger 2%, drawdown 3% → kill switch. Проверки: лимит лотов, суммарная позиция
(`|текущая + новая| <= max_lots`), отклонение цены от bid/ask (fallback — prev close),
дневной убыток, drawdown. В live-режиме без данных счёта сделки запрещены. Kill switch
персистится в SQLite, блокирует новые сигналы до `cmd/trader -reset-kill-switch` и
эскалируется в Telegram.

## Хранилище, аудит, наблюдаемость

SQLite (`modernc.org/sqlite`, WAL + busy timeout 5000ms), файл из `storage.path`:

- `audit_events` — каждая стадия цикла (`ingest`, `llm`, `risk_gate`, `executor`) в JSON;
- `trade_signals` — сигналы отдельной таблицей;
- `kill_switch` — одна строка с флагом.

Текущая позиция не хранится отдельно: `CurrentLots` выводится из executor-событий
(BUY прибавляет лоты, SELL вычитает). Это значит, что бот видит только свои сделки и не
знает о ручных позициях человека на том же счёте — SELL по такому тикеру откроет шорт,
а не продаст акции человека.

Наблюдаемость: Prometheus на `:9090` (`-metrics-addr`) — `signals_generated_total`,
`risk_rejections_total`, `llm_inference_duration_seconds`; Telegram — ошибки генерации
сигналов, kill switch и решения BUY/SELL с исходом исполнения (фильтр
`telegram.signal_tickers`, дедупликация по «тикер + действие + исход»); `cmd/verifier` —
markdown-отчёт по P&L (gross, комиссии, net).

## Конфигурация

YAML + `.env` рядом с конфигом (реальные env-переменные приоритетнее `.env`). Секреты
(`MOEX_TRADER_TINKOFF_TOKEN`, `MOEX_TRADER_FINAM_SECRET_TOKEN`, `MOEX_TRADER_ALGOPACK_TOKEN`)
читаются только из окружения, в YAML их нет. Шаблон — `config.example.yaml`, песочница —
`config.sandbox.yaml`, реальные `config.yaml` и `.env` в git не попадают. Список
переменных окружения — в `.env.example` (только имена, без значений).

| Поле | Env |
|---|---|
| `tickers` | `MOEX_TRADER_TICKERS` (через запятую) |
| `model.path` | `MOEX_TRADER_MODEL_PATH` |
| `model.ensemble_path` | — (только YAML) |
| `moex_iss_base_url` | `MOEX_TRADER_MOEX_ISS_URL` |
| `algopack_base_url` / `algopack_token` | `MOEX_TRADER_ALGOPACK_BASE_URL` / `_TOKEN` (секрет) |
| `storage.path` | `MOEX_TRADER_STORAGE_PATH` |
| `risk.max_lots` | `MOEX_TRADER_RISK_MAX_LOTS` |
| `commission.broker` / `rate` | `MOEX_TRADER_COMMISSION_BROKER` / `_RATE` |
| `finam.base_url` / `secret_token` | `MOEX_TRADER_FINAM_BASE_URL` / `_SECRET_TOKEN` (секрет) |
| `tinkoff.endpoint` / `token` / `sandbox` / `account_id` / `pay_in` / `order_type` | `MOEX_TRADER_TINKOFF_ENDPOINT` / `_TOKEN` (секрет) / `_SANDBOX` / `_ACCOUNT_ID` / `_PAY_IN` / `_ORDER_TYPE` |
| `preflight.enabled` / `days` / `deposit` / `min_net_pnl` | `MOEX_TRADER_PREFLIGHT_ENABLED` / `_DAYS` / `_DEPOSIT` / `_MIN_NET_PNL` |
| `broker` | `MOEX_TRADER_BROKER` (`paper` / `tinkoff` / `finam`) |
| `poll_interval` | `MOEX_TRADER_POLL_INTERVAL` |
| `is_paper_trading` | `MOEX_TRADER_IS_PAPER_TRADING` |
| `telegram.bot_token` / `chat_id` / `signal_tickers` / `proxy` | `MOEX_TRADER_TELEGRAM_BOT_TOKEN` (секрет) / `_CHAT_ID` / `_SIGNAL_TICKERS` / `_PROXY` |

Telegram из этой сети напрямую недоступен: `telegram.proxy: "socks5://127.0.0.1:3333"`.

## Команды

```sh
go build ./...

# живой контур
go run ./cmd/trader -config config.yaml                 # paper
go run ./cmd/trader -config config.sandbox.yaml         # T-Bank sandbox
go run ./cmd/trader -reset-kill-switch                  # сбросить kill switch и выйти

# проверка песочницы
go run ./cmd/sandboxcheck -config config.sandbox.yaml [-ticker SBER] [-place-order]

# модель
go run ./cmd/exportdataset -config config.yaml -from 2024-09-01 -split 2026-06-18 -out /tmp/moex-dataset.csv
DATASET=/tmp/moex-dataset.csv ENSEMBLE_OUT=ensemble_model.json python3 scripts/export_ensemble.py
go run ./cmd/trainmodel -config config.yaml             # резервная logreg + model.json
go run ./cmd/calibrate -config config.yaml -out report-calibration.md
go run ./cmd/leadlag -config config.yaml -out report-leadlag.md

# бэктест и отчёты
go run ./cmd/backtest -signal-source=ensemble -ensemble-path ensemble_model.json -commission-rate 0.0005
go run ./cmd/verifier -db trader.db -since 24h -interval 1h
```

Тесты:

```sh
go test ./...
go vet ./...
gofmt -l .
```

Внешние сервисы в тестах замоканы (`httptest`, in-memory fake-и, `miniredis`), сеть не
используется. Перед коммитом эти три команды обязательны.

## Отказоустойчивость: основной Mac + резервный ai-box

Трейдер работает на двух машинах, но **одновременно только на одной**: оба узла смотрят
в один sandbox-счёт. Арбитр — lease-witness на VPS (`cmd/witness`), узлы — `cmd/watchdog`:

- **Mac** (`watchdog.yaml`, launchd `com.example.moex-trader.watchdog`) — основной. Пока он
  жив, lease у него и трейдер работает на нём.
- **ai-box** (`user@<aibox-lan-ip>`, systemd user unit `moex-trader-watchdog`) — резервный.
  Подхватывает, только если Mac молчит дольше `failover_after` (10 минут) или его трейдер
  не стартует дольше `peer_error_grace` (2 минуты).
- **Witness** (`https://<witness-host>/witness`, FreeBSD VPS, rc.d `moex_witness`)
  выдаёт lease на 5 минут; узел без действующего lease обязан остановить трейдер (fencing).
  Если VPS недоступен дольше TTL — активный узел останавливается, резерв не подхватывает:
  торговли нет, пока witness не вернётся (осознанный fail-safe, а не сбой).
- Каждый узел отдаёт heartbeat (`GET /healthz`) и снапшот SQLite (`GET /db`, `VACUUM INTO`).
  Пассивный узел тянет БД каждые 15 с, поэтому на резерве позиции и kill switch свежие.
  Источник БД определяется не mtime, а тем, **чей трейдер запускался позже** (`trader.last_start_at`
  в heartbeat): свежесозданная пустая БД имеет mtime новее, но не должна затирать рабочую
  (это уже случалось). Принятая от пира БД помечается как синхронизированная, при старте
  своего трейдера узел снова становится её владельцем.
- **Возврат**: когда Mac снова в строю, он просит handover (`POST /control/yield`), резерв
  останавливает трейдер, отдаёт lease, Mac подтягивает финальную БД и стартует. Если
  трейдер Mac после этого падает 3 раза, lease отпускается и резерв подхватывает снова
  (`post_failure_cooldown`, 30 минут против пинг-понга).
- Telegram-алерты шлёт watchdog обеих машин (на ai-box — через свой ssh-туннель до VPS).

Деплой и диагностика:

```sh
./scripts/deploy-failover.sh witness     # VPS: сборка freebsd/amd64 + rc.d
./scripts/deploy-failover.sh aibox       # ai-box: бинарники, конфиги, CA, ключ туннеля
./scripts/deploy-failover.sh mac         # Mac: launchd-юнит + свежий trader
./scripts/deploy-failover.sh start-aibox # включить standby-юнит
./scripts/deploy-failover.sh status      # /healthz всех трёх узлов
```

Токены (`MOEX_WITNESS_TOKEN`, `MOEX_FAILOVER_TOKEN`) — в `watchdog.env` (0600, gitignored)
на обеих машинах и в `/usr/local/etc/moex-witness.token` на VPS. На ai-box для T-Bank нужен
российский корневой CA: `certs/ca-bundle.pem` (системные CA + `deploy/certs/russian-trusted-ca.pem`),
подключён через `SSL_CERT_FILE` в юните.

Шаблоны в `deploy/` содержат плейсхолдеры (`<aibox-lan-ip>`, `<mac-lan-ip>`, `<witness-host>`,
`<vps-host>`, `<telegram-chat-id>`) и `/Users/you/...` — замените их на свои адреса и пути
перед деплоем (или переопределите env-переменными `AIBOX`, `AIBOX_DIR`, `VPS` в
`scripts/deploy-failover.sh`).

Preflight-гейт работает на обоих узлах одинаково: если модель не проходит гейт, не торгует
никто — это правильное поведение, а не сбой failover.

## SDLC: как это менять и дорабатывать

### Зоны ответственности

Репозиторий ведут две агентские сессии параллельно (согласовано 2026-09-16), границы:

1. **Ensemble/model:** `internal/model/**`, `cmd/trainmodel`, `cmd/calibrate`,
   `cmd/exportdataset`, `internal/features/**`, `scripts/*.py`, выбор сигнал-сорса в
   `cmd/trader`.
2. **Broker/sandbox:** `internal/broker/tinkoff/**`, `internal/ingestion/tinkoff/**`,
   `cmd/sandboxcheck/**`, `config.sandbox.yaml`, sandbox-обвязка в `cmd/trader`
   (`newBrokerRuntime`), Telegram-алерты.

Внутри своей зоны сессия правит код и тесты сама; пересечения (общие интерфейсы,
`cmd/trader`) — через пользователя. Перед параллельной работой — `git status`: рабочее
дерево часто содержит незакоммиченные изменения второй сессии.

### Процесс: план → ralphex → revmux → completed

Доработки идут не «на глаз», а через план:

1. **План** — `docs/plans/YYYYMMDD-<тема>.md` со структурой `Overview` →
   `Context (from discovery)` → `Development Approach` → `Tasks` с чекбоксами
   `- [ ]`. Образцы: `docs/plans/20260915-commissions-and-finam-broker.md`,
   `docs/plans/completed/`.
2. **Исполнение** — `ralphex --tasks-only --max-iterations N <plan>`: агент берёт первую
   невыполненную секцию, реализует, гоняет `go test ./...` / `go vet` / `gofmt`, отмечает
   чекбоксы и коммитит (один коммит на задачу, conventional commits: `feat:`, `fix:`).
   Прогресс — `.ralphex/progress/progress-<plan>.txt`.
3. **Ревью** — ветка уходит в revmux (мультиагентная панель линз: bugs, architecture,
   tests, grounding и т.д.) в цикле «review → fix → commit → re-review» до раунда без
   gating-находок. Архив раундов — `.revmux/tasks/<task>/<NN-profile>/`.
4. **Финализация** — план переносится в `docs/plans/completed/`.

Исключение: мелкие правки (README, конфиг, одна функция) можно делать напрямую — но
инварианты ниже действуют всегда.

### Инварианты, которые нельзя нарушать

- **Деньги — только `decimal.Decimal`**, никогда `float64` (GO_LIVE_CHECKLIST #1).
- **Реальные деньги выключены.** `cmd/trader` отказывается стартовать с не-песочным
  счётом; включать реальные ордера только после полного прохождения
  `docs/GO_LIVE_CHECKLIST.md` и явного согласования.
- **LLM/Ollama в проекте не используется.** Сигнал строит только ансамбль моделей — не
  добавляй LLM обратно ни в критический путь, ни в бэктест.
- **Preflight использует тот же сигнал-сорс, что и лайв**, и обязан жёстко падать (а не
  «проходить с нулём»), если все решения ошиблись или истории нет.
- **Порядок признаков зеркалится в 4 местах** (`features.go`, `cmd/exportdataset`,
  `scripts/export_ensemble.py`, `scripts/compare_models.py`) — менять синхронно.
- **Авто-переобучение с атомарной подменой артефакта запрещено** без нового явного
  согласования (AUC ~0.5 — слишком близко к шуму). Feature-drift/PSI-диагностика и
  per-ticker circuit breaker — можно.
- **Kill switch отменяет только ордера своих тикеров** и никогда не трогает ручные ордера.
- **`SBMM` не торгуется через T-Invest API** (`api_trade_available_flag=false`) — не
  возвращать его в список ордеров.

### Рецепты

**Добавить признак.** (1) поле в `domain.FeatureContext`; (2) расчёт в
`internal/features`; (3) в `ToVector`/`defaultFeatureOrder`; (4) заголовок CSV в
`cmd/exportdataset`; (5) `FEATURES` в `scripts/export_ensemble.py` и
`scripts/compare_models.py`; (6) переобучить ансамбль и сравнить val AUC — прошлый опыт
(MACD/Stochastic/Williams/Alligator) дал ±0.005, то есть шум: сначала данные, потом фичи.

**Новый сигнал-сорс.** Реализовать оба интерфейса — `orchestrator.SignalSource`
(`Generate(ctx, feature) → BUY/SELL/HOLD`) и `backtest.SignalSource` — тогда он работает и
в лайве, и в бэктесте без правок executor/risk. Подключить в `newModelSignalSource`
(`cmd/trader`) и в `buildSignalSource` (`cmd/backtest`).

**Новый брокер.** Клиент в `internal/ingestion/<broker>`, исполнитель в
`internal/executor`, ветка в `newBrokerRuntime`, проверка в `cmd/sandboxcheck`, затем
пункты GO_LIVE_CHECKLIST (снапшот счёта, отмена ордеров, идемпотентность, комиссия).

**Переобучение модели.** Вручную: `cmd/exportdataset` → `scripts/export_ensemble.py` →
сравнить AUC/бэктест (`cmd/backtest -signal-source=ensemble`) → заменить
`ensemble_model.json` → рестарт `cmd/trader` (preflight перепроверит конфигурацию).
Никакого cron-автосвапа.

### Известные грабли

- XGBoost/LGBM-экспорт: только `save_model`/`save_raw(raw_format="json")`, никогда
  `dump_model()`; `base_score` — строка в скобках и вероятностное пространство (см.
  «Модель и признаки»).
- LGBM-конвертер: сначала положить `-1`-заглушку для ребёнка, потом перезаписать
  (`lc[i] = walk(...)`), иначе индексы плоского массива перепутаются.
- Finmarket RSS — Windows-1251, не UTF-8 (`charset.NewReaderLabel`).
- AlgoPack молча выключается одной строкой лога, если нет `MOEX_TRADER_ALGOPACK_TOKEN`.
- `cmd/backtest` по умолчанию комиссия `0.003` (устаревший тариф «Инвестор») — для
  реальных прогонов передавать `-commission-rate 0.0005` (тариф «Трейдер»).
- `internal/bus` (Redis) не подключён нигде; не искать в нём живую логику.

## Текущее качество модели и приоритеты

- Валидационный AUC всех трёх моделей ансамбля — **~0.50–0.52**, на грани шума. Сильный
  in-sample P&L рядом с отрицательным out-of-sample — симптом нехватки данных, а не баг.
- Узкое место — объём и широта данных, а не признаки. Приоритет: (1) расширение списка
  тикеров, (2) больше истории, (3) только потом новые признаки и тюнинг порогов.
- Тюнинг порогов walk-forward преждевременен, пока AUC ~0.5 — это подгонка под шум.
- Preflight-гейт уже отказывает в старте при убыточной конфигурации — это ожидаемое
  поведение, а не поломка.

## Лицензия

Apache License 2.0 — см. [LICENSE](LICENSE). Copyright 2026 Oleg Sidorkin.

Проект использует MOEX ISS, T-Invest API и другие внешние сервисы на условиях их
собственных лицензий; ничего из этого не является инвестиционной рекомендацией.
