package executor

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

type fakePositionReader struct {
	mu   sync.Mutex
	lots map[string]int
}

func (f *fakePositionReader) set(ticker string, lots int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.lots == nil {
		f.lots = make(map[string]int)
	}
	f.lots[ticker] = lots
}

func (f *fakePositionReader) CurrentLots(_ context.Context, ticker string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lots[ticker], nil
}

type recordingExecutor struct {
	mu      sync.Mutex
	signals []domain.TradeSignal
	fill    Fill
}

func (r *recordingExecutor) Execute(_ context.Context, signal domain.TradeSignal, _ decimal.Decimal) (Fill, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.signals = append(r.signals, signal)
	return r.fill, nil
}

func TestTargetPositionExecutor_TradesOnlyDelta(t *testing.T) {
	now := time.Date(2024, 2, 11, 10, 30, 0, 0, time.UTC)
	positions := &fakePositionReader{}
	inner := &recordingExecutor{fill: Fill{Lots: 1}}
	exec := NewTargetPositionExecutor(inner, positions, func() time.Time { return now })

	ctx := context.Background()

	positions.set("SBER", 0)
	_, err := exec.Execute(ctx, domain.TradeSignal{Ticker: "SBER", Action: domain.ActionBuy, TargetLots: 5}, decimal.NewFromInt(100))
	if err != nil {
		t.Fatal(err)
	}
	if len(inner.signals) != 1 {
		t.Fatalf("expected 1 signal after empty position, got %d", len(inner.signals))
	}
	if s := inner.signals[0]; s.Action != domain.ActionBuy || s.TargetLots != 5 {
		t.Fatalf("expected BUY 5, got %v lots=%d", s.Action, s.TargetLots)
	}

	positions.set("SBER", 5)
	inner.signals = nil
	fill, err := exec.Execute(ctx, domain.TradeSignal{Ticker: "SBER", Action: domain.ActionBuy, TargetLots: 5}, decimal.NewFromInt(110))
	if err != nil {
		t.Fatal(err)
	}
	if len(inner.signals) != 0 {
		t.Fatalf("expected no inner call when already at target, got %d", len(inner.signals))
	}
	if fill.Action != domain.ActionHold || fill.Lots != 0 {
		t.Fatalf("expected HOLD noop fill, got %v lots=%d", fill.Action, fill.Lots)
	}

	positions.set("SBER", 5)
	inner.signals = nil
	_, err = exec.Execute(ctx, domain.TradeSignal{Ticker: "SBER", Action: domain.ActionSell, TargetLots: 2}, decimal.NewFromInt(90))
	if err != nil {
		t.Fatal(err)
	}
	if len(inner.signals) != 1 {
		t.Fatalf("expected 1 signal when target flips below current, got %d", len(inner.signals))
	}
	if s := inner.signals[0]; s.Action != domain.ActionSell || s.TargetLots != 7 {
		t.Fatalf("expected SELL 7 (close 5 + short 2), got %v lots=%d", s.Action, s.TargetLots)
	}

	positions.set("SBER", -2)
	inner.signals = nil
	_, err = exec.Execute(ctx, domain.TradeSignal{Ticker: "SBER", Action: domain.ActionSell, TargetLots: 2}, decimal.NewFromInt(85))
	if err != nil {
		t.Fatal(err)
	}
	if len(inner.signals) != 0 {
		t.Fatalf("expected no inner call when short already at target, got %d", len(inner.signals))
	}

	positions.set("SBER", 0)
	inner.signals = nil
	_, err = exec.Execute(ctx, domain.TradeSignal{Ticker: "SBER", Action: domain.ActionHold}, decimal.NewFromInt(95))
	if err != nil {
		t.Fatal(err)
	}
	if len(inner.signals) != 1 {
		t.Fatalf("expected HOLD passthrough to inner, got %d", len(inner.signals))
	}
}

func TestTargetPositionExecutor_ShortBuildsUp(t *testing.T) {
	now := time.Date(2024, 2, 11, 10, 30, 0, 0, time.UTC)
	positions := &fakePositionReader{}
	inner := &recordingExecutor{fill: Fill{Lots: 1}}
	exec := NewTargetPositionExecutor(inner, positions, func() time.Time { return now })
	ctx := context.Background()

	positions.set("SBER", 3)
	_, err := exec.Execute(ctx, domain.TradeSignal{Ticker: "SBER", Action: domain.ActionSell, TargetLots: 3}, decimal.NewFromInt(90))
	if err != nil {
		t.Fatal(err)
	}
	if len(inner.signals) != 1 {
		t.Fatalf("expected 1 signal when flipping long 3 to short 3, got %d", len(inner.signals))
	}
	if s := inner.signals[0]; s.Action != domain.ActionSell || s.TargetLots != 6 {
		t.Fatalf("expected SELL 6 (close long 3 + open short 3), got %v lots=%d", s.Action, s.TargetLots)
	}
}
