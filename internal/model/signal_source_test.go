package model

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

func inferenceWeights(coef0, bias float64) *Weights {
	_, names := ToVector(domain.FeatureContext{})
	weights := &Weights{
		FeatureOrder:  append([]string(nil), names...),
		Mean:          make([]float64, len(names)),
		Std:           make([]float64, len(names)),
		Coef:          make([]float64, len(names)),
		Bias:          bias,
		BuyThreshold:  0.55,
		SellThreshold: 0.45,
	}
	for i := range weights.Std {
		weights.Std[i] = 1
	}
	weights.Mean[0] = 10
	weights.Std[0] = 5
	weights.Coef[0] = coef0
	return weights
}

func TestSignalSourceGenerate(t *testing.T) {
	generatedAt := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		feature    domain.FeatureContext
		weights    *Weights
		maxLots    int
		wantAction domain.Action
		wantLots   int
		wantHold   string
		wantP      float64
	}{
		{
			name: "buy above threshold",
			feature: domain.FeatureContext{
				Ticker:      "SBER",
				ReturnPct:   decimal.NewFromFloat(20),
				GeneratedAt: generatedAt,
			},
			weights:    inferenceWeights(1, 0),
			maxLots:    3,
			wantAction: domain.ActionBuy,
			wantLots:   3,
			wantP:      sigmoid(2),
		},
		{
			name: "sell below threshold",
			feature: domain.FeatureContext{
				Ticker:      "GAZP",
				ReturnPct:   decimal.NewFromFloat(20),
				GeneratedAt: generatedAt,
			},
			weights:    inferenceWeights(-1, 0),
			maxLots:    5,
			wantAction: domain.ActionSell,
			wantLots:   5,
			wantP:      sigmoid(-2),
		},
		{
			name: "hold in dead zone",
			feature: domain.FeatureContext{
				Ticker:      "LKOH",
				GeneratedAt: generatedAt,
			},
			weights:    inferenceWeights(0, 0),
			maxLots:    7,
			wantAction: domain.ActionHold,
			wantLots:   0,
			wantHold:   domain.HoldReasonModel,
			wantP:      0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := &SignalSource{Weights: tt.weights, MaxLots: tt.maxLots}
			got, err := source.Generate(context.Background(), tt.feature)
			if err != nil {
				t.Fatalf("Generate failed: %v", err)
			}
			if got.Action != tt.wantAction {
				t.Fatalf("Action = %q, want %q", got.Action, tt.wantAction)
			}
			if got.TargetLots != tt.wantLots {
				t.Fatalf("TargetLots = %d, want %d", got.TargetLots, tt.wantLots)
			}
			if got.HoldReason != tt.wantHold {
				t.Fatalf("HoldReason = %q, want %q", got.HoldReason, tt.wantHold)
			}
			wantConfidence := decimal.NewFromFloat(math.Abs(tt.wantP-0.5) * 2)
			if !got.Confidence.Equal(wantConfidence) {
				t.Fatalf("Confidence = %s, want %s", got.Confidence, wantConfidence)
			}
			if !strings.Contains(got.Reasoning, "model:p=") {
				t.Fatalf("Reasoning = %q, want probability prefix", got.Reasoning)
			}
			if !strings.Contains(got.Reasoning, "top_feature=return_pct:") {
				t.Fatalf("Reasoning = %q, want top feature", got.Reasoning)
			}
			if !got.GeneratedAt.Equal(generatedAt) {
				t.Fatalf("GeneratedAt = %v, want %v", got.GeneratedAt, generatedAt)
			}
		})
	}
}

func TestSignalSourceRawProbability(t *testing.T) {
	source := &SignalSource{Weights: inferenceWeights(1, 0), MaxLots: 3}
	feature := domain.FeatureContext{
		Ticker:      "SBER",
		ReturnPct:   decimal.NewFromFloat(20),
		GeneratedAt: time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC),
	}
	want := sigmoid(2)
	got, err := source.RawProbability(feature)
	if err != nil {
		t.Fatalf("RawProbability failed: %v", err)
	}
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("RawProbability = %v, want %v", got, want)
	}
}

func TestSignalSourceGenerateErrorCases(t *testing.T) {
	source := &SignalSource{Weights: inferenceWeights(0, 0), MaxLots: 1}

	if _, err := source.Generate(context.Background(), domain.FeatureContext{Ticker: ""}); err == nil {
		t.Fatal("empty ticker did not return an error")
	}
	if _, err := (&SignalSource{}).Generate(context.Background(), domain.FeatureContext{Ticker: "SBER"}); err == nil {
		t.Fatal("nil weights did not return an error")
	}

	nanWeights := inferenceWeights(0, math.NaN())
	if _, err := (&SignalSource{Weights: nanWeights, MaxLots: 1}).Generate(context.Background(), domain.FeatureContext{Ticker: "SBER"}); err == nil {
		t.Fatal("NaN probability did not return an error")
	}
}

func TestSignalSourceGenerateFeatureOrderMismatch(t *testing.T) {
	reordered := inferenceWeights(0, 0)
	reordered.FeatureOrder[0], reordered.FeatureOrder[1] = reordered.FeatureOrder[1], reordered.FeatureOrder[0]

	_, err := (&SignalSource{Weights: reordered, MaxLots: 1}).Generate(context.Background(), domain.FeatureContext{Ticker: "SBER"})
	if err == nil {
		t.Fatal("reordered feature_order did not return an error")
	}
	if !strings.Contains(err.Error(), "feature order mismatch") {
		t.Fatalf("Generate() error = %v, want feature order mismatch", err)
	}
}
