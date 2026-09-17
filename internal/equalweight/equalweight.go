package equalweight

import (
	"context"
	"math"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/domain"
)

// SignalSource holds every universe ticker at a constant long notional,
// rebalanced by the target-position executor on every poll. It is the
// null-universe benchmark: no selection, no timing, just equal weight across
// the 18 deployed names.
type SignalSource struct {
	TargetNotional decimal.Decimal
	MaxLots        int
}

func (s *SignalSource) targetLots(feature domain.FeatureContext) int {
	if s == nil || !s.TargetNotional.IsPositive() {
		if s != nil && s.MaxLots > 0 {
			return s.MaxLots
		}
		return 1
	}
	if !feature.LastPrice.IsPositive() {
		if s.MaxLots > 0 {
			return s.MaxLots
		}
		return 1
	}
	perUnit := feature.LastPrice
	if feature.LotSize.IsPositive() {
		perUnit = feature.LastPrice.Mul(feature.LotSize)
	}
	lots := s.TargetNotional.Div(perUnit).Round(0).IntPart()
	if lots < 1 {
		return 1
	}
	if lots > math.MaxInt32 {
		return math.MaxInt32
	}
	return int(lots)
}

func (s *SignalSource) TargetPosition(feature domain.FeatureContext) (int, bool) {
	return s.targetLots(feature), true
}

func (s *SignalSource) Generate(_ context.Context, feature domain.FeatureContext) (domain.TradeSignal, error) {
	lots := s.targetLots(feature)
	return domain.TradeSignal{
		Ticker:      feature.Ticker,
		Action:      domain.ActionBuy,
		Confidence:  decimal.NewFromInt(1),
		TargetLots:  lots,
		Reasoning:   "equal-weight",
		GeneratedAt: feature.GeneratedAt,
	}, nil
}

var _ backtest.SignalSource = (*SignalSource)(nil)
var _ backtest.TargetPositionSource = (*SignalSource)(nil)
