> Перевод [ARCHITECTURE.md](ARCHITECTURE.md). При расхождении прав английский оригинал.

# Архитектура

Русский перевод: [ARCHITECTURE.ru.md](ARCHITECTURE.ru.md). Этот файл (английский) — канонический.

Закон: этот документ обновляется в том же коммите, что и любое изменение, которое добавляет, удаляет или переименовывает пакет `cmd/*` или `internal/*`, либо меняет живой цикл, поток данных, ключи конфигурации или внешнюю интеграцию. `internal/docscheck` валит сборку, если пакета нет в картах ниже.

Направление и цели: [VISION.md](VISION.md). Жёсткие инварианты и владение зонами: [../AGENTS.md](../AGENTS.md). Числа: [metrics.md](metrics.md).

## 1. Обзор системы

Однопроцессный модульный монолит на Go. Пакеты в `internal/*` общаются через интерфейсы; сетевых переходов между компонентами нет. `internal/bus` (Redis Streams) — неиспользуемый задел под будущее разделение процессов, его не импортирует ничто.

```
MOEX ISS / RSS + Telegram news / AlgoPack ──> orchestrator.MOEXIngestor
                                                   │
                                                   v
                                  features.Builder ──> domain.FeatureContext
                                                   │
                                                   v
        signal source chain (see 2.2): model | ensemble | score table
                                                   │  domain.TradeSignal
                                                   v
                      risk.HardenedGate ──> executor (paper | T-Bank sandbox | Finam)
                                                   │  Fill
                                                   v
         SQLite (audit_events, trade_signals, kill_switch) ──> cmd/verifier, cmd/livepnl
         Telegram alerts, Prometheus :9090
```

Офлайн-часть (исследования, обучение, валидация) делит с живым путём `internal/backtest`, `internal/features` и `internal/model`, поэтому бэктест проигрывает тот же код, что исполняет трейдер.

## 2. Живой трейдер (`cmd/trader`)

### 2.1 Порядок запуска

конфиг → storage (SQLite) → необязательный `-reset-kill-switch` (затем выход) → клиенты MOEX/новостей/AlgoPack/Telegram → брокерский runtime (`newBrokerRuntime`) → базовый источник сигнала (`newModelSignalSource`) → обёртки (2.2) → **preflight-гейт** → риск-гейт с circuit breaker по тикеру (заполняется из истории аудита через `seedTickerBreaker`) → сервер Prometheus → цикл оркестратора (`orchestrator.Run`) плюс фоновые горутины (метрики экспозиции, отслеживание качества исполнения, дневная сводка).

### 2.2 Выбор источника сигнала и цепочка обёрток

Базовый источник, срабатывает первое совпадение (`newModelSignalSource`, `cmd/trader/main.go`):

1. `model.score_table_path` → `model.ScoreTableSource` (`internal/model/scoretable.go`): дневная вероятность `p` по тикеру, читаемая из CSV (`ticker,date,p`, сейчас `data/ridge_scores.csv`, строится офлайн `scripts/ridge_probs.py` и забирается с ai-box через `scripts/ridge-scores-pull.sh`); строки старше 7 дней считаются устаревшими; пороги `p >= 0.60` BUY, `p <= 0.40` SELL.
2. `model.ensemble_path` → `model.EnsembleSignalSource` (`internal/model/ensemble.go`): LightGBM + XGBoost + логистическая регрессия, считаются на чистом Go; пороги хранятся в артефакте.
3. иначе `model.SignalSource` поверх `model.json` (объединённая логистическая регрессия, `internal/model/logistic.go`).

Обёртки, от внутренней к внешней: определитель размера лота (`lotSizeSignalSource`) → champion/challenger shadow (`challenger.go`; challenger никогда не доходит до executor) → монитор PSI-дрейфа (`driftSignalSource`, только предупреждает) → наблюдатель за коллапсом вероятности → гистерезис сигнала на два опроса (`orchestrator.SignalHysteresisSource`) → новостной гейт (`newsGateSignalSource`: вето по инциденту/шорт по инциденту, выход из позиции по новости через `newsExitManager`) → обёртка алертов (Telegram при ошибках сигнала).

Preflight-гейт использует базовый источник до обёрток shadow, drift и news (`newPreflight(cfg, modelSource, ...)`); он обязан использовать тот же базовый источник, что и живой цикл.

