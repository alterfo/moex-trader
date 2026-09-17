package backtest

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

func borrowCandles() []moex.Candle {
	candles := benchCandles(120)
	for i := range candles {
		candles[i].Open = decimal.NewFromInt(100)
		candles[i].Close = decimal.NewFromInt(100)
	}
	return candles
}

func borrowEngine(t *testing.T, action domain.Action, rate decimal.Decimal) *Result {
	t.Helper()
	engine, err := NewEngine(Config{
		Tickers:         []string{"TEST"},
		From:            time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
		Till:            time.Date(2024, 1, 30, 0, 0, 0, 0, time.UTC),
		Deposit:         decimal.NewFromInt(100000),
		MaxLots:         1,
		CommissionRate:  decimal.Zero,
		SpreadPct:       decimal.Zero,
		SlippagePct:     decimal.Zero,
		BorrowPctPerDay: rate,
		KillSwitch:      true,
		SignalSource:    fixedSignal{action: action},
		Source:          fakeSource{candles: borrowCandles()},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestEngine_ShortBorrowAccruesPerOpenDay(t *testing.T) {
	rate := decimal.RequireFromString("0.00005")
	result := borrowEngine(t, domain.ActionSell, rate)

	if !result.TotalBorrow.IsPositive() {
		t.Fatalf("TotalBorrow = %s, want positive", result.TotalBorrow)
	}
	want := decimal.RequireFromString("0.28")
	if !result.TotalBorrow.Equal(want) {
		t.Fatalf("TotalBorrow = %s, want %s", result.TotalBorrow, want)
	}
	if !result.RealizedPnlNetBorrow.Equal(result.RealizedPnl.Sub(result.TotalBorrow)) {
		t.Fatalf("RealizedPnlNetBorrow = %s, want RealizedPnl - TotalBorrow = %s",
			result.RealizedPnlNetBorrow, result.RealizedPnl.Sub(result.TotalBorrow))
	}
}

func TestEngine_BorrowReducesFinalEquity(t *testing.T) {
	rate := decimal.RequireFromString("0.00005")
	withBorrow := borrowEngine(t, domain.ActionSell, rate)
	withoutBorrow := borrowEngine(t, domain.ActionSell, decimal.Zero)

	if !withoutBorrow.TotalBorrow.IsZero() {
		t.Fatalf("TotalBorrow without borrow = %s, want 0", withoutBorrow.TotalBorrow)
	}
	want := withoutBorrow.FinalEquity.Sub(withBorrow.TotalBorrow)
	if !withBorrow.FinalEquity.Equal(want) {
		t.Fatalf("FinalEquity with borrow = %s, want %s", withBorrow.FinalEquity, want)
	}
}

func TestEngine_LongPositionsDoNotAccrueBorrow(t *testing.T) {
	result := borrowEngine(t, domain.ActionBuy, decimal.RequireFromString("0.00005"))
	if !result.TotalBorrow.IsZero() {
		t.Fatalf("TotalBorrow for long-only book = %s, want 0", result.TotalBorrow)
	}
}
