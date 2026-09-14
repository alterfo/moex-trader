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
	ID         string          `json:"id"`
	Ticker     string          `json:"ticker"`
	Action     domain.Action   `json:"action"`
	Lots       int             `json:"lots"`
	Price      decimal.Decimal `json:"price"`
	ExecutedAt time.Time       `json:"executed_at"`
}

type Executor interface {
	Execute(ctx context.Context, signal domain.TradeSignal, price decimal.Decimal) (Fill, error)
}

type PaperExecutor struct {
	store *storage.Store
	now   func() time.Time
}

func NewPaperExecutor(store *storage.Store, now func() time.Time) *PaperExecutor {
	if now == nil {
		now = time.Now
	}
	return &PaperExecutor{store: store, now: now}
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
