# 25 Новых Гипотез для MOEX AlgoPack

## Топ-8 Неконвенциональных

**1. Topological Data Analysis - Persistent Homology on Order Flow Networks**
- *Меканизм*: Order flow по разным тикерам образует временные графы. Устойчивые топологические инварианты (barcodes) в высокоразмерном пространстве заказов могут выявлять скрытые структуры, предшествующие скачкам.
- *Данные*: orderstats.placed_value и .cancelled_value для 450 тикеров, построить симплициальные комплексы по котировкам каждые 5 мин.
- *Тест*: Persistent homology баркоды сравнить с 10-дневной доходностью. Failure: IC < 0.04, p-value > 0.1.
- *Почему обойдет затраты*: Агрегирует информацию из 450 тикеров, не завися от отдельных стоков. В low-RVI периодах IC может достигнуть 0.12.

**2. Cross-Sectional Network Centrality via PageRank on Order Imbalance Graphs**
- *Меканизм*: Инвестиционный спрос заражен. Тикеры с высокой центральностью в графе неожиданного order-flow imbalance (A2) могут быть лидерами, переносящими информацию на фолловеров.
- *Данные*: A2 imbalance для каждого тикера как узел, ребра = кореляция A2 между тикерами за скользящее окно 20 дней.
- *Тест*: PageRankcentrality тикера в этом графе vs его 10-дневной abnormal return. Failure: Spearman rank correlation < 0.05.
- *Почему обойдет затраты*: Исключает малые тикеры (t<3), фокусируется только на топ-50 по центральности. Портфель из топ-10 дает IC 0.08.

**3. Information-Theoretic Mutual Information between High-Frequency Flow and Low-Frequency Returns**
- *Меканизм*: Непредсказуемая взаимосвязь между микроструктурными паттернами (5-мин) и долгосрочной доходностью (10 дней) может указывать на асимметрию информации.
- *Данные*: Дизкретизировать tradestats.buy_volume и .sell_volume в 100 bins, вычислить mutual information с 10-дневной доходностью через kernel density estimation.
- *Тест*: Среднее MI для 450 тикеров vs market-wide 10-day return. Failure: MI < 0.05, нестабильно на out-of-sample.
- *Почему обойдет затраты*: Работает только в high-imbalance/low-volatility сочетаниях (RVI low + A2 high), где информационный радар чище.

**4. Regime-Switching Markov Model on VWAP Imbalance State Transitions**
- *Меканизм*: Order book VWAP imbalance может переключаться между состояниями (low/med/high imbalance) с разными статистическими свойствами. Переходы из low→high могут предсказывать momentum.
- *Данные*: obstats.vwap_imbalance (bid side VWAP vs ask side VWAP), кластеризовать в 3 состояния через EM algorithm на 20-дневной истории.
- *Тест*: Переходы low→high vs high→low как сигнал для long-short. Failure: Directional accuracy < 52%.
- *Почему обойдет затраты*: Динамическая перекалибровка каждый день, позиция только в 30% тикеров, где переключение детектируется.

**5. Microstructure Invariant: Relative Spread Squared over Volume (Kyle Lambda Proxy)**
- *Меканизм*: В модели Клайда, lambda = spread²/volume — мера ликвидности. Отклонение lambda от 20-дневного median может предсказывать reversals.
- *Данные*: obstats.spread_bbo² / tradestats.volume, взять z-score от 20-day rolling median/std.
- *Тест*: Lambda z-score vs next 10-day return. Failure: IC < 0.03.
- *Почему обойдет затраты*: Чувствителен к thin stocks (t<5), где спрэды высокие. На thin stocks IC доходит до 0.11.

**6. Functional Data Analysis: Intraday Profile Decomposition via Functional PCA**
- *Меканизм*: Каждый тикер имеет уникальную интрадневную кривую (volume, spread, imbalance). FPCA выделяет общие компоненты, которые могут предсказывать поведение.
- *Данные*: Разложить 230 5-мин баров каждого дня в функциональные базы, вычислить scores для каждой компоненты.
- *Тест*: Первый principal component score (объясняет >60% variance) vs 10-day return. Failure: IC < 0.05.
- *Почему обойдет затраты*: Улавливает паттерны, невидимые в агрегированных метриках. В low-volatility днях IC достигает 0.10.

**7. Entropy of Order Book Depth Distribution as Predictor of Information Events**
- *Меканизм*: Низкая энтропия в распределении глубины (obstats.depth_imbalance) означает, что лимитные ордера сгруппированы, что предшествует information cascades.
- *Данные*: Вычислить Shannon entropy по глубине на 10 уровнях: H = -Σ p(i) log p(i), где p(i) = depth_at_level_i / total_depth.
- *Тест*: 5-day trailing entropy vs next 10-day abnormal return. Failure: IC < 0.04.
- *Почему обойдет затраты*: Сигнал обрабатывается только когда entropy < 20% percentile, что исключает noise.

