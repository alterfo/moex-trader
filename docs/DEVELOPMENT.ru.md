> Перевод [DEVELOPMENT.md](DEVELOPMENT.md). При расхождении прав английский оригинал.

# Руководство по разработке и эксплуатации

Русский перевод: [DEVELOPMENT.ru.md](DEVELOPMENT.ru.md). Этот файл (английский) — канонический.

Как запускать, настраивать, тестировать и менять проект. Структура системы описана в [ARCHITECTURE.md](ARCHITECTURE.md); правила и инварианты — в [../AGENTS.md](../AGENTS.md).

## 1. Команды

```sh
go build ./...

go run ./cmd/trader -config config.yaml                 # paper
go run ./cmd/trader -config config.sandbox.yaml         # T-Bank sandbox
go run ./cmd/trader -reset-kill-switch                  # reset the kill switch and exit
go run ./cmd/sandboxcheck -config config.sandbox.yaml [-ticker SBER] [-place-order]

go run ./cmd/exportdataset -config config.yaml -from 2024-09-01 -split 2026-06-18 -out /tmp/moex-dataset.csv
DATASET=/tmp/moex-dataset.csv ENSEMBLE_OUT=ensemble_model.json python3 scripts/export_ensemble.py
go run ./cmd/trainmodel -config config.yaml             # fallback logistic regression -> model.json
go run ./cmd/calibrate -config config.yaml -out report-calibration.md
go run ./cmd/listtqbr -out data/train_tickers.json

go run ./cmd/backtest -signal-source=ensemble -ensemble-path ensemble_model.json
go run ./cmd/verifier -db trader.db -since 24h -interval 1h

go run ./cmd/tailprecision -config config.sandbox.yaml -out report-tailprecision.md
go run ./cmd/momentumbench -config config.sandbox.yaml -out report-momentumbench.md
go run ./cmd/betaregime -config config.sandbox.yaml -out report-betaregime.md
go run ./cmd/gapstress -config config.sandbox.yaml -portfolio artifacts/portfolio.json
go run ./cmd/borrowmeasure -config config.sandbox.yaml -out report-borrow.md
go run ./cmd/oosfreeze -action status
go run ./cmd/strategyvalidation
```

Тесты и линт: `make check` (`go test ./...` + `go vet ./...`) и `gofmt -l .`. CI (`.github/workflows/ci.yml`) запускает `make check` на каждый push и PR с версией Go из `go.mod`. Внешние сервисы в тестах замоканы (`httptest`, in-memory-фейки); тесты никогда не ходят в сеть. `make check` и `gofmt -l .` обязательны перед коммитом. `internal/docscheck` валит сборку, когда у документа нет двойника `.ru.md`, английский документ содержит кириллицу, пакета нет в `docs/ARCHITECTURE*.md` или сломана относительная ссылка.

## 2. Вектор признаков

Порядок задан один раз в `internal/model/features.go` (`defaultFeatureOrder`, `ToVector`): 28 имён, модели обучаются на 18 из них.

```
return_pct, realized_volatility, news_sentiment, news_count, order_book_imbalance,
mom_5d, mom_21d, mom_63d, reversal_1d, rsi_14, dist_ma20_pct, dist_ma50_pct,
realized_vol_21d_annualized_pct, volume_zscore_20d,
macd_hist_pct, stoch_k_14, williams_r_14, alligator_spread_pct,
event_dividend, event_buyback, event_sanctions, event_ipo, event_report,
event_delisting, event_mna, event_default, negotiations_signal, sanctions_signal
```

Осцилляторы считаются в `internal/features/price_features.go`. Порядок обязан совпадать в трёх местах: `internal/model/features.go`, `FEATURES` в `scripts/export_ensemble.py` и в `scripts/compare_models.py`; `cmd/exportdataset` строит заголовок и строки CSV из `model.ToVector`. Рассинхрон — жёсткий отказ при загрузке модели (`checkFeatureOrder`) и в preflight. В развёрнутом `ensemble_model.json` коэффициенты `event_*`, `negotiations_signal` и `sanctions_signal` нулевые. Инварианты инференса (float32 для XGBoost, обработка `base_score`): [knowledge/feature-pipeline-and-inference.md](knowledge/feature-pipeline-and-inference.md).

