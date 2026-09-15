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
)

const minFeatureCandles = 64

type LabeledSample struct {
	Feature   domain.FeatureContext
	Label     float64
	LabelDate time.Time
}

func BuildSamples(ctx context.Context, source backtest.HistoricalSource, tickers []string, from, till time.Time, horizonDays int, deadbandPct float64) ([]LabeledSample, error) {
	if source == nil {
		return nil, fmt.Errorf("model: historical source is required")
	}
	if horizonDays <= 0 {
		return nil, fmt.Errorf("model: horizon days must be positive, got %d", horizonDays)
	}
	if deadbandPct < 0 {
		return nil, fmt.Errorf("model: deadband pct must be non-negative, got %v", deadbandPct)
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

	builder := features.NewBuilder(time.Now)
	var samples []LabeledSample
	for _, ticker := range normalized {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		candles, err := source.History(ctx, ticker, fetchFrom, till)
		if err != nil {
			return nil, fmt.Errorf("model: history %s: %w", ticker, err)
		}
		if len(candles) < minFeatureCandles+1 {
			continue
		}
		for d := minFeatureCandles; d < len(candles); d++ {
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
			entry := candles[d+1].Open
			if entry.Sign() <= 0 {
				continue
			}
			exitCandle := candles[d+1+horizonDays]
			exit := exitCandle.Close
			if exit.Sign() <= 0 {
				continue
			}
			forwardReturn := exit.Sub(entry).Div(entry).Mul(decimal.NewFromInt(100))
			forwardPct, _ := forwardReturn.Float64()
			if math.Abs(forwardPct) < deadbandPct {
				continue
			}
			label := 0.0
			if forwardPct > 0 {
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
