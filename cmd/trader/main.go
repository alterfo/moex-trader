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
	"github.com/olegsidorkin/moex-trader/internal/borrowcost"
	brokertinkoff "github.com/olegsidorkin/moex-trader/internal/broker/tinkoff"
	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/drift"
	"github.com/olegsidorkin/moex-trader/internal/executor"
	"github.com/olegsidorkin/moex-trader/internal/features"
	"github.com/olegsidorkin/moex-trader/internal/filltracking"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/algopack"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/news"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/tinkoff"
	"github.com/olegsidorkin/moex-trader/internal/metrics"
	"github.com/olegsidorkin/moex-trader/internal/model"
	"github.com/olegsidorkin/moex-trader/internal/orchestrator"
	"github.com/olegsidorkin/moex-trader/internal/risk"
	"github.com/olegsidorkin/moex-trader/internal/runbook"
	"github.com/olegsidorkin/moex-trader/internal/spread"
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
		runtime, err := newBrokerRuntime(context.Background(), cfg, store, time.Now, defaultBrokerDeps())
		if err != nil {
			return fmt.Errorf("reset kill switch: %w", err)
		}
		defer func() {
			if runtime.closeFn == nil {
				return
			}
			if err := runtime.closeFn(); err != nil {
				log.Printf("close broker client: %v", err)
			}
		}()
		if err := runKillSwitchReset(context.Background(), store, runtime); err != nil {
			return err
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
	driftMonitor, err := newDriftMonitor(cfg, log.Default())
	if err != nil {
		return err
	}
	liveModelSource := modelSource
	if driftMonitor != nil {
		liveModelSource = &driftSignalSource{source: modelSource, monitor: driftMonitor}
	}
	bandSource := orchestrator.NewSignalHysteresisSource(liveModelSource, orchestrator.DefaultSignalHysteresisPolls)
	gatedSource := newNewsGateSignalSource(bandSource, cfg.News, telegramClient, log.Default())
	signalSource := newAlertingSignalSource(gatedSource, telegramClient, log.Default())
	notifier := newDecisionNotifier(telegramClient, log.Default(), cfg.Telegram.SignalTickers)

	historySource := backtest.NewISSSource(cfg.MOEXISSBaseURL, moexClient)
	preflight := newPreflight(cfg, modelSource, historySource, time.Now)
	var spreadTable spread.Table
	events, err := store.ListAllAuditEvents(ctx)
	if err != nil {
		log.Printf("trader: load per-ticker spread audit events: %v", err)
	} else {
		derived, deriveErr := spread.FromAuditEvents(events, spread.Options{})
		if deriveErr != nil {
			log.Printf("trader: derive per-ticker spreads: %v", deriveErr)
		} else {
			spreadTable = derived
			preflight.withSpreadPcts(map[string]decimal.Decimal(derived))
			log.Printf("trader: loaded %d per-ticker half-spreads from %s", len(derived), cfg.Storage.Path)
		}
	}
	if err := preflight.check(ctx); err != nil {
		return err
	}

	riskConfig := risk.DefaultConfig()
	riskConfig.MaxLots = cfg.Risk.MaxLots
	riskConfig.Positions = store
	if runtime.positionReader != nil {
		riskConfig.Positions = runtime.positionReader
	}
	riskConfig.Store = store
	riskConfig.Alerter = telegramClient
	riskConfig.Canceller = runtime.canceller
	breaker := risk.NewTickerBreaker(risk.TickerBreakerConfig{
		MaxConsecutiveLosses: cfg.Risk.CircuitBreakerMaxLosses,
		MaxCumulativeLossPct: cfg.Risk.CircuitBreakerMaxLossPct,
		Notional:             cfg.Risk.TargetNotional,
	})
	seedTickerBreaker(breaker, events, log.Default())
	riskConfig.Breaker = breaker
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
	if polarity := loadNewsPolarity(cfg, log.Default()); polarity != nil {
		featureBuilder.SetNewsPolarity(polarity)
	}
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
		Observer:         &fanoutObserver{observers: []orchestrator.DecisionObserver{notifier, &breakerFillObserver{breaker: breaker, logger: log.Default()}, &fillQualityObserver{logger: log.Default()}}},
		KillSwitch:       store,
		Shadow:           shadow,
		ShadowDigestPath: shadowDigestPath,
	})
	if err != nil {
		return fmt.Errorf("create orchestrator: %w", err)
	}

	qualityReporter := &executionQualityReporter{
		store:          store,
		spreads:        spreadTable,
		maxSlippagePct: cfg.Risk.MaxSlippagePct,
		borrowSource:   runtime.borrowSource,
		logger:         log.Default(),
	}
	go runExecutionQualityTracking(ctx, qualityReporter)

	log.Printf("starting trader: tickers=%d broker=%s paper=%v poll_interval=%s", len(cfg.Tickers), cfg.Broker, cfg.IsPaperTrading, cfg.PollInterval.Std())
	log.Printf("telegram alerts: enabled=%v signal_tickers=%v", telegramClient.Enabled(), cfg.Telegram.SignalTickers)
	if cfg.IsPaperTrading {
		log.Printf("paper trading mode: account-based risk limits are disabled; max-lot and fat-finger checks still apply")
	} else {
		log.Printf("tinkoff sandbox mode: orders are sent to the sandbox; account-based risk limits are enforced")
	}
	if cfg.Telegram.DailySummaryEnabled && telegramClient.Enabled() {
		fire, err := time.Parse("15:04", cfg.Telegram.DailySummaryTime)
		if err != nil {
			return fmt.Errorf("parse daily summary time %q: %w", cfg.Telegram.DailySummaryTime, err)
		}
		fireOfDay := time.Duration(fire.Hour())*time.Hour + time.Duration(fire.Minute())*time.Minute
		go runDailySummary(ctx, dailySummaryConfig{
			store:   store,
			history: historySource,
			tickers: cfg.Tickers,
			alerter: telegramClient,
			fire:    fireOfDay,
			now:     time.Now,
			logger:  log.Default(),
		})
		log.Printf("daily summary: enabled, fires at %s MSK after the close (IMOEX + green tickers, bot day P&L)", cfg.Telegram.DailySummaryTime)
	} else {
		log.Printf("daily summary: disabled (config=%v telegram_enabled=%v)", cfg.Telegram.DailySummaryEnabled, telegramClient.Enabled())
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

func loadNewsPolarity(cfg *config.Config, logger *log.Logger) func(string) decimal.Decimal {
	if cfg == nil {
		return nil
	}
	path := strings.TrimSpace(cfg.News.ClassifierPath)
	if path == "" {
		logger.Printf("trader: news.classifier_path is empty; falling back to keyword lexicon sentiment")
		return nil
	}
	weights, err := model.LoadNewsClassifier(path)
	if err != nil {
		logger.Printf("trader: load news classifier %q failed: %v; falling back to keyword lexicon sentiment", path, err)
		return nil
	}
	logger.Printf("trader: loaded news classifier from %s (vocab=%d, trained_at=%s)", path, len(weights.Vocab), weights.TrainedAt.Format(time.RFC3339))
	return weights.Scorer()
}

// newDriftMonitor builds a PSI drift monitor from the ensemble model's
// logistic scaler statistics. Features with a zero learned coefficient are
// skipped because they do not influence the model (structurally-zero event
// features and order_book_imbalance fall into this group), avoiding spurious
// drift warnings on placeholder columns.
func newDriftMonitor(cfg *config.Config, logger *log.Logger) (*drift.Monitor, error) {
	if cfg == nil || strings.TrimSpace(cfg.Model.EnsemblePath) == "" {
		return nil, nil
	}
	m, err := model.LoadEnsembleModel(cfg.Model.EnsemblePath)
	if err != nil {
		return nil, fmt.Errorf("load ensemble model for drift monitoring: %w", err)
	}
	features := make([]string, 0, len(m.FeatureOrder))
	mean := make([]float64, 0, len(m.FeatureOrder))
	std := make([]float64, 0, len(m.FeatureOrder))
	for i, name := range m.FeatureOrder {
		if i >= len(m.Logistic.Coef) || i >= len(m.Logistic.Mean) || i >= len(m.Logistic.Std) || m.Logistic.Coef[i] == 0 {
			continue
		}
		features = append(features, name)
		mean = append(mean, m.Logistic.Mean[i])
		std = append(std, m.Logistic.Std[i])
	}
	reference, err := drift.NormalReference(features, mean, std, 10)
	if err != nil {
		return nil, err
	}
	window := cfg.Risk.DriftPSIWindow
	if window <= 0 {
		window = 256
	}
	return drift.NewMonitor(reference, cfg.Risk.DriftPSIThreshold, window, window, logger), nil
}

// driftSignalSource observes every live feature vector through the PSI monitor
// and then delegates to the wrapped model. It emits warnings only; it never
// changes the decision.
type driftSignalSource struct {
	source  orchestrator.SignalSource
	monitor *drift.Monitor
}

func (s *driftSignalSource) Generate(ctx context.Context, feature domain.FeatureContext) (domain.TradeSignal, error) {
	if s.monitor != nil {
		vector, order := model.ToVector(feature)
		s.monitor.Observe(vector, order)
	}
	return s.source.Generate(ctx, feature)
}

// fanoutObserver fans a decision out to multiple observers.
type fanoutObserver struct {
	observers []orchestrator.DecisionObserver
}

func (f *fanoutObserver) Observe(ctx context.Context, decision orchestrator.Decision) {
	for _, observer := range f.observers {
		if observer != nil {
			observer.Observe(ctx, decision)
		}
	}
}

// breakerFillObserver records executed fills into the per-ticker circuit
// breaker and logs a one-off line when a ticker first trips.
type breakerFillObserver struct {
	breaker  *risk.TickerBreaker
	logger   *log.Logger
	mu       sync.Mutex
	notified map[string]bool
}

func (o *breakerFillObserver) Observe(_ context.Context, decision orchestrator.Decision) {
	if o.breaker == nil || !decision.Approved || decision.Err != nil || decision.Fill.Lots <= 0 {
		return
	}
	o.breaker.RecordFill(risk.Fill{
		Ticker:     decision.Fill.Ticker,
		Action:     decision.Fill.Action,
		Lots:       decision.Fill.Lots,
		Price:      decision.Fill.Price,
		Commission: decision.Fill.Commission,
	})
	ticker := decision.Fill.Ticker
	if !o.breaker.Blocked(ticker) || o.logger == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.notified == nil {
		o.notified = make(map[string]bool)
	}
	if o.notified[ticker] {
		return
	}
	o.notified[ticker] = true
	o.logger.Printf("trader: circuit breaker tripped for %s: %s", ticker, o.breaker.Reason(ticker))
}

// fillQualityObserver logs the expected (bounded limit) versus actual fill
// price for every executed fill and counts broker rejections in real time, so
// the marketable-limit cap's fill quality is visible without waiting for the
// periodic digest.
type fillQualityObserver struct {
	logger   *log.Logger
	mu       sync.Mutex
	filled   int
	rejected int
	other    int
}

func (o *fillQualityObserver) Observe(_ context.Context, decision orchestrator.Decision) {
	if !decision.Approved {
		return
	}
	if decision.Err != nil {
		if strings.Contains(decision.Err.Error(), "rejected") {
			o.mu.Lock()
			o.rejected++
			total := o.filled + o.rejected + o.other
			o.mu.Unlock()
			if o.logger != nil {
				o.logger.Printf("trader: order rejected for %s: %v (rejected %d/%d)", decision.Ticker, decision.Err, o.rejected, total)
			}
		} else {
			o.mu.Lock()
			o.other++
			o.mu.Unlock()
		}
		return
	}
	fill := decision.Fill
	if fill.Lots <= 0 || fill.Action == domain.ActionHold {
		return
	}
	if !fill.ExpectedPrice.IsPositive() {
		return
	}
	bps := filltracking.SlippageBps(fill.ExpectedPrice, fill.Price, fill.Action)
	o.mu.Lock()
	o.filled++
	o.mu.Unlock()
	if o.logger != nil {
		o.logger.Printf("trader: fill %s %s %d lot(s): expected %s, actual %s (%s bps %s)",
			fill.Ticker, fill.Action, fill.Lots, fill.ExpectedPrice.String(), fill.Price.String(), bps.Round(2).String(), slippageWord(bps))
	}
}

func slippageWord(bps decimal.Decimal) string {
	if bps.Sign() < 0 {
		return "worse"
	}
	return "better"
}

const (
	executionQualityReportInterval = 6 * time.Hour
	borrowTrackLookbackDays        = 180
)

// executionQualityReporter emits a periodic digest of live fill quality and
// cross-references it against the Task 11 borrow stress and the Task 12
// per-ticker spread measurements.
type executionQualityReporter struct {
	store          *storage.Store
	spreads        spread.Table
	maxSlippagePct decimal.Decimal
	borrowSource   borrowFeeSource
	logger         *log.Logger
}

func (r *executionQualityReporter) Report(ctx context.Context) {
	events, err := r.store.ListAllAuditEvents(ctx)
	if err != nil {
		r.logger.Printf("trader: load execution-quality audit events: %v", err)
		return
	}
	digest := filltracking.FromAuditEvents(events, filltracking.Options{MaxSlippagePct: r.maxSlippagePct})
	r.logger.Printf("trader: execution quality: fills=%d rejected=%d submitted=%d rejection_rate=%s mean_slippage_bps=%s max_adverse_bps=%s max_favorable_bps=%s cap=%s",
		digest.FillCount(), digest.Rejected, digest.Submitted,
		digest.RejectionRate().Round(6).String(),
		digest.MeanSlippageBps().Round(2).String(),
		digest.MaxAdverseSlippageBps().Round(2).String(),
		digest.MaxFavorableSlippageBps().Round(2).String(),
		r.maxSlippagePct.String())
	r.reportSpreadCrossReference(digest)
	r.reportBorrowCrossReference(ctx)
}

func (r *executionQualityReporter) reportSpreadCrossReference(digest filltracking.Digest) {
	if len(r.spreads) == 0 {
		return
	}
	for ticker, bps := range digest.MeanSlippageBpsByTicker() {
		halfSpread, ok := r.spreads.Lookup(ticker)
		if !ok {
			continue
		}
		r.logger.Printf("trader: execution quality %s: observed slippage %s bps vs measured half-spread %s bps",
			ticker, bps.Round(2).String(), halfSpread.Mul(decimal.NewFromInt(10000)).Round(2).String())
	}
}

func (r *executionQualityReporter) reportBorrowCrossReference(ctx context.Context) {
	if r.borrowSource == nil {
		return
	}
	from := time.Now().AddDate(0, 0, -borrowTrackLookbackDays)
	fees, count, err := r.borrowSource.MarginFees(ctx, from, time.Now())
	if err != nil {
		r.logger.Printf("trader: query actual borrow charges: %v", err)
		return
	}
	r.logger.Printf("trader: actual short-borrow charges: %s RUB across %d margin-fee ops (Task 11 stress fallback is %s%%/day)",
		fees.Round(2).String(), count, borrowcost.StressPctPerDay)
}

func runExecutionQualityTracking(ctx context.Context, reporter *executionQualityReporter) {
	if reporter == nil {
		return
	}
	reporter.Report(ctx)
	ticker := time.NewTicker(executionQualityReportInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reporter.Report(ctx)
		}
	}
}

