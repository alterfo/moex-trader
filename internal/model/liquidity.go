package model

import (
	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

const TurnoverWindow = 20

func medianTrailingTurnover(candles []moex.Candle, idx, window int) decimal.Decimal {
	if idx < 0 || idx >= len(candles) || window <= 0 {
		return decimal.Zero
	}
	start := idx - window + 1
	if start < 0 {
		return decimal.Zero
	}
	values := make([]decimal.Decimal, 0, window)
	for i := start; i <= idx; i++ {
		values = append(values, candles[i].Value)
	}
	return domain.MedianDecimal(values)
}

func LiquidOn(candles []moex.Candle, idx int, window int, threshold decimal.Decimal) bool {
	if idx-window+1 < 0 {
		return false
	}
	return medianTrailingTurnover(candles, idx, window).GreaterThanOrEqual(threshold)
}
