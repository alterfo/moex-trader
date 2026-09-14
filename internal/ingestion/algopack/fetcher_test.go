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
		if r.URL.Path != "/algopack/eq/obstats/SBER.json" {
			http.NotFound(w, r)
			return
		}
		payload := map[string]any{
			"obstats": map[string]any{
				"columns": []string{"tradedate", "tradetime", "secid", "imbalance_vol"},
				"data": [][]any{
					{"2024-01-11", "12:00:00", "SBER", 0.5},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer server.Close()

	fetcher := NewHTTPFetcher(server.URL, "", nil)
	book, err := fetcher.FetchOrderBook(context.Background(), "SBER")
	if err != nil {
		t.Fatalf("FetchOrderBook() error = %v", err)
	}
	if book.Ticker != "SBER" {
		t.Fatalf("expected ticker filled from request, got %q", book.Ticker)
	}
	want := decimal.NewFromFloat(0.5)
	if book.Imbalance().Sub(want).Abs().GreaterThan(decimal.NewFromFloat(0.000000000000001)) {
		t.Fatalf("unexpected imbalance %s", book.Imbalance())
	}
}

func TestHTTPFetcherSendsBearerToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/algopack/eq/obstats/OZON.json" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q, want Bearer test-token", got)
		}
		payload := map[string]any{
			"obstats": map[string]any{
				"columns": []string{"secid", "imbalance_vol"},
				"data":    [][]any{{"OZON", -0.25}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer server.Close()

	_, err := NewHTTPFetcher(server.URL, "test-token", nil).FetchOrderBook(context.Background(), "OZON")
	if err != nil {
		t.Fatalf("FetchOrderBook() error = %v", err)
	}
}

func TestHTTPFetcherError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := NewHTTPFetcher(server.URL, "", nil).FetchOrderBook(context.Background(), "SBER")
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
}
