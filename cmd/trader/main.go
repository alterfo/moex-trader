package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/shopspring/decimal"
	pb "github.com/tinkoff/invest-api-go-sdk/proto"

	"github.com/olegsidorkin/moex-trader/internal/alert/telegram"
	"github.com/olegsidorkin/moex-trader/internal/backtest"
	brokertinkoff "github.com/olegsidorkin/moex-trader/internal/broker/tinkoff"
	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/executor"
	"github.com/olegsidorkin/moex-trader/internal/features"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/algopack"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/news"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/tinkoff"
	"github.com/olegsidorkin/moex-trader/internal/metrics"
	"github.com/olegsidorkin/moex-trader/internal/model"
	"github.com/olegsidorkin/moex-trader/internal/orchestrator"
	"github.com/olegsidorkin/moex-trader/internal/risk"
	"github.com/olegsidorkin/moex-trader/internal/storage"
)

const tinkoffAppName = "moex-trader"

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var configPath string
	var metricsAddr string
	var shadowDigestPath string
	var resetKillSwitch bool
	flag.StringVar(&configPath, "config", "config.yaml", "path to config YAML")
	flag.StringVar(&metricsAddr, "metrics-addr", ":9090", "address for Prometheus /metrics endpoint")
	flag.StringVar(&shadowDigestPath, "shadow-digest", "", "write the shadow reconciliation digest to this markdown file")
	flag.BoolVar(&resetKillSwitch, "reset-kill-switch", false, "reset persisted kill switch and exit")
	flag.Parse()

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	store, err := storage.Open(cfg.Storage.Path)
	if err != nil {
		return fmt.Errorf("open storage: %w", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			log.Printf("close storage: %v", err)
		}
	}()

	if resetKillSwitch {
		if err := store.SetKillSwitchActive(context.Background(), false); err != nil {
			return fmt.Errorf("reset kill switch: %w", err)
		}
		log.Printf("kill switch reset")
		return nil
	}

	moexClient := moex.NewClient(cfg.MOEXISSBaseURL, nil)
	newsProxy := cfg.News.Proxy
	if newsProxy == "" {
		newsProxy = cfg.Telegram.Proxy
	}
	fetcher := news.NewFetcher(newNewsHTTPClient(newsProxy))
	matcher := news.NewMatcher(news.DefaultAliases())
	var algopackFetcher algopack.Fetcher
	if strings.TrimSpace(cfg.AlgoPackToken) != "" {
		algopackFetcher = algopack.NewHTTPFetcher(cfg.AlgoPackBaseURL, cfg.AlgoPackToken, nil)
	} else {
		log.Printf("algopack enrichment disabled: MOEX_TRADER_ALGOPACK_TOKEN is not set")
	}
	ingestor := orchestrator.NewMOEXIngestor(moexClient, fetcher, matcher, news.DefaultSources(), algopackFetcher)

	telegramHTTPClient, err := newTelegramHTTPClient(cfg.Telegram.Proxy)
	if err != nil {
		return err
	}
	telegramClient := telegram.New(cfg.Telegram.BotToken, cfg.Telegram.ChatID, telegramHTTPClient)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runtime, err := newBrokerRuntime(ctx, cfg, store, time.Now, defaultBrokerDeps())
	if err != nil {
		return err
	}
	defer func() {
		if runtime.closeFn == nil {
			return
		}
		if err := runtime.closeFn(); err != nil {
			log.Printf("close broker client: %v", err)
		}
	}()

	modelSource, err := newModelSignalSource(cfg)
	if err != nil {
		return err
	}
	shadowSource := modelSource
	if resolver, ok := runtime.accountSource.(lotSizeResolver); ok {
		modelSource = newLotSizeSignalSource(modelSource, resolver, log.Default())
	}
	bandSource := orchestrator.NewSignalHysteresisSource(modelSource, orchestrator.DefaultSignalHysteresisPolls)
	gatedSource := newNewsGateSignalSource(bandSource, cfg.News, telegramClient, log.Default())
	signalSource := newAlertingSignalSource(gatedSource, telegramClient, log.Default())
	notifier := newDecisionNotifier(telegramClient, log.Default(), cfg.Telegram.SignalTickers)

	historySource := backtest.NewISSSource(cfg.MOEXISSBaseURL, moexClient)
	if err := newPreflight(cfg, modelSource, historySource, time.Now).check(ctx); err != nil {
		return err
	}

	riskConfig := risk.DefaultConfig()
	riskConfig.MaxLots = cfg.Risk.MaxLots
	riskConfig.Positions = store
	riskConfig.Store = store
	riskConfig.Alerter = telegramClient
	riskConfig.Canceller = runtime.canceller
	gate, err := risk.NewHardenedGate(riskConfig)
	if err != nil {
		return fmt.Errorf("create risk gate: %w", err)
	}

	appMetrics := metrics.New()

	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", appMetrics.Handler())
	metricsServer := &http.Server{Addr: metricsAddr, Handler: metricsMux}
	go func() {
		if err := metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("metrics server: %v", err)
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := metricsServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("metrics server shutdown: %v", err)
		}
	}()

	featureBuilder := features.NewBuilder(time.Now)
	var shadow *orchestrator.ShadowReconciler
	if strings.TrimSpace(shadowDigestPath) != "" {
		shadow = orchestrator.NewShadowReconciler(featureBuilder, shadowSource)
	}

	orch, err := orchestrator.New(orchestrator.Options{
		Tickers:          cfg.Tickers,
		Ingestor:         ingestor,
		Builder:          featureBuilder,
		Source:           signalSource,
		Gate:             gate,
		Executor:         runtime.exec,
		Audit:            store,
		PollInterval:     cfg.PollInterval.Std(),
		Metrics:          appMetrics,
		AccountSource:    runtime.accountSource,
		Observer:         notifier,
		KillSwitch:       store,
		Shadow:           shadow,
		ShadowDigestPath: shadowDigestPath,
	})
	if err != nil {
		return fmt.Errorf("create orchestrator: %w", err)
	}

	log.Printf("starting trader: tickers=%d broker=%s paper=%v poll_interval=%s", len(cfg.Tickers), cfg.Broker, cfg.IsPaperTrading, cfg.PollInterval.Std())
	log.Printf("telegram alerts: enabled=%v signal_tickers=%v", telegramClient.Enabled(), cfg.Telegram.SignalTickers)
	if cfg.IsPaperTrading {
		log.Printf("paper trading mode: account-based risk limits are disabled; max-lot and fat-finger checks still apply")
	} else {
		log.Printf("tinkoff sandbox mode: orders are sent to the sandbox; account-based risk limits are enforced")
	}
	orch.Run(ctx)
	log.Printf("trader stopped")
	return nil
}