## 3. Модели

- **Таблица оценок** (`model.score_table_path`, боевая в профиле песочницы): дневная вероятность по тикеру из `data/ridge_scores.csv` (`ticker,date,p`), строится офлайн `scripts/ridge_probs.py`; на Mac забирается через `scripts/ridge-scores-pull.sh`. Максимальная устарелость 7 дней; BUY при `p >= 0.60`, SELL при `p <= 0.40`.
- **Ансамбль** (`model.ensemble_path`): LightGBM + XGBoost + логистическая регрессия, `p = (p_lgb + p_xgb + p_lr) / 3`, пороги в артефакте (`0.60`/`0.40`), `Confidence = |p - 0.5| * 2`. Обучение: `cmd/exportdataset` (метка = избыточная доходность над IMOEX, горизонт 5 дней, мёртвая зона 0.5%; `-news-history` подмешивает реальные новостные признаки) → `scripts/export_ensemble.py` → `ensemble_model.json`; сравнение через `scripts/compare_models.py`.
- **Запасной вариант** (`model.path`): объединённая логистическая регрессия от `cmd/trainmodel` (чистый Go).
- **Challenger**: `model.challenger_ensemble_path` оценивает каждый вектор в тени; см. AGENTS.md.

Диагностика, которая никогда не касается торгового пути: `cmd/calibrate`, `cmd/leadlag`, `cmd/tailprecision`, `cmd/momentumbench`, `cmd/betaregime`, `cmd/gapstress`, `cmd/borrowmeasure`, `cmd/oosfreeze`, `cmd/strategyvalidation`, `cmd/newsfetch`.

## 4. Бэктест и preflight

`internal/backtest/engine.go` — это живой путь без сети:

- дневные свечи MOEX ISS (`ISSSource`, корректный пейджинг; прогрев покрывает полный 300-барный lookback признаков, 434 календарных дня, чтобы индикаторы EMA/SMMA сходились так же, как в лайве и при обучении);
- на дне `d` признаки строятся из свечей `[:d]`, вход по `candles[d+1].Open`, mark-to-market по `candles[d].Close`;
- сигнал проходит тот же `risk.HardenedGate`; комиссия берётся на входе и выходе;
- результат: net/gross P&L, комиссии, hit rate, Sharpe, максимальная просадка, число решений, причины hold (включая `error`/`timeout`), разбивка realized/unrealized, стоимость займа, атрибуция по тикерам.

`cmd/backtest -signal-source`: `model` (по умолчанию, `model.json`), `ensemble` (нужен `-ensemble-path`), `rule` (`-reversal-threshold`), `csvprob` (вероятности из CSV). Полезные флаги: `-commission-rate` (по умолчанию `0.0005`), `-lookback-days`, `-min-confidence`, `-kill-switch`, `-cache`, `-out`, `-tearsheet`, `-spread-pct`/`-slippage-pct`, `-spread-db`/`-spread-min-obs`, `-borrow-pct-day`, `-max-net-exposure`/`-max-net-short`/`-max-net-long`, `-sector-caps`/`-sectors`, `-wf-dir`, `-ensemble-member lgb|xgb|logistic`. `-members-csv <path>` (вместе с `-signal-source ensemble`) пишет по строке на закрытую сделку с вероятностями членов ансамбля на решении, открывшем её, плюс `<path>.meta.json` со счётчиком несопоставленных; `-members-window` и `-members-seed` размечают строки, а `cmd/memberagree -in <file|dir> [-out report.md]` собирает отчёт о согласии членов.

