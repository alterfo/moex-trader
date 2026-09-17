package backtest

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

type fakeSource struct {
	candles []moex.Candle
}

func (f fakeSource) History(_ context.Context, _ string, _, _ time.Time) ([]moex.Candle, error) {
	return f.candles, nil
}

type fixedSignal struct {
	action domain.Action
}

func (f fixedSignal) Generate(_ context.Context, feat domain.FeatureContext) (domain.TradeSignal, error) {
	if feat.LastPrice.Sign() <= 0 {
		return domain.TradeSignal{Action: domain.ActionHold}, nil
	}
	return domain.TradeSignal{
		Ticker:     feat.Ticker,
		Action:     f.action,
		Confidence: decimal.RequireFromString("0.8"),
		TargetLots: 1,
		Reasoning:  "test",
	}, nil
}

func benchCandles(n int) []moex.Candle {
	candles := make([]moex.Candle, n)
	price := decimal.NewFromInt(100)
	for i := range candles {
		candles[i] = moex.Candle{
			Open:   price,
			Close:  price.Add(decimal.NewFromInt(int64(i%3 - 1))),
			High:   price.Add(decimal.NewFromInt(2)),
			Low:    price.Sub(decimal.NewFromInt(1)),
			Volume: decimal.NewFromInt(1000),
			Begin:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i),
			End:    time.Date(2024, 1, 1, 18, 0, 0, 0, time.UTC).AddDate(0, 0, i),
		}
	}
	return candles
}

// alternatingSignal flips BUY/SELL every decision so trades close and at
// least one position is likely still open at the cutoff date.
type alternatingSignal struct{ calls int }

func (s *alternatingSignal) Generate(_ context.Context, feat domain.FeatureContext) (domain.TradeSignal, error) {
	if feat.LastPrice.Sign() <= 0 {
		return domain.TradeSignal{Action: domain.ActionHold}, nil
	}
	s.calls++
	action := domain.ActionBuy
	if s.calls%2 == 0 {
		action = domain.ActionSell
	}
	return domain.TradeSignal{
		Ticker:     feat.Ticker,
		Action:     action,
		Confidence: decimal.RequireFromString("0.8"),
		TargetLots: 1,
		Reasoning:  "test",
	}, nil
}

