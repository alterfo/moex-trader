package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/features"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
	"github.com/olegsidorkin/moex-trader/internal/model"
	"github.com/olegsidorkin/moex-trader/internal/tailprecision"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var configPath, tickersStr, fromStr, tillStr, ensemblePath, cachePath, outPath string
	var horizonDays int
	var deadbandPct, buyPct, sellPct float64
	var trials int
	var seed int64

	flag.StringVar(&configPath, "config", "config.sandbox.yaml", "path to config YAML")
	flag.StringVar(&tickersStr, "tickers", "", "comma-separated tickers (overrides config)")
	flag.StringVar(&fromStr, "from", "", "window start YYYY-MM-DD (default: till - 90 days)")
	flag.StringVar(&tillStr, "till", "", "window end YYYY-MM-DD (default: today)")
	flag.StringVar(&ensemblePath, "ensemble-path", "", "ensemble model JSON (default: model.ensemble_path from config)")
	flag.StringVar(&cachePath, "cache", "", "path to persistent decision cache (replays cached probabilities when present)")
	flag.StringVar(&outPath, "out", "", "path to write markdown report (default: stdout)")
	flag.IntVar(&horizonDays, "horizon-days", 10, "forward outcome horizon in trading days (matches the training label)")
	flag.Float64Var(&deadbandPct, "deadband-pct", 0.5, "deadband in %%: outcome requires |move| > deadband")
	flag.Float64Var(&buyPct, "buy-pct", 0.60, "BUY tail threshold p >= buy-pct")
	flag.Float64Var(&sellPct, "sell-pct", 0.40, "SELL tail threshold p <= sell-pct")
	flag.IntVar(&trials, "trials", 10000, "bootstrap trials")
	flag.Int64Var(&seed, "seed", 42, "bootstrap RNG seed")
	flag.Parse()

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	tickers := cfg.Tickers
	if tickersStr != "" {
		tickers = splitComma(tickersStr)
	}
	if ensemblePath == "" {
		ensemblePath = cfg.Model.EnsemblePath
	}
	if ensemblePath == "" {
		ensemblePath = "ensemble_model.json"
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
	source := backtest.NewISSSource(cfg.MOEXISSBaseURL, moexClient)

	signalSource := &model.EnsembleSignalSource{Model: ens, MaxLots: 1}
	var cached *backtest.CachedSignalSource
	if cachePath != "" {
		cached, err = backtest.NewCachedSignalSource(signalSource, cachePath)
		if err != nil {
			return err
		}
		defer func() {
			if err := cached.Save(); err != nil {
				log.Printf("save decision cache: %v", err)
			}
		}()
	}

	ctx := context.Background()
	var returns []float64
	var decisions []tailprecision.Decision

	for _, ticker := range tickers {
		ticker = strings.ToUpper(strings.TrimSpace(ticker))
		if ticker == "" {
			continue
		}
		candles, err := source.History(ctx, ticker, from.AddDate(0, 0, -features.PriceFeatureConfig{}.FetchCalendarDays()), till)
		if err != nil {
			log.Printf("tailprecision: %s: history: %v", ticker, err)
			continue
		}
		candles = dropIncompleteTrailing(candles)
		const warmup = 64
		if len(candles) < warmup+1 {
			continue
		}
		builder := features.NewBuilderWithConfig(time.Now, features.PriceFeatureConfig{})
		for d := warmup; d < len(candles); d++ {
			decisionDay := candles[d].Begin
			if decisionDay.Before(from) {
				continue
			}
			if d+1+horizonDays >= len(candles) {
				continue
			}
			entry := candles[d+1].Open
			exit := candles[d+1+horizonDays].Close
			if entry.Sign() <= 0 || exit.Sign() <= 0 {
				continue
			}
			forwardReturn, _ := exit.Sub(entry).Div(entry).Mul(decimal.NewFromInt(100)).Float64()

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

			var signal domain.TradeSignal
			if cached != nil {
				signal, err = cached.Generate(ctx, feature)
			} else {
				signal, err = signalSource.Generate(ctx, feature)
			}
			if err != nil {
				continue
			}
			probability, ok := probabilityFromReasoning(signal.Reasoning)
			if !ok {
				vector, _ := model.ToVector(feature)
				probability = ens.Probability(vector)
			}

			returns = append(returns, forwardReturn)

			var direction tailprecision.Direction
			switch {
			case probability >= buyPct:
				direction = tailprecision.DirectionBuy
			case probability <= sellPct:
				direction = tailprecision.DirectionSell
			default:
				direction = tailprecision.DirectionHold
			}
			if direction == tailprecision.DirectionHold {
				continue
			}
			decisions = append(decisions, tailprecision.Decision{
				Ticker:           ticker,
				Date:             decisionDay,
				Direction:        direction,
				Probability:      probability,
				ForwardReturnPct: forwardReturn,
			})
		}
	}

	base := tailprecision.ComputeBaseRate(returns, deadbandPct)
	precision, wins := tailprecision.PooledPrecision(decisions, deadbandPct)
	pooledBase := tailprecision.PooledBaseRate(decisions, base)
	boot := tailprecision.BootstrapPValue(decisions, pooledBase, deadbandPct, rand.New(rand.NewSource(seed)), trials)

	var buys, sells int
	for _, d := range decisions {
		if d.Direction == tailprecision.DirectionBuy {
			buys++
		} else {
			sells++
		}
	}
	episodes := len(tailprecision.Episodes(decisions))

	report := buildReport(reportInput{
		from:       from,
		till:       till,
		tickers:    len(tickers),
		ensemble:   ensemblePath,
		horizon:    horizonDays,
		deadband:   deadbandPct,
		buyPct:     buyPct,
		sellPct:    sellPct,
		samples:    base.Samples,
		upRate:     base.Up,
		downRate:   base.Down,
		buys:       buys,
		sells:      sells,
		total:      len(decisions),
		precision:  precision,
		wins:       wins,
		pooledBase: pooledBase,
		episodes:   episodes,
		boot:       boot,
		useCache:   cachePath != "",
	})

	if outPath != "" {
		if err := os.WriteFile(outPath, []byte(report), 0o644); err != nil {
			return fmt.Errorf("write report %q: %w", outPath, err)
		}
		log.Printf("tailprecision: report written to %s", outPath)
	} else {
		fmt.Print(report)
	}
	return nil
}