type signalFailureAlerter interface {
	Send(ctx context.Context, text string) error
}

// lotSizeResolver looks up the exchange lot size for a ticker (shares per
// lot) so notional-based sizing (Risk.TargetNotional) computes the right
// number of lots instead of treating price-per-share as price-per-lot.
type lotSizeResolver interface {
	ResolveLotSize(ctx context.Context, ticker string) (decimal.Decimal, error)
}

type lotSizeSignalSource struct {
	source   orchestrator.SignalSource
	resolver lotSizeResolver
	logger   *log.Logger
}

func newLotSizeSignalSource(source orchestrator.SignalSource, resolver lotSizeResolver, logger *log.Logger) *lotSizeSignalSource {
	if logger == nil {
		logger = log.Default()
	}
	return &lotSizeSignalSource{source: source, resolver: resolver, logger: logger}
}

func (l *lotSizeSignalSource) Generate(ctx context.Context, feature domain.FeatureContext) (domain.TradeSignal, error) {
	if l.resolver != nil && !feature.LotSize.IsPositive() {
		lot, err := l.resolver.ResolveLotSize(ctx, feature.Ticker)
		if err != nil {
			l.logger.Printf("trader: resolve lot size for %s: %v", feature.Ticker, err)
		} else {
			feature.LotSize = lot
		}
	}
	return l.source.Generate(ctx, feature)
}

type alertingSignalSource struct {
	source  orchestrator.SignalSource
	alerter signalFailureAlerter
	logger  *log.Logger
}

