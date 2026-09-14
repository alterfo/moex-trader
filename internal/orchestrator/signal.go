package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

type RuleSignalSource struct {
	now func() time.Time
}

func NewRuleSignalSource(now func() time.Time) *RuleSignalSource {
	if now == nil {
		now = time.Now
	}
	return &RuleSignalSource{now: now}
}

func (s *RuleSignalSource) Generate(ctx context.Context, feature domain.FeatureContext) (domain.TradeSignal, error) {
	var empty domain.TradeSignal
	ticker := strings.TrimSpace(feature.Ticker)
	if ticker == "" {
		return empty, fmt.Errorf("rule signal source: ticker must not be empty")
	}

	signal := domain.TradeSignal{
		Ticker:      ticker,
		GeneratedAt: s.now(),
	}

	positiveThreshold := decimal.NewFromFloat(0.1)
	negativeThreshold := decimal.NewFromFloat(-0.1)
	switch {
	case feature.NewsSentiment.GreaterThan(positiveThreshold):
		signal.Action = domain.ActionBuy
		signal.TargetLots = 1
		signal.Confidence = boundedConfidence(feature.NewsSentiment.Abs().Add(decimal.NewFromFloat(0.2)))
		signal.Reasoning = "positive news sentiment"
	case feature.NewsSentiment.LessThan(negativeThreshold):
		signal.Action = domain.ActionSell
		signal.TargetLots = 1
		signal.Confidence = boundedConfidence(feature.NewsSentiment.Abs().Add(decimal.NewFromFloat(0.2)))
		signal.Reasoning = "negative news sentiment"
	default:
		signal.Action = domain.ActionHold
		signal.TargetLots = 0
		signal.Confidence = decimal.NewFromFloat(0.5)
		signal.Reasoning = "neutral news sentiment"
	}

	if err := signal.Validate(); err != nil {
		return empty, fmt.Errorf("rule signal source: %w", err)
	}
	return signal, nil
}

func boundedConfidence(value decimal.Decimal) decimal.Decimal {
	min := decimal.NewFromFloat(0.3)
	max := decimal.NewFromFloat(0.95)
	if value.LessThan(min) {
		return min
	}
	if value.GreaterThan(max) {
		return max
	}
	return value
}
