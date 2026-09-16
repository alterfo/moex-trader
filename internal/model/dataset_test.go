package model

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

type datasetSource struct {
	series map[string][]moex.Candle
}

func (s datasetSource) History(_ context.Context, ticker string, _, _ time.Time) ([]moex.Candle, error) {
	return s.series[ticker], nil
}

func flatIndexCandles(n int) []moex.Candle {
	candles := make([]moex.Candle, n)
	for i := range candles {
		candles[i] = moex.Candle{
			Open:   decimal.NewFromInt(3000),
			Close:  decimal.NewFromInt(3000),
			High:   decimal.NewFromInt(3010),
			Low:    decimal.NewFromInt(2990),
			Volume: decimal.NewFromInt(1000000),
			Begin:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i),
			End:    time.Date(2024, 1, 1, 18, 0, 0, 0, time.UTC).AddDate(0, 0, i),
		}
	}
	return candles
}

func trendCandles(n int, up bool) []moex.Candle {
	candles := make([]moex.Candle, n)
	for i := range candles {
		if up {
			candles[i] = moex.Candle{
				Open:   decimal.NewFromInt(int64(100 + i)),
				Close:  decimal.NewFromFloat(float64(100+i) + 0.5),
				High:   decimal.NewFromFloat(float64(100+i) + 1),
				Low:    decimal.NewFromFloat(float64(100+i) - 1),
				Volume: decimal.NewFromInt(1000),
				Begin:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i),
				End:    time.Date(2024, 1, 1, 18, 0, 0, 0, time.UTC).AddDate(0, 0, i),
			}
			continue
		}
		candles[i] = moex.Candle{
			Open:   decimal.NewFromInt(int64(200 - i)),
			Close:  decimal.NewFromInt(int64(199 - i)),
			High:   decimal.NewFromInt(int64(201 - i)),
			Low:    decimal.NewFromInt(int64(198 - i)),
			Volume: decimal.NewFromInt(1000),
			Begin:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i),
			End:    time.Date(2024, 1, 1, 18, 0, 0, 0, time.UTC).AddDate(0, 0, i),
		}
	}
	return candles
}

func TestBuildSamplesUpAndDownLabels(t *testing.T) {
	source := datasetSource{series: map[string][]moex.Candle{
		"UP":    trendCandles(80, true),
		"DOWN":  trendCandles(80, false),
		"IMOEX": flatIndexCandles(80),
	}}
	samples, err := BuildSamples(
		context.Background(),
		source,
		[]string{"UP", "DOWN"},
		time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		5,
		0.5,
		LabelModeExcess,
	)
	if err != nil {
		t.Fatalf("BuildSamples failed: %v", err)
	}
	if len(samples) != 20 {
		t.Fatalf("got %d samples, want 20", len(samples))
	}
	upCount, downCount := 0, 0
	for _, sample := range samples {
		switch sample.Feature.Ticker {
		case "UP":
			upCount++
			if sample.Label != 1 {
				t.Fatalf("UP sample label = %v, want 1", sample.Label)
			}
		case "DOWN":
			downCount++
			if sample.Label != 0 {
				t.Fatalf("DOWN sample label = %v, want 0", sample.Label)
			}
		default:
			t.Fatalf("unexpected ticker %q", sample.Feature.Ticker)
		}
	}
	if upCount != 10 || downCount != 10 {
		t.Fatalf("sample split = %d up / %d down, want 10/10", upCount, downCount)
	}
}

func TestBuildSamplesSkipsInsufficientFutureData(t *testing.T) {
	source := datasetSource{series: map[string][]moex.Candle{
		"UP":    trendCandles(70, true),
		"IMOEX": flatIndexCandles(70),
	}}
	samples, err := BuildSamples(
		context.Background(),
		source,
		[]string{"UP"},
		time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		10,
		0.5,
		LabelModeExcess,
	)
	if err != nil {
		t.Fatalf("BuildSamples failed: %v", err)
	}
	if len(samples) != 0 {
		t.Fatalf("got %d samples, want 0 when horizon exceeds available future data", len(samples))
	}
}

