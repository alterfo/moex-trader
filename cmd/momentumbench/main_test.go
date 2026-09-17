package main

import (
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
)

func resultWith(realized, mtm string, trades int, dd float64, tripped bool) *backtest.Result {
	r := &backtest.Result{
		RealizedPnl:       decimal.RequireFromString(realized),
		NetPnl:            decimal.RequireFromString(mtm),
		ClosedTrades:      trades,
		MaxDrawdownPct:    dd,
		KillSwitchTripped: tripped,
	}
	if trades > 0 {
		r.WinningTrades = trades / 2
		r.HitRate = 0.5
	}
	return r
}

func TestBeatsAll(t *testing.T) {
	ensemble := resultWith("100", "120", 10, 1.0, false)
	if !beatsAll(ensemble, []namedResult{
		{name: "long-only", result: resultWith("50", "60", 5, 1.0, false)},
		{name: "long+short", result: resultWith("90", "95", 6, 1.0, false)},
	}) {
		t.Fatal("ensemble should beat both benchmarks")
	}
	if beatsAll(ensemble, []namedResult{
		{name: "long-only", result: resultWith("150", "160", 5, 1.0, false)},
	}) {
		t.Fatal("ensemble should not beat a higher benchmark")
	}
}

func TestBuildReportContainsRealizedAndMTM(t *testing.T) {
	in := reportInput{
		from:           time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC),
		till:           time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC),
		tickers:        []string{"SBER", "GAZP"},
		ensemblePath:   "ensemble_model.json",
		deposit:        decimal.NewFromInt(1000000),
		commission:     decimal.RequireFromString("0.0005"),
		spread:         decimal.RequireFromString("0.0005"),
		slippage:       decimal.RequireFromString("0.0005"),
		targetNotional: decimal.NewFromInt(15000),
		k:              5,
		rebalanceEvery: 10,
		tradingDays:    63,
		ensemble:       resultWith("100", "120", 10, 1.0, false),
		momentum: []namedResult{
			{name: "long-only", result: resultWith("50", "60", 5, 1.0, false)},
			{name: "long+short", result: resultWith("90", "95", 6, 1.0, false)},
		},
		ensembleBeats: true,
	}

	report := buildReport(in)
	for _, want := range []string{"realized P&L", "MTM P&L", "ensemble (deployed)", "momentum long-only", "momentum long+short", "100", "120"} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
}
