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

func realizedVolPct(closes []decimal.Decimal, window int) decimal.Decimal {
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
	return decimal.NewFromFloat(math.Sqrt(v) * math.Sqrt(252) * 100)
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
	Mom5d             decimal.Decimal
	Mom21d            decimal.Decimal
	Mom63d            decimal.Decimal
	Reversal1d        decimal.Decimal
	RSI14             decimal.Decimal
	DistMA20Pct       decimal.Decimal
	DistMA50Pct       decimal.Decimal
	RealizedVol21dPct decimal.Decimal
	VolumeZScore20d   decimal.Decimal
}

func ComputePriceFeatures(candles []moex.Candle) PriceFeatures {
	closes := make([]decimal.Decimal, 0, len(candles))
	volumes := make([]decimal.Decimal, 0, len(candles))
	for _, c := range candles {
		closes = append(closes, c.Close)
		volumes = append(volumes, c.Volume)
	}
	return PriceFeatures{
		Mom5d:             pctChange(closes, 5),
		Mom21d:            pctChange(closes, 21),
		Mom63d:            pctChange(closes, 63),
		Reversal1d:        pctChange(closes, 1),
		RSI14:             rsi(closes, 14),
		DistMA20Pct:       distFromMAPct(closes, 20),
		DistMA50Pct:       distFromMAPct(closes, 50),
		RealizedVol21dPct: realizedVolPct(closes, 21),
		VolumeZScore20d:   volumeZScore(volumes, 20),
	}
}
