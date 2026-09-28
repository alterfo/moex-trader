# MOEX Pulse — realtime edge→cloud слой сигналов (Yandex Cloud)

## Overview

Cloud/distribution-слой поверх существующего `moex-trader`: раздаёт в realtime по WebSocket
поток «новость → сентимент → тикер» + торговые сигналы BUY/SELL/HOLD, плюс архив и дашборды.

**Проблема, которую решает.** Основная цель — глубоко освоить Yandex Cloud и пройти
сертификацию *Certified Engineer Associate* (6 доменов: инфраструктура, данные, DevOps,
serverless, безопасность, биллинг). Строим живой highload-продукт, которым автор пользуется
сам (он торгует на MOEX), — это эффективнее сухого изучения сервисов.

**Как интегрируется.** Тяжёлый инференс остаётся на домашнем GPU-боксе `192.168.88.193`
(Ollama-сентимент + ML-ансамбль — всё уже в `moex-trader`). Облако — это plane приёма,
хранения, realtime-раздачи и наблюдаемости. Новый edge-паблишер (команда в этом же репо)
читает выход существующих internal-пакетов и проталкивает готовые события в облако.

**Ключевое решение:** проект живёт внутри `moex-trader`, а не отдельным репозиторием —
ISS-клиент (`internal/ingestion`), шина (`internal/bus`), сигнал-ансамбль
(`internal/orchestrator`, `internal/model`), новостной сентимент (`cmd/newsscore` и
связанные) уже здесь. Cloud-слой переиспользует их напрямую, без кросс-репо контрактов.

**Масштаб (по решению автора):** этот план = тонкий serverless-MVP (Functions + API Gateway
+ YDB + Object Storage). Highload-слой (Kafka, ClickHouse, k8s+автоскейл, load-тесты) —
отдельный следующий план, см. Post-Completion.

## Context (from discovery)

- **Переиспользуемые пакеты (доноры, тот же Go-модуль):**
  - `internal/ingestion` — клиент MOEX ISS API.
  - `internal/orchestrator` + `internal/model` — формирование сигнала BUY/SELL/HOLD.
  - `cmd/newsscore` / `cmd/newsfetch` / `internal/*` новостей — сентимент по тикеру.
  - `internal/bus` — паттерн Redis Streams (пригодится в Фазе 2).
  - `internal/metrics` — Prometheus-метрики.
  - `internal/config` — конфигурация.
  - `deploy/*.plist` — launchd-паттерн для запуска edge-паблишера на `.193`.
- **Инфраструктурный шаблон (только infra, не доменный код):**
  `/Users/olegsidorkin/dev/system/fincontext-mcp/infra/*.tf` (Functions+API Gateway+YDB+
  Lockbox), `src/sigv4.js` (SigV4-подписант для YMQ/YDB API), `src/lockbox.js` (паттерн Lockbox).
- **Соглашения репо:** планы в `docs/plans/YYYYMMDD-*.md`, завершённые уезжают в
  `docs/plans/completed/`; ralphex/revmux-скаффолдинг присутствует.
- **Новые директории проекта:** `cmd/pulseedge/`, `cloud/functions/{ingest,quotes,ws}/`,
  `cloud/infra/`, `cloud/web/`, `internal/pulse/{event,sigv4,transport}/`.

## Development Approach

- **Testing approach:** Regular (сначала реализация, затем тесты — как в остальном `moex-trader`).
- Каждую задачу доводить до конца перед следующей; маленькие сфокусированные изменения.
- **CRITICAL: каждая задача с кодом ОБЯЗАНА включать новые/обновлённые тесты** (success +
  error/edge сценарии), тесты — отдельные пункты чек-листа, не в связке с реализацией.
- **CRITICAL: все тесты проходят перед стартом следующей задачи.**
- Terraform-задачи вместо unit-тестов используют `terraform fmt -check` + `terraform validate`.
- **CRITICAL: обновлять этот план при изменении скоупа.**
- Никаких комментариев в коде (глобальное правило автора). Язык нового кода — Go.

## Testing Strategy

- **Unit-тесты:** обязательны для каждой кодовой задачи (Go `testing`, table-driven).
- **Инфра-«тесты»:** `terraform fmt -check` + `terraform validate` для `cloud/infra/`.
- **E2E:** проект без UI-e2e-фреймворка; сквозная проверка бокс→облако→браузер выполняется
  вручную против YC-sandbox — вынесена в Post-Completion (требует облачных кредов).
- Все тесты должны проходить перед переходом к следующей задаче.

## Progress Tracking

