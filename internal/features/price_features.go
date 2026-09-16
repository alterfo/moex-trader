package features

import (
	"math"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

func sma(closes []decimal.Decimal, window int) decimal.Decimal {
	if len(closes) < window {
		return decimal.Zero
	}
	var sum decimal.Decimal
	for _, v := range closes[len(closes)-window:] {
		sum = sum.Add(v)
	}
	return sum.Div(decimal.NewFromInt(int64(window)))
}

func pctChange(closes []decimal.Decimal, n int) decimal.Decimal {
	if len(closes) < n+1 || closes[len(closes)-n-1].Sign() <= 0 {
		return decimal.Zero
	}
	return closes[len(closes)-1].Sub(closes[len(closes)-n-1]).
		Div(closes[len(closes)-n-1]).Mul(decimal.NewFromInt(100))
}

func rsi(closes []decimal.Decimal, period int) decimal.Decimal {
	if len(closes) < period+1 {
		return decimal.Zero
	}
	var avgGain, avgLoss decimal.Decimal
	for i := len(closes) - period; i < len(closes); i++ {
		change := closes[i].Sub(closes[i-1])
		if change.IsPositive() {
			avgGain = avgGain.Add(change)
		} else {
			avgLoss = avgLoss.Add(change.Abs())
		}
	}
	avgGain = avgGain.Div(decimal.NewFromInt(int64(period)))
	avgLoss = avgLoss.Div(decimal.NewFromInt(int64(period)))
	if avgLoss.IsZero() {
		return decimal.NewFromInt(100)
	}
	rs := avgGain.Div(avgLoss)
	return decimal.NewFromInt(100).Sub(decimal.NewFromInt(100).Div(decimal.NewFromInt(1).Add(rs)))
}

func realizedVolPct(closes []decimal.Decimal, window, barsPerYear int) decimal.Decimal {
	if barsPerYear <= 0 {
		barsPerYear = 252
	}
	if len(closes) < window+1 {
		return decimal.Zero
	}
	var rets []decimal.Decimal
	for i := len(closes) - window; i < len(closes); i++ {
		if closes[i-1].Sign() <= 0 || closes[i].Sign() <= 0 {
			continue
		}
		rets = append(rets, closes[i].Sub(closes[i-1]).Div(closes[i-1]))
	}
	if len(rets) < 2 {
		return decimal.Zero
	}
	var sum decimal.Decimal
	for _, r := range rets {
		sum = sum.Add(r)
	}
	mean := sum.Div(decimal.NewFromInt(int64(len(rets))))
	var squaredDeviation decimal.Decimal
	for _, r := range rets {
		deviation := r.Sub(mean)
		squaredDeviation = squaredDeviation.Add(deviation.Mul(deviation))
	}
	variance := squaredDeviation.Div(decimal.NewFromInt(int64(len(rets))))
	v, _ := variance.Float64()
	if v <= 0 {
		return decimal.Zero
	}
	return decimal.NewFromFloat(math.Sqrt(v) * math.Sqrt(float64(barsPerYear)) * 100)
}

func distFromMAPct(closes []decimal.Decimal, window int) decimal.Decimal {
	ma := sma(closes, window)
	if ma.Sign() <= 0 {
		return decimal.Zero
	}
	return closes[len(closes)-1].Sub(ma).Div(ma).Mul(decimal.NewFromInt(100))
}

func volumeZScore(volumes []decimal.Decimal, window int) decimal.Decimal {
	if len(volumes) < window+1 {
		return decimal.Zero
	}
	hist := volumes[len(volumes)-window-1 : len(volumes)-1]
	if len(hist) < 2 {
		return decimal.Zero
	}
	var sum decimal.Decimal
	for _, v := range hist {
		sum = sum.Add(v)
	}
	mean := sum.Div(decimal.NewFromInt(int64(len(hist))))
	var sqDev decimal.Decimal
	for _, v := range hist {
		d := v.Sub(mean)
		sqDev = sqDev.Add(d.Mul(d))
	}
	sd := decimal.NewFromFloat(math.Sqrt(sqDev.Div(decimal.NewFromInt(int64(len(hist)))).InexactFloat64()))
	if sd.Sign() <= 0 {
		return decimal.Zero
	}
	return volumes[len(volumes)-1].Sub(mean).Div(sd)
}

type PriceFeatures struct {
	Mom5d              decimal.Decimal
	Mom21d             decimal.Decimal
	Mom63d             decimal.Decimal
	Reversal1d         decimal.Decimal
	RSI14              decimal.Decimal
	DistMA20Pct        decimal.Decimal
	DistMA50Pct        decimal.Decimal
	RealizedVol21dPct  decimal.Decimal
	VolumeZScore20d    decimal.Decimal
	MACDHistPct        decimal.Decimal
	StochK14           decimal.Decimal
	WilliamsR14        decimal.Decimal
	AlligatorSpreadPct decimal.Decimal
}

// maxIndicatorLookbackCandles bounds how much history ComputePriceFeatures
// ever looks at. Every indicator here needs at most 63 trailing candles
// (Mom63d) except the EMA/SMMA-based ones (MACD, Alligator), which are
// recursive and technically depend on the entire series — but their seed
// value's influence decays geometrically (EMA-26/SMMA-13 have a per-step
// decay around 0.92-0.93), so after a few hundred bars the exact seed is
// numerically irrelevant. 300 bars leaves a wide convergence margin while
// keeping this function's cost bounded instead of growing with the ticker's
// full history — callers that scan history day-by-day (backtests, training)
// would otherwise pay O(history_length) per day, i.e. O(n^2) overall.
const maxIndicatorLookbackCandles = 300

// PriceFeatureConfig scales day-denominated indicator windows to the candle
// interval actually in use. A zero value behaves exactly like the original
// daily series: every window (Mom5d = 5 bars, RSI14 = 14 bars, ...) is the
// day count itself. For intraday candles, set BarsPerDay to the number of
// bars in one trading session so e.g. Mom5d means 5 * BarsPerDay bars - the
// same calendar window expressed in the current granularity. This is
// deliberate: shorting the lookback windows on 1/5-min bars would collapse
// the momentum/reversal features to minutes of market time and quietly
// change what the model is learning.
type PriceFeatureConfig struct {
	BarsPerDay int
}

// barsPerDay returns at least 1 so a zero config stays the daily default.
func (c PriceFeatureConfig) barsPerDay() int {
	if c.BarsPerDay <= 0 {
		return 1
	}
	return c.BarsPerDay
}

// BarsPerDayForInterval converts an ISS candle interval (minutes; 24 = daily)
// into the number of bars per trading session, so Callers can build a
// PriceFeatureConfig for an intraday source. MOEX's main session is roughly
// 500 minutes (10:00-18:25), so a 5-min interval yields ~100 bars/day; large
// intervals like 60 or 24 collapse to a handful or exactly 1.
func BarsPerDayForInterval(intervalMin int) int {
	if intervalMin <= 0 || intervalMin >= 1440 {
		return 1
	}
	const sessionMinutes = 500
	bpd := sessionMinutes / intervalMin
	if bpd < 1 {
		bpd = 1
	}
	return bpd
}

// ConfigForInterval returns a PriceFeatureConfig matching an ISS candle
// interval in minutes (24 = daily, per MOEX ISS semantics) - the zero config
// for daily, or a BarsPerDay-scaled one for intraday.
func ConfigForInterval(intervalMin int) PriceFeatureConfig {
	if intervalMin <= 0 || intervalMin == 24 {
		return PriceFeatureConfig{}
	}
	return PriceFeatureConfig{BarsPerDay: BarsPerDayForInterval(intervalMin)}
}

// lookbackBounds returns the number of trailing candles needed so that every
// window - including the largest (Mom63d) plus the EMA/SMMA convergence
// margin - fits, while keeping the daily (BarsPerDay=1) cost bound unchanged.
func (c PriceFeatureConfig) lookbackBounds() int {
	if c.barsPerDay() == 1 {
		return maxIndicatorLookbackCandles
	}
	return 63*c.barsPerDay() + maxIndicatorLookbackCandles
}

// WarmupCandles is the minimum number of candles a caller must feed before a
// sample's feature vector is fully populated: on daily bars that is the
// historical 64 (large enough for the 63-bar Mom63d window). On intraday bars
// the day-denominated windows scale up, so the warmup grows with them -
// otherwise the first ~63*barsPerDay bars of a scan would emit zeroed
// momentum/MA/vol features and quietly poison the training set.
func (c PriceFeatureConfig) WarmupCandles() int {
	if c.barsPerDay() == 1 {
		return 64
	}
	return c.lookbackBounds()
}

// WarmupCalendarDays is the calendar-day fetch lead an intraday source needs
// so WarmupCandles intraday bars actually exist (trading days are ~5/7 of
// calendar days; a small buffer covers holidays and shortened sessions). It
// returns 0 for daily bars, where callers already have their own warmup and
// the intraday scaling must not change the existing chain.
func (c PriceFeatureConfig) WarmupCalendarDays() int {
	if c.barsPerDay() == 1 {
		return 0
	}
	bpd := c.barsPerDay()
	tradingDays := (c.WarmupCandles() + bpd - 1) / bpd
	return tradingDays*7/5 + 10
}

func ComputePriceFeatures(candles []moex.Candle) PriceFeatures {
	return ComputePriceFeaturesWithConfig(candles, PriceFeatureConfig{})
}

func ComputePriceFeaturesWithConfig(candles []moex.Candle, cfg PriceFeatureConfig) PriceFeatures {
	lookback := cfg.lookbackBounds()
	if len(candles) > lookback {
		candles = candles[len(candles)-lookback:]
	}
	bpd := cfg.barsPerDay()
	closes := make([]decimal.Decimal, 0, len(candles))
	volumes := make([]decimal.Decimal, 0, len(candles))
	for _, c := range candles {
		closes = append(closes, c.Close)
		volumes = append(volumes, c.Volume)
	}
	return PriceFeatures{
		Mom5d:              pctChange(closes, 5*bpd),
		Mom21d:             pctChange(closes, 21*bpd),
		Mom63d:             pctChange(closes, 63*bpd),
		Reversal1d:         pctChange(closes, 1*bpd),
		RSI14:              rsi(closes, 14*bpd),
		DistMA20Pct:        distFromMAPct(closes, 20*bpd),
		DistMA50Pct:        distFromMAPct(closes, 50*bpd),
		RealizedVol21dPct:  realizedVolPct(closes, 21*bpd, 252*bpd),
		VolumeZScore20d:    volumeZScore(volumes, 20*bpd),
		MACDHistPct:        macdHistPct(closes, 12*bpd, 26*bpd, 9*bpd),
		StochK14:           stochasticK(candles, 14*bpd),
		WilliamsR14:        williamsR(candles, 14*bpd),
		AlligatorSpreadPct: alligatorSpreadPct(candles, cfg),
	}
}

// ema returns the exponential moving average series for values, seeded with
// a simple average of the first `period` values. Indices before period-1 are
// zero (undefined) and must not be read by callers.
func ema(values []decimal.Decimal, period int) []decimal.Decimal {
	if len(values) < period {
		return nil
	}
	out := make([]decimal.Decimal, len(values))
	out[period-1] = sma(values[:period], period)
	k := decimal.NewFromFloat(2.0 / float64(period+1))
	for i := period; i < len(values); i++ {
		out[i] = values[i].Sub(out[i-1]).Mul(k).Add(out[i-1])
	}
	return out
}

// smma returns Wilder's smoothed moving average series, same indexing rules as ema.
func smma(values []decimal.Decimal, period int) []decimal.Decimal {
	if len(values) < period {
		return nil
	}
	out := make([]decimal.Decimal, len(values))
	out[period-1] = sma(values[:period], period)
	periodDec := decimal.NewFromInt(int64(period))
	for i := period; i < len(values); i++ {
		out[i] = out[i-1].Mul(periodDec.Sub(decimal.NewFromInt(1))).Add(values[i]).Div(periodDec)
	}
	return out
}

// macdHistPct returns the MACD histogram (MACD line minus its signal line),
// normalized by the latest close so it is comparable across tickers.
func macdHistPct(closes []decimal.Decimal, fast, slow, signalPeriod int) decimal.Decimal {
	if len(closes) < slow+signalPeriod {
		return decimal.Zero
	}
	emaFast := ema(closes, fast)
	emaSlow := ema(closes, slow)
	start := slow - 1
	macdLine := make([]decimal.Decimal, 0, len(closes)-start)
	for i := start; i < len(closes); i++ {
		macdLine = append(macdLine, emaFast[i].Sub(emaSlow[i]))
	}
	if len(macdLine) < signalPeriod {
		return decimal.Zero
	}
	signalLine := ema(macdLine, signalPeriod)
	hist := macdLine[len(macdLine)-1].Sub(signalLine[len(signalLine)-1])
	price := closes[len(closes)-1]
	if price.Sign() <= 0 {
		return decimal.Zero
	}
	return hist.Div(price).Mul(decimal.NewFromInt(100))
}

func highLow(candles []moex.Candle) (decimal.Decimal, decimal.Decimal) {
	highest, lowest := candles[0].High, candles[0].Low
	for _, c := range candles[1:] {
		if c.High.GreaterThan(highest) {
			highest = c.High
		}
		if c.Low.LessThan(lowest) {
			lowest = c.Low
		}
	}
	return highest, lowest
}

// stochasticK returns the %K stochastic oscillator (0-100).
func stochasticK(candles []moex.Candle, period int) decimal.Decimal {
	if len(candles) < period {
		return decimal.Zero
	}
	window := candles[len(candles)-period:]
	highest, lowest := highLow(window)
	rangeVal := highest.Sub(lowest)
	if rangeVal.Sign() <= 0 {
		return decimal.NewFromInt(50)
	}
	close := candles[len(candles)-1].Close
	return close.Sub(lowest).Div(rangeVal).Mul(decimal.NewFromInt(100))
}

// williamsR returns Williams %R (-100 to 0).
func williamsR(candles []moex.Candle, period int) decimal.Decimal {
	if len(candles) < period {
		return decimal.Zero
	}
	window := candles[len(candles)-period:]
	highest, lowest := highLow(window)
	rangeVal := highest.Sub(lowest)
	if rangeVal.Sign() <= 0 {
		return decimal.NewFromInt(-50)
	}
	close := candles[len(candles)-1].Close
	return highest.Sub(close).Div(rangeVal).Mul(decimal.NewFromInt(-100))
}

// alligatorSpreadPct implements Bill Williams' Alligator on median price
// ((H+L)/2): Jaw = SMMA(13) shifted 8 bars forward, Teeth = SMMA(8) shifted 5,
// Lips = SMMA(5) shifted 3. The shift means the line's "current" plotted value
// was computed that many bars ago, so we index each SMMA series accordingly.
// The feature is the Lips-vs-Jaw spread (%): positive and widening signals an
// open, upward mouth (uptrend); negative signals a downtrend. Window/shift
// counts are day-denominated and scaled by cfg like everything else.
func alligatorSpreadPct(candles []moex.Candle, cfg PriceFeatureConfig) decimal.Decimal {
	const jawBars, teethBars, lipsBars = 13, 8, 5
	const jawShift, teethShift, lipsShift = 8, 5, 3
	bpd := cfg.barsPerDay()
	jaw, teeth, lips := jawBars*bpd, teethBars*bpd, lipsBars*bpd
	shiftJ, shiftL := jawShift*bpd, lipsShift*bpd
	minCandles := jaw + shiftJ
	if len(candles) < minCandles {
		return decimal.Zero
	}
	median := make([]decimal.Decimal, len(candles))
	for i, c := range candles {
		median[i] = c.High.Add(c.Low).Div(decimal.NewFromInt(2))
	}
	jawSeries := smma(median, jaw)
	lipsSeries := smma(median, lips)
	jawIdx := len(median) - 1 - shiftJ
	lipsIdx := len(median) - 1 - shiftL
	if jawIdx < jaw-1 || lipsIdx < lips-1 {
		return decimal.Zero
	}
	_ = teeth // teeth line is unused in the chosen spread feature (Lips vs Jaw)
	return lipsSubJaw(jawSeries, lipsSeries, jawIdx, lipsIdx)
}

func lipsSubJaw(jawSeries, lipsSeries []decimal.Decimal, jawIdx, lipsIdx int) decimal.Decimal {
	jaw := jawSeries[jawIdx]
	lips := lipsSeries[lipsIdx]
	if jaw.Sign() <= 0 {
		return decimal.Zero
	}
	return lips.Sub(jaw).Div(jaw).Mul(decimal.NewFromInt(100))
}
