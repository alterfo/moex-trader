package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/features"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
	"github.com/olegsidorkin/moex-trader/internal/model"
	"github.com/olegsidorkin/moex-trader/internal/trainrun"
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
	configPath         string
	tickersFlag        string
	fromStr            string
	tillStr            string
	horizonDays        int
	deadbandPct        float64
	labelCommissionPct float64
	learningRate       float64
	l2Lambda           float64
	epochs             int
	splitDateStr       string
	valDays            int
	maxLots            int
	intervalMin        int
	featureBPD         int
	outPath            string
	newsHistory        string
	labelMode          string
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
	var source backtest.HistoricalSource
	featureBPD := opts.featureBPD
	if opts.intervalMin > 0 && opts.intervalMin != 24 {
		source = backtest.NewISSSourceInterval(cfg.MOEXISSBaseURL, moexClient, opts.intervalMin)
	} else {
		source = backtest.NewISSSource(cfg.MOEXISSBaseURL, moexClient)
	}
	if featureBPD < 0 {
		featureBPD = features.ConfigForInterval(opts.intervalMin).BarsPerDay
	}
	log.Printf("trainmodel: interval %d min, feature barsPerDay=%d (0 = raw bar-count windows)", opts.intervalMin, featureBPD)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	_, _, err = trainrun.Run(ctx, trainrun.Config{
		Tickers:            tickers,
		From:               from,
		Till:               till,
		Split:              split,
		HorizonDays:        opts.horizonDays,
		DeadbandPct:        opts.deadbandPct,
		LabelCommissionPct: opts.labelCommissionPct,
		IntervalMin:        opts.intervalMin,
		FeatureBPD:         featureBPD,
		TrainCfg: model.TrainConfig{
			LearningRate: opts.learningRate,
			L2Lambda:     opts.l2Lambda,
			Epochs:       opts.epochs,
		},
		BuyThreshold:   defaultBuyPct,
		SellThreshold:  defaultSellPct,
		MaxLots:        maxLots,
		Deposit:        deposit,
		CommissionRate: cfg.Commission.Rate,
		OutPath:        opts.outPath,
		NewsHistory:    opts.newsHistory,
		LabelMode:      model.LabelMode(opts.labelMode),
		Now:            time.Now,
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
	fs.IntVar(&opts.horizonDays, "horizon-days", opts.horizonDays, "forward-return horizon, in trading days on daily bars or in bars when -interval-min is set")
	fs.Float64Var(&opts.deadbandPct, "deadband-pct", opts.deadbandPct, "exclude labels with absolute forward return below this percent")
	fs.Float64Var(&opts.labelCommissionPct, "label-commission-pct", 0, "one-way commission rate to bake into the label dead zone (e.g. 0.0005); 0 disables cost-adjustment and matches prior behavior")
	fs.IntVar(&opts.intervalMin, "interval-min", 0, "candle interval in minutes for intraday bars (24 or 0 = daily; ISS supports 1/10/60)")
	fs.IntVar(&opts.featureBPD, "feature-bars-per-day", 0, "scale day-named feature windows by this many bars/session (0 = keep raw bar-count windows; -1 = auto/calendar from -interval-min; positive = explicit)")
	fs.Float64Var(&opts.learningRate, "learning-rate", opts.learningRate, "gradient descent learning rate")
	fs.Float64Var(&opts.l2Lambda, "l2-lambda", opts.l2Lambda, "L2 regularization strength")
	fs.IntVar(&opts.epochs, "epochs", opts.epochs, "gradient descent epochs")
	fs.StringVar(&opts.splitDateStr, "split-date", "", "validation split date YYYY-MM-DD (default: till minus val-days)")
	fs.IntVar(&opts.valDays, "val-days", opts.valDays, "validation window length when split-date is not set")
	fs.IntVar(&opts.maxLots, "max-lots", 0, "max lots for validation (default: config risk.max_lots)")
	fs.StringVar(&opts.outPath, "out", opts.outPath, "path to write trained model JSON")
	fs.StringVar(&opts.newsHistory, "news-history", "", "path to a finanalys-format news_history.jsonl to override news_sentiment/news_count with real historical values where available")
	fs.StringVar(&opts.labelMode, "label-mode", "excess", "label target: excess (vs IMOEX) or absolute forward return")
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