- Отмечать выполненное `[x]` сразу.
- Новые задачи — с префиксом ➕. Блокеры — с префиксом ⚠️.
- Держать план в синхроне с фактической работой; при отклонении скоупа — править план.

## What Goes Where

- **Implementation Steps** (`[ ]`): всё, что агент делает в этом репо — Go-код, тесты,
  Terraform-файлы (`validate`), статичный клиент, документация.
- **Post-Completion** (без чекбоксов): реальный `terraform apply` в YC, деплой Functions,
  сквозной E2E против облака, Фаза 2 (highload) как следующий план.

## Implementation Steps

### Task 1: Доменная модель события (internal/pulse/event)
- [ ] создать пакет `internal/pulse/event` со структурой `Event` (ticker, sentiment score/label,
      signal BUY/SELL/HOLD, source, produced_at, staleness), константы источников
- [ ] реализовать JSON-сериализацию/десериализацию и валидацию (обязательные поля, диапазоны)
- [ ] реализовать детерминированный `ID()` события (sha256 от tuple ticker+produced_at+source)
- [ ] написать тесты сериализации и валидации (success)
- [ ] написать тесты ошибок (пустой тикер, битый label, будущий timestamp)
- [ ] прогнать тесты — должны пройти до Task 2

### Task 2: SigV4-подписант запросов в облако (internal/pulse/sigv4)
- [ ] создать пакет `internal/pulse/sigv4`, портировать логику подписи из
      `fincontext-mcp/src/sigv4.js` на Go (canonical request, string-to-sign, signing key)
- [ ] реализовать `Sign(req, credentials, region, service)` для HTTP-запросов
- [ ] написать тесты по known-answer векторам (совпадение подписи с эталоном)
- [ ] написать тесты ошибок (отсутствующие креды, пустой payload)
- [ ] прогнать тесты — должны пройти до Task 3

### Task 3: HTTP-транспорт в облако (internal/pulse/transport)
- [ ] создать пакет `internal/pulse/transport` — клиент, отправляющий `Event` на облачный
      ingest-endpoint (API Gateway), с подписью из Task 2 и ключом из конфига
- [ ] добавить ретраи с backoff и таймаутами; интерфейс для мока в тестах
- [ ] написать тесты успешной отправки (мок-сервер) и ретраев
- [ ] написать тесты ошибок (5xx, таймаут, отказ подписи)
- [ ] прогнать тесты — должны пройти до Task 4

### Task 4: Edge-паблишер (cmd/pulseedge)
- [ ] создать команду `cmd/pulseedge`, читающую сигнал из `internal/orchestrator`/`internal/model`
      и сентимент из новостного пайплайна (`cmd/newsscore`/связанные internal-пакеты)
- [ ] собрать `Event` и отправить через `internal/pulse/transport`; конфиг через `internal/config`
- [ ] добавить Prometheus-метрики через `internal/metrics` (кол-во/латентность/ошибки публикаций)
- [ ] добавить launchd-plist в `deploy/` по образцу существующих (запуск на `.193`)
- [ ] написать тесты сборки события из сигнала+сентимента (success + отсутствие данных)
- [ ] прогнать тесты — должны пройти до Task 5

### Task 5: Cloud Function ingest (cloud/functions/ingest)
- [ ] создать Go-функцию `cloud/functions/ingest`: проверка подписи, валидация `Event`
- [ ] запись последнего состояния по тикеру в YDB; сырой архив события в Object Storage
- [ ] интерфейсы хранилищ (YDB/S3) для мока в тестах
- [ ] написать тесты хендлера (валидное событие → запись; невалидная подпись → 401)
- [ ] написать тесты ошибок (битый payload, недоступность хранилища)
- [ ] прогнать тесты — должны пройти до Task 6

### Task 6: Cloud Function quotes-poller (cloud/functions/quotes)
- [ ] создать Go-функцию `cloud/functions/quotes` для timer-триггера: тянет котировки MOEX ISS
      через `internal/ingestion`, пишет свежесть в YDB
- [ ] обработка задержки/пустого ответа ISS, идемпотентность записи
- [ ] написать тесты парсинга ISS-ответа и записи (success)
- [ ] написать тесты ошибок (пустой/битый ответ ISS, ошибка YDB)
- [ ] прогнать тесты — должны пройти до Task 7

### Task 7: Cloud Function WebSocket-раздача (cloud/functions/ws)
- [ ] создать Go-функцию `cloud/functions/ws` для API Gateway WebSocket:
      connect/message/disconnect, реестр коннектов в YDB
