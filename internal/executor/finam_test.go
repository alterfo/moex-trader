package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/finam"
)

type finamOrderRequest struct {
	Symbol   string `json:"symbol"`
	Quantity struct {
		Value string `json:"value"`
	} `json:"quantity"`
	Side          string `json:"side"`
	Type          string `json:"type"`
	TimeInForce   string `json:"time_in_force"`
	ClientOrderID string `json:"client_order_id"`
}

type finamOrderResponse struct {
	OrderID          string `json:"order_id"`
	Status           string `json:"status"`
	ExecutedQuantity struct {
		Value string `json:"value"`
	} `json:"executed_quantity"`
}

func writeFinamJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func newFinamExecutorForTest(t *testing.T, server *httptest.Server, now time.Time, rate string) *FinamExecutor {
	t.Helper()
	client, err := finam.New(context.Background(), finam.Config{BaseURL: server.URL, SecretToken: "secret"})
	if err != nil {
		t.Fatalf("finam.New() error = %v", err)
	}
	commissionRate, err := decimal.NewFromString(rate)
	if err != nil {
		t.Fatalf("commission rate %q: %v", rate, err)
	}
	exec, err := NewFinamExecutor(client, FinamConfig{
		AccountID: "account-1",
		ResolveSymbol: func(ticker string) (string, error) {
			return ticker + "@MISX", nil
		},
		ResolveLotSize: func(symbol string) (decimal.Decimal, error) {
			return decimal.NewFromInt(1), nil
		},
		Store:          openTestStore(t),
		Now:            func() time.Time { return now },
		CommissionRate: commissionRate,
	})
	if err != nil {
		t.Fatalf("NewFinamExecutor() error = %v", err)
	}
	return exec
}

