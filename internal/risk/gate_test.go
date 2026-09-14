package risk

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

func testSignal() domain.TradeSignal {
	return domain.TradeSignal{
		Ticker:      "SBER",
		Action:      domain.ActionBuy,
		Confidence:  decimal.NewFromInt(1),
		TargetLots:  1,
		Reasoning:   "test signal",
		GeneratedAt: time.Now(),
	}
}

func testRequest() Request {
	return Request{
		Signal: testSignal(),
		Market: Market{
			OrderPrice: decimal.NewFromFloat(100),
			Bid:        decimal.NewFromFloat(99.5),
			Ask:        decimal.NewFromFloat(100.5),
		},
	}
}

func newTestGate(t *testing.T) *HardenedGate {
	t.Helper()
	gate, err := NewHardenedGate(DefaultConfig())
	if err != nil {
		t.Fatalf("NewHardenedGate() error = %v", err)
	}
	return gate
}

type fakeCanceller struct {
	calls int
	err   error
}

func (f *fakeCanceller) CancelOpenOrders(ctx context.Context) error {
	f.calls++
	return f.err
}

type fakeKillSwitchStore struct {
	active bool
	calls  int
	err    error
	setErr error
}

type fakeKillSwitchAlerter struct {
	reasons []string
	err     error
}

func (f *fakeKillSwitchAlerter) KillSwitchTriggered(ctx context.Context, reason string) error {
	f.reasons = append(f.reasons, reason)
	return f.err
}

func (f *fakeKillSwitchStore) IsKillSwitchActive(ctx context.Context) (bool, error) {
	f.calls++
	return f.active, f.err
}

func (f *fakeKillSwitchStore) SetKillSwitchActive(ctx context.Context, active bool) error {
	f.active = active
	return f.setErr
}

func TestNewHardenedGateRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{name: "zero max lots", mutate: func(c *Config) { c.MaxLots = 0 }, want: "max lots"},
		{name: "zero daily loss", mutate: func(c *Config) { c.MaxDailyLossPct = decimal.Zero }, want: "daily loss"},
		{name: "zero fat finger", mutate: func(c *Config) { c.FatFingerPct = decimal.Zero }, want: "fat finger"},
		{name: "zero drawdown", mutate: func(c *Config) { c.MaxDrawdownPct = decimal.Zero }, want: "drawdown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.mutate(&cfg)
			_, err := NewHardenedGate(cfg)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestNewLotLimitGateRejectsInvalidMaxLots(t *testing.T) {
	for _, maxLots := range []int{0, -1} {
		if _, err := NewLotLimitGate(maxLots); err == nil {
			t.Fatalf("NewLotLimitGate(%d) error = nil, want error", maxLots)
		}
	}
}

func TestHardenedGateApprove(t *testing.T) {
	gate := newTestGate(t)

	tests := []struct {
		name    string
		mutate  func(*Request)
		want    bool
		wantErr string
	}{
		{name: "valid signal", want: true},
		{name: "empty ticker", mutate: func(r *Request) { r.Signal.Ticker = " " }, wantErr: "ticker must not be empty"},
		{name: "negative lots", mutate: func(r *Request) { r.Signal.TargetLots = -1 }, wantErr: "target lots must be positive for BUY/SELL"},
		{name: "zero lots buy", mutate: func(r *Request) { r.Signal.TargetLots = 0 }, wantErr: "target lots must be positive for BUY/SELL"},
		{name: "one lot at max", want: true},
		{name: "two lots above max", mutate: func(r *Request) { r.Signal.TargetLots = 2 }, want: false},
		{name: "zero lots hold", mutate: func(r *Request) {
			r.Signal.Action = domain.ActionHold
			r.Signal.TargetLots = 0
		}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := testRequest()
			if tt.mutate != nil {
				tt.mutate(&request)
			}

			approved, err := gate.Approve(context.Background(), request)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Approve() error = %v", err)
			}
			if approved != tt.want {
				t.Fatalf("Approve() = %v, want %v", approved, tt.want)
			}
		})
	}
}

