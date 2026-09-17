package equalweight

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

func TestTargetPositionAlwaysLong(t *testing.T) {
	src := &SignalSource{TargetNotional: decimal.NewFromInt(15000), MaxLots: 1000}
	signed, ok := src.TargetPosition(domain.FeatureContext{
		Ticker:      "SBER",
		GeneratedAt: day(2026, 1, 5),
		LastPrice:   decimal.NewFromInt(150),
	})
	if !ok || signed <= 0 {
		t.Fatalf("equal-weight target must always be long, got %d/%v", signed, ok)
	}
	if signed != 100 {
		t.Fatalf("15000 notional / 150 price = 100 lots, got %d", signed)
	}
}

func TestTargetPositionRespectsLotSize(t *testing.T) {
	src := &SignalSource{TargetNotional: decimal.NewFromInt(15000), MaxLots: 1000}
	signed, _ := src.TargetPosition(domain.FeatureContext{
		Ticker:      "GAZP",
		GeneratedAt: day(2026, 1, 5),
		LastPrice:   decimal.NewFromInt(150),
		LotSize:     decimal.NewFromInt(10),
	})
	if signed != 10 {
		t.Fatalf("15000 / (150*10) = 10 lots, got %d", signed)
	}
}

func TestTargetPositionMaxLotsFallback(t *testing.T) {
	src := &SignalSource{MaxLots: 7}
	signed, ok := src.TargetPosition(domain.FeatureContext{
		Ticker:      "SBER",
		GeneratedAt: day(2026, 1, 5),
		LastPrice:   decimal.Zero,
	})
	if !ok || signed != 7 {
		t.Fatalf("without notional and a non-positive price, maxLots should be used, got %d/%v", signed, ok)
	}
}

func TestGenerateEmitsBuy(t *testing.T) {
	src := &SignalSource{TargetNotional: decimal.NewFromInt(15000), MaxLots: 1000}
	signal, err := src.Generate(context.Background(), domain.FeatureContext{
		Ticker:      "SBER",
		GeneratedAt: day(2026, 1, 5),
		LastPrice:   decimal.NewFromInt(150),
	})
	if err != nil {
		t.Fatal(err)
	}
	if signal.Action != domain.ActionBuy || signal.TargetLots <= 0 {
		t.Fatalf("equal-weight must emit BUY with positive lots, got %s/%d", signal.Action, signal.TargetLots)
	}
	if signal.Reasoning != "equal-weight" {
		t.Fatalf("unexpected reasoning %q", signal.Reasoning)
	}
}

func day(year int, month int, d int) time.Time {
	return time.Date(year, time.Month(month), d, 0, 0, 0, 0, time.UTC)
}
