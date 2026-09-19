package main

import (
	"bytes"
	"context"
	"log"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/risk"
	"github.com/olegsidorkin/moex-trader/internal/storage"
)

func seedTestBreaker(t *testing.T, events []domain.AuditEvent) *risk.TickerBreaker {
	t.Helper()
	breaker := risk.NewTickerBreaker(risk.TickerBreakerConfig{
		MaxConsecutiveLosses: 3,
		MaxCumulativeLossPct: risk.DefaultTickerBreakerConfig().MaxCumulativeLossPct,
		Notional:             risk.DefaultTickerBreakerConfig().Notional,
	})
	seedTickerBreaker(breaker, events, log.New(&bytes.Buffer{}, "", 0))
	return breaker
}

func TestSeedTickerBreakerRestoresOpenPositionAcrossRestart(t *testing.T) {
	store, err := storage.Open(t.TempDir() + "/seed.db")
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	if err := store.InsertAuditEvent(ctx, domain.AuditEvent{Stage: "executor", Payload: `{"ticker":"SBER","action":"SELL","lots":10,"price":"250.00","commission":"1.25","executed_at":"2026-09-15T10:00:00Z"}`}); err != nil {
		t.Fatalf("append: %v", err)
	}
	events, err := store.ListAllAuditEvents(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	breaker := seedTestBreaker(t, events)

	lots, realized, _, _, _ := breaker.State("SBER")
	if lots != -10 {
		t.Fatalf("open lots = %d, want -10 (short survives the restart)", lots)
	}
	if realized.IsNegative() {
		t.Fatalf("realized = %s, want non-negative after only an open", realized.String())
	}
}

func TestSeedTickerBreakerTracksOpenThenClosePnl(t *testing.T) {
	store, err := storage.Open(t.TempDir() + "/seed.db")
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	ff := domain.AuditEvent{Stage: "executor", Payload: `{"ticker":"T","action":"SELL","lots":6,"price":"260.00","commission":"0.78","executed_at":"2026-09-15T10:00:00Z"}`}
	if err := store.InsertAuditEvent(ctx, ff); err != nil {
		t.Fatalf("append open: %v", err)
	}
	events, err := store.ListAllAuditEvents(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	breaker := seedTestBreaker(t, events)

	// A BUY after the restart is a CLOSE of the seeded short, not a fresh
	// long: realized P&L must be computed against the seeded entry basis and
	// the open position must shrink instead of flipping sign.
	breaker.RecordFill(risk.Fill{
		Ticker:     "T",
		Action:     domain.ActionBuy,
		Lots:       6,
		Price:      decimal.RequireFromString("250"),
		Commission: decimal.RequireFromString("0.75"),
	})

	lots, realized, _, tripped, _ := breaker.State("T")
	if lots != 0 {
		t.Fatalf("open lots = %d, want 0 (position fully closed)", lots)
	}
	want := decimal.RequireFromString("58.47") // (259.87-250)*6 - 0.75
	if !realized.Equal(want) {
		t.Fatalf("realized = %s, want %s (short entry 260, exit 250, per-lot commissions ~0.255)", realized.String(), want.String())
	}
	if tripped {
		t.Fatalf("breaker tripped after a winning close")
	}
}
