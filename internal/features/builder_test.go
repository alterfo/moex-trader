package features

import (
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/news"
)

func TestBuildFullData(t *testing.T) {
	now := time.Date(2024, 1, 11, 12, 0, 0, 0, time.UTC)
	builder := NewBuilder(func() time.Time { return now })

	input := Input{
		Ticker: "SBER",
		Price: PriceSnapshot{
			LastPrice: decimal.NewFromFloat(110),
			PrevClose: decimal.NewFromFloat(100),
			Bid:       decimal.NewFromFloat(109.9),
			Ask:       decimal.NewFromFloat(110.1),
		},
		Candles: []moex.Candle{
			{Close: decimal.NewFromFloat(100)},
			{Close: decimal.NewFromFloat(110)},
			{Close: decimal.NewFromFloat(105)},
		},
		News: []news.MatchedArticle{
			{
				Ticker:      "SBER",
				Title:       "Прибыль Сбербанка выросла",
				Description: "Позитивный отчет",
				TrustWeight: decimal.NewFromFloat(0.8),
			},
			{
				Ticker:      "SBER",
				Title:       "Сбербанк получил штраф",
				Description: "Негативная новость",
				TrustWeight: decimal.NewFromFloat(0.5),
			},
		},
	}

	ctx, err := builder.Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if ctx.Ticker != "SBER" {
		t.Fatalf("unexpected ticker %q", ctx.Ticker)
	}
	if !ctx.GeneratedAt.Equal(now) {
		t.Fatalf("unexpected generated at %v", ctx.GeneratedAt)
	}
	if !ctx.LastPrice.Equal(decimal.NewFromFloat(110)) {
		t.Fatalf("unexpected last price %s", ctx.LastPrice)
	}
	if !ctx.ReturnPct.Equal(decimal.NewFromFloat(10)) {
		t.Fatalf("unexpected return %s", ctx.ReturnPct)
	}
	if ctx.NewsCount != 2 {
		t.Fatalf("unexpected news count %d", ctx.NewsCount)
	}
	wantSentiment := decimal.NewFromFloat(0.23076923076923078)
	if ctx.NewsSentiment.Sub(wantSentiment).Abs().GreaterThan(decimal.NewFromFloat(0.000000000000001)) {
		t.Fatalf("unexpected news sentiment %s", ctx.NewsSentiment)
	}
	if !ctx.RealizedVolatility.GreaterThan(decimal.Zero) {
		t.Fatalf("expected positive volatility, got %s", ctx.RealizedVolatility)
	}
	if !ctx.OrderBookImbalance.IsZero() {
		t.Fatalf("expected zero order book imbalance, got %s", ctx.OrderBookImbalance)
	}
	if !ctx.Mom5d.IsZero() {
		t.Fatalf("expected zero mom_5d with 3 candles, got %s", ctx.Mom5d)
	}
	if !ctx.Reversal1d.Equal(percentChange(decimal.NewFromFloat(105), decimal.NewFromFloat(110))) {
		t.Fatalf("unexpected reversal_1d %s", ctx.Reversal1d)
	}
}

func TestBuildMissingNews(t *testing.T) {
	builder := NewBuilder(nil)
	input := Input{
		Ticker: "OZON",
		Price: PriceSnapshot{
			LastPrice: decimal.NewFromFloat(210),
			PrevClose: decimal.NewFromFloat(200),
		},
		Candles: []moex.Candle{
			{Close: decimal.NewFromFloat(195)},
			{Close: decimal.NewFromFloat(200)},
			{Close: decimal.NewFromFloat(210)},
		},
	}

	ctx, err := builder.Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if ctx.NewsCount != 0 {
		t.Fatalf("unexpected news count %d", ctx.NewsCount)
	}
	if !ctx.NewsSentiment.IsZero() {
		t.Fatalf("unexpected news sentiment %s", ctx.NewsSentiment)
	}
	if !ctx.ReturnPct.Equal(decimal.NewFromFloat(5)) {
		t.Fatalf("unexpected return %s", ctx.ReturnPct)
	}
}

func TestBuildMissingPriceData(t *testing.T) {
	builder := NewBuilder(nil)
	tests := []struct {
		name  string
		price PriceSnapshot
		want  string
	}{
		{
			name:  "zero last price",
			price: PriceSnapshot{PrevClose: decimal.NewFromFloat(100)},
			want:  "last price must be positive",
		},
		{
			name:  "zero prev close",
			price: PriceSnapshot{LastPrice: decimal.NewFromFloat(110)},
			want:  "prev close must be positive",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := builder.Build(Input{Ticker: "SBER", Price: test.price})
			if err == nil {
				t.Fatal("expected error for missing price data")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected error containing %q, got %v", test.want, err)
			}
		})
	}
}