### 2.3 Цикл (`orchestrator.RunOnce`, каждые `poll_interval`, по умолчанию 5 мин)

1. Сброс кэша новостей цикла и предзагрузка стаканов AlgoPack.
2. Снимок счёта (`AccountSource`): портфель песочницы; в paper-режиме его нет.
3. Для каждого тикера: проверка kill switch → ingest (поиск бумаги, котировка, дневные свечи за 10 дней, новости, стакан) → `features.Builder.Build` → `SignalSource.Generate` → `risk.HardenedGate.Approve` → `Executor.Execute`. Каждая стадия пишет строку `audit_events` с именами стадий `ingest`, `llm` (историческое имя стадии сигнала; LLM на пути нет), `risk_gate`, `executor`.
4. Наблюдатели (Telegram-уведомитель о решениях, наблюдатель исполнений для breaker по тикеру, наблюдатель качества исполнения) видят каждое решение.

В песочнице executor обёрнут в guard торговых часов; открытые позиции сверяются с портфелем брокера (`position_reconcile.go`).

### 2.4 Preflight-гейт

`cmd/trader` проигрывает текущую конфигурацию за последние `preflight.days` (по умолчанию 90) дней через `internal/backtest` перед циклом и отказывается стартовать, когда реализованный P&L ниже `preflight.min_net_pnl`, закрытых сделок меньше `preflight.min_closed_trades`, история недоступна для всех тикеров или каждое решение завершилось ошибкой. Решение логируется вместе с SHA-256 от релевантной для гейта конфигурации.

### 2.5 Риск

`risk.HardenedGate` (`internal/risk/gate.go`): лимит лотов, лимиты чистой/короткой/длинной экспозиции, секторные лимиты, полоса fat-finger по цене, лимит дневного убытка, kill switch по просадке (хранится в SQLite, сбрасывается только через `-reset-kill-switch`, для чего нужен капитал ≥ 1.5x наибольшего открытого номинала по `runbook.Decide`). `risk.TickerBreaker` останавливает отдельную бумагу после подряд идущих или накопленных убытков. `risk/var.go` даёт 1-дневные VaR/ES только для мониторинга.

### 2.6 Режимы исполнения

- **paper** (`broker: paper`): `PaperExecutor`, виртуальные исполнения, без проверок счёта.
- **Песочница T-Bank** (`broker: tinkoff`, `tinkoff.sandbox: true`): клиент `internal/ingestion/tinkoff`, `internal/broker/tinkoff` — создание счёта/снимок/отмена по инструменту, `executor.LiveExecutor`.
- **Finam** (`broker: finam`): `internal/ingestion/finam`, `internal/executor/finam.go`.
- **Реальные деньги**: намеренно отключены; `cmd/trader` отказывается работать с не-песочным счётом T-Bank. См. [GO_LIVE_CHECKLIST.md](GO_LIVE_CHECKLIST.md).

## 3. Отказоустойчивость (два узла, один счёт песочницы)

Mac (основной) и ai-box (резервный) никогда не торгуют одновременно. Witness (`cmd/witness`, VPS на FreeBSD) выдаёт lease на 5 минут; `cmd/watchdog` на каждом узле запускает трейдер только пока держит его и останавливает (fence) при потере lease (`internal/failover`: выборы, клиент witness, heartbeat пира, синхронизация снимка SQLite). Авторитетная БД выбирается по `trader.last_start_at`, никогда по mtime файла. Деплой: `scripts/deploy-failover.sh {witness|aibox|mac|start-aibox|status}`, шаблоны в `deploy/`. Подробности и инварианты: [../AGENTS.md](../AGENTS.md).

## 4. Офлайн-конвейер

Данные: `cmd/pricehist`, `cmd/tqbrsnap`, `cmd/intradayfetch`, `cmd/futoifetch`/`futoihist`, `cmd/macrofetch`, `cmd/fundsnap`, `cmd/reportsfetch`, `cmd/dividendscalendar`, `cmd/tradesrecord`, `cmd/algopackfetch`/`algopackcheck`/`orderflowstudy`, новостные инструменты. Источники и ограничения: [knowledge/data-sources.md](knowledge/data-sources.md).

