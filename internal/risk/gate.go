package risk

import (
	"context"
	"fmt"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

type Gate interface {
	Approve(ctx context.Context, signal domain.TradeSignal) (bool, error)
}

type LotLimitGate struct {
	maxLots int
}

func NewLotLimitGate(maxLots int) (*LotLimitGate, error) {
	if maxLots <= 0 {
		return nil, fmt.Errorf("risk gate: max lots must be positive")
	}
	return &LotLimitGate{maxLots: maxLots}, nil
}

func (g *LotLimitGate) Approve(ctx context.Context, signal domain.TradeSignal) (bool, error) {
	if err := signal.Validate(); err != nil {
		return false, fmt.Errorf("risk gate: invalid signal: %w", err)
	}
	if signal.TargetLots < 0 {
		return false, fmt.Errorf("risk gate: target lots must be non-negative")
	}
	if signal.TargetLots > g.maxLots {
		return false, nil
	}
	return true, nil
}
