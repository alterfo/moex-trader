package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/alert/telegram"
	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/executor"
	"github.com/olegsidorkin/moex-trader/internal/features"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/algopack"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/news"
	"github.com/olegsidorkin/moex-trader/internal/metrics"
	"github.com/olegsidorkin/moex-trader/internal/model"
	"github.com/olegsidorkin/moex-trader/internal/orchestrator"
	"github.com/olegsidorkin/moex-trader/internal/risk"
	"github.com/olegsidorkin/moex-trader/internal/storage"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var configPath string
	var metricsAddr string
	var resetKillSwitch bool
	flag.StringVar(&configPath, "config", "config.yaml", "path to config YAML")
	flag.StringVar(&metricsAddr, "metrics-addr", ":9090", "address for Prometheus /metrics endpoint")
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
	fetcher := news.NewFetcher(nil)
	matcher := news.NewMatcher(news.DefaultAliases())
	algopackFetcher := algopack.NewHTTPFetcher(cfg.AlgoPackBaseURL, cfg.AlgoPackToken, nil)
	ingestor := orchestrator.NewMOEXIngestor(moexClient, fetcher, matcher, news.DefaultSources(), algopackFetcher)

	telegramClient := telegram.New(cfg.Telegram.BotToken, cfg.Telegram.ChatID, nil)

	riskConfig := risk.DefaultConfig()
	riskConfig.MaxLots = cfg.Risk.MaxLots
	riskConfig.Positions = store
	riskConfig.Store = store
	riskConfig.Alerter = telegramClient
	gate, err := risk.NewHardenedGate(riskConfig)
	if err != nil {
		return fmt.Errorf("create risk gate: %w", err)
	}

	modelSource, err := newModelSignalSource(cfg)
	if err != nil {
		return err
	}
	signalSource := &alertingSignalSource{source: modelSource, alerter: telegramClient, logger: log.Default()}
	appMetrics := metrics.New()

	tradeExecutor, err := selectExecutor(cfg, store, time.Now)
	if err != nil {
		return err
	}

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

	orch, err := orchestrator.New(orchestrator.Options{
		Tickers:      cfg.Tickers,
		Ingestor:     ingestor,
		Builder:      features.NewBuilder(time.Now),
		Source:       signalSource,
		Gate:         gate,
		Executor:     tradeExecutor,
		Audit:        store,
		PollInterval: cfg.PollInterval.Std(),
		Metrics:      appMetrics,
		KillSwitch:   store,
	})
	if err != nil {
		return fmt.Errorf("create orchestrator: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("starting trader: tickers=%d broker=%s paper=%v poll_interval=%s", len(cfg.Tickers), cfg.Broker, cfg.IsPaperTrading, cfg.PollInterval.Std())
	log.Printf("paper trading mode: account-based risk limits are disabled; max-lot and fat-finger checks still apply")
	orch.Run(ctx)
	log.Printf("trader stopped")
	return nil
}

type signalFailureAlerter interface {
	Send(ctx context.Context, text string) error
}

type alertingSignalSource struct {
	source  orchestrator.SignalSource
	alerter signalFailureAlerter
	logger  *log.Logger
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

func newModelSignalSource(cfg *config.Config) (*model.SignalSource, error) {
	if cfg == nil {
		return nil, fmt.Errorf("load model: config is nil")
	}
	weights, err := model.LoadWeights(cfg.Model.Path)
	if err != nil {
		return nil, fmt.Errorf("load model: %w", err)
	}
	return &model.SignalSource{Weights: weights, MaxLots: cfg.Risk.MaxLots}, nil
}

func selectExecutor(cfg *config.Config, store *storage.Store, now func() time.Time) (executor.Executor, error) {
	if cfg == nil {
		return nil, fmt.Errorf("select executor: config is nil")
	}
	if !cfg.IsPaperTrading {
		return nil, fmt.Errorf("live trading mode is not wired for broker %q: set broker: paper and is_paper_trading: true", cfg.Broker)
	}
	switch cfg.Broker {
	case "", config.BrokerPaper:
		return executor.NewPaperExecutorWithCommission(store, now, cfg.Commission.Rate), nil
	case config.BrokerTinkoff, config.BrokerFinam:
		return nil, fmt.Errorf("live trading mode is not wired for broker %q: set broker: paper and is_paper_trading: true", cfg.Broker)
	default:
		return nil, fmt.Errorf("select executor: unknown broker %q", cfg.Broker)
	}
}