func newAlertingSignalSource(source orchestrator.SignalSource, alerter signalFailureAlerter, logger *log.Logger) *alertingSignalSource {
	if logger == nil {
		logger = log.Default()
	}
	return &alertingSignalSource{source: source, alerter: alerter, logger: logger}
}

func (a *alertingSignalSource) Generate(ctx context.Context, feature domain.FeatureContext) (domain.TradeSignal, error) {
	signal, err := a.source.Generate(ctx, feature)
	if err != nil && a.alerter != nil {
		text := fmt.Sprintf("MOEX trader: signal generation failed for %s: %v", feature.Ticker, err)
		if alertErr := a.alerter.Send(ctx, text); alertErr != nil {
			a.logger.Printf("trader: alert signal failure for %s: %v", feature.Ticker, alertErr)
		}
	}
	return signal, err
}

const (
	holdReasonNews = "news" // signal gated by a conflicting real-time news item
)

type newsGateSignalSource struct {
	source      orchestrator.SignalSource
	alerter     signalFailureAlerter
	logger      *log.Logger
	enabled     bool
	sentiment   decimal.Decimal
	minPositive int
}

func newNewsGateSignalSource(source orchestrator.SignalSource, newsCfg config.News, alerter signalFailureAlerter, logger *log.Logger) *newsGateSignalSource {
	if logger == nil {
		logger = log.Default()
	}
	return &newsGateSignalSource{
		source:      source,
		alerter:     alerter,
		logger:      logger,
		enabled:     newsCfg.VetoEnabled,
		sentiment:   newsCfg.VetoSentiment,
		minPositive: newsCfg.VetoMinCount,
	}
}

func (g *newsGateSignalSource) Generate(ctx context.Context, feature domain.FeatureContext) (domain.TradeSignal, error) {
	signal, err := g.source.Generate(ctx, feature)
	if err != nil {
		return signal, err
	}
	if !g.enabled || feature.NewsCount < g.minPositive || feature.NewsCount == 0 {
		return signal, nil
	}
	if signal.Action == domain.ActionBuy && feature.NewsSentiment.LessThan(g.sentiment.Neg()) {
		text := fmt.Sprintf("⚠️ MOEX trader: BUY %s отменён новостным вето\nсен: %.2f (новостей: %d), порог veto: %.2f",
			feature.Ticker, feature.NewsSentiment.InexactFloat64(), feature.NewsCount, g.sentiment.Neg().InexactFloat64())
		signal.Action = domain.ActionHold
		signal.HoldReason = holdReasonNews
		signal.Reasoning = "конфликтная негативная новость"
		signal.TargetLots = 0
		g.logger.Printf("trader: news veto BUY %s (sentiment %.2f, count %d)", feature.Ticker, feature.NewsSentiment.InexactFloat64(), feature.NewsCount)
		if g.alerter != nil {
			if alertErr := g.alerter.Send(ctx, text); alertErr != nil {
				g.logger.Printf("trader: news veto alert for %s: %v", feature.Ticker, alertErr)
			}
		}
		return signal, nil
	}
	if signal.Action == domain.ActionSell && feature.NewsSentiment.GreaterThan(g.sentiment) {
		text := fmt.Sprintf("⚠️ MOEX trader: SELL %s отменён новостным вето\nсен: %.2f (новостей: %d), порог veto: %.2f",
			feature.Ticker, feature.NewsSentiment.InexactFloat64(), feature.NewsCount, g.sentiment.InexactFloat64())
		signal.Action = domain.ActionHold
		signal.HoldReason = holdReasonNews
		signal.Reasoning = "конфликтная позитивная новость"
		signal.TargetLots = 0
		g.logger.Printf("trader: news veto SELL %s (sentiment %.2f, count %d)", feature.Ticker, feature.NewsSentiment.InexactFloat64(), feature.NewsCount)
		if g.alerter != nil {
			if alertErr := g.alerter.Send(ctx, text); alertErr != nil {
				g.logger.Printf("trader: news veto alert for %s: %v", feature.Ticker, alertErr)
			}
		}
		return signal, nil
	}
	return signal, nil
}

type decisionNotifier struct {
	alerter signalFailureAlerter
	logger  *log.Logger
	tickers map[string]struct{}

	mu   sync.Mutex
	last map[string]string
}

