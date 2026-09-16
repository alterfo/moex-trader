package model

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

func TestEnsembleMatchesPython(t *testing.T) {
	modelPath := os.Getenv("ENSEMBLE_MODEL")
	if modelPath == "" {
		t.Skip("ENSEMBLE_MODEL not set")
	}
	refPath := os.Getenv("ENSEMBLE_REF")
	if refPath == "" {
		t.Skip("ENSEMBLE_REF not set")
	}

	m, err := LoadEnsembleModel(modelPath)
	if err != nil {
		t.Fatalf("load ensemble model: %v", err)
	}

	raw, err := os.ReadFile(refPath)
	if err != nil {
		t.Fatalf("read ref: %v", err)
	}
	var ref struct {
		Rows [][]float64 `json:"rows"`
		Want []float64   `json:"want"`
	}
	if err := json.Unmarshal(raw, &ref); err != nil {
		t.Fatalf("parse ref: %v", err)
	}

	wantDim := len(m.FeatureOrder)
	var maxErr float64
	var worstRow int
	for i, row := range ref.Rows {
		if len(row) != wantDim {
			t.Fatalf("row %d has %d features, want %d", i, len(row), wantDim)
		}
		got := m.Probability(row)
		diff := math.Abs(got - ref.Want[i])
		if diff > maxErr {
			maxErr = diff
			worstRow = i
		}
	}

	t.Logf("rows=%d max_abs_err=%.3e (row %d)", len(ref.Rows), maxErr, worstRow)
	if maxErr > 1e-4 {
		t.Fatalf("probability mismatch vs python: max_abs_err=%.3e", maxErr)
	}
}

func TestEnsembleSignalSourceGenerate(t *testing.T) {
	modelPath := os.Getenv("ENSEMBLE_MODEL")
	if modelPath == "" {
		t.Skip("ENSEMBLE_MODEL not set")
	}
	m, err := LoadEnsembleModel(modelPath)
	if err != nil {
		t.Fatalf("load ensemble model: %v", err)
	}
	src := &EnsembleSignalSource{Model: m, MaxLots: 2}

	feature := domain.FeatureContext{
		Ticker:             "SBER",
		ReturnPct:          decimal.NewFromFloat(-0.5),
		RealizedVolatility: decimal.NewFromFloat(2.1),
		NewsSentiment:      decimal.Zero,
		NewsCount:          0,
		OrderBookImbalance: decimal.Zero,
		Mom5d:              decimal.NewFromFloat(-1.2),
		Mom21d:             decimal.NewFromFloat(3.1),
		Mom63d:             decimal.NewFromFloat(5.7),
		Reversal1d:         decimal.NewFromFloat(-0.5),
		RSI14:              decimal.NewFromFloat(48.3),
		DistMA20Pct:        decimal.NewFromFloat(0.2),
		DistMA50Pct:        decimal.NewFromFloat(1.1),
		RealizedVol21d:     decimal.NewFromFloat(24.0),
		VolumeZScore20d:    decimal.NewFromFloat(-0.3),
	}

	signal, err := src.Generate(context.Background(), feature)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if signal.Ticker != "SBER" {
		t.Fatalf("signal ticker = %q, want SBER", signal.Ticker)
	}
	if signal.Action != domain.ActionHold && signal.Action != domain.ActionBuy && signal.Action != domain.ActionSell {
		t.Fatalf("unexpected action %v", signal.Action)
	}
	if signal.Confidence.IsNegative() {
		t.Fatalf("negative confidence %s", signal.Confidence)
	}
}
