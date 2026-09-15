package model

import (
	"math"
	"math/rand"
	"testing"
)

func TestStandardize(t *testing.T) {
	samples := []Sample{
		{X: []float64{1, 10, 5}, Y: 0},
		{X: []float64{3, 10, 7}, Y: 1},
		{X: []float64{5, 10, 3}, Y: 1},
	}
	mean, std := Standardize(samples)
	if len(mean) != 3 || len(std) != 3 {
		t.Fatalf("got mean/std lengths %d/%d, want 3/3", len(mean), len(std))
	}
	if mean[0] != 3 {
		t.Fatalf("mean[0] = %v, want 3", mean[0])
	}
	if std[0] == 0 || std[1] != 1 || std[2] == 0 {
		t.Fatalf("std = %v, want non-zero for varying columns and 1 for constant", std)
	}
}

func TestStandardizeEmpty(t *testing.T) {
	mean, std := Standardize(nil)
	if mean != nil || std != nil {
		t.Fatalf("empty samples returned mean=%v std=%v, want nil/nil", mean, std)
	}
}

func TestTrainSeparableData(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	samples := make([]Sample, 400)
	for i := range samples {
		x := rng.Float64()*2 - 1
		label := 0.0
		if x > 0 {
			label = 1
		}
		samples[i] = Sample{
			X: []float64{x, rng.Float64()*2 - 1, rng.Float64()*2 - 1},
			Y: label,
		}
	}
	weights, bias, err := Train(samples, DefaultTrainConfig())
	if err != nil {
		t.Fatalf("Train failed: %v", err)
	}
	if weights[0] <= 0 {
		t.Fatalf("informative feature weight = %v, want > 0", weights[0])
	}

	correct := 0
	total := 200
	for i := 0; i < total; i++ {
		x := rng.Float64()*2 - 1
		row := []float64{x, rng.Float64()*2 - 1, rng.Float64()*2 - 1}
		prediction := sigmoid(dot(weights, row) + bias)
		label := 0.0
		if x > 0 {
			label = 1
		}
		if (prediction >= 0.5) == (label == 1) {
			correct++
		}
	}
	if accuracy := float64(correct) / float64(total); accuracy <= 0.9 {
		t.Fatalf("accuracy = %v, want > 0.9", accuracy)
	}
}

func TestTrainZeroVarianceFeaturesStayFinite(t *testing.T) {
	samples := []Sample{
		{X: []float64{0, -1}, Y: 0},
		{X: []float64{0, -0.5}, Y: 0},
		{X: []float64{0, 0.5}, Y: 1},
		{X: []float64{0, 1}, Y: 1},
	}
	weights, bias, err := Train(samples, DefaultTrainConfig())
	if err != nil {
		t.Fatalf("Train failed: %v", err)
	}
	for _, value := range append(weights, bias) {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			t.Fatalf("non-finite model value: %v", value)
		}
	}
	if weights[0] != 0 {
		t.Fatalf("zero-variance feature weight = %v, want 0", weights[0])
	}
}

func TestTrainErrorCases(t *testing.T) {
	if _, _, err := Train(nil, DefaultTrainConfig()); err == nil {
		t.Fatal("empty samples did not return an error")
	}
	if _, _, err := Train([]Sample{{X: []float64{}, Y: 0}}, DefaultTrainConfig()); err == nil {
		t.Fatal("empty feature vector did not return an error")
	}
	mismatched := []Sample{
		{X: []float64{1, 2}, Y: 0},
		{X: []float64{1}, Y: 1},
	}
	if _, _, err := Train(mismatched, DefaultTrainConfig()); err == nil {
		t.Fatal("mismatched feature lengths did not return an error")
	}
	if _, _, err := Train([]Sample{{X: []float64{1}, Y: 2}}, DefaultTrainConfig()); err == nil {
		t.Fatal("invalid label did not return an error")
	}
}
