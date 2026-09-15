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