func TestBuildSamplesDeadbandExclusion(t *testing.T) {
	flat := trendCandles(80, true)
	for i := range flat {
		flat[i].Open = decimal.NewFromInt(100)
		flat[i].Close = decimal.NewFromInt(100)
		flat[i].High = decimal.NewFromInt(101)
		flat[i].Low = decimal.NewFromInt(99)
	}
	source := datasetSource{series: map[string][]moex.Candle{
		"FLAT":  flat,
		"IMOEX": flatIndexCandles(80),
	}}

	excluded, err := BuildSamples(
		context.Background(),
		source,
		[]string{"FLAT"},
		time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		5,
		0.5,
		LabelModeExcess,
	)
	if err != nil {
		t.Fatalf("BuildSamples failed: %v", err)
	}
	if len(excluded) != 0 {
		t.Fatalf("deadband 0.5 produced %d samples, want 0", len(excluded))
	}

	included, err := BuildSamples(
		context.Background(),
		source,
		[]string{"FLAT"},
		time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		5,
		0,
		LabelModeExcess,
	)
	if err != nil {
		t.Fatalf("BuildSamples failed: %v", err)
	}
	if len(included) == 0 {
		t.Fatal("deadband 0 excluded all zero-return samples, want them included")
	}
	for _, sample := range included {
		if sample.Label != 0 {
			t.Fatalf("zero-return sample label = %v, want 0", sample.Label)
		}
	}
}

func TestBuildSamplesLabelIsExcessOverBenchmark(t *testing.T) {
	flat := trendCandles(80, true)
	for i := range flat {
		flat[i].Open = decimal.NewFromInt(100)
		flat[i].Close = decimal.NewFromInt(100)
		flat[i].High = decimal.NewFromInt(101)
		flat[i].Low = decimal.NewFromInt(99)
	}
	decliningIndex := trendCandles(80, false)

	source := datasetSource{series: map[string][]moex.Candle{
		"FLAT":  flat,
		"IMOEX": decliningIndex,
	}}

	samples, err := BuildSamples(
		context.Background(),
		source,
		[]string{"FLAT"},
		time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		5,
		0,
		LabelModeExcess,
	)
	if err != nil {
		t.Fatalf("BuildSamples failed: %v", err)
	}
	if len(samples) == 0 {
		t.Fatal("got 0 samples, want samples showing a flat ticker beating a declining benchmark")
	}
	for _, sample := range samples {
		if sample.Label != 1 {
			t.Fatalf("flat ticker vs declining IMOEX: label = %v, want 1 (a flat return has a positive excess return over a falling benchmark)", sample.Label)
		}
	}
}

func TestBuildSamplesAbsoluteModeDoesNotRequireBenchmark(t *testing.T) {
	source := datasetSource{series: map[string][]moex.Candle{
		"UP": trendCandles(80, true),
	}}

	samples, err := BuildSamples(
		context.Background(),
		source,
		[]string{"UP"},
		time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		5,
		0.5,
		LabelModeAbsolute,
	)
	if err != nil {
		t.Fatalf("BuildSamples failed: %v", err)
	}
	if len(samples) == 0 {
		t.Fatal("absolute mode without benchmark history returned no samples")
	}
	for _, sample := range samples {
		if sample.Label != 1 {
			t.Fatalf("rising ticker label = %v, want 1 in absolute mode", sample.Label)
		}
	}
}

func TestBuildSamplesAbsoluteLabelDiffersFromExcess(t *testing.T) {
	flat := trendCandles(80, true)
	for i := range flat {
		flat[i].Open = decimal.NewFromInt(100)
		flat[i].Close = decimal.NewFromInt(100)
		flat[i].High = decimal.NewFromInt(101)
		flat[i].Low = decimal.NewFromInt(99)
	}

	source := datasetSource{series: map[string][]moex.Candle{
		"FLAT":  flat,
		"IMOEX": trendCandles(80, false),
	}}

	absolute, err := BuildSamples(
		context.Background(),
		source,
		[]string{"FLAT"},
		time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		5,
		0,
		LabelModeAbsolute,
	)
	if err != nil {
		t.Fatalf("BuildSamples failed: %v", err)
	}
	if len(absolute) == 0 {
		t.Fatal("absolute mode returned no samples")
	}
	for _, sample := range absolute {
		if sample.Label != 0 {
			t.Fatalf("flat ticker absolute label = %v, want 0 while excess mode labels the same row 1", sample.Label)
		}
	}
}

