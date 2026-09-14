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