Прочие флаги: `-embargo-bars N` (`cmd/walkforward`, `cmd/backtest -wf-dir`, `cmd/trainmodel`); `cmd/strategyvalidation -returns-dirs a,b,c` (PBO), `-reliability <wf-dir>`, `-consistency <wf-dir>`; `-label-mode absolute_tr` вместе с `-dividends` (`cmd/exportdataset`, `cmd/trainmodel`); `-train-tickers`/`-train-min-turnover` (`cmd/exportdataset`); `-vol-scale*` (`cmd/backtest`, `cmd/walkforward`; в `cmd/walkforward` только вместе с `-target-notional`).

Preflight в `cmd/trader` запускает тот же движок на последних `preflight.days` (90) днях с `preflight.deposit`, `preflight.spread_pct`/`preflight.slippage_pct` (по умолчанию по 0.05%) и базовым живым источником сигнала. Порог применяется к **реализованному** P&L. Условия отказа перечислены в [ARCHITECTURE.md](ARCHITECTURE.md), раздел 2.4.

## 5. Профили исполнения

**Paper** (`broker: paper`, `is_paper_trading: true`, по умолчанию): виртуальные исполнения с комиссией `price x lots x commission.rate`; снимка счёта нет, поэтому проверки дневного убытка и просадки выключены; лимит лотов и fat-finger остаются включёнными.

**Песочница T-Bank** (`broker: tinkoff`, `is_paper_trading: false`, `tinkoff.sandbox: true`) — рабочий контур. Шаблон: `config.sandbox.yaml`.

```yaml
broker: "tinkoff"
is_paper_trading: false
commission:
  rate: "0.0005"
tinkoff:
  endpoint: "sandbox-invest-public-api.tbank.ru:443"
  sandbox: true
  account_id: ""
  pay_in: "1000000"
  order_type: "market"
```

- `EnsureAccount` переиспользует первый счёт или открывает новый и пополняет его на `pay_in`; закрепи id в `tinkoff.account_id`.
- `Snapshot` даёт гейту `Deposit = pay_in`, `DayStartEquity` и `CurrentEquity`.
- Ордера идемпотентны (UUID v4 `OrderID`); `market` исполняется по последней цене.
- Kill switch отменяет только ордера по инструментам бота (`TestCancelOpenOrdersLeavesForeignOrdersAlone`).
- Песочница не начисляет дивиденды и налоги и не имеет реального проскальзывания; P&L приблизительный.
- Текущая позиция бота выводится из его собственных событий executor; ручные позиции на том же счёте ему не видны (SELL там открывает шорт).

**Live**: намеренно не подключён. См. [GO_LIVE_CHECKLIST.md](GO_LIVE_CHECKLIST.md).

Значения риска по умолчанию (`risk.DefaultConfig()`): дневной убыток 0.5% депозита, fat-finger 2%, просадка 3% до kill switch. Breaker по тикеру (`risk.circuit_breaker_max_losses`, `_max_loss_pct`), PSI-дрейф (`risk.drift_psi_*`, только предупреждение), гистерезис ребалансировки (`risk.rebalance_min_deviation_pct`).

## 6. Хранилище и наблюдаемость

SQLite (`modernc.org/sqlite`, WAL, busy timeout 5000 мс) по пути `storage.path`: `audit_events` (JSON стадий), `trade_signals`, `kill_switch`. Prometheus на `:9090` (`-metrics-addr`); каталог метрик в [metrics.md](metrics.md). Telegram: ошибки сигнала, kill switch, исполненные решения (фильтр `telegram.signal_tickers`), cooldown-алерты (`internal/alert/telegram/watch.go`). Из этой сети Telegram требует `telegram.proxy` (`socks5://127.0.0.1:3333`).

## 7. Конфигурация

YAML плюс `.env` рядом (реальные переменные окружения приоритетнее). Секреты только из окружения. Шаблоны: `config.example.yaml`, `config.paper.yaml`, `config.sandbox.yaml`; `config.yaml` и `.env` в gitignore; имена переменных в `.env.example`.

