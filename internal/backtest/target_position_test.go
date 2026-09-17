package backtest

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

type timedTarget struct {
	cutoff time.Time
	before int
	after  int
}

func (t timedTarget) Generate(_ context.Context, feat domain.FeatureContext) (domain.TradeSignal, error) {
	return domain.TradeSignal{Ticker: feat.Ticker, Action: domain.ActionHold, HoldReason: domain.HoldReasonModel}, nil
}

func (t timedTarget) TargetPosition(feat domain.FeatureContext) (int, bool) {
	if feat.GeneratedAt.Before(t.cutoff) {
		return t.before, true
	}
	return t.after, true
}

func TestEngine_TargetPositionClosesToFlat(t *testing.T) {
	candles := benchCandles(120)
	cutoff := time.Date(2024, 3, 20, 0, 0, 0, 0, time.UTC)
	engine, err := NewEngine(Config{
		Tickers:        []string{"TEST"},
		From:           time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
		Till:           time.Date(2024, 1, 30, 0, 0, 0, 0, time.UTC),
		Deposit:        decimal.NewFromInt(100000),
		MaxLots:        100,
		CommissionRate: decimal.Zero,
		KillSwitch:     true,
		SignalSource:   timedTarget{cutoff: cutoff, before: 1, after: 0},
		Source:         fakeSource{candles: candles},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := engine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.ClosedTrades != 1 {
		t.Fatalf("expected exactly one closed trade from the flat close, got %d", result.ClosedTrades)
	}
	if result.Trades[0].Action != domain.ActionBuy {
		t.Fatalf("expected the closed trade to be the opened long, got %s", result.Trades[0].Action)
	}
	if !result.RealizedPnl.Equal(result.Trades[0].NetPnl) {
		t.Fatalf("realized P&L %s should equal the single trade net %s", result.RealizedPnl, result.Trades[0].NetPnl)
	}
}

func TestEngine_TargetPositionFlatIsNoopWithoutPosition(t *testing.T) {
	candles := benchCandles(120)
	engine, err := NewEngine(Config{
		Tickers:        []string{"TEST"},
		From:           time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
		Till:           time.Date(2024, 1, 30, 0, 0, 0, 0, time.UTC),
		Deposit:        decimal.NewFromInt(100000),
		MaxLots:        100,
		CommissionRate: decimal.Zero,
		KillSwitch:     true,
		SignalSource:   timedTarget{cutoff: time.Time{}, before: 0, after: 0},
		Source:         fakeSource{candles: candles},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.ClosedTrades != 0 {
		t.Fatalf("expected no trades for an always-flat target, got %d", result.ClosedTrades)
	}
	if result.Decisions == 0 {
		t.Fatal("expected decisions to still be recorded for the flat target")
	}
}

func TestApplyTargetPositionOpenFlipClose(t *testing.T) {
	engine, err := NewEngine(Config{
		Tickers:        []string{"TEST"},
		Deposit:        decimal.NewFromInt(100000),
		MaxLots:        100,
		CommissionRate: decimal.Zero,
		KillSwitch:     true,
		SignalSource:   &alternatingSignal{},
		Source:         fakeSource{candles: benchCandles(120)},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	price := decimal.NewFromInt(100)
	day := time.Date(2024, 3, 5, 0, 0, 0, 0, time.UTC)

	if err := engine.applyTargetPosition(ctx, "TEST", 2, price, day); err != nil {
		t.Fatal(err)
	}
	if got := currentSignedLots(engine, "TEST"); got != 2 {
		t.Fatalf("after opening long, position = %d, want 2", got)
	}

	if err := engine.applyTargetPosition(ctx, "TEST", 2, price, day); err != nil {
		t.Fatal(err)
	}
	if len(engine.trades) != 0 {
		t.Fatalf("redundant same-target call should not trade, got %d trades", len(engine.trades))
	}

	if err := engine.applyTargetPosition(ctx, "TEST", -2, price, day); err != nil {
		t.Fatal(err)
	}
	if got := currentSignedLots(engine, "TEST"); got != -2 {
		t.Fatalf("after flip, position = %d, want -2", got)
	}
	if len(engine.trades) != 1 {
		t.Fatalf("flip should close the long, got %d trades", len(engine.trades))
	}

	if err := engine.applyTargetPosition(ctx, "TEST", 0, price, day); err != nil {
		t.Fatal(err)
	}
	if got := currentSignedLots(engine, "TEST"); got != 0 {
		t.Fatalf("after flat close, position = %d, want 0", got)
	}
	if len(engine.trades) != 2 {
		t.Fatalf("flat close should close the short, got %d trades", len(engine.trades))
	}
}

func currentSignedLots(engine *Engine, ticker string) int {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	pos := engine.positions[ticker]
	if pos == nil {
		return 0
	}
	if pos.action == domain.ActionSell {
		return -pos.lots
	}
	return pos.lots
}
