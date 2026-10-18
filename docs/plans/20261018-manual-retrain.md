# Ручная ретренировка ансамбля (2026-10-18)

Владелец одобрил 2026-10-18 («давай 1 2 3», пункт 2). Повод: D4 из диагностики SELL-перекоса — свежая модель даёт 2.2% BUY против 0% у развёрнутой на том же окне (устаревание BUY-хвоста). Автозамена по-прежнему запрещена; это однократная ручная ретренировка с явным согласием.

## Кандидат

Рецепт развёрнутой модели без изменений: absolute-10d, 18 признаков (включая `news_sentiment`/`news_count`), 28-имённый порядок, colsample 0.8, пороги 0.60/0.40, удержание до флипа. Меняется только обучающее окно.

- Данные (ai-box, `~/moex-trader-task12`): `exportdataset -config config.sandbox.yaml -tickers <17 живых + DATA> -from 2024-01-01 -till 2026-10-18 -split 2026-06-18 -horizon-days 10 -deadband-pct 0.5 -label-mode absolute -news-history news_history.jsonl -out data-18f-retrain.csv`; `news_history.jsonl` — текущий архив с Mac (14619 записей).
- Обучение: `scripts/export_ensemble.py`, `ENSEMBLE_TRAIN_TILL=2026-10-17`, `ENSEMBLE_EMBARGO_SPLIT=2026-10-17`, `ENSEMBLE_EMBARGO_BARS=10`, FEAT18/ORDER из Task 5, venv с закреплёнными версиями (`scripts/requirements-ensemble.txt`).
- Серверный путь не меняется: новостные признаки маскируются (`untrainedFeatures`), гейт out_of_range с фиксом coef=0 — оба уже в HEAD.

## Гейт замены (зафиксирован до запуска)

Оба реплея — движком HEAD (`backtest-fixed` с фиксом гейта, linux/amd64), одинаковые издержки (0.0005×3), 1M депозит, 15000₽/позиция, kill-switch on, 17 тикеров:

1. Холдаут 30 дней (2026-09-17..2026-10-17): realized P&L кандидата ≥ realized P&L развёрнутой модели на том же окне.
2. Префлайт-окно 90 дней (2026-07-20..2026-10-17): realized P&L кандидата ≥ realized P&L развёрнутой модели на том же окне, и ≥ `preflight.min_net_pnl`/`min_closed_trades` (1/1), и max DD кандидата ≤ max DD развёрнутой + 0.5 п.п.

Замена только если выполнены оба пункта; иначе числа фиксируются, модель не меняется.

## После замены

- `ensemble_model.json` заменяется; golden backtest обновляется (`-update`), parity fixture перегенерируется (`scripts/dump_parity_fixture.py` на ai-box); `make check` проходит; метрики и реестр (`manual_retrain_20261018`) — тем же коммитом.
- Деплой (пункт 1): пересборка `./trader` на Mac, рестарт через вотчдог (вотчдог держит лизинг, failure-счётчик сбрасывается после активации); ai-box обновляется штатным aibox-sync (standby).
- OOS: `cmd/oosfreeze -action freeze` сразу после деплоя.
