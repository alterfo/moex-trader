package model

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestVolScaleMultiplier(t *testing.T) {
	minMult := decimal.NewFromFloat(0.5)
	maxMult := decimal.NewFromFloat(1.5)
	tests := []struct {
		name   string
		sigma  string
		median string
		want   string
	}{
		{name: "below min clamps", sigma: "10", median: "2", want: "0.5"},
		{name: "above max clamps", sigma: "2", median: "10", want: "1.5"},
		{name: "inside band", sigma: "10", median: "8", want: "0.8"},
		{name: "equal returns one", sigma: "10", median: "10", want: "1"},
		{name: "zero sigma falls back", sigma: "0", median: "10", want: "1"},
		{name: "zero median falls back", sigma: "10", median: "0", want: "1"},
		{name: "negative sigma falls back", sigma: "-1", median: "10", want: "1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sigma := decimal.RequireFromString(tt.sigma)
			median := decimal.RequireFromString(tt.median)
			want := decimal.RequireFromString(tt.want)
			got := VolScaleMultiplier(sigma, median, minMult, maxMult)
			if !got.Equal(want) {
				t.Fatalf("multiplier = %s, want %s", got.String(), want.String())
			}
		})
	}
}

func TestVolScaleMultiplierDisabled(t *testing.T) {
	scale := VolScale{Enabled: false, MinMult: decimal.NewFromFloat(0.5), MaxMult: decimal.NewFromFloat(1.5)}
	got := scale.Multiplier(decimal.NewFromFloat(2), decimal.NewFromFloat(10))
	if !got.Equal(decimal.NewFromInt(1)) {
		t.Fatalf("disabled multiplier = %s, want 1", got.String())
	}
}

func TestTargetLotsVolScaleDisabledMatchesLegacy(t *testing.T) {
	notional := decimal.NewFromFloat(15000)
	price := decimal.NewFromFloat(300)
	got := targetLotsForNotional(notional, price, decimal.NewFromInt(1), 1000, VolScale{}, decimal.NewFromFloat(2), decimal.NewFromFloat(10))
	if got != 50 {
		t.Fatalf("disabled lots = %d, want 50", got)
	}
}

func TestTargetLotsVolScaleAppliesMultiplier(t *testing.T) {
	scale := VolScale{Enabled: true, MinMult: decimal.NewFromFloat(0.5), MaxMult: decimal.NewFromFloat(1.5)}
	notional := decimal.NewFromFloat(15000)
	price := decimal.NewFromFloat(300)
	got := targetLotsForNotional(notional, price, decimal.NewFromInt(1), 1000, scale, decimal.NewFromFloat(2), decimal.NewFromFloat(10))
	if got != 75 {
		t.Fatalf("scaled lots = %d, want 75 (notional 22500 / 300)", got)
	}
}

func TestTargetLotsVolScaleZeroVolatilityFallsBack(t *testing.T) {
	scale := VolScale{Enabled: true, MinMult: decimal.NewFromFloat(0.5), MaxMult: decimal.NewFromFloat(1.5)}
	notional := decimal.NewFromFloat(15000)
	price := decimal.NewFromFloat(300)
	got := targetLotsForNotional(notional, price, decimal.NewFromInt(1), 1000, scale, decimal.Zero, decimal.NewFromFloat(10))
	if got != 50 {
		t.Fatalf("zero-volatility lots = %d, want 50", got)
	}
}
