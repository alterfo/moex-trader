# Правила проекта

Перевод [AGENTS.md](AGENTS.md); при расхождении прав английский текст. Направление: `docs/VISION.md` (читать первым); инварианты ниже важнее любого плана.

## Владение (две сессии на одном дереве)

1. **Зона модели:** ансамбль lgbm+xgb+logreg (buy/sell 0.60/0.40) и Go-инференс, `internal/model/**`, `internal/features/**`, `cmd/trainmodel|calibrate|exportdataset`, `scripts/*.py`, выбор источника сигнала в `cmd/trader`.
2. **Зона брокера/песочницы:** `internal/broker/tinkoff/**`, `internal/ingestion/tinkoff/**`, `cmd/sandboxcheck/**`, `config.sandbox.yaml`, `newBrokerRuntime`, алерты в Telegram.
3. **Зона отказоустойчивости:** `cmd/watchdog|witness`, `internal/failover/**`, `deploy/**`, `scripts/deploy-failover.sh`, `watchdog.*`. Перед правкой выборов запустить `go test ./internal/failover/ ./cmd/witness/ ./cmd/watchdog/`.
4. Источник сигнала реализует и `orchestrator.SignalSource`, и `backtest.SignalSource`.
5. `cmd/trader` запускает preflight-бэктест (`preflight.*`) с тем же источником сигнала, что и живой цикл, и отказывается стартовать при отрицательной реализованной P&L, малом числе закрытых сделок или отсутствии истории; не обходить.
6. Общение между сессиями идёт через пользователя (или agterm/peer-chat); без headless `claude -p` (403).
7. **Никаких параллельных правок кода.** Пишет один; сначала `git status`/`git diff`, коммитить быстро.
8. **Изменения с метриками коммитятся сразу.** Все числа бэктестов и анализа лежат в `docs/metrics.md` (индекс) и `docs/metrics/*.md`; правятся в том же коммите. Метрики читать из `HEAD` или файла, названного пользователем.

## Жёсткие инварианты

- Деньги — `decimal.Decimal`, не `float64`.
- Реальное исполнение выключено: `cmd/trader` отказывается работать с не-песочничным счётом. Реальные ордера только с явным разрешением и полным прохождением `docs/GO_LIVE_CHECKLIST.md`.
- Автоматической горячей подмены живой модели нет (отказ 2026-09-16). Ежедневное переобучение КАНДИДАТА разрешено (2026-10-18); подмена ручная; ни одна задача не пишет `ensemble_model.json`, не перезапускает `./trader` и не запускает `deploy-failover.sh`. Подробности: `docs/knowledge/validation-and-ops.md`.
- `cmd/trader` запускает только один узел: право торговать — аренда от VPS-свидетеля (`cmd/witness`). Терять аренду — останавливаться; без аренды и вручную на резервном узле трейдер не запускать.
- Не выбирать `trader-sandbox.db` по mtime; узел, не стартовавший `trader.max_failures` раз, остаётся в `error` на время cooldown (подробности в том же файле).

## Документация

- Каноничен английский. Русские близнецы (`X.ru.md`) есть только у `AGENTS`, `README`, `docs/VISION`, `docs/ARCHITECTURE`, `docs/DEVELOPMENT`, `docs/GO_LIVE_CHECKLIST` (решение владельца 2026-10-23); обновлять в том же коммите, что и английский файл. Агенты читают и правят только английский. Всё остальное, включая планы, только на английском.
- **Закон архитектуры:** коммит, добавляющий, удаляющий или переименовывающий пакет `cmd/*` или `internal/*`, либо меняющий живой цикл, поток данных, ключи конфигурации или интеграцию, обновляет `docs/ARCHITECTURE.md` и `.ru.md`. `internal/docscheck` (в `make check`) проверяет покрытие, близнецов, английскость файлов и ссылки.
- Документы короткие: таблицы «результат и вердикт», без повествования; числа только в `docs/metrics.md` и `docs/metrics/*.md`.

## Индекс знаний (читать ДО правки области)

| Область | Читать |
|---|---|
| пакеты, живой цикл, топология развёртывания | `docs/ARCHITECTURE.md` |
| команды, ключи конфигурации, флаги бэктеста | `docs/DEVELOPMENT.md` |
| порядок признаков, `ToVector`, экспорт модели, Go-инференс | `docs/knowledge/feature-pipeline-and-inference.md` |
| новости, классификатор, детекторы событий | `docs/knowledge/news-and-events.md` |
| признаки, метки, горизонты, пороги, переобучение | `docs/knowledge/model-quality-history.md` |
| плечо, экспозиция, дивиденды, хедж | `docs/knowledge/income-levers.md` |
| валидация, shadow-сверка, алерты, kill switch, challenger, задача переобучения | `docs/knowledge/validation-and-ops.md` |
| оптимизация настроенных параметров (Arnold guard `scripts/hooks/arnold-guard.sh`) | `docs/knowledge/soft-models-and-optimization.md` |
| источники данных и AlgoPack | `docs/knowledge/data-sources.md` |
| любое число | индекс `docs/metrics.md`, затем один `docs/metrics/s*.md` |

## Уроки

- Инцидент, выразимый поведением кода, получает регрессионный тест в том же коммите; прозаические уроки идут в нужный файл `docs/knowledge/`, не сюда.
- Ревьюер судит по независимым доказательствам (закоммиченные метрики, вывод тестов), а не по словам автора «готово».
