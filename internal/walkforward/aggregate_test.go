package walkforward

import (
	"math"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/domain"
)

func closeF(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Fatalf("%s: got %v, want %v (tol %v)", name, got, want, tol)
	}
}

func TestAggregateEquityCurve_ChainsWindowReturns(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	w1 := []backtest.EquityPoint{
		{Date: start, Equity: decimal.NewFromInt(100000)},
		{Date: start.AddDate(0, 0, 1), Equity: decimal.NewFromInt(110000)},
	}
	w2Start := start.AddDate(0, 0, 2)
	w2 := []backtest.EquityPoint{
		{Date: w2Start, Equity: decimal.NewFromInt(50000)},
		{Date: w2Start.AddDate(0, 0, 1), Equity: decimal.NewFromInt(55000)},
	}
	results := []backtest.Result{
		{EquityCurve: w1},
		{EquityCurve: w2},
	}
	deposit := decimal.NewFromInt(100000)
	curve := AggregateEquityCurve(results, deposit)

	if len(curve) != 3 {
		t.Fatalf("len(curve) = %d, want 3 (seam point of window 2 must be skipped)", len(curve))
	}
	first, _ := curve[0].Equity.Float64()
	closeF(t, "first", first, 100000, 1e-6)
	second, _ := curve[1].Equity.Float64()
	closeF(t, "second (window1 end, +10%%)", second, 110000, 1e-6)
	third, _ := curve[2].Equity.Float64()
	closeF(t, "third (window2 end, chained +10%% then +10%%)", third, 121000, 1e-6)
}

func TestAggregateEquityCurve_SkipsEmptyOrInvalidWindows(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	results := []backtest.Result{
		{EquityCurve: nil},
		{EquityCurve: []backtest.EquityPoint{{Date: start, Equity: decimal.NewFromInt(100000)}, {Date: start.AddDate(0, 0, 1), Equity: decimal.NewFromInt(105000)}}},
	}
	curve := AggregateEquityCurve(results, decimal.NewFromInt(100000))
	if len(curve) != 2 {
		t.Fatalf("len(curve) = %d, want 2", len(curve))
	}
}

func TestAggregate_SumsAcrossWindows(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	mid := start.AddDate(0, 0, 30)
	end := start.AddDate(0, 0, 60)

	r1 := backtest.Result{
		Start: start, End: mid,
		NetPnl: decimal.NewFromInt(1000), RealizedPnl: decimal.NewFromInt(900),
		ClosedTrades: 20, WinningTrades: 12,
		Trades: []backtest.Trade{
			{Ticker: "SBER", Action: domain.ActionBuy, NetPnl: decimal.NewFromInt(500)},
		},
		EquityCurve: []backtest.EquityPoint{
			{Date: start, Equity: decimal.NewFromInt(100000)},
			{Date: mid, Equity: decimal.NewFromInt(101000)},
		},
	}
	r2 := backtest.Result{
		Start: mid, End: end,
		NetPnl: decimal.NewFromInt(-200), RealizedPnl: decimal.NewFromInt(-250),
		ClosedTrades: 15, WinningTrades: 5,
		Trades: []backtest.Trade{
			{Ticker: "GAZP", Action: domain.ActionSell, NetPnl: decimal.NewFromInt(-100)},
		},
		EquityCurve: []backtest.EquityPoint{
			{Date: mid, Equity: decimal.NewFromInt(50000)},
			{Date: end, Equity: decimal.NewFromInt(49000)},
		},
	}

	agg := Aggregate([]string{"SBER", "GAZP"}, decimal.NewFromInt(100000), []backtest.Result{r1, r2})

	if agg.ClosedTrades != 35 {
		t.Fatalf("ClosedTrades = %d, want 35", agg.ClosedTrades)
	}
	if agg.WinningTrades != 17 {
		t.Fatalf("WinningTrades = %d, want 17", agg.WinningTrades)
	}
	closeF(t, "HitRate", agg.HitRate, 17.0/35.0, 1e-9)
	if !agg.NetPnl.Equal(decimal.NewFromInt(800)) {
		t.Fatalf("NetPnl = %s, want 800", agg.NetPnl)
	}
	if !agg.RealizedPnl.Equal(decimal.NewFromInt(650)) {
		t.Fatalf("RealizedPnl = %s, want 650", agg.RealizedPnl)
	}
	if len(agg.Trades) != 2 {
		t.Fatalf("len(Trades) = %d, want 2 (must concatenate all window trades)", len(agg.Trades))
	}
	if !agg.Start.Equal(start) {
		t.Fatalf("Start = %v, want %v", agg.Start, start)
	}
	if !agg.End.Equal(end) {
		t.Fatalf("End = %v, want %v", agg.End, end)
	}
	if len(agg.EquityCurve) == 0 {
		t.Fatal("expected a non-empty aggregate equity curve")
	}
	if agg.Attribution.TradeCount != 2 {
		t.Fatalf("Attribution.TradeCount = %d, want 2", agg.Attribution.TradeCount)
	}
}

func TestAggregate_EmptyResultsProducesZeroValue(t *testing.T) {
	agg := Aggregate([]string{"SBER"}, decimal.NewFromInt(100000), nil)
	if agg.ClosedTrades != 0 || agg.WinningTrades != 0 {
		t.Fatalf("expected zero trades, got closed=%d winning=%d", agg.ClosedTrades, agg.WinningTrades)
	}
	if agg.HitRate != 0 {
		t.Fatalf("HitRate = %v, want 0 for no trades", agg.HitRate)
	}
	if !agg.FinalEquity.Equal(decimal.NewFromInt(100000)) {
		t.Fatalf("FinalEquity = %s, want deposit unchanged when no windows ran", agg.FinalEquity)
	}
}
