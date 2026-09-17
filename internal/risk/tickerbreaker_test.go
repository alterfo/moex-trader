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
