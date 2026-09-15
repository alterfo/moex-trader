package backtest

import (
	"context"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

// ConfidenceGateSource wraps another SignalSource and demotes any BUY/SELL
// whose confidence falls below MinConfidence to HOLD. The gate sits above the
// cache layer so a single LLM decision cache can be replayed against several
// confidence thresholds without re-invoking the model.
type ConfidenceGateSource struct {
	Inner         SignalSource
	MinConfidence decimal.Decimal
}

func (s *ConfidenceGateSource) Generate(ctx context.Context, feature domain.FeatureContext) (domain.TradeSignal, error) {
	signal, err := s.Inner.Generate(ctx, feature)
	if err != nil {
		return signal, err
	}
	if signal.Action == domain.ActionHold {
		return signal, nil
	}
	if signal.Confidence.LessThan(s.MinConfidence) {
		signal.Action = domain.ActionHold
		signal.TargetLots = 0
		signal.HoldReason = domain.HoldReasonConfidence
	}
	return signal, nil
}
