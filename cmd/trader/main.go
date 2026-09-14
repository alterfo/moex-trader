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

	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/executor"
	"github.com/olegsidorkin/moex-trader/internal/features"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/news"
	"github.com/olegsidorkin/moex-trader/internal/llm"
	"github.com/olegsidorkin/moex-trader/internal/metrics"
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
	flag.StringVar(&configPath, "config", "config.yaml", "path to config YAML")
	flag.StringVar(&metricsAddr, "metrics-addr", ":9090", "address for Prometheus /metrics endpoint")
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

	moexClient := moex.NewClient(cfg.MOEXISSBaseURL, nil)
	fetcher := news.NewFetcher(nil)
	matcher := news.NewMatcher(news.DefaultAliases())
	ingestor := orchestrator.NewMOEXIngestor(moexClient, fetcher, matcher, news.DefaultSources())

	gate, err := risk.NewLotLimitGate(cfg.Risk.MaxLots)
	if err != nil {
		return fmt.Errorf("create risk gate: %w", err)
	}

	llmClient := llm.New(cfg.Ollama.Host, cfg.Ollama.Model, cfg.Ollama.Timeout.Std())
	decisionEngine := llm.NewDecisionEngine(llmClient, llm.NewPromptBuilder(), store, time.Now)
	signalSource := orchestrator.NewLLMSignalSource(decisionEngine, cfg.Ollama.Timeout.Std(), log.Default())
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

	orch, err := orchestrator.New(orchestrator.Options{
		Tickers:      cfg.Tickers,
		Ingestor:     ingestor,
		Builder:      features.NewBuilder(time.Now),
		Source:       signalSource,
		Gate:         gate,
		Executor:     executor.NewPaperExecutor(store, time.Now),
		Audit:        store,
		PollInterval: cfg.PollInterval.Std(),
		Metrics:      appMetrics,
	})
	if err != nil {
		return fmt.Errorf("create orchestrator: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("starting trader: tickers=%d paper=%v poll_interval=%s", len(cfg.Tickers), cfg.IsPaperTrading, cfg.PollInterval.Std())
	orch.Run(ctx)
	log.Printf("trader stopped")
	return nil
}
