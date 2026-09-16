package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/model"
)

func writeValidModel(t *testing.T) string {
	t.Helper()

	_, names := model.ToVector(domain.FeatureContext{})
	weights := &model.Weights{
		FeatureOrder:  names,
		Mean:          make([]float64, len(names)),
		Std:           make([]float64, len(names)),
		Coef:          make([]float64, len(names)),
		Bias:          0,
		BuyThreshold:  0.55,
		SellThreshold: 0.45,
		HorizonDays:   5,
		DeadbandPct:   0.5,
		TrainedAt:     time.Now(),
	}
	for i := range weights.Std {
		weights.Std[i] = 1
	}
	weights.Coef[0] = 10

	path := filepath.Join(t.TempDir(), "model.json")
	if err := weights.Save(path); err != nil {
		t.Fatalf("save model fixture: %v", err)
	}
	return path
}

func TestBuildSignalSourceModel(t *testing.T) {
	source, save, err := buildSignalSource(nil, signalSourceOptions{
		Mode:      signalSourceModel,
		ModelPath: writeValidModel(t),
		MaxLots:   2,
	})
	if err != nil {
		t.Fatalf("buildSignalSource model: %v", err)
	}
	if save != nil {
		t.Fatalf("save callback = %T, want nil", save)
	}

	modelSource, ok := source.(*model.SignalSource)
	if !ok {
		t.Fatalf("source type = %T, want *model.SignalSource", source)
	}

	signal, err := modelSource.Generate(context.Background(), domain.FeatureContext{
		Ticker:      "SBER",
		ReturnPct:   decimal.NewFromInt(1),
		GeneratedAt: time.Unix(0, 0),
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if signal.Action != domain.ActionBuy {
		t.Fatalf("Action = %q, want %q", signal.Action, domain.ActionBuy)
	}
	if signal.TargetLots != 2 {
		t.Fatalf("TargetLots = %d, want 2", signal.TargetLots)
	}
}

func TestBuildSignalSourceModelWithCache(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "cache.json")
	source, save, err := buildSignalSource(nil, signalSourceOptions{
		Mode:      signalSourceModel,
		ModelPath: writeValidModel(t),
		MaxLots:   1,
		CachePath: cachePath,
	})
	if err != nil {
		t.Fatalf("buildSignalSource model cache: %v", err)
	}
	if save == nil {
		t.Fatal("save callback = nil, want non-nil")
	}
	if _, ok := source.(*backtest.CachedSignalSource); !ok {
		t.Fatalf("source type = %T, want *backtest.CachedSignalSource", source)
	}
}

func TestBuildSignalSourceModelMissingWeights(t *testing.T) {
	_, _, err := buildSignalSource(nil, signalSourceOptions{
		Mode:      signalSourceModel,
		ModelPath: filepath.Join(t.TempDir(), "missing.json"),
	})
	if err == nil {
		t.Fatal("buildSignalSource did not return an error for missing model weights")
	}
}

func TestBuildSignalSourceUnknownMode(t *testing.T) {
	_, _, err := buildSignalSource(nil, signalSourceOptions{Mode: "other"})
	if err == nil {
		t.Fatal("buildSignalSource did not return an error for unknown mode")
	}
}
