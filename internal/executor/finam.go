package executor

import (
	"context"
	"encoding/json"
	"errors"
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
type FinamLotSizeResolver func(symbol string) (decimal.Decimal, error)

type FinamConfig struct {
	AccountID      string
	ResolveSymbol  FinamSymbolResolver
	ResolveLotSize FinamLotSizeResolver
	Store          *storage.Store
	Now            func() time.Time
	CommissionRate decimal.Decimal
}

type FinamExecutor struct {
	client         *finam.Client
	accountID      string
	resolve        FinamSymbolResolver
	resolveLotSize FinamLotSizeResolver
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
		resolveLotSize: cfg.ResolveLotSize,
		store:          cfg.Store,
		now:            now,
		commissionRate: cfg.CommissionRate,
	}, nil
}

func (f *FinamExecutor) Execute(ctx context.Context, signal domain.TradeSignal, price decimal.Decimal) (Fill, error) {
	if err := validateFinamInput(signal, price); err != nil {
		return Fill{}, err
	}
	return f.ExecuteWithOrderID(ctx, signal, price, uuid.NewString())
}

func (f *FinamExecutor) ExecuteWithOrderID(ctx context.Context, signal domain.TradeSignal, price decimal.Decimal, orderID string) (Fill, error) {
	if err := validateFinamInput(signal, price); err != nil {
		return Fill{}, err
	}
	orderID = strings.TrimSpace(orderID)
	if signal.Action == domain.ActionHold {
		fill := Fill{
			ID:         orderID,
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

	parsedOrderID, err := uuid.Parse(orderID)
	if err != nil {
		return Fill{}, fmt.Errorf("finam executor: invalid order id: %w", err)
	}
	if parsedOrderID.Version() != 4 {
		return Fill{}, fmt.Errorf("finam executor: order id must be a v4 UUID")
	}

	if fill, status, found, err := f.loadPersistedOrder(ctx, orderID); err != nil {
		return Fill{}, err
	} else if found {
		if status == "partially_filled" {
			return Fill{}, fmt.Errorf("finam executor: order %q is partially filled and requires reconciliation", orderID)
		}
		if status == "submitted" || status == "requires_reconciliation" || status == "new" || status == "pending_new" {
			return Fill{}, fmt.Errorf("finam executor: order %q requires reconciliation", orderID)
		}
		if isPersistedFill(fill) {
			return fill, nil
		}
		return Fill{}, fmt.Errorf("finam executor: order %q was already submitted and has no recorded fill", orderID)
	}

	return f.placeOrder(ctx, signal, price, orderID)
}

func (f *FinamExecutor) placeOrder(ctx context.Context, signal domain.TradeSignal, price decimal.Decimal, orderID string) (Fill, error) {
	symbol, err := f.resolve(signal.Ticker)
	if err != nil {
		return Fill{}, fmt.Errorf("finam executor: resolve symbol for %q: %w", signal.Ticker, err)
	}
	symbol = strings.TrimSpace(symbol)
	if symbol == "" {
		return Fill{}, fmt.Errorf("finam executor: symbol for %q must not be empty", signal.Ticker)
	}

	lotSize, err := f.lotSize(ctx, symbol)
	if err != nil {
		return Fill{}, fmt.Errorf("finam executor: resolve lot size for %q: %w", signal.Ticker, err)
	}

	side, err := finamSide(signal.Action)
	if err != nil {
		return Fill{}, err
	}

	if err := f.recordIntent(ctx, signal, price, orderID); err != nil {
		return Fill{}, err
	}

	response, err := f.client.PlaceOrder(ctx, f.accountID, finam.PlaceOrderRequest{
		Symbol:        symbol,
		Quantity:      decimal.NewFromInt(int64(signal.TargetLots)).Mul(lotSize),
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
			lots, err = finamLotsFromUnits(response.ExecutedQuantity, lotSize)
			if err != nil {
				return Fill{}, fmt.Errorf("finam executor: order %q reported %s with quantity %s: %w", orderID, response.Status, response.ExecutedQuantity, err)
			}
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
		executed, err := finamLotsFromUnits(response.ExecutedQuantity, lotSize)
		if err != nil {
			return Fill{}, fmt.Errorf("finam executor: order %q reported partially filled with quantity %s: %w", orderID, response.ExecutedQuantity, err)
		}
		if executed <= 0 {
			return Fill{}, fmt.Errorf("finam executor: order %q reported partially filled with zero executed lots", orderID)
		}
		if err := f.recordPartialFill(ctx, signal, price, orderID, executed); err != nil {
			return Fill{}, fmt.Errorf("finam executor: order %q partially filled; record partial fill: %w", orderID, err)
		}
		return Fill{}, fmt.Errorf("finam executor: order %q partially filled: %d of %d lots executed", orderID, executed, signal.TargetLots)
	case finam.OrderStatusRejected, finam.OrderStatusRejectedByExch, finam.OrderStatusDeniedByBroker, finam.OrderStatusCanceled, finam.OrderStatusExpired, finam.OrderStatusFailed:
		status := finamPersistedStatus(response.Status)
		if err := f.recordOrderStatus(ctx, signal, price, orderID, status); err != nil {
			return Fill{}, fmt.Errorf("finam executor: order %q rejected with status %s; record status: %w", orderID, response.Status, err)
		}
		return Fill{}, fmt.Errorf("finam executor: order %q rejected with status %s", orderID, response.Status)
	case finam.OrderStatusNew, finam.OrderStatusPendingNew:
		if err := f.recordOrderStatus(ctx, signal, price, orderID, finamPersistedStatus(response.Status)); err != nil {
			return Fill{}, fmt.Errorf("finam executor: order %q accepted but not filled; record status: %w", orderID, err)
		}
		return Fill{}, fmt.Errorf("finam executor: order %q was accepted but not filled", orderID)
	default:
		if err := f.recordOrderStatus(ctx, signal, price, orderID, "unspecified"); err != nil {
			return Fill{}, fmt.Errorf("finam executor: order %q has unsupported status %s; record status: %w", orderID, response.Status, err)
		}
		return Fill{}, fmt.Errorf("finam executor: order %q has unsupported status %s", orderID, response.Status)
	}
}

func (f *FinamExecutor) lotSize(ctx context.Context, symbol string) (decimal.Decimal, error) {
	if f.resolveLotSize != nil {
		lotSize, err := f.resolveLotSize(symbol)
		if err != nil {
			return decimal.Zero, err
		}
		if !lotSize.IsPositive() {
			return decimal.Zero, fmt.Errorf("lot size must be positive, got %s", lotSize)
		}
		return lotSize, nil
	}

	asset, err := f.client.GetAsset(ctx, f.accountID, symbol)
	if err != nil {
		return decimal.Zero, err
	}
	return asset.LotSize, nil
}

func finamLotsFromUnits(quantity, lotSize decimal.Decimal) (int, error) {
	if !quantity.IsPositive() {
		return 0, fmt.Errorf("executed quantity must be positive, got %s", quantity)
	}
	if !lotSize.IsPositive() {
		return 0, fmt.Errorf("lot size must be positive, got %s", lotSize)
	}
	lots := quantity.Div(lotSize)
	rounded := lots.Round(0)
	if !lots.Equal(rounded) {
		return 0, fmt.Errorf("quantity %s is not a multiple of lot size %s", quantity, lotSize)
	}
	if !lots.IsPositive() {
		return 0, fmt.Errorf("quantity %s is less than lot size %s", quantity, lotSize)
	}
	return int(lots.IntPart()), nil
}

func finamPersistedStatus(status finam.OrderStatus) string {
	switch status {
	case finam.OrderStatusNew:
		return "new"
	case finam.OrderStatusPendingNew:
		return "pending_new"
	case finam.OrderStatusRejected, finam.OrderStatusRejectedByExch, finam.OrderStatusDeniedByBroker:
		return "rejected"
	case finam.OrderStatusCanceled:
		return "cancelled"
	case finam.OrderStatusExpired:
		return "expired"
	case finam.OrderStatusFailed:
		return "failed"
	case finam.OrderStatusPartiallyFilled:
		return "partially_filled"
	case finam.OrderStatusFilled, finam.OrderStatusExecuted:
		return "filled"
	default:
		return "unspecified"
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

func (f *FinamExecutor) recordIntent(ctx context.Context, signal domain.TradeSignal, price decimal.Decimal, orderID string) error {
	now := f.now()
	payload, err := json.Marshal(liveOrderIntent{
		Status:      "requires_reconciliation",
		Ticker:      signal.Ticker,
		Action:      signal.Action,
		TargetLots:  signal.TargetLots,
		Price:       price,
		SubmittedAt: now,
	})
	if err != nil {
		return fmt.Errorf("finam executor: marshal order intent: %w", err)
	}
	event := domain.AuditEvent{
		ID:        orderID,
		Ticker:    signal.Ticker,
		Stage:     "executor",
		Payload:   string(payload),
		CreatedAt: now,
	}
	if err := f.store.UpsertAuditEvent(ctx, event); err != nil {
		return fmt.Errorf("finam executor: persist order intent: %w", err)
	}
	return nil
}

func (f *FinamExecutor) recordOrderStatus(ctx context.Context, signal domain.TradeSignal, price decimal.Decimal, orderID, status string) error {
	payload, err := json.Marshal(liveOrderIntent{
		Status:      status,
		Ticker:      signal.Ticker,
		Action:      signal.Action,
		TargetLots:  signal.TargetLots,
		Price:       price,
		SubmittedAt: f.now(),
	})
	if err != nil {
		return fmt.Errorf("finam executor: marshal order status: %w", err)
	}
	event := domain.AuditEvent{
		ID:        orderID,
		Ticker:    signal.Ticker,
		Stage:     "executor",
		Payload:   string(payload),
		CreatedAt: f.now(),
	}
	if err := f.store.UpsertAuditEvent(ctx, event); err != nil {
		return fmt.Errorf("finam executor: persist order status: %w", err)
	}
	return nil
}

func (f *FinamExecutor) recordPartialFill(ctx context.Context, signal domain.TradeSignal, price decimal.Decimal, orderID string, lots int) error {
	now := f.now()
	payload, err := json.Marshal(persistedOrder{
		Ticker:     signal.Ticker,
		Action:     signal.Action,
		Lots:       lots,
		Price:      price,
		Commission: commissionAmount(price, lots, f.commissionRate),
		ExecutedAt: now,
		Status:     "partially_filled",
	})
	if err != nil {
		return fmt.Errorf("finam executor: marshal partial fill: %w", err)
	}
	event := domain.AuditEvent{
		ID:        orderID,
		Ticker:    signal.Ticker,
		Stage:     "executor",
		Payload:   string(payload),
		CreatedAt: now,
	}
	if err := f.store.UpsertAuditEvent(ctx, event); err != nil {
		return fmt.Errorf("finam executor: persist partial fill: %w", err)
	}
	return nil
}

func (f *FinamExecutor) loadPersistedOrder(ctx context.Context, orderID string) (Fill, string, bool, error) {
	event, err := f.store.GetAuditEvent(ctx, orderID)
	if err != nil {
		if errors.Is(err, storage.ErrAuditEventNotFound) {
			return Fill{}, "", false, nil
		}
		return Fill{}, "", false, err
	}

	var order persistedOrder
	if err := json.Unmarshal([]byte(event.Payload), &order); err != nil {
		return Fill{}, "", true, nil
	}
	if strings.TrimSpace(order.Ticker) == "" {
		order.Ticker = event.Ticker
	}
	fill := Fill{
		ID:         orderID,
		Ticker:     order.Ticker,
		Action:     order.Action,
		Lots:       order.Lots,
		Price:      order.Price,
		Commission: order.Commission,
		ExecutedAt: order.ExecutedAt,
	}
	return fill, order.Status, true, nil
}
