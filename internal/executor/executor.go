package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/storage"
)

type Fill struct {
	ID     string          `json:"id"`
	Ticker string          `json:"ticker"`
	Action domain.Action   `json:"action"`
	Lots   int             `json:"lots"`
	Price  decimal.Decimal `json:"price"`
	// ExpectedPrice is the bounded LIMIT order price (decision price adjusted
	// by MaxSlippagePct) against which the realized fill price is measured.
	// It is zero for MARKET orders, which carry no price bound.
	ExpectedPrice decimal.Decimal `json:"expected_price,omitempty"`
	Commission    decimal.Decimal `json:"commission"`
	ExecutedAt    time.Time       `json:"executed_at"`
}

type Executor interface {
	Execute(ctx context.Context, signal domain.TradeSignal, price decimal.Decimal) (Fill, error)
}

type PaperExecutor struct {
	store          *storage.Store
	now            func() time.Time
	commissionRate decimal.Decimal
}

func NewPaperExecutor(store *storage.Store, now func() time.Time) *PaperExecutor {
	return NewPaperExecutorWithCommission(store, now, decimal.Zero)
}

func NewPaperExecutorWithCommission(store *storage.Store, now func() time.Time, commissionRate decimal.Decimal) *PaperExecutor {
	if now == nil {
		now = time.Now
	}
	return &PaperExecutor{store: store, now: now, commissionRate: commissionRate}
}

func (p *PaperExecutor) Execute(ctx context.Context, signal domain.TradeSignal, price decimal.Decimal) (Fill, error) {
	var empty Fill
	if err := signal.Validate(); err != nil {
		return empty, fmt.Errorf("paper executor: invalid signal: %w", err)
	}
	if signal.TargetLots < 0 {
		return empty, fmt.Errorf("paper executor: target lots must be non-negative")
	}
	if price.Sign() <= 0 {
		return empty, fmt.Errorf("paper executor: fill price must be positive")
	}
	if p.store == nil {
		return empty, fmt.Errorf("paper executor: storage store is nil")
	}

	lots := 0
	if signal.Action == domain.ActionBuy || signal.Action == domain.ActionSell {
		lots = signal.TargetLots
	}

	fill := Fill{
		ID:         uuid.NewString(),
		Ticker:     signal.Ticker,
		Action:     signal.Action,
		Lots:       lots,
		Price:      price,
		Commission: commissionAmount(price, lots, p.commissionRate),
		ExecutedAt: p.now(),
	}

	payload, err := json.Marshal(fill)
	if err != nil {
		return empty, fmt.Errorf("paper executor: marshal fill: %w", err)
	}

	event := domain.AuditEvent{
		ID:        fill.ID,
		Ticker:    fill.Ticker,
		Stage:     "executor",
		Payload:   string(payload),
		CreatedAt: fill.ExecutedAt,
	}
	if err := p.store.InsertAuditEvent(ctx, event); err != nil {
		return empty, fmt.Errorf("paper executor: persist fill: %w", err)
	}

	return fill, nil
}

func commissionAmount(price decimal.Decimal, lots int, rate decimal.Decimal) decimal.Decimal {
	if lots <= 0 || rate.IsNegative() || price.Sign() <= 0 {
		return decimal.Zero
	}
	return price.Mul(decimal.NewFromInt(int64(lots))).Mul(rate)
}
