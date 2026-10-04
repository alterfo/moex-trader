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
	"github.com/olegsidorkin/moex-trader/internal/dividends"
	"github.com/olegsidorkin/moex-trader/internal/domain"
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
	configPath       string
	tickersFlag      string
	fromStr          string
	tillStr          string
	splitStr         string
	horizonDays      int
	horizonMode      string
	deadbandPct      float64
	labelMode        string
	commissionPct    float64
	intervalMin      int
	featureBPD       int
	outPath          string
	newsHistory      string
	dividends        string
	trainTickers     string
	trainMinTurnover string
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

	tradingTickers := normalizeTickerList(splitComma(opts.tickersFlag))
	if len(tradingTickers) == 0 {
		tradingTickers = normalizeTickerList(cfg.Tickers)
	}
	if len(tradingTickers) == 0 {
		return errors.New("tickers must not be empty")
	}

	trainTickers := normalizeTickerList(splitComma(opts.trainTickers))
	if len(trainTickers) == 0 {
		trainTickers = tradingTickers
	} else if err := validateTrainTickers(trainTickers); err != nil {
		return err
	}
	allTickers := unionUnique(tradingTickers, trainTickers)

	var newsHistory map[string]map[string]model.NewsAggregate
	var topicHistory map[string]map[string]features.TopicSignalAggregate
	var eventHistory map[string]map[string]model.EventAggregate
	if opts.newsHistory != "" {
		records, err := model.LoadFinanalysNewsHistory(opts.newsHistory)
		if err != nil {
			return fmt.Errorf("load news history: %w", err)
		}
		newsHistory = model.AggregateDailySentiment(records)
		topicHistory = model.AggregateDailyTopicSignals(records)
		eventHistory = model.AggregateDailyEvents(records)
		log.Printf("exportdataset: loaded %d historical news records for real news_sentiment/news_count and events", len(records))
	}

	moexClient := moex.NewClient(cfg.MOEXISSBaseURL, nil)
	var source backtest.HistoricalSource
	featureBPD := opts.featureBPD
	if featureBPD < 0 {
		featureBPD = features.ConfigForInterval(opts.intervalMin).BarsPerDay
	}
	featureCfg := features.PriceFeatureConfig{BarsPerDay: featureBPD}
	if opts.intervalMin > 0 && opts.intervalMin != 24 {
		source = backtest.NewISSSourceInterval(cfg.MOEXISSBaseURL, moexClient, opts.intervalMin)
		log.Printf("exportdataset: using intraday interval %d min; feature barsPerDay=%d (0 = raw bar-count windows)", opts.intervalMin, featureBPD)
	} else {
		source = backtest.NewISSSource(cfg.MOEXISSBaseURL, moexClient)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var divRecords []dividends.Record
	if model.LabelMode(opts.labelMode) == model.LabelModeAbsoluteTR {
		divRecords, err = loadDividendCalendar(opts.dividends)
		if err != nil {
			return err
		}
	}
	samples, err := model.BuildSamplesWithDividends(ctx, source, allTickers, from, till, opts.horizonDays, opts.deadbandPct, model.LabelMode(opts.labelMode), opts.commissionPct, featureCfg, model.HorizonMode(opts.horizonMode), divRecords)
	if err != nil {
		return fmt.Errorf("build labeled samples: %w", err)
	}
	labelByKey := make(map[string]labeledRow, len(samples))
	for _, sample := range samples {
		key := sample.Feature.Ticker + "|" + barKey(sample.Feature.GeneratedAt)
		labelByKey[key] = labeledRow{label: sample.Label, date: sample.LabelDate}
	}

	out, err := os.Create(opts.outPath)
	if err != nil {
		return fmt.Errorf("create output %q: %w", opts.outPath, err)
	}
	defer out.Close()

	fetchFrom := from.AddDate(0, 0, -backtest.DefaultWarmupDays)
	if extra := featureCfg.FetchCalendarDays(); extra > backtest.DefaultWarmupDays {
		fetchFrom = from.AddDate(0, 0, -extra)
	}
	writer := csv.NewWriter(out)
	defer writer.Flush()

	header := datasetHeader()
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	for _, ticker := range tradingTickers {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		builder := features.NewBuilderWithConfig(time.Now, featureCfg)
		if err := writeTicker(ctx, writer, source, builder, ticker, fetchFrom, till, from, split, opts.horizonDays, featureCfg.WarmupCandles(), labelByKey, newsHistory, topicHistory, eventHistory, header, false, nil); err != nil {
			return err
		}
	}
	if len(trainTickers) > 0 && !sameTickerList(tradingTickers, trainTickers) {
		minTurnover, err := decimal.NewFromString(opts.trainMinTurnover)
		if err != nil {
			return fmt.Errorf("parse -train-min-turnover: %w", err)
		}
		eligible := func(candles []moex.Candle, d int) bool {
			return model.LiquidOn(candles, d, model.TurnoverWindow, minTurnover)
		}
		for _, ticker := range trainTickers {
			if containsTicker(tradingTickers, ticker) {
				continue
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			builder := features.NewBuilderWithConfig(time.Now, featureCfg)
			if err := writeTicker(ctx, writer, source, builder, ticker, fetchFrom, till, from, split, opts.horizonDays, featureCfg.WarmupCandles(), labelByKey, newsHistory, topicHistory, eventHistory, header, true, eligible); err != nil {
				return err
			}
		}
	}
	return nil
}

type labeledRow struct {
	label float64
	date  time.Time
}

func writeTicker(ctx context.Context, writer *csv.Writer, source backtest.HistoricalSource, builder *features.Builder,
	ticker string, fetchFrom, till, from, split time.Time, horizonDays, warmup int, labels map[string]labeledRow,
	newsHistory map[string]map[string]model.NewsAggregate, topicHistory map[string]map[string]features.TopicSignalAggregate, eventHistory map[string]map[string]model.EventAggregate, header []string, trainOnly bool, eligible func([]moex.Candle, int) bool) error {
	candles, err := source.History(ctx, ticker, fetchFrom, till)
	if err != nil {
		return fmt.Errorf("history %s: %w", ticker, err)
	}
	if len(candles) < warmup+1 {
		return nil
	}
	for d := warmup; d < len(candles); d++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if eligible != nil && !eligible(candles, d) {
			continue
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
		if byDate, ok := topicHistory[ticker]; ok {
			if agg, ok := byDate[dateKey(decisionDay)]; ok {
				feature.NegotiationsSignal = decimal.NewFromFloat(agg.Negotiations)
				feature.SanctionsSignal = decimal.NewFromFloat(agg.Sanctions)
			}
		}
		if byDate, ok := eventHistory[ticker]; ok {
			if agg, ok := byDate[dateKey(decisionDay)]; ok {
				feature.EventDividend = agg.Dividend
				feature.EventBuyback = agg.Buyback
				feature.EventSanctions = agg.Sanctions
				feature.EventIPO = agg.IPO
				feature.EventReport = agg.Report
				feature.EventDelisting = agg.Delisting
				feature.EventMNA = agg.MNA
				feature.EventDefault = agg.Default
			}
		}
		vec, _ := model.ToVector(feature)
		key := ticker + "|" + barKey(decisionDay)
		row := make([]string, len(header))
		row[0] = ticker
		row[1] = dateKey(decisionDay)
		if label, ok := labels[key]; ok {
			if trainOnly && !label.date.Before(split) {
				continue
			}
			row[2] = fmt.Sprintf("%d", int(label.label))
			row[3] = dateKey(label.date)
			if label.date.Before(split) {
				row[4] = "train"
			} else {
				row[4] = "val"
			}
		} else if trainOnly {
			continue
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

// barKey identifies one candle within a ticker for label matching. It keeps
// the time of day, unlike dateKey: intraday datasets have many decision bars
// per calendar date, and a date-only key would collapse them so every bar of
// a day inherited the same (last) label. News/event overrides keep using
// dateKey - those are daily by nature.
func barKey(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

func parseOptions(args []string) (options, error) {
	opts := options{
		configPath:       defaultConfigPath,
		horizonDays:      defaultHorizonDays,
		deadbandPct:      defaultDeadbandPct,
		trainMinTurnover: "10000000",
	}
	fs := flag.NewFlagSet("exportdataset", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.configPath, "config", opts.configPath, "config path")
	fs.StringVar(&opts.tickersFlag, "tickers", "", "comma-separated tickers (default: config)")
	fs.StringVar(&opts.fromStr, "from", "", "dataset start YYYY-MM-DD")
	fs.StringVar(&opts.tillStr, "till", "", "dataset end YYYY-MM-DD")
	fs.StringVar(&opts.splitStr, "split", "", "train/val split YYYY-MM-DD")
	fs.IntVar(&opts.horizonDays, "horizon-days", opts.horizonDays, "forward-return horizon, in trading days on daily bars or in bars when -interval-min is set")
	fs.StringVar(&opts.horizonMode, "horizon-mode", "", "forward-window unit: \"\" / bars (fixed bar count) or calendar_days (nearest candle to entry+horizon-days)")
	fs.Float64Var(&opts.deadbandPct, "deadband-pct", opts.deadbandPct, "label deadband percent")
	fs.StringVar(&opts.labelMode, "label-mode", "excess", "label target: excess (vs IMOEX) or absolute forward return")
	fs.Float64Var(&opts.commissionPct, "commission-pct", 0, "one-way commission rate (e.g. 0.0005); widens the dead zone by round-trip cost plus the entry bar's spread proxy (0 = disabled, matches prior behavior)")
	fs.IntVar(&opts.intervalMin, "interval-min", 0, "candle interval in minutes for intraday bars (24 or 0 = daily; ISS supports 1/10/60)")
	fs.IntVar(&opts.featureBPD, "feature-bars-per-day", 0, "scale day-named feature windows by this many bars/session (0 = keep raw bar-count windows; -1 = auto/calendar from -interval-min; positive = explicit)")
	fs.StringVar(&opts.outPath, "out", "dataset.csv", "output CSV path")
	fs.StringVar(&opts.newsHistory, "news-history", "", "path to a finanalys-format news_history.jsonl to override news_sentiment/news_count with real historical values where available")
	fs.StringVar(&opts.dividends, "dividends", "data/dividends.jsonl", "path to the dividend calendar JSONL used by -label-mode absolute_tr")
	fs.StringVar(&opts.trainTickers, "train-tickers", "", "comma-separated extra training tickers (default: same as trading tickers); FX tickers rejected")
	fs.StringVar(&opts.trainMinTurnover, "train-min-turnover", opts.trainMinTurnover, "minimum median 20-day turnover (RUB) for a -train-tickers name to contribute a training row")
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

func loadDividendCalendar(path string) ([]dividends.Record, error) {
	records, err := dividends.Load(path)
	if err != nil {
		return nil, fmt.Errorf("load dividend calendar: %w", err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("dividend calendar %s is empty; -label-mode absolute_tr requires dividend records", path)
	}
	return records, nil
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

func normalizeTickerList(tickers []string) []string {
	out := make([]string, 0, len(tickers))
	for _, ticker := range tickers {
		ticker = strings.ToUpper(strings.TrimSpace(ticker))
		if ticker != "" {
			out = append(out, ticker)
		}
	}
	return out
}

func unionUnique(a, b []string) []string {
	out := make([]string, 0, len(a)+len(b))
	for _, ticker := range a {
		if !containsTicker(out, ticker) {
			out = append(out, ticker)
		}
	}
	for _, ticker := range b {
		if !containsTicker(out, ticker) {
			out = append(out, ticker)
		}
	}
	return out
}

func containsTicker(tickers []string, want string) bool {
	for _, ticker := range tickers {
		if ticker == want {
			return true
		}
	}
	return false
}

func sameTickerList(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for _, ticker := range a {
		if !containsTicker(b, ticker) {
			return false
		}
	}
	return true
}

func isFXTicker(ticker string) bool {
	return strings.HasSuffix(ticker, "_TOM") || strings.HasSuffix(ticker, "_TOD")
}

func validateTrainTickers(tickers []string) error {
	for _, ticker := range tickers {
		if isFXTicker(ticker) {
			return fmt.Errorf("train ticker %q is an FX instrument and must not be trained on", ticker)
		}
	}
	return nil
}

func datasetHeader() []string {
	_, names := model.ToVector(domain.FeatureContext{})
	header := []string{"ticker", "date", "label", "label_date", "split"}
	return append(header, names...)
}
