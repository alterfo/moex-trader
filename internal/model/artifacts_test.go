package model

import (
	"path/filepath"
	"testing"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

func TestDeployedModelArtifactsMatchFeatureOrder(t *testing.T) {
	_, names := ToVector(domain.FeatureContext{})
	if len(names) == 0 {
		t.Fatal("ToVector returned an empty feature order")
	}

	ensemble, err := LoadEnsembleModel(filepath.Join("..", "..", "ensemble_model.json"))
	if err != nil {
		t.Fatalf("load deployed ensemble_model.json: %v", err)
	}
	if err := checkFeatureOrder(ensemble.FeatureOrder, names); err != nil {
		t.Fatalf("ensemble_model.json no longer matches ToVector: %v", err)
	}

	weights, err := LoadWeights(filepath.Join("..", "..", "model.json"))
	if err != nil {
		t.Fatalf("load deployed model.json: %v", err)
	}
	if err := validateFeatureDimensions(weights); err != nil {
		t.Fatalf("model.json no longer matches ToVector: %v", err)
	}
}
