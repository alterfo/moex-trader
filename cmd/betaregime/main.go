package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/betaregime"
	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/equalweight"
	"github.com/olegsidorkin/moex-trader/internal/features"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
	"github.com/olegsidorkin/moex-trader/internal/model"
	"github.com/olegsidorkin/moex-trader/internal/momentum"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var configPath, tickersStr, fromStr, tillStr, ensemblePath, outPath string
	var depositStr, commissionStr, spreadStr, slippageStr, targetNotionalStr string
	var windowsStr string
	var k, rebalanceEvery, maxLots int
	var flatBand float64

	flag.StringVar(&configPath, "config", "config.sandbox.yaml", "path to config YAML")
	flag.StringVar(&tickersStr, "tickers", "", "comma-separated tickers (overrides config)")
	flag.StringVar(&fromStr, "from", "2025-04-01", "window start YYYY-MM-DD")
	flag.StringVar(&tillStr, "till", "2026-09-17", "window end YYYY-MM-DD")
	flag.StringVar(&ensemblePath, "ensemble-path", "", "ensemble model JSON (default: model.ensemble_path from config)")
	flag.StringVar(&windowsStr, "windows", "", "comma-separated from:till OOS quarters (default: the six Task 4 quarters)")
	flag.StringVar(&outPath, "out", "", "path to write markdown report (default: stdout)")
	flag.StringVar(&depositStr, "deposit", "1000000", "starting deposit in RUB")
	flag.StringVar(&commissionStr, "commission-rate", "0.0005", "commission rate per fill")
	flag.StringVar(&spreadStr, "spread-pct", "0.0005", "half-spread cost per fill as fraction of price")
	flag.StringVar(&slippageStr, "slippage-pct", "0.0005", "slippage cost per fill as fraction of price")
	flag.StringVar(&targetNotionalStr, "target-notional", "15000", "target ruble notional per position")
	flag.IntVar(&k, "k", 5, "top-k (and bottom-k for long+short) momentum selection")
	flag.IntVar(&rebalanceEvery, "rebalance-every", 10, "rebalance every N trading days")
	flag.IntVar(&maxLots, "max-lots", 1000, "risk-gate max lots ceiling")
	flag.Float64Var(&flatBand, "flat-band", 0.03, "quarterly IMOEX |return| at or below which a quarter is flat")
	flag.Parse()

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	tickers := cfg.Tickers
	if tickersStr != "" {
		tickers = splitComma(tickersStr)
	}
	if len(tickers) == 0 {
		return fmt.Errorf("no tickers configured")
	}
	if ensemblePath == "" {
		ensemblePath = cfg.Model.EnsemblePath
	}
	if ensemblePath == "" {
		ensemblePath = "ensemble_model.json"
	}

	deposit, err := decimal.NewFromString(depositStr)
	if err != nil {
		return fmt.Errorf("parse -deposit: %w", err)
	}
	commissionRate, err := decimal.NewFromString(commissionStr)
	if err != nil {
		return fmt.Errorf("parse -commission-rate: %w", err)
	}
	spreadPct, err := decimal.NewFromString(spreadStr)
	if err != nil {
		return fmt.Errorf("parse -spread-pct: %w", err)
	}
	slippagePct, err := decimal.NewFromString(slippageStr)
	if err != nil {
		return fmt.Errorf("parse -slippage-pct: %w", err)
	}
	targetNotional, err := decimal.NewFromString(targetNotionalStr)
	if err != nil {
		return fmt.Errorf("parse -target-notional: %w", err)
	}

	from, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		return fmt.Errorf("parse -from: %w", err)
	}
	till, err := time.Parse("2006-01-02", tillStr)
	if err != nil {
		return fmt.Errorf("parse -till: %w", err)
	}
	windows, err := resolveWindows(windowsStr, from, till)
	if err != nil {
		return err
	}

	ens, err := model.LoadEnsembleModel(ensemblePath)
	if err != nil {
		return fmt.Errorf("load ensemble: %w", err)
	}

	moexClient := moex.NewClient(cfg.MOEXISSBaseURL, nil)
	fetchSource := backtest.NewISSSource(cfg.MOEXISSBaseURL, moexClient)
	const warmupDays = 100
	const featureWarmup = 64

	ctx := context.Background()
	mem := &memorySource{candles: make(map[string][]moex.Candle)}
	values := make(map[string]map[time.Time]float64)
	dateSet := make(map[time.Time]struct{})
	fetchFrom := from.AddDate(0, 0, -warmupDays)

	for _, raw := range tickers {
		ticker := strings.ToUpper(strings.TrimSpace(raw))
		if ticker == "" {
			continue
		}
		candles, err := fetchSource.History(ctx, ticker, fetchFrom, till)
		if err != nil {
			return fmt.Errorf("%s: history: %w", ticker, err)
		}
		candles = dropIncompleteTrailing(candles)
		if len(candles) < featureWarmup+1 {
			return fmt.Errorf("%s: only %d candles, need %d", ticker, len(candles), featureWarmup+1)
		}
		mem.candles[ticker] = candles

		builder := features.NewBuilderWithConfig(time.Now, features.PriceFeatureConfig{})
		byDate := make(map[time.Time]float64)
		for d := featureWarmup; d < len(candles); d++ {
			decisionDay := candles[d].Begin
			if decisionDay.Before(from) {
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
			value, _ := feature.Mom21d.Float64()
			byDate[decisionDay] = value
			dateSet[decisionDay] = struct{}{}
		}
		values[ticker] = byDate
	}

	imoexCandles, err := fetchSource.History(ctx, "IMOEX", fetchFrom, till)
	if err != nil {
		return fmt.Errorf("imoex history: %w", err)
	}
	imoexCandles = dropIncompleteTrailing(imoexCandles)

	dates := make([]time.Time, 0, len(dateSet))
	for date := range dateSet {
		dates = append(dates, date)
	}
	sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })

	engineConfig := func(signalSource backtest.SignalSource) backtest.Config {
		return backtest.Config{
			Tickers:        tickers,
			From:           from,
			Till:           till,
			Deposit:        deposit,
			MaxLots:        maxLots,
			CommissionRate: commissionRate,
			SpreadPct:      spreadPct,
			SlippagePct:    slippagePct,
			WarmupDays:     warmupDays,
			KillSwitch:     true,
			SignalSource:   signalSource,
			Source:         mem,
			FeatureConfig:  features.PriceFeatureConfig{},
		}
	}

	ensResult, err := runEngine(ctx, engineConfig(&model.EnsembleSignalSource{
		Model:          ens,
		MaxLots:        maxLots,
		TargetNotional: targetNotional,
	}))
	if err != nil {
		return fmt.Errorf("ensemble backtest: %w", err)
	}

	equalResult, err := runEngine(ctx, engineConfig(&equalweight.SignalSource{
		TargetNotional: targetNotional,
		MaxLots:        maxLots,
	}))
	if err != nil {
		return fmt.Errorf("equal-weight backtest: %w", err)
	}

	momentumResults := make([]namedResult, 0, 2)
	for _, variant := range []momentum.Variant{momentum.VariantLongOnly, momentum.VariantLongShort} {
		plan, err := momentum.BuildPlan(values, dates, k, rebalanceEvery, variant)
		if err != nil {
			return fmt.Errorf("build %s plan: %w", variant, err)
		}
		src := &momentum.SignalSource{Plan: plan, TargetNotional: targetNotional, MaxLots: maxLots}
		res, err := runEngine(ctx, engineConfig(src))
		if err != nil {
			return fmt.Errorf("%s backtest: %w", variant, err)
		}
		momentumResults = append(momentumResults, namedResult{name: variant.String(), result: res})
	}

	momentumLongShort := findResult(momentumResults, "long+short")
	if momentumLongShort == nil {
		return fmt.Errorf("momentum long+short result missing")
	}

	imoexReturns := betaregime.DailyReturnsFromCandles(imoexCandles)
	benchReturns := betaregime.DailyReturnsFromCurve(equalResult.EquityCurve)
	momentumReturns := betaregime.DailyReturnsFromCurve(momentumLongShort.EquityCurve)

	samples := betaregime.BuildTradeSamples(ensResult.Trades, imoexReturns, benchReturns, momentumReturns)
	decomposition := betaregime.Decompose(samples)

	tickerIDs := make(map[string]int)
	for _, s := range samples {
		ticker := strings.ToUpper(strings.TrimSpace(s.Ticker))
		if _, ok := tickerIDs[ticker]; ok {
			continue
		}
		tickerIDs[ticker] = len(tickerIDs)
	}
	tradeY := make([]float64, 0, len(samples))
	tradeX := make([][]float64, 0, len(samples))
	tradeClusters := make([]int, 0, len(samples))
	for _, s := range samples {
		tradeY = append(tradeY, s.ReturnPct)
		tradeX = append(tradeX, []float64{s.BenchReturn, s.MomentumReturn})
		tradeClusters = append(tradeClusters, tickerIDs[strings.ToUpper(strings.TrimSpace(s.Ticker))])
	}
	tradeRegression, err := betaregime.FitClusterOLS(tradeY, tradeX, tradeClusters)
	if err != nil {
		return fmt.Errorf("per-trade regression: %w", err)
	}

	ensDaily := betaregime.DailyReturnsFromCurve(ensResult.EquityCurve)
	dailyY, dailyX, dailyClusters, _, dailySkipped := betaregime.AlignDaily(ensDaily, benchReturns, momentumReturns, windows)
	dailyRegression, err := betaregime.FitClusterOLS(dailyY, dailyX, dailyClusters)
	if err != nil {
		return fmt.Errorf("per-day regression: %w", err)
	}

	regimes := betaregime.ClassifyWindows(imoexCandles, windows, flatBand)

	report := buildReport(reportInput{
		from:            from,
		till:            till,
		tickers:         tickers,
		ensemblePath:    ensemblePath,
		deposit:         deposit,
		commission:      commissionRate,
		spread:          spreadPct,
		slippage:        slippagePct,
		targetNotional:  targetNotional,
		k:               k,
		rebalanceEvery:  rebalanceEvery,
		flatBand:        flatBand,
		ensemble:        ensResult,
		equalWeight:     equalResult,
		momentum:        momentumResults,
		decomposition:   decomposition,
		samples:         len(samples),
		tradeRegression: tradeRegression,
		dailyRegression: dailyRegression,
		dailySkipped:    dailySkipped,
		regimes:         regimes,
	})

	if outPath != "" {
		if err := os.WriteFile(outPath, []byte(report), 0o644); err != nil {
			return fmt.Errorf("write report: %w", err)
		}
		log.Printf("betaregime: report written to %s", outPath)
	} else {
		fmt.Print(report)
	}
	return nil
}