type brokerRuntime struct {
	exec           executor.Executor
	accountSource  orchestrator.AccountSource
	canceller      risk.OrderCanceller
	borrowSource   borrowFeeSource
	positionReader risk.PositionReader
	closeFn        func() error
}

type borrowFeeSource interface {
	MarginFees(ctx context.Context, from, to time.Time) (decimal.Decimal, int, error)
}

type openNotionalSource interface {
	MaxOpenPositionNotional(ctx context.Context) (decimal.Decimal, error)
}

func runKillSwitchReset(ctx context.Context, store *storage.Store, runtime *brokerRuntime) error {
	if runtime == nil || runtime.accountSource == nil {
		if err := store.SetKillSwitchActive(ctx, false); err != nil {
			return fmt.Errorf("kill switch reset: %w", err)
		}
		log.Printf("kill switch reset: no broker account source (paper mode); resetting without the equity preflight check")
		return nil
	}
	account, err := runtime.accountSource.Snapshot(ctx)
	if err != nil {
		return fmt.Errorf("kill switch reset: account snapshot: %w", err)
	}
	maxNotional := decimal.Zero
	if source, ok := runtime.accountSource.(openNotionalSource); ok {
		maxNotional, err = source.MaxOpenPositionNotional(ctx)
		if err != nil {
			return fmt.Errorf("kill switch reset: open position notional: %w", err)
		}
	}
	decision := runbook.Decide(account.CurrentEquity, maxNotional)
	if !decision.Allowed {
		return fmt.Errorf("kill switch reset refused: equity %s is below required %s (1.5x max open-position notional %s); liquidate or reduce positions first",
			decision.Equity.String(), decision.RequiredEquity.String(), decision.MaxOpenNotional.String())
	}
	if err := store.SetKillSwitchActive(ctx, false); err != nil {
		return fmt.Errorf("kill switch reset: %w", err)
	}
	return nil
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
		cooldownExecutor := executor.NewRejectionCooldownExecutor(windowedExecutor, now, rejectedOrderThreshold, rejectedOrderCooldown, log.Default())
		positionReader := newReconcilingPositionReader(sandbox, store, cfg.Tickers, positionReconcileTTL, now, log.Default())
		targetExec := executor.NewTargetPositionExecutorWithConfig(cooldownExecutor, positionReader, now, executor.TargetPositionConfig{
			RebalanceMinDeviationPct: cfg.Risk.RebalanceMinDeviationPct,
		})
		return &brokerRuntime{
			exec:           targetExec,
			accountSource:  sandbox,
			canceller:      sandbox,
			borrowSource:   sandbox,
			positionReader: positionReader,
			closeFn:        sandbox.Close,
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
	borrowPctPerDay decimal.Decimal
	maxLots         int
	commissionRate  decimal.Decimal
	tickers         []string
	source          backtest.SignalSource
	history         backtest.HistoricalSource
	spreadPcts      map[string]decimal.Decimal
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
		borrowPctPerDay: borrowcost.StressRatePerDay(),
		maxLots:         cfg.Risk.MaxLots,
		commissionRate:  cfg.Commission.Rate,
		tickers:         append([]string(nil), cfg.Tickers...),
		source:          source,
		history:         history,
		now:             now,
		configHash:      preflightConfigHash(cfg),
	}
}

