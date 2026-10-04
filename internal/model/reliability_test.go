package model

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

func reliabilityDay(year int, month int, day int) time.Time {
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

func closeF64(t *testing.T, name string, got, want, tolerance float64) {
	t.Helper()
	if math.Abs(got-want) > tolerance {
		t.Fatalf("%s = %v, want %v (tolerance %v)", name, got, want, tolerance)
	}
}

func TestBrierScoreKnownInput(t *testing.T) {
	pairs := []ReliabilityPair{{Probability: 0.7, Label: 1}, {Probability: 0.3, Label: 0}}
	closeF64(t, "Brier", BrierScore(pairs), 0.09, 1e-12)
	closeF64(t, "ECE", ExpectedCalibrationError(pairs, 10), 0.3, 1e-12)
}

func TestReliabilityTableKnownInput(t *testing.T) {
	pairs := []ReliabilityPair{{Probability: 0.7, Label: 1}, {Probability: 0.3, Label: 0}}
	table := ReliabilityTable(pairs, 10)
	if len(table) != 10 {
		t.Fatalf("len(table) = %d, want 10", len(table))
	}
	if table[2].Count != 1 || table[6].Count != 1 {
		t.Fatalf("unexpected table counts: %+v", table)
	}
	closeF64(t, "bin 2 positive rate", table[2].PositiveRate, 0, 1e-12)
	closeF64(t, "bin 6 positive rate", table[6].PositiveRate, 1, 1e-12)
}

func TestComputeReliabilityEmptyAndDegenerate(t *testing.T) {
	stats, table := ComputeReliability(nil, 10)
	if stats.Samples != 0 || stats.Brier != 0 || stats.ECE != 0 {
		t.Fatalf("empty stats = %+v, want zero", stats)
	}
	if len(table) != 10 {
		t.Fatalf("len(table) = %d, want 10", len(table))
	}
	for i, bin := range table {
		if bin.Count != 0 {
			t.Fatalf("bin %d count = %d, want 0", i, bin.Count)
		}
	}

	stats, _ = ComputeReliability([]ReliabilityPair{{Probability: 0.9, Label: 1}, {Probability: 0.9, Label: 1}}, 10)
	closeF64(t, "degenerate Brier", stats.Brier, 0.01, 1e-12)
	closeF64(t, "degenerate ECE", stats.ECE, 0.1, 1e-12)
}

func TestComputeReliabilitySkipsInvalidPairs(t *testing.T) {
	pairs := []ReliabilityPair{
		{Probability: 1.5, Label: 1},
		{Probability: 0.7, Label: 2},
		{Probability: 0.7, Label: 1},
		{Probability: 0.3, Label: 0},
	}
	stats, _ := ComputeReliability(pairs, 10)
	if stats.Samples != 2 {
		t.Fatalf("Samples = %d, want 2", stats.Samples)
	}
}

type reliabilityFakeSource struct {
	candles map[string][]moex.Candle
}

func (s reliabilityFakeSource) History(_ context.Context, ticker string, _, _ time.Time) ([]moex.Candle, error) {
	return s.candles[ticker], nil
}

func risingReliabilityCandles(n int, start time.Time) []moex.Candle {
	candles := make([]moex.Candle, n)
	for i := range candles {
		begin := start.AddDate(0, 0, i)
		open := decimal.NewFromInt(int64(100 + i))
		closePrice := decimal.NewFromInt(int64(101 + i))
		candles[i] = moex.Candle{Begin: begin, End: begin, Open: open, Close: closePrice, High: closePrice, Low: open}
	}
	return candles
}

func TestRealizedReliabilityPairsAbsoluteLabel(t *testing.T) {
	start := reliabilityDay(2025, 1, 1)
	source := reliabilityFakeSource{candles: map[string][]moex.Candle{"SBER": risingReliabilityCandles(12, start)}}
	requests := []DecisionLabelRequest{{Ticker: "SBER", Date: start, Probability: 0.8}}
	labeled, err := RealizedReliabilityPairs(context.Background(), source, requests, 2, 0.5, LabelModeAbsolute)
	if err != nil {
		t.Fatal(err)
	}
	if len(labeled) != 1 {
		t.Fatalf("len(labeled) = %d, want 1", len(labeled))
	}
	closeF64(t, "label", labeled[0].Label, 1, 1e-12)
	closeF64(t, "probability", labeled[0].Probability, 0.8, 1e-12)
}

func TestRealizedReliabilityPairsDropsDeadbandAndTruncated(t *testing.T) {
	start := reliabilityDay(2025, 1, 1)
	source := reliabilityFakeSource{candles: map[string][]moex.Candle{"SBER": risingReliabilityCandles(6, start)}}
	requests := []DecisionLabelRequest{
		{Ticker: "SBER", Date: start, Probability: 0.8},
		{Ticker: "SBER", Date: start.AddDate(0, 0, 4), Probability: 0.8},
	}
	labeled, err := RealizedReliabilityPairs(context.Background(), source, requests, 2, 20, LabelModeAbsolute)
	if err != nil {
		t.Fatal(err)
	}
	if len(labeled) != 0 {
		t.Fatalf("len(labeled) = %d, want 0 (deadband or truncated)", len(labeled))
	}
}

func TestRealizedReliabilityPairsDeduplicatesSameDay(t *testing.T) {
	start := reliabilityDay(2025, 1, 1)
	source := reliabilityFakeSource{candles: map[string][]moex.Candle{"SBER": risingReliabilityCandles(12, start)}}
	requests := []DecisionLabelRequest{
		{Ticker: "SBER", Date: start, Probability: 0.8},
		{Ticker: "SBER", Date: start.Add(10 * time.Hour), Probability: 0.9},
	}
	labeled, err := RealizedReliabilityPairs(context.Background(), source, requests, 2, 0.5, LabelModeAbsolute)
	if err != nil {
		t.Fatal(err)
	}
	if len(labeled) != 1 {
		t.Fatalf("len(labeled) = %d, want 1 (same-day requests deduplicated)", len(labeled))
	}
	if labeled[0].Probability != 0.8 {
		t.Fatalf("probability = %v, want 0.8 (first request wins)", labeled[0].Probability)
	}
}

var _ backtest.HistoricalSource = reliabilityFakeSource{}
