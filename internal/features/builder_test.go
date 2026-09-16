package features

import (
	"math"
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

func TestComputePriceFeaturesLookbackIsBoundedAndConverged(t *testing.T) {
	// A long, realistic-looking price series (mild upward drift with noise)
	// so MACD/Alligator have something non-trivial to converge on.
	const total = 5000
	candles := make([]moex.Candle, total)
	price := 100.0
	for i := 0; i < total; i++ {
		price += 0.03 + 0.5*math.Sin(float64(i)/17.0)
		candles[i] = moex.Candle{
			Close: decimal.NewFromFloat(price),
			High:  decimal.NewFromFloat(price + 1),
			Low:   decimal.NewFromFloat(price - 1),
		}
	}

	start := time.Now()
	full := ComputePriceFeatures(candles)
	elapsed := time.Since(start)
	if elapsed > 200*time.Millisecond {
		t.Fatalf("ComputePriceFeatures on %d candles took %s, want it bounded by maxIndicatorLookbackCandles regardless of history length", total, elapsed)
	}

	// Computing on exactly the bounded tail must give the identical result:
	// this is what makes truncating inside ComputePriceFeatures safe.
	truncated := ComputePriceFeatures(candles[total-maxIndicatorLookbackCandles:])
	if !full.MACDHistPct.Equal(truncated.MACDHistPct) {
		t.Errorf("MACDHistPct = %s with full history, %s with the bounded tail alone, want equal", full.MACDHistPct, truncated.MACDHistPct)
	}
	if !full.AlligatorSpreadPct.Equal(truncated.AlligatorSpreadPct) {
		t.Errorf("AlligatorSpreadPct = %s with full history, %s with the bounded tail alone, want equal", full.AlligatorSpreadPct, truncated.AlligatorSpreadPct)
	}

	// And the truncation must not have drifted MACD from what a from-scratch
	// EMA over a much longer, but still bounded, warmup would produce -
	// guards against maxIndicatorLookbackCandles being too small to let the
	// EMA/SMMA seed value decay to numerical irrelevance.
	longerWarmup := ComputePriceFeatures(candles[total-maxIndicatorLookbackCandles-500:])
	diff := full.MACDHistPct.Sub(longerWarmup.MACDHistPct).Abs()
	if diff.GreaterThan(decimal.NewFromFloat(0.01)) {
		t.Errorf("MACDHistPct = %s at %d-bar lookback vs %s with 500 extra warmup bars, diff %s exceeds tolerance - lookback may be too short to converge",
			full.MACDHistPct, maxIndicatorLookbackCandles, longerWarmup.MACDHistPct, diff)
	}
}

func TestComputePriceFeaturesScalesWindowsByBarsPerDay(t *testing.T) {
	// With BarsPerDay=105 (roughly a MOEX 5-min session), Mom5d must span
	// 5*105 bars of history, not 5 raw bars. Verify with a plateau that only
	// makes sense at the scaled window.
	const bpd = 105
	candles := make([]moex.Candle, 8*bpd+50)
	for i := range candles {
		candles[i] = moex.Candle{Close: decimal.NewFromFloat(100)}
	}
	for i := 8*bpd + 1; i < len(candles); i++ {
		candles[i] = moex.Candle{Close: decimal.NewFromFloat(200)}
	}

	daily := ComputePriceFeaturesWithConfig(candles, PriceFeatureConfig{})
	scaled := ComputePriceFeaturesWithConfig(candles, PriceFeatureConfig{BarsPerDay: bpd})

	// Daily window (5 bars) still sits on the recent plateau -> ~0 momentum.
	if daily.Mom5d.Abs().GreaterThan(decimal.NewFromFloat(1)) {
		t.Fatalf("daily Mom5d expected ~0 on a recent plateau, got %s", daily.Mom5d)
	}
	// Scaled window (5*105 bars) reaches back to the old price level -> big momentum.
	if scaled.Mom5d.LessThanOrEqual(decimal.NewFromFloat(20)) {
		t.Fatalf("scaled Mom5d expected >20%% (5-day momentum over a 100->200 move), got %s", scaled.Mom5d)
	}
}

func TestConfigForIntervalMapsToBarsPerDay(t *testing.T) {
	if got := ConfigForInterval(24); got.BarsPerDay != 0 {
		t.Fatalf("daily interval should map to zero config, got %+v", got)
	}
	if got := ConfigForInterval(0); got.BarsPerDay != 0 {
		t.Fatalf("zero interval should map to zero config, got %+v", got)
	}
	if got := BarsPerDayForInterval(5); got < 80 || got > 150 {
		t.Fatalf("5-min interval expected ~100 bars/session, got %d", got)
	}
	if got := BarsPerDayForInterval(1440); got != 1 {
		t.Fatalf(">=1440-min interval expected 1 bar/session, got %d", got)
	}
}

func TestWarmupCandlesMatchesLookback(t *testing.T) {
	if got := (PriceFeatureConfig{}).WarmupCandles(); got != 64 {
		t.Fatalf("daily warmup should stay 64 to preserve the existing chain, got %d", got)
	}
	const bpd = 105
	cfg := PriceFeatureConfig{BarsPerDay: bpd}
	want := 63*bpd + maxIndicatorLookbackCandles
	if got := cfg.WarmupCandles(); got != want {
		t.Fatalf("intraday warmup = %d, want %d (63-day window + EMA margin)", got, want)
	}
	if cfg.WarmupCandles() <= 64 {
		t.Fatal("intraday warmup must exceed the daily 64 or long windows would be zeroed")
	}
	if got := (PriceFeatureConfig{}).WarmupCalendarDays(); got != 0 {
		t.Fatalf("daily warmup calendar days should be 0 (callers keep their own), got %d", got)
	}
	cal := cfg.WarmupCalendarDays()
	trading := (want + bpd - 1) / bpd
	if cal < trading*7/5 {
		t.Fatalf("warmup calendar days %d too short to fit %d trading bars of warmup at %d bars/day", cal, trading, bpd)
	}
}

func TestStochasticK(t *testing.T) {
	candles := make([]moex.Candle, 14)
	candles[0] = moex.Candle{High: decimal.NewFromFloat(110), Low: decimal.NewFromFloat(100), Close: decimal.NewFromFloat(105)}
	for i := 1; i < 13; i++ {
		candles[i] = moex.Candle{High: decimal.NewFromFloat(100), Low: decimal.NewFromFloat(100), Close: decimal.NewFromFloat(100)}
	}
	candles[13] = moex.Candle{High: decimal.NewFromFloat(100), Low: decimal.NewFromFloat(90), Close: decimal.NewFromFloat(105)}

	got := stochasticK(candles, 14)
	want := decimal.NewFromFloat(75)
	if got.Sub(want).Abs().GreaterThan(decimal.NewFromFloat(0.0001)) {
		t.Fatalf("stochasticK = %s, want %s", got, want)
	}
	if !stochasticK(candles[:5], 14).IsZero() {
		t.Fatal("stochasticK with insufficient data should be zero")
	}
}

func TestWilliamsR(t *testing.T) {
	candles := make([]moex.Candle, 14)
	candles[0] = moex.Candle{High: decimal.NewFromFloat(110), Low: decimal.NewFromFloat(100), Close: decimal.NewFromFloat(105)}
	for i := 1; i < 13; i++ {
		candles[i] = moex.Candle{High: decimal.NewFromFloat(100), Low: decimal.NewFromFloat(100), Close: decimal.NewFromFloat(100)}
	}
	candles[13] = moex.Candle{High: decimal.NewFromFloat(100), Low: decimal.NewFromFloat(90), Close: decimal.NewFromFloat(105)}

	got := williamsR(candles, 14)
	want := decimal.NewFromFloat(-25)
	if got.Sub(want).Abs().GreaterThan(decimal.NewFromFloat(0.0001)) {
		t.Fatalf("williamsR = %s, want %s", got, want)
	}
	if !williamsR(candles[:5], 14).IsZero() {
		t.Fatal("williamsR with insufficient data should be zero")
	}
}

func TestMACDHistPct(t *testing.T) {
	if !macdHistPct(nil, 12, 26, 9).IsZero() {
		t.Fatal("macdHistPct with no data should be zero")
	}
	// Flat prices followed by a sustained rally: the MACD line rises faster
	// than its (lagging) signal line during the acceleration, so the
	// histogram should be genuinely positive rather than converged to ~0
	// (a pure constant-slope ramp converges fast/slow EMAs to a fixed gap,
	// making the histogram numerically indistinguishable from zero).
	var closes []decimal.Decimal
	for i := 0; i < 30; i++ {
		closes = append(closes, decimal.NewFromFloat(100))
	}
	for i := 0; i < 40; i++ {
		closes = append(closes, decimal.NewFromFloat(float64(100+i)))
	}
	got := macdHistPct(closes, 12, 26, 9)
	if got.Sign() <= 0 {
		t.Fatalf("expected positive macd histogram during a rally, got %s", got)
	}
}

func TestAlligatorSpreadPct(t *testing.T) {
	if !alligatorSpreadPct(nil, PriceFeatureConfig{}).IsZero() {
		t.Fatal("alligatorSpreadPct with no data should be zero")
	}
	candles := make([]moex.Candle, 60)
	for i := range candles {
		candles[i] = moex.Candle{
			High: decimal.NewFromFloat(float64(101 + i)),
			Low:  decimal.NewFromFloat(float64(99 + i)),
		}
	}
	got := alligatorSpreadPct(candles, PriceFeatureConfig{})
	if got.Sign() <= 0 {
		t.Fatalf("expected positive alligator spread (lips above jaw) in an uptrend, got %s", got)
	}
}