func newDecisionNotifier(alerter signalFailureAlerter, logger *log.Logger, tickers []string) *decisionNotifier {
	if logger == nil {
		logger = log.Default()
	}
	filter := make(map[string]struct{}, len(tickers))
	for _, ticker := range tickers {
		key := strings.ToUpper(strings.TrimSpace(ticker))
		if key != "" {
			filter[key] = struct{}{}
		}
	}
	return &decisionNotifier{
		alerter: alerter,
		logger:  logger,
		tickers: filter,
		last:    make(map[string]string),
	}
}

func (n *decisionNotifier) Observe(ctx context.Context, decision orchestrator.Decision) {
	if n.alerter == nil || decision.Signal.Action == domain.ActionHold {
		return
	}
	key := strings.ToUpper(strings.TrimSpace(decision.Ticker))
	if len(n.tickers) > 0 {
		if _, ok := n.tickers[key]; !ok {
			return
		}
	}

	outcome := decisionOutcome(decision)
	dedupKey := string(decision.Signal.Action) + "|" + outcome
	n.mu.Lock()
	changed := n.last[key] != dedupKey
	n.last[key] = dedupKey
	n.mu.Unlock()
	if outcome != "filled" || !changed {
		return
	}

	text := decisionMessage(decision, outcome)
	if err := n.alerter.Send(ctx, text); err != nil {
		n.logger.Printf("trader: notify %s decision for %s: %v", decision.Signal.Action, decision.Ticker, err)
	}
}

func decisionOutcome(decision orchestrator.Decision) string {
	switch {
	case !decision.Approved:
		return "rejected"
	case decision.Err != nil:
		return "error"
	case decision.Fill.Lots <= 0:
		return "skipped"
	default:
		return "filled"
	}
}

func decisionMessage(decision orchestrator.Decision, outcome string) string {
	return fmt.Sprintf("%s → %s\n%s ₽ · лоты: %d · уверенность %s\n%s\n%s",
		decision.Ticker,
		decision.Signal.Action,
		decision.Price,
		decision.Signal.TargetLots,
		decision.Signal.Confidence.StringFixed(2),
		decisionDetails(decision.Signal),
		decisionStatus(decision, outcome),
	)
}

func decisionDetails(signal domain.TradeSignal) string {
	reasoning := strings.TrimSpace(signal.Reasoning)
	if reasoning == "" {
		return "p=n/a"
	}
	return strings.TrimPrefix(reasoning, "ensemble:")
}

func decisionStatus(decision orchestrator.Decision, outcome string) string {
	switch outcome {
	case "rejected":
		return "не исполнено — риск-гейт"
	case "error":
		return fmt.Sprintf("не исполнено — ошибка: %v", decision.Err)
	case "skipped":
		return "не исполнено — биржа закрыта"
	default:
		return fmt.Sprintf("исполнено — %d лот(а) @ %s ₽", decision.Fill.Lots, decision.Fill.Price)
	}
}

func newTelegramHTTPClient(proxy string) (*http.Client, error) {
	proxy = strings.TrimSpace(proxy)
	if proxy == "" {
		return nil, nil
	}
	proxyURL, err := url.Parse(proxy)
	if err != nil {
		return nil, fmt.Errorf("parse telegram proxy %q: %w", proxy, err)
	}
	return &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
	}, nil
}

func newNewsHTTPClient(proxy string) *http.Client {
	proxy = strings.TrimSpace(proxy)
	if proxy == "" {
		return nil
	}
	proxyURL, err := url.Parse(proxy)
	if err != nil {
		log.Printf("trader: parse news proxy %q: %v (falling back to direct)", proxy, err)
		return nil
	}
	return &http.Client{
		Timeout:   60 * time.Second,
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
	}
}

