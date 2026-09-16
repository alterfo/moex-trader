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
