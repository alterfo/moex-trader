package model

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

func TestPearsonCorrelationPerfectPositive(t *testing.T) {
	xs := []float64{1, 2, 3, 4, 5}
	ys := []float64{2, 4, 6, 8, 10}
	corr, ok := pearsonCorrelation(xs, ys)
	if !ok {
		t.Fatal("pearsonCorrelation() ok = false, want true")
	}
	if math.Abs(corr-1.0) > 1e-9 {
		t.Fatalf("pearsonCorrelation() = %v, want 1.0", corr)
	}
}

func TestPearsonCorrelationPerfectNegative(t *testing.T) {
	xs := []float64{1, 2, 3, 4, 5}
	ys := []float64{10, 8, 6, 4, 2}
	corr, ok := pearsonCorrelation(xs, ys)
	if !ok {
		t.Fatal("pearsonCorrelation() ok = false, want true")
	}
	if math.Abs(corr-(-1.0)) > 1e-9 {
		t.Fatalf("pearsonCorrelation() = %v, want -1.0", corr)
	}
}

func TestPearsonCorrelationConstantInput(t *testing.T) {
	xs := []float64{5, 5, 5, 5}
	ys := []float64{1, 2, 3, 4}
	if _, ok := pearsonCorrelation(xs, ys); ok {
		t.Fatal("pearsonCorrelation() ok = true for zero-variance input, want false")
	}
}

func TestPearsonCorrelationTooFewPoints(t *testing.T) {
	if _, ok := pearsonCorrelation([]float64{1, 2}, []float64{1, 2}); ok {
		t.Fatal("pearsonCorrelation() ok = true for n<3, want false")
	}
}

func TestHitRate(t *testing.T) {
	xs := []float64{1, -1, 2, -2}
	ys := []float64{1, -1, -1, 1}
	if got := hitRate(xs, ys); got != 0.5 {
		t.Fatalf("hitRate() = %v, want 0.5", got)
	}
}

func TestCalibrateDetectsInformativeFeature(t *testing.T) {
	samples := make([]CalibrationSample, 0, 60)
	for i := range 60 {
		informative := float64(i%20) - 10
		noise := 3.0
		if i%2 == 0 {
			noise = -3.0
		}
		samples = append(samples, CalibrationSample{
			Ticker:       "TEST",
			Date:         time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i),
			Vector:       []float64{informative, noise},
			Names:        []string{"informative", "noise"},
			ExcessReturn: map[int]float64{5: informative * 2},
		})
	}

	pooled := Calibrate(samples, []int{5})
	informativeStats := pooled["informative"][5]
	if informativeStats.Correlation < 0.9 {
		t.Fatalf("informative feature correlation = %v, want > 0.9", informativeStats.Correlation)
	}
	noiseStats := pooled["noise"][5]
	if math.Abs(noiseStats.Correlation) > 0.5 {
		t.Fatalf("noise feature correlation = %v, want close to 0", noiseStats.Correlation)
	}
}

func TestBuildCalibrationSamplesAgainstBenchmark(t *testing.T) {
	up := trendCandles(90, true)
	source := datasetSource{series: map[string][]moex.Candle{
		"UP":    up,
		"IMOEX": flatIndexCandles(90),
	}}

	samples, err := BuildCalibrationSamples(
		context.Background(),
		source,
		[]string{"UP"},
		time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		[]int{1, 3, 5},
		3,
	)
	if err != nil {
		t.Fatalf("BuildCalibrationSamples failed: %v", err)
	}
	if len(samples) == 0 {
		t.Fatal("got 0 calibration samples")
	}
	for _, s := range samples {
		for h, excess := range s.ExcessReturn {
			if excess <= 0 {
				t.Fatalf("horizon %d: excess return = %v, want positive for an uptrending ticker vs flat benchmark", h, excess)
			}
		}
		if len(s.Vector) != len(s.Names) {
			t.Fatalf("vector/names length mismatch: %d vs %d", len(s.Vector), len(s.Names))
		}
	}
}

func TestBuildCalibrationSamplesMissingBenchmarkFails(t *testing.T) {
	source := datasetSource{series: map[string][]moex.Candle{"UP": trendCandles(90, true)}}
	_, err := BuildCalibrationSamples(
		context.Background(),
		source,
		[]string{"UP"},
		time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		[]int{1, 3, 5},
		3,
	)
	if err == nil {
		t.Fatal("missing IMOEX benchmark history did not return an error")
	}
}

func TestBuildCalibrationReportContainsRankedFeatures(t *testing.T) {
	up := trendCandles(90, true)
	source := datasetSource{series: map[string][]moex.Candle{
		"UP":    up,
		"IMOEX": flatIndexCandles(90),
	}}

	samples, err := BuildCalibrationSamples(
		context.Background(),
		source,
		[]string{"UP"},
		time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		[]int{1, 3, 5},
		3,
	)
	if err != nil {
		t.Fatalf("BuildCalibrationSamples failed: %v", err)
	}

	report := BuildCalibrationReport(samples, []string{"UP"}, []int{1, 3, 5})
	if !strings.Contains(report, "# Калибровка фич") {
		t.Fatalf("report missing title: %q", report)
	}
	if !strings.Contains(report, "mom_5d") {
		t.Fatalf("report missing feature row: %q", report)
	}
	if !strings.Contains(report, "| UP |") {
		t.Fatalf("report missing per-ticker row: %q", report)
	}
}