Обучение: `cmd/exportdataset` (CSV из `model.ToVector`) → `scripts/export_ensemble.py` → `ensemble_model.json`; `cmd/trainmodel` → `model.json`; `cmd/trainnewsmodel` → `news_classifier.json`. Порядок признаков продублирован в `internal/model/features.go`, `scripts/export_ensemble.py` и `scripts/compare_models.py`; рассинхрон — жёсткая ошибка загрузки (`checkFeatureOrder`) и отказ preflight. Подробности: [knowledge/feature-pipeline-and-inference.md](knowledge/feature-pipeline-and-inference.md).

Валидация: `cmd/backtest`, `cmd/walkforward` (purged, embargo), `cmd/strategyvalidation` (deflated Sharpe, PBO, Pass^k), `cmd/oosfreeze`, `cmd/challengercompare`. Каждое число попадает в [metrics.md](metrics.md) (линтер: `internal/metricsdoc`). Автоподмена живой модели запрещена; ежедневная задача КАНДИДАТА разрешена ([../AGENTS.md](../AGENTS.md)).

Жизненный цикл исследования: предварительно зарегистрированный план в [plans/](plans/README.md) → прогон → числа в `metrics.md` → план переносится в `plans/completed/`.

## 5. Карта пакетов: `cmd/*`

Живой контур и эксплуатация (в живом пути или пути отказоустойчивости):

