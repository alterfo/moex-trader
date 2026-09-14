package risk

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

func validSignal() domain.TradeSignal {
	return domain.TradeSignal{
		Ticker:      "SBER",
		Action:      domain.ActionBuy,
		Confidence:  decimal.NewFromFloat(0.6),
		TargetLots:  1,
		Reasoning:   "test signal",
		GeneratedAt: time.Now(),
	}
}

func TestNewLotLimitGateRejectsInvalidMaxLots(t *testing.T) {
	for _, maxLots := range []int{0, -1} {
		if _, err := NewLotLimitGate(maxLots); err == nil {
			t.Fatalf("NewLotLimitGate(%d) error = nil, want error", maxLots)
		}
	}
}

func TestLotLimitGateApprove(t *testing.T) {
	gate, err := NewLotLimitGate(1)
	if err != nil {
		t.Fatalf("NewLotLimitGate() error = %v", err)
	}

	tests := []struct {
		name    string
		mutate  func(*domain.TradeSignal)
		want    bool
		wantErr string
	}{
		{name: "one lot at limit", want: true},
		{name: "zero lots hold", mutate: func(s *domain.TradeSignal) {
			s.Action = domain.ActionHold
			s.TargetLots = 0
		}, want: true},
		{name: "two lots above limit", mutate: func(s *domain.TradeSignal) { s.TargetLots = 2 }, want: false},
		{name: "empty ticker", mutate: func(s *domain.TradeSignal) { s.Ticker = " " }, wantErr: "ticker must not be empty"},
		{name: "negative lots", mutate: func(s *domain.TradeSignal) { s.TargetLots = -1 }, wantErr: "target lots must be non-negative"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signal := validSignal()
			if tt.mutate != nil {
				tt.mutate(&signal)
			}

			approved, err := gate.Approve(context.Background(), signal)
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
