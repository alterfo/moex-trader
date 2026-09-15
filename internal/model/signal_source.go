package model

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/orchestrator"
)

var _ orchestrator.SignalSource = (*SignalSource)(nil)

type SignalSource struct {
	Weights *Weights
	MaxLots int
}

func (s *SignalSource) Generate(ctx context.Context, feature domain.FeatureContext) (domain.TradeSignal, error) {
	if strings.TrimSpace(feature.Ticker) == "" {
		return domain.TradeSignal{}, fmt.Errorf("model: feature ticker must not be empty")
	}
	if s == nil || s.Weights == nil {
		return domain.TradeSignal{}, fmt.Errorf("model: weights are required")
	}
	if err := validateFeatureDimensions(s.Weights); err != nil {
		return domain.TradeSignal{}, err
	}

	vector, names := ToVector(feature)
	standardized := make([]float64, len(vector))
	for i, value := range vector {
		std := s.Weights.Std[i]
		if std == 0 {
			std = 1
		}
		standardized[i] = (value - s.Weights.Mean[i]) / std
	}

	logit := dot(s.Weights.Coef, standardized) + s.Weights.Bias
	probability := sigmoid(logit)
	if math.IsNaN(probability) || math.IsInf(probability, 0) {
		return domain.TradeSignal{}, fmt.Errorf("model: computed probability is not finite for %s", feature.Ticker)
	}

	signal := domain.TradeSignal{
		Ticker:      feature.Ticker,
		Action:      domain.ActionHold,
		Confidence:  decimal.NewFromFloat(math.Abs(probability-0.5) * 2),
		TargetLots:  0,
		Reasoning:   reasoning(probability, names, s.Weights.Coef, standardized),
		GeneratedAt: feature.GeneratedAt,
	}

	switch {
	case probability >= s.Weights.BuyThreshold:
		signal.Action = domain.ActionBuy
		signal.TargetLots = s.MaxLots
	case probability <= s.Weights.SellThreshold:
		signal.Action = domain.ActionSell
		signal.TargetLots = s.MaxLots
	default:
		signal.HoldReason = domain.HoldReasonModel
	}

	return signal, nil
}

func validateFeatureDimensions(weights *Weights) error {
	_, names := ToVector(domain.FeatureContext{})
	if len(names) == 0 {
		return fmt.Errorf("model: feature order is empty")
	}
	if len(weights.FeatureOrder) != len(names) {
		return fmt.Errorf("model: feature order has %d fields, want %d", len(weights.FeatureOrder), len(names))
	}
	if len(weights.Coef) != len(names) || len(weights.Mean) != len(names) || len(weights.Std) != len(names) {
		return fmt.Errorf("model: weights dimension mismatch: coef=%d mean=%d std=%d feature_order=%d",
			len(weights.Coef), len(weights.Mean), len(weights.Std), len(names))
	}
	for i, name := range names {
		if weights.FeatureOrder[i] != name {
			return fmt.Errorf("model: feature order mismatch at index %d: got %q, want %q", i, weights.FeatureOrder[i], name)
		}
	}
	return nil
}

func reasoning(probability float64, names []string, coef, standardized []float64) string {
	topIndex := 0
	topContribution := math.Abs(coef[0] * standardized[0])
	for i := 1; i < len(names); i++ {
		contribution := math.Abs(coef[i] * standardized[i])
		if contribution > topContribution {
			topIndex = i
			topContribution = contribution
		}
	}
	return fmt.Sprintf("model:p=%.4f;top_feature=%s:%+.4f", probability, names[topIndex], coef[topIndex]*standardized[topIndex])
}
