package executor

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/storage"
)

func openTestStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "trader.db"))
	if err != nil {
		t.Fatalf("storage.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("store.Close() error = %v", err)
		}
	})
	return store
}

func TestPaperExecutorSimulatesFill(t *testing.T) {
	store := openTestStore(t)
	now := time.Date(2024, 1, 11, 12, 30, 0, 0, time.UTC)
	exec := NewPaperExecutor(store, func() time.Time { return now })

	signal := domain.TradeSignal{
		Ticker:      "SBER",
		Action:      domain.ActionBuy,
		Confidence:  decimal.NewFromFloat(0.8),
		TargetLots:  2,
		Reasoning:   "positive momentum",
		GeneratedAt: now.Add(-time.Second),
	}
	price := decimal.NewFromFloat(270.5)

	fill, err := exec.Execute(context.Background(), signal, price)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if fill.Ticker != "SBER" {
		t.Fatalf("fill.Ticker = %q, want SBER", fill.Ticker)
	}
	if fill.Action != domain.ActionBuy {
		t.Fatalf("fill.Action = %q, want BUY", fill.Action)
	}
	if fill.Lots != 2 {
		t.Fatalf("fill.Lots = %d, want 2", fill.Lots)
	}
	if !fill.Price.Equal(price) {
		t.Fatalf("fill.Price = %s, want %s", fill.Price, price)
	}
	if !fill.ExecutedAt.Equal(now) {
		t.Fatalf("fill.ExecutedAt = %v, want %v", fill.ExecutedAt, now)
	}
	if fill.ID == "" {
		t.Fatal("fill.ID is empty")
	}
}

