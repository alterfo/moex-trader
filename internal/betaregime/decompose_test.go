package betaregime

import (
	"math"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/domain"
)

func TestBuildTradeSamplesSideAndReturns(t *testing.T) {
	trades := []backtest.Trade{
		{
			Ticker: "SBER", Action: domain.ActionBuy, Lots: 100,
			EntryPrice: decimal.NewFromInt(150), OpenedAt: d(2026, 1, 5), ClosedAt: d(2026, 1, 7),
			NetPnl: decimal.NewFromInt(300),
		},
		{
			Ticker: "GAZP", Action: domain.ActionSell, Lots: 10,
			EntryPrice: decimal.NewFromInt(150), OpenedAt: d(2026, 1, 5), ClosedAt: d(2026, 1, 7),
			NetPnl: decimal.NewFromInt(-200),
		},
	}
	imoex := map[time.Time]float64{d(2026, 1, 6): 0.02, d(2026, 1, 7): 0.03}
	samples := BuildTradeSamples(trades, imoex, imoex, imoex)
	if len(samples) != 2 {
		t.Fatalf("got %d samples, want 2", len(samples))
	}
	if samples[0].Side != 1 || samples[1].Side != -1 {
		t.Fatalf("sides = %d/%d, want 1/-1", samples[0].Side, samples[1].Side)
	}
	// IMOEX compounded over (1/5, 1/7] = 1.02*1.03 - 1 = 0.0506
	want := 1.02*1.03 - 1
	if math.Abs(samples[0].ImoexReturn-want) > 1e-12 {
		t.Fatalf("imoex return = %v, want %v", samples[0].ImoexReturn, want)
	}
	// ReturnPct for long: 300 / (150*100) = 0.02
	if math.Abs(samples[0].ReturnPct-0.02) > 1e-12 {
		t.Fatalf("long return = %v, want 0.02", samples[0].ReturnPct)
	}
}

func TestDecomposeLongShortLegs(t *testing.T) {
	samples := []TradeSample{
		{Side: 1, Notional: decimal.NewFromInt(10000), NetPnl: decimal.NewFromInt(500), ImoexReturn: 0.02},
		{Side: -1, Notional: decimal.NewFromInt(5000), NetPnl: decimal.NewFromInt(100), ImoexReturn: 0.02},
	}
	dec := Decompose(samples)

	if dec.Long.Trades != 1 || dec.Short.Trades != 1 {
		t.Fatalf("leg trades = %d/%d", dec.Long.Trades, dec.Short.Trades)
	}
	// Long IMOEX component = +0.02*10000 = 200; excess = 500-200 = 300.
	if !dec.Long.ImoexPnl.Equal(decimal.NewFromInt(200)) || !dec.Long.ExcessPnl.Equal(decimal.NewFromInt(300)) {
		t.Fatalf("long imoex/excess = %s/%s", dec.Long.ImoexPnl, dec.Long.ExcessPnl)
	}
	// Short IMOEX component = -0.02*5000 = -100; excess = 100-(-100) = 200.
	if !dec.Short.ImoexPnl.Equal(decimal.NewFromInt(-100)) || !dec.Short.ExcessPnl.Equal(decimal.NewFromInt(200)) {
		t.Fatalf("short imoex/excess = %s/%s", dec.Short.ImoexPnl, dec.Short.ExcessPnl)
	}
	if !dec.GrossExposure.Equal(decimal.NewFromInt(15000)) {
		t.Fatalf("gross exposure = %s, want 15000", dec.GrossExposure)
	}
	// Return on gross exposure = (500+100)/15000 = 0.04.
	if math.Abs(dec.ReturnOnGrossExposure-0.04) > 1e-12 {
		t.Fatalf("return on gross exposure = %v, want 0.04", dec.ReturnOnGrossExposure)
	}
}