func (p *preflight) withSpreadPcts(table map[string]decimal.Decimal) *preflight {
	if p == nil {
		return p
	}
	p.spreadPcts = table
	return p
}

func preflightConfigHash(cfg *config.Config) string {
	h := sha256.New()
	fmt.Fprintf(h, "tickers=%v|model=%s|ensemble=%s|preflight_days=%d|deposit=%s|min_net_pnl=%s|min_closed_trades=%d|spread=%s|slippage=%s|borrow_pct_per_day=%s|max_lots=%d|target_notional=%s|commission=%s",
		cfg.Tickers,
		cfg.Model.Path,
		cfg.Model.EnsemblePath,
		cfg.Preflight.Days,
		cfg.Preflight.Deposit.String(),
		cfg.Preflight.MinNetPnL.String(),
		cfg.Preflight.MinClosedTrades,
		cfg.Preflight.SpreadPct.String(),
		cfg.Preflight.SlippagePct.String(),
		borrowcost.StressRatePerDay().String(),
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
		Tickers:         p.tickers,
		From:            from,
		Till:            till,
		Deposit:         p.deposit,
		MaxLots:         p.maxLots,
		CommissionRate:  p.commissionRate,
		SpreadPct:       p.spreadPct,
		SpreadPcts:      p.spreadPcts,
		SlippagePct:     p.slippagePct,
		BorrowPctPerDay: p.borrowPctPerDay,
		KillSwitch:      true,
		SignalSource:    p.source,
		Source:          p.history,
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
	if result.RealizedPnlNetBorrow.LessThan(p.minNetPnL) {
		return fmt.Errorf("preflight rejected the configuration: realized P&L net of borrow %s over %d days (gross realized=%s, borrow=%s, closed trades=%d, hit rate=%.1f%%, max drawdown=%.2f%%, MTM=%s) is below the minimum %s; refusing to start (config_hash=%s)",
			result.RealizedPnlNetBorrow.StringFixed(2), p.days, result.RealizedPnl.StringFixed(2), result.TotalBorrow.StringFixed(2), result.ClosedTrades, result.HitRate*100, result.MaxDrawdownPct, result.NetPnl.StringFixed(2), p.minNetPnL.StringFixed(2), p.configHash)
	}
	log.Printf("preflight passed: realized P&L net of borrow %s over %d days (gross realized=%s, borrow=%s, closed trades=%d, hit rate=%.1f%%, max drawdown=%.2f%%, MTM=%s, decisions=%d, config_hash=%s)",
		result.RealizedPnlNetBorrow.StringFixed(2), p.days, result.RealizedPnl.StringFixed(2), result.TotalBorrow.StringFixed(2), result.ClosedTrades, result.HitRate*100, result.MaxDrawdownPct, result.NetPnl.StringFixed(2), result.Decisions, p.configHash)
	return nil
}