type reportInput struct {
	from, till                time.Time
	tickers                   int
	ensemble                  string
	horizon                   int
	deadband, buyPct, sellPct float64
	samples                   int
	upRate, downRate          float64
	buys, sells, total        int
	precision                 float64
	wins                      int
	pooledBase                float64
	episodes                  int
	boot                      tailprecision.BootstrapResult
	useCache, approxNote      bool
}

func buildReport(in reportInput) string {
	var b strings.Builder
	b.WriteString("# Tail-precision falsification test (Task 1)\n\n")
	fmt.Fprintf(&b, "- Window: %s -> %s\n", in.from.Format("2006-01-02"), in.till.Format("2006-01-02"))
	fmt.Fprintf(&b, "- Tickers: %d\n", in.tickers)
	fmt.Fprintf(&b, "- Artifact: %s (deployed abs-10d ensemble)\n", in.ensemble)
	fmt.Fprintf(&b, "- Horizon: %d trading days, deadband: +/-%.1f%%\n", in.horizon, in.deadband)
	fmt.Fprintf(&b, "- Tail bands: p >= %.2f (BUY), p <= %.2f (SELL)\n", in.buyPct, in.sellPct)
	if in.useCache {
		b.WriteString("- Probabilities: decision cache (replayed when present)\n")
	}
	b.WriteString("\n## Base rate\n\n")
	fmt.Fprintf(&b, "- Decision-day samples: %d\n", in.samples)
	fmt.Fprintf(&b, "- P(move > +%.1f%%): %.1f%%\n", in.deadband, in.upRate*100)
	fmt.Fprintf(&b, "- P(move < -%.1f%%): %.1f%%\n", in.deadband, in.downRate*100)
	b.WriteString("\n## Tail decisions\n\n")
	fmt.Fprintf(&b, "- BUY (p >= %.2f): %d\n", in.buyPct, in.buys)
	fmt.Fprintf(&b, "- SELL (p <= %.2f): %d\n", in.sellPct, in.sells)
	fmt.Fprintf(&b, "- Total tail decisions: %d\n", in.total)
	fmt.Fprintf(&b, "- Pooled precision (outcome in signal direction): %.1f%% (%d/%d)\n", in.precision*100, in.wins, in.total)
	fmt.Fprintf(&b, "- Pooled base rate for comparison: %.1f%%\n", in.pooledBase*100)
	b.WriteString("\n## Bootstrap p-value\n\n")
	fmt.Fprintf(&b, "- Episodes (position runs): %d\n", in.episodes)
	fmt.Fprintf(&b, "- Trials: %d\n", in.boot.Trials)
	b.WriteString("- Method: block bootstrap over episodes, recentered at the base rate, right-tailed\n")
	fmt.Fprintf(&b, "- Bootstrap mean/SD precision: %.4f / %.4f\n", in.boot.BootstrapMean, in.boot.BootstrapSD)
	fmt.Fprintf(&b, "- p-value (H0: precision = base rate): %.4f\n", in.boot.PValue)
	b.WriteString("\n## Caveat\n\n")
	b.WriteString("- This run uses the deployed artifact over the available window as an approximation: per-quarter models were not saved, so the exact walk-forward version depends on Task 13's persisted per-quarter models.\n")
	b.WriteString("- Episodes block the serial dependence within a position, but cross-ticker correlation (beta) is not modeled here; that is Task 9's decomposition.\n")
	return b.String()
}

func probabilityFromReasoning(reasoning string) (float64, bool) {
	idx := strings.Index(reasoning, "p=")
	if idx < 0 {
		return 0, false
	}
	rest := reasoning[idx+2:]
	if end := strings.IndexAny(rest, " ,"); end >= 0 {
		rest = rest[:end]
	}
	value, err := strconv.ParseFloat(rest, 64)
	if err != nil {
		return 0, false
	}
	return value, true
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