| Package | Role |
|---|---|
| `cmd/trader` | живой цикл (раздел 2) |
| `cmd/watchdog` | супервизор узла: lease, fencing, синхронизация БД, алерты |
| `cmd/witness` | арбитр lease на VPS |
| `cmd/sandboxcheck` | дымовой тест песочницы T-Bank (`-place-order` по желанию) |
| `cmd/verifier` | markdown-отчёт по P&L из событий аудита |
| `cmd/pnlexporter` | экспортёр P&L для Prometheus из SQLite |
| `cmd/algopackgrafana` | дневная SQLite из AlgoPack и IMOEX (`data/grafana/algopack.db`) для дашборда Grafana `deploy/grafana/dashboards/algopack.json`; скользящие альфа и бета считаются в SQL на лету; таблица `ridge` грузится из `data/ridge_scores.csv` (тянется `scripts/ridge-scores-pull.sh`), дашборд показывает накопленные спреды верхнего квинтиля A2 и верхней трети ridge и последний рейтинг ridge; синхронизируется на ai-box скриптом `scripts/algopack-daily.sh` |
| `cmd/livepnl` | живой P&L за диапазон дат из аудита SQLite |
| `cmd/retrainreport` | Telegram-отчёт о ежедневном переобучении кандидата (без подмены модели) |
| `cmd/tradesrecord` | регистратор отложенных сделок AlgoPack (cron на ai-box) |
| `cmd/algopackfetch` | история Super Candles AlgoPack (`tradestats`, `obstats`, `orderstats`, `hi2`) в `data/algopack/<dataset>/<date>.csv.gz`, с возобновлением; ежедневная перезагрузка последних четырёх дней `scripts/algopack-daily.sh` (launchd, 00:20 MSK) |
| `cmd/algopackcheck` | проверка скачанной истории AlgoPack (календарь, покрытие, метрики hi2, масштаб цен относительно ISS); доходности не считает |
| `cmd/orderflowstudy` | предварительно зарегистрированное исследование потока ордеров на истории AlgoPack (блоки A-D, послефактумные проверки `-diag`), офлайн, читает `data/algopack` |
| `cmd/flowscan` | предварительно зарегистрированный скан 100 гипотез на AlgoPack (план `20261023-algopack-100-hypotheses`): режимы `prep` (M_eff, порог и списки контролей без результатов), `explore`, `verdict`; плюс режимы планов `20261023-algopack-forecast-improvement` и `20261023-algopack-round2`: `export` (дневной блок из десяти колонок AlgoPack с форвардными ценами закрытия), `volfc` и `spreadfc` (прогноз волатильности и спреда по Фама-Макбету с out-of-fold приростом R-квадрат), `econ` (экономические варианты A2, флаг размера вселенной), `mktprep` и `mkt` (семейства рыночной доходности и волатильности на уровне даты); Fama-MacBeth и нетто-торговая форма на строку, офлайн, читает `data/algopack` и `data/dividends.jsonl` |
| `cmd/algopackincremental` | парное тестирование прироста блока AlgoPack для развёрнутого ансамбля (планы `20261023-algopack-forecast-improvement` и `20261023-algopack-round2`): `merge` склеивает CSV `exportdataset` с блоком `flowscan export` (демин, abnormal-метка, плацебо-перестановка внутри даты), `score` прогоняет per-fold JSON ансамбля через `internal/model` и печатает парные ΔAUC и ΔIC с блочным бутстрапом, скорректированную на плацебо разность и поведенческое чтение, `ks` проверяет сдвиг распределения регенерированной и realtime-истории; офлайн, живой путь не затрагивается |
| `cmd/futscan` | предварительно зарегистрированный скан гипотез AlgoPack по фьючерсам (`fo`) и валюте (`fx`) (план `20261023-algopack-futures-fx`): `panel` строит дневную панель признаков по контрактам из трёх датасетов с флагами realtime по каждому датасету, `step0` (проверка происхождения данных, контрактов и распределений без исходов), `prep` (эффективное число тестов и порог без исходов), `scan` (Book 1: исходы по фьючерсам и валюте, Fama-MacBeth и временные ряды) и `scan2` (Book 2: исходы по акциям и IMOEX, только лиды); офлайн, живой путь не затрагивается |
| `cmd/alphafit` | исследование композитного альфа-сигнала и торговли с учётом издержек на файле `cmd/flowscan -mode matrix` (план `20261023-algopack-alphafit`): ранговые признаки, ridge и elastic net на Go с очищенным расширяющимся walk-forward, плацебо с перемешанными признаками, долларово-нейтральная лонг-шорт книга квинтилей с издержками и займом, исполнение с частичной подстройкой Гарлеану-Педерсена; режимы `explore` (гиперпараметры и скорость торговли на регенерированном окне) `verdict` (окно realtime, запуск один раз), `freeze`, `forward` и `dump`; флаги `-ext`, `-horizon`, `-cost` и `-entry` задают горизонт цели, модель издержек по спреду и вход по следующему открытию (план `20261024-algopack-wave3-costs-horizon`); `-with3b` оставляет четыре базовые колонки волны 3b как признаки, режим `shadow` пишет точечные во времени оценки замороженных моделей (план `20261023-algopack-alphafit`), `-div` добавляет производные колонки `DivFlow` и `A2PR`, `-aucinc` оставляет `AucA1` для входа по следующему открытию, режим `compare` сравнивает два выгруженных ряда (план `20261024-algopack-wave3b-model-ideas`), режим `lib2` проверяет 100 сгенерированных преобразований сильнейших признаков на прирост поверх композита (план `20261024-algopack-library-v2`), режим `libext` проверяет столбцы-кандидаты из CSV (например прогнозы TimesFM 3, план `20261025-algopack-timesfm3`) на прирост поверх композита, режим `lib3` проверяет те же кандидаты и пять характеристик акций как модераторы IC композита с диагностикой по стратам уверенности (план `20261025-algopack-library-v3-moderators`), флаг `-book longonly` строит книгу верхнего квинтиля против равновзвешенной вселенной (план `20261025-algopack-thin-universe`), режим `dsr` считает дефлированный коэффициент Шарпа и регрессию на рыночную бету по выгруженному ряду книги, а `-neutral sector` вычитает среднее прогноза по замороженным секторным спискам (план `20261025-algopack-deflation-neutrality`); офлайн, живой путь не затрагивается; режимы `fundgen`, `fundcover`, `fundmeff` и `fundtest` (план `20261027-algopack-fund-playbook`) строят 94 столбца по описаниям фондов на сетке дат панели, печатают покрытие и избыточность, считают порог Nyholt и запускают поэтапные тесты и шесть правил экспозиции книги; `momsleeve` принимает также `-sig gap|blend` (книги разворота гэпа открытия, план `20261027-algopack-gap-reversal-sleeve`), `-stage profile` и `-eqw` |
| `cmd/pairscan` | пары обыкновенных и привилегированных акций (план `20261025-algopack-pairs-arbitrage`): причинный индекс относительной доходности без дивидендных разрывов, строки возврата к среднему, импульса и разности A2 с регрессиями с фиксированными эффектами, p-значением круговой перестановкой и экономическим правилом с издержками и платой за перенос шорта на файле `cmd/flowscan -mode matrix`; офлайн, живой путь не затрагивается |
| `cmd/exectiming` | аналитика времени исполнения (план `20261025-algopack-execution-timing`): устойчивость внутридневного профиля котируемого спреда по бумагам и экономия от предсказанного самого дешёвого 30-минутного окна против открытия, закрытия и среднего по сессии, по `obstats` AlgoPack; офлайн |
| `cmd/newsimpulse` | исследование новостного импульса (план `20261025-algopack-news-impulse`): шоки внимания и словарной тональности по файлу заголовков с тикерами против избыточной доходности на 1, 5 и 10 дней и против доходности следующего дня от открытия до закрытия для новостей после закрытия, ранговый IC с Newey-West и плацебо на файле `cmd/flowscan -mode matrix`; офлайн |
| `cmd/newsllm` | оценка заголовков локальной LLM для исследования новостной тональности (план `20261026-algopack-news-llm-sentiment`): режим `pilot` (скорость на 500 заголовках, гистограмма меток, корреляция со словарём, двойник без тикера) и возобновляемый `score` через локальную модель Ollama, пишет `scores.csv` и `meta.json` с digest модели и хэшем промпта; `cmd/newsimpulse -llm` затем считает строки L1-L5 и плацебо; офлайн |
| `cmd/divrunup` | исследование дивидендного предвосхищения (план `20261025-algopack-dividend-runup`): избыточная доходность до и после последнего дня покупки против дивидендной доходности, кластерный бутстрап и лонг-оценка за вычетом спреда по закрытиям матрицы `cmd/flowscan -mode matrix` и календарю дивидендов; офлайн |
| `cmd/calmscan` | исследование «спокойного» состояния (план `20261026-calm-before-move`): ранжирует ли тихая бумага (низкое отношение волатильностей или плоский ценовой коридор) свою 10-дневную избыточную доходность лучше остальных; шесть зарегистрированных ячеек, плацебо, окно репликации |
| `cmd/metascan` | исследование мета-переменных (план `20261024-algopack-meta-variables`): причинные бинарные макро- и внешние состояния (ключевая ставка, CNY-RUB, Brent, IMOEX, золото, RVI, новости о санкциях, активность фондов, календарь) и негативные контроли (Сатурн, фаза Луны, Kp), разбиение дневного IC композита и чистой прибыли книги с частичной подстройкой по состояниям, взаимодействия с экспозициями акций, p-значение круговой перестановкой и порог по Nyholt M_eff; режимы `fetch`, `fetch2` и `fetch4` (ряды ISS, GFZ, Binance, Open-Meteo, Yahoo и GDELT в `data/macro-long`), `run` (волна 1), `wave2` (строки волны 2 и описательная диагностика) и `wave4` (строки данных о мире, план `20261025-algopack-wave4-world-data`); читают ряды `cmd/alphafit -mode dump`; офлайн, живой путь не затрагивается |
| `cmd/tinvestprobe` | проба фьючерсных ордеров в песочнице T-Bank |
| `cmd/borrowmeasure` | измерение ставок/лимитов короткого займа в песочнице |

