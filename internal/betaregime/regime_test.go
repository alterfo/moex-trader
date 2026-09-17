package betaregime

import (
	"math"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

func TestLabelRegime(t *testing.T) {
	if got := LabelRegime(0.05, 0.03); got != RegimeTrendUp {
		t.Fatalf("got %q, want trend-up", got)
	}
	if got := LabelRegime(-0.05, 0.03); got != RegimeTrendDown {
		t.Fatalf("got %q, want trend-down", got)
	}
	if got := LabelRegime(0.01, 0.03); got != RegimeFlat {
		t.Fatalf("got %q, want flat", got)
	}
	if got := LabelRegime(0.03, 0.03); got != RegimeFlat {
		t.Fatalf("boundary must be flat, got %q", got)
	}
}

func TestClassifyWindows(t *testing.T) {
	candles := []moex.Candle{
		{Begin: d(2025, 12, 30), Close: decimal.NewFromInt(100)},
		{Begin: d(2026, 1, 5), Close: decimal.NewFromInt(110)},
		{Begin: d(2026, 2, 2), Close: decimal.NewFromInt(99)},
	}
	windows := []Window{
		{From: d(2026, 1, 1), Till: d(2026, 1, 30)},
		{From: d(2026, 2, 1), Till: d(2026, 2, 28)},
	}
	regimes := ClassifyWindows(candles, windows, 0.03)
	if len(regimes) != 2 {
		t.Fatalf("got %d regimes, want 2", len(regimes))
	}
	// First window: 110/100 - 1 = 0.10 -> trend-up.
	if math.Abs(regimes[0].ImoexReturn-0.10) > 1e-12 || regimes[0].Regime != RegimeTrendUp {
		t.Fatalf("window 1 = %v/%s, want 0.10/trend-up", regimes[0].ImoexReturn, regimes[0].Regime)
	}
	// Second window: 99/110 - 1 = -0.10 -> trend-down.
	if math.Abs(regimes[1].ImoexReturn-(-0.10)) > 1e-12 || regimes[1].Regime != RegimeTrendDown {
		t.Fatalf("window 2 = %v/%s, want -0.10/trend-down", regimes[1].ImoexReturn, regimes[1].Regime)
	}
}

func TestClassifyWindowsMissingStart(t *testing.T) {
	candles := []moex.Candle{{Begin: d(2026, 1, 2), Close: decimal.NewFromInt(100)}}
	regimes := ClassifyWindows(candles, []Window{{From: d(2025, 1, 1), Till: d(2026, 1, 10)}}, 0.03)
	if regimes[0].HasData {
		t.Fatal("window before the first close must have HasData=false")
	}
}