func newModelSignalSource(cfg *config.Config) (orchestrator.SignalSource, error) {
	if cfg == nil {
		return nil, fmt.Errorf("load model: config is nil")
	}
	if strings.TrimSpace(cfg.Model.EnsemblePath) != "" {
		m, err := model.LoadEnsembleModel(cfg.Model.EnsemblePath)
		if err != nil {
			return nil, fmt.Errorf("load ensemble model: %w", err)
		}
		return &model.EnsembleSignalSource{Model: m, MaxLots: cfg.Risk.MaxLots, TargetNotional: cfg.Risk.TargetNotional}, nil
	}
	weights, err := model.LoadWeights(cfg.Model.Path)
	if err != nil {
		return nil, fmt.Errorf("load model: %w", err)
	}
	return &model.SignalSource{Weights: weights, MaxLots: cfg.Risk.MaxLots}, nil
}

type brokerRuntime struct {
	exec          executor.Executor
	accountSource orchestrator.AccountSource
	canceller     risk.OrderCanceller
	closeFn       func() error
}

type brokerDeps struct {
	dialTinkoff func(ctx context.Context, cfg tinkoff.Config) (brokertinkoff.Client, error)
}

func defaultBrokerDeps() brokerDeps {
	return brokerDeps{
		dialTinkoff: func(ctx context.Context, cfg tinkoff.Config) (brokertinkoff.Client, error) {
			return tinkoff.New(ctx, cfg)
		},
	}
}

func newBrokerRuntime(ctx context.Context, cfg *config.Config, store *storage.Store, now func() time.Time, deps brokerDeps) (*brokerRuntime, error) {
	if cfg == nil {
		return nil, fmt.Errorf("select executor: config is nil")
	}
	switch cfg.Broker {
	case "", config.BrokerPaper, config.BrokerTinkoff, config.BrokerFinam:
	default:
		return nil, fmt.Errorf("select executor: unknown broker %q", cfg.Broker)
	}
	if cfg.IsPaperTrading {
		switch cfg.Broker {
		case "", config.BrokerPaper:
			paperExec := executor.NewPaperExecutorWithCommission(store, now, cfg.Commission.Rate)
			targetExec := executor.NewTargetPositionExecutorWithConfig(paperExec, store, now, executor.TargetPositionConfig{
				RebalanceMinDeviationPct: cfg.Risk.RebalanceMinDeviationPct,
			})
			return &brokerRuntime{exec: targetExec}, nil
		default:
			return nil, fmt.Errorf("select executor: broker %q is not allowed while is_paper_trading is true; set broker: paper", cfg.Broker)
		}
	}
	switch cfg.Broker {
	case config.BrokerTinkoff:
		if !cfg.Tinkoff.Sandbox {
			return nil, fmt.Errorf("live trading mode is not wired for broker %q: set tinkoff.sandbox: true for the sandbox, or broker: paper and is_paper_trading: true", cfg.Broker)
		}
		if deps.dialTinkoff == nil {
			return nil, fmt.Errorf("select executor: tinkoff dialer is not configured")
		}
		client, err := deps.dialTinkoff(ctx, tinkoff.Config{
			Endpoint: cfg.Tinkoff.Endpoint,
			Token:    cfg.Tinkoff.Token,
			AppName:  tinkoffAppName,
		})
		if err != nil {
			return nil, fmt.Errorf("dial tinkoff sandbox: %w", err)
		}
		sandbox, err := brokertinkoff.NewSandbox(client, brokertinkoff.Config{
			AccountID: cfg.Tinkoff.AccountID,
			PayIn:     cfg.Tinkoff.PayIn,
			Now:       now,
		})
		if err != nil {
			_ = client.Close()
			return nil, err
		}
		accountID, err := sandbox.EnsureAccount(ctx)
		if err != nil {
			_ = sandbox.Close()
			return nil, fmt.Errorf("tinkoff sandbox account: %w", err)
		}
		orderType, err := orderTypeFromConfig(cfg.Tinkoff.OrderType)
		if err != nil {
			_ = sandbox.Close()
			return nil, err
		}
		liveExecutor, err := executor.NewLiveExecutor(sandbox, executor.LiveConfig{
			AccountID:           accountID,
			ResolveInstrumentID: sandbox.ResolveInstrumentID,
			OrderType:           orderType,
			Store:               store,
			Now:                 now,
			CommissionRate:      cfg.Commission.Rate,
			MaxSlippagePct:      cfg.Risk.MaxSlippagePct,
		})
		if err != nil {
			_ = sandbox.Close()
			return nil, err
		}
		guardedExecutor, err := newMarketHoursExecutor(liveExecutor, sandbox, now, log.Default())
		if err != nil {
			_ = sandbox.Close()
			return nil, err
		}
		windowedExecutor, err := newTradingWindowExecutor(guardedExecutor, now, cfg.Risk.NoTradeAfterOpenMinutes, cfg.Risk.BlackoutWindows, log.Default())
		if err != nil {
			_ = sandbox.Close()
			return nil, err
		}
		log.Printf("tinkoff sandbox: account %s ready; set tinkoff.account_id to reuse it on the next run", accountID)
		targetExec := executor.NewTargetPositionExecutorWithConfig(windowedExecutor, store, now, executor.TargetPositionConfig{
			RebalanceMinDeviationPct: cfg.Risk.RebalanceMinDeviationPct,
		})
		return &brokerRuntime{
			exec:          targetExec,
			accountSource: sandbox,
			canceller:     sandbox,
			closeFn:       sandbox.Close,
		}, nil
	case config.BrokerFinam, config.BrokerPaper:
		return nil, fmt.Errorf("live trading mode is not wired for broker %q: set broker: paper and is_paper_trading: true", cfg.Broker)
	}
	return nil, fmt.Errorf("select executor: unknown broker %q", cfg.Broker)
}

