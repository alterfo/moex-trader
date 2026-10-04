package model

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type parityFixture struct {
	FeatureOrder []string    `json:"feature_order"`
	Rows         []parityRow `json:"rows"`
}

type parityRow struct {
	Ticker   string    `json:"ticker"`
	Date     string    `json:"date"`
	Features []float64 `json:"features"`
	LGB      float64   `json:"lgb"`
	XGB      float64   `json:"xgb"`
	Logistic float64   `json:"logistic"`
	Ensemble float64   `json:"ensemble"`
}

func readParityFixture(path string, featureOrder []string) (*parityFixture, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("parity fixture: read %q: %w", path, err)
	}
	var fixture parityFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		return nil, fmt.Errorf("parity fixture: parse %q: %w", path, err)
	}
	if err := validateParityFixture(&fixture, featureOrder); err != nil {
		return nil, fmt.Errorf("parity fixture %q: %w", path, err)
	}
	return &fixture, nil
}

func validateParityFixture(fixture *parityFixture, featureOrder []string) error {
	if err := checkFeatureOrder(featureOrder, fixture.FeatureOrder); err != nil {
		return err
	}
	if len(fixture.Rows) == 0 {
		return fmt.Errorf("parity fixture: no rows")
	}
	for i, row := range fixture.Rows {
		if len(row.Features) != len(featureOrder) {
			return fmt.Errorf("parity fixture: row %d has %d features, want %d", i, len(row.Features), len(featureOrder))
		}
	}
	return nil
}

func loadDeployedParityFixture(t *testing.T) (*EnsembleModel, *parityFixture) {
	t.Helper()
	ensemble, err := LoadEnsembleModel(filepath.Join("..", "..", "ensemble_model.json"))
	if err != nil {
		t.Fatalf("load deployed ensemble model: %v", err)
	}
	fixture, err := readParityFixture(filepath.Join("testdata", "parity.json"), ensemble.FeatureOrder)
	if err != nil {
		t.Fatalf("load parity fixture: %v", err)
	}
	return ensemble, fixture
}

func TestEnsembleMatchesPythonPredictProba(t *testing.T) {
	ensemble, fixture := loadDeployedParityFixture(t)

	maxLGB := 0.0
	maxXGB := 0.0
	maxLogistic := 0.0
	maxEnsemble := 0.0
	for _, row := range fixture.Rows {
		lgb, xgb, logistic := ensemble.MemberProbabilities(row.Features)
		ensembleProb := ensemble.Probability(row.Features)
		maxLGB = math.Max(maxLGB, math.Abs(lgb-row.LGB))
		maxXGB = math.Max(maxXGB, math.Abs(xgb-row.XGB))
		maxLogistic = math.Max(maxLogistic, math.Abs(logistic-row.Logistic))
		maxEnsemble = math.Max(maxEnsemble, math.Abs(ensembleProb-row.Ensemble))
	}

	t.Logf("rows=%d max_abs_err lgb=%.3e xgb=%.3e logistic=%.3e ensemble=%.3e",
		len(fixture.Rows), maxLGB, maxXGB, maxLogistic, maxEnsemble)
	const tolerance = 1e-6
	if maxLGB > tolerance {
		t.Fatalf("lgb probability mismatch vs python: max_abs_err=%.3e", maxLGB)
	}
	if maxXGB > tolerance {
		t.Fatalf("xgb probability mismatch vs python: max_abs_err=%.3e", maxXGB)
	}
	if maxLogistic > tolerance {
		t.Fatalf("logistic probability mismatch vs python: max_abs_err=%.3e", maxLogistic)
	}
	if maxEnsemble > tolerance {
		t.Fatalf("ensemble probability mismatch vs python: max_abs_err=%.3e", maxEnsemble)
	}
}

func TestParityFixtureFeatureOrderMismatchFails(t *testing.T) {
	want := []string{"return_pct", "realized_volatility"}
	fixture := &parityFixture{
		FeatureOrder: []string{"return_pct", "news_sentiment"},
		Rows: []parityRow{{
			Features: []float64{1.0, 2.0},
		}},
	}
	err := validateParityFixture(fixture, want)
	if err == nil {
		t.Fatal("expected feature order mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "feature order mismatch") {
		t.Fatalf("expected a feature order mismatch error, got: %v", err)
	}
}

func TestParityFixtureRowDimensionMismatchFails(t *testing.T) {
	want := []string{"return_pct", "realized_volatility"}
	fixture := &parityFixture{
		FeatureOrder: want,
		Rows: []parityRow{{
			Features: []float64{1.0},
		}},
	}
	err := validateParityFixture(fixture, want)
	if err == nil {
		t.Fatal("expected row dimension error, got nil")
	}
	if !strings.Contains(err.Error(), "has 1 features") {
		t.Fatalf("expected a row dimension error, got: %v", err)
	}
}
