package backtest

import (
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

func (r Result) Markdown() string {
	var b strings.Builder
	b.WriteString("# Backtest Report\n\n")

	fmt.Fprintf(&b, "- Window: %s → %s\n", formatDay(r.Start), formatDay(r.End))
	fmt.Fprintf(&b, "- Tickers: %s\n", strings.Join(r.Tickers, ", "))
	fmt.Fprintf(&b, "- Deposit: %s RUB\n", r.Deposit.String())
	fmt.Fprintf(&b, "- Final equity: %s RUB\n\n", r.FinalEquity.String())

	b.WriteString("## Performance\n\n")
	fmt.Fprintf(&b, "- Decisions: %d\n", r.Decisions)
	fmt.Fprintf(&b, "- Hold breakdown: %s\n", holdBreakdown(r.HoldReasons))
	fmt.Fprintf(&b, "- Net P&L: **%s RUB** (%s)\n", r.NetPnl.String(), signWord(r.NetPnl))
	fmt.Fprintf(&b, "- Gross P&L: %s RUB\n", r.GrossPnl.String())
	fmt.Fprintf(&b, "- Total commission: %s RUB\n", r.TotalCommission.String())
	fmt.Fprintf(&b, "- Closed trades: %d\n", r.ClosedTrades)
	fmt.Fprintf(&b, "- Winning trades: %d\n", r.WinningTrades)
	if r.ClosedTrades > 0 {
		fmt.Fprintf(&b, "- Hit rate: %.1f%%\n", r.HitRate*100)
	}
	fmt.Fprintf(&b, "- Sharpe (annualized): %.2f\n", r.Sharpe)
	fmt.Fprintf(&b, "- Max drawdown: %.2f%% (%s RUB)\n", r.MaxDrawdownPct, r.MaxDrawdownRub.String())
	fmt.Fprintf(&b, "- Kill switch tripped: %v\n\n", r.KillSwitchTripped)

	if len(r.Trades) == 0 {
		b.WriteString("No closed trades in this window.\n")
		return b.String()
	}

	b.WriteString("## Closed trades\n\n")
	b.WriteString("| Ticker | Side | Lots | Entry | Exit | Gross | Commission | Net | From | To |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|---|---|\n")
	for _, t := range r.Trades {
		fmt.Fprintf(&b, "| %s | %s | %d | %s | %s | %s | %s | **%s** | %s | %s |\n",
			t.Ticker, t.Action, t.Lots, t.EntryPrice.String(), t.ExitPrice.String(),
			t.GrossPnl.String(), t.Commission.String(), t.NetPnl.String(),
			t.OpenedAt.Format("2006-01-02"), t.ClosedAt.Format("2006-01-02"))
	}
	return b.String()
}

func formatDay(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format("2006-01-02")
}

func signWord(value decimal.Decimal) string {
	switch {
	case value.IsNegative():
		return "LOSS"
	case value.IsZero():
		return "flat"
	default:
		return "PROFIT"
	}
}

func holdBreakdown(reasons map[string]int) string {
	if len(reasons) == 0 {
		return "n/a"
	}
	var parts []string
	for _, reason := range []string{"model", "timeout", "invalid", "error"} {
		count, ok := reasons[reason]
		if !ok || count == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%d", reason, count))
	}
	if len(parts) == 0 {
		return "n/a"
	}
	return strings.Join(parts, ", ")
}
