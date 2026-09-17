package model

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
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

func TestEnsembleTargetLotsFromNotional(t *testing.T) {
	src := &EnsembleSignalSource{MaxLots: 1, TargetNotional: decimal.NewFromInt(100000)}

	cases := []struct {
		name    string
		price   string
		lotSize string
		want    int
	}{
		{name: "expensive-ticker-per-share", price: "1800", want: 56},
		{name: "cheap-ticker-per-share", price: "30", want: 3333},
		{name: "lot-size-rounded", price: "300", lotSize: "10", want: 33},
		{name: "below-one-lot-floor", price: "99000", want: 1},
		{name: "zero-price-fallback-to-maxlots", price: "0", want: 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := domain.FeatureContext{
				Ticker:             "SBER",
				LastPrice:          decimal.RequireFromString(tc.price),
				ReturnPct:          decimal.Zero,
				RealizedVolatility: decimal.Zero,
				NewsSentiment:      decimal.Zero,
				NewsCount:          0,
				OrderBookImbalance: decimal.Zero,
				Mom5d:              decimal.Zero,
				Mom21d:             decimal.Zero,
				Mom63d:             decimal.Zero,
				Reversal1d:         decimal.Zero,
				RSI14:              decimal.Zero,
				DistMA20Pct:        decimal.Zero,
				DistMA50Pct:        decimal.Zero,
				RealizedVol21d:     decimal.Zero,
				VolumeZScore20d:    decimal.Zero,
			}
			if tc.lotSize != "" {
				f.LotSize = decimal.RequireFromString(tc.lotSize)
			}
			if got := src.targetLots(f); got != tc.want {
				t.Fatalf("targetLots = %d, want %d", got, tc.want)
			}
		})
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

func writeDeterministicModel(t *testing.T, bias float64) *EnsembleModel {
	t.Helper()
	order := append([]string(nil), defaultFeatureOrder...)
	zeros := make([]float64, len(order))
	std := make([]float64, len(order))
	for i := range std {
		std[i] = 1
	}
	m := EnsembleModel{
		FeatureOrder:  order,
		BuyThreshold:  0.60,
		SellThreshold: 0.40,
		Logistic: logisticWeights{
			Mean: append([]float64(nil), zeros...),
			Std:  std,
			Coef: append([]float64(nil), zeros...),
			Bias: bias,
		},
	}
	path := filepath.Join(t.TempDir(), "model.json")
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal model: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write model: %v", err)
	}
	loaded, err := LoadEnsembleModel(path)
	if err != nil {
		t.Fatalf("load model: %v", err)
	}
	return loaded
}

func TestEnsembleSignalSourceDeterministic(t *testing.T) {
	feature := domain.FeatureContext{
		Ticker:    "SBER",
		LastPrice: decimal.NewFromInt(100),
		PrevClose: decimal.NewFromInt(100),
	}

	src := &EnsembleSignalSource{Model: writeDeterministicModel(t, 0), MaxLots: 1}
	probability, err := src.RawProbability(feature)
	if err != nil {
		t.Fatalf("RawProbability: %v", err)
	}
	if math.IsNaN(probability) || math.IsInf(probability, 0) || probability < 0.49 || probability > 0.51 {
		t.Fatalf("RawProbability = %f, want finite ~0.5", probability)
	}
	signal, err := src.Generate(context.Background(), feature)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if signal.Action != domain.ActionHold {
		t.Fatalf("neutral probability action = %v, want HOLD", signal.Action)
	}

	buy := &EnsembleSignalSource{Model: writeDeterministicModel(t, 2), MaxLots: 1}
	signal, err = buy.Generate(context.Background(), feature)
	if err != nil {
		t.Fatalf("Generate(buy): %v", err)
	}
	if signal.Action != domain.ActionBuy {
		t.Fatalf("buy-tail action = %v, want BUY", signal.Action)
	}

	sell := &EnsembleSignalSource{Model: writeDeterministicModel(t, -2), MaxLots: 1}
	signal, err = sell.Generate(context.Background(), feature)
	if err != nil {
		t.Fatalf("Generate(sell): %v", err)
	}
	if signal.Action != domain.ActionSell {
		t.Fatalf("sell-tail action = %v, want SELL", signal.Action)
	}
}

func TestEnsembleRawProbabilityRejectsMismatchAndNil(t *testing.T) {
	feature := domain.FeatureContext{Ticker: "SBER"}

	if _, err := (&EnsembleSignalSource{}).RawProbability(feature); err == nil {
		t.Fatal("RawProbability with nil model: error = nil, want error")
	}
	if _, err := (&EnsembleSignalSource{Model: writeDeterministicModel(t, 0)}).RawProbability(domain.FeatureContext{}); err == nil {
		t.Fatal("RawProbability with empty ticker: error = nil, want error")
	}

	mismatched := writeDeterministicModel(t, 0)
	mismatched.FeatureOrder[0] = "wrong_feature_name"
	if _, err := (&EnsembleSignalSource{Model: mismatched}).RawProbability(feature); err == nil {
		t.Fatal("RawProbability with mismatched feature order: error = nil, want error")
	}
}
