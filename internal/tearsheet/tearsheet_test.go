package tearsheet

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/domain"
)

func dec(v string) decimal.Decimal {
	return decimal.RequireFromString(v)
}

func sampleResult() backtest.Result {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	curve := []backtest.EquityPoint{
		{Date: start, Equity: dec("1000000")},
		{Date: start.AddDate(0, 0, 1), Equity: dec("1010000")},
		{Date: start.AddDate(0, 0, 2), Equity: dec("990000")},
		{Date: start.AddDate(0, 0, 3), Equity: dec("1005000")},
	}
	trades := []backtest.Trade{
		{Ticker: "SBER", Action: domain.ActionBuy, Lots: 10, EntryPrice: dec("250.00"), ExitPrice: dec("255.00"), GrossPnl: dec("500"), Commission: dec("20"), NetPnl: dec("480"), OpenedAt: start, ClosedAt: start.AddDate(0, 0, 1)},
		{Ticker: "GAZP", Action: domain.ActionSell, Lots: 5, EntryPrice: dec("170.00"), ExitPrice: dec("168.00"), GrossPnl: dec("-10"), Commission: dec("5"), NetPnl: dec("-15"), OpenedAt: start.AddDate(0, 0, 1), ClosedAt: start.AddDate(0, 0, 2)},
	}
	return backtest.Result{
		Tickers:         []string{"SBER", "GAZP"},
		Start:           start,
		End:             start.AddDate(0, 0, 3),
		Deposit:         dec("1000000"),
		FinalEquity:     dec("1005000"),
		NetPnl:          dec("5000"),
		RealizedPnl:     dec("465"),
		UnrealizedPnl:   dec("4535"),
		GrossPnl:        dec("490"),
		TotalCommission: dec("25"),
		ClosedTrades:    2,
		WinningTrades:   1,
		HitRate:         0.5,
		Sharpe:          0.42,
		Sortino:         -0.10,
		Calmar:          1.2,
		CAGR:            5.4,
		MaxDrawdownPct:  1.98,
		MaxDrawdownRub:  dec("20000"),
		Trades:          trades,
		EquityCurve:     curve,
		Attribution: backtest.Attribution{
			Tickers: []backtest.TickerAttribution{
				{Ticker: "GAZP", Trades: 1, Wins: 0, Losses: 1, GrossPnl: dec("-10"), Commission: dec("5"), RealizedPnl: dec("-15")},
				{Ticker: "SBER", Trades: 1, Wins: 1, Losses: 0, GrossPnl: dec("500"), Commission: dec("20"), RealizedPnl: dec("480")},
			},
			RealizedTotal: dec("465"),
			TopN:          5,
			TradeCount:    2,
		},
	}
}

func TestRender_ContainsAllSections(t *testing.T) {
	r := sampleResult()
	html, metricsJSON, err := Render(r)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	page := string(html)

	if !strings.Contains(page, "<!doctype html>") {
		t.Fatalf("missing doctype")
	}
	for _, want := range []string{
		"Key metrics",
		"Equity curve",
		"Drawdown (underwater)",
		"Attribution by ticker",
		"Closed trades",
		"SBER",
		"GAZP",
		"<svg",
		"polyline",
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("rendered HTML missing %q", want)
		}
	}

	var export MetricsExport
	if err := json.Unmarshal(metricsJSON, &export); err != nil {
		t.Fatalf("metricsJSON invalid: %v", err)
	}
	if export.Sharpe != r.Sharpe || export.Sortino != r.Sortino || export.Calmar != r.Calmar {
		t.Fatalf("metrics export mismatch: %+v", export)
	}
	if export.ClosedTrades != r.ClosedTrades {
		t.Fatalf("ClosedTrades = %d, want %d", export.ClosedTrades, r.ClosedTrades)
	}
	if export.StatisticallySignificant {
		t.Fatalf("expected StatisticallySignificant=false for %d trades", r.ClosedTrades)
	}
}

func TestRender_SignificanceWarningShownBelowThreshold(t *testing.T) {
	r := sampleResult()
	html, _, err := Render(r)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	if !strings.Contains(string(html), "not statistically significant") {
		t.Fatalf("expected significance warning in HTML for %d closed trades", r.ClosedTrades)
	}
}

func TestRender_NoWarningWhenSignificant(t *testing.T) {
	r := sampleResult()
	r.ClosedTrades = backtest.MinTradesForSignificance
	html, _, err := Render(r)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	if strings.Contains(string(html), "not statistically significant") {
		t.Fatalf("did not expect significance warning for %d closed trades", r.ClosedTrades)
	}
}

func TestLineChartSVG_CoordinatesWithinViewBox(t *testing.T) {
	values := []float64{100, 110, 90, 95, 105}
	svg := string(lineChartSVG(values, "--ts-equity", "--ts-equity", false))
	if svg == "" {
		t.Fatalf("expected non-empty SVG")
	}
	if !strings.Contains(svg, `viewBox="0 0 760 180"`) {
		t.Fatalf("unexpected viewBox: %s", svg)
	}
	start := strings.Index(svg, `points="`) + len(`points="`)
	end := strings.Index(svg[start:], `"`)
	pointsStr := svg[start : start+end]
	pairs := strings.Fields(pointsStr)
	if len(pairs) != len(values) {
		t.Fatalf("got %d coordinate pairs, want %d", len(pairs), len(values))
	}
	for _, pair := range pairs {
		parts := strings.SplitN(pair, ",", 2)
		if len(parts) != 2 {
			t.Fatalf("malformed coordinate pair %q", pair)
		}
	}
}

func TestLineChartSVG_EmptyValuesReturnsEmpty(t *testing.T) {
	if svg := lineChartSVG(nil, "--ts-equity", "--ts-equity", false); svg != "" {
		t.Fatalf("expected empty SVG for no values, got %q", svg)
	}
}