Обучение и инструменты модели:

| Package | Role |
|---|---|
| `cmd/exportdataset` | исторический CSV признаков/меток для обучения на Python |
| `cmd/trainmodel` | объединённая логистическая регрессия → `model.json` |
| `cmd/trainnewsmodel` | классификатор тональности новостей → `news_classifier.json` |
| `cmd/calibrate` | диагностика меток по горизонтам |
| `cmd/explaincheck` | проверка, что окклюзионные драйверы `internal/model/explain.go` объясняют вероятность |
| `cmd/featureparity` | проверка паритета вектора признаков Go и Python |
| `cmd/listtqbr` | список вселенной тикеров TQBR |

Валидация и бэктест:

| Package | Role |
|---|---|
| `cmd/backtest` | исторический реплей с `-signal-source=model\|ensemble\|rule\|csvprob` |
| `cmd/walkforward` | purged walk-forward с артефактами по окнам |
| `cmd/strategyvalidation` | реестр попыток, DSR, PBO, надёжность, консистентность |
| `cmd/oosfreeze` | заморозка OOS-рецепта (`-action record\|status`) |
| `cmd/challengercompare` | challenger против champion по shadow-логу |
| `cmd/tailprecision`, `cmd/momentumbench`, `cmd/betaregime`, `cmd/gapstress`, `cmd/alphahedge` | исследования валидации стратегии (см. [reports/](reports/)) |

Сбор данных:

