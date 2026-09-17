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
	"github.com/olegsidorkin/moex-trader/internal/config"
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
	var configPath, tickersStr, fromStr, tillStr, ensemblePath, outPath, variantName string
	var depositStr, commissionStr, spreadStr, slippageStr, targetNotionalStr string
	var k, rebalanceEvery int
	var maxLots int

	flag.StringVar(&configPath, "config", "config.sandbox.yaml", "path to config YAML")
	flag.StringVar(&tickersStr, "tickers", "", "comma-separated tickers (overrides config)")
	flag.StringVar(&fromStr, "from", "", "window start YYYY-MM-DD (default: till - 90 days)")
	flag.StringVar(&tillStr, "till", "", "window end YYYY-MM-DD (default: today)")
	flag.StringVar(&ensemblePath, "ensemble-path", "", "ensemble model JSON (default: model.ensemble_path from config)")
	flag.StringVar(&variantName, "variant", "long-only", "momentum variant: long-only, long+short or both")
	flag.StringVar(&outPath, "out", "", "path to write markdown report (default: stdout)")
	flag.StringVar(&depositStr, "deposit", "1000000", "starting deposit in RUB")
	flag.StringVar(&commissionStr, "commission-rate", "0.0005", "commission rate per fill")
	flag.StringVar(&spreadStr, "spread-pct", "0.0005", "half-spread cost per fill as fraction of price")
	flag.StringVar(&slippageStr, "slippage-pct", "0.0005", "slippage cost per fill as fraction of price")
	flag.StringVar(&targetNotionalStr, "target-notional", "15000", "target ruble notional per position")
	flag.IntVar(&k, "k", 5, "top-k (and bottom-k for long+short) momentum selection")
	flag.IntVar(&rebalanceEvery, "rebalance-every", 10, "rebalance every N trading days")
	flag.IntVar(&maxLots, "max-lots", 1000, "risk-gate max lots ceiling")
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

	till := time.Now()
	if tillStr != "" {
		till, err = time.Parse("2006-01-02", tillStr)
		if err != nil {
			return fmt.Errorf("parse -till: %w", err)
		}
	}
	from := till.AddDate(0, 0, -90)
	if fromStr != "" {
		from, err = time.Parse("2006-01-02", fromStr)
		if err != nil {
			return fmt.Errorf("parse -from: %w", err)
		}
	}

	ens, err := model.LoadEnsembleModel(ensemblePath)
	if err != nil {
		return fmt.Errorf("load ensemble: %w", err)
	}

	moexClient := moex.NewClient(cfg.MOEXISSBaseURL, nil)
	fetchSource := backtest.NewISSSource(cfg.MOEXISSBaseURL, moexClient)
	warmupDays := features.PriceFeatureConfig{}.FetchCalendarDays()
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
			log.Printf("momentumbench: %s: history: %v", ticker, err)
			continue
		}
		candles = dropIncompleteTrailing(candles)
		if len(candles) < featureWarmup+1 {
			log.Printf("momentumbench: %s: only %d candles, skipping", ticker, len(candles))
			continue
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

	var variants []momentum.Variant
	switch variantName {
	case "long+short", "long-short", "longshort":
		variants = []momentum.Variant{momentum.VariantLongShort}
	case "long-only", "longonly":
		variants = []momentum.Variant{momentum.VariantLongOnly}
	case "both":
		variants = []momentum.Variant{momentum.VariantLongOnly, momentum.VariantLongShort}
	default:
		return fmt.Errorf("unknown -variant %q: want long-only, long+short or both", variantName)
	}

	var momentumResults []namedResult
	for _, variant := range variants {
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

	report := buildReport(reportInput{
		from:           from,
		till:           till,
		tickers:        tickers,
		ensemblePath:   ensemblePath,
		deposit:        deposit,
		commission:     commissionRate,
		spread:         spreadPct,
		slippage:       slippagePct,
		targetNotional: targetNotional,
		k:              k,
		rebalanceEvery: rebalanceEvery,
		tradingDays:    len(dates),
		ensemble:       ensResult,
		momentum:       momentumResults,
		ensembleBeats:  beatsAll(ensResult, momentumResults),
	})

	if outPath != "" {
		if err := os.WriteFile(outPath, []byte(report), 0o644); err != nil {
			return fmt.Errorf("write report: %w", err)
		}
		log.Printf("momentumbench: report written to %s", outPath)
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
		return nil, fmt.Errorf("momentumbench: no history for %s", ticker)
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

func beatsAll(ensemble *backtest.Result, benchmarks []namedResult) bool {
	for _, bench := range benchmarks {
		if ensemble.RealizedPnl.LessThanOrEqual(bench.result.RealizedPnl) {
			return false
		}
	}
	return true
}

type reportInput struct {
	from           time.Time
	till           time.Time
	tickers        []string
	ensemblePath   string
	deposit        decimal.Decimal
	commission     decimal.Decimal
	spread         decimal.Decimal
	slippage       decimal.Decimal
	targetNotional decimal.Decimal
	k              int
	rebalanceEvery int
	tradingDays    int
	ensemble       *backtest.Result
	momentum       []namedResult
	ensembleBeats  bool
}

func buildReport(in reportInput) string {
	var b strings.Builder
	b.WriteString("# Momentum benchmark (Task 2)\n\n")
	fmt.Fprintf(&b, "- Window: %s -> %s (%d trading days)\n", in.from.Format("2006-01-02"), in.till.Format("2006-01-02"), in.tradingDays)
	fmt.Fprintf(&b, "- Tickers: %d (%s)\n", len(in.tickers), strings.Join(in.tickers, ", "))
	fmt.Fprintf(&b, "- Ensemble artifact: %s\n", in.ensemblePath)
	fmt.Fprintf(&b, "- Costs: commission %s, spread %s, slippage %s (fractions of price)\n", in.commission.String(), in.spread.String(), in.slippage.String())
	fmt.Fprintf(&b, "- Deposit: %s RUB, target notional: %s RUB/position, k=%d, rebalance every %d trading days\n", in.deposit.String(), in.targetNotional.String(), in.k, in.rebalanceEvery)

	b.WriteString("\n## Results (net of costs)\n\n")
	b.WriteString("| strategy | realized P&L | MTM P&L | closed trades | win rate | max DD | kill switch |\n")
	b.WriteString("|---|---|---|---|---|---|---|\n")
	writeRow := func(name string, r *backtest.Result) {
		win := "-"
		if r.ClosedTrades > 0 {
			win = fmt.Sprintf("%.1f%%", r.HitRate*100)
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %d | %s | %.2f%% | %v |\n",
			name, r.RealizedPnl.Round(2).String(), r.NetPnl.Round(2).String(), r.ClosedTrades, win, r.MaxDrawdownPct, r.KillSwitchTripped)
	}
	writeRow("ensemble (deployed)", in.ensemble)
	for _, bench := range in.momentum {
		writeRow("momentum "+bench.name, bench.result)
	}

	b.WriteString("\n## Verdict\n\n")
	if in.ensembleBeats {
		b.WriteString("- Ensemble realized P&L beats every momentum benchmark variant net of costs.\n")
	} else {
		b.WriteString("- Ensemble realized P&L does NOT beat every momentum benchmark variant net of costs; treat the deployed model as momentum + noise pending further evidence.\n")
	}
	b.WriteString("- Realized P&L is the decision metric (AGENTS.md realized-vs-MTM invariant); MTM is shown separately.\n")
	return b.String()
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
