> Перевод [GO_LIVE_CHECKLIST.md](GO_LIVE_CHECKLIST.md). При расхождении прав английский оригинал.

# Чек-лист перед запуском в бой

Этот чек-лист сопоставляет пять требований исходной спецификации к запуску в бой с реализацией
и тестами, которые их обеспечивают. Текущий статус проверен по репозиторию
на момент завершения Task 21.

## Требования исходной спецификации

| # | Требование | Реализация | Тесты |
|---|---|---|---|
| 1 | Все значения цен/денег используют `decimal.Decimal`; никогда `float64` | `internal/domain/types.go`, `internal/features/builder.go`, `internal/risk/gate.go`, `internal/executor/executor.go`, `internal/executor/live.go`, `internal/ingestion/moex`, `internal/ingestion/tinkoff`, `internal/ingestion/algopack`, `internal/verifier` | `internal/domain/types_test.go`, `internal/features/builder_test.go`, `internal/risk/gate_test.go`, `internal/executor/live_test.go` (граница decimal и проверки котировки `270.5`) |
| 2 | SQLite использует WAL и busy timeout 5000 мс | `internal/storage/storage.go` `buildDSN` добавляет `_journal_mode=WAL&_busy_timeout=5000`; DSN использует `modernc.org/sqlite` | `internal/storage/storage_test.go`: `TestOpenConfiguresWALBusyTimeoutAndSchema`, `TestConcurrentWALWrites` |
| 3 | Fat-finger проверка отклоняет цену ордера, отстоящую более чем на 2% от текущего лучшего bid/ask | `internal/risk/gate.go` `fatFingerOK` считает отклонение от bid/ask через `decimal.Decimal` и откатывается на предыдущее закрытие, когда живого bid/ask нет | `internal/risk/gate_test.go`: `TestHardenedGateFatFingerBoundary` покрывает ровно 2.00% против 2.01% выше/ниже; `TestHardenedGateFatFingerUsesPrevCloseFallback`; `TestHardenedGateFatFingerRejectsNonPositiveOrderPrice` |
| 4 | Живой исполнитель использует UUID v4 `OrderID` и идемпотентен | `internal/executor/live.go` генерирует `uuid.NewString`, разбирает и требует `uuid.Version() == 4`, и атомарно резервирует ID ордеров в полёте/отправленных; неисполненные ордера NEW записывают ноль исполненных лотов вместо фиктивного филла | `internal/executor/live_test.go`: `TestLiveExecutorPlacesOrder`, `TestLiveExecutorDuplicateOrderIDIsIdempotent`, `TestLiveExecutorNewOrderRecordsZeroLots` |
| 5 | Kill switch срабатывает при просадке >3% и блокирует новые сигналы до сброса | `internal/risk/gate.go` `exceedsDrawdown`, `triggerKillSwitch`, `TripKillSwitch`, `ResetKillSwitchContext`; `internal/storage/storage.go` сохраняет `kill_switch`; `cmd/trader` подключает гейт на базе хранилища и `-reset-kill-switch` | `internal/risk/gate_test.go`: `TestHardenedGateKillSwitchOnDrawdown`, `TestHardenedGateDrawdownBoundary`, `TestHardenedGateManualKillSwitch`, `TestHardenedGatePersistsAndReadsKillSwitch`, `TestHardenedGateAlertsOnKillSwitch`; `internal/orchestrator/orchestrator_test.go`: `TestOrchestratorKillSwitchBlocksNextCycle` |

## Проверка реализации фаз 1-4

- Фаза 1: конфиг, домен, хранилище SQLite, ingest MOEX/новостей, построитель признаков, paper-исполнитель,
  заглушка риск-гейта и цикл оркестрации реализованы и протестированы.
- Фаза 3: клиент рыночных данных Tinkoff, обогащение AlgoPack, подключённое к оркестратору
  как асинхронный воркер на цикл, шина Redis Streams, метрики Prometheus и алерты Telegram
  реализованы и протестированы.
- Фаза 4: усиленный риск-гейт, идемпотентный живой исполнитель, сохраняемый kill switch и
  агент-верификатор реализованы и протестированы. Работающий
  `cmd/trader` поддерживает paper-исполнение и исполнение в песочнице Tinkoff; режим
  реальных денег по-прежнему отклоняется, пока не проверены снимок счёта, отмена ордеров и
  сверка, специфичная для брокера.

## Проверка в песочнице

- Исполнение в песочнице T-Банка подключено через `internal/broker/tinkoff` (начальная
  настройка счёта, снимок портфеля для риск-гейта, отмена открытых ордеров) и
  `cmd/trader` (`broker: tinkoff`, `is_paper_trading: false`, `tinkoff.sandbox: true`).
- Песочница берёт плоскую комиссию 0.05% и исполняет рыночные ордера по цене последней
  сделки; задайте `commission.rate: "0.0005"`, чтобы записанный P&L совпадал с песочницей.
- Перед реальными деньгами: повторить учения по kill switch на счёте песочницы,
  убедиться, что снимок портфеля песочницы совпадает с собственным представлением стратегии о позициях,
  и проверить отмену ордеров при срабатывании kill switch.

## Проверки перед запуском, специфичные для брокера

- Убедиться, что настроенный `commission.rate` совпадает с реальным договорным тарифом
  выбранного брокера, и обновить ставку, если демо- или живой филл покажет иное.

## Результаты валидации

- `go test -count=1 ./...` проходит для всех пакетов.
- `go vet ./...` не сообщает замечаний.
- `gofmt -l .` не сообщает неотформатированных Go-файлов.
- Покрытие критичных для запуска в бой пакетов приемлемо: `internal/risk` 84.9%,
  `internal/executor` 73.8%.
