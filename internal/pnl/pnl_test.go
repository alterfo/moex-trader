package pnl

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

func dec(v string) decimal.Decimal {
	return decimal.RequireFromString(v)
}

func fill(ticker string, action domain.Action, lots int, price, commission string, at time.Time) Fill {
	return Fill{Ticker: ticker, Action: action, Lots: lots, Price: dec(price), Commission: dec(commission), ExecutedAt: at}
}

func TestReplayRoundTripRealizesNetOfCommission(t *testing.T) {
	base := time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC)
	snap := Replay([]Fill{
		fill("SBER", domain.ActionBuy, 1, "100", "0.05", base),
		fill("SBER", domain.ActionSell, 1, "110", "0.055", base.Add(time.Hour)),
	})

	want := dec("9.895")
	if !snap.RealizedTotal.Equal(want) {
		t.Fatalf("RealizedTotal = %s, want %s", snap.RealizedTotal, want)
	}
	if snap.ClosedTrades != 1 || snap.WinningTrades != 1 {
		t.Fatalf("closed=%d wins=%d, want 1/1", snap.ClosedTrades, snap.WinningTrades)
	}
	if pos := snap.Positions["SBER"]; pos.Lots != 0 {
		t.Fatalf("Lots = %d, want 0", pos.Lots)
	}
	if !snap.CommissionsTotal.Equal(dec("0.105")) {
		t.Fatalf("CommissionsTotal = %s, want 0.105", snap.CommissionsTotal)
	}
	if snap.Fills != 2 {
		t.Fatalf("Fills = %d, want 2", snap.Fills)
	}
}

func TestReplayPartialCloseKeepsRemainderAtEntryBasis(t *testing.T) {
	base := time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC)
	snap := Replay([]Fill{
		fill("GAZP", domain.ActionBuy, 2, "100", "0.1", base),
		fill("GAZP", domain.ActionSell, 1, "120", "0.06", base.Add(time.Hour)),
	})

	if !snap.RealizedTotal.Equal(dec("19.89")) {
		t.Fatalf("RealizedTotal = %s, want 19.89", snap.RealizedTotal)
	}
	pos := snap.Positions["GAZP"]
	if pos.Lots != 1 {
		t.Fatalf("Lots = %d, want 1", pos.Lots)
	}
	if !pos.AvgEntry.Equal(dec("100.05")) {
		t.Fatalf("AvgEntry = %s, want 100.05", pos.AvgEntry)
	}
}

func TestReplayShortProfitsWhenPriceFalls(t *testing.T) {
	base := time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC)
	snap := Replay([]Fill{
		fill("LKOH", domain.ActionSell, 1, "200", "0.1", base),
		fill("LKOH", domain.ActionBuy, 1, "180", "0.09", base.Add(time.Hour)),
	})

	if !snap.RealizedTotal.Equal(dec("19.81")) {
		t.Fatalf("RealizedTotal = %s, want 19.81", snap.RealizedTotal)
	}
	if snap.WinningTrades != 1 {
		t.Fatalf("WinningTrades = %d, want 1", snap.WinningTrades)
	}
}

func TestReplayTracksMaxDrawdownOfRealizedCurve(t *testing.T) {
	base := time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC)
	snap := Replay([]Fill{
		fill("A", domain.ActionBuy, 1, "100", "0", base),
		fill("A", domain.ActionSell, 1, "150", "0", base.Add(time.Hour)),
		fill("B", domain.ActionBuy, 1, "100", "0", base.Add(2*time.Hour)),
		fill("B", domain.ActionSell, 1, "70", "0", base.Add(3*time.Hour)),
	})

	if !snap.RealizedTotal.Equal(dec("20")) {
		t.Fatalf("RealizedTotal = %s, want 20", snap.RealizedTotal)
	}
	if !snap.MaxDrawdown.Equal(dec("30")) {
		t.Fatalf("MaxDrawdown = %s, want 30", snap.MaxDrawdown)
	}
}

func TestUnrealizedUsesCloseAndFallsBackToEntry(t *testing.T) {
	base := time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC)
	snap := Replay([]Fill{
		fill("SBER", domain.ActionBuy, 2, "100", "0", base),
		fill("GAZP", domain.ActionBuy, 1, "50", "0", base),
	})

	closes := map[string]decimal.Decimal{"SBER": dec("120")}
	if got := snap.Unrealized(closes); !got.Equal(dec("40")) {
		t.Fatalf("Unrealized = %s, want 40", got)
	}
	if got := snap.GrossExposure(closes); !got.Equal(dec("290")) {
		t.Fatalf("GrossExposure = %s, want 290", got)
	}
	if got := snap.NetExposure(closes); !got.Equal(dec("290")) {
		t.Fatalf("NetExposure = %s, want 290", got)
	}
	if snap.OpenPositions() != 2 {
		t.Fatalf("OpenPositions = %d, want 2", snap.OpenPositions())
	}
	if tickers := snap.OpenTickers(); len(tickers) != 2 || tickers[0] != "GAZP" || tickers[1] != "SBER" {
		t.Fatalf("OpenTickers = %v, want [GAZP SBER]", tickers)
	}
}

func TestReplayIgnoresZeroLotStatusRecords(t *testing.T) {
	base := time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC)
	snap := Replay([]Fill{
		fill("SBER", domain.ActionHold, 0, "100", "0", base),
		fill("SBER", domain.ActionBuy, 1, "100", "0.05", base.Add(time.Minute)),
	})
	if snap.Fills != 1 {
		t.Fatalf("Fills = %d, want 1", snap.Fills)
	}
}
