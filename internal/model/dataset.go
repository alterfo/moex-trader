package model

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/features"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

const minFeatureCandles = 64

const benchmarkTicker = "IMOEX"

const dateKeyLayout = "2006-01-02"

type LabeledSample struct {
	Feature   domain.FeatureContext
	Label     float64
	LabelDate time.Time
}

type LabelMode string

const (
	LabelModeExcess   LabelMode = "excess"
	LabelModeAbsolute LabelMode = "absolute"
)

// BuildSamples labels each decision day/bar by the sign of its forward
// return (see LabelMode). commissionPct is the one-way commission rate (e.g.
// 0.0005 for 0.05%); pass 0 to disable cost-adjustment and keep prior
// behavior exactly. When positive, a sample's dead zone is widened by the
// round-trip commission (2x) plus that bar's own (High-Low)/Close spread
// proxy, so labels aren't assigned to moves too small to trade profitably -
// important on short (intraday) horizons where typical moves can be
// comparable to round-trip costs; on multi-day horizons this cost floor is
// negligible next to deadbandPct and changes nothing in practice.
func BuildSamples(ctx context.Context, source backtest.HistoricalSource, tickers []string, from, till time.Time, horizonDays int, deadbandPct float64, mode LabelMode, commissionPct float64) ([]LabeledSample, error) {
	return buildSamples(ctx, source, tickers, from, till, horizonDays, deadbandPct, mode, commissionPct, features.PriceFeatureConfig{})
}

// BuildSamplesWithFeatureConfig behaves exactly like BuildSamples but scales
// the day-denominated feature windows to the candle interval in use (see
// features.PriceFeatureConfig) - needed for intraday sources where a bar is
// minutes, not a day. The zero config is identical to BuildSamples.
func BuildSamplesWithFeatureConfig(ctx context.Context, source backtest.HistoricalSource, tickers []string, from, till time.Time, horizonDays int, deadbandPct float64, mode LabelMode, commissionPct float64, featureCfg features.PriceFeatureConfig) ([]LabeledSample, error) {
	return buildSamples(ctx, source, tickers, from, till, horizonDays, deadbandPct, mode, commissionPct, featureCfg)
}

func buildSamples(ctx context.Context, source backtest.HistoricalSource, tickers []string, from, till time.Time, horizonDays int, deadbandPct float64, mode LabelMode, commissionPct float64, featureCfg features.PriceFeatureConfig) ([]LabeledSample, error) {
	if source == nil {
		return nil, fmt.Errorf("model: historical source is required")
	}
	if horizonDays <= 0 {
		return nil, fmt.Errorf("model: horizon days must be positive, got %d", horizonDays)
	}
	if deadbandPct < 0 {
		return nil, fmt.Errorf("model: deadband pct must be non-negative, got %v", deadbandPct)
	}
	if commissionPct < 0 {
		return nil, fmt.Errorf("model: commission pct must be non-negative, got %v", commissionPct)
	}
	if mode == "" {
		mode = LabelModeExcess
	}
	if mode != LabelModeExcess && mode != LabelModeAbsolute {
		return nil, fmt.Errorf("model: unknown label mode %q", mode)
	}
	normalized := normalizeTickers(tickers)
	if len(normalized) == 0 {
		return nil, fmt.Errorf("model: tickers must not be empty")
	}
	if till.IsZero() {
		till = time.Now()
	}
	if from.IsZero() {
		from = till.AddDate(0, 0, -365)
	}
	fetchFrom := from.AddDate(0, 0, -backtest.DefaultWarmupDays)
	if extra := featureCfg.FetchCalendarDays(); extra > backtest.DefaultWarmupDays {
		fetchFrom = from.AddDate(0, 0, -extra)
	}

	var indexByDate map[string]moex.Candle
	if mode == LabelModeExcess {
		indexCandles, err := source.History(ctx, benchmarkTicker, fetchFrom, till)
		if err != nil {
			return nil, fmt.Errorf("model: history %s: %w", benchmarkTicker, err)
		}
		if len(indexCandles) == 0 {
			return nil, fmt.Errorf("model: benchmark %s has no history in the requested window", benchmarkTicker)
		}
		indexByDate = indexCandlesByDate(indexCandles)
	}

	builder := features.NewBuilderWithConfig(time.Now, featureCfg)
	var samples []LabeledSample
	for _, ticker := range normalized {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		candles, err := source.History(ctx, ticker, fetchFrom, till)
		if err != nil {
			return nil, fmt.Errorf("model: history %s: %w", ticker, err)
		}
		warmup := featureCfg.WarmupCandles()
		if len(candles) < warmup+1 {
			continue
		}
		for d := warmup; d < len(candles); d++ {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			decisionDay := candles[d].Begin
			if decisionDay.Before(from) {
				continue
			}
			if d+1+horizonDays >= len(candles) {
				continue
			}
			entryCandle := candles[d+1]
			entry := entryCandle.Open
			if entry.Sign() <= 0 {
				continue
			}
			exitCandle := candles[d+1+horizonDays]
			exit := exitCandle.Close
			if exit.Sign() <= 0 {
				continue
			}

			indexEntry, ok := indexByDate[dateKey(entryCandle.Begin)]
			if mode == LabelModeExcess && (!ok || indexEntry.Open.Sign() <= 0) {
				continue
			}
			indexExit, ok := indexByDate[dateKey(exitCandle.Begin)]
			if mode == LabelModeExcess && (!ok || indexExit.Close.Sign() <= 0) {
				continue
			}

			forwardReturn := exit.Sub(entry).Div(entry).Mul(decimal.NewFromInt(100))
			labelReturn := forwardReturn
			if mode == LabelModeExcess {
				indexForwardReturn := indexExit.Close.Sub(indexEntry.Open).Div(indexEntry.Open).Mul(decimal.NewFromInt(100))
				labelReturn = forwardReturn.Sub(indexForwardReturn)
			}
			labelPct, _ := labelReturn.Float64()
			threshold := deadbandPct
			if commissionPct > 0 {
				spreadPct := 0.0
				if entryCandle.Close.Sign() > 0 {
					spreadPct, _ = entryCandle.High.Sub(entryCandle.Low).Div(entryCandle.Close).Mul(decimal.NewFromInt(100)).Float64()
				}
				threshold += 2*commissionPct*100 + spreadPct
			}
			if math.Abs(labelPct) < threshold {
				continue
			}
			label := 0.0
			if labelPct > 0 {
				label = 1
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
			samples = append(samples, LabeledSample{Feature: feature, Label: label, LabelDate: exitCandle.Begin})
		}
	}
	return samples, nil
}

func indexCandlesByDate(candles []moex.Candle) map[string]moex.Candle {
	byDate := make(map[string]moex.Candle, len(candles))
	for _, c := range candles {
		byDate[dateKey(c.Begin)] = c
	}
	return byDate
}

func dateKey(t time.Time) string {
	return t.Format(dateKeyLayout)
}

func normalizeTickers(tickers []string) []string {
	out := make([]string, 0, len(tickers))
	for _, ticker := range tickers {
		ticker = strings.ToUpper(strings.TrimSpace(ticker))
		if ticker != "" {
			out = append(out, ticker)
		}
	}
	return out
}
