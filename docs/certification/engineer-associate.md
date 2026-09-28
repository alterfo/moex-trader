# Yandex Cloud Certified Engineer Associate — study kit

Учебный набор для подготовки к экзамену, привязанный к проекту **MOEX Pulse**
(`docs/plans/20260927-moex-pulse.md`). Идея: каждую задачу проекта строим, читая
соответствующий раздел материалов — практика и подготовка идут вместе.

## Факты об экзамене

- Уровень: базовый. Формат: **65 вопросов онлайн, 90 минут**, проходной балл **70%**.
- Стоимость: **7 000 ₽**. Прокторинг через **Examus** (нужна вторая камера на телефоне,
  чистый стол, выключенные фоновые приложения; всё проверить заранее).
- **Экзамен теоретический** (не лабы), но с сильным упором на **последовательности CLI-команд**,
  редко используемые опции сервисов и точные формулировки. Иногда встречаются неоднозначные вопросы.
- Стратегия сдачи в 3 прохода: сначала уверенные ответы → потом сложные → финальная проверка.
- Готовиться минимум неделю; знания держать «свежими» (повторить прямо перед экзаменом).
- Регистрация: https://app.examus.net/yandex-cloud · инфо: https://yandex.cloud/ru/certification

## Приоритетные пробелы (по отзывам сдавших)

Уделить больше времени тому, где меньше практики:
- **CLI-команды** (`yc ...`) — даже если привык к GUI/Terraform, экзамен спрашивает CLI.
- Сеть (VPC, security groups, маршрутизация), **CDN** и Big Data.
- **DataLens** (BI-визуализация).
- **Lockbox** (управление секретами).
- **Расчёт потребления/биллинга**.

## Официальные материалы (бесплатные, если не помечено)

- **Бесплатный курс «Основы работы с Yandex Cloud»** (с сертификатом) — база: https://yandex.cloud/ru/training
- **Документация** по каждому сервису + практические руководства (tutorials): https://yandex.cloud/ru/docs
- **YouTube-канал Yandex Cloud** — разборы сервисов.
- **Learning paths** в training-хабе: Cloud Base, DevOps, Data, Security.
- Telegram: подготовка к сертификации https://t.me/YCCertification · курсы https://t.me/yc_courses
- Компетенции/блупринт: https://yandex.cloud/ru/certification/engineer/competencies
- FAQ/правила: https://yandex.cloud/ru/certification/faq · https://yandex.cloud/ru/certification/requirements
- **Платный, опционально:** курс «Инженер облачных сервисов» (Яндекс Практикум) —
  практические задания в облаке: https://practicum.yandex.ru/ycloud

## Блупринт: домены → сервисы → задача MOEX Pulse

### 1. Инфраструктура
Compute Cloud (ВМ, снапшоты/образы), VPC, Network/Application Load Balancer,
security groups, instance groups + автоскейл.
→ *MOEX Pulse:* Фаза 2 (k8s-ноды, ALB, instance groups); VPC — Task 8.

### 2. Данные и хранение
Object Storage (классы, доступ), Managed PostgreSQL/MySQL, **YDB**, **ClickHouse**,
OpenSearch, **Managed Kafka**, Data Proc, Data Transfer, **DataLens**, DataSphere.
→ *MOEX Pulse:* YDB + Object Storage (Task 5–8); ClickHouse + DataLens + Kafka + Data Transfer (Фаза 2).

### 3. DevOps и автоматизация
Yandex Cloud CLI, **Terraform**, Docker/**Container Registry**, **Managed Kubernetes**,
Managed GitLab, **Monitoring** (кастомные метрики).
→ *MOEX Pulse:* Terraform + Container Registry + Monitoring (Task 8); k8s (Фаза 2).
Отдельно прогнать CLI-эквиваленты всего, что делаешь в Terraform (для экзамена).

### 4. Serverless
**Cloud Functions**, **API Gateway**, **Message Queue**, Data Streams, Serverless Containers,
Serverless YDB.
→ *MOEX Pulse:* Functions + API Gateway + Serverless YDB — ядро Фазы 1 (Task 5–8);
Message Queue/Data Streams (Фаза 2).

### 5. Безопасность
**IAM**, **KMS**, **Lockbox**, Certificate Manager.
→ *MOEX Pulse:* IAM (least-privilege SA) + KMS + Lockbox — Task 8; Certificate Manager (Фаза 2, TLS на ALB).

### 6. Биллинг
Расчёт стоимости, платёжный аккаунт, оптимизация затрат.
→ *MOEX Pulse:* оценивать стоимость каждого сервиса при провижене; следить за расходом sandbox.

## Чек-лист подготовки

- [ ] Пройти бесплатный курс «Основы работы с Yandex Cloud».
- [ ] По каждому домену прочитать docs выбранных сервисов + один tutorial.
- [ ] Для каждого сервиса, что поднимаю в MOEX Pulse, выписать **CLI-эквивалент** (`yc ...`).
- [ ] Отдельно закрыть пробелы: сеть/CDN, DataLens, Lockbox, расчёт биллинга.
- [ ] Пройти пробные вопросы/обсуждения в @YCCertification.
- [ ] Настроить и протестировать Examus (вторая камера, чистый стол) заранее.
- [ ] Вести таблицу «домен → сервис → потрогал на практике» по мере стройки проекта.

## Источники

- https://yandex.cloud/ru/certification
- https://yandex.cloud/ru/certification/engineer/competencies
- https://yandex.cloud/ru/training
- https://habr.com/ru/companies/yandex_cloud_and_infra/articles/827412/
- https://practicum.yandex.ru/ycloud
