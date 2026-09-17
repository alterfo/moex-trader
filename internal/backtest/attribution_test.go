package backtest

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

func dec(v string) decimal.Decimal {
	return decimal.RequireFromString(v)
}

func TestComputeAttributionGroupsRealizedByTicker(t *testing.T) {
	trades := []Trade{
		{Ticker: "SBER", Action: domain.ActionBuy, GrossPnl: dec("120"), Commission: dec("20"), NetPnl: dec("100"), OpenedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
		{Ticker: "GAZP", Action: domain.ActionSell, GrossPnl: dec("60"), Commission: dec("10"), NetPnl: dec("50"), OpenedAt: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)},
		{Ticker: "SBER", Action: domain.ActionSell, GrossPnl: dec("-20"), Commission: dec("10"), NetPnl: dec("-30"), OpenedAt: time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC)},
	}
	open := []OpenPosition{
		{Ticker: "SBER", UnrealizedPnl: dec("10")},
		{Ticker: "GAZP", UnrealizedPnl: dec("-5")},
	}

	attribution := computeAttribution(trades, open, DefaultAttributionTopN)

	if !attribution.RealizedTotal.Equal(dec("120")) {
		t.Fatalf("RealizedTotal = %s, want 120", attribution.RealizedTotal)
	}
	if !attribution.UnrealizedTotal.Equal(dec("5")) {
		t.Fatalf("UnrealizedTotal = %s, want 5", attribution.UnrealizedTotal)
	}
	if attribution.TradeCount != 3 {
		t.Fatalf("TradeCount = %d, want 3", attribution.TradeCount)
	}
	if len(attribution.Tickers) != 2 {
		t.Fatalf("len(Tickers) = %d, want 2", len(attribution.Tickers))
	}
	if attribution.Tickers[0].Ticker != "GAZP" || attribution.Tickers[1].Ticker != "SBER" {
		t.Fatalf("tickers not sorted: %+v", attribution.Tickers)
	}

	sber := attribution.Tickers[1]
	if sber.Trades != 2 || sber.Wins != 1 || sber.Losses != 1 {
		t.Fatalf("SBER attribution = %+v, want trades=2 wins=1 losses=1", sber)
	}
	if !sber.GrossPnl.Equal(dec("100")) || !sber.Commission.Equal(dec("30")) || !sber.RealizedPnl.Equal(dec("70")) {
		t.Fatalf("SBER attribution = %+v", sber)
	}
	if !sber.UnrealizedPnl.Equal(dec("10")) {
		t.Fatalf("SBER unrealized = %s, want 10", sber.UnrealizedPnl)
	}

	gazp := attribution.Tickers[0]
	if gazp.Trades != 1 || gazp.Wins != 1 || gazp.Losses != 0 {
		t.Fatalf("GAZP attribution = %+v", gazp)
	}
	if !gazp.RealizedPnl.Equal(dec("50")) || !gazp.UnrealizedPnl.Equal(dec("-5")) {
		t.Fatalf("GAZP attribution = %+v", gazp)
	}
}

func TestComputeAttributionTopTradeConcentration(t *testing.T) {
	trades := []Trade{
		{Ticker: "A", NetPnl: dec("10")},
		{Ticker: "B", NetPnl: dec("90")},
		{Ticker: "C", NetPnl: dec("-20")},
	}
	attribution := computeAttribution(trades, nil, 1)
	want := 90.0 / 120.0
	if math.Abs(attribution.TopTradeShare-want) > 1e-9 {
		t.Fatalf("TopTradeShare = %f, want %f", attribution.TopTradeShare, want)
	}
	if attribution.TopTickerShare == 0 {
		t.Fatal("TopTickerShare should be non-zero")
	}

	all := computeAttribution(trades, nil, len(trades))
	if math.Abs(all.TopTradeShare-1.0) > 1e-9 {
		t.Fatalf("TopTradeShare with topN=len(trades) = %f, want 1.0", all.TopTradeShare)
	}
}

func TestComputeAttributionTopTickerConcentration(t *testing.T) {
	trades := []Trade{
		{Ticker: "A", NetPnl: dec("10")},
		{Ticker: "B", NetPnl: dec("90")},
		{Ticker: "C", NetPnl: dec("-20")},
	}
	attribution := computeAttribution(trades, nil, 1)
	want := 90.0 / 120.0
	if math.Abs(attribution.TopTickerShare-want) > 1e-9 {
		t.Fatalf("TopTickerShare = %f, want %f", attribution.TopTickerShare, want)
	}
}

func TestComputeAttributionEmpty(t *testing.T) {
	attribution := computeAttribution(nil, nil, DefaultAttributionTopN)
	if len(attribution.Tickers) != 0 {
		t.Fatalf("expected no tickers, got %+v", attribution.Tickers)
	}
	if !attribution.RealizedTotal.IsZero() || !attribution.UnrealizedTotal.IsZero() {
		t.Fatalf("expected zero totals, got %+v", attribution)
	}
	if attribution.TopTradeShare != 0 || attribution.TopTickerShare != 0 {
		t.Fatalf("expected zero concentration, got %+v", attribution)
	}
}

func TestResultMarkdownIncludesAttribution(t *testing.T) {
	result := Result{
		Deposit: dec("100000"),
		Attribution: Attribution{
			TopN:            DefaultAttributionTopN,
			RealizedTotal:   dec("120"),
			UnrealizedTotal: dec("5"),
			TopTradeShare:   0.75,
			TopTickerShare:  0.6,
			TradeCount:      3,
			Tickers: []TickerAttribution{
				{Ticker: "SBER", Trades: 2, Wins: 1, Losses: 1, GrossPnl: dec("100"), Commission: dec("30"), RealizedPnl: dec("70"), UnrealizedPnl: dec("10")},
			},
		},
	}
	markdown := result.Markdown()
	for _, want := range []string{"## Attribution", "Realized/unrealized split", "Top 5 trade concentration", "Realized P&L by ticker", "SBER"} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("Markdown missing %q:\n%s", want, markdown)
		}
	}
}