| Field | Env |
|---|---|
| `tickers` | `MOEX_TRADER_TICKERS` (через запятую) |
| `model.path` | `MOEX_TRADER_MODEL_PATH` |
| `model.ensemble_path`, `model.score_table_path`, `model.challenger_*` | только YAML |
| `news.classifier_path` | `MOEX_TRADER_NEWS_CLASSIFIER_PATH` |
| `moex_iss_base_url` | `MOEX_TRADER_MOEX_ISS_URL` |
| `algopack_base_url` / `algopack_token` | `MOEX_TRADER_ALGOPACK_BASE_URL` / `_TOKEN` (секрет) |
| `storage.path` | `MOEX_TRADER_STORAGE_PATH` |
| `risk.max_lots`, `risk.target_notional`, `risk.rebalance_min_deviation_pct` | `MOEX_TRADER_RISK_MAX_LOTS`, `_TARGET_NOTIONAL`, `_REBALANCE_MIN_DEVIATION_PCT` |
| `risk.circuit_breaker_*`, `risk.drift_psi_*`, `risk.max_slippage_pct`, `risk.no_trade_after_open_minutes`, `risk.blackout_windows`, `risk.max_net_*`, `risk.vol_scale`, `risk.sector_caps`, `risk.sectors` | только YAML |
| `commission.broker` / `rate` | `MOEX_TRADER_COMMISSION_BROKER` / `_RATE` |
| `tinkoff.endpoint` / `token` / `sandbox` / `account_id` / `pay_in` / `order_type` | `MOEX_TRADER_TINKOFF_*` (`_TOKEN` — секрет) |
| `finam.base_url` / `secret_token` | `MOEX_TRADER_FINAM_SECRET_TOKEN` (секрет) |
| `preflight.enabled` / `days` / `deposit` / `min_net_pnl` | `MOEX_TRADER_PREFLIGHT_*` |
| `preflight.min_closed_trades` / `spread_pct` / `slippage_pct` | только YAML |
| `broker` (`paper` / `tinkoff` / `finam`) | `MOEX_TRADER_BROKER` |
| `poll_interval`, `is_paper_trading` | `MOEX_TRADER_POLL_INTERVAL`, `MOEX_TRADER_IS_PAPER_TRADING` |
| `telegram.bot_token` / `chat_id` / `signal_tickers` / `proxy` | `MOEX_TRADER_TELEGRAM_*` (`_BOT_TOKEN` — секрет) |

`alerts.*` (`probability_collapse_window`, `stale_candle_age`, `cooldown`) имеет активные значения по умолчанию (`20`, `24h`, `1h`) и молчит только без учётных данных Telegram. Если `news.classifier_path` пуст или файл не загрузился, трейдер пишет предупреждение и откатывается на лексикон ключевых слов.

## 8. Эксплуатация отказоустойчивости

Топология и правила: [ARCHITECTURE.md](ARCHITECTURE.md), раздел 3, и AGENTS.md.

```sh
./scripts/deploy-failover.sh witness     # VPS: freebsd/amd64 build + rc.d
./scripts/deploy-failover.sh aibox       # ai-box: binaries, configs, CA, tunnel key
./scripts/deploy-failover.sh mac         # Mac: launchd unit + fresh trader
./scripts/deploy-failover.sh start-aibox # enable the standby unit
./scripts/deploy-failover.sh status      # /healthz of all three nodes
```

Токены `MOEX_WITNESS_TOKEN` и `MOEX_FAILOVER_TOKEN` лежат в `watchdog.env` (0600, в gitignore) на обеих машинах и в `/usr/local/etc/moex-witness.token` на VPS. ai-box нужен российский корневой CA: `certs/ca-bundle.pem` (системные CA + `deploy/certs/russian-trusted-ca.pem`) через `SSL_CERT_FILE`. Шаблоны в `deploy/` содержат плейсхолдеры (`<aibox-lan-ip>`, `<mac-lan-ip>`, `<witness-host>`, `<vps-host>`, `<telegram-chat-id>`) и `/Users/you/...`; замени их или переопредели переменными окружения `AIBOX`, `AIBOX_DIR`, `VPS`. Preflight-гейт ведёт себя одинаково на обоих узлах: если модель не проходит гейт, не торгует никто — это правильное поведение, а не сбой failover.