func orderTypeFromConfig(value string) (pb.OrderType, error) {
	switch value {
	case "", config.OrderTypeLimit:
		return pb.OrderType_ORDER_TYPE_LIMIT, nil
	case config.OrderTypeMarket:
		return pb.OrderType_ORDER_TYPE_MARKET, nil
	default:
		return pb.OrderType_ORDER_TYPE_UNSPECIFIED, fmt.Errorf("select executor: unknown tinkoff order type %q", value)
	}
}

type marketHoursExecutor struct {
	inner   executor.Executor
	sandbox *brokertinkoff.Sandbox
	now     func() time.Time
	logger  *log.Logger

	mu     sync.Mutex
	logged map[string]string
}

func newMarketHoursExecutor(inner executor.Executor, sandbox *brokertinkoff.Sandbox, now func() time.Time, logger *log.Logger) (*marketHoursExecutor, error) {
	if inner == nil {
		return nil, fmt.Errorf("market hours executor: inner executor is required")
	}
	if sandbox == nil {
		return nil, fmt.Errorf("market hours executor: sandbox is required")
	}
	if now == nil {
		now = time.Now
	}
	if logger == nil {
		logger = log.Default()
	}
	return &marketHoursExecutor{
		inner:   inner,
		sandbox: sandbox,
		now:     now,
		logger:  logger,
		logged:  make(map[string]string),
	}, nil
}

func (m *marketHoursExecutor) Execute(ctx context.Context, signal domain.TradeSignal, price decimal.Decimal) (executor.Fill, error) {
	if signal.Action == domain.ActionHold {
		return m.inner.Execute(ctx, signal, price)
	}
	open, err := m.sandbox.MarketOpen(ctx, signal.Ticker)
	if err != nil {
		return executor.Fill{}, fmt.Errorf("market hours check for %s: %w", signal.Ticker, err)
	}
	if !open {
		m.logClosed(signal.Ticker)
		return executor.Fill{}, nil
	}
	return m.inner.Execute(ctx, signal, price)
}

func (m *marketHoursExecutor) logClosed(ticker string) {
	day := m.now().Format("2006-01-02")
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.logged[ticker] == day {
		return
	}
	m.logged[ticker] = day
	m.logger.Printf("trader: %s is not available for trading, skipping orders until the next session", ticker)
}

type blackoutWindow struct {
	start, end time.Time
}

type tradingWindowExecutor struct {
	inner      executor.Executor
	now        func() time.Time
	loc        *time.Location
	openWarmup time.Duration
	blackouts  []blackoutWindow
	logger     *log.Logger

	mu     sync.Mutex
	logged map[string]string
}

