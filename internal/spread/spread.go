package spread

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

const StageIngest = "ingest"

type Options struct {
	MinObservations int
}

type Table map[string]decimal.Decimal

type quoteSnapshot struct {
	Bid json.RawMessage `json:"bid"`
	Ask json.RawMessage `json:"ask"`
}

func FromAuditEvents(events []domain.AuditEvent, opts Options) (Table, error) {
	minObservations := opts.MinObservations
	if minObservations < 1 {
		minObservations = 1
	}

	observations := make(map[string][]decimal.Decimal)
	for _, event := range events {
		if event.Stage != StageIngest {
			continue
		}
		ticker := strings.ToUpper(strings.TrimSpace(event.Ticker))
		if ticker == "" {
			continue
		}
		var quote quoteSnapshot
		if err := json.Unmarshal([]byte(event.Payload), &quote); err != nil {
			continue
		}
		bid, err := decimalFromJSON(quote.Bid)
		if err != nil {
			continue
		}
		ask, err := decimalFromJSON(quote.Ask)
		if err != nil {
			continue
		}
		halfSpread, ok := HalfSpreadPct(bid, ask)
		if !ok {
			continue
		}
		observations[ticker] = append(observations[ticker], halfSpread)
	}

	table := make(Table, len(observations))
	for ticker, values := range observations {
		if len(values) < minObservations {
			continue
		}
		table[ticker] = median(values)
	}
	return table, nil
}

func HalfSpreadPct(bid, ask decimal.Decimal) (decimal.Decimal, bool) {
	if bid.Sign() <= 0 || ask.Sign() <= 0 || ask.LessThan(bid) {
		return decimal.Decimal{}, false
	}
	mid := bid.Add(ask).Div(decimal.NewFromInt(2))
	if mid.Sign() <= 0 {
		return decimal.Decimal{}, false
	}
	return ask.Sub(bid).Div(decimal.NewFromInt(2)).Div(mid), true
}

func (t Table) Lookup(ticker string) (decimal.Decimal, bool) {
	value, ok := t[strings.ToUpper(strings.TrimSpace(ticker))]
	return value, ok
}

func (t Table) Resolve(ticker string, fallback decimal.Decimal) decimal.Decimal {
	if value, ok := t.Lookup(ticker); ok {
		return value
	}
	return fallback
}

func decimalFromJSON(raw json.RawMessage) (decimal.Decimal, error) {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" {
		return decimal.Decimal{}, fmt.Errorf("empty decimal")
	}
	value = strings.Trim(value, `"`)
	parsed, err := decimal.NewFromString(value)
	if err != nil {
		return decimal.Decimal{}, err
	}
	return parsed, nil
}

func median(values []decimal.Decimal) decimal.Decimal {
	sorted := append([]decimal.Decimal(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Cmp(sorted[j]) < 0 })
	if len(sorted)%2 == 1 {
		return sorted[len(sorted)/2]
	}
	upper := len(sorted) / 2
	lower := upper - 1
	return sorted[lower].Add(sorted[upper]).Div(decimal.NewFromInt(2))
}
