package executor

import (
	"context"
	"encoding/json"
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

type TinkoffOrderPoster struct {
	client pb.OrdersServiceClient
}

func NewTinkoffOrderPoster(client pb.OrdersServiceClient) *TinkoffOrderPoster {
	return &TinkoffOrderPoster{client: client}
}

func (p *TinkoffOrderPoster) PostOrder(ctx context.Context, request *pb.PostOrderRequest) (*pb.PostOrderResponse, error) {
	if p == nil || p.client == nil {
		return nil, fmt.Errorf("live executor: tinkoff orders client is nil")
	}
	return p.client.PostOrder(ctx, request)
}

type InstrumentIDResolver func(ticker string) (string, error)

type LiveConfig struct {
	AccountID           string
	ResolveInstrumentID InstrumentIDResolver
	OrderType           pb.OrderType
	Store               *storage.Store
	Now                 func() time.Time
}

type LiveExecutor struct {
	orders    OrderPoster
	accountID string
	resolve   InstrumentIDResolver
	orderType pb.OrderType
	store     *storage.Store
	now       func() time.Time

	mu   sync.Mutex
	sent map[string]Fill
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
		orders:    orders,
		accountID: accountID,
		resolve:   cfg.ResolveInstrumentID,
		orderType: orderType,
		store:     cfg.Store,
		now:       now,
		sent:      make(map[string]Fill),
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
	l.mu.Unlock()

	instrumentID, err := l.resolve(signal.Ticker)
	if err != nil {
		return Fill{}, fmt.Errorf("live executor: resolve instrument id for %q: %w", signal.Ticker, err)
	}
	instrumentID = strings.TrimSpace(instrumentID)
	if instrumentID == "" {
		return Fill{}, fmt.Errorf("live executor: instrument id for %q must not be empty", signal.Ticker)
	}

	request, err := l.orderRequest(signal, price, orderID, instrumentID)
	if err != nil {
		return Fill{}, err
	}

	response, err := l.orders.PostOrder(ctx, request)
	if err != nil {
		return Fill{}, fmt.Errorf("live executor: post order %q: %w", orderID, err)
	}
	if response == nil {
		return Fill{}, fmt.Errorf("live executor: post order %q: nil response", orderID)
	}
	if response.GetExecutionReportStatus() == pb.OrderExecutionReportStatus_EXECUTION_REPORT_STATUS_REJECTED {
		return Fill{}, fmt.Errorf("live executor: order %q rejected: %s", orderID, response.GetMessage())
	}

	fill := Fill{
		ID:         orderID,
		Ticker:     signal.Ticker,
		Action:     signal.Action,
		Lots:       signal.TargetLots,
		Price:      price,
		ExecutedAt: l.now(),
	}
	if executedLots := response.GetLotsExecuted(); executedLots > 0 {
		fill.Lots = int(executedLots)
	}

	if err := l.record(ctx, fill); err != nil {
		return Fill{}, err
	}

	l.mu.Lock()
	l.sent[orderID] = fill
	l.mu.Unlock()

	return fill, nil
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
	if err := l.store.InsertAuditEvent(ctx, event); err != nil {
		return fmt.Errorf("live executor: persist fill: %w", err)
	}
	return nil
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
