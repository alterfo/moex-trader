package model

import (
	"math"
	"strings"
	"testing"
)

func explainTestModel() *EnsembleModel {
	stump := treeNode{
		SplitIndices:    []int{1, 0, 0},
		SplitConditions: []float64{0.0, -0.4, 0.4},
		LeftChildren:    []int{1, -1, -1},
		RightChildren:   []int{2, -1, -1},
		DefaultLeft:     []int{1, 0, 0},
	}
	return &EnsembleModel{
		FeatureOrder:  []string{"rsi_14", "mom_21d", "news_sentiment"},
		BuyThreshold:  0.6,
		SellThreshold: 0.4,
		Logistic: logisticWeights{
			Mean: []float64{50, 0.02, 0},
			Std:  []float64{10, 0.05, 1},
			Coef: []float64{0.3, 0.8, 5},
			Bias: 0.1,
		},
		LGBTrees: []treeNode{stump},
		XGBTrees: []treeNode{stump},
	}
}

func logit(p float64) float64 { return math.Log(p / (1 - p)) }

func TestDriversLogisticMatchesExactLinearContribution(t *testing.T) {
	m := explainTestModel()
	vector := []float64{62, 0.11, 0}
	contrib := m.LogisticContributions(vector)
	base := logit(m.logisticProbability(vector))
	for i := range vector {
		probe := append([]float64(nil), vector...)
		probe[i] = m.Logistic.Mean[i]
		got := base - logit(m.logisticProbability(probe))
		if math.Abs(got-contrib[i]) > 1e-9 {
			t.Fatalf("feature %d: occlusion logit delta %.12f != linear contribution %.12f", i, got, contrib[i])
		}
	}
}

func TestDriversZeroAtBaselineVector(t *testing.T) {
	m := explainTestModel()
	baseline := append([]float64(nil), m.Logistic.Mean...)
	for _, member := range []EnsembleMember{EnsembleMemberAll, EnsembleMemberLGB, EnsembleMemberLogistic} {
		if got := m.Drivers(member, baseline, 3); len(got) != 0 {
			t.Fatalf("member %q: baseline vector must have no drivers, got %+v", member, got)
		}
	}
}

func TestDriversOrderedDeterministicAndSkipUntrained(t *testing.T) {
	m := explainTestModel()
	vector := []float64{62, 0.11, 3}
	first := m.Drivers(EnsembleMemberAll, vector, 3)
	second := m.Drivers(EnsembleMemberAll, vector, 3)
	if len(first) != len(second) {
		t.Fatalf("non-deterministic length: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("non-deterministic driver %d: %+v vs %+v", i, first[i], second[i])
		}
		if first[i].Feature == "news_sentiment" {
			t.Fatalf("untrained feature must not be a driver: %+v", first)
		}
		if i > 0 && math.Abs(first[i].Delta) > math.Abs(first[i-1].Delta) {
			t.Fatalf("drivers must be ordered by absolute delta: %+v", first)
		}
	}
	if len(first) != 2 {
		t.Fatalf("expected 2 drivers (rsi_14, mom_21d), got %+v", first)
	}
}

func TestFormatDriversSuffixKeepsProbabilityParsable(t *testing.T) {
	suffix := FormatDrivers([]Driver{{Feature: "mom_21d", Delta: 0.0312}, {Feature: "rsi_14", Delta: -0.0124}})
	if suffix != " drivers=mom_21d:+3.1,rsi_14:-1.2" {
		t.Fatalf("unexpected suffix %q", suffix)
	}
	reasoning := "ensemble:p=0.6234" + suffix
	idx := strings.Index(reasoning, "p=")
	rest := reasoning[idx+2:]
	if end := strings.IndexAny(rest, " ,"); end >= 0 {
		rest = rest[:end]
	}
	if rest != "0.6234" {
		t.Fatalf("probability parse broke: %q", rest)
	}
	if FormatDrivers(nil) != "" {
		t.Fatal("empty drivers must produce an empty suffix")
	}
}
