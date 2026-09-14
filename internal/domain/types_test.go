package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func newValidSignal() TradeSignal {
	return TradeSignal{
		Ticker:      "SBER",
		Action:      ActionBuy,
		Confidence:  decimal.NewFromFloat(0.75),
		TargetLots:  1,
		Reasoning:   "positive news and rising price",
		GeneratedAt: time.Now(),
	}
}

func TestTradeSignalValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*TradeSignal)
		wantErr string
	}{
		{name: "valid signal"},
		{name: "confidence zero", mutate: func(s *TradeSignal) { s.Confidence = decimal.Zero }},
		{name: "confidence one", mutate: func(s *TradeSignal) { s.Confidence = decimal.NewFromInt(1) }},
		{name: "empty ticker", mutate: func(s *TradeSignal) { s.Ticker = " " }, wantErr: "ticker must not be empty"},
		{name: "unknown action", mutate: func(s *TradeSignal) { s.Action = Action("HODL") }, wantErr: "action must be one of BUY, SELL, HOLD"},
		{name: "negative confidence", mutate: func(s *TradeSignal) { s.Confidence = decimal.NewFromFloat(-0.01) }, wantErr: "confidence must be in [0,1]"},
		{name: "confidence above one", mutate: func(s *TradeSignal) { s.Confidence = decimal.NewFromFloat(1.01) }, wantErr: "confidence must be in [0,1]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signal := newValidSignal()
			if tt.mutate != nil {
				tt.mutate(&signal)
			}
			err := signal.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected nil error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestActionIsValid(t *testing.T) {
	tests := []struct {
		action Action
		want   bool
	}{
		{ActionBuy, true},
		{ActionSell, true},
		{ActionHold, true},
		{Action(""), false},
		{Action("buy"), false},
	}
	for _, tt := range tests {
		if got := tt.action.IsValid(); got != tt.want {
			t.Fatalf("Action(%q).IsValid() = %v, want %v", tt.action, got, tt.want)
		}
	}
}

func TestFeatureContextUsesDecimalForPrices(t *testing.T) {
	ctx := FeatureContext{
		Ticker:             "YDEX",
		LastPrice:          decimal.NewFromFloat(4500.25),
		PrevClose:          decimal.NewFromFloat(4480.10),
		Bid:                decimal.NewFromFloat(4499.90),
		Ask:                decimal.NewFromFloat(4500.60),
		ReturnPct:          decimal.NewFromFloat(0.45),
		RealizedVolatility: decimal.NewFromFloat(12.5),
		NewsSentiment:      decimal.NewFromFloat(0.3),
		OrderBookImbalance: decimal.NewFromFloat(-0.1),
	}
	if ctx.LastPrice.String() != "4500.25" {
		t.Fatalf("unexpected last price: %s", ctx.LastPrice.String())
	}
	if !ctx.LastPrice.Equal(decimal.NewFromFloat(4500.25)) {
		t.Fatal("decimal last price did not round-trip")
	}
}