func TestPaperExecutorPersistsFillAsAuditEvent(t *testing.T) {
	store := openTestStore(t)
	now := time.Date(2024, 1, 11, 12, 30, 0, 0, time.UTC)
	exec := NewPaperExecutor(store, func() time.Time { return now })

	signal := domain.TradeSignal{
		Ticker:      "YDEX",
		Action:      domain.ActionSell,
		Confidence:  decimal.NewFromFloat(0.6),
		TargetLots:  1,
		Reasoning:   "bearish reversal",
		GeneratedAt: now.Add(-time.Second),
	}
	price := decimal.NewFromFloat(4500.25)

	if _, err := exec.Execute(context.Background(), signal, price); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	events, err := store.ListAuditEvents(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("ListAuditEvents() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("ListAuditEvents() returned %d events, want 1", len(events))
	}

	event := events[0]
	if event.Stage != "executor" {
		t.Fatalf("event.Stage = %q, want executor", event.Stage)
	}
	if event.Ticker != "YDEX" {
		t.Fatalf("event.Ticker = %q, want YDEX", event.Ticker)
	}
	if event.ID == "" {
		t.Fatal("event.ID is empty")
	}

	var persisted Fill
	if err := json.Unmarshal([]byte(event.Payload), &persisted); err != nil {
		t.Fatalf("unmarshal fill payload: %v", err)
	}
	if !persisted.Price.Equal(price) {
		t.Fatalf("persisted.Price = %s, want %s", persisted.Price, price)
	}
	if persisted.Lots != 1 {
		t.Fatalf("persisted.Lots = %d, want 1", persisted.Lots)
	}
	if persisted.Action != domain.ActionSell {
		t.Fatalf("persisted.Action = %q, want SELL", persisted.Action)
	}
}

func TestPaperExecutorHoldSkipsFill(t *testing.T) {
	store := openTestStore(t)
	exec := NewPaperExecutor(store, time.Now)

	signal := domain.TradeSignal{
		Ticker:      "OZON",
		Action:      domain.ActionHold,
		Confidence:  decimal.NewFromFloat(0.4),
		TargetLots:  0,
		Reasoning:   "no edge",
		GeneratedAt: time.Now(),
	}

	fill, err := exec.Execute(context.Background(), signal, decimal.NewFromFloat(210))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if fill.Lots != 0 {
		t.Fatalf("fill.Lots = %d, want 0 for HOLD", fill.Lots)
	}
}

func TestPaperExecutorRejectsInvalidInputs(t *testing.T) {
	store := openTestStore(t)
	exec := NewPaperExecutor(store, time.Now)
	valid := domain.TradeSignal{
		Ticker:      "SBER",
		Action:      domain.ActionBuy,
		Confidence:  decimal.NewFromFloat(0.5),
		TargetLots:  1,
		Reasoning:   "valid",
		GeneratedAt: time.Now(),
	}

	tests := []struct {
		name    string
		signal  domain.TradeSignal
		price   decimal.Decimal
		wantErr string
	}{
		{
			name:    "empty ticker",
			signal:  func() domain.TradeSignal { s := valid; s.Ticker = " "; return s }(),
			price:   decimal.NewFromFloat(100),
			wantErr: "ticker must not be empty",
		},
		{
			name:    "unknown action",
			signal:  func() domain.TradeSignal { s := valid; s.Action = domain.Action("HODL"); return s }(),
			price:   decimal.NewFromFloat(100),
			wantErr: "action must be one of BUY, SELL, HOLD",
		},
		{
			name:    "negative lots",
			signal:  func() domain.TradeSignal { s := valid; s.TargetLots = -1; return s }(),
			price:   decimal.NewFromFloat(100),
			wantErr: "target lots must be positive for BUY/SELL",
		},
		{
			name:    "zero lots buy",
			signal:  func() domain.TradeSignal { s := valid; s.TargetLots = 0; return s }(),
			price:   decimal.NewFromFloat(100),
			wantErr: "target lots must be positive for BUY/SELL",
		},
		{
			name:    "zero price",
			signal:  valid,
			price:   decimal.Zero,
			wantErr: "fill price must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := exec.Execute(context.Background(), tt.signal, tt.price)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestPaperExecutorComputesCommission(t *testing.T) {
	tests := []struct {
		name  string
		price string
		lots  int
		rate  string
		want  string
	}{
		{name: "single lot basis point rate", price: "100", lots: 1, rate: "0.0001", want: "0.01"},
		{name: "multiple lots fractional price", price: "270.5", lots: 2, rate: "0.0001", want: "0.0541"},
		{name: "percent rate", price: "4500.25", lots: 3, rate: "0.01", want: "135.0075"},
		{name: "zero rate", price: "270.5", lots: 2, rate: "0", want: "0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := openTestStore(t)
			price, err := decimal.NewFromString(tt.price)
			if err != nil {
				t.Fatalf("price %q: %v", tt.price, err)
			}
			rate, err := decimal.NewFromString(tt.rate)
			if err != nil {
				t.Fatalf("rate %q: %v", tt.rate, err)
			}
			want, err := decimal.NewFromString(tt.want)
			if err != nil {
				t.Fatalf("want %q: %v", tt.want, err)
			}
			exec := NewPaperExecutorWithCommission(store, time.Now, rate)
			signal := domain.TradeSignal{
				Ticker:      "SBER",
				Action:      domain.ActionBuy,
				Confidence:  decimal.NewFromFloat(0.8),
				TargetLots:  tt.lots,
				Reasoning:   "commission test",
				GeneratedAt: time.Now(),
			}
			fill, err := exec.Execute(context.Background(), signal, price)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if !fill.Commission.Equal(want) {
				t.Fatalf("fill.Commission = %s, want %s", fill.Commission, want)
			}
		})
	}
}

func TestPaperExecutorHoldHasZeroCommission(t *testing.T) {
	store := openTestStore(t)
	exec := NewPaperExecutorWithCommission(store, time.Now, decimal.New(1, -4))
	signal := domain.TradeSignal{
		Ticker:      "SBER",
		Action:      domain.ActionHold,
		Confidence:  decimal.NewFromFloat(0.4),
		TargetLots:  0,
		Reasoning:   "hold",
		GeneratedAt: time.Now(),
	}
	fill, err := exec.Execute(context.Background(), signal, decimal.New(100, 0))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !fill.Commission.IsZero() {
		t.Fatalf("fill.Commission = %s, want 0", fill.Commission)
	}
}
