package executor

import (
	"context"
	"fmt"
	"io"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

type stubExecutor struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (s *stubExecutor) Execute(context.Context, domain.TradeSignal, decimal.Decimal) (Fill, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return Fill{}, s.err
}

func (s *stubExecutor) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func newCooldownForTest(t *testing.T, inner Executor, now *time.Time, threshold int, cooldown time.Duration) *RejectionCooldownExecutor {
	t.Helper()
	return NewRejectionCooldownExecutor(inner, func() time.Time { return *now }, threshold, cooldown, log.New(io.Discard, "", 0))
}

func TestRejectionCooldownSuppressesAfterThreshold(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	inner := &stubExecutor{err: fmt.Errorf("post order: rpc error: code = InvalidArgument desc = 30034")}
	exec := newCooldownForTest(t, inner, &now, 3, 15*time.Minute)
	signal := domain.TradeSignal{Ticker: "DATA", Action: domain.ActionSell, TargetLots: 169}
	price := decimal.NewFromInt(89)

	for i := 0; i < 3; i++ {
		if _, err := exec.Execute(context.Background(), signal, price); err == nil {
			t.Fatalf("call %d: error = nil, want rejection", i+1)
		}
	}
	fill, err := exec.Execute(context.Background(), signal, price)
	if err != nil {
		t.Fatalf("suppressed call error = %v, want nil", err)
	}
	if fill.Action != domain.ActionHold || fill.Lots != 0 {
		t.Fatalf("suppressed fill = %+v, want HOLD 0 lots", fill)
	}
	if inner.callCount() != 3 {
		t.Fatalf("inner calls = %d, want 3 (suppressed call must not reach inner)", inner.callCount())
	}

	now = now.Add(15*time.Minute + time.Second)
	if _, err := exec.Execute(context.Background(), signal, price); err == nil {
		t.Fatal("after cooldown error = nil, want retry to reach inner and fail")
	}
	if inner.callCount() != 4 {
		t.Fatalf("inner calls after cooldown = %d, want 4", inner.callCount())
	}
}

func TestRejectionCooldownOnlySuppressesRejectedAction(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	inner := &stubExecutor{err: fmt.Errorf("desc = 30034")}
	exec := newCooldownForTest(t, inner, &now, 1, time.Hour)
	sell := domain.TradeSignal{Ticker: "DATA", Action: domain.ActionSell, TargetLots: 169}
	buy := domain.TradeSignal{Ticker: "DATA", Action: domain.ActionBuy, TargetLots: 5}
	price := decimal.NewFromInt(89)

	if _, err := exec.Execute(context.Background(), sell, price); err == nil {
		t.Fatal("first SELL error = nil, want rejection")
	}
	if fill, err := exec.Execute(context.Background(), sell, price); err != nil || fill.Action != domain.ActionHold {
		t.Fatalf("second SELL = %+v err=%v, want suppressed HOLD", fill, err)
	}
	if _, err := exec.Execute(context.Background(), buy, price); err == nil {
		t.Fatal("BUY error = nil, want rejection to reach inner")
	}
	if inner.callCount() != 2 {
		t.Fatalf("inner calls = %d, want 2 (suppressed SELL skipped)", inner.callCount())
	}
}

func TestRejectionCooldownResetsOnSuccess(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	inner := &stubExecutor{err: fmt.Errorf("desc = 30034")}
	exec := newCooldownForTest(t, inner, &now, 3, time.Hour)
	signal := domain.TradeSignal{Ticker: "DATA", Action: domain.ActionSell, TargetLots: 169}
	price := decimal.NewFromInt(89)

	for i := 0; i < 2; i++ {
		if _, err := exec.Execute(context.Background(), signal, price); err == nil {
			t.Fatal("expected rejection")
		}
	}
	inner.err = nil
	if _, err := exec.Execute(context.Background(), signal, price); err != nil {
		t.Fatalf("successful call error = %v", err)
	}
	inner.err = fmt.Errorf("desc = 30034")
	for i := 0; i < 2; i++ {
		if _, err := exec.Execute(context.Background(), signal, price); err == nil {
			t.Fatal("expected rejection after reset, not suppression")
		}
	}
	if inner.callCount() != 5 {
		t.Fatalf("inner calls = %d, want 5 (success must reset the failure streak)", inner.callCount())
	}
}

func TestRejectionCooldownIgnoresOtherErrors(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	inner := &stubExecutor{err: fmt.Errorf("network timeout")}
	exec := newCooldownForTest(t, inner, &now, 2, time.Hour)
	signal := domain.TradeSignal{Ticker: "DATA", Action: domain.ActionSell, TargetLots: 169}
	price := decimal.NewFromInt(89)

	for i := 0; i < 5; i++ {
		if _, err := exec.Execute(context.Background(), signal, price); err == nil {
			t.Fatal("expected error")
		}
	}
	if inner.callCount() != 5 {
		t.Fatalf("inner calls = %d, want 5 (non-30034 errors must not suppress)", inner.callCount())
	}
}
