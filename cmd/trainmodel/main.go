package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
	"github.com/olegsidorkin/moex-trader/internal/model"
)

const (
	defaultConfigPath  = "config.yaml"
	defaultHorizonDays = 5
	defaultDeadbandPct = 0.5
	defaultValDays     = 90
	defaultOutPath     = "model.json"
	defaultBuyPct      = 0.55
	defaultSellPct     = 0.45
	validationDeposit  = "100000"
)

type options struct {
	configPath   string
	tickersFlag  string
	fromStr      string
	tillStr      string
	horizonDays  int
	deadbandPct  float64
	learningRate float64
	l2Lambda     float64
	epochs       int
	splitDateStr string
	valDays      int
	maxLots      int
	outPath      string
	newsHistory  string
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func run(args []string, stdout io.Writer) error {
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}

	cfg, err := config.Load(opts.configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	now := time.Now()
	from, till, split, err := resolveWindow(opts, now)
	if err != nil {
		return err
	}

	tickers := splitComma(opts.tickersFlag)
	if len(tickers) == 0 {
		tickers = cfg.Tickers
	}
	if len(tickers) == 0 {
		return errors.New("tickers must not be empty")
	}

	maxLots := opts.maxLots
	if maxLots <= 0 {
		maxLots = cfg.Risk.MaxLots
	}
	if maxLots <= 0 {
		return errors.New("max lots must be positive")
	}

	deposit, err := decimal.NewFromString(validationDeposit)
	if err != nil {
		return fmt.Errorf("parse validation deposit: %w", err)
	}

	moexClient := moex.NewClient(cfg.MOEXISSBaseURL, nil)
	source := backtest.NewISSSource(cfg.MOEXISSBaseURL, moexClient)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	_, _, err = runPipeline(ctx, pipelineConfig{
		tickers:     tickers,
		from:        from,
		till:        till,
		split:       split,
		horizonDays: opts.horizonDays,
		deadbandPct: opts.deadbandPct,
		trainCfg: model.TrainConfig{
			LearningRate: opts.learningRate,
			L2Lambda:     opts.l2Lambda,
			Epochs:       opts.epochs,
		},
		maxLots:        maxLots,
		deposit:        deposit,
		commissionRate: cfg.Commission.Rate,
		outPath:        opts.outPath,
		newsHistory:    opts.newsHistory,
		now:            time.Now,
	}, source, stdout)
	return err
}

func parseOptions(args []string) (options, error) {
	defaults := model.DefaultTrainConfig()
	opts := options{
		configPath:   defaultConfigPath,
		horizonDays:  defaultHorizonDays,
		deadbandPct:  defaultDeadbandPct,
		learningRate: defaults.LearningRate,
		l2Lambda:     defaults.L2Lambda,
		epochs:       defaults.Epochs,
		valDays:      defaultValDays,
		outPath:      defaultOutPath,
	}

	fs := flag.NewFlagSet("trainmodel", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.configPath, "config", opts.configPath, "path to config YAML")
	fs.StringVar(&opts.tickersFlag, "tickers", "", "comma-separated tickers (default: config tickers)")
	fs.StringVar(&opts.fromStr, "from", "", "training start date YYYY-MM-DD (default: two years ago)")
	fs.StringVar(&opts.tillStr, "till", "", "training end date YYYY-MM-DD (default: today)")
	fs.IntVar(&opts.horizonDays, "horizon-days", opts.horizonDays, "forward-return horizon in trading days")
	fs.Float64Var(&opts.deadbandPct, "deadband-pct", opts.deadbandPct, "exclude labels with absolute forward return below this percent")
	fs.Float64Var(&opts.learningRate, "learning-rate", opts.learningRate, "gradient descent learning rate")
	fs.Float64Var(&opts.l2Lambda, "l2-lambda", opts.l2Lambda, "L2 regularization strength")
	fs.IntVar(&opts.epochs, "epochs", opts.epochs, "gradient descent epochs")
	fs.StringVar(&opts.splitDateStr, "split-date", "", "validation split date YYYY-MM-DD (default: till minus val-days)")
	fs.IntVar(&opts.valDays, "val-days", opts.valDays, "validation window length when split-date is not set")
	fs.IntVar(&opts.maxLots, "max-lots", 0, "max lots for validation (default: config risk.max_lots)")
	fs.StringVar(&opts.outPath, "out", opts.outPath, "path to write trained model JSON")
	fs.StringVar(&opts.newsHistory, "news-history", "", "path to a finanalys-format news_history.jsonl to override news_sentiment/news_count with real historical values where available")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if fs.NArg() > 0 {
		return options{}, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	return opts, nil
}

func resolveWindow(opts options, now time.Time) (from, till, split time.Time, err error) {
	if opts.fromStr != "" {
		from, err = time.Parse("2006-01-02", opts.fromStr)
		if err != nil {
			return time.Time{}, time.Time{}, time.Time{}, fmt.Errorf("parse -from: %w", err)
		}
	} else {
		from = now.AddDate(-2, 0, 0)
	}
	if opts.tillStr != "" {
		till, err = time.Parse("2006-01-02", opts.tillStr)
		if err != nil {
			return time.Time{}, time.Time{}, time.Time{}, fmt.Errorf("parse -till: %w", err)
		}
	} else {
		till = now
	}
	if !from.Before(till) {
		return time.Time{}, time.Time{}, time.Time{}, errors.New("from must be before till")
	}
	split, err = computeSplitDate(till, opts.splitDateStr, opts.valDays)
	if err != nil {
		return time.Time{}, time.Time{}, time.Time{}, err
	}
	if !split.After(from) || !split.Before(till) {
		return time.Time{}, time.Time{}, time.Time{}, errors.New("split date must be after from and before till")
	}
	return from, till, split, nil
}

func computeSplitDate(till time.Time, splitDateStr string, valDays int) (time.Time, error) {
	if splitDateStr != "" {
		split, err := time.Parse("2006-01-02", splitDateStr)
		if err != nil {
			return time.Time{}, fmt.Errorf("parse -split-date: %w", err)
		}
		return split, nil
	}
	if valDays <= 0 {
		return time.Time{}, errors.New("val-days must be positive when split-date is not set")
	}
	return till.AddDate(0, 0, -valDays), nil
}

type pipelineConfig struct {
	tickers        []string
	from           time.Time
	till           time.Time
	split          time.Time
	horizonDays    int
	deadbandPct    float64
	trainCfg       model.TrainConfig
	maxLots        int
	deposit        decimal.Decimal
	commissionRate decimal.Decimal
	outPath        string
	newsHistory    string
	now            func() time.Time
}

func runPipeline(ctx context.Context, cfg pipelineConfig, source backtest.HistoricalSource, stdout io.Writer) (*model.Weights, *backtest.Result, error) {
	if err := validatePipelineConfig(cfg); err != nil {
		return nil, nil, err
	}
	now := cfg.now
	if now == nil {
		now = time.Now
	}

	samples, err := model.BuildSamples(ctx, source, cfg.tickers, cfg.from, cfg.till, cfg.horizonDays, cfg.deadbandPct)
	if err != nil {
		return nil, nil, fmt.Errorf("build samples: %w", err)
	}
	if len(samples) == 0 {
		return nil, nil, errors.New("no labeled samples in the selected window")
	}

	if cfg.newsHistory != "" {
		records, err := model.LoadFinanalysNewsHistory(cfg.newsHistory)
		if err != nil {
			return nil, nil, fmt.Errorf("load news history: %w", err)
		}
		news := model.AggregateDailySentiment(records)
		applied := model.ApplyNewsOverride(samples, news)
		log.Printf("trainmodel: applied real news_sentiment/news_count to %d/%d samples from %d records", applied, len(samples), len(records))
	}

	trainSamples, valSamples := splitTrainVal(samples, cfg.split)
	if len(trainSamples) == 0 {
		return nil, nil, errors.New("no training samples before the validation split")
	}

	rows := make([]model.Sample, 0, len(trainSamples))
	names := make([]string, 0)
	for _, sample := range trainSamples {
		vector, order := model.ToVector(sample.Feature)
		names = order
		rows = append(rows, model.Sample{X: vector, Y: sample.Label})
	}
	mean, std := model.Standardize(rows)
	coef, bias, err := model.Train(rows, cfg.trainCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("train: %w", err)
	}

	weights := &model.Weights{
		FeatureOrder:  names,
		Mean:          mean,
		Std:           std,
		Coef:          coef,
		Bias:          bias,
		BuyThreshold:  defaultBuyPct,
		SellThreshold: defaultSellPct,
		HorizonDays:   cfg.horizonDays,
		DeadbandPct:   cfg.deadbandPct,
		TrainedAt:     now().UTC(),
		Training: model.TrainingMetadata{
			TrainFrom:     cfg.from,
			TrainTill:     cfg.split,
			ValFrom:       cfg.split,
			ValTill:       cfg.till,
			Tickers:       append([]string(nil), cfg.tickers...),
			TrainSamples:  len(trainSamples),
			ValSamples:    len(valSamples),
			TrainAccuracy: classificationAccuracy(rows, coef, bias, mean, std),
			ValAccuracy:   labeledAccuracy(valSamples, coef, bias, mean, std),
		},
	}
	signalSource := &model.SignalSource{Weights: weights, MaxLots: cfg.maxLots}
	engine, err := backtest.NewEngine(backtest.Config{
		Tickers:        cfg.tickers,
		From:           cfg.split,
		Till:           cfg.till,
		Deposit:        cfg.deposit,
		MaxLots:        cfg.maxLots,
		CommissionRate: cfg.commissionRate,
		WarmupDays:     backtest.DefaultWarmupDays,
		KillSwitch:     true,
		SignalSource:   signalSource,
		Source:         source,
		Logger:         log.Default(),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("build validation engine: %w", err)
	}

	result, err := engine.Run(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("validation backtest: %w", err)
	}

	weights.Training.ValSharpe = result.Sharpe
	weights.Training.ValHitRate = result.HitRate
	weights.Training.ValMaxDrawdownPct = result.MaxDrawdownPct
	if err := weights.Save(cfg.outPath); err != nil {
		return nil, nil, fmt.Errorf("save trained weights: %w", err)
	}

	if stdout != nil {
		fmt.Fprint(stdout, result.Markdown())
	}
	return weights, result, nil
}

func splitTrainVal(samples []model.LabeledSample, split time.Time) (train, val []model.LabeledSample) {
	for _, sample := range samples {
		if sample.LabelDate.Before(split) {
			train = append(train, sample)
		} else {
			val = append(val, sample)
		}
	}
	return train, val
}

func validatePipelineConfig(cfg pipelineConfig) error {
	if len(cfg.tickers) == 0 {
		return errors.New("tickers must not be empty")
	}
	if cfg.horizonDays <= 0 {
		return errors.New("horizon-days must be positive")
	}
	if cfg.deadbandPct < 0 {
		return errors.New("deadband-pct must be non-negative")
	}
	if cfg.maxLots <= 0 {
		return errors.New("max lots must be positive")
	}
	if cfg.deposit.Sign() <= 0 {
		return errors.New("deposit must be positive")
	}
	if cfg.commissionRate.IsNegative() {
		return errors.New("commission rate must be non-negative")
	}
	if strings.TrimSpace(cfg.outPath) == "" {
		return errors.New("out path must not be empty")
	}
	if !cfg.from.Before(cfg.split) || !cfg.split.Before(cfg.till) {
		return errors.New("split date must be after from and before till")
	}
	return nil
}

func classificationAccuracy(samples []model.Sample, coef []float64, bias float64, mean, std []float64) float64 {
	if len(samples) == 0 {
		return 0
	}
	correct := 0
	for _, sample := range samples {
		normalized := make([]float64, len(sample.X))
		for i, value := range sample.X {
			divisor := std[i]
			if divisor == 0 {
				divisor = 1
			}
			normalized[i] = (value - mean[i]) / divisor
		}
		logit := dotVector(coef, normalized) + bias
		prediction := 0.0
		if sigmoidValue(logit) >= 0.5 {
			prediction = 1
		}
		if prediction == sample.Y {
			correct++
		}
	}
	return float64(correct) / float64(len(samples))
}

func labeledAccuracy(samples []model.LabeledSample, coef []float64, bias float64, mean, std []float64) float64 {
	rows := make([]model.Sample, 0, len(samples))
	for _, sample := range samples {
		vector, _ := model.ToVector(sample.Feature)
		rows = append(rows, model.Sample{X: vector, Y: sample.Label})
	}
	return classificationAccuracy(rows, coef, bias, mean, std)
}

func dotVector(a, b []float64) float64 {
	total := 0.0
	for i := range a {
		total += a[i] * b[i]
	}
	return total
}

func sigmoidValue(z float64) float64 {
	if z >= 0 {
		return 1 / (1 + math.Exp(-z))
	}
	exponential := math.Exp(z)
	return exponential / (1 + exponential)
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
