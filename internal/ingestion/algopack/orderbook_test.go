package algopack

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

func TestParseOrderBook(t *testing.T) {
	book, err := ParseOrderBook([]byte(`{
		"ticker": "SBER",
		"timestamp": "2024-01-11T12:00:00Z",
		"bids": [
			{"price": 270.1, "quantity": 1000},
			{"price": 270.0, "quantity": 500}
		],
		"asks": [
			{"price": 270.3, "quantity": 400},
			{"price": 270.4, "quantity": 600}
		]
	}`))
	if err != nil {
		t.Fatalf("ParseOrderBook() error = %v", err)
	}
	if book.Ticker != "SBER" {
		t.Fatalf("unexpected ticker %q", book.Ticker)
	}
	if len(book.Bids) != 2 || len(book.Asks) != 2 {
		t.Fatalf("unexpected level counts bids=%d asks=%d", len(book.Bids), len(book.Asks))
	}
	want := decimal.NewFromFloat(0.2)
	if !book.Imbalance().Equal(want) {
		t.Fatalf("unexpected imbalance %s, want %s", book.Imbalance(), want)
	}
}

func TestParseObstatsEnvelope(t *testing.T) {
	book, err := ParseOrderBook([]byte(`{
		"obstats": {
			"columns": ["tradedate", "tradetime", "secid", "imbalance_vol"],
			"data": [
				["2024-01-11", "11:00:00", "SBER", 0.1],
				["2024-01-11", "12:00:00", "SBER", -0.4]
			]
		}
	}`))
	if err != nil {
		t.Fatalf("ParseOrderBook() error = %v", err)
	}
	if book.Ticker != "SBER" {
		t.Fatalf("unexpected ticker %q", book.Ticker)
	}
	if book.Timestamp.IsZero() {
		t.Fatal("expected timestamp from obstats row")
	}
	if !book.Imbalance().Equal(decimal.NewFromFloat(-0.4)) {
		t.Fatalf("unexpected imbalance %s, want -0.4", book.Imbalance())
	}
}

func TestParseObstatsMissingImbalanceFallsBack(t *testing.T) {
	book, err := ParseOrderBook([]byte(`{
		"obstats": {
			"columns": ["secid", "levels_b"],
			"data": [["SBER", 10]]
		}
	}`))
	if err != nil {
		t.Fatalf("ParseOrderBook() error = %v", err)
	}
	if !book.Imbalance().IsZero() {
		t.Fatalf("expected zero imbalance without imbalance_vol, got %s", book.Imbalance())
	}
}

func TestParseOrderBookPartialData(t *testing.T) {
	book, err := ParseOrderBook([]byte(`{
		"ticker": "OZON",
		"bids": [
			{"price": 210.0, "quantity": 900},
			{"price": 209.9, "quantity": 0},
			{"price": 0, "quantity": 100}
		]
	}`))
	if err != nil {
		t.Fatalf("ParseOrderBook() error = %v", err)
	}
	if len(book.Bids) != 1 {
		t.Fatalf("expected invalid levels to be skipped, got %d bids", len(book.Bids))
	}
	if !book.Imbalance().Equal(decimal.NewFromInt(1)) {
		t.Fatalf("expected ask-only imbalance of 1, got %s", book.Imbalance())
	}
}

func TestParseOrderBookEmptySides(t *testing.T) {
	book, err := ParseOrderBook([]byte(`{"ticker": "SBER"}`))
	if err != nil {
		t.Fatalf("ParseOrderBook() error = %v", err)
	}
	if !book.Imbalance().IsZero() {
		t.Fatalf("expected zero imbalance for empty book, got %s", book.Imbalance())
	}
}

func TestParseOrderBookBalanced(t *testing.T) {
	book, err := ParseOrderBook([]byte(`{
		"bids": [{"price": 100, "quantity": 1000}],
		"asks": [{"price": 101, "quantity": 1000}]
	}`))
	if err != nil {
		t.Fatalf("ParseOrderBook() error = %v", err)
	}
	if !book.Imbalance().IsZero() {
		t.Fatalf("expected zero imbalance for balanced book, got %s", book.Imbalance())
	}
}

func TestParseOrderBookMalformed(t *testing.T) {
	_, err := ParseOrderBook([]byte(`{"ticker": `))
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "parse algopack order book") {
		t.Fatalf("unexpected error %v", err)
	}
}
