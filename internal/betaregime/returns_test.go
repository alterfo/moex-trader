package betaregime

import (
	"math"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

func d(year, month, day int) time.Time {
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

func TestDailyReturnsFromCurve(t *testing.T) {
	curve := []backtest.EquityPoint{
		{Date: d(2026, 1, 5), Equity: decimal.NewFromFloat(100000.1)},
		{Date: d(2026, 1, 6), Equity: decimal.NewFromFloat(101000.2)},
		{Date: d(2026, 1, 8), Equity: decimal.NewFromFloat(99000.3)},
	}
	ret := DailyReturnsFromCurve(curve)
	if got := ret[d(2026, 1, 6)]; math.Abs(got-(101000.2/100000.1-1)) > 1e-12 {
		t.Fatalf("day 2 return = %v", got)
	}
	if got := ret[d(2026, 1, 8)]; math.Abs(got-(99000.3/101000.2-1)) > 1e-12 {
		t.Fatalf("day 3 return = %v", got)
	}
	if _, ok := ret[d(2026, 1, 5)]; ok {
		t.Fatal("first day must not have a return")
	}
}

func TestDailyReturnsFromCandles(t *testing.T) {
	candles := []moex.Candle{
		{Begin: d(2026, 1, 5), Close: decimal.NewFromFloat(100.1)},
		{Begin: d(2026, 1, 6), Close: decimal.NewFromFloat(110.2)},
	}
	ret := DailyReturnsFromCandles(candles)
	if got := ret[d(2026, 1, 6)]; math.Abs(got-(110.2/100.1-1)) > 1e-12 {
		t.Fatalf("return = %v, want %.12f", got, 110.2/100.1-1)
	}
}

func TestCompoundReturnWindow(t *testing.T) {
	ret := map[time.Time]float64{
		d(2026, 1, 5): 0.01,
		d(2026, 1, 6): 0.02,
		d(2026, 1, 7): 0.03,
	}
	got, ok := CompoundReturn(ret, d(2026, 1, 5), d(2026, 1, 7))
	if !ok {
		t.Fatal("expected a compounded return")
	}
	want := 1.02*1.03 - 1
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("got %v, want %v", got, want)
	}

	if _, ok := CompoundReturn(ret, d(2026, 1, 7), d(2026, 1, 8)); ok {
		t.Fatal("empty interval should report no data")
	}
}