func TestFinamExecutorPlacesMarketOrder(t *testing.T) {
	now := time.Date(2024, 2, 11, 10, 30, 0, 0, time.UTC)
	var orderCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sessions":
			writeFinamJSON(w, http.StatusOK, map[string]string{"token": "jwt-1"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/accounts/account-1/orders":
			orderCalls.Add(1)
			if got := r.Header.Get("Authorization"); got != "Bearer jwt-1" {
				t.Errorf("Authorization = %q, want Bearer jwt-1", got)
			}
			var request finamOrderRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode order request: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			if request.Symbol != "SBER@MISX" {
				t.Errorf("symbol = %q, want SBER@MISX", request.Symbol)
			}
			if request.Quantity.Value != "2" {
				t.Errorf("quantity = %q, want 2", request.Quantity.Value)
			}
			if request.Side != string(finam.SideBuy) {
				t.Errorf("side = %q, want SIDE_BUY", request.Side)
			}
			if request.Type != string(finam.OrderTypeMarket) {
				t.Errorf("type = %q, want ORDER_TYPE_MARKET", request.Type)
			}
			if request.TimeInForce != string(finam.TimeInForceDay) {
				t.Errorf("time_in_force = %q, want TIME_IN_FORCE_DAY", request.TimeInForce)
			}
			if request.ClientOrderID == "" {
				t.Error("client_order_id is empty")
			}
			if len(request.ClientOrderID) > finam.MaxClientOrderIDLength {
				t.Errorf("client_order_id length = %d, want <= %d", len(request.ClientOrderID), finam.MaxClientOrderIDLength)
			}
			response := finamOrderResponse{OrderID: "broker-order-1", Status: string(finam.OrderStatusFilled)}
			response.ExecutedQuantity.Value = "2"
			writeFinamJSON(w, http.StatusOK, response)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	exec := newFinamExecutorForTest(t, server, now, "0.0001")
	price := decimal.NewFromFloat(270.5)
	signal := domain.TradeSignal{
		Ticker:      "SBER",
		Action:      domain.ActionBuy,
		Confidence:  decimal.NewFromFloat(0.8),
		TargetLots:  2,
		Reasoning:   "positive momentum",
		GeneratedAt: now.Add(-time.Second),
	}
	fill, err := exec.Execute(context.Background(), signal, price)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if fill.Ticker != "SBER" || fill.Action != domain.ActionBuy || fill.Lots != 2 {
		t.Fatalf("unexpected fill: %+v", fill)
	}
	if !fill.Price.Equal(price) {
		t.Fatalf("fill.Price = %s, want %s", fill.Price, price)
	}
	wantCommission := decimal.NewFromFloat(0.0541)
	if !fill.Commission.Equal(wantCommission) {
		t.Fatalf("fill.Commission = %s, want %s", fill.Commission, wantCommission)
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
	if orderCalls.Load() != 1 {
		t.Fatalf("order calls = %d, want 1", orderCalls.Load())
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
	if !persisted.Commission.Equal(fill.Commission) {
		t.Fatalf("persisted.Commission = %s, want %s", persisted.Commission, fill.Commission)
	}
}

func TestFinamExecutorRejectedOrderSurfacesError(t *testing.T) {
	now := time.Date(2024, 2, 11, 10, 30, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sessions":
			writeFinamJSON(w, http.StatusOK, map[string]string{"token": "jwt-1"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/accounts/account-1/orders":
			writeFinamJSON(w, http.StatusBadRequest, map[string]string{"message": "insufficient funds"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	exec := newFinamExecutorForTest(t, server, now, "0.0001")
	_, err := exec.Execute(context.Background(), newBuySignal(now), decimal.NewFromFloat(270.5))
	if err == nil {
		t.Fatal("expected rejected order error, got nil")
	}
	if !strings.Contains(err.Error(), "insufficient funds") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFinamExecutorRefreshesJWTOnUnauthorizedOrder(t *testing.T) {
	now := time.Date(2024, 2, 11, 10, 30, 0, 0, time.UTC)
	var authCalls atomic.Int32
	var orderCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sessions":
			call := authCalls.Add(1)
			writeFinamJSON(w, http.StatusOK, map[string]string{"token": fmt.Sprintf("jwt-%d", call)})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/accounts/account-1/orders":
			call := orderCalls.Add(1)
			if call == 1 {
				if got := r.Header.Get("Authorization"); got != "Bearer jwt-1" {
					t.Errorf("first Authorization = %q, want Bearer jwt-1", got)
				}
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if got := r.Header.Get("Authorization"); got != "Bearer jwt-2" {
				t.Errorf("second Authorization = %q, want Bearer jwt-2", got)
			}
			response := finamOrderResponse{OrderID: "broker-order-1", Status: string(finam.OrderStatusFilled)}
			response.ExecutedQuantity.Value = "1"
			writeFinamJSON(w, http.StatusOK, response)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	exec := newFinamExecutorForTest(t, server, now, "0.0001")
	fill, err := exec.Execute(context.Background(), newBuySignal(now), decimal.NewFromFloat(270.5))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if fill.Lots != 1 {
		t.Fatalf("fill.Lots = %d, want 1", fill.Lots)
	}
	if authCalls.Load() != 2 {
		t.Fatalf("auth calls = %d, want 2", authCalls.Load())
	}
	if orderCalls.Load() != 2 {
		t.Fatalf("order calls = %d, want 2", orderCalls.Load())
	}
}

func TestFinamExecutorRecordsIntentBeforePlacingOrder(t *testing.T) {
	now := time.Date(2024, 2, 11, 10, 30, 0, 0, time.UTC)
	var exec *FinamExecutor
	var orderCalls atomic.Int32
	var intentEvents atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sessions":
			writeFinamJSON(w, http.StatusOK, map[string]string{"token": "jwt-1"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/accounts/account-1/orders":
			orderCalls.Add(1)
			events, err := exec.store.ListAuditEvents(context.Background(), time.Time{})
			if err == nil {
				intentEvents.Store(int32(len(events)))
			}
			http.Error(w, "gateway timeout", http.StatusGatewayTimeout)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	exec = newFinamExecutorForTest(t, server, now, "0.0001")
	_, err := exec.Execute(context.Background(), newBuySignal(now), decimal.NewFromFloat(270.5))
	if err == nil {
		t.Fatal("expected place order error, got nil")
	}
	if orderCalls.Load() != 1 {
		t.Fatalf("order calls = %d, want 1", orderCalls.Load())
	}
	if intentEvents.Load() != 1 {
		t.Fatalf("audit events before order = %d, want 1 persisted intent", intentEvents.Load())
	}
}

func TestFinamExecutorNonTerminalStatusesPersistOrderState(t *testing.T) {
	now := time.Date(2024, 2, 11, 10, 30, 0, 0, time.UTC)
	tests := []struct {
		name   string
		status finam.OrderStatus
		want   string
	}{
		{name: "new", status: finam.OrderStatusNew, want: "new"},
		{name: "pending new", status: finam.OrderStatusPendingNew, want: "pending_new"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodPost && r.URL.Path == "/v1/sessions":
					writeFinamJSON(w, http.StatusOK, map[string]string{"token": "jwt-1"})
				case r.Method == http.MethodPost && r.URL.Path == "/v1/accounts/account-1/orders":
					writeFinamJSON(w, http.StatusOK, finamOrderResponse{OrderID: "broker-order-1", Status: string(tt.status)})
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			exec := newFinamExecutorForTest(t, server, now, "0.0001")
			_, err := exec.Execute(context.Background(), newBuySignal(now), decimal.NewFromFloat(270.5))
			if err == nil {
				t.Fatal("expected non-terminal order error, got nil")
			}
			if !strings.Contains(err.Error(), "accepted but not filled") {
				t.Fatalf("unexpected error: %v", err)
			}

			events, listErr := exec.store.ListAuditEvents(context.Background(), time.Time{})
			if listErr != nil {
				t.Fatalf("ListAuditEvents() error = %v", listErr)
			}
			if len(events) != 1 {
				t.Fatalf("audit events = %d, want 1", len(events))
			}
			var intent liveOrderIntent
			if err := json.Unmarshal([]byte(events[0].Payload), &intent); err != nil {
				t.Fatalf("unmarshal audit payload: %v", err)
			}
			if intent.Status != tt.want {
				t.Fatalf("persisted status = %q, want %q", intent.Status, tt.want)
			}
		})
	}
}

func TestFinamExecutorPartiallyFilledOrderPersistsQuantity(t *testing.T) {
	now := time.Date(2024, 2, 11, 10, 30, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sessions":
			writeFinamJSON(w, http.StatusOK, map[string]string{"token": "jwt-1"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/accounts/account-1/orders":
			response := finamOrderResponse{OrderID: "broker-order-1", Status: string(finam.OrderStatusPartiallyFilled)}
			response.ExecutedQuantity.Value = "1"
			writeFinamJSON(w, http.StatusOK, response)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	exec := newFinamExecutorForTest(t, server, now, "0.0001")
	_, err := exec.Execute(context.Background(), newBuySignal(now), decimal.NewFromFloat(270.5))
	if err == nil {
		t.Fatal("expected partial fill error, got nil")
	}
	if !strings.Contains(err.Error(), "partially filled") {
		t.Fatalf("unexpected error: %v", err)
	}

	events, listErr := exec.store.ListAuditEvents(context.Background(), time.Time{})
	if listErr != nil {
		t.Fatalf("ListAuditEvents() error = %v", listErr)
	}
	if len(events) != 1 {
		t.Fatalf("audit events = %d, want 1", len(events))
	}
	var persisted persistedOrder
	if err := json.Unmarshal([]byte(events[0].Payload), &persisted); err != nil {
		t.Fatalf("unmarshal audit payload: %v", err)
	}
	if persisted.Status != "partially_filled" {
		t.Fatalf("persisted status = %q, want partially_filled", persisted.Status)
	}
	if persisted.Lots != 1 {
		t.Fatalf("persisted lots = %d, want 1", persisted.Lots)
	}
	wantCommission := decimal.NewFromFloat(0.02705)
	if !persisted.Commission.Equal(wantCommission) {
		t.Fatalf("persisted commission = %s, want %s", persisted.Commission, wantCommission)
	}
}

func TestFinamExecutorDoesNotRepostAfterTransientPlaceOrderError(t *testing.T) {
	now := time.Date(2024, 2, 11, 10, 30, 0, 0, time.UTC)
	var orderCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sessions":
			writeFinamJSON(w, http.StatusOK, map[string]string{"token": "jwt-1"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/accounts/account-1/orders":
			orderCalls.Add(1)
			http.Error(w, "gateway timeout", http.StatusGatewayTimeout)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	exec := newFinamExecutorForTest(t, server, now, "0.0001")
	orderID := uuid.NewString()
	signal := newBuySignal(now)
	price := decimal.NewFromFloat(270.5)

	if _, err := exec.ExecuteWithOrderID(context.Background(), signal, price, orderID); err == nil {
		t.Fatal("expected first place order error, got nil")
	}
	_, err := exec.ExecuteWithOrderID(context.Background(), signal, price, orderID)
	if err == nil || !strings.Contains(err.Error(), "requires reconciliation") {
		t.Fatalf("second ExecuteWithOrderID() error = %v, want reconciliation error", err)
	}
	if orderCalls.Load() != 1 {
		t.Fatalf("order calls = %d, want 1 without automatic retry", orderCalls.Load())
	}

	events, listErr := exec.store.ListAuditEvents(context.Background(), time.Time{})
	if listErr != nil {
		t.Fatalf("ListAuditEvents() error = %v", listErr)
	}
	if len(events) != 1 {
		t.Fatalf("audit events = %d, want 1", len(events))
	}
	var intent liveOrderIntent
	if err := json.Unmarshal([]byte(events[0].Payload), &intent); err != nil {
		t.Fatalf("unmarshal audit payload: %v", err)
	}
	if intent.Status != "requires_reconciliation" {
		t.Fatalf("persisted status = %q, want requires_reconciliation", intent.Status)
	}
}

func TestFinamExecutorConvertsLotsToFinamUnits(t *testing.T) {
	now := time.Date(2024, 2, 11, 10, 30, 0, 0, time.UTC)
	var received finamOrderRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sessions":
			writeFinamJSON(w, http.StatusOK, map[string]string{"token": "jwt-1"})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/assets/SBER@MISX":
			if got := r.URL.Query().Get("account_id"); got != "account-1" {
				t.Errorf("account_id = %q, want account-1", got)
			}
			writeFinamJSON(w, http.StatusOK, map[string]any{
				"symbol":   "SBER@MISX",
				"lot_size": map[string]string{"value": "10"},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/accounts/account-1/orders":
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
				t.Errorf("decode order request: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			response := finamOrderResponse{OrderID: "broker-order-1", Status: string(finam.OrderStatusFilled)}
			response.ExecutedQuantity.Value = "10"
			writeFinamJSON(w, http.StatusOK, response)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := finam.New(context.Background(), finam.Config{BaseURL: server.URL, SecretToken: "secret"})
	if err != nil {
		t.Fatalf("finam.New() error = %v", err)
	}
	rate, err := decimal.NewFromString("0.0001")
	if err != nil {
		t.Fatalf("commission rate: %v", err)
	}
	exec, err := NewFinamExecutor(client, FinamConfig{
		AccountID: "account-1",
		ResolveSymbol: func(ticker string) (string, error) {
			return ticker + "@MISX", nil
		},
		Store:          openTestStore(t),
		Now:            func() time.Time { return now },
		CommissionRate: rate,
	})
	if err != nil {
		t.Fatalf("NewFinamExecutor() error = %v", err)
	}

	fill, err := exec.Execute(context.Background(), newBuySignal(now), decimal.NewFromFloat(270.5))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if received.Quantity.Value != "10" {
		t.Fatalf("order quantity = %q, want 10 units", received.Quantity.Value)
	}
	if fill.Lots != 1 {
		t.Fatalf("fill.Lots = %d, want 1 lot", fill.Lots)
	}
	wantCommission := decimal.NewFromFloat(0.02705)
	if !fill.Commission.Equal(wantCommission) {
		t.Fatalf("fill.Commission = %s, want %s", fill.Commission, wantCommission)
	}
}
