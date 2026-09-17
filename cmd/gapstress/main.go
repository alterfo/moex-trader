package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/features"
	"github.com/olegsidorkin/moex-trader/internal/gapstress"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
	"github.com/olegsidorkin/moex-trader/internal/model"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var configPath, tickersStr, fromStr, tillStr, ensemblePath, outPath, portfolioPath string
	var depositStr, commissionStr, spreadStr, slippageStr, targetNotionalStr string
	var maxLots, lookbackDays, gapLookbackDays int

	flag.StringVar(&configPath, "config", "config.sandbox.yaml", "path to config YAML")
	flag.StringVar(&tickersStr, "tickers", "", "comma-separated tickers (overrides config)")
	flag.StringVar(&fromStr, "from", "", "portfolio window start YYYY-MM-DD (default: till - lookback days)")
	flag.StringVar(&tillStr, "till", "", "portfolio window end YYYY-MM-DD (default: today)")
	flag.StringVar(&ensemblePath, "ensemble-path", "", "ensemble model JSON (default: model.ensemble_path from config)")
	flag.StringVar(&portfolioPath, "portfolio", "", "JSON portfolio to stress instead of backtest open positions")
	flag.StringVar(&outPath, "out", "", "path to write markdown report (default: stdout)")
	flag.StringVar(&depositStr, "deposit", "1000000", "starting deposit in RUB")
	flag.StringVar(&commissionStr, "commission-rate", "0.0005", "commission rate per fill")
	flag.StringVar(&spreadStr, "spread-pct", "0.0005", "half-spread cost per fill as fraction of price")
	flag.StringVar(&slippageStr, "slippage-pct", "0.0005", "slippage cost per fill as fraction of price")
	flag.StringVar(&targetNotionalStr, "target-notional", "15000", "target ruble notional per position")
	flag.IntVar(&maxLots, "max-lots", 1000, "risk-gate max lots ceiling")
	flag.IntVar(&lookbackDays, "lookback-days", 90, "portfolio replay window in calendar days")
	flag.IntVar(&gapLookbackDays, "gap-lookback-days", 365, "history window for overnight-gap extraction in calendar days")
	flag.Parse()

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

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	tickers := cfg.Tickers
	if tickersStr != "" {
		tickers = splitComma(tickersStr)
	}
	if len(tickers) == 0 {
		return fmt.Errorf("gapstress: no tickers configured")
	}
	if ensemblePath == "" {
		ensemblePath = cfg.Model.EnsemblePath
	}

	till := time.Now()
	if tillStr != "" {
		till, err = time.Parse("2006-01-02", tillStr)
		if err != nil {
			return fmt.Errorf("parse -till: %w", err)
		}
	}
	from := till.AddDate(0, 0, -lookbackDays)
	if fromStr != "" {
		from, err = time.Parse("2006-01-02", fromStr)
		if err != nil {
			return fmt.Errorf("parse -from: %w", err)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	portfolio, err := buildPortfolio(ctx, portfolioPath, deposit, from, till, tickers, ensemblePath, targetNotional, maxLots, commissionRate, spreadPct, slippagePct, cfg)
	if err != nil {
		return err
	}

	source := backtest.NewISSSource(cfg.MOEXISSBaseURL, moex.NewClient(cfg.MOEXISSBaseURL, nil))
	gapFrom := till.AddDate(0, 0, -gapLookbackDays)
	candlesByTicker, err := fetchGapCandles(ctx, source, tickers, gapFrom, till)
	if err != nil {
		return err
	}
	gaps := gapstress.OvernightGapsByTicker(candlesByTicker)

	var scenarios []gapstress.Scenario
	if worst, worstDate, ok := gapstress.WorstHistoricalDayScenario(portfolio, gaps); ok {
		worst.Name = fmt.Sprintf("worst historical day (%s)", worstDate.Format("2006-01-02"))
		scenarios = append(scenarios, worst)
	}
	scenarios = append(scenarios, gapstress.WorstPerTickerScenario(portfolio, gaps))
	scenarios = append(scenarios, gapstress.UniformShockScenario(tickers, "synthetic -10%", -0.10))
	scenarios = append(scenarios, gapstress.UniformShockScenario(tickers, "synthetic -20%", -0.20))
	scenarios = append(scenarios, gapstress.UniformShockScenario(tickers, "synthetic +10%", 0.10))
	scenarios = append(scenarios, gapstress.UniformShockScenario(tickers, "synthetic +20%", 0.20))

	report := buildReport(portfolio, scenarios, tickers, ensemblePath, from, till, gapLookbackDays, deposit, commissionRate, spreadPct, slippagePct)
	if outPath != "" {
		if err := os.WriteFile(outPath, []byte(report), 0o644); err != nil {
			return fmt.Errorf("write report: %w", err)
		}
		log.Printf("gapstress: report written to %s", outPath)
	} else {
		fmt.Print(report)
	}
	return nil
}

func buildPortfolio(ctx context.Context, portfolioPath string, deposit decimal.Decimal, from, till time.Time, tickers []string, ensemblePath string, targetNotional decimal.Decimal, maxLots int, commissionRate, spreadPct, slippagePct decimal.Decimal, cfg *config.Config) (gapstress.Portfolio, error) {
	if portfolioPath != "" {
		raw, err := os.ReadFile(portfolioPath)
		if err != nil {
			return gapstress.Portfolio{}, fmt.Errorf("read portfolio: %w", err)
		}
		var portfolio gapstress.Portfolio
		if err := json.Unmarshal(raw, &portfolio); err != nil {
			return gapstress.Portfolio{}, fmt.Errorf("parse portfolio: %w", err)
		}
		return portfolio, nil
	}

	ensemble, err := model.LoadEnsembleModel(ensemblePath)
	if err != nil {
		return gapstress.Portfolio{}, err
	}
	source := &model.EnsembleSignalSource{Model: ensemble, MaxLots: maxLots, TargetNotional: targetNotional}
	engine, err := backtest.NewEngine(backtest.Config{
		Tickers:        tickers,
		From:           from,
		Till:           till,
		Deposit:        deposit,
		MaxLots:        maxLots,
		CommissionRate: commissionRate,
		SpreadPct:      spreadPct,
		SlippagePct:    slippagePct,
		WarmupDays:     100,
		KillSwitch:     true,
		SignalSource:   source,
		Source:         backtest.NewISSSource(cfg.MOEXISSBaseURL, moex.NewClient(cfg.MOEXISSBaseURL, nil)),
		FeatureConfig:  features.PriceFeatureConfig{},
	})
	if err != nil {
		return gapstress.Portfolio{}, err
	}
	result, err := engine.Run(ctx)
	if err != nil {
		return gapstress.Portfolio{}, err
	}
	portfolio := gapstress.Portfolio{Deposit: deposit, Positions: make([]gapstress.Position, 0, len(result.OpenPositions))}
	for _, open := range result.OpenPositions {
		side := gapstress.SideLong
		if open.Side == domain.ActionSell {
			side = gapstress.SideShort
		}
		notional := open.MarkPrice.Mul(decimal.NewFromInt(int64(open.Lots)))
		if !notional.IsPositive() {
			notional = open.EntryPrice.Mul(decimal.NewFromInt(int64(open.Lots)))
		}
		portfolio.Positions = append(portfolio.Positions, gapstress.Position{Ticker: open.Ticker, Side: side, Notional: notional})
	}
	return portfolio, nil
}

func fetchGapCandles(ctx context.Context, source backtest.HistoricalSource, tickers []string, from, till time.Time) (map[string][]moex.Candle, error) {
	out := make(map[string][]moex.Candle, len(tickers))
	for _, ticker := range tickers {
		ticker = strings.ToUpper(strings.TrimSpace(ticker))
		candles, err := source.History(ctx, ticker, from, till)
		if err != nil {
			return nil, fmt.Errorf("gapstress: history %s: %w", ticker, err)
		}
		out[ticker] = candles
	}
	return out, nil
}

func buildReport(portfolio gapstress.Portfolio, scenarios []gapstress.Scenario, tickers []string, ensemblePath string, from, till time.Time, gapLookbackDays int, deposit, commission, spread, slippage decimal.Decimal) string {
	impacts := gapstress.Run(portfolio, scenarios)
	var b strings.Builder
	b.WriteString("# Gap-stress test (Task 8)\n\n")
	b.WriteString("- Window: ")
	b.WriteString(from.Format("2006-01-02"))
	b.WriteString(" -> ")
	b.WriteString(till.Format("2006-01-02"))
	b.WriteString("\n")
	fmt.Fprintf(&b, "- Tickers: %d (%s)\n", len(tickers), strings.Join(tickers, ", "))
	fmt.Fprintf(&b, "- Ensemble artifact: %s\n", ensemblePath)
	fmt.Fprintf(&b, "- Costs per fill: commission %s, spread %s, slippage %s\n", commission.String(), spread.String(), slippage.String())
	fmt.Fprintf(&b, "- Deposit: %s RUB; gap history lookback: %d days\n", deposit.String(), gapLookbackDays)
	fmt.Fprintf(&b, "- Open positions: %d; gross exposure %s RUB (long %s, short %s, net %s)\n\n",
		len(portfolio.Positions), gapstress.GrossExposure(portfolio).String(),
		gapstress.LongExposure(portfolio).String(), gapstress.ShortExposure(portfolio).String(),
		gapstress.NetExposure(portfolio).String())

	b.WriteString("## Open positions\n\n")
	if len(portfolio.Positions) == 0 {
		b.WriteString("- none\n\n")
	} else {
		b.WriteString("| ticker | side | notional |\n|---|---|---|\n")
		sorted := append([]gapstress.Position(nil), portfolio.Positions...)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].Ticker < sorted[j].Ticker })
		for _, pos := range sorted {
			fmt.Fprintf(&b, "| %s | %s | %s |\n", pos.Ticker, pos.Side, pos.Notional.String())
		}
		b.WriteString("\n")
	}

	b.WriteString("## Scenarios vs floors\n\n")
	fmt.Fprintf(&b, "- Drawdown floor: %.1f%% (%s RUB); daily-loss floor: %.1f%%\n\n",
		gapstress.DrawdownFloorPct, portfolio.Deposit.Mul(decimal.NewFromFloat(gapstress.DrawdownFloorPct)).Div(decimal.NewFromInt(100)).String(),
		gapstress.DailyLossFloorPct)
	b.WriteString("| scenario | P&L | P&L % of deposit | breaches 3% DD | breaches 0.5% daily |\n")
	b.WriteString("|---|---|---|---|---|\n")
	for _, impact := range impacts {
		fmt.Fprintf(&b, "| %s | %s | %s%% | %t | %t |\n",
			impact.Name, impact.Pnl.Round(2).String(), impact.PnlPct.Round(2).String(),
			impact.BreachesDrawdownFloor, impact.BreachesDailyLoss)
	}
	b.WriteString("\n")
	b.WriteString("- A breach is a loss at least as large as the floor. The sticky kill switch does not liquidate open positions, so a gap hits equity in full before the switch can freeze new entries.\n")
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
