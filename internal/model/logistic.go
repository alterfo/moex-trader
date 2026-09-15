package model

import (
	"errors"
	"fmt"
	"math"
)

type Sample struct {
	X []float64
	Y float64
}

type TrainConfig struct {
	LearningRate float64
	L2Lambda     float64
	Epochs       int
}

const (
	defaultLearningRate = 0.1
	defaultL2Lambda     = 0.001
	defaultEpochs       = 1000
)

func DefaultTrainConfig() TrainConfig {
	return TrainConfig{
		LearningRate: defaultLearningRate,
		L2Lambda:     defaultL2Lambda,
		Epochs:       defaultEpochs,
	}
}

func Standardize(samples []Sample) ([]float64, []float64) {
	if len(samples) == 0 {
		return nil, nil
	}
	dim := len(samples[0].X)
	for _, sample := range samples {
		if len(sample.X) != dim {
			return nil, nil
		}
	}
	mean := make([]float64, dim)
	for _, sample := range samples {
		for i, value := range sample.X {
			mean[i] += value
		}
	}
	for i := range mean {
		mean[i] /= float64(len(samples))
	}
	std := make([]float64, dim)
	for _, sample := range samples {
		for i, value := range sample.X {
			delta := value - mean[i]
			std[i] += delta * delta
		}
	}
	for i := range std {
		std[i] = math.Sqrt(std[i] / float64(len(samples)))
		if std[i] == 0 {
			std[i] = 1
		}
	}
	return mean, std
}

func Train(samples []Sample, cfg TrainConfig) ([]float64, float64, error) {
	if err := validateSamples(samples); err != nil {
		return nil, 0, err
	}
	if cfg.LearningRate <= 0 {
		cfg.LearningRate = defaultLearningRate
	}
	if cfg.L2Lambda < 0 {
		cfg.L2Lambda = defaultL2Lambda
	}
	if cfg.Epochs <= 0 {
		cfg.Epochs = defaultEpochs
	}

	mean, std := Standardize(samples)
	dim := len(samples[0].X)
	normalized := make([][]float64, len(samples))
	for i, sample := range samples {
		normalized[i] = make([]float64, dim)
		for j, value := range sample.X {
			normalized[i][j] = (value - mean[j]) / std[j]
		}
	}

	weights := make([]float64, dim)
	bias := 0.0
	regularization := 2 * cfg.L2Lambda
	scale := 1.0 / float64(len(samples))
	for epoch := 0; epoch < cfg.Epochs; epoch++ {
		gradient := make([]float64, dim)
		gradientBias := 0.0
		for i, row := range normalized {
			prediction := sigmoid(dot(weights, row) + bias)
			errorTerm := prediction - samples[i].Y
			gradientBias += errorTerm
			for j, value := range row {
				gradient[j] += errorTerm * value
			}
		}
		for j := range weights {
			weights[j] -= cfg.LearningRate * (gradient[j]*scale + regularization*weights[j])
		}
		bias -= cfg.LearningRate * gradientBias * scale
	}
	return weights, bias, nil
}

func validateSamples(samples []Sample) error {
	if len(samples) == 0 {
		return errors.New("model: training samples must not be empty")
	}
	dim := len(samples[0].X)
	if dim == 0 {
		return errors.New("model: feature vector must not be empty")
	}
	for i, sample := range samples {
		if len(sample.X) != dim {
			return fmt.Errorf("model: sample %d has %d features, want %d", i, len(sample.X), dim)
		}
		if sample.Y != 0 && sample.Y != 1 {
			return fmt.Errorf("model: sample %d label must be 0 or 1, got %v", i, sample.Y)
		}
	}
	return nil
}

func dot(a, b []float64) float64 {
	total := 0.0
	for i := range a {
		total += a[i] * b[i]
	}
	return total
}

func sigmoid(z float64) float64 {
	if z >= 0 {
		return 1 / (1 + math.Exp(-z))
	}
	exponential := math.Exp(z)
	return exponential / (1 + exponential)
}
