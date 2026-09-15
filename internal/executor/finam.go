package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/finam"
	"github.com/olegsidorkin/moex-trader/internal/storage"
)

type FinamSymbolResolver func(ticker string) (string, error)

type FinamConfig struct {
	AccountID      string
	ResolveSymbol  FinamSymbolResolver
	Store          *storage.Store
	Now            func() time.Time
	CommissionRate decimal.Decimal
}

type FinamExecutor struct {
	client         *finam.Client
	accountID      string
	resolve        FinamSymbolResolver
	store          *storage.Store
	now            func() time.Time
	commissionRate decimal.Decimal
}

// NewFinamExecutor builds a live Finam executor. CommissionRate is a config-rate estimate because Finam does not return commission in the order-placement response.
func NewFinamExecutor(client *finam.Client, cfg FinamConfig) (*FinamExecutor, error) {
	if client == nil {
		return nil, fmt.Errorf("finam executor: client is nil")
	}
	accountID := strings.TrimSpace(cfg.AccountID)
	if accountID == "" {
		return nil, fmt.Errorf("finam executor: account id must not be empty")
	}
	if cfg.ResolveSymbol == nil {
		return nil, fmt.Errorf("finam executor: symbol resolver must not be nil")
	}
	if cfg.Store == nil {
		return nil, fmt.Errorf("finam executor: storage store is nil")
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &FinamExecutor{
		client:         client,
		accountID:      accountID,
		resolve:        cfg.ResolveSymbol,
		store:          cfg.Store,
		now:            now,
		commissionRate: cfg.CommissionRate,
	}, nil
}

func (f *FinamExecutor) Execute(ctx context.Context, signal domain.TradeSignal, price decimal.Decimal) (Fill, error) {
	if err := validateFinamInput(signal, price); err != nil {
		return Fill{}, err
	}
	if signal.Action == domain.ActionHold {
		fill := Fill{
			ID:         uuid.NewString(),
			Ticker:     signal.Ticker,
			Action:     signal.Action,
			Lots:       0,
			Price:      price,
			ExecutedAt: f.now(),
		}
		if err := f.record(ctx, fill); err != nil {
			return Fill{}, err
		}
		return fill, nil
	}

	symbol, err := f.resolve(signal.Ticker)
	if err != nil {
		return Fill{}, fmt.Errorf("finam executor: resolve symbol for %q: %w", signal.Ticker, err)
	}
	symbol = strings.TrimSpace(symbol)
	if symbol == "" {
		return Fill{}, fmt.Errorf("finam executor: symbol for %q must not be empty", signal.Ticker)
	}

	orderID := uuid.NewString()
	side, err := finamSide(signal.Action)
	if err != nil {
		return Fill{}, err
	}
	response, err := f.client.PlaceOrder(ctx, f.accountID, finam.PlaceOrderRequest{
		Symbol:        symbol,
		Quantity:      decimal.NewFromInt(int64(signal.TargetLots)),
		Side:          side,
		Type:          finam.OrderTypeMarket,
		TimeInForce:   finam.TimeInForceDay,
		ClientOrderID: finamClientOrderID(orderID),
	})
	if err != nil {
		return Fill{}, fmt.Errorf("finam executor: place order %q: %w", orderID, err)
	}

	switch response.Status {
	case finam.OrderStatusFilled, finam.OrderStatusExecuted:
		lots := signal.TargetLots
		if response.ExecutedQuantity.IsPositive() {
			lots = int(response.ExecutedQuantity.IntPart())
		}
		if lots <= 0 {
			return Fill{}, fmt.Errorf("finam executor: order %q reported %s with zero executed lots", orderID, response.Status)
		}
		fill := Fill{
			ID:         orderID,
			Ticker:     signal.Ticker,
			Action:     signal.Action,
			Lots:       lots,
			Price:      price,
			Commission: commissionAmount(price, lots, f.commissionRate),
			ExecutedAt: f.now(),
		}
		if err := f.record(ctx, fill); err != nil {
			return fill, err
		}
		return fill, nil
	case finam.OrderStatusPartiallyFilled:
		executed := int(response.ExecutedQuantity.IntPart())
		return Fill{}, fmt.Errorf("finam executor: order %q partially filled: %d of %d lots executed", orderID, executed, signal.TargetLots)
	case finam.OrderStatusRejected, finam.OrderStatusRejectedByExch, finam.OrderStatusDeniedByBroker, finam.OrderStatusCanceled, finam.OrderStatusExpired, finam.OrderStatusFailed:
		return Fill{}, fmt.Errorf("finam executor: order %q rejected with status %s", orderID, response.Status)
	case finam.OrderStatusNew, finam.OrderStatusPendingNew:
		return Fill{}, fmt.Errorf("finam executor: order %q was accepted but not filled", orderID)
	default:
		return Fill{}, fmt.Errorf("finam executor: order %q has unsupported status %s", orderID, response.Status)
	}
}

func finamClientOrderID(orderID string) string {
	compact := strings.ReplaceAll(orderID, "-", "")
	if len(compact) <= finam.MaxClientOrderIDLength {
		return compact
	}
	return compact[:finam.MaxClientOrderIDLength]
}

func validateFinamInput(signal domain.TradeSignal, price decimal.Decimal) error {
	if err := signal.Validate(); err != nil {
		return fmt.Errorf("finam executor: invalid signal: %w", err)
	}
	if signal.TargetLots < 0 {
		return fmt.Errorf("finam executor: target lots must be non-negative")
	}
	if price.Sign() <= 0 {
		return fmt.Errorf("finam executor: order price must be positive")
	}
	return nil
}

func finamSide(action domain.Action) (finam.Side, error) {
	switch action {
	case domain.ActionBuy:
		return finam.SideBuy, nil
	case domain.ActionSell:
		return finam.SideSell, nil
	default:
		return finam.SideUnspecified, fmt.Errorf("finam executor: cannot place order for action %q", action)
	}
}

func (f *FinamExecutor) record(ctx context.Context, fill Fill) error {
	payload, err := json.Marshal(fill)
	if err != nil {
		return fmt.Errorf("finam executor: marshal fill: %w", err)
	}
	event := domain.AuditEvent{
		ID:        fill.ID,
		Ticker:    fill.Ticker,
		Stage:     "executor",
		Payload:   string(payload),
		CreatedAt: fill.ExecutedAt,
	}
	if err := f.store.UpsertAuditEvent(ctx, event); err != nil {
		return fmt.Errorf("finam executor: persist fill: %w", err)
	}
	return nil
}
