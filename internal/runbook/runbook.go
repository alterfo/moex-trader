package runbook

import "github.com/shopspring/decimal"

// KillSwitchResetEquityMultiplier is the pre-registered runbook rule: a manual
// kill-switch reset is only allowed when account equity is at least this
// multiple of the largest open-position notional.
var KillSwitchResetEquityMultiplier = decimal.RequireFromString("1.5")

// Decision is the result of the manual kill-switch reset runbook check.
type Decision struct {
	Allowed         bool
	Equity          decimal.Decimal
	MaxOpenNotional decimal.Decimal
	RequiredEquity  decimal.Decimal
}

// Decide evaluates the runbook rule "equity >= 1.5x max_notional across all
// open positions". An account with no open positions (max notional <= 0)
// always passes.
func Decide(equity, maxOpenNotional decimal.Decimal) Decision {
	if maxOpenNotional.Sign() <= 0 {
		return Decision{
			Allowed:         true,
			Equity:          equity,
			MaxOpenNotional: decimal.Zero,
			RequiredEquity:  decimal.Zero,
		}
	}
	required := maxOpenNotional.Mul(KillSwitchResetEquityMultiplier)
	return Decision{
		Allowed:         equity.GreaterThanOrEqual(required),
		Equity:          equity,
		MaxOpenNotional: maxOpenNotional,
		RequiredEquity:  required,
	}
}
