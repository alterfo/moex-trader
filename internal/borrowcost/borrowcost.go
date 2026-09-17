package borrowcost

import (
	"fmt"

	"github.com/shopspring/decimal"
	pb "github.com/tinkoff/invest-api-go-sdk/proto"
)

// StressPctPerDay is the pre-registered fallback short-borrow cost, expressed
// as a percentage of short-leg notional per calendar day, carried over from
// the source debate (Task 11 / 3.1). It is applied only when the real borrow
// rate cannot be measured from the broker.
const StressPctPerDay = "0.005"

var (
	stressPct  = decimal.RequireFromString(StressPctPerDay)
	oneHundred = decimal.NewFromInt(100)
)

// StressRatePerDay returns the stress cost as a fraction of notional per day
// (0.005% = 0.00005).
func StressRatePerDay() decimal.Decimal {
	return stressPct.Div(oneHundred)
}

// DailyStressCost returns the daily borrow cost for a short position of the
// given ruble notional under the stress rate.
func DailyStressCost(notional decimal.Decimal) decimal.Decimal {
	if notional.Sign() <= 0 {
		return decimal.Zero
	}
	return notional.Mul(StressRatePerDay())
}

// StressCostForDays returns the stress borrow cost for holding a short
// position of the given notional for the given number of days.
func StressCostForDays(notional decimal.Decimal, days int) decimal.Decimal {
	if notional.Sign() <= 0 || days <= 0 {
		return decimal.Zero
	}
	return DailyStressCost(notional).Mul(decimal.NewFromInt(int64(days)))
}

// Resolution is the go/no-go decision on which borrow rate to use.
type Resolution struct {
	MeasuredRatePerDay decimal.Decimal
	UsingStress        bool
	Measurable         bool
}

// ResolveDailyRate decides whether a measured daily borrow rate is usable.
// marginFees is the total observed borrow fee (for example the sum of
// OPERATION_TYPE_MARGIN_FEE payments) and shortNotionalDays is the sum over
// holding days of short-leg notional in ruble-days. A measured rate is used
// only when both are positive; otherwise the pre-registered stress rate is
// applied. The go/no-go decision must always use the stress rate unless a
// real measured rate exists.
func ResolveDailyRate(marginFees, shortNotionalDays decimal.Decimal) Resolution {
	if marginFees.IsPositive() && shortNotionalDays.IsPositive() {
		return Resolution{
			MeasuredRatePerDay: marginFees.Div(shortNotionalDays),
			UsingStress:        false,
			Measurable:         true,
		}
	}
	return Resolution{
		MeasuredRatePerDay: StressRatePerDay(),
		UsingStress:        true,
		Measurable:         false,
	}
}

// ShareTerm is the per-ticker short-selling information exposed by the broker
// for one instrument. The borrow fee rate is not part of it because Tinkoff's
// API reports only short availability and margin risk rates.
type ShareTerm struct {
	Ticker       string
	ShortEnabled bool
	Kshort       decimal.Decimal
	Dshort       decimal.Decimal
	DshortMin    decimal.Decimal
}

// SumMarginFees totals the observed short-borrow charges from a broker
// operations history. Tinkoff reports borrow charges only as
// OPERATION_TYPE_MARGIN_FEE payments, so this is the direct real-world
// counterpart to the Task 11 stress rate.
func SumMarginFees(operations []*pb.Operation) (fees decimal.Decimal, count int) {
	for _, op := range operations {
		if op == nil || op.GetOperationType() != pb.OperationType_OPERATION_TYPE_MARGIN_FEE {
			continue
		}
		payment, err := tinkoffMoneyValueToDecimal(op.GetPayment())
		if err != nil {
			continue
		}
		fees = fees.Add(payment.Abs())
		count++
	}
	return fees, count
}

func tinkoffMoneyValueToDecimal(value *pb.MoneyValue) (decimal.Decimal, error) {
	if value == nil {
		return decimal.Zero, fmt.Errorf("borrowcost: nil money value")
	}
	whole := decimal.NewFromInt(value.GetUnits())
	fraction := decimal.NewFromInt(int64(value.GetNano())).Div(decimal.NewFromInt(1_000_000_000))
	return whole.Add(fraction), nil
}