| Package | Role |
|---|---|
| `cmd/pricehist`, `cmd/tqbrsnap`, `cmd/intradayfetch` | дневные свечи MOEX ISS, борд TQBR и 10-минутные свечи |
| `cmd/futoifetch`, `cmd/futoihist` | открытый интерес по фьючерсам |
| `cmd/macrofetch` | макрорялы (Brent, CNY/RUB, ключевая ставка, IMOEX) |
| `cmd/fundsnap`, `cmd/reportsfetch` | фундаментальные данные T-Invest и календарь отчётности |
| `cmd/dividendscalendar` | календарь дивидендов → `data/dividends.jsonl` |
| `cmd/newsfetch`, `cmd/newsimport`, `cmd/newsclean`, `cmd/newsrematch`, `cmd/tgarchive` | архив новостей: загрузка, импорт, очистка, перепривязка, архив Telegram |
| `cmd/newsscore` | оценка новостей лексиконом/regex/моделью |

Исследовательские работы (не на торговом пути): (placebo-проверка таблицы оценок: `cmd/placebocheck` и `internal/placebo`, H-R8, прогоняет через движок бэктеста таблицы с циклическим сдвигом и перестановкой по датам) и `cmd/tickerselect` и `internal/tickersel` (walk-forward и случайный отсев убыточных тикеров на реплее), `cmd/gapattrib` и `internal/gapattrib` (атрибуция разрыва между исследованием и live для книги ridge, H-R9; читает перцентили всей вселенной, которые пишет `scripts/ridge_full_dump.py`), `cmd/eventstudy` (событийное исследование новостей), `cmd/dividendstudy` (дивидендный дрейф), `cmd/newsdecay` (затухание реакции по горизонтам), `cmd/newsvalidate` и `cmd/newsportfolio` (валидация новостного классификатора и портфель), `cmd/llmdirection` (zero-shot направление LLM по заголовкам, только офлайн), `cmd/leadlag` (lead-lag между тикерами), `cmd/rankspread` и `cmd/xsecrank` (кросс-секционное ранжирование по сохранённым OOS-вероятностям), `cmd/regimemask` (режимный фильтр IMOEX поверх сохранённых вероятностей walk-forward), `cmd/icaportfolio` (портфель VAR-ICA-MGARCH). Результаты и вердикты: [metrics.md](metrics.md) и [plans/](plans/README.md).

## 6. Карта пакетов: `internal/*`

| Package | Role | Path |
|---|---|---|
| `internal/domain` | `FeatureContext`, `TradeSignal`, `AuditEvent`, помощники для decimal | live |
| `internal/config` | загрузка YAML + `.env`, тикеры, секторы, длительности | live |
| `internal/ingestion` | клиенты `moex` (ISS), `news` (RSS/Telegram/spool, matcher, industry), `algopack` (стакан, сделки), `tinkoff`, `finam` | live |
| `internal/features` | `Builder` и ценовые/рыночные/событийные/инцидентные/сентиментные признаки | live |
| `internal/model` | ансамбль, логистика, таблица оценок, новостной классификатор, масштабирование по волатильности, explain, калибровка, надёжность | live |
| `internal/orchestrator` | ingest, цикл, гистерезис сигнала, shadow-reconciler | live |
| `internal/risk` | усиленный гейт, breaker по тикеру, VaR/ES | live |
| `internal/executor` | исполнители paper/live/Finam, восстановление, cooldown отказов, целевая позиция | live |
| `internal/broker` | `tinkoff`: создание счёта песочницы, снимок, отмена | live |
| `internal/storage` | SQLite WAL: аудит, сигналы, kill switch | live |
| `internal/alert` | клиент `telegram` и наблюдатель за вероятностью/устаревшими свечами/PSI | live |
| `internal/metrics` | метрики Prometheus | live |
| `internal/drift` | PSI-дрейф признаков | live |
| `internal/dailysummary`, `internal/pnl` | дневная сводка в Telegram, расчёт P&L | live |
| `internal/filltracking`, `internal/spread`, `internal/borrowcost` | качество исполнения, пер-тикерные спреды из аудита, стоимость займа | live/analysis |
| `internal/incidentcheck` | аудит/проверка детектора инцидентов | live |
| `internal/dividends`, `internal/macro` | провайдеры календаря дивидендов и макрорядов | live/training |
| `internal/runbook` | правило капитала для сброса kill switch | live |
| `internal/failover` | выборы, witness, пир, снимок, heartbeat | failover |
| `internal/bus` | заготовка Redis Streams (не используется) | none |
| `internal/backtest` | движок реплея, источник ISS, кэш решений, атрибуция, отчёты | offline + preflight |
| `internal/walkforward`, `internal/strategyvalidation`, `internal/oosfreeze`, `internal/trainrun`, `internal/verifier`, `internal/tearsheet` | валидация, прогон обучения, отчёт по P&L, HTML-tearsheet | offline |
| `internal/metricsdoc` | линтер `docs/metrics.md` | offline |
| `internal/betaregime`, `internal/momentum`, `internal/equalweight`, `internal/gapstress`, `internal/tailprecision`, `internal/newsstudy`, `internal/icaport` | исследовательские библиотеки за CLI исследований | research |

