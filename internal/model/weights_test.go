package model

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func testWeights() *Weights {
	return &Weights{
		FeatureOrder:  []string{"return_pct", "rsi_14"},
		Mean:          []float64{0.01, 50},
		Std:           []float64{1.0, 10},
		Coef:          []float64{0.5, -0.25},
		Bias:          -0.1,
		BuyThreshold:  0.55,
		SellThreshold: 0.45,
		HorizonDays:   5,
		DeadbandPct:   0.5,
		TrainedAt:     time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC),
		Training: TrainingMetadata{
			TrainFrom:         time.Date(2024, 9, 15, 0, 0, 0, 0, time.UTC),
			TrainTill:         time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC),
			ValFrom:           time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC),
			ValTill:           time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC),
			Tickers:           []string{"SBER", "GAZP"},
			TrainSamples:      400,
			ValSamples:        80,
			TrainAccuracy:     0.72,
			ValAccuracy:       0.68,
			ValSharpe:         0.9,
			ValHitRate:        0.55,
			ValMaxDrawdownPct: 12.3,
		},
	}
}

func assertWeightsEqual(t *testing.T, want, got *Weights) {
	t.Helper()
	if !reflect.DeepEqual(want.FeatureOrder, got.FeatureOrder) {
		t.Fatalf("FeatureOrder = %v, want %v", got.FeatureOrder, want.FeatureOrder)
	}
	if !reflect.DeepEqual(want.Mean, got.Mean) {
		t.Fatalf("Mean = %v, want %v", got.Mean, want.Mean)
	}
	if !reflect.DeepEqual(want.Std, got.Std) {
		t.Fatalf("Std = %v, want %v", got.Std, want.Std)
	}
	if !reflect.DeepEqual(want.Coef, got.Coef) {
		t.Fatalf("Coef = %v, want %v", got.Coef, want.Coef)
	}
	if got.Bias != want.Bias {
		t.Fatalf("Bias = %v, want %v", got.Bias, want.Bias)
	}
	if got.BuyThreshold != want.BuyThreshold || got.SellThreshold != want.SellThreshold {
		t.Fatalf("thresholds = (%v, %v), want (%v, %v)", got.BuyThreshold, got.SellThreshold, want.BuyThreshold, want.SellThreshold)
	}
	if got.HorizonDays != want.HorizonDays || got.DeadbandPct != want.DeadbandPct {
		t.Fatalf("horizon/deadband = (%d, %v), want (%d, %v)", got.HorizonDays, got.DeadbandPct, want.HorizonDays, want.DeadbandPct)
	}
	if !got.TrainedAt.Equal(want.TrainedAt) {
		t.Fatalf("TrainedAt = %v, want %v", got.TrainedAt, want.TrainedAt)
	}
	if !got.Training.TrainFrom.Equal(want.Training.TrainFrom) || !got.Training.TrainTill.Equal(want.Training.TrainTill) {
		t.Fatalf("training date range = %v..%v, want %v..%v", got.Training.TrainFrom, got.Training.TrainTill, want.Training.TrainFrom, want.Training.TrainTill)
	}
	if !got.Training.ValFrom.Equal(want.Training.ValFrom) || !got.Training.ValTill.Equal(want.Training.ValTill) {
		t.Fatalf("validation date range = %v..%v, want %v..%v", got.Training.ValFrom, got.Training.ValTill, want.Training.ValFrom, want.Training.ValTill)
	}
	if !reflect.DeepEqual(got.Training.Tickers, want.Training.Tickers) {
		t.Fatalf("Training.Tickers = %v, want %v", got.Training.Tickers, want.Training.Tickers)
	}
	if got.Training.TrainSamples != want.Training.TrainSamples || got.Training.ValSamples != want.Training.ValSamples {
		t.Fatalf("sample counts = (%d, %d), want (%d, %d)", got.Training.TrainSamples, got.Training.ValSamples, want.Training.TrainSamples, want.Training.ValSamples)
	}
	if got.Training.TrainAccuracy != want.Training.TrainAccuracy || got.Training.ValAccuracy != want.Training.ValAccuracy {
		t.Fatalf("accuracies = (%v, %v), want (%v, %v)", got.Training.TrainAccuracy, got.Training.ValAccuracy, want.Training.TrainAccuracy, want.Training.ValAccuracy)
	}
	if got.Training.ValSharpe != want.Training.ValSharpe || got.Training.ValHitRate != want.Training.ValHitRate || got.Training.ValMaxDrawdownPct != want.Training.ValMaxDrawdownPct {
		t.Fatalf("validation metrics = (%v, %v, %v), want (%v, %v, %v)", got.Training.ValSharpe, got.Training.ValHitRate, got.Training.ValMaxDrawdownPct, want.Training.ValSharpe, want.Training.ValHitRate, want.Training.ValMaxDrawdownPct)
	}
}

func TestWeightsRoundTrip(t *testing.T) {
	want := testWeights()
	path := filepath.Join(t.TempDir(), "nested", "model.json")
	if err := want.Save(path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	got, err := LoadWeights(path)
	if err != nil {
		t.Fatalf("LoadWeights failed: %v", err)
	}
	assertWeightsEqual(t, want, got)
}

func TestLoadWeightsMalformedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if _, err := LoadWeights(path); err == nil {
		t.Fatal("LoadWeights did not return an error for malformed JSON")
	}
}

func TestLoadWeightsMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")
	if _, err := LoadWeights(path); err == nil {
		t.Fatal("LoadWeights did not return an error for a missing file")
	}
}

func TestLoadWeightsDimensionMismatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model.json")
	data := `{
		"feature_order": ["return_pct"],
		"mean": [0, 0],
		"std": [1, 1],
		"coef": [0.5, -0.25],
		"bias": 0,
		"buy_threshold": 0.55,
		"sell_threshold": 0.45,
		"horizon_days": 5,
		"deadband_pct": 0.5,
		"trained_at": "2026-09-15T12:00:00Z"
	}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if _, err := LoadWeights(path); err == nil {
		t.Fatal("LoadWeights did not return an error for dimension mismatch")
	}
}

func TestWeightsValidationThresholds(t *testing.T) {
	base := testWeights()
	cases := []struct {
		name   string
		mutate func(*Weights)
	}{
		{
			name: "buy below sell",
			mutate: func(w *Weights) {
				w.BuyThreshold = 0.4
				w.SellThreshold = 0.45
			},
		},
		{
			name: "buy outside range",
			mutate: func(w *Weights) {
				w.BuyThreshold = 1.1
			},
		},
		{
			name: "sell outside range",
			mutate: func(w *Weights) {
				w.SellThreshold = -0.1
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := base
			tc.mutate(w)
			if err := w.Save(filepath.Join(t.TempDir(), "model.json")); err == nil {
				t.Fatal("Save did not return an error for invalid thresholds")
			}
		})
	}
}