func TestHardenedGateDailyLossBoundary(t *testing.T) {
	gate := newTestGate(t)

	tests := []struct {
		name        string
		dayStart    decimal.Decimal
		current     decimal.Decimal
		wantApprove bool
	}{
		{name: "loss below limit", dayStart: decimal.RequireFromString("1000"), current: decimal.RequireFromString("996"), wantApprove: true},
		{name: "loss exactly at limit", dayStart: decimal.RequireFromString("1000"), current: decimal.RequireFromString("995"), wantApprove: true},
		{name: "loss above limit", dayStart: decimal.RequireFromString("1000"), current: decimal.RequireFromString("994.99"), wantApprove: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := testRequest()
			request.Account = Account{
				Deposit:        decimal.RequireFromString("1000"),
				DayStartEquity: tt.dayStart,
				CurrentEquity:  tt.current,
			}

			approved, err := gate.Approve(context.Background(), request)
			if err != nil {
				t.Fatalf("Approve() error = %v", err)
			}
			if approved != tt.wantApprove {
				t.Fatalf("Approve() = %v, want %v", approved, tt.wantApprove)
			}
		})
	}
}

func TestHardenedGateFatFingerBoundary(t *testing.T) {
	gate := newTestGate(t)

	tests := []struct {
		name       string
		orderPrice decimal.Decimal
		want       bool
	}{
		{name: "exactly two percent above", orderPrice: decimal.RequireFromString("102"), want: true},
		{name: "just above two percent", orderPrice: decimal.RequireFromString("102.01"), want: false},
		{name: "exactly two percent below", orderPrice: decimal.RequireFromString("98"), want: true},
		{name: "just below two percent", orderPrice: decimal.RequireFromString("97.99"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := testRequest()
			request.Market = Market{
				OrderPrice: tt.orderPrice,
				Bid:        decimal.RequireFromString("100"),
				Ask:        decimal.RequireFromString("100"),
			}

			approved, err := gate.Approve(context.Background(), request)
			if err != nil {
				t.Fatalf("Approve() error = %v", err)
			}
			if approved != tt.want {
				t.Fatalf("Approve() = %v, want %v", approved, tt.want)
			}
		})
	}
}

func TestHardenedGateFatFingerUsesPrevCloseFallback(t *testing.T) {
	gate := newTestGate(t)

	request := testRequest()
	request.Market = Market{OrderPrice: decimal.NewFromInt(105), PrevClose: decimal.NewFromInt(100)}
	approved, err := gate.Approve(context.Background(), request)
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if approved {
		t.Fatal("expected rejection when order price is 5% above prev close")
	}

	request.Market = Market{OrderPrice: decimal.NewFromInt(100)}
	approved, err = gate.Approve(context.Background(), request)
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if approved {
		t.Fatal("expected rejection when no quote reference is available")
	}
}

func TestHardenedGateFatFingerRejectsNonPositiveOrderPrice(t *testing.T) {
	gate := newTestGate(t)
	request := testRequest()
	request.Market = Market{
		OrderPrice: decimal.Zero,
		Bid:        decimal.NewFromInt(100),
		Ask:        decimal.NewFromInt(100),
	}

	approved, err := gate.Approve(context.Background(), request)
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if approved {
		t.Fatal("expected rejection for non-positive order price")
	}
}

func TestHardenedGateKillSwitchOnDrawdown(t *testing.T) {
	canceller := &fakeCanceller{}
	cfg := DefaultConfig()
	cfg.Canceller = canceller
	gate, err := NewHardenedGate(cfg)
	if err != nil {
		t.Fatalf("NewHardenedGate() error = %v", err)
	}

	request := testRequest()
	request.Account = Account{
		Deposit:        decimal.RequireFromString("1000"),
		DayStartEquity: decimal.RequireFromString("969.99"),
		CurrentEquity:  decimal.RequireFromString("969.99"),
	}

	approved, err := gate.Approve(context.Background(), request)
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if approved {
		t.Fatal("expected drawdown to reject the signal")
	}
	if canceller.calls != 1 {
		t.Fatalf("cancel calls = %d, want 1", canceller.calls)
	}
	if !gate.IsKillSwitchActive() {
		t.Fatal("expected kill switch to be active")
	}

	request = testRequest()
	approved, err = gate.Approve(context.Background(), request)
	if err != nil {
		t.Fatalf("second Approve() error = %v", err)
	}
	if approved {
		t.Fatal("expected active kill switch to block the next signal")
	}
	if canceller.calls != 1 {
		t.Fatalf("cancel calls = %d, want still 1", canceller.calls)
	}
}

