package risk

import (
	"testing"

	"github.com/shopspring/decimal"
)

func breakerTestConfig() TickerBreakerConfig {
	return TickerBreakerConfig{
		MaxConsecutiveLosses: 3,
		MaxCumulativeLossPct: decimal.NewFromFloat(0.05),
		Notional:             decimal.NewFromInt(1000),
	}
}

func buy(ticker string, lots int, price string) Fill {
	return Fill{Ticker: ticker, Action: "BUY", Lots: lots, Price: decimal.RequireFromString(price)}
}

func sell(ticker string, lots int, price string) Fill {
	return Fill{Ticker: ticker, Action: "SELL", Lots: lots, Price: decimal.RequireFromString(price)}
}

func TestTickerBreakerTripsAfterConsecutiveLosses(t *testing.T) {
	b := NewTickerBreaker(breakerTestConfig())

	b.RecordFill(buy("SBER", 1, "100"))
	b.RecordFill(sell("SBER", 1, "90"))
	b.RecordFill(buy("SBER", 1, "100"))
	b.RecordFill(sell("SBER", 1, "90"))
	if b.Blocked("SBER") {
		t.Fatal("Blocked() = true after 2 losses, want false")
	}

	b.RecordFill(buy("SBER", 1, "100"))
	b.RecordFill(sell("SBER", 1, "90"))
	if !b.Blocked("SBER") {
		t.Fatal("Blocked() = false after 3 consecutive losses, want true")
	}
	if got := b.Reason("SBER"); got == "" {
		t.Fatal("Reason() empty for blocked ticker")
	}
}

func TestTickerBreakerResetsLossStreakOnWin(t *testing.T) {
	b := NewTickerBreaker(breakerTestConfig())

	b.RecordFill(buy("SBER", 1, "100"))
	b.RecordFill(sell("SBER", 1, "90"))
	b.RecordFill(buy("SBER", 1, "100"))
	b.RecordFill(sell("SBER", 1, "90"))
	b.RecordFill(buy("SBER", 1, "100"))
	b.RecordFill(sell("SBER", 1, "120"))

	if b.Blocked("SBER") {
		t.Fatal("Blocked() = true after a winning round-trip, want false")
	}
}

func TestTickerBreakerTripsOnCumulativeLoss(t *testing.T) {
	cfg := breakerTestConfig()
	cfg.MaxConsecutiveLosses = 10
	b := NewTickerBreaker(cfg)

	// One losing round-trip: loss of 60 on a 1000 notional exceeds 5% (50).
	b.RecordFill(buy("SBER", 1, "100"))
	b.RecordFill(sell("SBER", 1, "40"))
	if !b.Blocked("SBER") {
		t.Fatal("Blocked() = false after cumulative loss > 5% of notional, want true")
	}
	if got := b.Reason("SBER"); got == "" {
		t.Fatal("Reason() empty for cumulative-loss trip")
	}
}

func TestTickerBreakerShortProfitAndLoss(t *testing.T) {
	cfg := breakerTestConfig()
	cfg.MaxConsecutiveLosses = 3
	b := NewTickerBreaker(cfg)

	// Short at 100, cover at 90: profit, not a loss.
	b.RecordFill(sell("GAZP", 1, "100"))
	b.RecordFill(buy("GAZP", 1, "90"))
	_, realized, consecutive, tripped, _ := b.State("GAZP")
	if !realized.IsPositive() || consecutive != 0 || tripped {
		t.Fatalf("State() = realized %s, consecutive %d, tripped %v; want profit, 0, false", realized, consecutive, tripped)
	}

	// Short at 100, cover at 110: loss.
	b.RecordFill(sell("GAZP", 1, "100"))
	b.RecordFill(buy("GAZP", 1, "110"))
	_, realized, consecutive, tripped, _ = b.State("GAZP")
	if realized.IsPositive() || consecutive != 1 || tripped {
		t.Fatalf("State() = realized %s, consecutive %d, tripped %v; want loss, 1, false", realized, consecutive, tripped)
	}
}

func TestTickerBreakerAddsToPositionWithAverageCost(t *testing.T) {
	b := NewTickerBreaker(breakerTestConfig())

	b.RecordFill(buy("SBER", 1, "100"))
	b.RecordFill(buy("SBER", 1, "200"))
	openLots, realized, _, _, _ := b.State("SBER")
	if openLots != 2 || !realized.IsZero() {
		t.Fatalf("State() openLots = %d, realized = %s; want 2 and zero", openLots, realized)
	}

	// Sell one lot at the average cost of 150: no realized P&L.
	b.RecordFill(sell("SBER", 1, "150"))
	openLots, realized, _, _, _ = b.State("SBER")
	if openLots != 1 || !realized.IsZero() {
		t.Fatalf("State() openLots = %d, realized = %s; want 1 and zero", openLots, realized)
	}
}

func TestTickerBreakerCountsEntryCommission(t *testing.T) {
	b := NewTickerBreaker(breakerTestConfig())

	b.RecordFill(Fill{Ticker: "SBER", Action: "BUY", Lots: 1, Price: decimal.NewFromInt(100), Commission: decimal.NewFromInt(1)})
	b.RecordFill(Fill{Ticker: "SBER", Action: "SELL", Lots: 1, Price: decimal.NewFromInt(100), Commission: decimal.NewFromInt(1)})

	_, realized, _, _, _ := b.State("SBER")
	want := decimal.NewFromInt(-2)
	if !realized.Equal(want) {
		t.Fatalf("State() realized = %s, want %s (entry + exit commission)", realized, want)
	}
}

