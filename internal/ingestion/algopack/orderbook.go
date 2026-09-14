package algopack

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type Level struct {
	Price    decimal.Decimal `json:"price"`
	Quantity decimal.Decimal `json:"quantity"`
}

type OrderBook struct {
	Ticker         string          `json:"ticker"`
	Timestamp      time.Time       `json:"timestamp"`
	Bids           []Level         `json:"bids,omitempty"`
	Asks           []Level         `json:"asks,omitempty"`
	ImbalanceValue decimal.Decimal `json:"-"`
	imbalanceSet   bool
}

type issObstatsBlock struct {
	Columns []string `json:"columns"`
	Data    [][]any  `json:"data"`
}

func ParseOrderBook(data []byte) (OrderBook, error) {
	var probe struct {
		Obstats json.RawMessage `json:"obstats"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return OrderBook{}, fmt.Errorf("parse algopack order book: %w", err)
	}
	if len(probe.Obstats) > 0 {
		var payload struct {
			Obstats issObstatsBlock `json:"obstats"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return OrderBook{}, fmt.Errorf("parse algopack order book: %w", err)
		}
		return parseObstatsBlock(payload.Obstats)
	}

	var book OrderBook
	if err := json.Unmarshal(data, &book); err != nil {
		return OrderBook{}, fmt.Errorf("parse algopack order book: %w", err)
	}
	book.Bids = validLevels(book.Bids)
	book.Asks = validLevels(book.Asks)
	return book, nil
}

func (b OrderBook) Imbalance() decimal.Decimal {
	if b.imbalanceSet {
		return b.ImbalanceValue
	}
	bidTotal := sumQuantities(b.Bids)
	askTotal := sumQuantities(b.Asks)
	total := bidTotal.Add(askTotal)
	if total.Sign() == 0 {
		return decimal.Zero
	}
	return bidTotal.Sub(askTotal).Div(total)
}

func parseObstatsBlock(block issObstatsBlock) (OrderBook, error) {
	if len(block.Columns) == 0 || len(block.Data) == 0 {
		return OrderBook{}, fmt.Errorf("parse algopack order book: missing obstats columns or data")
	}
	var book OrderBook
	for _, row := range block.Data {
		values := rowMap(block.Columns, row)
		if raw, ok := values["imbalance_vol"]; ok {
			if value, ok := decimalFromAny(raw); ok {
				book.ImbalanceValue = value
				book.imbalanceSet = true
			}
		}
		if book.Ticker == "" {
			book.Ticker = strings.TrimSpace(firstString(values, "secid"))
		}
		if timestamp := timestampFromRow(values); !timestamp.IsZero() {
			book.Timestamp = timestamp
		}
	}
	return book, nil
}

func rowMap(columns []string, row []any) map[string]any {
	values := make(map[string]any, len(columns))
	for index, column := range columns {
		key := strings.ToLower(strings.TrimSpace(column))
		if key == "" || index >= len(row) {
			continue
		}
		values[key] = row[index]
	}
	return values
}

func decimalFromAny(value any) (decimal.Decimal, bool) {
	switch typed := value.(type) {
	case nil:
		return decimal.Decimal{}, false
	case decimal.Decimal:
		return typed, true
	case float64:
		return decimal.NewFromFloat(typed), true
	case float32:
		return decimal.NewFromFloat(float64(typed)), true
	case json.Number:
		parsed, err := decimal.NewFromString(typed.String())
		return parsed, err == nil
	case string:
		parsed, err := decimal.NewFromString(strings.TrimSpace(typed))
		return parsed, err == nil
	case int:
		return decimal.NewFromInt(int64(typed)), true
	case int64:
		return decimal.NewFromInt(typed), true
	case uint64:
		return decimal.NewFromUint64(typed), true
	default:
		return decimal.Decimal{}, false
	}
}

func firstString(values map[string]any, key string) string {
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func timestampFromRow(values map[string]any) time.Time {
	if timestamp, ok := parseTimestamp(firstString(values, "systime")); ok {
		return timestamp
	}
	date := strings.TrimSpace(firstString(values, "tradedate"))
	clock := strings.TrimSpace(firstString(values, "tradetime"))
	combined := date
	if combined != "" && clock != "" {
		combined += " " + clock
	}
	if timestamp, ok := parseTimestamp(combined); ok {
		return timestamp
	}
	if timestamp, ok := parseTimestamp(date); ok {
		return timestamp
	}
	return time.Time{}
}

func parseTimestamp(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
	} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func validLevels(levels []Level) []Level {
	out := make([]Level, 0, len(levels))
	for _, level := range levels {
		if level.Price.Sign() <= 0 || level.Quantity.Sign() <= 0 {
			continue
		}
		out = append(out, level)
	}
	return out
}

func sumQuantities(levels []Level) decimal.Decimal {
	var total decimal.Decimal
	for _, level := range levels {
		total = total.Add(level.Quantity)
	}
	return total
}