type memorySource struct {
	candles map[string][]moex.Candle
}

func (m memorySource) History(_ context.Context, ticker string, _, _ time.Time) ([]moex.Candle, error) {
	candles, ok := m.candles[strings.ToUpper(strings.TrimSpace(ticker))]
	if !ok {
		return nil, fmt.Errorf("betaregime: no history for %s", ticker)
	}
	return candles, nil
}

func runEngine(ctx context.Context, cfg backtest.Config) (*backtest.Result, error) {
	engine, err := backtest.NewEngine(cfg)
	if err != nil {
		return nil, err
	}
	return engine.Run(ctx)
}

type namedResult struct {
	name   string
	result *backtest.Result
}

func findResult(results []namedResult, name string) *backtest.Result {
	for _, r := range results {
		if r.name == name {
			return r.result
		}
	}
	return nil
}

func splitComma(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func dropIncompleteTrailing(candles []moex.Candle) []moex.Candle {
	if len(candles) == 0 {
		return candles
	}
	last := candles[len(candles)-1]
	now := time.Now().UTC()
	if !last.Begin.IsZero() {
		lastYear, lastMonth, lastDay := last.Begin.Date()
		nowYear, nowMonth, nowDay := now.Date()
		if lastYear == nowYear && lastMonth == nowMonth && lastDay == nowDay {
			return candles[:len(candles)-1]
		}
	}
	return candles
}

func defaultWindows() []betaregime.Window {
	return []betaregime.Window{
		{From: day(2025, 4, 1), Till: day(2025, 6, 30)},
		{From: day(2025, 7, 1), Till: day(2025, 9, 30)},
		{From: day(2025, 10, 1), Till: day(2025, 12, 30)},
		{From: day(2026, 1, 5), Till: day(2026, 3, 31)},
		{From: day(2026, 4, 1), Till: day(2026, 6, 30)},
		{From: day(2026, 7, 1), Till: day(2026, 9, 17)},
	}
}

func resolveWindows(flagValue string, from, till time.Time) ([]betaregime.Window, error) {
	if flagValue == "" {
		return defaultWindows(), nil
	}
	var out []betaregime.Window
	for _, raw := range splitComma(flagValue) {
		parts := strings.Split(raw, ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("parse -windows: %q is not a from:till pair", raw)
		}
		wFrom, err := time.Parse("2006-01-02", strings.TrimSpace(parts[0]))
		if err != nil {
			return nil, fmt.Errorf("parse -windows from: %w", err)
		}
		wTill, err := time.Parse("2006-01-02", strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, fmt.Errorf("parse -windows till: %w", err)
		}
		out = append(out, betaregime.Window{From: wFrom, Till: wTill})
	}
	return out, nil
}

func day(year, month, day int) time.Time {
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}
