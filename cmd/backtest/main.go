package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
	"github.com/olegsidorkin/moex-trader/internal/llm"
	"github.com/olegsidorkin/moex-trader/internal/model"
	"github.com/olegsidorkin/moex-trader/internal/orchestrator"
)

const defaultLLMTimeout = 90 * time.Second

const (
	signalSourceModel = "model"
	signalSourceLLM   = "llm"
	signalSourceRule  = "rule"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var configPath string
	var fromStr, tillStr string
	var tickersStr string
	var depositStr string
	var maxLots int
	var commissionStr string
	var llmTimeout time.Duration
	var llmAttempts int
	var llmBackoff time.Duration
	var maxConsecutiveTimeouts int
	var lookbackDays int
	var cachePath string
	var outPath string
	var killSwitch bool
	var signalSourceName string
	var modelPath string
	var reversalThresholdPct float64
	var minConfidence float64

	flag.StringVar(&configPath, "config", "config.yaml", "path to config YAML")
	flag.StringVar(&fromStr, "from", "", "backtest start date YYYY-MM-DD (default: one year ago)")
	flag.StringVar(&tillStr, "till", "", "backtest end date YYYY-MM-DD (default: today)")
	flag.StringVar(&tickersStr, "tickers", "", "comma-separated tickers (overrides config)")
	flag.StringVar(&depositStr, "deposit", "100000", "starting paper deposit in RUB")
	flag.IntVar(&maxLots, "max-lots", 1, "max position in lots")
	flag.StringVar(&commissionStr, "commission-rate", "0.003", "commission rate applied to notional per fill")
	flag.DurationVar(&llmTimeout, "llm-timeout", defaultLLMTimeout, "per-decision LLM timeout")
	flag.IntVar(&llmAttempts, "llm-attempts", 3, "retries per LLM call on timeout")
	flag.DurationVar(&llmBackoff, "llm-backoff", 5*time.Second, "initial exponential backoff between retries")
	flag.IntVar(&maxConsecutiveTimeouts, "max-consecutive-timeouts", 5, "circuit breaker: halt after this many consecutive LLM timeouts")
	flag.IntVar(&lookbackDays, "lookback-days", 30, "max LLM decision points per ticker (0 = unlimited)")
	flag.StringVar(&cachePath, "cache", "", "path to persistent LLM decision cache (e.g. .backtest-cache.json)")
	flag.StringVar(&outPath, "out", "", "path to write markdown report (default: -)")
	flag.BoolVar(&killSwitch, "kill-switch", true, "enable drawdown kill switch")
	flag.StringVar(&signalSourceName, "signal-source", signalSourceModel, "signal source: model, llm or rule")
	flag.StringVar(&modelPath, "model-path", "", "path to trained model JSON (default: model.path from config)")
	flag.Float64Var(&reversalThresholdPct, "reversal-threshold", 0.5, "rule-only: |reversal_1d| %% required to trade")
	flag.Float64Var(&minConfidence, "min-confidence", 0, "demote BUY/SELL to HOLD when confidence below threshold (0 = off)")
	flag.Parse()

	deposit, err := decimal.NewFromString(depositStr)
	if err != nil {
		return fmt.Errorf("parse -deposit: %w", err)
	}
	commissionRate, err := decimal.NewFromString(commissionStr)
	if err != nil {
		return fmt.Errorf("parse -commission-rate: %w", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if modelPath == "" {
		modelPath = cfg.Model.Path
	}

	var from, till time.Time
	if fromStr != "" {
		from, err = time.Parse("2006-01-02", fromStr)
		if err != nil {
			return fmt.Errorf("parse -from: %w", err)
		}
	}
	if tillStr != "" {
		till, err = time.Parse("2006-01-02", tillStr)
		if err != nil {
			return fmt.Errorf("parse -till: %w", err)
		}
	}
	tickers := cfg.Tickers
	if tickersStr != "" {
		tickers = splitComma(tickersStr)
	}

	moexClient := moex.NewClient(cfg.MOEXISSBaseURL, nil)
	source := backtest.NewISSSource(cfg.MOEXISSBaseURL, moexClient)

	signalSource, saveCache, err := buildSignalSource(cfg, signalSourceOptions{
		Mode:                   signalSourceName,
		ModelPath:              modelPath,
		MaxLots:                maxLots,
		LLMTimeout:             llmTimeout,
		LLMAttempts:            llmAttempts,
		LLMBackoff:             llmBackoff,
		MaxConsecutiveTimeouts: maxConsecutiveTimeouts,
		CachePath:              cachePath,
		ReversalThresholdPct:   reversalThresholdPct,
		MinConfidence:          minConfidence,
	})
	if err != nil {
		return err
	}
	if saveCache != nil {
		defer func() {
			if err := saveCache(); err != nil {
				log.Printf("save decision cache: %v", err)
			}
		}()
	}

	engine, err := backtest.NewEngine(backtest.Config{
		Tickers:               tickers,
		From:                  from,
		Till:                  till,
		Deposit:               deposit,
		MaxLots:               maxLots,
		CommissionRate:        commissionRate,
		WarmupDays:            100,
		MaxDecisionsPerTicker: lookbackDays,
		KillSwitch:            killSwitch,
		SignalSource:          signalSource,
		Source:                source,
	})
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("backtest: window=%s..%s tickers=%d deposit=%s lots=%d commission=%s signal_source=%s lookback=%d kill_switch=%v min_confidence=%s",
		formatFlag(from), formatFlag(till), len(tickers), deposit.String(), maxLots, commissionRate.String(), signalSourceName, lookbackDays, killSwitch, decimal.NewFromFloat(minConfidence).String())

	result, err := engine.Run(ctx)
	if err != nil {
		return fmt.Errorf("backtest run: %w", err)
	}

	report := result.Markdown()
	if outPath != "" {
		if err := os.WriteFile(outPath, []byte(report), 0o644); err != nil {
			return fmt.Errorf("write report %q: %w", outPath, err)
		}
		log.Printf("backtest: report written to %s", outPath)
	} else {
		fmt.Print(report)
	}
	return nil
}

type signalSourceOptions struct {
	Mode                   string
	ModelPath              string
	MaxLots                int
	LLMTimeout             time.Duration
	LLMAttempts            int
	LLMBackoff             time.Duration
	MaxConsecutiveTimeouts int
	CachePath              string
	ReversalThresholdPct   float64
	MinConfidence          float64
}

func buildSignalSource(cfg *config.Config, opts signalSourceOptions) (backtest.SignalSource, func() error, error) {
	var source backtest.SignalSource
	var saveCache func() error
	switch opts.Mode {
	case signalSourceModel:
		weights, err := model.LoadWeights(opts.ModelPath)
		if err != nil {
			return nil, nil, fmt.Errorf("load model: %w", err)
		}
		source = &model.SignalSource{Weights: weights, MaxLots: opts.MaxLots}
	case signalSourceLLM:
		if cfg == nil {
			return nil, nil, fmt.Errorf("load llm signal source: config is nil")
		}
		if strings.TrimSpace(cfg.Ollama.Host) == "" || strings.TrimSpace(cfg.Ollama.Model) == "" {
			return nil, nil, fmt.Errorf("load llm signal source: ollama.host and ollama.model must be set in config for -signal-source=%s", signalSourceLLM)
		}
		llmClient := llm.New(cfg.Ollama.Host, cfg.Ollama.Model, opts.LLMTimeout)
		decisionEngine := llm.NewDecisionEngine(llmClient, llm.NewPromptBuilder(), nil, time.Now)
		llmSource := orchestrator.NewLLMSignalSource(decisionEngine, opts.LLMTimeout, log.Default())

		source = llmSource
		if opts.LLMAttempts > 1 {
			source = backtest.NewRetryingSignalSource(
				llmSource, opts.LLMAttempts, opts.LLMBackoff, opts.LLMBackoff*8, opts.MaxConsecutiveTimeouts, log.Default())
		}
	case signalSourceRule:
		threshold := backtestDecimal(opts.ReversalThresholdPct)
		source = &backtest.ReversalRuleSource{Threshold: threshold, MaxLots: opts.MaxLots}
	default:
		return nil, nil, fmt.Errorf("unknown signal source %q: want %q, %q or %q", opts.Mode, signalSourceModel, signalSourceLLM, signalSourceRule)
	}

	if opts.CachePath != "" {
		cache, err := backtest.NewCachedSignalSource(source, opts.CachePath)
		if err != nil {
			return nil, nil, err
		}
		source = cache
		saveCache = cache.Save
	}

	if opts.MinConfidence > 0 {
		source = &backtest.ConfidenceGateSource{
			Inner:         source,
			MinConfidence: decimal.NewFromFloat(opts.MinConfidence),
		}
	}
	return source, saveCache, nil
}

func backtestDecimal(v float64) decimal.Decimal {
	return decimal.NewFromFloat(v)
}

func splitComma(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func formatFlag(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format("2006-01-02")
}
