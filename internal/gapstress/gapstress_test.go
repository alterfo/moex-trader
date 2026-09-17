package gapstress

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

func dec(s string) decimal.Decimal {
	return decimal.RequireFromString(s)
}

func TestImpactPnlLongShort(t *testing.T) {
	portfolio := Portfolio{
		Deposit: dec("1000000"),
		Positions: []Position{
			{Ticker: "LONG", Side: SideLong, Notional: dec("10000")},
			{Ticker: "SHORT", Side: SideShort, Notional: dec("5000")},
		},
	}
	pnl := ImpactPnl(portfolio, map[string]float64{"LONG": -0.10, "SHORT": -0.10})
	want := dec("-500")
	if !pnl.Equal(want) {
		t.Fatalf("ImpactPnl = %s, want %s (long -1000 + short +500)", pnl, want)
	}
}

func TestImpactPnlIgnoresMissingAndNonFinite(t *testing.T) {
	portfolio := Portfolio{
		Deposit:   dec("1000000"),
		Positions: []Position{{Ticker: "LONG", Side: SideLong, Notional: dec("10000")}},
	}
	pnl := ImpactPnl(portfolio, map[string]float64{"OTHER": -0.10})
	if !pnl.IsZero() {
		t.Fatalf("missing ticker should not move P&L, got %s", pnl)
	}
	pnl = ImpactPnl(portfolio, map[string]float64{"LONG": 0})
	if !pnl.IsZero() {
		t.Fatalf("zero shock should not move P&L, got %s", pnl)
	}
}

func TestRunFloors(t *testing.T) {
	portfolio := Portfolio{
		Deposit: dec("1000000"),
		Positions: []Position{
			{Ticker: "LONG", Side: SideLong, Notional: dec("100000")},
		},
	}
	impacts := Run(portfolio, []Scenario{
		{Name: "tiny", Shocks: map[string]float64{"LONG": -0.01}},
		{Name: "daily", Shocks: map[string]float64{"LONG": -0.05}},
		{Name: "drawdown", Shocks: map[string]float64{"LONG": -0.30}},
	})
	byName := make(map[string]Impact, len(impacts))
	for _, impact := range impacts {
		byName[impact.Name] = impact
	}
	if byName["tiny"].BreachesDrawdownFloor || byName["tiny"].BreachesDailyLoss {
		t.Errorf("1%% loss should not breach floors: %+v", byName["tiny"])
	}
	if byName["daily"].BreachesDrawdownFloor {
		t.Errorf("5%% loss should breach only the daily-loss floor: %+v", byName["daily"])
	}
	if !byName["daily"].BreachesDailyLoss {
		t.Errorf("5%% loss should breach the 0.5%% daily-loss floor")
	}
	if !byName["drawdown"].BreachesDrawdownFloor {
		t.Errorf("30%% loss should breach the 3%% drawdown floor")
	}
	if !byName["drawdown"].Pnl.Equal(dec("-30000")) {
		t.Errorf("drawdown P&L = %s, want -30000", byName["drawdown"].Pnl)
	}
}

func TestOvernightGaps(t *testing.T) {
	day := func(d int) time.Time {
		return time.Date(2024, 1, d, 0, 0, 0, 0, time.UTC)
	}
	candles := []moex.Candle{
		{Open: dec("100"), Close: dec("110"), Begin: day(1)},
		{Open: dec("121"), Close: dec("120"), Begin: day(2)},
		{Open: dec("0"), Close: dec("130"), Begin: day(3)},
	}
	gaps := OvernightGaps(candles)
	if got := gaps[day(2)]; got != 0.1 {
		t.Errorf("gap day 2 = %f, want 0.1", got)
	}
	if _, ok := gaps[day(3)]; ok {
		t.Errorf("day 3 has zero open and must be skipped")
	}
}

func TestWorstHistoricalDayScenario(t *testing.T) {
	day := func(d int) time.Time {
		return time.Date(2024, 1, d, 0, 0, 0, 0, time.UTC)
	}
	portfolio := Portfolio{
		Deposit: dec("1000000"),
		Positions: []Position{
			{Ticker: "A", Side: SideLong, Notional: dec("10000")},
			{Ticker: "B", Side: SideLong, Notional: dec("10000")},
		},
	}
	gaps := map[string]map[time.Time]float64{
		"A": {day(2): -0.01, day(3): -0.20},
		"B": {day(2): -0.02, day(3): -0.03},
	}
	scenario, worstDate, ok := WorstHistoricalDayScenario(portfolio, gaps)
	if !ok {
		t.Fatal("expected a worst historical day to be found")
	}
	if !worstDate.Equal(day(3)) {
		t.Errorf("worst date = %v, want %v", worstDate, day(3))
	}
	pnl := ImpactPnl(portfolio, scenario.Shocks)
	if !pnl.Equal(dec("-2300")) {
		t.Errorf("worst day P&L = %s, want -2300 (-2000 + -300)", pnl)
	}
}

func TestWorstPerTickerScenario(t *testing.T) {
	day := func(d int) time.Time {
		return time.Date(2024, 1, d, 0, 0, 0, 0, time.UTC)
	}
	portfolio := Portfolio{
		Deposit: dec("1000000"),
		Positions: []Position{
			{Ticker: "LONG", Side: SideLong, Notional: dec("10000")},
			{Ticker: "SHORT", Side: SideShort, Notional: dec("10000")},
		},
	}
	gaps := map[string]map[time.Time]float64{
		"LONG":  {day(1): -0.05, day(2): -0.12, day(3): 0.03},
		"SHORT": {day(1): 0.04, day(2): 0.09, day(3): -0.02},
	}
	scenario := WorstPerTickerScenario(portfolio, gaps)
	if got := scenario.Shocks["LONG"]; got != -0.12 {
		t.Errorf("LONG shock = %f, want -0.12", got)
	}
	if got := scenario.Shocks["SHORT"]; got != 0.09 {
		t.Errorf("SHORT shock = %f, want 0.09", got)
	}
}

func TestUniformShockScenario(t *testing.T) {
	scenario := UniformShockScenario([]string{" a ", "B"}, "synthetic", -0.2)
	if len(scenario.Shocks) != 2 {
		t.Fatalf("expected 2 normalized shocks, got %d", len(scenario.Shocks))
	}
	if got := scenario.Shocks["A"]; got != -0.2 {
		t.Errorf("A shock = %f, want -0.2", got)
	}
	if got := scenario.Shocks["B"]; got != -0.2 {
		t.Errorf("B shock = %f, want -0.2", got)
	}
}

func TestExposures(t *testing.T) {
	portfolio := Portfolio{
		Deposit: dec("1000000"),
		Positions: []Position{
			{Ticker: "A", Side: SideLong, Notional: dec("15000")},
			{Ticker: "B", Side: SideLong, Notional: dec("10000")},
			{Ticker: "C", Side: SideShort, Notional: dec("5000")},
		},
	}
	if !GrossExposure(portfolio).Equal(dec("30000")) {
		t.Errorf("gross = %s, want 30000", GrossExposure(portfolio))
	}
	if !LongExposure(portfolio).Equal(dec("25000")) {
		t.Errorf("long = %s, want 25000", LongExposure(portfolio))
	}
	if !ShortExposure(portfolio).Equal(dec("5000")) {
		t.Errorf("short = %s, want 5000", ShortExposure(portfolio))
	}
	if !NetExposure(portfolio).Equal(dec("20000")) {
		t.Errorf("net = %s, want 20000", NetExposure(portfolio))
	}
}
