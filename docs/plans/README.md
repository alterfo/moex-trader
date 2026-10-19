# Планы и предрегистрации

Индекс составлен 2026-10-18 по заголовкам файлов и разделам `docs/metrics.md`. Правила: критерии пишутся до запуска, числа попадают в `docs/metrics.md` тем же коммитом, что и результат, итог плана отражается в его заголовке `Status:`. Завершённые старые планы лежат в `completed/`.

## Действующие

| План | Статус |
|---|---|
| `20261018-data-depth-and-open-problems.md` | Идёт. Шаги 1, 2, 3, 5, 6 и H23 выполнены (глубина цен, новостной классификатор, калибровка: нулевые; объяснимость: принято; VAR-ICA-MGARCHSK: отклонён), шаг 2б (сдвиг домена новостей) выполнен: вердикта нет по числу дат (38 < 40), сдвиг домена не подтверждён, шаг 4 отложен |
| `20261015-hypotheses-from-llm-review.md` | Реестр гипотез H1-H22 из обзоров LLM-прогнозирования; результаты закрытых гипотез в `docs/metrics.md` (H20, H22: нулевые) |
| `20261010-news-exit-tuning-and-buy-side.md` | PLANNED, заблокирован данными |
| `20260927-moex-pulse.md` | Статус в файле не указан (облачный слой раздачи сигналов); проверить, живой ли план |

## Выполненные (результат в `docs/metrics.md`)

| План (лежит в `completed/`) | Итог |
|---|---|
| `20261018-excess-label-walkforward.md` | Experiment F, отвергнут по обеим гипотезам |
| `20261018-manual-retrain.md` | Кандидат не прошёл гейт |
| `20261018-sell-bias-diagnostic.md` | Диагностика SELL-перекоса, раздел «Sell-bias diagnostic» |
| `20261018-preflight-concentration.md` | Расследование отказа префлайта, раздел «Preflight-concentration analysis» |
| `20261010-macro-fundamental-features.md` | EXECUTED, прироста AUC нет, модель не переобучалась |
| `20261009-event-causal-study.md` | EXECUTED, эффекта не найдено |
| `20260925-event-driven-dividend-drift.md` | EXECUTED, отвергнут (захват дивидендов) |
| `20260925-alpha-without-beta-test.md` | EXECUTED, альфа переживает бета-нейтральное исполнение |
| `20260917-strategy-validation-action-plan.md` | Согласованный план валидации; реализация в `completed/20260917-strategy-validation-implementation-plan.md` |

## Перенос в `completed/` (выполнен 2026-10-18)

Девять выполненных планов из таблицы выше перенесены в `completed/` (`git mv`) одним коммитом вместе с заменой путей `docs/plans/<имя>` на `docs/plans/completed/<имя>` в `docs/metrics.md`, `docs/knowledge/*`, `docs/reports/dividendstudy-report.md` и в самих планах; заодно исправлена старая битая ссылка на `completed/20261004-model-quality-and-robustness.md`. Правило: новый выполненный план переносится в `completed/` в том же коммите, где его итог попадает в `docs/metrics.md`, с правкой ссылок.

## Замечание

В корне репозитория лежит неотслеживаемый бинарник `backtest` (результат `go build ./cmd/backtest`); он не относится к планам, не тронут.