- [ ] broadcast нового события подключённым клиентам (fanout через реестр)
- [ ] написать тесты жизненного цикла коннекта (connect→register, disconnect→cleanup)
- [ ] написать тесты broadcast (событие рассылается активным, пропускает мёртвые)
- [ ] прогнать тесты — должны пройти до Task 8

### Task 8: Terraform-инфраструктура MVP (cloud/infra)
- [ ] описать least-privilege сервис-аккаунт + IAM-роли, VPC
- [ ] описать YDB serverless, Object Storage bucket (архив + статика), KMS-ключ шифрования
- [ ] описать API Gateway (HTTP ingest + WebSocket), три Cloud Functions, timer-триггер quotes
- [ ] описать Lockbox (ключ edge-паблишера, токены источников) и Monitoring-дашборд,
      Container Registry (заготовка под Фазу 2); шаблон взять из `fincontext-mcp/infra`
- [ ] прогнать `terraform fmt -check` и `terraform validate` — без ошибок до Task 9

### Task 9: Статичный веб-клиент (cloud/web)
- [ ] создать статичную страницу: подписка по WebSocket, лента «новость→сентимент→тикер»,
      текущие сигналы по тикерам
- [ ] добавить явный disclaimer «не является инвестиционной рекомендацией»
- [ ] дизайн по глобальным правилам (SaaS-направление, токены, WCAG AA, контраст ≥14px)
- [ ] добавить smoke-тест рендера/парсинга WS-сообщения (если есть JS-раннер) либо
      задокументировать ручную проверку в README (fails until деплой — отметить)
- [ ] прогнать доступные тесты — должны пройти до Task 10

### Task 10: Verify acceptance criteria
- [ ] проверить, что все требования из Overview реализованы (event→ingest→YDB→WS→клиент)
- [ ] прогнать полный набор unit-тестов репо
- [ ] прогнать `terraform validate` для `cloud/infra`
- [ ] прогнать линтер — все замечания исправить
- [ ] проверить покрытие тестами по стандарту проекта

*Ручной E2E против облака и деплой — в Post-Completion (не автоматизируется агентом).*

### Task 11: [Final] Документация
- [ ] добавить `cloud/README.md` (архитектура, деплой, конфиг edge-паблишера)
- [ ] обновить корневой README/доки репо: раздел про MOEX Pulse и запуск на `.193`

## Technical Details

- **Модель события:** `{ticker, sentiment:{score,label}, signal:BUY|SELL|HOLD, source,
  produced_at, staleness_sec}`; деньги/скор — фиксированная точность; ID детерминированный.
- **Транспорт MVP:** HTTPS на API Gateway → ingest-Function; авторизация подписью SigV4 +
  ключ из Lockbox. (Фаза 2: замена на YMQ/Kafka.)
- **Хранение:** YDB serverless — последнее состояние по тикеру + реестр WS-коннектов;
  Object Storage — сырой архив событий (позже источник для ClickHouse).
- **Данные MOEX:** бесплатный ISS (`iss.moex.com`, задержка ~15 мин для акций) — достаточно
  для self+friends MVP; платный realtime — конфиг-точка Фазы 2.
- **Новости:** переиспользуем то, что уже настроено в новостном пайплайне репо.

## Post-Completion

*Требует ручного вмешательства или внешних систем — без чекбоксов, информационно.*

**Ручная проверка / деплой:**
- `terraform apply` в тестовом каталоге Yandex Cloud, деплой трёх Functions и API Gateway.
- Запуск `cmd/pulseedge` на `.193` (launchd) против облачного sandbox.
- Сквозной E2E: событие на `.193` → запись в YDB → обновление в браузере realtime.
- Проверка Monitoring-дашборда (латентность/ошибки ingest) и биллинга.

**Следующий план — Фаза 2 (highload, отдельный `docs/plans/`):**
- Managed Kafka как событийный backbone (edge → топик → consumers), либо Data Streams + YMQ.
- ClickHouse (Managed) для истории + DataLens-дашборды.
- Managed Kubernetes: WS-fanout как stateful-сервис, кросс-под fanout через Managed Redis,
  Application Load Balancer + Certificate Manager, instance groups/HPA + автоскейл,
  образы из Container Registry.
- Data Transfer (репликация YDB↔ClickHouse).
- Load-тестирование (k6 / Yandex Load Testing): тысячи WS-коннектов + высокочастотный ингест,
  проверка автоскейла и алертов.
- Цель: закрыть практикой все 6 доменов сертификации (вести чек-лист сервисов).
