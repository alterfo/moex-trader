package main

import (
	"context"
	"encoding/csv"
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
)

const (
	defaultConfigPath  = "config.yaml"
	defaultHorizonDays = 5
	defaultDeadbandPct = 0.5
	minLabelCandles    = 64
)

type options struct {
	configPath  string
	tickersFlag string
	fromStr     string
	tillStr     string
	splitStr    string
	horizonDays int
	deadbandPct float64
	outPath     string
	newsHistory string
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}

	cfg, err := config.Load(opts.configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	now := time.Now()
	from, till, err := resolveWindow(opts, now)
	if err != nil {
		return err
	}
	split, err := parseDate(opts.splitStr, "split")
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

	var newsHistory map[string]map[string]model.NewsAggregate
	if opts.newsHistory != "" {
		records, err := model.LoadFinanalysNewsHistory(opts.newsHistory)
		if err != nil {
			return fmt.Errorf("load news history: %w", err)
		}
		newsHistory = model.AggregateDailySentiment(records)
		log.Printf("exportdataset: loaded %d historical news records for real news_sentiment/news_count", len(records))
	}

	moexClient := moex.NewClient(cfg.MOEXISSBaseURL, nil)
	source := backtest.NewISSSource(cfg.MOEXISSBaseURL, moexClient)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	samples, err := model.BuildSamples(ctx, source, tickers, from, till, opts.horizonDays, opts.deadbandPct)
	if err != nil {
		return fmt.Errorf("build labeled samples: %w", err)
	}
	labelByKey := make(map[string]labeledRow, len(samples))
	for _, sample := range samples {
		key := sample.Feature.Ticker + "|" + dateKey(sample.Feature.GeneratedAt)
		labelByKey[key] = labeledRow{label: sample.Label, date: sample.LabelDate}
	}

	out, err := os.Create(opts.outPath)
	if err != nil {
		return fmt.Errorf("create output %q: %w", opts.outPath, err)
	}
	defer out.Close()

	fetchFrom := from.AddDate(0, 0, -backtest.DefaultWarmupDays)
	writer := csv.NewWriter(out)
	defer writer.Flush()

	header := []string{"ticker", "date", "label", "label_date", "split", "return_pct", "realized_volatility",
		"news_sentiment", "news_count", "order_book_imbalance", "mom_5d", "mom_21d", "mom_63d",
		"reversal_1d", "rsi_14", "dist_ma20_pct", "dist_ma50_pct", "realized_vol_21d_annualized_pct", "volume_zscore_20d",
		"macd_hist_pct", "stoch_k_14", "williams_r_14", "alligator_spread_pct"}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	for _, ticker := range tickers {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		builder := features.NewBuilder(time.Now)
		if err := writeTicker(ctx, writer, source, builder, ticker, fetchFrom, till, from, split, opts.horizonDays, labelByKey, newsHistory, header); err != nil {
			return err
		}
	}
	return nil
}

type labeledRow struct {
	label float64
	date  time.Time
}

func writeTicker(ctx context.Context, writer *csv.Writer, source backtest.HistoricalSource, builder *features.Builder,
	ticker string, fetchFrom, till, from, split time.Time, horizonDays int, labels map[string]labeledRow,
	newsHistory map[string]map[string]model.NewsAggregate, header []string) error {
	candles, err := source.History(ctx, ticker, fetchFrom, till)
	if err != nil {
		return fmt.Errorf("history %s: %w", ticker, err)
	}
	if len(candles) < minLabelCandles+1 {
		return nil
	}
	for d := minLabelCandles; d < len(candles); d++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		decisionDay := candles[d].Begin
		if decisionDay.Before(from) {
			continue
		}
		if decisionPrice(candles, d).Sign() <= 0 {
			continue
		}
		input := features.Input{
			Ticker: ticker,
			Price: features.PriceSnapshot{
				LastPrice: candles[d-1].Close,
				PrevClose: candles[d-2].Close,
				AsOf:      decisionDay,
			},
			Candles: candles[:d],
		}
		feature, err := builder.Build(input)
		if err != nil {
			continue
		}
		if byDate, ok := newsHistory[ticker]; ok {
			if agg, ok := byDate[dateKey(decisionDay)]; ok {
				feature.NewsSentiment = decimal.NewFromFloat(agg.Sentiment)
				feature.NewsCount = agg.Count
			}
		}
		vec, _ := model.ToVector(feature)
		key := ticker + "|" + dateKey(decisionDay)
		row := make([]string, len(header))
		row[0] = ticker
		row[1] = dateKey(decisionDay)
		if label, ok := labels[key]; ok {
			row[2] = fmt.Sprintf("%d", int(label.label))
			row[3] = dateKey(label.date)
			if label.date.Before(split) {
				row[4] = "train"
			} else {
				row[4] = "val"
			}
		}
		for i, value := range vec {
			row[5+i] = formatFloat(value)
		}
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("write row %s %s: %w", ticker, dateKey(decisionDay), err)
		}
	}
	return nil
}

func decisionPrice(candles []moex.Candle, d int) decimal.Decimal {
	if d+1 < len(candles) {
		return candles[d+1].Open
	}
	return candles[d].Close
}

func formatFloat(value float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.8f", value), "0"), ".")
}

func dateKey(t time.Time) string {
	return t.Format("2006-01-02")
}

func parseOptions(args []string) (options, error) {
	opts := options{
		configPath:  defaultConfigPath,
		horizonDays: defaultHorizonDays,
		deadbandPct: defaultDeadbandPct,
	}
	fs := flag.NewFlagSet("exportdataset", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.configPath, "config", opts.configPath, "config path")
	fs.StringVar(&opts.tickersFlag, "tickers", "", "comma-separated tickers (default: config)")
	fs.StringVar(&opts.fromStr, "from", "", "dataset start YYYY-MM-DD")
	fs.StringVar(&opts.tillStr, "till", "", "dataset end YYYY-MM-DD")
	fs.StringVar(&opts.splitStr, "split", "", "train/val split YYYY-MM-DD")
	fs.IntVar(&opts.horizonDays, "horizon-days", opts.horizonDays, "forward-return horizon")
	fs.Float64Var(&opts.deadbandPct, "deadband-pct", opts.deadbandPct, "label deadband percent")
	fs.StringVar(&opts.outPath, "out", "dataset.csv", "output CSV path")
	fs.StringVar(&opts.newsHistory, "news-history", "", "path to a finanalys-format news_history.jsonl to override news_sentiment/news_count with real historical values where available")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if fs.NArg() > 0 {
		return options{}, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if opts.outPath == "" {
		return options{}, errors.New("out path must not be empty")
	}
	return opts, nil
}

func resolveWindow(opts options, now time.Time) (from, till time.Time, err error) {
	if opts.fromStr != "" {
		from, err = time.Parse("2006-01-02", opts.fromStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("parse -from: %w", err)
		}
	} else {
		from = now.AddDate(0, 0, -365)
	}
	if opts.tillStr != "" {
		till, err = time.Parse("2006-01-02", opts.tillStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("parse -till: %w", err)
		}
	} else {
		till = now
	}
	if !from.Before(till) {
		return time.Time{}, time.Time{}, errors.New("from must be before till")
	}
	return from, till, nil
}

func parseDate(value, name string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse -%s: %w", name, err)
	}
	return parsed, nil
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
