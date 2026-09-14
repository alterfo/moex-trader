package algopack

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

type Level struct {
	Price    decimal.Decimal `json:"price"`
	Quantity decimal.Decimal `json:"quantity"`
}

type OrderBook struct {
	Ticker    string    `json:"ticker"`
	Timestamp time.Time `json:"timestamp"`
	Bids      []Level   `json:"bids"`
	Asks      []Level   `json:"asks"`
}

func ParseOrderBook(data []byte) (OrderBook, error) {
	var book OrderBook
	if err := json.Unmarshal(data, &book); err != nil {
		return OrderBook{}, fmt.Errorf("parse algopack order book: %w", err)
	}
	book.Bids = validLevels(book.Bids)
	book.Asks = validLevels(book.Asks)
	return book, nil
}

func (b OrderBook) Imbalance() decimal.Decimal {
	bidTotal := sumQuantities(b.Bids)
	askTotal := sumQuantities(b.Asks)
	total := bidTotal.Add(askTotal)
	if total.Sign() == 0 {
		return decimal.Zero
	}
	return bidTotal.Sub(askTotal).Div(total)
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