func newTradingWindowExecutor(inner executor.Executor, now func() time.Time, noTradeAfterOpenMinutes int, blackoutSpecs []string, logger *log.Logger) (*tradingWindowExecutor, error) {
	if inner == nil {
		return nil, fmt.Errorf("trading window executor: inner executor is required")
	}
	if now == nil {
		now = time.Now
	}
	if logger == nil {
		logger = log.Default()
	}
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		loc = time.FixedZone("MSK", 3*3600)
	}
	blackouts := make([]blackoutWindow, 0, len(blackoutSpecs))
	for _, spec := range blackoutSpecs {
		start, end, err := config.ParseBlackoutWindow(spec)
		if err != nil {
			return nil, fmt.Errorf("trading window executor: %w", err)
		}
		blackouts = append(blackouts, blackoutWindow{start: start, end: end})
	}
	return &tradingWindowExecutor{
		inner:      inner,
		now:        now,
		loc:        loc,
		openWarmup: time.Duration(noTradeAfterOpenMinutes) * time.Minute,
		blackouts:  blackouts,
		logger:     logger,
		logged:     make(map[string]string),
	}, nil
}

func (t *tradingWindowExecutor) Execute(ctx context.Context, signal domain.TradeSignal, price decimal.Decimal) (executor.Fill, error) {
	if signal.Action == domain.ActionHold {
		return t.inner.Execute(ctx, signal, price)
	}
	now := t.now()
	if reason, blocked := t.blocked(now); blocked {
		t.logBlocked(signal.Ticker, reason, now)
		return executor.Fill{}, nil
	}
	return t.inner.Execute(ctx, signal, price)
}

func (t *tradingWindowExecutor) blocked(now time.Time) (string, bool) {
	if t.openWarmup > 0 {
		local := now.In(t.loc)
		open := time.Date(local.Year(), local.Month(), local.Day(), 10, 0, 0, 0, t.loc)
		cutoff := open.Add(t.openWarmup)
		if !local.Before(open) && local.Before(cutoff) {
			return fmt.Sprintf("opening cooldown until %s MSK", cutoff.Format("15:04:05")), true
		}
	}
	for _, w := range t.blackouts {
		if !now.Before(w.start) && now.Before(w.end) {
			return fmt.Sprintf("blackout window %s..%s", w.start.Format(time.RFC3339), w.end.Format(time.RFC3339)), true
		}
	}
	return "", false
}

func (t *tradingWindowExecutor) logBlocked(ticker, reason string, now time.Time) {
	key := ticker + "|" + reason
	slot := now.Truncate(5 * time.Minute).Format(time.RFC3339)
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.logged[key] == slot {
		return
	}
	t.logged[key] = slot
	t.logger.Printf("trader: %s order skipped: %s", ticker, reason)
}

type preflight struct {
	enabled         bool
	days            int
	deposit         decimal.Decimal
	minNetPnL       decimal.Decimal
	minClosedTrades int
	spreadPct       decimal.Decimal
	slippagePct     decimal.Decimal
	maxLots         int
	commissionRate  decimal.Decimal
	tickers         []string
	source          backtest.SignalSource
	history         backtest.HistoricalSource
	now             func() time.Time
	configHash      string
}

func newPreflight(cfg *config.Config, source backtest.SignalSource, history backtest.HistoricalSource, now func() time.Time) *preflight {
	if cfg == nil || !cfg.Preflight.Enabled {
		return &preflight{}
	}
	if now == nil {
		now = time.Now
	}
	return &preflight{
		enabled:         true,
		days:            cfg.Preflight.Days,
		deposit:         cfg.Preflight.Deposit,
		minNetPnL:       cfg.Preflight.MinNetPnL,
		minClosedTrades: cfg.Preflight.MinClosedTrades,
		spreadPct:       cfg.Preflight.SpreadPct,
		slippagePct:     cfg.Preflight.SlippagePct,
		maxLots:         cfg.Risk.MaxLots,
		commissionRate:  cfg.Commission.Rate,
		tickers:         append([]string(nil), cfg.Tickers...),
		source:          source,
		history:         history,
		now:             now,
		configHash:      preflightConfigHash(cfg),
	}
}