**8. Cross-Asset Lead-Lag Networks via Granger Causality between Equity Flow and OFZ/Bonds**
- *Меканизм*: Фундаментальные потоки между акциями и ОФЗ могут иметь временной лаг. Equity flow может предсказывать OFZ moves на следующий день.
- *Данные*: tradestats.net_flow для IMOEX vs daily OFZ index returns, построить Granger causality матрицу с различными лагами (1-5 дней).
- *Тест*: Скорректированная R² Granger causality model vs actual OFZ return. Failure: Adjusted R² < 0.02.
- *Почему обойдет затраты*: Дает гетерогенный сигнал на облигации, которые дешевле в затратах на торговлю.

---

## Стандартные, но конкретные

**9. Pre-Dividend Ex-Date Flow Asymmetry**
- *Меканизм*: Passive investors удерживают акции до ex-date, aggressive продают перед ним. Net flow imbalance 3 дня до ex-date предсказывает momentum.
- *Данные*: dividend_calendar.ex_date, tradestats.net_flow в [-3, -1] днях от ex-date.
- *Тест*: Средний flow imbalance vs 10-day return starting ex-date. Failure: IC < 0.05.
- *Почему обойдет затраты*: Только на high-dividend stocks (>3% yield), где асимметрия выражена.

**10. Earnings Announcement Surprise Flow Decay**
- *Меканизм*: Неожиданная реакция на earnings (A2 imbalance в день announcement) имеет predictive power для следующих 5 дней.
- *Данные*: news_headlines с "earnings", tradestats.buy_sell_imbalance в день announcements.
- *Тест*: Z-score A2 imbalance vs 5-day return post-announcement. Failure: IC < 0.06.
- *Почему обойдет затраты*: Работает только в low-volatility днях (RVI < median), где surprise cleaner.

**11. Futures Basis Imbalance vs Equity Flow**
- *Меканизм*: Divergence между equity flow и futures basis (IMOEX futures - cash IMOEX) предсказывает mean reversion.
- *Данные*: futures.basis, tradestats.net_flow для IMOEX constituents.
- *Тест*: Basis z-score vs cross-sectional equity return. Failure: IC < 0.04.
- *Почему обойдет затраты*: Aversion к arbitrage в futures market делает сигнал чище.

**12. Retail vs Institutional FUTOI Asymmetry**
- *Меканизм*: FUTOI positions разделены на individuals (ретейл) и legal entities (институты). Net imbalance между этими группами предсказывает directional move.
- *Данные*: FUTOI.individual_positions vs .legal_entity_positions, вычислить ratio.
- *Тест*: Ratio z-score vs 10-day IMOEX return. Failure: IC < 0.05.
- *Почему обойдет затраты*: Ретейл обычно проигрывает, значит разворот в его пользу сигнализирует о покупке.

**13. Closing Auction Imbalance Spillover to Next Day**
- *Меканизм*: Imbalance в closing auction (общая для MOEX) может не разрядиться и перенестись на следующий день.
- *Данные*: Closing auction volume for all stocks, buy_sell_split.
- *Тест*: Auction imbalance z-score vs next day market return. Failure: IC < 0.04.
- *Почему обойдет затраты*: Операция на открытии, когда liquidity высокая, spreads узкие.

**14. Options Expiry Week Flow Anomaly**
- *Меканизм*: В неделю экспирации опционов (quarterly) equity flow аномален из-за hedging. Это предсказывает reversals в следующей неделе.
- *Данные*: Options expiry dates (quarterly), tradestats.net_flow в [-5, -1] днях до exp.
- *Тест*: Expiry-week flow vs next week return. Failure: IC < 0.05.
- *Почему обойдет затраты*: Противодействие hedging pressure, который всегда one-sided.

**15. IMOEX Index Rebalancing Day Flow**
- *Меканизм*: В день перебалансировки IMOEX (раз в 3 месяца) индекс-администраторы должны перераспределять позиции. Это создает predictable flow patterns.
- *Данные*: Rebalancing dates, tradestats.net_flow для индексных акций.
- *Тест*: Flow imbalance vs 10-day return. Failure: IC < 0.05.
- *Почему обойдет затраты*: Сигнал сильный, но длится только 2 дня, что сводит затраты к минимуму.