func TestBuildSamplesMissingBenchmarkFails(t *testing.T) {
	source := datasetSource{series: map[string][]moex.Candle{"UP": trendCandles(80, true)}}
	_, err := BuildSamples(
		context.Background(),
		source,
		[]string{"UP"},
		time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		5,
		0.5,
		LabelModeExcess,
	)
	if err == nil {
		t.Fatal("missing IMOEX benchmark history did not return an error")
	}
}

func TestBuildSamplesValidation(t *testing.T) {
	source := datasetSource{series: map[string][]moex.Candle{
		"UP":    trendCandles(80, true),
		"IMOEX": flatIndexCandles(80),
	}}
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	till := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	if _, err := BuildSamples(context.Background(), nil, []string{"UP"}, from, till, 5, 0.5, LabelModeExcess); err == nil {
		t.Fatal("nil source did not return an error")
	}
	if _, err := BuildSamples(context.Background(), source, nil, from, till, 5, 0.5, LabelModeExcess); err == nil {
		t.Fatal("empty tickers did not return an error")
	}
	if _, err := BuildSamples(context.Background(), source, []string{"UP"}, from, till, 0, 0.5, LabelModeExcess); err == nil {
		t.Fatal("non-positive horizon did not return an error")
	}
	if _, err := BuildSamples(context.Background(), source, []string{"UP"}, from, till, 5, -1, LabelModeExcess); err == nil {
		t.Fatal("negative deadband did not return an error")
	}
	if _, err := BuildSamples(context.Background(), source, []string{"UP"}, from, till, 5, 0.5, LabelMode("bogus")); err == nil {
		t.Fatal("unknown label mode did not return an error")
	}
}

func TestToVector(t *testing.T) {
	feature := domain.FeatureContext{
		ReturnPct:          decimal.NewFromFloat(1.25),
		RealizedVolatility: decimal.NewFromFloat(2.5),
		NewsSentiment:      decimal.NewFromFloat(0.75),
		NewsCount:          7,
		OrderBookImbalance: decimal.NewFromFloat(-0.4),
		Mom5d:              decimal.NewFromFloat(5),
		Mom21d:             decimal.NewFromFloat(6),
		Mom63d:             decimal.NewFromFloat(7),
		Reversal1d:         decimal.NewFromFloat(8),
		RSI14:              decimal.NewFromFloat(9),
		DistMA20Pct:        decimal.NewFromFloat(10),
		DistMA50Pct:        decimal.NewFromFloat(11),
		RealizedVol21d:     decimal.NewFromFloat(12),
		VolumeZScore20d:    decimal.NewFromFloat(13),
		MACDHistPct:        decimal.NewFromFloat(14),
		StochK14:           decimal.NewFromFloat(15),
		WilliamsR14:        decimal.NewFromFloat(16),
		AlligatorSpreadPct: decimal.NewFromFloat(17),
		EventDividend:      1,
		EventBuyback:       2,
		EventSanctions:     3,
		EventIPO:           4,
		EventReport:        5,
		EventDelisting:     6,
		EventMNA:           7,
		EventDefault:       8,
	}

	values, names := ToVector(feature)
	if len(values) != 26 || len(names) != 26 {
		t.Fatalf("ToVector returned %d values and %d names, want 26/26", len(values), len(names))
	}
	if !equalStrings(names, defaultFeatureOrder) {
		t.Fatalf("names = %v, want %v", names, defaultFeatureOrder)
	}
	want := []float64{1.25, 2.5, 0.75, 7, -0.4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 1, 2, 3, 4, 5, 6, 7, 8}
	for i := range want {
		if values[i] != want[i] {
			t.Fatalf("values[%d] = %v, want %v", i, values[i], want[i])
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