func TestBuildEmptyTicker(t *testing.T) {
	_, err := NewBuilder(nil).Build(Input{Price: PriceSnapshot{
		LastPrice: decimal.NewFromFloat(100),
		PrevClose: decimal.NewFromFloat(90),
	}})
	if err == nil {
		t.Fatal("expected error for empty ticker")
	}
	if !strings.Contains(err.Error(), "ticker must not be empty") {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestBuildWithOrderBookImbalance(t *testing.T) {
	builder := NewBuilder(nil)
	input := Input{
		Ticker: "SBER",
		Price: PriceSnapshot{
			LastPrice: decimal.NewFromFloat(270),
			PrevClose: decimal.NewFromFloat(260),
		},
		OrderBookImbalance: decimal.NewFromFloat(0.4),
	}

	ctx, err := builder.Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !ctx.OrderBookImbalance.Equal(decimal.NewFromFloat(0.4)) {
		t.Fatalf("unexpected order book imbalance %s", ctx.OrderBookImbalance)
	}
}

func TestBuildClampsOrderBookImbalance(t *testing.T) {
	base := Input{
		Ticker: "SBER",
		Price: PriceSnapshot{
			LastPrice: decimal.NewFromFloat(270),
			PrevClose: decimal.NewFromFloat(260),
		},
	}
	tests := []struct {
		name      string
		imbalance decimal.Decimal
		want      decimal.Decimal
	}{
		{"above one", decimal.NewFromFloat(1.4), decimal.NewFromInt(1)},
		{"below minus one", decimal.NewFromFloat(-1.2), decimal.NewFromInt(-1)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := base
			input.OrderBookImbalance = test.imbalance
			ctx, err := NewBuilder(nil).Build(input)
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			if !ctx.OrderBookImbalance.Equal(test.want) {
				t.Fatalf("unexpected order book imbalance %s, want %s", ctx.OrderBookImbalance, test.want)
			}
		})
	}
}

func TestEnrichWithAlgoPack(t *testing.T) {
	enriched := EnrichWithAlgoPack(domain.FeatureContext{Ticker: "SBER"}, decimal.NewFromFloat(1.7))
	if !enriched.OrderBookImbalance.Equal(decimal.NewFromInt(1)) {
		t.Fatalf("unexpected clamped imbalance %s", enriched.OrderBookImbalance)
	}
}

func TestRealizedVolatility(t *testing.T) {
	candles := []moex.Candle{
		{Close: decimal.NewFromFloat(100)},
		{Close: decimal.NewFromFloat(110)},
		{Close: decimal.NewFromFloat(105)},
		{Close: decimal.NewFromFloat(108)},
	}

	volatility := realizedVolatility(candles)
	want := decimal.NewFromFloat(7.273113780700483)
	if volatility.Sub(want).Abs().GreaterThan(decimal.NewFromFloat(0.000001)) {
		t.Fatalf("unexpected volatility %s, want %s", volatility, want)
	}
}

func TestRealizedVolatilityWithTooFewCandles(t *testing.T) {
	if !realizedVolatility(nil).IsZero() {
		t.Fatal("expected zero volatility for no candles")
	}
	if !realizedVolatility([]moex.Candle{{Close: decimal.NewFromFloat(100)}}).IsZero() {
		t.Fatal("expected zero volatility for a single candle")
	}
}

func TestPctChange(t *testing.T) {
	closes := []decimal.Decimal{
		decimal.NewFromFloat(100),
		decimal.NewFromFloat(105),
		decimal.NewFromFloat(110),
	}
	got := pctChange(closes, 1)
	want := decimal.NewFromFloat(4.76190476190476)
	if got.Sub(want).Abs().GreaterThan(decimal.NewFromFloat(0.0001)) {
		t.Fatalf("pctChange(1) = %s, want %s", got, want)
	}
	got5 := pctChange([]decimal.Decimal{
		decimal.NewFromFloat(100), decimal.NewFromFloat(101),
		decimal.NewFromFloat(102), decimal.NewFromFloat(103),
		decimal.NewFromFloat(104), decimal.NewFromFloat(110),
	}, 5)
	want5 := decimal.NewFromFloat(10)
	if got5.Sub(want5).Abs().GreaterThan(decimal.NewFromFloat(0.0001)) {
		t.Fatalf("pctChange(5) = %s, want %s", got5, want5)
	}
}

func TestRSI(t *testing.T) {
	closes := []decimal.Decimal{
		decimal.NewFromFloat(44), decimal.NewFromFloat(44.34), decimal.NewFromFloat(44.09),
		decimal.NewFromFloat(43.61), decimal.NewFromFloat(44.33), decimal.NewFromFloat(44.83),
		decimal.NewFromFloat(45.10), decimal.NewFromFloat(45.42), decimal.NewFromFloat(45.84),
		decimal.NewFromFloat(46.08), decimal.NewFromFloat(45.89), decimal.NewFromFloat(46.03),
		decimal.NewFromFloat(45.61), decimal.NewFromFloat(46.28), decimal.NewFromFloat(46.28),
		decimal.NewFromFloat(46.00),
	}
	got := rsi(closes, 14)
	if got.LessThan(decimal.NewFromFloat(50)) || got.GreaterThan(decimal.NewFromFloat(100)) {
		t.Fatalf("RSI out of range: %s", got)
	}
	if rsi(nil, 14).IsZero() != true {
		t.Fatal("RSI of nil should be zero")
	}
}

func TestSMA(t *testing.T) {
	closes := []decimal.Decimal{
		decimal.NewFromFloat(10), decimal.NewFromFloat(20),
		decimal.NewFromFloat(30), decimal.NewFromFloat(40),
	}
	got := sma(closes, 3)
	want := decimal.NewFromFloat(30)
	if got.Sub(want).Abs().GreaterThan(decimal.NewFromFloat(0.0001)) {
		t.Fatalf("sma(3) = %s, want %s", got, want)
	}
	if !sma(closes, 10).IsZero() {
		t.Fatal("sma with insufficient data should be zero")
	}
}

func TestDistFromMAPct(t *testing.T) {
	closes := []decimal.Decimal{
		decimal.NewFromFloat(100), decimal.NewFromFloat(100),
		decimal.NewFromFloat(100), decimal.NewFromFloat(100),
		decimal.NewFromFloat(100), decimal.NewFromFloat(110),
	}
	got := distFromMAPct(closes, 5)
	// sma = (100+100+100+100+110)/5 = 102, dist = (110-102)/102 = 7.843137%
	if got.LessThan(decimal.NewFromFloat(7.8)) || got.GreaterThan(decimal.NewFromFloat(7.9)) {
		t.Fatalf("distFromMAPct = %s, expected ~7.84", got)
	}
}

func TestVolumeZScore(t *testing.T) {
	var volumes []decimal.Decimal
	for i := 0; i < 25; i++ {
		volumes = append(volumes, decimal.NewFromFloat(100))
	}
	volumes = append(volumes, decimal.NewFromFloat(100))
	if !volumeZScore(volumes, 20).IsZero() {
		t.Fatal("z-score of constant volume should be zero")
	}
	if !volumeZScore(nil, 20).IsZero() {
		t.Fatal("z-score of nil should be zero")
	}
}

func TestComputePriceFeatures(t *testing.T) {
	candles := make([]moex.Candle, 70)
	for i := range candles {
		candles[i] = moex.Candle{
			Close:  decimal.NewFromFloat(float64(100 + i)),
			Volume: decimal.NewFromFloat(1000),
		}
	}
	pf := ComputePriceFeatures(candles)
	if pf.Mom5d.Sign() <= 0 {
		t.Fatal("expected positive mom_5d")
	}
	if pf.Mom21d.Sign() <= 0 {
		t.Fatal("expected positive mom_21d")
	}
	if pf.Mom63d.Sign() <= 0 {
		t.Fatal("expected positive mom_63d")
	}
	if !pf.Reversal1d.IsPositive() {
		t.Fatal("expected positive reversal_1d")
	}
	if pf.RSI14.Sign() <= 0 {
		t.Fatal("expected positive RSI")
	}
	if pf.DistMA20Pct.Sign() <= 0 {
		t.Fatal("expected positive dist_ma20_pct")
	}
	if pf.DistMA50Pct.Sign() <= 0 {
		t.Fatal("expected positive dist_ma50_pct")
	}
	if pf.RealizedVol21dPct.Sign() <= 0 {
		t.Fatal("expected positive realized vol")
	}
}
