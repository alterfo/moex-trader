package algopack

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shopspring/decimal"
)

func TestHTTPFetcherSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/algopack/SBER.json" {
			http.NotFound(w, r)
			return
		}
		payload := map[string]any{
			"timestamp": "2024-01-11T12:00:00Z",
			"bids": []any{
				map[string]any{"price": 270.1, "quantity": 1000},
			},
			"asks": []any{
				map[string]any{"price": 270.3, "quantity": 500},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer server.Close()

	fetcher := NewHTTPFetcher(server.URL, nil)
	book, err := fetcher.FetchOrderBook(context.Background(), "SBER")
	if err != nil {
		t.Fatalf("FetchOrderBook() error = %v", err)
	}
	if book.Ticker != "SBER" {
		t.Fatalf("expected ticker filled from request, got %q", book.Ticker)
	}
	want := decimal.NewFromInt(1).Div(decimal.NewFromInt(3))
	if book.Imbalance().Sub(want).Abs().GreaterThan(decimal.NewFromFloat(0.000000000000001)) {
		t.Fatalf("unexpected imbalance %s", book.Imbalance())
	}
}

func TestHTTPFetcherError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := NewHTTPFetcher(server.URL, nil).FetchOrderBook(context.Background(), "SBER")
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
}
