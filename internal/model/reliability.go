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
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

type ReliabilityPair struct {
	Probability float64
	Label       float64
}

type ReliabilityBin struct {
	Start            float64
	End              float64
	Count            int
	MeanProbability  float64
	PositiveRate     float64
	CalibrationError float64
}

type ReliabilityStats struct {
	Samples int
	Brier   float64
	ECE     float64
}

type DecisionLabelRequest struct {
	Ticker      string
	Date        time.Time
	Probability float64
}

type LabeledProbability struct {
	Ticker      string
	Date        time.Time
	Probability float64
	Label       float64
}

func BrierScore(pairs []ReliabilityPair) float64 {
	stats, _ := ComputeReliability(pairs, 10)
	return stats.Brier
}

func ExpectedCalibrationError(pairs []ReliabilityPair, bins int) float64 {
	stats, _ := ComputeReliability(pairs, bins)
	return stats.ECE
}

func ReliabilityTable(pairs []ReliabilityPair, bins int) []ReliabilityBin {
	_, table := ComputeReliability(pairs, bins)
	return table
}

func ComputeReliability(pairs []ReliabilityPair, bins int) (ReliabilityStats, []ReliabilityBin) {
	if bins < 1 {
		bins = 10
	}
	table := make([]ReliabilityBin, bins)
	binWidth := 1.0 / float64(bins)
	for i := range table {
		table[i].Start = float64(i) * binWidth
		table[i].End = table[i].Start + binWidth
	}
	table[len(table)-1].End = 1.0

	var brierSum, eceNumerator float64
	samples := 0
	for _, pair := range pairs {
		if math.IsNaN(pair.Probability) || math.IsInf(pair.Probability, 0) || pair.Probability < 0 || pair.Probability > 1 {
			continue
		}
		if pair.Label != 0 && pair.Label != 1 {
			continue
		}
		bin := int(pair.Probability / binWidth)
		if bin >= len(table) {
			bin = len(table) - 1
		}
		table[bin].Count++
		table[bin].MeanProbability += pair.Probability
		table[bin].PositiveRate += pair.Label
		samples++
		diff := pair.Probability - pair.Label
		brierSum += diff * diff
	}

	for i := range table {
		if table[i].Count == 0 {
			continue
		}
		count := float64(table[i].Count)
		table[i].MeanProbability /= count
		table[i].PositiveRate /= count
		table[i].CalibrationError = math.Abs(table[i].MeanProbability - table[i].PositiveRate)
		eceNumerator += count * table[i].CalibrationError
	}

	stats := ReliabilityStats{Samples: samples}
	if samples > 0 {
		stats.Brier = brierSum / float64(samples)
		stats.ECE = eceNumerator / float64(samples)
	}
	return stats, table
}

func RealizedReliabilityPairs(ctx context.Context, source backtest.HistoricalSource, requests []DecisionLabelRequest, horizonDays int, deadbandPct float64, mode LabelMode) ([]LabeledProbability, error) {
	if source == nil {
		return nil, fmt.Errorf("model: historical source is required for reliability labels")
	}
	if horizonDays <= 0 {
		return nil, fmt.Errorf("model: horizon days must be positive, got %d", horizonDays)
	}
	if deadbandPct < 0 {
		return nil, fmt.Errorf("model: deadband pct must be non-negative, got %v", deadbandPct)
	}
	if mode == "" {
		mode = LabelModeAbsolute
	}
	if mode != LabelModeAbsolute && mode != LabelModeExcess {
		return nil, fmt.Errorf("model: reliability labels support absolute and excess modes, got %q", mode)
	}

	byTicker := make(map[string][]DecisionLabelRequest)
	var overallFrom, overallTill time.Time
	for _, request := range requests {
		if strings.TrimSpace(request.Ticker) == "" || request.Date.IsZero() {
			continue
		}
		ticker := normalizeTickers([]string{request.Ticker})[0]
		byTicker[ticker] = append(byTicker[ticker], request)
		if overallFrom.IsZero() || request.Date.Before(overallFrom) {
			overallFrom = request.Date
		}
		if overallTill.IsZero() || request.Date.After(overallTill) {
			overallTill = request.Date
		}
	}
	if len(byTicker) == 0 {
		return nil, nil
	}

	var indexByDate map[string]moex.Candle
	if mode == LabelModeExcess {
		indexCandles, err := source.History(ctx, benchmarkTicker, overallFrom.AddDate(0, 0, -10), overallTill.AddDate(0, 0, horizonDays+10))
		if err != nil {
			return nil, fmt.Errorf("model: reliability history %s: %w", benchmarkTicker, err)
		}
		indexByDate = indexCandlesByDate(indexCandles)
	}

	var out []LabeledProbability
	for ticker, group := range byTicker {
		sort.Slice(group, func(i, j int) bool { return group[i].Date.Before(group[j].Date) })
		candles, err := source.History(ctx, ticker, group[0].Date.AddDate(0, 0, -10), group[len(group)-1].Date.AddDate(0, 0, horizonDays+10))
		if err != nil {
			return nil, fmt.Errorf("model: reliability history %s: %w", ticker, err)
		}
		seen := make(map[string]struct{}, len(group))
		for _, request := range group {
			if _, ok := seen[dateKey(request.Date)]; ok {
				continue
			}
			seen[dateKey(request.Date)] = struct{}{}
			if math.IsNaN(request.Probability) || math.IsInf(request.Probability, 0) || request.Probability < 0 || request.Probability > 1 {
				continue
			}
			label, ok := realizedLabelForDate(candles, request.Date, horizonDays, deadbandPct, mode, indexByDate)
			if !ok {
				continue
			}
			out = append(out, LabeledProbability{
				Ticker:      ticker,
				Date:        request.Date,
				Probability: request.Probability,
				Label:       label,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date.Equal(out[j].Date) {
			return out[i].Ticker < out[j].Ticker
		}
		return out[i].Date.Before(out[j].Date)
	})
	return out, nil
}

func realizedLabelForDate(candles []moex.Candle, decisionDay time.Time, horizonDays int, deadbandPct float64, mode LabelMode, indexByDate map[string]moex.Candle) (float64, bool) {
	decisionKey := dateKey(decisionDay)
	entryIdx := -1
	for i, candle := range candles {
		if dateKey(candle.Begin) == decisionKey {
			entryIdx = i + 1
			break
		}
	}
	if entryIdx < 0 || entryIdx >= len(candles) {
		return 0, false
	}
	exitIdx := entryIdx + horizonDays
	if exitIdx >= len(candles) {
		return 0, false
	}

	entry := candles[entryIdx].Open
	exit := candles[exitIdx].Close
	if entry.Sign() <= 0 || exit.Sign() <= 0 {
		return 0, false
	}
	labelReturn := exit.Sub(entry).Div(entry).Mul(decimal.NewFromInt(100))
	if mode == LabelModeExcess {
		indexEntry, entryOK := indexByDate[dateKey(candles[entryIdx].Begin)]
		indexExit, exitOK := indexByDate[dateKey(candles[exitIdx].Begin)]
		if !entryOK || !exitOK || indexEntry.Open.Sign() <= 0 || indexExit.Close.Sign() <= 0 {
			return 0, false
		}
		indexReturn := indexExit.Close.Sub(indexEntry.Open).Div(indexEntry.Open).Mul(decimal.NewFromInt(100))
		labelReturn = labelReturn.Sub(indexReturn)
	}
	labelPct, _ := labelReturn.Float64()
	if math.Abs(labelPct) < deadbandPct {
		return 0, false
	}
	if labelPct > 0 {
		return 1, true
	}
	return 0, true
}