## 9. Процесс изменений

Планы лежат в `docs/plans/YYYYMMDD-<topic>.md` (`Overview` → `Context` → `Development Approach` → `Tasks` с `- [ ]`); примеры в `docs/plans/completed/`. Исполнение: `ralphex --tasks-only --max-iterations N <plan>` (один conventional-коммит на задачу). Ревью: цикл review-fix в revmux до раунда без блокирующих находок. Финализация: план переносится в `docs/plans/completed/` в том же коммите, который вносит его числа в `docs/metrics.md`. Мелкие правки (документы, конфигурация, одна функция) можно делать напрямую; инварианты из AGENTS.md действуют всегда.

Каждое изменение обновляет документацию в том же коммите: сначала английский `X.md`, затем его `X.ru.md`; структурные изменения кода обновляют `docs/ARCHITECTURE.md` и `docs/ARCHITECTURE.ru.md`.

### Рецепты

**Добавить признак.** (1) поле в `domain.FeatureContext`; (2) расчёт в `internal/features`; (3) `ToVector`/`defaultFeatureOrder`; (4) заголовок CSV следует за `ToVector` автоматически; (5) `FEATURES` в `scripts/export_ensemble.py` и `scripts/compare_models.py`; (6) переобучить и сравнить validation AUC (прошлые признаки-осцилляторы сдвигали AUC на +-0.005, то есть шум: сначала данные, потом признаки).

**Новый источник сигнала.** Реализовать и `orchestrator.SignalSource` (`Generate(ctx, feature)` → BUY/SELL/HOLD), и `backtest.SignalSource`; подключить в `newModelSignalSource` (`cmd/trader`) и `buildSignalSource` (`cmd/backtest`). Preflight должен использовать тот же источник.

**Новый брокер.** Клиент в `internal/ingestion/<broker>`, исполнитель в `internal/executor`, ветка в `newBrokerRuntime`, проверка в `cmd/sandboxcheck`, затем пункты GO_LIVE_CHECKLIST (снимок счёта, отмена ордеров, идемпотентность, комиссия).

**Переобучить модель.** Только вручную: экспорт → обучение → сравнение AUC и бэктеста → замена артефакта → перезапуск `cmd/trader` (preflight перепроверит). Шаги: [plans/completed/20261018-manual-retrain.md](plans/completed/20261018-manual-retrain.md). Никакой автоподмены по cron.

### Известные грабли

- Экспорт XGBoost/LGBM: только `save_model`/`save_raw(raw_format="json")`, никогда `dump_model()`.
- Конвертер LGBM: сначала положить заглушку дочернего узла `-1`, затем перезаписать (`lc[i] = walk(...)`), иначе индексы плоского массива перепутаются.
- Finmarket RSS в Windows-1251 (`charset.NewReaderLabel`).
- AlgoPack молча отключается одной строкой лога, если нет `MOEX_TRADER_ALGOPACK_TOKEN`.
- `SBMM` не торгуется через T-Invest API (`api_trade_available_flag=false`).
- `internal/bus` нигде не подключён; не ищи в нём живую логику.
- ISS ограничивает страницы 500 строками: постраничные загрузчики должны останавливаться на пустой странице, а не на короткой.

## 10. Качество модели в одном абзаце

Validation AUC всех членов ансамбля около 0.50-0.52, на грани шума; узкое место — объём и широта данных, а не признаки. Порядок приоритетов: больше тикеров, больше истории, и только потом признаки и пороги. Текущие числа и вердикты: [metrics.md](metrics.md); история отвергнутых идей: [knowledge/model-quality-history.md](knowledge/model-quality-history.md).