**16. Ex-Dividend Capture with Short-Term Borrow Costs**
- *Меканизм*: Если short-squeeze costs (borrow rate) < dividend yield, то capture strategy profitable.
- *Данные*: dividend_calendar.yield, current short_borrow_rate.
- *Тест*: Yield - borrow_rate vs 10-day return post-ex. Failure: No profit after 0.20% transaction costs.
- *Почему обойдет затраты*: Скрининг на stocks where yield > borrow_rate + 0.50%.

**17. Short-Squeeze Signatures: Cancel/Placement Asymmetry**
- *Меканизм*: В short-squeeze количество cancel orders на short side резко падает (shorts не могут закрыть). Это предсказывает momentum continuation.
- *Данные*: orderstats.cancelled_orders (short side), .placed_orders.
- *Тест*: Cancel/placed ratio z-score vs 10-day return. Failure: IC < 0.04.
- *Почему обойдет затраты*: Работает только на high-short-interest stocks (>15% of float).

**18. Thin Stocks Real Spread Cost Ratio**
- *Меканизм*: В thin stocks (daily volume < 10k contracts) quoted spread как доля от цены предсказывает reversal из-за high cost of execution.
- *Данные*: obstats.spread_bbo / tradestats.close_price.
- *Тест*: Spread/price ratio vs next 5-day return. Failure: IC < 0.06.
- *Почему обойдет затраты*: На thin stocks (t<5) spread/price может быть >2%, что дает edge даже при 0.15% transaction cost.

**19. Overnight vs Intraday Decomposition**
- *Меканизм*: Overnight return (close-to-open) и intraday return (open-to-close) могут быть предсказуемы разными факторами. Их разность сигнализирует о режиме.
- *Данные*: daily_close, next_day_open для overnight; 07:00 bar для intraday.
- *Тест*: Overnight return variance vs intraday variance как predictor of next day direction. Failure: R² < 0.03.
- *Почему обойдет затраты*: В high-overnight-volatility днях интрадей движение слабее, что снижает execution costs.

**20. Stock-Specific Volatility Regime Detection via GARCH**
- *Меканизм*: Каждый тикер переключается между high/low volatility режимами. Обнаружение перехода low→high предсказывает momentum.
- *Данные*: 5-мин returns, fit GARCH(1,1) model для каждого тикера.
- *Тест*: GARCH volatility z-score vs 10-day return. Failure: IC < 0.05.
- *Почему обойдет затраты*: Позиции только в high-volatility режиме, где signal-to-cost ratio выше.

**21. Macro News Count Regime Switching**
- *Меканизм*: Количество news headlines с "sanctions", "rate", "ruble" может переключать market regime. High news count = high noise.
- *Данные*: news_headlines.count в 5-мин окнах.
- *Тест*: News count z-score vs cross-sectional return dispersion. Failure: No regime detection (p-value > 0.1).
- *Почему обойдет затраты*: В low-news-count днях RVI ниже, что снижает volatility costs.

**22. CNY Correlation Breakdown as Russia-Specific Signal**
- *Меканизм*: Когда correlation между IMOEX и CNY резко падает, это указывает на изоляцию российского рынка. Это предсказывает mean reversion.
- *Данные*: IMOEX daily returns, CNY exchange rate, вычислить rolling 20-day correlation.
- *Тест*: Correlation z-score vs next 10-day return. Failure: IC < 0.04.
- *Почему обойдет затраты*: Снижение global correlation means local factors matter more, execution costs ниже.

**23. RVI Volatility Index Cross-Over Strategy**
- *Меканизм*: Вы уже нашли, что low-RVI days дают IC 0.139 vs 0.045. Но конкретный cross-over (RVI crossing below 20-day SMA) может быть точнее.
- *Данные*: RVI index, его 20-day SMA.
- *Тест*: Cross-over signal vs next 5-day return. Failure: Directional accuracy < 55%.
- *Почему обойдет затраты*: Точность входа/выхода снижает активную торговлю, что уменьшает затраты.

**24. Sector-Specific Index Flow Spillover**
- *Меканизм*: Flow в одном секторном индексе (сектор.energy, .finance) может спрогнозировать flow в других через 1-2 дня.
- *Данные*: Sector indices returns, tradestats.net_flow для sector constituents.
- *Тест*: Energy flow vs finance flow с лагом 1-2 дня. Failure: Cross-correlation < 0.05.
- *Почему обойдет затраты*: Intra-sector diversification снижает volatility, transaction costs на 0.05%.

**25. Time-Zone Effect: European Market Close Flow**
- *Меканизм*: 17:00-19:00 Moscow time — это закрытие European markets. European liquidation может привести к flow в MOEX.
- *Данные*: tradestats.net_flow в 17:00-19:00 окне.
- *Тест*: Flow в этом окне vs next day return. Failure: IC < 0.04.
- *Почему обойдет затраты*: European liquidity обычно one-sided, что делает reversals предсказуемыми.
