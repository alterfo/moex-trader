package model

import (
	"math"

	"github.com/shopspring/decimal"
)

type VolScale struct {
	Enabled bool
	MinMult decimal.Decimal
	MaxMult decimal.Decimal
}

func (v VolScale) Multiplier(sigma, median decimal.Decimal) decimal.Decimal {
	if !v.Enabled {
		return decimal.NewFromInt(1)
	}
	return VolScaleMultiplier(sigma, median, v.MinMult, v.MaxMult)
}

func VolScaleMultiplier(sigma, median, minMult, maxMult decimal.Decimal) decimal.Decimal {
	if sigma.Sign() <= 0 || median.Sign() <= 0 {
		return decimal.NewFromInt(1)
	}
	ratio := median.Div(sigma)
	if ratio.LessThan(minMult) {
		return minMult
	}
	if ratio.GreaterThan(maxMult) {
		return maxMult
	}
	return ratio
}

func targetLotsForNotional(notional, lastPrice, lotSize decimal.Decimal, maxLots int, volScale VolScale, sigma, median decimal.Decimal) int {
	if !notional.IsPositive() {
		return maxLots
	}
	if !lastPrice.IsPositive() {
		return maxLots
	}
	scaled := notional
	if volScale.Enabled {
		scaled = notional.Mul(volScale.Multiplier(sigma, median))
	}
	perUnit := lastPrice
	if lotSize.IsPositive() {
		perUnit = lastPrice.Mul(lotSize)
	}
	lots := scaled.Div(perUnit).Round(0).IntPart()
	if lots < 1 {
		return 1
	}
	if lots > math.MaxInt32 {
		return math.MaxInt32
	}
	return int(lots)
}
