package backtest

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

type position struct {
	action   domain.Action
	lots     int
	avg      decimal.Decimal
	comm     decimal.Decimal
	openedAt time.Time
}

func commissionAmount(price decimal.Decimal, lots int, rate decimal.Decimal) decimal.Decimal {
	if lots <= 0 || rate.IsNegative() || price.Sign() <= 0 {
		return decimal.Zero
	}
	return price.Mul(decimal.NewFromInt(int64(lots))).Mul(rate)
}