func TestHardenedGateDrawdownBoundary(t *testing.T) {
	canceller := &fakeCanceller{}
	cfg := DefaultConfig()
	cfg.Canceller = canceller
	gate, err := NewHardenedGate(cfg)
	if err != nil {
		t.Fatalf("NewHardenedGate() error = %v", err)
	}

	request := testRequest()
	request.Account = Account{
		Deposit:        decimal.RequireFromString("1000"),
		DayStartEquity: decimal.RequireFromString("970"),
		CurrentEquity:  decimal.RequireFromString("970"),
	}

	approved, err := gate.Approve(context.Background(), request)
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if !approved {
		t.Fatal("expected exactly 3 percent drawdown to still approve")
	}
	if gate.IsKillSwitchActive() {
		t.Fatal("expected kill switch to remain inactive at exactly 3 percent drawdown")
	}
	if canceller.calls != 0 {
		t.Fatalf("cancel calls = %d, want 0", canceller.calls)
	}
}

func TestHardenedGateKillSwitchCancellationError(t *testing.T) {
	canceller := &fakeCanceller{err: errors.New("cancel failed")}
	cfg := DefaultConfig()
	cfg.Canceller = canceller
	gate, err := NewHardenedGate(cfg)
	if err != nil {
		t.Fatalf("NewHardenedGate() error = %v", err)
	}

	request := testRequest()
	request.Account = Account{
		Deposit:        decimal.RequireFromString("1000"),
		DayStartEquity: decimal.RequireFromString("969"),
		CurrentEquity:  decimal.RequireFromString("969"),
	}

	_, err = gate.Approve(context.Background(), request)
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if !strings.Contains(err.Error(), "cancel failed") {
		t.Fatalf("expected cancellation error, got %v", err)
	}
	if !gate.IsKillSwitchActive() {
		t.Fatal("expected kill switch to stay active after cancellation error")
	}
}

func TestHardenedGateManualKillSwitch(t *testing.T) {
	gate := newTestGate(t)

	if err := gate.TripKillSwitch(context.Background()); err != nil {
		t.Fatalf("TripKillSwitch() error = %v", err)
	}
	if !gate.IsKillSwitchActive() {
		t.Fatal("expected manual kill switch to be active")
	}

	approved, err := gate.Approve(context.Background(), testRequest())
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if approved {
		t.Fatal("expected active kill switch to block the signal")
	}

	gate.ResetKillSwitch()
	if gate.IsKillSwitchActive() {
		t.Fatal("expected reset kill switch to be inactive")
	}
	approved, err = gate.Approve(context.Background(), testRequest())
	if err != nil {
		t.Fatalf("Approve() after reset error = %v", err)
	}
	if !approved {
		t.Fatal("expected approval after kill switch reset")
	}
}

func TestHardenedGatePersistsAndReadsKillSwitch(t *testing.T) {
	store := &fakeKillSwitchStore{}
	cfg := DefaultConfig()
	cfg.Store = store
	gate, err := NewHardenedGate(cfg)
	if err != nil {
		t.Fatalf("NewHardenedGate() error = %v", err)
	}

	request := testRequest()
	request.Account = Account{
		Deposit:        decimal.RequireFromString("1000"),
		DayStartEquity: decimal.RequireFromString("969"),
		CurrentEquity:  decimal.RequireFromString("969"),
	}

	approved, err := gate.Approve(context.Background(), request)
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if approved {
		t.Fatal("expected drawdown to reject the signal")
	}
	if !store.active {
		t.Fatal("expected kill switch to be persisted as active")
	}

	approved, err = gate.Approve(context.Background(), testRequest())
	if err != nil {
		t.Fatalf("second Approve() error = %v", err)
	}
	if approved {
		t.Fatal("expected persisted kill switch to block the next signal")
	}
	if store.calls < 1 {
		t.Fatalf("expected gate to read persisted kill switch state")
	}
}

