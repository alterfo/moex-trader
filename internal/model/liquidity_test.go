package model

import (
	"testing"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

func turnoverCandles(values ...string) []moex.Candle {
	out := make([]moex.Candle, len(values))
	for i, value := range values {
		out[i].Value = decimal.RequireFromString(value)
	}
	return out
}

func TestMedianTrailingTurnoverKnownValues(t *testing.T) {
	candles := turnoverCandles("1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20")
	got := medianTrailingTurnover(candles, 19, 20)
	want := decimal.RequireFromString("10.5")
	if !got.Equal(want) {
		t.Fatalf("median = %s, want %s", got, want)
	}
}

func TestMedianTrailingTurnoverIgnoresFutureBars(t *testing.T) {
	before := turnoverCandles("1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20")
	first := medianTrailingTurnover(before, 19, 20)
	after := append(append([]moex.Candle{}, before...), moex.Candle{Value: decimal.RequireFromString("1000000")})
	second := medianTrailingTurnover(after, 19, 20)
	if !first.Equal(second) {
		t.Fatalf("median at idx 19 changed from %s to %s after appending a future bar", first, second)
	}
}

func TestLiquidOnRequiresFullWindow(t *testing.T) {
	candles := turnoverCandles("1000000", "1000000", "1000000")
	if LiquidOn(candles, 2, 20, decimal.Zero) {
		t.Fatal("LiquidOn() = true with fewer than 20 trailing bars, want false")
	}
}

func TestLiquidOnThresholdComparison(t *testing.T) {
	values := make([]string, 20)
	for i := range values {
		values[i] = "1000000"
	}
	candles := turnoverCandles(values...)
	if !LiquidOn(candles, 19, 20, decimal.RequireFromString("1000000")) {
		t.Fatal("LiquidOn() = false at the threshold, want true")
	}
	if LiquidOn(candles, 19, 20, decimal.RequireFromString("1000001")) {
		t.Fatal("LiquidOn() = true above the threshold, want false")
	}
}
