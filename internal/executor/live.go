package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	pb "github.com/tinkoff/invest-api-go-sdk/proto"

	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/storage"
)

const quotationScale = int64(1_000_000_000)

type OrderPoster interface {
	PostOrder(ctx context.Context, request *pb.PostOrderRequest) (*pb.PostOrderResponse, error)
}

type orderResult struct {
	fill Fill
	err  error
}

type liveOrderIntent struct {
	Status      string          `json:"status"`
	Ticker      string          `json:"ticker"`
	Action      domain.Action   `json:"action"`
	TargetLots  int             `json:"target_lots"`
	Price       decimal.Decimal `json:"price"`
	Message     string          `json:"message,omitempty"`
	SubmittedAt time.Time       `json:"submitted_at"`
}

type persistedOrder struct {
	Ticker     string          `json:"ticker"`
	Action     domain.Action   `json:"action"`
	Lots       int             `json:"lots"`
	Price      decimal.Decimal `json:"price"`
	Commission decimal.Decimal `json:"commission"`
	ExecutedAt time.Time       `json:"executed_at"`
	Status     string          `json:"status"`
}

type InstrumentIDResolver func(ticker string) (string, error)

type LiveConfig struct {
	AccountID           string
	ResolveInstrumentID InstrumentIDResolver
	OrderType           pb.OrderType
	Store               *storage.Store
	Now                 func() time.Time
	CommissionRate      decimal.Decimal
}

type LiveExecutor struct {
	orders         OrderPoster
	accountID      string
	resolve        InstrumentIDResolver
	orderType      pb.OrderType
	store          *storage.Store
	now            func() time.Time
	commissionRate decimal.Decimal

	mu      sync.Mutex
	sent    map[string]Fill
	pending map[string]chan struct{}
	results map[string]orderResult
}