func TestHardenedGateAlertsOnKillSwitch(t *testing.T) {
	alerter := &fakeKillSwitchAlerter{}
	cfg := DefaultConfig()
	cfg.Alerter = alerter
	gate, err := NewHardenedGate(cfg)
	if err != nil {
		t.Fatalf("NewHardenedGate() error = %v", err)
	}

	request := testRequest()
	request.Account = Account{
		Deposit:        decimal.RequireFromString("1000"),
		DayStartEquity: decimal.RequireFromString("969"),
		CurrentEquity:  decimal.RequireFromString("969"),
	}

	if _, err := gate.Approve(context.Background(), request); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if len(alerter.reasons) != 1 || alerter.reasons[0] != "drawdown limit exceeded" {
		t.Fatalf("alerter reasons = %v, want drawdown limit exceeded", alerter.reasons)
	}
}

func TestHardenedGateBlocksPrePersistedKillSwitch(t *testing.T) {
	store := &fakeKillSwitchStore{active: true}
	cfg := DefaultConfig()
	cfg.Store = store
	gate, err := NewHardenedGate(cfg)
	if err != nil {
		t.Fatalf("NewHardenedGate() error = %v", err)
	}

	approved, err := gate.Approve(context.Background(), testRequest())
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if approved {
		t.Fatal("expected pre-persisted kill switch to block the signal")
	}
	if !gate.IsKillSwitchActive() {
		t.Fatal("expected in-memory kill switch state to be synced from storage")
	}
}

func TestHardenedGateLocalKillSwitchBlocksAfterPersistenceFailure(t *testing.T) {
	store := &fakeKillSwitchStore{setErr: errors.New("persist failed")}
	cfg := DefaultConfig()
	cfg.Store = store
	gate, err := NewHardenedGate(cfg)
	if err != nil {
		t.Fatalf("NewHardenedGate() error = %v", err)
	}

	request := testRequest()
	request.Account = Account{
		Deposit:        decimal.RequireFromString("1000"),
		DayStartEquity: decimal.RequireFromString("969"),
		CurrentEquity:  decimal.RequireFromString("969"),
	}

	if _, err := gate.Approve(context.Background(), request); err == nil {
		t.Fatal("expected persistence failure from triggerKillSwitch")
	}
	if !gate.IsKillSwitchActive() {
		t.Fatal("expected local kill switch to stay active after persistence failure")
	}

	store.calls = 0
	approved, err := gate.Approve(context.Background(), testRequest())
	if err != nil {
		t.Fatalf("second Approve() error = %v, want blocked without a store read", err)
	}
	if approved {
		t.Fatal("expected local kill switch to block the next signal")
	}
	if store.calls != 0 {
		t.Fatalf("kill switch store calls = %d, want 0 when local flag is active", store.calls)
	}
}

func TestHardenedGateKillSwitchStillCancelsAndAlertsOnPersistenceFailure(t *testing.T) {
	store := &fakeKillSwitchStore{setErr: errors.New("persist failed")}
	canceller := &fakeCanceller{}
	alerter := &fakeKillSwitchAlerter{}
	cfg := DefaultConfig()
	cfg.Store = store
	cfg.Canceller = canceller
	cfg.Alerter = alerter
	gate, err := NewHardenedGate(cfg)
	if err != nil {
		t.Fatalf("NewHardenedGate() error = %v", err)
	}

	request := testRequest()
	request.Account = Account{
		Deposit:        decimal.RequireFromString("1000"),
		DayStartEquity: decimal.RequireFromString("969"),
		CurrentEquity:  decimal.RequireFromString("969"),
	}

	if _, err := gate.Approve(context.Background(), request); err == nil {
		t.Fatal("expected error from triggerKillSwitch")
	}
	if canceller.calls != 1 {
		t.Fatalf("CancelOpenOrders calls = %d, want 1", canceller.calls)
	}
	if len(alerter.reasons) != 1 || alerter.reasons[0] != "drawdown limit exceeded" {
		t.Fatalf("alerter reasons = %v, want drawdown limit exceeded", alerter.reasons)
	}
}
