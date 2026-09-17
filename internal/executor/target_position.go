package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/risk"
)

func nowFunc() time.Time { return time.Now() }

type TargetPositionExecutor struct {
	inner           Executor
	positions       risk.PositionReader
	now             func() time.Time
	minDeviationPct decimal.Decimal
}

func NewTargetPositionExecutor(inner Executor, positions risk.PositionReader, now func() time.Time) *TargetPositionExecutor {
	return NewTargetPositionExecutorWithConfig(inner, positions, now, TargetPositionConfig{})
}

type TargetPositionConfig struct {
	RebalanceMinDeviationPct decimal.Decimal
}

func NewTargetPositionExecutorWithConfig(inner Executor, positions risk.PositionReader, now func() time.Time, cfg TargetPositionConfig) *TargetPositionExecutor {
	if now == nil {
		now = nowFunc
	}
	return &TargetPositionExecutor{
		inner:           inner,
		positions:       positions,
		now:             now,
		minDeviationPct: cfg.RebalanceMinDeviationPct,
	}
}

func (t *TargetPositionExecutor) Inner() Executor {
	return t.inner
}

func (t *TargetPositionExecutor) Execute(ctx context.Context, signal domain.TradeSignal, price decimal.Decimal) (Fill, error) {
	if signal.Action != domain.ActionBuy && signal.Action != domain.ActionSell {
		return t.inner.Execute(ctx, signal, price)
	}
	current, err := t.positions.CurrentLots(ctx, signal.Ticker)
	if err != nil {
		return Fill{}, fmt.Errorf("target position executor: read current lots for %s: %w", signal.Ticker, err)
	}
	desired := signal.TargetLots
	if signal.Action == domain.ActionSell {
		desired = -signal.TargetLots
	}
	delta := desired - current
	if delta == 0 {
		return t.noopFill(signal, price), nil
	}
	if t.skipRebalance(delta, desired) {
		return t.noopFill(signal, price), nil
	}
	target := signal
	target.TargetLots = delta
	if delta < 0 {
		target.TargetLots = -delta
		target.Action = domain.ActionSell
	} else {
		target.Action = domain.ActionBuy
	}
	return t.inner.Execute(ctx, target, price)
}

func (t *TargetPositionExecutor) skipRebalance(delta, desired int) bool {
	if !t.minDeviationPct.IsPositive() {
		return false
	}
	if desired == 0 {
		return false
	}
	deviation := decimal.NewFromInt(int64(absInt(delta))).Div(decimal.NewFromInt(int64(absInt(desired))))
	return deviation.LessThanOrEqual(t.minDeviationPct)
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func (t *TargetPositionExecutor) noopFill(signal domain.TradeSignal, price decimal.Decimal) Fill {
	return Fill{
		ID:         "noop-" + signal.Ticker,
		Ticker:     signal.Ticker,
		Action:     domain.ActionHold,
		Lots:       0,
		Price:      price,
		ExecutedAt: t.now(),
	}
}
