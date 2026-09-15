package backtest

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

// ReversalRuleSource implements a deterministic mean-reversion rule: sell when
// the one-day return (Reversal1d) is above the threshold, buy when below
// minus-threshold, hold otherwise. Used for A/B comparison against the LLM to
// establish whether the model adds value over the bare rule.
type ReversalRuleSource struct {
	Threshold decimal.Decimal
	MaxLots   int
}

func (s *ReversalRuleSource) Generate(_ context.Context, feature domain.FeatureContext) (domain.TradeSignal, error) {
	if s == nil {
		return domain.TradeSignal{}, fmt.Errorf("reversal rule: source is nil")
	}
	if s.Threshold.Sign() < 0 {
		return domain.TradeSignal{}, fmt.Errorf("reversal rule: threshold must not be negative")
	}
	lots := s.MaxLots
	if lots <= 0 {
		lots = 1
	}

	signal := domain.TradeSignal{
		Ticker:      feature.Ticker,
		Action:      domain.ActionHold,
		Confidence:  decimal.NewFromInt(1),
		TargetLots:  0,
		GeneratedAt: feature.GeneratedAt,
		HoldReason:  domain.HoldReasonModel,
	}
	switch {
	case feature.Reversal1d.GreaterThanOrEqual(s.Threshold):
		signal.Action = domain.ActionSell
		signal.TargetLots = lots
		signal.Reasoning = fmt.Sprintf("rule:reversal_1d=%s >= %s", feature.Reversal1d.String(), s.Threshold.String())
	case feature.Reversal1d.LessThanOrEqual(s.Threshold.Neg()):
		signal.Action = domain.ActionBuy
		signal.TargetLots = lots
		signal.Reasoning = fmt.Sprintf("rule:reversal_1d=%s <= %s", feature.Reversal1d.String(), s.Threshold.Neg().String())
	}
	return signal, nil
}
