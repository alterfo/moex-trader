package executor

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	pb "github.com/tinkoff/invest-api-go-sdk/proto"

	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/storage"
)

type fakeOrderPoster struct {
	calls     []*pb.PostOrderRequest
	responses []*pb.PostOrderResponse
	err       error
	onPost    func()
}

func (f *fakeOrderPoster) PostOrder(ctx context.Context, request *pb.PostOrderRequest) (*pb.PostOrderResponse, error) {
	f.calls = append(f.calls, request)
	if f.onPost != nil {
		f.onPost()
	}
	if f.err != nil {
		return nil, f.err
	}
	index := len(f.calls) - 1
	if index >= len(f.responses) {
		return nil, context.DeadlineExceeded
	}
	return f.responses[index], nil
}

func newLiveExecutorForTest(t *testing.T, poster OrderPoster, now time.Time) *LiveExecutor {
	t.Helper()
	store := openTestStore(t)
	exec, err := NewLiveExecutor(poster, LiveConfig{
		AccountID: "account-1",
		ResolveInstrumentID: func(_ context.Context, ticker string) (string, error) {
			return "instrument-" + ticker, nil
		},
		Store: store,
		Now:   func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewLiveExecutor() error = %v", err)
	}
	return exec
}

func newBuySignal(now time.Time) domain.TradeSignal {
	return domain.TradeSignal{
		Ticker:      "SBER",
		Action:      domain.ActionBuy,
		Confidence:  decimal.NewFromFloat(0.8),
		TargetLots:  1,
		Reasoning:   "positive momentum",
		GeneratedAt: now.Add(-time.Second),
	}
}

func TestLiveExecutorPlacesOrder(t *testing.T) {
	now := time.Date(2024, 2, 11, 10, 30, 0, 0, time.UTC)
	poster := &fakeOrderPoster{
		responses: []*pb.PostOrderResponse{
			{
				OrderId:               "broker-order-1",
				ExecutionReportStatus: pb.OrderExecutionReportStatus_EXECUTION_REPORT_STATUS_FILL,
				LotsRequested:         1,
				LotsExecuted:          1,
			},
		},
	}
	exec := newLiveExecutorForTest(t, poster, now)
	price := decimal.NewFromFloat(270.5)

	fill, err := exec.Execute(context.Background(), newBuySignal(now), price)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if fill.Ticker != "SBER" || fill.Action != domain.ActionBuy || fill.Lots != 1 {
		t.Fatalf("unexpected fill: %+v", fill)
	}
	if !fill.Price.Equal(price) {
		t.Fatalf("fill.Price = %s, want %s", fill.Price, price)
	}
	if !fill.ExecutedAt.Equal(now) {
		t.Fatalf("fill.ExecutedAt = %v, want %v", fill.ExecutedAt, now)
	}
	parsed, err := uuid.Parse(fill.ID)
	if err != nil {
		t.Fatalf("fill.ID %q is not a UUID: %v", fill.ID, err)
	}
	if parsed.Version() != 4 {
		t.Fatalf("fill.ID UUID version = %d, want 4", parsed.Version())
	}

	if len(poster.calls) != 1 {
		t.Fatalf("PostOrder calls = %d, want 1", len(poster.calls))
	}
	request := poster.calls[0]
	if request.GetOrderId() != fill.ID {
		t.Fatalf("request OrderId = %q, want %q", request.GetOrderId(), fill.ID)
	}
	if request.GetAccountId() != "account-1" {
		t.Fatalf("request AccountId = %q, want account-1", request.GetAccountId())
	}
	if request.GetInstrumentId() != "instrument-SBER" {
		t.Fatalf("request InstrumentId = %q, want instrument-SBER", request.GetInstrumentId())
	}
	if request.GetQuantity() != 1 {
		t.Fatalf("request Quantity = %d, want 1", request.GetQuantity())
	}
	if request.GetDirection() != pb.OrderDirection_ORDER_DIRECTION_BUY {
		t.Fatalf("request Direction = %s, want BUY", request.GetDirection())
	}
	if request.GetOrderType() != pb.OrderType_ORDER_TYPE_LIMIT {
		t.Fatalf("request OrderType = %s, want LIMIT", request.GetOrderType())
	}
	if got := request.GetPrice(); got == nil || got.GetUnits() != 270 || got.GetNano() != 500000000 {
		t.Fatalf("request Price = %v, want 270.5", got)
	}

	events, err := exec.store.ListAuditEvents(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("ListAuditEvents() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("audit events = %d, want 1", len(events))
	}
	var persisted Fill
	if err := json.Unmarshal([]byte(events[0].Payload), &persisted); err != nil {
		t.Fatalf("unmarshal audit payload: %v", err)
	}
	if persisted.ID != fill.ID || persisted.Ticker != "SBER" {
		t.Fatalf("unexpected persisted fill: %+v", persisted)
	}
}

func TestLiveExecutorDuplicateOrderIDIsIdempotent(t *testing.T) {
	now := time.Date(2024, 2, 11, 10, 30, 0, 0, time.UTC)
	poster := &fakeOrderPoster{
		responses: []*pb.PostOrderResponse{
			{
				OrderId:               "broker-order-1",
				ExecutionReportStatus: pb.OrderExecutionReportStatus_EXECUTION_REPORT_STATUS_FILL,
				LotsRequested:         1,
				LotsExecuted:          1,
			},
			{
				OrderId:               "broker-order-1",
				ExecutionReportStatus: pb.OrderExecutionReportStatus_EXECUTION_REPORT_STATUS_FILL,
				LotsRequested:         1,
				LotsExecuted:          1,
			},
		},
	}
	exec := newLiveExecutorForTest(t, poster, now)
	orderID := uuid.NewString()
	price := decimal.NewFromFloat(270.5)

	first, err := exec.ExecuteWithOrderID(context.Background(), newBuySignal(now), price, orderID)
	if err != nil {
		t.Fatalf("first ExecuteWithOrderID() error = %v", err)
	}
	second, err := exec.ExecuteWithOrderID(context.Background(), newBuySignal(now), price, orderID)
	if err != nil {
		t.Fatalf("second ExecuteWithOrderID() error = %v", err)
	}
	if first.ID != orderID || second.ID != orderID {
		t.Fatalf("fill IDs = %q/%q, want %q", first.ID, second.ID, orderID)
	}
	if second.Lots != 1 {
		t.Fatalf("second fill Lots = %d, want 1", second.Lots)
	}
	if len(poster.calls) != 1 {
		t.Fatalf("PostOrder calls = %d, want 1 for duplicate order id", len(poster.calls))
	}

	events, err := exec.store.ListAuditEvents(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("ListAuditEvents() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("audit events = %d, want 1 (no double record)", len(events))
	}
}

func TestLiveExecutorDoesNotCachePostedOrderWhenAuditWriteFails(t *testing.T) {
	now := time.Date(2024, 2, 11, 10, 30, 0, 0, time.UTC)
	poster := &fakeOrderPoster{
		responses: []*pb.PostOrderResponse{
			{
				OrderId:               "broker-order-1",
				ExecutionReportStatus: pb.OrderExecutionReportStatus_EXECUTION_REPORT_STATUS_FILL,
				LotsRequested:         1,
				LotsExecuted:          1,
			},
		},
	}
	store, err := storage.Open(filepath.Join(t.TempDir(), "trader.db"))
	if err != nil {
		t.Fatalf("storage.Open() error = %v", err)
	}
	exec, err := NewLiveExecutor(poster, LiveConfig{
		AccountID: "account-1",
		ResolveInstrumentID: func(_ context.Context, ticker string) (string, error) {
			return "instrument-" + ticker, nil
		},
		Store: store,
		Now:   func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewLiveExecutor() error = %v", err)
	}
	poster.onPost = func() {
		_ = store.Close()
	}

	orderID := uuid.NewString()
	_, err = exec.ExecuteWithOrderID(context.Background(), newBuySignal(now), decimal.NewFromFloat(270.5), orderID)
	if err == nil {
		t.Fatal("expected audit write failure, got nil")
	}
	_, err = exec.ExecuteWithOrderID(context.Background(), newBuySignal(now), decimal.NewFromFloat(270.5), orderID)
	if err == nil {
		t.Fatal("second ExecuteWithOrderID() error = nil, want no cached fill when persistence failed")
	}
	if len(poster.calls) != 1 {
		t.Fatalf("PostOrder calls = %d, want 1", len(poster.calls))
	}
}

func TestLiveExecutorNewOrderReturnsPendingError(t *testing.T) {
	now := time.Date(2024, 2, 11, 10, 30, 0, 0, time.UTC)
	poster := &fakeOrderPoster{
		responses: []*pb.PostOrderResponse{
			{
				OrderId:               "broker-order-new",
				ExecutionReportStatus: pb.OrderExecutionReportStatus_EXECUTION_REPORT_STATUS_NEW,
				LotsRequested:         1,
			},
		},
	}
	exec := newLiveExecutorForTest(t, poster, now)
	orderID := uuid.NewString()

	_, err := exec.ExecuteWithOrderID(context.Background(), newBuySignal(now), decimal.NewFromFloat(270.5), orderID)
	if err == nil {
		t.Fatal("expected NEW order to return a pending error, got nil")
	}
	if !strings.Contains(err.Error(), "accepted but not filled") {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = exec.ExecuteWithOrderID(context.Background(), newBuySignal(now), decimal.NewFromFloat(270.5), orderID)
	if err == nil {
		t.Fatal("expected duplicate NEW order to avoid submitting again, got nil")
	}
	if !strings.Contains(err.Error(), "already submitted") {
		t.Fatalf("unexpected duplicate error: %v", err)
	}
	if len(poster.calls) != 1 {
		t.Fatalf("PostOrder calls = %d, want 1", len(poster.calls))
	}
}

func TestLiveExecutorCancelledOrderSurfacesError(t *testing.T) {
	now := time.Date(2024, 2, 11, 10, 30, 0, 0, time.UTC)
	poster := &fakeOrderPoster{
		responses: []*pb.PostOrderResponse{
			{
				OrderId:               "broker-order-cancelled",
				ExecutionReportStatus: pb.OrderExecutionReportStatus_EXECUTION_REPORT_STATUS_CANCELLED,
			},
		},
	}
	exec := newLiveExecutorForTest(t, poster, now)

	_, err := exec.Execute(context.Background(), newBuySignal(now), decimal.NewFromFloat(270.5))
	if err == nil {
		t.Fatal("expected cancelled order error, got nil")
	}
	if !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLiveExecutorPersistedFillIsIdempotentAfterRestart(t *testing.T) {
	now := time.Date(2024, 2, 11, 10, 30, 0, 0, time.UTC)
	store, err := storage.Open(filepath.Join(t.TempDir(), "trader.db"))
	if err != nil {
		t.Fatalf("storage.Open() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	poster := &fakeOrderPoster{
		responses: []*pb.PostOrderResponse{
			{
				OrderId:               "broker-order-1",
				ExecutionReportStatus: pb.OrderExecutionReportStatus_EXECUTION_REPORT_STATUS_FILL,
				LotsRequested:         1,
				LotsExecuted:          1,
			},
		},
	}
	first, err := NewLiveExecutor(poster, LiveConfig{
		AccountID: "account-1",
		ResolveInstrumentID: func(_ context.Context, ticker string) (string, error) {
			return "instrument-" + ticker, nil
		},
		Store: store,
		Now:   func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewLiveExecutor() error = %v", err)
	}

	orderID := uuid.NewString()
	if _, err := first.ExecuteWithOrderID(context.Background(), newBuySignal(now), decimal.NewFromFloat(270.5), orderID); err != nil {
		t.Fatalf("first ExecuteWithOrderID() error = %v", err)
	}

	restarted, err := NewLiveExecutor(&fakeOrderPoster{err: context.DeadlineExceeded}, LiveConfig{
		AccountID: "account-1",
		ResolveInstrumentID: func(_ context.Context, ticker string) (string, error) {
			return "instrument-" + ticker, nil
		},
		Store: store,
		Now:   func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewLiveExecutor() restarted error = %v", err)
	}
	fill, err := restarted.ExecuteWithOrderID(context.Background(), newBuySignal(now), decimal.NewFromFloat(270.5), orderID)
	if err != nil {
		t.Fatalf("restarted ExecuteWithOrderID() error = %v", err)
	}
	if fill.ID != orderID || fill.Lots != 1 {
		t.Fatalf("restarted fill = %+v, want persisted fill", fill)
	}
}

func TestLiveExecutorRejectedOrderSurfacesError(t *testing.T) {
	now := time.Date(2024, 2, 11, 10, 30, 0, 0, time.UTC)
	poster := &fakeOrderPoster{
		responses: []*pb.PostOrderResponse{
			{
				OrderId:               "broker-order-rejected",
				ExecutionReportStatus: pb.OrderExecutionReportStatus_EXECUTION_REPORT_STATUS_REJECTED,
				Message:               "insufficient funds",
			},
		},
	}
	exec := newLiveExecutorForTest(t, poster, now)

	_, err := exec.Execute(context.Background(), newBuySignal(now), decimal.NewFromFloat(270.5))
	if err == nil {
		t.Fatal("expected rejected order error, got nil")
	}
	if !strings.Contains(err.Error(), "rejected") || !strings.Contains(err.Error(), "insufficient funds") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLiveExecutorPostOrderErrorSurfaces(t *testing.T) {
	now := time.Date(2024, 2, 11, 10, 30, 0, 0, time.UTC)
	poster := &fakeOrderPoster{err: context.DeadlineExceeded}
	exec := newLiveExecutorForTest(t, poster, now)

	_, err := exec.Execute(context.Background(), newBuySignal(now), decimal.NewFromFloat(270.5))
	if err == nil {
		t.Fatal("expected PostOrder error, got nil")
	}
	if !strings.Contains(err.Error(), "post order") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLiveExecutorRequiresReconciliationAfterTransientPostError(t *testing.T) {
	now := time.Date(2024, 2, 11, 10, 30, 0, 0, time.UTC)
	poster := &fakeOrderPoster{err: context.DeadlineExceeded}
	exec := newLiveExecutorForTest(t, poster, now)
	orderID := uuid.NewString()
	price := decimal.NewFromFloat(270.5)
	signal := newBuySignal(now)

	if _, err := exec.ExecuteWithOrderID(context.Background(), signal, price, orderID); err == nil {
		t.Fatal("expected first PostOrder error, got nil")
	}
	_, err := exec.ExecuteWithOrderID(context.Background(), signal, price, orderID)
	if err == nil || !strings.Contains(err.Error(), "requires reconciliation") {
		t.Fatalf("second ExecuteWithOrderID() error = %v, want reconciliation error", err)
	}
	if len(poster.calls) != 1 {
		t.Fatalf("PostOrder calls = %d, want 1 without automatic retry", len(poster.calls))
	}
}

func TestLiveExecutorDoesNotRepostUncertainOrderAfterRestart(t *testing.T) {
	now := time.Date(2024, 2, 11, 10, 30, 0, 0, time.UTC)
	store, err := storage.Open(filepath.Join(t.TempDir(), "trader.db"))
	if err != nil {
		t.Fatalf("storage.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	first, err := NewLiveExecutor(&fakeOrderPoster{err: context.DeadlineExceeded}, LiveConfig{
		AccountID: "account-1",
		ResolveInstrumentID: func(_ context.Context, ticker string) (string, error) {
			return "instrument-" + ticker, nil
		},
		Store: store,
		Now:   func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewLiveExecutor() error = %v", err)
	}
	orderID := uuid.NewString()
	if _, err := first.ExecuteWithOrderID(context.Background(), newBuySignal(now), decimal.NewFromFloat(270.5), orderID); err == nil {
		t.Fatal("expected first PostOrder error, got nil")
	}

	restartedPoster := &fakeOrderPoster{
		responses: []*pb.PostOrderResponse{
			{
				OrderId:               "broker-order-1",
				ExecutionReportStatus: pb.OrderExecutionReportStatus_EXECUTION_REPORT_STATUS_FILL,
				LotsRequested:         1,
				LotsExecuted:          1,
			},
		},
	}
	restarted, err := NewLiveExecutor(restartedPoster, LiveConfig{
		AccountID: "account-1",
		ResolveInstrumentID: func(_ context.Context, ticker string) (string, error) {
			return "instrument-" + ticker, nil
		},
		Store: store,
		Now:   func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewLiveExecutor() restarted error = %v", err)
	}
	if _, err := restarted.ExecuteWithOrderID(context.Background(), newBuySignal(now), decimal.NewFromFloat(270.5), orderID); err == nil || !strings.Contains(err.Error(), "requires reconciliation") {
		t.Fatalf("restarted ExecuteWithOrderID() error = %v, want reconciliation error", err)
	}
	if len(restartedPoster.calls) != 0 {
		t.Fatalf("restarted PostOrder calls = %d, want 0", len(restartedPoster.calls))
	}
}

func TestLiveExecutorRejectsZeroLotsBuy(t *testing.T) {
	poster := &fakeOrderPoster{}
	exec := newLiveExecutorForTest(t, poster, time.Now())
	signal := newBuySignal(time.Now())
	signal.TargetLots = 0

	_, err := exec.Execute(context.Background(), signal, decimal.NewFromFloat(270.5))
	if err == nil {
		t.Fatal("expected zero-lots error, got nil")
	}
	if !strings.Contains(err.Error(), "target lots must be positive") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(poster.calls) != 0 {
		t.Fatalf("PostOrder calls = %d, want 0", len(poster.calls))
	}
}

func TestLiveExecutorMarketOrderOmitsPrice(t *testing.T) {
	now := time.Date(2024, 2, 11, 10, 30, 0, 0, time.UTC)
	poster := &fakeOrderPoster{
		responses: []*pb.PostOrderResponse{
			{
				ExecutionReportStatus: pb.OrderExecutionReportStatus_EXECUTION_REPORT_STATUS_FILL,
				LotsRequested:         1,
				LotsExecuted:          1,
				ExecutedOrderPrice:    &pb.MoneyValue{Currency: "RUB", Units: 271, Nano: 250000000},
			},
		},
	}
	store := openTestStore(t)
	exec, err := NewLiveExecutor(poster, LiveConfig{
		AccountID: "account-1",
		ResolveInstrumentID: func(_ context.Context, ticker string) (string, error) {
			return "instrument-" + ticker, nil
		},
		OrderType: pb.OrderType_ORDER_TYPE_MARKET,
		Store:     store,
		Now:       func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewLiveExecutor() error = %v", err)
	}

	fill, err := exec.Execute(context.Background(), newBuySignal(now), decimal.NewFromFloat(270.5))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(poster.calls) != 1 {
		t.Fatalf("PostOrder calls = %d, want 1", len(poster.calls))
	}
	request := poster.calls[0]
	if request.GetPrice() != nil {
		t.Fatalf("market order price = %v, want nil", request.GetPrice())
	}
	if request.GetOrderType() != pb.OrderType_ORDER_TYPE_MARKET {
		t.Fatalf("request OrderType = %s, want MARKET", request.GetOrderType())
	}
	if !fill.Price.Equal(decimal.NewFromFloat(271.25)) {
		t.Fatalf("fill.Price = %s, want 271.25", fill.Price)
	}
}

func TestNewLiveExecutorValidatesConfig(t *testing.T) {
	store := openTestStore(t)
	resolver := func(_ context.Context, ticker string) (string, error) { return ticker, nil }
	tests := []struct {
		name   string
		orders OrderPoster
		cfg    LiveConfig
	}{
		{
			name:   "nil poster",
			orders: nil,
			cfg:    LiveConfig{AccountID: "account", ResolveInstrumentID: resolver, Store: store},
		},
		{
			name:   "empty account",
			orders: &fakeOrderPoster{},
			cfg:    LiveConfig{AccountID: " ", ResolveInstrumentID: resolver, Store: store},
		},
		{
			name:   "nil resolver",
			orders: &fakeOrderPoster{},
			cfg:    LiveConfig{AccountID: "account", Store: store},
		},
		{
			name:   "nil store",
			orders: &fakeOrderPoster{},
			cfg:    LiveConfig{AccountID: "account", ResolveInstrumentID: resolver},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewLiveExecutor(tt.orders, tt.cfg); err == nil {
				t.Fatal("expected config validation error, got nil")
			}
		})
	}
}

func TestLiveExecutorUsesExecutedOrderPrice(t *testing.T) {
	now := time.Date(2024, 2, 11, 10, 30, 0, 0, time.UTC)
	poster := &fakeOrderPoster{
		responses: []*pb.PostOrderResponse{
			{
				ExecutionReportStatus: pb.OrderExecutionReportStatus_EXECUTION_REPORT_STATUS_FILL,
				LotsRequested:         1,
				LotsExecuted:          1,
				ExecutedOrderPrice:    &pb.MoneyValue{Currency: "RUB", Units: 271, Nano: 250000000},
			},
		},
	}
	exec := newLiveExecutorForTest(t, poster, now)

	fill, err := exec.Execute(context.Background(), newBuySignal(now), decimal.NewFromFloat(270.5))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := decimal.NewFromFloat(271.25)
	if !fill.Price.Equal(want) {
		t.Fatalf("fill.Price = %s, want %s", fill.Price, want)
	}
}

func TestLiveExecutorPartialFillIsNotTerminal(t *testing.T) {
	now := time.Date(2024, 2, 11, 10, 30, 0, 0, time.UTC)
	poster := &fakeOrderPoster{
		responses: []*pb.PostOrderResponse{
			{
				ExecutionReportStatus: pb.OrderExecutionReportStatus_EXECUTION_REPORT_STATUS_PARTIALLYFILL,
				LotsRequested:         2,
				LotsExecuted:          1,
			},
		},
	}
	exec := newLiveExecutorForTest(t, poster, now)
	signal := newBuySignal(now)
	signal.TargetLots = 2
	orderID := uuid.NewString()

	_, err := exec.ExecuteWithOrderID(context.Background(), signal, decimal.NewFromFloat(270.5), orderID)
	if err == nil {
		t.Fatal("expected partial fill error, got nil")
	}
	if !strings.Contains(err.Error(), "partially filled") {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = exec.ExecuteWithOrderID(context.Background(), signal, decimal.NewFromFloat(270.5), orderID)
	if err == nil {
		t.Fatal("expected retry to remain non-terminal, got nil")
	}
	if len(poster.calls) != 1 {
		t.Fatalf("PostOrder calls = %d, want 1", len(poster.calls))
	}
}

func TestLiveExecutorConcurrentWaitersShareResult(t *testing.T) {
	now := time.Date(2024, 2, 11, 10, 30, 0, 0, time.UTC)
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	poster := &fakeOrderPoster{
		responses: []*pb.PostOrderResponse{
			{
				ExecutionReportStatus: pb.OrderExecutionReportStatus_EXECUTION_REPORT_STATUS_FILL,
				LotsRequested:         1,
				LotsExecuted:          1,
			},
		},
		onPost: func() {
			entered <- struct{}{}
			<-release
		},
	}
	exec := newLiveExecutorForTest(t, poster, now)
	signal := newBuySignal(now)
	orderID := uuid.NewString()
	price := decimal.NewFromFloat(270.5)

	type result struct {
		fill Fill
		err  error
	}
	results := make(chan result, 3)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		fill, err := exec.ExecuteWithOrderID(context.Background(), signal, price, orderID)
		results <- result{fill: fill, err: err}
	}()

	<-entered

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fill, err := exec.ExecuteWithOrderID(context.Background(), signal, price, orderID)
			results <- result{fill: fill, err: err}
		}()
	}
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()
	close(results)

	if len(poster.calls) != 1 {
		t.Fatalf("PostOrder calls = %d, want 1", len(poster.calls))
	}
	count := 0
	for res := range results {
		count++
		if res.err != nil {
			t.Fatalf("ExecuteWithOrderID() error = %v", res.err)
		}
		if res.fill.ID != orderID || res.fill.Lots != 1 {
			t.Fatalf("unexpected fill: %+v", res.fill)
		}
	}
	if count != 3 {
		t.Fatalf("results = %d, want 3", count)
	}
}

func TestLiveExecutorComputesCommission(t *testing.T) {
	now := time.Date(2024, 2, 11, 10, 30, 0, 0, time.UTC)
	tests := []struct {
		name         string
		rate         string
		executedNano int32
		executedUnit int64
		want         string
	}{
		{name: "basis point rate", rate: "0.0001", executedUnit: 271, executedNano: 250000000, want: "0.027125"},
		{name: "zero rate", rate: "0", executedUnit: 271, executedNano: 250000000, want: "0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rate, err := decimal.NewFromString(tt.rate)
			if err != nil {
				t.Fatalf("rate %q: %v", tt.rate, err)
			}
			want, err := decimal.NewFromString(tt.want)
			if err != nil {
				t.Fatalf("want %q: %v", tt.want, err)
			}
			poster := &fakeOrderPoster{
				responses: []*pb.PostOrderResponse{
					{
						OrderId:               "broker-order-1",
						ExecutionReportStatus: pb.OrderExecutionReportStatus_EXECUTION_REPORT_STATUS_FILL,
						LotsRequested:         1,
						LotsExecuted:          1,
						ExecutedOrderPrice:    &pb.MoneyValue{Currency: "RUB", Units: tt.executedUnit, Nano: tt.executedNano},
					},
				},
			}
			store := openTestStore(t)
			exec, err := NewLiveExecutor(poster, LiveConfig{
				AccountID: "account-1",
				ResolveInstrumentID: func(_ context.Context, ticker string) (string, error) {
					return "instrument-" + ticker, nil
				},
				Store:          store,
				Now:            func() time.Time { return now },
				CommissionRate: rate,
			})
			if err != nil {
				t.Fatalf("NewLiveExecutor() error = %v", err)
			}
			fill, err := exec.Execute(context.Background(), newBuySignal(now), decimal.NewFromFloat(270.5))
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if !fill.Commission.Equal(want) {
				t.Fatalf("fill.Commission = %s, want %s", fill.Commission, want)
			}
		})
	}
}

func TestLiveExecutorPersistsCommission(t *testing.T) {
	now := time.Date(2024, 2, 11, 10, 30, 0, 0, time.UTC)
	poster := &fakeOrderPoster{
		responses: []*pb.PostOrderResponse{
			{
				OrderId:               "broker-order-1",
				ExecutionReportStatus: pb.OrderExecutionReportStatus_EXECUTION_REPORT_STATUS_FILL,
				LotsRequested:         1,
				LotsExecuted:          1,
				ExecutedOrderPrice:    &pb.MoneyValue{Currency: "RUB", Units: 271, Nano: 250000000},
			},
		},
	}
	store := openTestStore(t)
	exec, err := NewLiveExecutor(poster, LiveConfig{
		AccountID: "account-1",
		ResolveInstrumentID: func(_ context.Context, ticker string) (string, error) {
			return "instrument-" + ticker, nil
		},
		Store:          store,
		Now:            func() time.Time { return now },
		CommissionRate: decimal.New(1, -4),
	})
	if err != nil {
		t.Fatalf("NewLiveExecutor() error = %v", err)
	}
	fill, err := exec.Execute(context.Background(), newBuySignal(now), decimal.NewFromFloat(270.5))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	events, err := store.ListAuditEvents(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("ListAuditEvents() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("audit events = %d, want 1", len(events))
	}
	var persisted Fill
	if err := json.Unmarshal([]byte(events[0].Payload), &persisted); err != nil {
		t.Fatalf("unmarshal audit payload: %v", err)
	}
	if !persisted.Commission.Equal(fill.Commission) {
		t.Fatalf("persisted.Commission = %s, want %s", persisted.Commission, fill.Commission)
	}
}
