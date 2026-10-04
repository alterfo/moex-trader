package risk

import (
	"sort"

	"github.com/shopspring/decimal"
)

const varTrailingWindow = 250

type VarResult struct {
	VaR95 decimal.Decimal
	VaR99 decimal.Decimal
	ES95  decimal.Decimal
	ES99  decimal.Decimal
	Valid bool
}

func DailyReturns(closes []decimal.Decimal) []decimal.Decimal {
	out := make([]decimal.Decimal, 0, len(closes))
	for i := 1; i < len(closes); i++ {
		prev := closes[i-1]
		if !prev.IsPositive() {
			continue
		}
		out = append(out, closes[i].Div(prev).Sub(decimal.NewFromInt(1)))
	}
	return out
}

func TrailingVaRES(returns []decimal.Decimal) VarResult {
	if len(returns) < 2 {
		return VarResult{}
	}
	start := len(returns) - varTrailingWindow
	if start < 0 {
		start = 0
	}
	sample := append([]decimal.Decimal(nil), returns[start:]...)
	sort.Slice(sample, func(i, j int) bool {
		return sample[i].LessThan(sample[j])
	})

	var95 := quantile(sample, decimal.RequireFromString("0.95"))
	var99 := quantile(sample, decimal.RequireFromString("0.99"))
	return VarResult{
		VaR95: var95,
		VaR99: var99,
		ES95:  expectedShortfall(sample, var95),
		ES99:  expectedShortfall(sample, var99),
		Valid: true,
	}
}

func BookVaRES(book map[string]decimal.Decimal, history map[string][]decimal.Decimal) VarResult {
	returnsByTicker := make(map[string][]decimal.Decimal, len(book))
	minLen := -1
	for ticker, notional := range book {
		if notional.IsZero() {
			continue
		}
		key := normalizeTicker(ticker)
		series := DailyReturns(history[key])
		if len(series) == 0 {
			continue
		}
		returnsByTicker[key] = series
		if minLen < 0 || len(series) < minLen {
			minLen = len(series)
		}
	}
	if minLen < 1 {
		return VarResult{}
	}
	if minLen > varTrailingWindow {
		minLen = varTrailingWindow
	}

	combined := make([]decimal.Decimal, 0, minLen)
	for offset := 0; offset < minLen; offset++ {
		total := decimal.Zero
		for ticker, series := range returnsByTicker {
			index := len(series) - 1 - offset
			total = total.Add(book[normalizeTicker(ticker)].Mul(series[index]))
		}
		combined = append(combined, total)
	}
	return TrailingVaRES(combined)
}

func quantile(sorted []decimal.Decimal, q decimal.Decimal) decimal.Decimal {
	if len(sorted) == 0 {
		return decimal.Zero
	}
	rank := q.Mul(decimal.NewFromInt(int64(len(sorted)))).Ceil()
	index := int(rank.IntPart()) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func expectedShortfall(sorted []decimal.Decimal, threshold decimal.Decimal) decimal.Decimal {
	total := decimal.Zero
	count := decimal.Zero
	for _, value := range sorted {
		if value.LessThanOrEqual(threshold) {
			total = total.Add(value)
			count = count.Add(decimal.NewFromInt(1))
		}
	}
	if count.IsZero() {
		return threshold
	}
	return total.Div(count)
}