func TestTickerBreakerCountsShortEntryCommission(t *testing.T) {
	b := NewTickerBreaker(breakerTestConfig())

	// Short at 100 and cover at 100 with 1 commission each: realized is -2
	// (entry + exit commission), not zero or a commission credit.
	b.RecordFill(Fill{Ticker: "SBER", Action: "SELL", Lots: 1, Price: decimal.NewFromInt(100), Commission: decimal.NewFromInt(1)})
	b.RecordFill(Fill{Ticker: "SBER", Action: "BUY", Lots: 1, Price: decimal.NewFromInt(100), Commission: decimal.NewFromInt(1)})

	_, realized, consecutive, _, _ := b.State("SBER")
	if want := decimal.NewFromInt(-2); !realized.Equal(want) {
		t.Fatalf("State() realized = %s, want %s (entry + exit commission)", realized, want)
	}
	if consecutive != 1 {
		t.Fatalf("State() consecutive = %d, want 1", consecutive)
	}
}

func TestTickerBreakerReversalSplitsCommission(t *testing.T) {
	b := NewTickerBreaker(breakerTestConfig())

	// Open 5 long at 100 with 5 commission (1 per lot): average entry 101.
	b.RecordFill(Fill{Ticker: "SBER", Action: "BUY", Lots: 5, Price: decimal.NewFromInt(100), Commission: decimal.NewFromInt(5)})

	// Reversal: sell 8 at 110 with 8 commission (1 per lot). It closes the
	// 5-lot long, realizing (110-101)*5 minus the closed leg's commission
	// (5), and opens a 3-lot short with a commission-inclusive basis of 109.
	b.RecordFill(Fill{Ticker: "SBER", Action: "SELL", Lots: 8, Price: decimal.NewFromInt(110), Commission: decimal.NewFromInt(8)})

	openLots, realized, consecutive, tripped, _ := b.State("SBER")
	if openLots != -3 {
		t.Fatalf("State() openLots = %d, want -3", openLots)
	}
	if want := decimal.NewFromInt(40); !realized.Equal(want) {
		t.Fatalf("State() realized after reversal = %s, want %s", realized, want)
	}
	if consecutive != 0 || tripped {
		t.Fatalf("State() consecutive = %d, tripped = %v; want 0, false", consecutive, tripped)
	}

	// Cover the short at its commission-inclusive basis of 109; the exit
	// commission is the only remaining cost.
	b.RecordFill(Fill{Ticker: "SBER", Action: "BUY", Lots: 3, Price: decimal.NewFromInt(109), Commission: decimal.NewFromInt(3)})
	openLots, realized, _, _, _ = b.State("SBER")
	if openLots != 0 {
		t.Fatalf("State() openLots after cover = %d, want 0", openLots)
	}
	if want := decimal.NewFromInt(37); !realized.Equal(want) {
		t.Fatalf("State() realized after cover = %s, want %s", realized, want)
	}
}

func TestTickerBreakerResetClearsTripOnly(t *testing.T) {
	b := NewTickerBreaker(breakerTestConfig())

	b.RecordFill(buy("SBER", 1, "100"))
	b.RecordFill(sell("SBER", 1, "90"))
	b.RecordFill(buy("SBER", 1, "100"))
	b.RecordFill(sell("SBER", 1, "90"))
	b.RecordFill(buy("SBER", 1, "100"))
	b.RecordFill(sell("SBER", 1, "90"))
	if !b.Blocked("SBER") {
		t.Fatal("Blocked() = false after 3 losses, want true")
	}

	b.Reset("SBER")
	if b.Blocked("SBER") {
		t.Fatal("Blocked() = true after Reset, want false")
	}
	if got := b.Reason("SBER"); got != "" {
		t.Fatalf("Reason() = %q after Reset, want empty", got)
	}
	_, realized, consecutive, _, _ := b.State("SBER")
	if !realized.IsNegative() {
		t.Fatalf("Reset() discarded realized P&L: %s", realized)
	}
	if consecutive != 0 {
		t.Fatalf("Reset() consecutive losses = %d, want 0", consecutive)
	}
}

func TestTickerBreakerIgnoresNonTradeActionsAndInvalidFills(t *testing.T) {
	b := NewTickerBreaker(breakerTestConfig())

	b.RecordFill(Fill{Ticker: "SBER", Action: "HOLD", Lots: 1, Price: decimal.NewFromInt(100)})
	b.RecordFill(Fill{Ticker: "SBER", Action: "BUY", Lots: 0, Price: decimal.NewFromInt(100)})
	b.RecordFill(Fill{Ticker: "SBER", Action: "BUY", Lots: 1, Price: decimal.Zero})
	if _, realized, _, tripped, _ := b.State("SBER"); !realized.IsZero() || tripped {
		t.Fatalf("State() realized = %s, tripped = %v; want zero and false", realized, tripped)
	}
}

func TestTickerBreakerTickerNormalization(t *testing.T) {
	b := NewTickerBreaker(breakerTestConfig())
	b.RecordFill(buy("sber", 1, "100"))
	if openLots, _, _, _, _ := b.State("SBER"); openLots != 1 {
		t.Fatalf("State(\"SBER\") openLots = %d, want 1 (fill recorded under \"sber\" should normalize)", openLots)
	}
}