func preflightConfigHash(cfg *config.Config) string {
	h := sha256.New()
	fmt.Fprintf(h, "tickers=%v|model=%s|ensemble=%s|preflight_days=%d|deposit=%s|min_net_pnl=%s|min_closed_trades=%d|spread=%s|slippage=%s|max_lots=%d|target_notional=%s|commission=%s",
		cfg.Tickers,
		cfg.Model.Path,
		cfg.Model.EnsemblePath,
		cfg.Preflight.Days,
		cfg.Preflight.Deposit.String(),
		cfg.Preflight.MinNetPnL.String(),
		cfg.Preflight.MinClosedTrades,
		cfg.Preflight.SpreadPct.String(),
		cfg.Preflight.SlippagePct.String(),
		cfg.Risk.MaxLots,
		cfg.Risk.TargetNotional.String(),
		cfg.Commission.Rate.String(),
	)
	return hex.EncodeToString(h.Sum(nil))
}

func (p *preflight) check(ctx context.Context) error {
	if p == nil || !p.enabled {
		return nil
	}
	if p.source == nil {
		return fmt.Errorf("preflight: signal source is required")
	}
	if p.history == nil {
		return fmt.Errorf("preflight: historical source is required")
	}

	till := p.now()
	from := till.AddDate(0, 0, -p.days)
	engine, err := backtest.NewEngine(backtest.Config{
		Tickers:        p.tickers,
		From:           from,
		Till:           till,
		Deposit:        p.deposit,
		MaxLots:        p.maxLots,
		CommissionRate: p.commissionRate,
		SpreadPct:      p.spreadPct,
		SlippagePct:    p.slippagePct,
		KillSwitch:     true,
		SignalSource:   p.source,
		Source:         p.history,
	})
	if err != nil {
		return fmt.Errorf("preflight backtest: %w", err)
	}

	log.Printf("preflight backtest: running %d days over %d tickers (config_hash=%s)", p.days, len(p.tickers), p.configHash)
	result, err := engine.Run(ctx)
	if err != nil {
		return fmt.Errorf("preflight backtest: %w", err)
	}
	if result.Decisions == 0 {
		return fmt.Errorf("preflight backtest: no decisions produced, history is unavailable for all %d tickers", len(p.tickers))
	}
	failed := result.HoldReasons[domain.HoldReasonError] + result.HoldReasons[domain.HoldReasonTimeout]
	if failed > 0 {
		log.Printf("preflight backtest: %d of %d decisions failed (errors=%d, timeouts=%d)",
			failed, result.Decisions, result.HoldReasons[domain.HoldReasonError], result.HoldReasons[domain.HoldReasonTimeout])
	}
	if failed == result.Decisions {
		return fmt.Errorf("preflight rejected the configuration: all %d decisions failed (errors=%d, timeouts=%d); check the signal source and the model artifact (config_hash=%s)",
			result.Decisions, result.HoldReasons[domain.HoldReasonError], result.HoldReasons[domain.HoldReasonTimeout], p.configHash)
	}
	if result.ClosedTrades < p.minClosedTrades {
		return fmt.Errorf("preflight rejected the configuration: closed trades=%d is below the required minimum %d over %d days; refusing to start (config_hash=%s)",
			result.ClosedTrades, p.minClosedTrades, p.days, p.configHash)
	}
	if result.RealizedPnl.LessThan(p.minNetPnL) {
		return fmt.Errorf("preflight rejected the configuration: realized P&L %s over %d days (closed trades=%d, hit rate=%.1f%%, max drawdown=%.2f%%, MTM=%s) is below the minimum %s; refusing to start (config_hash=%s)",
			result.RealizedPnl.StringFixed(2), p.days, result.ClosedTrades, result.HitRate*100, result.MaxDrawdownPct, result.NetPnl.StringFixed(2), p.minNetPnL.StringFixed(2), p.configHash)
	}
	log.Printf("preflight passed: realized P&L %s over %d days (closed trades=%d, hit rate=%.1f%%, max drawdown=%.2f%%, MTM=%s, decisions=%d, config_hash=%s)",
		result.RealizedPnl.StringFixed(2), p.days, result.ClosedTrades, result.HitRate*100, result.MaxDrawdownPct, result.NetPnl.StringFixed(2), result.Decisions, p.configHash)
	return nil
}