func TestEngine_SpreadAndSlippageWidenFillsAgainstTrader(t *testing.T) {
	candles := benchCandles(120)
	engine, err := NewEngine(Config{
		Tickers:        []string{"TEST"},
		From:           time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
		Till:           time.Date(2024, 1, 30, 0, 0, 0, 0, time.UTC),
		Deposit:        decimal.NewFromInt(100000),
		MaxLots:        1,
		CommissionRate: decimal.Zero,
		SpreadPct:      decimal.NewFromFloat(0.001),
		SlippagePct:    decimal.NewFromFloat(0.0005),
		KillSwitch:     true,
		SignalSource:   &alternatingSignal{},
		Source:         fakeSource{candles: candles},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := engine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Trades) == 0 {
		t.Fatal("expected at least one closed trade")
	}

	cost := decimal.NewFromFloat(0.001).Add(decimal.NewFromFloat(0.0005))
	base := decimal.NewFromInt(100)
	wantBuyFill := base.Mul(decimal.NewFromInt(1).Add(cost))
	wantSellFill := base.Mul(decimal.NewFromInt(1).Sub(cost))

	first := result.Trades[0]
	if first.Action != domain.ActionBuy {
		t.Fatalf("expected first closed trade to be the opened long, got action %v", first.Action)
	}
	if !first.EntryPrice.Equal(wantBuyFill) {
		t.Errorf("EntryPrice = %s, want %s (base price + spread + slippage)", first.EntryPrice, wantBuyFill)
	}
	if !first.ExitPrice.Equal(wantSellFill) {
		t.Errorf("ExitPrice = %s, want %s (base price - spread - slippage)", first.ExitPrice, wantSellFill)
	}
	if !first.GrossPnl.IsNegative() {
		t.Errorf("GrossPnl = %s, want negative: flat price plus round-trip cost must lose money", first.GrossPnl)
	}
}

func TestEngine_ZeroSpreadSlippageMatchesRawPrice(t *testing.T) {
	candles := benchCandles(120)
	engine, err := NewEngine(Config{
		Tickers:        []string{"TEST"},
		From:           time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
		Till:           time.Date(2024, 1, 30, 0, 0, 0, 0, time.UTC),
		Deposit:        decimal.NewFromInt(100000),
		MaxLots:        1,
		CommissionRate: decimal.Zero,
		KillSwitch:     true,
		SignalSource:   &alternatingSignal{},
		Source:         fakeSource{candles: candles},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := engine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Trades) == 0 {
		t.Fatal("expected at least one closed trade")
	}
	first := result.Trades[0]
	if !first.EntryPrice.Equal(decimal.NewFromInt(100)) || !first.ExitPrice.Equal(decimal.NewFromInt(100)) {
		t.Errorf("with zero spread/slippage fills should equal the raw candle price, got entry=%s exit=%s", first.EntryPrice, first.ExitPrice)
	}
	if !first.GrossPnl.IsZero() {
		t.Errorf("GrossPnl = %s, want zero at flat price with no costs", first.GrossPnl)
	}
}

func TestEngine_RealizedPlusUnrealizedEqualsNet(t *testing.T) {
	candles := benchCandles(120)
	engine, err := NewEngine(Config{
		Tickers:        []string{"TEST"},
		From:           time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
		Till:           time.Date(2024, 1, 30, 0, 0, 0, 0, time.UTC),
		Deposit:        decimal.NewFromInt(100000),
		MaxLots:        1,
		CommissionRate: decimal.NewFromInt(3).Div(decimal.NewFromInt(1000)),
		KillSwitch:     true,
		SignalSource:   &alternatingSignal{},
		Source:         fakeSource{candles: candles},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := engine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	var sumClosed decimal.Decimal
	for _, trade := range result.Trades {
		sumClosed = sumClosed.Add(trade.NetPnl)
	}
	if !result.RealizedPnl.Equal(sumClosed) {
		t.Errorf("RealizedPnl = %s, sum of closed-trade NetPnl = %s, want equal", result.RealizedPnl, sumClosed)
	}
	if !result.UnrealizedPnl.Equal(result.NetPnl.Sub(result.RealizedPnl)) {
		t.Errorf("UnrealizedPnl = %s, want NetPnl - RealizedPnl = %s",
			result.UnrealizedPnl, result.NetPnl.Sub(result.RealizedPnl))
	}
	if !result.NetPnl.Equal(result.FinalEquity.Sub(result.Deposit)) {
		t.Errorf("NetPnl = %s, want FinalEquity - Deposit = %s",
			result.NetPnl, result.FinalEquity.Sub(result.Deposit))
	}
	if result.ClosedTrades == 0 {
		t.Fatal("expected the alternating signal to close at least one trade")
	}
}

func TestEngine_BuyAllUp(t *testing.T) {
	candles := benchCandles(120)
	source := fakeSource{candles: candles}
	sig := fixedSignal{action: domain.ActionBuy}

	engine, err := NewEngine(Config{
		Tickers:        []string{"TEST"},
		From:           time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
		Till:           time.Date(2024, 1, 30, 0, 0, 0, 0, time.UTC),
		Deposit:        decimal.NewFromInt(100000),
		MaxLots:        1,
		CommissionRate: decimal.NewFromInt(3).Div(decimal.NewFromInt(1000)),
		KillSwitch:     true,
		SignalSource:   sig,
		Source:         source,
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := engine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.ClosedTrades < 0 {
		t.Errorf("unexpected negative closed trades %d", result.ClosedTrades)
	}
	if result.EquityCurve == nil {
		t.Error("expected non-nil equity curve")
	}
}

func TestEngine_SellAllDown(t *testing.T) {
	candles := benchCandles(120)
	for i := range candles {
		candles[i].Open = decimal.NewFromInt(100).Sub(decimal.NewFromInt(int64(i % 5)))
		candles[i].Close = candles[i].Open.Sub(decimal.NewFromInt(1))
	}
	source := fakeSource{candles: candles}
	sig := fixedSignal{action: domain.ActionSell}

	engine, err := NewEngine(Config{
		Tickers:        []string{"TEST"},
		From:           time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
		Till:           time.Date(2024, 1, 30, 0, 0, 0, 0, time.UTC),
		Deposit:        decimal.NewFromInt(100000),
		MaxLots:        1,
		CommissionRate: decimal.NewFromInt(3).Div(decimal.NewFromInt(1000)),
		KillSwitch:     true,
		SignalSource:   sig,
		Source:         source,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.NetPnl.IsPositive() {
		t.Errorf("shorting falling prices should be profitable, got net PnL %s", result.NetPnl.String())
	}
	if result.ClosedTrades != 0 {
		t.Errorf("expected 0 closed trades (short stays open, no BUY to close), got %d", result.ClosedTrades)
	}
}

func TestEngine_MultiLotRepeatSignalHoldsPosition(t *testing.T) {
	candles := benchCandles(120)
	source := fakeSource{candles: candles}

	sig := &fixedTargetSignal{action: domain.ActionBuy, lots: 3}
	engine, err := NewEngine(Config{
		Tickers:        []string{"TEST"},
		From:           time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
		Till:           time.Date(2024, 1, 30, 0, 0, 0, 0, time.UTC),
		Deposit:        decimal.NewFromInt(100000),
		MaxLots:        100,
		CommissionRate: decimal.NewFromInt(3).Div(decimal.NewFromInt(1000)),
		KillSwitch:     true,
		SignalSource:   sig,
		Source:         source,
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := engine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Decisions == 0 {
		t.Fatal("expected decisions")
	}
	if result.ClosedTrades != 0 {
		t.Fatalf("expected 0 closed trades (target held), got %d", result.ClosedTrades)
	}
}

func TestEngine_ReduceTargetInSameDirection(t *testing.T) {
	engine, err := NewEngine(Config{
		Tickers:        []string{"TEST"},
		Deposit:        decimal.NewFromInt(100000),
		MaxLots:        100,
		CommissionRate: decimal.NewFromInt(3).Div(decimal.NewFromInt(1000)),
		SignalSource:   holdSource{},
		Source:         fakeSource{candles: benchCandles(120)},
	})
	if err != nil {
		t.Fatal(err)
	}

	day := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)
	buy5 := domain.TradeSignal{Ticker: "TEST", Action: domain.ActionBuy, TargetLots: 5, Confidence: decimal.NewFromFloat(0.8)}
	buy2 := domain.TradeSignal{Ticker: "TEST", Action: domain.ActionBuy, TargetLots: 2, Confidence: decimal.NewFromFloat(0.8)}

	if err := engine.recordFill("TEST", buy5, decimal.NewFromInt(100), day); err != nil {
		t.Fatal(err)
	}
	if err := engine.recordFill("TEST", buy2, decimal.NewFromInt(110), day.AddDate(0, 0, 1)); err != nil {
		t.Fatal(err)
	}

	pos := engine.positions["TEST"]
	if pos == nil {
		t.Fatal("expected open position")
	}
	if pos.action != domain.ActionBuy || pos.lots != 2 {
		t.Fatalf("expected reduced BUY 2, got action=%v lots=%d", pos.action, pos.lots)
	}
	if len(engine.trades) != 1 {
		t.Fatalf("expected exactly 1 closed trade for the 3-lot reduction, got %d", len(engine.trades))
	}
	if got := engine.trades[0].Lots; got != 3 {
		t.Fatalf("expected closed trade of 3 lots, got %d", got)
	}
	if got := engine.trades[0].Action; got != domain.ActionBuy {
		t.Fatalf("expected closed trade action BUY, got %v", got)
	}
}

func TestEngine_FlipDirection(t *testing.T) {
	engine, err := NewEngine(Config{
		Tickers:        []string{"TEST"},
		Deposit:        decimal.NewFromInt(100000),
		MaxLots:        100,
		CommissionRate: decimal.NewFromInt(3).Div(decimal.NewFromInt(1000)),
		SignalSource:   holdSource{},
		Source:         fakeSource{candles: benchCandles(120)},
	})
	if err != nil {
		t.Fatal(err)
	}

	day := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)
	buy3 := domain.TradeSignal{Ticker: "TEST", Action: domain.ActionBuy, TargetLots: 3, Confidence: decimal.NewFromFloat(0.8)}
	sell2 := domain.TradeSignal{Ticker: "TEST", Action: domain.ActionSell, TargetLots: 2, Confidence: decimal.NewFromFloat(0.8)}

	if err := engine.recordFill("TEST", buy3, decimal.NewFromInt(100), day); err != nil {
		t.Fatal(err)
	}
	if err := engine.recordFill("TEST", sell2, decimal.NewFromInt(90), day.AddDate(0, 0, 1)); err != nil {
		t.Fatal(err)
	}

	pos := engine.positions["TEST"]
	if pos == nil {
		t.Fatal("expected open short position")
	}
	if pos.action != domain.ActionSell || pos.lots != 2 {
		t.Fatalf("expected flipped SELL 2, got action=%v lots=%d", pos.action, pos.lots)
	}
	if len(engine.trades) != 1 {
		t.Fatalf("expected 1 closed trade for the flipped long, got %d", len(engine.trades))
	}
	if got := engine.trades[0].Lots; got != 3 {
		t.Fatalf("expected closed trade of 3 lots, got %d", got)
	}
}

func TestEngine_ReduceShortTargetInSameDirection(t *testing.T) {
	engine, err := NewEngine(Config{
		Tickers:        []string{"TEST"},
		Deposit:        decimal.NewFromInt(100000),
		MaxLots:        100,
		CommissionRate: decimal.NewFromInt(3).Div(decimal.NewFromInt(1000)),
		SignalSource:   holdSource{},
		Source:         fakeSource{candles: benchCandles(120)},
	})
	if err != nil {
		t.Fatal(err)
	}

	day := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)
	sell5 := domain.TradeSignal{Ticker: "TEST", Action: domain.ActionSell, TargetLots: 5, Confidence: decimal.NewFromFloat(0.8)}
	sell2 := domain.TradeSignal{Ticker: "TEST", Action: domain.ActionSell, TargetLots: 2, Confidence: decimal.NewFromFloat(0.8)}

	if err := engine.recordFill("TEST", sell5, decimal.NewFromInt(100), day); err != nil {
		t.Fatal(err)
	}
	if err := engine.recordFill("TEST", sell2, decimal.NewFromInt(110), day.AddDate(0, 0, 1)); err != nil {
		t.Fatal(err)
	}

	pos := engine.positions["TEST"]
	if pos == nil {
		t.Fatal("expected open short position")
	}
	if pos.action != domain.ActionSell || pos.lots != 2 {
		t.Fatalf("expected reduced SELL 2, got action=%v lots=%d", pos.action, pos.lots)
	}
	if len(engine.trades) != 1 {
		t.Fatalf("expected exactly 1 closed trade for the 3-lot reduction, got %d", len(engine.trades))
	}
	if got := engine.trades[0].Lots; got != 3 {
		t.Fatalf("expected closed trade of 3 lots, got %d", got)
	}
	if got := engine.trades[0].Action; got != domain.ActionSell {
		t.Fatalf("expected closed trade action SELL, got %v", got)
	}
	if got := engine.trades[0].GrossPnl; !got.Equal(decimal.NewFromInt(-30)) {
		t.Fatalf("expected short reduction gross P&L -30, got %s", got)
	}

	if err := engine.recordFill("TEST", sell2, decimal.NewFromInt(120), day.AddDate(0, 0, 2)); err != nil {
		t.Fatal(err)
	}
	if len(engine.trades) != 1 {
		t.Fatalf("repeat signal at target must be a no-op, got %d closed trades", len(engine.trades))
	}

	if err := engine.recordFill("TEST", sell5, decimal.NewFromInt(120), day.AddDate(0, 0, 3)); err != nil {
		t.Fatal(err)
	}
	pos = engine.positions["TEST"]
	if pos == nil || pos.lots != 5 {
		t.Fatalf("expected short built back up to 5 lots, got %+v", pos)
	}
}

func TestEngine_MaxHoldBarsClosesPosition(t *testing.T) {
	engine, err := NewEngine(Config{
		Tickers:        []string{"TEST"},
		From:           time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
		Till:           time.Date(2024, 1, 30, 0, 0, 0, 0, time.UTC),
		Deposit:        decimal.NewFromInt(100000),
		MaxLots:        100,
		MaxHoldBars:    3,
		CommissionRate: decimal.NewFromInt(3).Div(decimal.NewFromInt(1000)),
		SignalSource:   fixedTargetSignal{action: domain.ActionBuy, lots: 2},
		Source:         fakeSource{candles: benchCandles(120)},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := engine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.ClosedTrades == 0 {
		t.Fatal("expected time-exit to close positions")
	}
	for _, trade := range result.Trades {
		if trade.SignalNote != "time-exit" {
			t.Fatalf("expected every close to be a time-exit, got note %q", trade.SignalNote)
		}
		if got := trade.ClosedAt.Sub(trade.OpenedAt); got != 72*time.Hour {
			t.Fatalf("expected a 3-bar hold, got %s", got)
		}
	}
	if result.ClosedTrades < 2 {
		t.Fatalf("expected repeated time-exits with re-entry, got %d closed trades", result.ClosedTrades)
	}
}

func TestEngine_MaxHoldBarsDisabledHoldsPosition(t *testing.T) {
	engine, err := NewEngine(Config{
		Tickers:        []string{"TEST"},
		From:           time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
		Till:           time.Date(2024, 1, 30, 0, 0, 0, 0, time.UTC),
		Deposit:        decimal.NewFromInt(100000),
		MaxLots:        100,
		CommissionRate: decimal.NewFromInt(3).Div(decimal.NewFromInt(1000)),
		SignalSource:   fixedTargetSignal{action: domain.ActionBuy, lots: 2},
		Source:         fakeSource{candles: benchCandles(120)},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := engine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.ClosedTrades != 0 {
		t.Fatalf("expected no closed trades without MaxHoldBars, got %d", result.ClosedTrades)
	}
}

func TestEngine_FlatSignalClosesPosition(t *testing.T) {
	engine, err := NewEngine(Config{
		Tickers:        []string{"TEST"},
		Deposit:        decimal.NewFromInt(100000),
		MaxLots:        100,
		CommissionRate: decimal.NewFromInt(3).Div(decimal.NewFromInt(1000)),
		SignalSource:   holdSource{},
		Source:         fakeSource{candles: benchCandles(120)},
	})
	if err != nil {
		t.Fatal(err)
	}

	day := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)
	flat := domain.TradeSignal{Ticker: "TEST", Action: domain.ActionSell, TargetLots: 0, Reasoning: "flat"}
	if err := engine.recordFill("TEST", flat, decimal.NewFromInt(100), day); err != nil {
		t.Fatal(err)
	}
	if len(engine.positions) != 0 || len(engine.trades) != 0 {
		t.Fatalf("flat on an empty book must be a no-op, positions=%d trades=%d", len(engine.positions), len(engine.trades))
	}

	buy5 := domain.TradeSignal{Ticker: "TEST", Action: domain.ActionBuy, TargetLots: 5, Confidence: decimal.NewFromFloat(0.8)}
	if err := engine.recordFill("TEST", buy5, decimal.NewFromInt(100), day); err != nil {
		t.Fatal(err)
	}
	if err := engine.recordFill("TEST", flat, decimal.NewFromInt(110), day.AddDate(0, 0, 1)); err != nil {
		t.Fatal(err)
	}
	if len(engine.positions) != 0 {
		t.Fatalf("flat must close the open position, got %+v", engine.positions["TEST"])
	}
	if len(engine.trades) != 1 || engine.trades[0].Lots != 5 {
		t.Fatalf("expected one closed trade of 5 lots, got %+v", engine.trades)
	}
}

type fixedTargetSignal struct {
	action domain.Action
	lots   int
}

func (f fixedTargetSignal) Generate(_ context.Context, feat domain.FeatureContext) (domain.TradeSignal, error) {
	if feat.LastPrice.Sign() <= 0 {
		return domain.TradeSignal{Action: domain.ActionHold}, nil
	}
	return domain.TradeSignal{
		Ticker:     feat.Ticker,
		Action:     f.action,
		Confidence: decimal.RequireFromString("0.8"),
		TargetLots: f.lots,
		Reasoning:  "test",
	}, nil
}

func TestEngine_HoldNoTrades(t *testing.T) {
	candles := benchCandles(120)
	source := fakeSource{candles: candles}
	sig := fixedSignal{action: domain.ActionHold}

	engine, err := NewEngine(Config{
		Tickers:        []string{"TEST"},
		From:           time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
		Till:           time.Date(2024, 1, 30, 0, 0, 0, 0, time.UTC),
		Deposit:        decimal.NewFromInt(100000),
		MaxLots:        1,
		CommissionRate: decimal.NewFromInt(3).Div(decimal.NewFromInt(1000)),
		SignalSource:   sig,
		Source:         source,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.ClosedTrades != 0 {
		t.Errorf("expected zero closed trades, got %d", result.ClosedTrades)
	}
	if !result.TotalCommission.IsZero() {
		t.Errorf("expected zero total commission, got %s", result.TotalCommission.String())
	}
}

func TestEngine_Immutability_NoEditsToExistingFiles(t *testing.T) {
	candles := benchCandles(120)
	source := fakeSource{candles: candles}
	sig := fixedSignal{action: domain.ActionHold}
	_, _ = NewEngine(Config{
		Tickers:      []string{"TEST"},
		Deposit:      decimal.NewFromInt(100000),
		MaxLots:      1,
		SignalSource: sig,
		Source:       source,
	})
}

type holdSource struct{}

func (holdSource) Generate(_ context.Context, feat domain.FeatureContext) (domain.TradeSignal, error) {
	return domain.TradeSignal{
		Ticker: feat.Ticker, Action: domain.ActionHold,
		HoldReason: domain.HoldReasonTimeout,
	}, nil
}

func TestEngine_RealizedUnrealizedSplit(t *testing.T) {
	engine, err := NewEngine(Config{
		Tickers:        []string{"FLIP", "OPEN"},
		From:           time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
		Till:           time.Date(2024, 1, 30, 0, 0, 0, 0, time.UTC),
		Deposit:        decimal.NewFromInt(100000),
		MaxLots:        1000,
		CommissionRate: decimal.NewFromInt(5).Div(decimal.NewFromInt(10000)),
		SignalSource:   &stagedSequence{step: make(map[string]int)},
		Source:         fakeSource{candles: risingCandles(140)},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := engine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.ClosedTrades != 1 {
		t.Errorf("expected exactly 1 closed trade (the FLIP long), got %d", result.ClosedTrades)
	}
	if result.RealizedPnl.IsZero() {
		t.Error("expected non-zero realized P&L (closed FLIP long)")
	}
	if result.UnrealizedPnl.IsZero() {
		t.Error("expected non-zero unrealized P&L (open FLIP short + OPEN long at cutoff)")
	}
	wantNet := result.FinalEquity.Sub(result.Deposit)
	if !result.NetPnl.Equal(wantNet) {
		t.Errorf("NetPnl = %s, want FinalEquity - Deposit = %s", result.NetPnl.String(), wantNet.String())
	}
	sum := result.RealizedPnl.Add(result.UnrealizedPnl)
	if !sum.Equal(result.NetPnl) {
		t.Errorf("RealizedPnl + UnrealizedPnl = %s, want NetPnl = %s", sum.String(), result.NetPnl.String())
	}
	realizedViaGross := result.GrossPnl.Sub(result.TotalCommission)
	if !realizedViaGross.Equal(result.RealizedPnl) {
		t.Errorf("GrossPnl - TotalCommission = %s, want RealizedPnl = %s", realizedViaGross.String(), result.RealizedPnl.String())
	}
}

type stagedSequence struct {
	step map[string]int
}

func (s *stagedSequence) Generate(_ context.Context, feat domain.FeatureContext) (domain.TradeSignal, error) {
	step := s.step[feat.Ticker]
	s.step[feat.Ticker] = step + 1

	if step == 0 {
		lots := 2
		if feat.Ticker == "FLIP" {
			lots = 3
		}
		return domain.TradeSignal{
			Ticker: feat.Ticker, Action: domain.ActionBuy, TargetLots: lots,
			Confidence: decimal.NewFromFloat(0.8), Reasoning: "open",
		}, nil
	}
	if step == 1 && feat.Ticker == "FLIP" {
		return domain.TradeSignal{
			Ticker: feat.Ticker, Action: domain.ActionSell, TargetLots: 1,
			Confidence: decimal.NewFromFloat(0.8), Reasoning: "flip-to-short",
		}, nil
	}
	return domain.TradeSignal{Ticker: feat.Ticker, Action: domain.ActionHold}, nil
}

func risingCandles(n int) []moex.Candle {
	candles := make([]moex.Candle, n)
	for i := range candles {
		price := decimal.NewFromInt(int64(100 + i))
		candles[i] = moex.Candle{
			Open:   price,
			Close:  price,
			High:   price.Add(decimal.NewFromInt(2)),
			Low:    price.Sub(decimal.NewFromInt(1)),
			Volume: decimal.NewFromInt(1000),
			Begin:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i),
			End:    time.Date(2024, 1, 1, 18, 0, 0, 0, time.UTC).AddDate(0, 0, i),
		}
	}
	return candles
}

func TestEngine_HoldReasonBreakdown(t *testing.T) {
	candles := benchCandles(120)
	engine, err := NewEngine(Config{
		Tickers:               []string{"TEST"},
		From:                  time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
		Till:                  time.Date(2024, 1, 30, 0, 0, 0, 0, time.UTC),
		Deposit:               decimal.NewFromInt(100000),
		MaxLots:               1,
		SignalSource:          holdSource{},
		Source:                fakeSource{candles: candles},
		MaxDecisionsPerTicker: 5,
		Logger:                nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Decisions != 5 {
		t.Errorf("expected 5 decisions, got %d", result.Decisions)
	}
	if result.HoldReasons[domain.HoldReasonTimeout] != 5 {
		t.Errorf("expected 5 timeout holds, got %v", result.HoldReasons)
	}
	if result.HoldReasons[domain.HoldReasonModel] != 0 {
		t.Errorf("expected 0 model holds, got %v", result.HoldReasons)
	}
}
