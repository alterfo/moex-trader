package main

import (
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/betaregime"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

func TestBuildReportContainsSections(t *testing.T) {
	in := reportInput{
		from:           day(2025, 4, 1),
		till:           day(2026, 9, 17),
		tickers:        []string{"SBER", "GAZP"},
		ensemblePath:   "ensemble_model.json",
		deposit:        decimal.NewFromInt(1000000),
		commission:     decimal.NewFromFloat(0.0005),
		spread:         decimal.NewFromFloat(0.0005),
		slippage:       decimal.NewFromFloat(0.0005),
		targetNotional: decimal.NewFromInt(15000),
		k:              5,
		rebalanceEvery: 10,
		flatBand:       0.03,
		ensemble: &backtest.Result{
			RealizedPnl:  decimal.NewFromInt(1000),
			NetPnl:       decimal.NewFromInt(900),
			ClosedTrades: 2,
			Trades: []backtest.Trade{
				{EntryPrice: decimal.NewFromInt(150), Lots: 100, NetPnl: decimal.NewFromInt(500)},
				{EntryPrice: decimal.NewFromInt(150), Lots: 100, NetPnl: decimal.NewFromInt(500)},
			},
		},
		equalWeight: &backtest.Result{RealizedPnl: decimal.NewFromInt(200), NetPnl: decimal.NewFromInt(200)},
		momentum: []namedResult{
			{name: "long-only", result: &backtest.Result{RealizedPnl: decimal.NewFromInt(300)}},
			{name: "long+short", result: &backtest.Result{RealizedPnl: decimal.NewFromInt(400)}},
		},
		decomposition: betaregime.Decomposition{
			Long:                  betaregime.LegStats{Trades: 1, RealizedPnl: decimal.NewFromInt(500), Notional: decimal.NewFromInt(15000), ImoexPnl: decimal.NewFromInt(100), ExcessPnl: decimal.NewFromInt(400), ReturnOnNotional: 0.0267},
			Short:                 betaregime.LegStats{Trades: 1, RealizedPnl: decimal.NewFromInt(500), Notional: decimal.NewFromInt(15000), ImoexPnl: decimal.NewFromInt(-50), ExcessPnl: decimal.NewFromInt(550), ReturnOnNotional: 0.0333},
			TotalRealizedPnl:      decimal.NewFromInt(1000),
			GrossExposure:         decimal.NewFromInt(30000),
			ImoexPnl:              decimal.NewFromInt(50),
			ExcessPnl:             decimal.NewFromInt(950),
			ReturnOnGrossExposure: 0.0333,
		},
		samples: 2,
		tradeRegression: betaregime.RegressionResult{
			Observations: 2, Parameters: 3, Clusters: 2, DegreesFree: 1, R2: 0.5,
			Coef: []float64{0.01, 0.5, -0.2}, StdErr: []float64{0.005, 0.1, 0.2},
			TStat: []float64{2, 5, -1}, PValue: []float64{0.1, 0.01, 0.5},
		},
		dailyRegression: betaregime.RegressionResult{
			Observations: 10, Parameters: 3, Clusters: 6, DegreesFree: 5, R2: 0.1,
			Coef: []float64{0.0, 0.1, 0.2}, StdErr: []float64{0.001, 0.02, 0.03},
			TStat: []float64{0, 5, 6.7}, PValue: []float64{1, 0.01, 0.01},
		},
		dailySkipped: 1,
		regimes: []betaregime.WindowRegime{
			{Window: betaregime.Window{From: day(2025, 4, 1), Till: day(2025, 6, 30)}, ImoexReturn: 0.08, Regime: betaregime.RegimeTrendUp, HasData: true},
		},
	}
	out := buildReport(in)
	for _, want := range []string{
		"# Beta / regime decomposition (Task 9)",
		"Results (net of costs)",
		"Realized P&L decomposition net of IMOEX",
		"Per-trade",
		"Per-day",
		"OOS quarter regimes (IMOEX)",
		"beta1 (equal-weight)",
		"beta2 (momentum)",
		"trend-up",
		"equal-weight 18-ticker",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("report missing %q", want)
		}
	}
}

func TestResolveWindowsDefault(t *testing.T) {
	windows, err := resolveWindows("", day(2025, 4, 1), day(2026, 9, 17))
	if err != nil {
		t.Fatal(err)
	}
	if len(windows) != 6 {
		t.Fatalf("default windows = %d, want 6", len(windows))
	}
	if !windows[0].From.Equal(day(2025, 4, 1)) || !windows[5].Till.Equal(day(2026, 9, 17)) {
		t.Fatalf("unexpected default windows: %+v", windows)
	}
}

func TestResolveWindowsParsesPairs(t *testing.T) {
	windows, err := resolveWindows("2025-04-01:2025-06-30,2025-07-01:2025-09-30", time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(windows) != 2 || !windows[1].Till.Equal(day(2025, 9, 30)) {
		t.Fatalf("unexpected parsed windows: %+v", windows)
	}
	if _, err := resolveWindows("2025-04-01", time.Time{}, time.Time{}); err == nil {
		t.Fatal("expected error for a malformed pair")
	}
}

func TestSplitCommaAndDropIncompleteTrailing(t *testing.T) {
	got := splitComma("SBER, GAZP ,")
	if len(got) != 2 || got[0] != "SBER" || got[1] != "GAZP" {
		t.Fatalf("splitComma = %v", got)
	}
	candles := []moex.Candle{
		{Begin: day(2026, 1, 5), End: day(2026, 1, 5).Add(23 * time.Hour)},
		{Begin: day(2026, 1, 6), End: day(2026, 1, 6).Add(10 * time.Hour)},
	}
	got2 := dropIncompleteTrailing(candles)
	if len(got2) != 1 || !got2[0].Begin.Equal(day(2026, 1, 5)) {
		t.Fatalf("dropIncompleteTrailing = %d candles", len(got2))
	}
}
