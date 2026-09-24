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
	fmt.Fprintf(&b, "- Net P&L (MTM): **%s RUB** (%s)\n", r.NetPnl.String(), signWord(r.NetPnl))
	fmt.Fprintf(&b, "- Realized P&L (closed trades, gross - commission): **%s RUB** (%s)\n", r.RealizedPnl.String(), signWord(r.RealizedPnl))
	fmt.Fprintf(&b, "- Unrealized P&L (open positions at cutoff, MTM): %s RUB\n", r.UnrealizedPnl.String())
	fmt.Fprintf(&b, "- Gross P&L (closed trades): %s RUB\n", r.GrossPnl.String())
	fmt.Fprintf(&b, "- Total commission: %s RUB\n", r.TotalCommission.String())
	if !r.TotalBorrow.IsZero() {
		fmt.Fprintf(&b, "- Short-borrow cost (stress): %s RUB\n", r.TotalBorrow.String())
		fmt.Fprintf(&b, "- Realized P&L net of borrow: **%s RUB** (%s)\n", r.RealizedPnlNetBorrow.String(), signWord(r.RealizedPnlNetBorrow))
	}
	fmt.Fprintf(&b, "- Closed trades: %d\n", r.ClosedTrades)
	fmt.Fprintf(&b, "- Winning trades: %d\n", r.WinningTrades)
	if r.ClosedTrades > 0 {
		fmt.Fprintf(&b, "- Hit rate: %.1f%%\n", r.HitRate*100)
	}
	fmt.Fprintf(&b, "- CAGR: %.2f%%\n", r.CAGR)
	fmt.Fprintf(&b, "- Sharpe (annualized): %.2f\n", r.Sharpe)
	fmt.Fprintf(&b, "- Sortino (annualized): %.2f\n", r.Sortino)
	fmt.Fprintf(&b, "- Calmar: %.2f\n", r.Calmar)
	fmt.Fprintf(&b, "- Max drawdown: %.2f%% (%s RUB)\n", r.MaxDrawdownPct, r.MaxDrawdownRub.String())
	fmt.Fprintf(&b, "- Kill switch tripped: %v\n", r.KillSwitchTripped)
	fmt.Fprintf(&b, "- Kill switch frozen days: %d\n", r.KillSwitchFrozenDays)
	fmt.Fprintf(&b, "- Daily-loss blocked days: %d\n", r.DailyLossBlockedDays)
	if !r.StatisticallySignificant() {
		fmt.Fprintf(&b, "- ⚠ %d closed trades < %d — metrics are not statistically significant\n", r.ClosedTrades, MinTradesForSignificance)
	}
	b.WriteString("\n")

	b.WriteString("## Attribution\n\n")
	fmt.Fprintf(&b, "%s\n", attributionSummary(r.Attribution))
	fmt.Fprintf(&b, "%s", attributionTable(r.Attribution))
	b.WriteString("\n")

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

func attributionSummary(attribution Attribution) string {
	realizedAbs := attribution.RealizedTotal.Abs()
	unrealizedAbs := attribution.UnrealizedTotal.Abs()
	denom := realizedAbs.Add(unrealizedAbs)
	realizedPct, unrealizedPct := 0.0, 0.0
	if denom.Sign() > 0 {
		realizedPct, _ = realizedAbs.Div(denom).Mul(decimal.NewFromInt(100)).Float64()
		unrealizedPct, _ = unrealizedAbs.Div(denom).Mul(decimal.NewFromInt(100)).Float64()
	}

	var b strings.Builder
	fmt.Fprintf(&b, "- Realized/unrealized split: realized %s RUB (%.1f%% of |realized|+|unrealized|), unrealized %s RUB (%.1f%%)\n",
		attribution.RealizedTotal.String(), realizedPct, attribution.UnrealizedTotal.String(), unrealizedPct)
	if attribution.TradeCount > 0 {
		fmt.Fprintf(&b, "- Top %d trade concentration: %.1f%% of |realized P&L|\n", attribution.TopN, attribution.TopTradeShare*100)
		fmt.Fprintf(&b, "- Top %d ticker concentration: %.1f%% of |realized P&L|\n", attribution.TopN, attribution.TopTickerShare*100)
	}
	return b.String()
}

func attributionTable(attribution Attribution) string {
	if len(attribution.Tickers) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("- Realized P&L by ticker:\n\n")
	b.WriteString("| Ticker | Trades | Wins | Losses | Gross | Commission | Realized | Unrealized | Total |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|---|\n")
	for _, ticker := range attribution.Tickers {
		total := ticker.RealizedPnl.Add(ticker.UnrealizedPnl)
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %s | %s | **%s** | %s | **%s** |\n",
			ticker.Ticker, ticker.Trades, ticker.Wins, ticker.Losses,
			ticker.GrossPnl.String(), ticker.Commission.String(),
			ticker.RealizedPnl.String(), ticker.UnrealizedPnl.String(), total.String())
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
	for _, reason := range []string{"model", "timeout", "invalid", "error", "confidence"} {
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
