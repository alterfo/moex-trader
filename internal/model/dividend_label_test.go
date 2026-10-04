package model

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/dividends"
	"github.com/olegsidorkin/moex-trader/internal/features"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

func flatDividendCandles(n int, start time.Time) []moex.Candle {
	candles := make([]moex.Candle, n)
	for i := range candles {
		day := start.AddDate(0, 0, i)
		candles[i] = moex.Candle{
			Open:   decimal.NewFromInt(100),
			Close:  decimal.NewFromInt(100),
			High:   decimal.NewFromInt(101),
			Low:    decimal.NewFromInt(99),
			Volume: decimal.NewFromInt(1000),
			Begin:  day,
			End:    day,
		}
	}
	return candles
}

func labelsByDate(samples []LabeledSample) map[time.Time]float64 {
	out := make(map[time.Time]float64, len(samples))
	for _, sample := range samples {
		out[sample.LabelDate] = sample.Label
	}
	return out
}

func TestBuildSamplesAbsoluteTRMatchesAbsoluteWithoutDividends(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	source := datasetSource{series: map[string][]moex.Candle{
		"DIV": flatDividendCandles(80, start),
	}}
	from := start
	till := start.AddDate(0, 0, 200)

	absolute, err := BuildSamples(context.Background(), source, []string{"DIV"}, from, till, 1, 0, LabelModeAbsolute, 0)
	if err != nil {
		t.Fatalf("BuildSamples absolute: %v", err)
	}
	totalReturn, err := BuildSamplesWithDividends(context.Background(), source, []string{"DIV"}, from, till, 1, 0, LabelModeAbsoluteTR, 0, features.PriceFeatureConfig{}, HorizonModeBars, []dividends.Record{})
	if err != nil {
		t.Fatalf("BuildSamplesWithDividends: %v", err)
	}
	if len(absolute) != len(totalReturn) {
		t.Fatalf("sample count = %d absolute / %d absolute_tr, want equal", len(absolute), len(totalReturn))
	}
	base := labelsByDate(absolute)
	tr := labelsByDate(totalReturn)
	for date, label := range base {
		if tr[date] != label {
			t.Fatalf("absolute_tr label at %s = %v, want %v (no dividend in window)", date, tr[date], label)
		}
	}
}

func TestBuildSamplesAbsoluteTRDividendAtExitIncluded(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	source := datasetSource{series: map[string][]moex.Candle{
		"DIV": flatDividendCandles(80, start),
	}}
	from := start
	till := start.AddDate(0, 0, 200)

	entryDay := start.AddDate(0, 0, 65)
	exitDay := start.AddDate(0, 0, 66)
	samples, err := BuildSamplesWithDividends(context.Background(), source, []string{"DIV"}, from, till, 1, 0, LabelModeAbsoluteTR, 0, features.PriceFeatureConfig{}, HorizonModeBars, []dividends.Record{
		{Ticker: "DIV", LastBuyDate: entryDay, DividendNet: "10"},
	})
	if err != nil {
		t.Fatalf("BuildSamplesWithDividends: %v", err)
	}
	labels := labelsByDate(samples)
	if labels[exitDay] != 1 {
		t.Fatalf("dividend at exit day %s: label = %v, want 1 (exit price adjusted by dividend)", exitDay, labels[exitDay])
	}
	positive := 0
	for date, label := range labels {
		if label == 1 {
			positive++
			if !date.Equal(exitDay) {
				t.Fatalf("unexpected positive label at %s, want only %s", date, exitDay)
			}
		}
	}
	if positive != 1 {
		t.Fatalf("positive labels = %d, want exactly 1", positive)
	}
}

func TestBuildSamplesAbsoluteTRDividendAtEntryExcluded(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	source := datasetSource{series: map[string][]moex.Candle{
		"DIV": flatDividendCandles(80, start),
	}}
	from := start
	till := start.AddDate(0, 0, 200)

	entryDay := start.AddDate(0, 0, 65)
	lastBuy := start.AddDate(0, 0, 64)
	samples, err := BuildSamplesWithDividends(context.Background(), source, []string{"DIV"}, from, till, 1, 0, LabelModeAbsoluteTR, 0, features.PriceFeatureConfig{}, HorizonModeBars, []dividends.Record{
		{Ticker: "DIV", LastBuyDate: lastBuy, DividendNet: "10"},
	})
	if err != nil {
		t.Fatalf("BuildSamplesWithDividends: %v", err)
	}
	labels := labelsByDate(samples)
	if labels[entryDay.AddDate(0, 0, 1)] != 0 {
		t.Fatalf("dividend ex-date on entry day %s was included; want entry-day dividend excluded", entryDay)
	}
	for date, label := range labels {
		if label != 0 {
			t.Fatalf("label at %s = %v, want 0 (ex-date falls on entry day, which is outside (entry, exit])", date, label)
		}
	}
}

func TestBuildSamplesAbsoluteTRRequiresCalendar(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	source := datasetSource{series: map[string][]moex.Candle{
		"DIV": flatDividendCandles(80, start),
	}}
	_, err := BuildSamplesWithDividends(context.Background(), source, []string{"DIV"}, start, start.AddDate(0, 0, 200), 1, 0, LabelModeAbsoluteTR, 0, features.PriceFeatureConfig{}, HorizonModeBars, nil)
	if err == nil {
		t.Fatal("BuildSamplesWithDividends with nil calendar did not return an error")
	}
}
