package model

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/features"
)

type CalibrationSample struct {
	Ticker       string
	Date         time.Time
	Vector       []float64
	Names        []string
	ExcessReturn map[int]float64
}

func BuildCalibrationSamples(ctx context.Context, source backtest.HistoricalSource, tickers []string, from, till time.Time, horizons []int, stride int) ([]CalibrationSample, error) {
	if source == nil {
		return nil, fmt.Errorf("model: historical source is required")
	}
	if len(horizons) == 0 {
		return nil, fmt.Errorf("model: at least one horizon is required")
	}
	for _, h := range horizons {
		if h <= 0 {
			return nil, fmt.Errorf("model: horizon days must be positive, got %d", h)
		}
	}
	if stride <= 0 {
		return nil, fmt.Errorf("model: stride must be positive, got %d", stride)
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

	indexCandles, err := source.History(ctx, benchmarkTicker, fetchFrom, till)
	if err != nil {
		return nil, fmt.Errorf("model: history %s: %w", benchmarkTicker, err)
	}
	if len(indexCandles) == 0 {
		return nil, fmt.Errorf("model: benchmark %s has no history in the requested window", benchmarkTicker)
	}
	indexByDate := indexCandlesByDate(indexCandles)

	builder := features.NewBuilder(time.Now)
	var samples []CalibrationSample
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
		for d := minFeatureCandles; d < len(candles); d += stride {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			decisionDay := candles[d].Begin
			if decisionDay.Before(from) {
				continue
			}
			if d+1 >= len(candles) {
				continue
			}
			entryCandle := candles[d+1]
			entry := entryCandle.Open
			if entry.Sign() <= 0 {
				continue
			}
			indexEntry, ok := indexByDate[dateKey(entryCandle.Begin)]
			if !ok || indexEntry.Open.Sign() <= 0 {
				continue
			}

			excess := make(map[int]float64, len(horizons))
			for _, h := range horizons {
				if d+1+h >= len(candles) {
					continue
				}
				exitCandle := candles[d+1+h]
				exit := exitCandle.Close
				if exit.Sign() <= 0 {
					continue
				}
				indexExit, ok := indexByDate[dateKey(exitCandle.Begin)]
				if !ok || indexExit.Close.Sign() <= 0 {
					continue
				}
				forwardReturn := exit.Sub(entry).Div(entry).Mul(decimal.NewFromInt(100))
				indexForwardReturn := indexExit.Close.Sub(indexEntry.Open).Div(indexEntry.Open).Mul(decimal.NewFromInt(100))
				excessPct, _ := forwardReturn.Sub(indexForwardReturn).Float64()
				excess[h] = excessPct
			}
			if len(excess) == 0 {
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
			vector, names := ToVector(feature)
			samples = append(samples, CalibrationSample{
				Ticker: ticker, Date: decisionDay, Vector: vector, Names: names, ExcessReturn: excess,
			})
		}
	}
	return samples, nil
}

type FeatureStats struct {
	Horizon     int
	N           int
	Correlation float64
	HitRate     float64
}

func Calibrate(samples []CalibrationSample, horizons []int) map[string]map[int]FeatureStats {
	return calibrateGroup(samples, horizons)
}

func CalibrateByTicker(samples []CalibrationSample, horizons []int) map[string]map[string]map[int]FeatureStats {
	byTicker := make(map[string][]CalibrationSample)
	for _, s := range samples {
		byTicker[s.Ticker] = append(byTicker[s.Ticker], s)
	}
	out := make(map[string]map[string]map[int]FeatureStats, len(byTicker))
	for ticker, rows := range byTicker {
		out[ticker] = calibrateGroup(rows, horizons)
	}
	return out
}

func calibrateGroup(samples []CalibrationSample, horizons []int) map[string]map[int]FeatureStats {
	if len(samples) == 0 {
		return map[string]map[int]FeatureStats{}
	}
	names := samples[0].Names
	result := make(map[string]map[int]FeatureStats, len(names))
	for i, name := range names {
		result[name] = make(map[int]FeatureStats, len(horizons))
		for _, h := range horizons {
			xs := make([]float64, 0, len(samples))
			ys := make([]float64, 0, len(samples))
			for _, s := range samples {
				y, ok := s.ExcessReturn[h]
				if !ok || i >= len(s.Vector) {
					continue
				}
				xs = append(xs, s.Vector[i])
				ys = append(ys, y)
			}
			corr, _ := pearsonCorrelation(xs, ys)
			result[name][h] = FeatureStats{Horizon: h, N: len(xs), Correlation: corr, HitRate: hitRate(xs, ys)}
		}
	}
	return result
}

func pearsonCorrelation(xs, ys []float64) (float64, bool) {
	n := len(xs)
	if n < 3 {
		return 0, false
	}
	var sumX, sumY float64
	for i := range xs {
		sumX += xs[i]
		sumY += ys[i]
	}
	meanX, meanY := sumX/float64(n), sumY/float64(n)
	var cov, varX, varY float64
	for i := range xs {
		dx, dy := xs[i]-meanX, ys[i]-meanY
		cov += dx * dy
		varX += dx * dx
		varY += dy * dy
	}
	if varX <= 0 || varY <= 0 {
		return 0, false
	}
	return cov / math.Sqrt(varX*varY), true
}

func hitRate(xs, ys []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	hits := 0
	for i := range xs {
		if (xs[i] > 0) == (ys[i] > 0) {
			hits++
		}
	}
	return float64(hits) / float64(len(xs))
}

func BuildCalibrationReport(samples []CalibrationSample, tickers []string, horizons []int) string {
	pooled := Calibrate(samples, horizons)
	byTicker := CalibrateByTicker(samples, horizons)
	mainHorizon := horizons[len(horizons)/2]

	names := make([]string, 0, len(pooled))
	for name := range pooled {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		return math.Abs(pooled[names[i]][mainHorizon].Correlation) > math.Abs(pooled[names[j]][mainHorizon].Correlation)
	})

	var b strings.Builder
	writeLine := func(s string) { b.WriteString(s); b.WriteByte('\n') }

	writeLine("# Калибровка фич по избыточной доходности (excess return над IMOEX)")
	writeLine("")
	writeLine(fmt.Sprintf("Samples: %d, tickers: %d, horizons (дней вперёд): %v", len(samples), len(tickers), horizons))
	writeLine("")
	writeLine("## Сводная корреляция признак × горизонт (все тикеры)")
	writeLine("")

	var header strings.Builder
	header.WriteString("| Признак |")
	for _, h := range horizons {
		fmt.Fprintf(&header, " %dд corr (N) |", h)
	}
	fmt.Fprintf(&header, " Hit-rate %dд |", mainHorizon)
	writeLine(header.String())

	var sep strings.Builder
	sep.WriteString("|---|")
	for range horizons {
		sep.WriteString("---:|")
	}
	sep.WriteString("---:|")
	writeLine(sep.String())

	for _, name := range names {
		var row strings.Builder
		fmt.Fprintf(&row, "| %s |", name)
		for _, h := range horizons {
			s := pooled[name][h]
			fmt.Fprintf(&row, " %+.3f (n=%d) |", s.Correlation, s.N)
		}
		main := pooled[name][mainHorizon]
		fmt.Fprintf(&row, " %.0f%% (n=%d) |", main.HitRate*100, main.N)
		writeLine(row.String())
	}
	writeLine("")

	topFeatures := names
	if len(topFeatures) > 4 {
		topFeatures = topFeatures[:4]
	}
	writeLine(fmt.Sprintf("## По тикерам (горизонт %dд)", mainHorizon))
	writeLine("")
	var tickerHeader strings.Builder
	tickerHeader.WriteString("| Тикер |")
	for _, f := range topFeatures {
		fmt.Fprintf(&tickerHeader, " %s |", f)
	}
	writeLine(tickerHeader.String())
	var tickerSep strings.Builder
	tickerSep.WriteString("|---|")
	for range topFeatures {
		tickerSep.WriteString("---:|")
	}
	writeLine(tickerSep.String())

	sortedTickers := append([]string(nil), tickers...)
	sort.Strings(sortedTickers)
	for _, ticker := range sortedTickers {
		stats, ok := byTicker[ticker]
		if !ok {
			continue
		}
		var row strings.Builder
		fmt.Fprintf(&row, "| %s |", ticker)
		for _, f := range topFeatures {
			s := stats[f][mainHorizon]
			fmt.Fprintf(&row, " %+.3f (n=%d) |", s.Correlation, s.N)
		}
		writeLine(row.String())
	}

	return b.String()
}
