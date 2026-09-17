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

func tradingDayCandles(n int) []moex.Candle {
	candles := make([]moex.Candle, 0, n)
	price := decimal.NewFromInt(100)
	day := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for len(candles) < n {
		if wd := day.Weekday(); wd != time.Saturday && wd != time.Sunday {
			candles = append(candles, moex.Candle{
				Open:   price,
				Close:  price,
				High:   price.Add(decimal.NewFromInt(2)),
				Low:    price.Sub(decimal.NewFromInt(1)),
				Volume: decimal.NewFromInt(1000),
				Begin:  day,
				End:    day.Add(18 * time.Hour),
			})
		}
		day = day.AddDate(0, 0, 1)
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

func TestEngine_ShortBorrowAccruesCalendarDays(t *testing.T) {
	rate := decimal.RequireFromString("0.00005")
	candles := tradingDayCandles(90)
	engine, err := NewEngine(Config{
		Tickers:         []string{"TEST"},
		From:            time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
		Till:            time.Date(2024, 6, 30, 0, 0, 0, 0, time.UTC),
		Deposit:         decimal.NewFromInt(100000),
		MaxLots:         1,
		CommissionRate:  decimal.Zero,
		SpreadPct:       decimal.Zero,
		SlippagePct:     decimal.Zero,
		BorrowPctPerDay: rate,
		KillSwitch:      true,
		SignalSource:    fixedSignal{action: domain.ActionSell},
		Source:          fakeSource{candles: candles},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	first := candles[64].Begin
	last := candles[len(candles)-1].Begin
	calendarDays := int(last.Sub(first).Hours()/24) + 1
	want := decimal.NewFromInt(100).Mul(rate).Mul(decimal.NewFromInt(int64(calendarDays)))
	if !result.TotalBorrow.Equal(want) {
		t.Fatalf("TotalBorrow = %s, want %s (calendar days=%d, trading days=%d)",
			result.TotalBorrow, want, calendarDays, len(candles)-64)
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
