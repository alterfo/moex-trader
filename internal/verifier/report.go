package verifier

import (
	"fmt"
	"strings"
	"time"
)

func (r Report) Markdown() string {
	var b strings.Builder
	b.WriteString("# Verifier Report\n\n")
	fmt.Fprintf(&b, "- Generated at: %s\n", r.GeneratedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "- Window: %s → %s\n", r.Since.Format(time.RFC3339), r.Until.Format(time.RFC3339))
	fmt.Fprintf(&b, "- Closed trades: %d\n", r.ClosedTrades)
	fmt.Fprintf(&b, "- Losing trades: %d\n", r.LosingTrades)
	fmt.Fprintf(&b, "- Total realized P&L (gross): %s\n", r.TotalGrossPnL.String())
	fmt.Fprintf(&b, "- Total commissions: %s\n", r.TotalCommission.String())
	fmt.Fprintf(&b, "- Total realized P&L (net): %s\n\n", r.TotalRealizedPnL.String())

	if len(r.Trades) == 0 {
		b.WriteString("No losing trades in this window.\n")
		return b.String()
	}

	for _, trade := range r.Trades {
		fmt.Fprintf(&b, "## Losing trade: %s\n", trade.Ticker)
		fmt.Fprintf(&b, "- Direction: %s\n", trade.Direction)
		fmt.Fprintf(&b, "- Lots: %d\n", trade.Lots)
		fmt.Fprintf(&b, "- Entry price: %s\n", trade.EntryPrice.String())
		fmt.Fprintf(&b, "- Exit price: %s\n", trade.ExitPrice.String())
		fmt.Fprintf(&b, "- Gross P&L: %s\n", trade.GrossPnL.String())
		fmt.Fprintf(&b, "- Commission: %s\n", trade.Commission.String())
		fmt.Fprintf(&b, "- Net P&L: %s\n", trade.RealizedPnL.String())
		fmt.Fprintf(&b, "- Opened at: %s\n", trade.OpenedAt.Format(time.RFC3339))
		fmt.Fprintf(&b, "- Closed at: %s\n", trade.ClosedAt.Format(time.RFC3339))
		if trade.Signal != nil {
			fmt.Fprintf(&b, "- LLM signal: %s (confidence %s)\n", trade.Signal.Action, trade.Signal.Confidence.String())
			fmt.Fprintf(&b, "- Reasoning: %s\n", trade.Signal.Reasoning)
		} else {
			b.WriteString("- LLM signal: none\n")
		}
		fmt.Fprintf(&b, "- Was the LLM signal adequate? %s\n\n", trade.Adequacy)
	}

	return b.String()
}
