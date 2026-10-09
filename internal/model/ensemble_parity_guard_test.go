package model

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

func liveLikeFeature() domain.FeatureContext {
	return domain.FeatureContext{
		Ticker:             "SBER",
		GeneratedAt:        time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC),
		LastPrice:          decimal.NewFromInt(300),
		RealizedVolatility: decimal.NewFromFloat(1.9),
		RSI14:              decimal.NewFromInt(50),
		RealizedVol21d:     decimal.NewFromInt(25),
		StochK14:           decimal.NewFromInt(50),
		WilliamsR14:        decimal.NewFromInt(-50),
	}
}

func TestEnsembleIgnoresUntrainedNewsFeatures(t *testing.T) {
	model, err := LoadEnsembleModel("../../ensemble_model.json")
	if err != nil {
		t.Fatal(err)
	}
	source := &EnsembleSignalSource{Model: model, MaxLots: 1}

	quiet := liveLikeFeature()
	noisy := liveLikeFeature()
	noisy.NewsCount = 109
	noisy.NewsSentiment = decimal.NewFromFloat(-0.06)

	pq, err := source.RawProbability(quiet)
	if err != nil {
		t.Fatal(err)
	}
	pn, err := source.RawProbability(noisy)
	if err != nil {
		t.Fatal(err)
	}
	if pq != pn {
		t.Fatalf("news features must not move the ensemble probability: quiet=%v noisy=%v", pq, pn)
	}
	if pn < 0.2 {
		t.Fatalf("probability collapsed to %v with live-like news volume", pn)
	}
}

func TestEnsembleHoldsOnOutOfRangeFeature(t *testing.T) {
	model, err := LoadEnsembleModel("../../ensemble_model.json")
	if err != nil {
		t.Fatal(err)
	}
	source := &EnsembleSignalSource{Model: model, MaxLots: 1}

	feature := liveLikeFeature()
	feature.Mom21d = decimal.NewFromInt(500)

	signal, err := source.Generate(context.Background(), feature)
	if err != nil {
		t.Fatal(err)
	}
	if signal.Action != domain.ActionHold || signal.HoldReason != domain.HoldReasonOutOfRange {
		t.Fatalf("want HOLD/out_of_range, got %s/%s (%s)", signal.Action, signal.HoldReason, signal.Reasoning)
	}
	if signal.TargetLots != 0 {
		t.Fatalf("held signal must carry no lots, got %d", signal.TargetLots)
	}
}

func TestEnsembleDoesNotHoldInRange(t *testing.T) {
	model, err := LoadEnsembleModel("../../ensemble_model.json")
	if err != nil {
		t.Fatal(err)
	}
	source := &EnsembleSignalSource{Model: model, MaxLots: 1}

	signal, err := source.Generate(context.Background(), liveLikeFeature())
	if err != nil {
		t.Fatal(err)
	}
	if signal.HoldReason == domain.HoldReasonOutOfRange {
		t.Fatalf("in-range feature held as out_of_range: %s", signal.Reasoning)
	}
}