func NewLiveExecutor(orders OrderPoster, cfg LiveConfig) (*LiveExecutor, error) {
	if orders == nil {
		return nil, fmt.Errorf("live executor: order poster is nil")
	}
	accountID := strings.TrimSpace(cfg.AccountID)
	if accountID == "" {
		return nil, fmt.Errorf("live executor: account id must not be empty")
	}
	if cfg.ResolveInstrumentID == nil {
		return nil, fmt.Errorf("live executor: instrument id resolver must not be nil")
	}
	if cfg.Store == nil {
		return nil, fmt.Errorf("live executor: storage store is nil")
	}
	orderType := cfg.OrderType
	if orderType == pb.OrderType_ORDER_TYPE_UNSPECIFIED {
		orderType = pb.OrderType_ORDER_TYPE_LIMIT
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &LiveExecutor{
		orders:         orders,
		accountID:      accountID,
		resolve:        cfg.ResolveInstrumentID,
		orderType:      orderType,
		store:          cfg.Store,
		now:            now,
		commissionRate: cfg.CommissionRate,
		sent:           make(map[string]Fill),
		pending:        make(map[string]chan struct{}),
		results:        make(map[string]orderResult),
	}, nil
}

func (l *LiveExecutor) Execute(ctx context.Context, signal domain.TradeSignal, price decimal.Decimal) (Fill, error) {
	if err := validateLiveInput(signal, price); err != nil {
		return Fill{}, err
	}
	if signal.Action == domain.ActionHold {
		fill := Fill{
			ID:         uuid.NewString(),
			Ticker:     signal.Ticker,
			Action:     signal.Action,
			Lots:       0,
			Price:      price,
			ExecutedAt: l.now(),
		}
		if err := l.record(ctx, fill); err != nil {
			return Fill{}, err
		}
		return fill, nil
	}
	return l.ExecuteWithOrderID(ctx, signal, price, uuid.NewString())
}

func (l *LiveExecutor) ExecuteWithOrderID(ctx context.Context, signal domain.TradeSignal, price decimal.Decimal, orderID string) (Fill, error) {
	if err := validateLiveInput(signal, price); err != nil {
		return Fill{}, err
	}
	if signal.Action == domain.ActionHold {
		fill := Fill{
			ID:         orderID,
			Ticker:     signal.Ticker,
			Action:     signal.Action,
			Lots:       0,
			Price:      price,
			ExecutedAt: l.now(),
		}
		if err := l.record(ctx, fill); err != nil {
			return Fill{}, err
		}
		return fill, nil
	}

	parsedOrderID, err := uuid.Parse(strings.TrimSpace(orderID))
	if err != nil {
		return Fill{}, fmt.Errorf("live executor: invalid order id: %w", err)
	}
	if parsedOrderID.Version() != 4 {
		return Fill{}, fmt.Errorf("live executor: order id must be a v4 UUID")
	}

	l.mu.Lock()
	if fill, ok := l.sent[orderID]; ok {
		l.mu.Unlock()
		return fill, nil
	}
	if done, ok := l.pending[orderID]; ok {
		l.mu.Unlock()
		return l.waitPending(ctx, orderID, done)
	}
	l.mu.Unlock()

	if fill, status, found, err := l.loadPersistedOrder(ctx, orderID); err != nil {
		return Fill{}, err
	} else if found {
		if status == "partially_filled" {
			return Fill{}, fmt.Errorf("live executor: order %q is partially filled and requires reconciliation", orderID)
		}
		if status == "submitted" || status == "requires_reconciliation" {
			return Fill{}, fmt.Errorf("live executor: order %q requires reconciliation", orderID)
		}
		if isPersistedFill(fill) {
			return fill, nil
		}
		if status != "submitted" {
			return Fill{}, fmt.Errorf("live executor: order %q was already submitted and has no recorded fill", orderID)
		}
	}

	l.mu.Lock()
	if fill, ok := l.sent[orderID]; ok {
		l.mu.Unlock()
		return fill, nil
	}
	if done, ok := l.pending[orderID]; ok {
		l.mu.Unlock()
		return l.waitPending(ctx, orderID, done)
	}
	done := make(chan struct{})
	l.pending[orderID] = done
	l.mu.Unlock()

	fill, err, posted := l.placeOrder(ctx, signal, price, orderID)
	l.mu.Lock()
	l.results[orderID] = orderResult{fill: fill, err: err}
	if err == nil || posted {
		l.sent[orderID] = fill
	}
	close(done)
	delete(l.pending, orderID)
	l.mu.Unlock()
	return fill, err
}

func (l *LiveExecutor) waitPending(ctx context.Context, orderID string, done chan struct{}) (Fill, error) {
	select {
	case <-done:
	case <-ctx.Done():
		return Fill{}, ctx.Err()
	}
	l.mu.Lock()
	result := l.results[orderID]
	l.mu.Unlock()
	return result.fill, result.err
}

func (l *LiveExecutor) placeOrder(ctx context.Context, signal domain.TradeSignal, price decimal.Decimal, orderID string) (Fill, error, bool) {
	instrumentID, err := l.resolve(signal.Ticker)
	if err != nil {
		return Fill{}, fmt.Errorf("live executor: resolve instrument id for %q: %w", signal.Ticker, err), false
	}
	instrumentID = strings.TrimSpace(instrumentID)
	if instrumentID == "" {
		return Fill{}, fmt.Errorf("live executor: instrument id for %q must not be empty", signal.Ticker), false
	}

	request, err := l.orderRequest(signal, price, orderID, instrumentID)
	if err != nil {
		return Fill{}, err, false
	}

	if err := l.recordIntent(ctx, signal, price, orderID); err != nil {
		return Fill{}, err, false
	}

	response, err := l.orders.PostOrder(ctx, request)
	if err != nil {
		return Fill{}, fmt.Errorf("live executor: post order %q: %w", orderID, err), false
	}
	if response == nil {
		return Fill{}, fmt.Errorf("live executor: post order %q: nil response", orderID), false
	}

	status := response.GetExecutionReportStatus()
	switch status {
	case pb.OrderExecutionReportStatus_EXECUTION_REPORT_STATUS_FILL:
		lots := int(response.GetLotsExecuted())
		if lots <= 0 {
			return Fill{}, fmt.Errorf("live executor: order %q reported %s with zero executed lots", orderID, status), false
		}
		fillPrice := executedOrderPrice(response, price)
		fill := Fill{
			ID:         orderID,
			Ticker:     signal.Ticker,
			Action:     signal.Action,
			Lots:       lots,
			Price:      fillPrice,
			Commission: commissionAmount(fillPrice, lots, l.commissionRate),
			ExecutedAt: l.now(),
		}
		if err := l.record(ctx, fill); err != nil {
			return fill, err, true
		}
		return fill, nil, true
	case pb.OrderExecutionReportStatus_EXECUTION_REPORT_STATUS_PARTIALLYFILL:
		lots := int(response.GetLotsExecuted())
		if lots <= 0 {
			return Fill{}, fmt.Errorf("live executor: order %q reported partially filled with zero executed lots", orderID), false
		}
		if err := l.recordPartialFill(ctx, signal, executedOrderPrice(response, price), orderID, lots); err != nil {
			return Fill{}, fmt.Errorf("live executor: order %q partially filled; record partial fill: %w", orderID, err), false
		}
		return Fill{}, fmt.Errorf("live executor: order %q partially filled: %d of %d lots executed", orderID, lots, response.GetLotsRequested()), false
	case pb.OrderExecutionReportStatus_EXECUTION_REPORT_STATUS_REJECTED:
		message := response.GetMessage()
		if err := l.recordOrderStatus(ctx, signal, price, orderID, "rejected", message); err != nil {
			return Fill{}, fmt.Errorf("live executor: order %q rejected: %s; record status: %w", orderID, message, err), false
		}
		return Fill{}, fmt.Errorf("live executor: order %q rejected: %s", orderID, message), false
	case pb.OrderExecutionReportStatus_EXECUTION_REPORT_STATUS_CANCELLED:
		if err := l.recordOrderStatus(ctx, signal, price, orderID, "cancelled", ""); err != nil {
			return Fill{}, fmt.Errorf("live executor: order %q cancelled; record status: %w", orderID, err), false
		}
		return Fill{}, fmt.Errorf("live executor: order %q was cancelled", orderID), false
	case pb.OrderExecutionReportStatus_EXECUTION_REPORT_STATUS_NEW:
		if err := l.recordOrderStatus(ctx, signal, price, orderID, "new", ""); err != nil {
			return Fill{}, fmt.Errorf("live executor: order %q accepted but not filled; record status: %w", orderID, err), false
		}
		return Fill{}, fmt.Errorf("live executor: order %q was accepted but not filled", orderID), false
	default:
		if err := l.recordOrderStatus(ctx, signal, price, orderID, "unspecified", ""); err != nil {
			return Fill{}, fmt.Errorf("live executor: order %q has status %s; record status: %w", orderID, status, err), false
		}
		return Fill{}, fmt.Errorf("live executor: order %q has unsupported execution report status %s", orderID, status), false
	}
}

func validateLiveInput(signal domain.TradeSignal, price decimal.Decimal) error {
	if err := signal.Validate(); err != nil {
		return fmt.Errorf("live executor: invalid signal: %w", err)
	}
	if signal.TargetLots < 0 {
		return fmt.Errorf("live executor: target lots must be non-negative")
	}
	if signal.Action != domain.ActionHold && signal.TargetLots == 0 {
		return fmt.Errorf("live executor: target lots must be positive for %s", signal.Action)
	}
	if price.Sign() <= 0 {
		return fmt.Errorf("live executor: order price must be positive")
	}
	return nil
}

func (l *LiveExecutor) orderRequest(signal domain.TradeSignal, price decimal.Decimal, orderID, instrumentID string) (*pb.PostOrderRequest, error) {
	direction := pb.OrderDirection_ORDER_DIRECTION_UNSPECIFIED
	switch signal.Action {
	case domain.ActionBuy:
		direction = pb.OrderDirection_ORDER_DIRECTION_BUY
	case domain.ActionSell:
		direction = pb.OrderDirection_ORDER_DIRECTION_SELL
	default:
		return nil, fmt.Errorf("live executor: cannot place order for action %q", signal.Action)
	}

	return &pb.PostOrderRequest{
		AccountId:    l.accountID,
		InstrumentId: instrumentID,
		Quantity:     int64(signal.TargetLots),
		Price:        decimalToQuotation(price),
		Direction:    direction,
		OrderType:    l.orderType,
		OrderId:      orderID,
	}, nil
}

func (l *LiveExecutor) record(ctx context.Context, fill Fill) error {
	payload, err := json.Marshal(fill)
	if err != nil {
		return fmt.Errorf("live executor: marshal fill: %w", err)
	}
	event := domain.AuditEvent{
		ID:        fill.ID,
		Ticker:    fill.Ticker,
		Stage:     "executor",
		Payload:   string(payload),
		CreatedAt: fill.ExecutedAt,
	}
	if err := l.store.UpsertAuditEvent(ctx, event); err != nil {
		return fmt.Errorf("live executor: persist fill: %w", err)
	}
	return nil
}

func (l *LiveExecutor) recordIntent(ctx context.Context, signal domain.TradeSignal, price decimal.Decimal, orderID string) error {
	now := l.now()
	payload, err := json.Marshal(liveOrderIntent{
		Status:      "requires_reconciliation",
		Ticker:      signal.Ticker,
		Action:      signal.Action,
		TargetLots:  signal.TargetLots,
		Price:       price,
		SubmittedAt: now,
	})
	if err != nil {
		return fmt.Errorf("live executor: marshal order intent: %w", err)
	}
	event := domain.AuditEvent{
		ID:        orderID,
		Ticker:    signal.Ticker,
		Stage:     "executor",
		Payload:   string(payload),
		CreatedAt: now,
	}
	if err := l.store.UpsertAuditEvent(ctx, event); err != nil {
		return fmt.Errorf("live executor: persist order intent: %w", err)
	}
	return nil
}

func (l *LiveExecutor) recordOrderStatus(ctx context.Context, signal domain.TradeSignal, price decimal.Decimal, orderID, status, message string) error {
	payload, err := json.Marshal(liveOrderIntent{
		Status:      status,
		Ticker:      signal.Ticker,
		Action:      signal.Action,
		TargetLots:  signal.TargetLots,
		Price:       price,
		Message:     message,
		SubmittedAt: l.now(),
	})
	if err != nil {
		return fmt.Errorf("live executor: marshal order status: %w", err)
	}
	event := domain.AuditEvent{
		ID:        orderID,
		Ticker:    signal.Ticker,
		Stage:     "executor",
		Payload:   string(payload),
		CreatedAt: l.now(),
	}
	if err := l.store.UpsertAuditEvent(ctx, event); err != nil {
		return fmt.Errorf("live executor: persist order status: %w", err)
	}
	return nil
}

func (l *LiveExecutor) recordPartialFill(ctx context.Context, signal domain.TradeSignal, price decimal.Decimal, orderID string, lots int) error {
	now := l.now()
	payload, err := json.Marshal(persistedOrder{
		Ticker:     signal.Ticker,
		Action:     signal.Action,
		Lots:       lots,
		Price:      price,
		Commission: commissionAmount(price, lots, l.commissionRate),
		ExecutedAt: now,
		Status:     "partially_filled",
	})
	if err != nil {
		return fmt.Errorf("live executor: marshal partial fill: %w", err)
	}
	event := domain.AuditEvent{
		ID:        orderID,
		Ticker:    signal.Ticker,
		Stage:     "executor",
		Payload:   string(payload),
		CreatedAt: now,
	}
	if err := l.store.UpsertAuditEvent(ctx, event); err != nil {
		return fmt.Errorf("live executor: persist partial fill: %w", err)
	}
	return nil
}

func (l *LiveExecutor) loadPersistedOrder(ctx context.Context, orderID string) (Fill, string, bool, error) {
	event, err := l.store.GetAuditEvent(ctx, orderID)
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

func isPersistedFill(fill Fill) bool {
	if fill.Action != domain.ActionBuy && fill.Action != domain.ActionSell {
		return false
	}
	if fill.Lots <= 0 || fill.Price.Sign() <= 0 || fill.ExecutedAt.IsZero() {
		return false
	}
	return true
}

func decimalToQuotation(value decimal.Decimal) *pb.Quotation {
	units := value.IntPart()
	remainder := value.Sub(decimal.NewFromInt(units))
	nano := remainder.Mul(decimal.NewFromInt(quotationScale)).Round(0).IntPart()
	return &pb.Quotation{
		Units: units,
		Nano:  int32(nano),
	}
}

func executedOrderPrice(response *pb.PostOrderResponse, fallback decimal.Decimal) decimal.Decimal {
	if response == nil {
		return fallback
	}
	converted, err := moneyValueToDecimal(response.GetExecutedOrderPrice())
	if err != nil || converted.Sign() <= 0 {
		return fallback
	}
	return converted
}

func moneyValueToDecimal(value *pb.MoneyValue) (decimal.Decimal, error) {
	if value == nil {
		return decimal.Zero, errors.New("live executor: nil money value")
	}
	whole := decimal.NewFromInt(value.GetUnits())
	fraction := decimal.NewFromInt(int64(value.GetNano())).Div(decimal.NewFromInt(quotationScale))
	return whole.Add(fraction), nil
}