## 7. Скрипты

| Script | Role | Runs on |
|---|---|---|
| `deploy-failover.sh`, `sync-aibox.sh` | деплой отказоустойчивости, синхронизация ai-box | Mac |
| `export_ensemble.py`, `compare_models.py`, `combos.py`, `dump_parity_fixture.py` | обучение/экспорт ансамбля, сравнение, фикстура паритета | Mac / ai-box |
| `timesfm_features.py` | вывод TimesFM 3 без дообучения в CSV прогнозов (только исследование, некоммерческие веса; единственное исключение по Python, изолированное виртуальное окружение на ai-box; план `20261025-algopack-timesfm3`) | ai-box |
| `timesfm_features_b.py` | вывод TimesFM 3, волна A (контексты разной длины, квантили, перемешанный хвост, горизонт 20, прогнозы потоков) в один CSV; то же исключение по Python; план `20261025-algopack-timesfm3-wave2` | ai-box |
| `daily-retrain.sh`, `daily-retrain-box.sh`, `recent-window-experiment.sh` | ежедневное переобучение КАНДИДАТА (только отчёт) | Mac / ai-box cron |
| `ridge_probs.py`, `ridge_full_dump.py`, `factors_wide.py`, `ridge-scores-pull.sh`, `factor_shadow*.py`, `quantile_probs.py` | таблица оценок ridge и исследование факторов | ai-box → Mac |
| `breadth*.sh/py`, `hypo*.sh/py`, `depth-candidate.sh`, `rerun-r2.sh`, `rerun-forward.sh`, `calendar_cycles.py`, `intraday_*.py`, `futoi_study.py`, `events_study.py`, `procvar.py`, `world_*.py`, `timesfm_features.py`, `make_candles_csv.py` | исследования гипотез | ai-box |
| `news-daily.sh`, `rebuild-news-archive.sh`, `score_news.py`, `tgarchive-live.sh`, `fundsnap-daily.sh`, `regime-fresh.sh`, `trades-record-deploy.sh` | cron сбора данных | Mac / ai-box |
| `launchd/` | юниты launchd для macOS | Mac |

Новый код пишется на Go; скрипты на Python — устаревший исследовательский инструментарий.

## 8. Конфигурация и файлы деплоя

`config.example.yaml` — шаблон; `config.paper.yaml` и `config.sandbox.yaml` — готовые профили; `config.yaml`, `.env`, `watchdog.yaml`, `watchdog.env` локальные и в gitignore. Секреты берутся только из окружения (`MOEX_TRADER_TINKOFF_TOKEN`, `MOEX_TRADER_ALGOPACK_TOKEN`, токен Telegram). Структура конфигурации: `internal/config/config.go`. Секции верхнего уровня: `tickers`, `model`, `moex_iss_base_url`, `algopack_base_url`, `storage`, `risk`, `commission`, `finam`, `tinkoff`, `preflight`, `broker`, `is_paper_trading`, `poll_interval`, `telegram`, `alerts`, `news`.

Артефакты в корне репозитория: `ensemble_model.json`, `model.json`, `news_classifier.json`, `data/ridge_scores.csv` (не отслеживается), `artifacts/oos/freeze.json`. Шаблоны отказоустойчивости и CA-бандл: `deploy/`.

## 9. Наблюдаемость

Prometheus `:9090`: длительность инференса, `signals_generated_total`, `risk_rejections_total{reason}`, `executor_skips_total{reason}`, `signal_probability`, `candle_age_seconds`, `feature_psi`, `position_notional`, `gross_exposure`, `lease_held`, `var_95/99`, `es_95/99`. Каталог: [metrics.md](metrics.md), раздел 10 «Observability metrics and alerts». Telegram: ошибки сигнала, kill switch, исполненные решения, cooldown-алерты.
