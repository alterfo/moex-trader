package moex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestLookupSecurity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/securities/SBER.json" {
			http.NotFound(w, r)
			return
		}
		payload := map[string]any{
			"description": map[string]any{
				"columns": []string{"secid", "name", "primary_boardid"},
				"data":    [][]any{{"SBER", "Сбербанк", "TQBR"}},
			},
			"boards": map[string]any{
				"columns": []string{"boardid", "engine", "market", "is_primary"},
				"data": [][]any{
					{"TQBR", "stock", "shares", 1},
					{"SMAL", "stock", "shares", 0},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer server.Close()

	client := NewClient(server.URL, nil)
	sec, err := client.LookupSecurity(context.Background(), "SBER")
	if err != nil {
		t.Fatalf("LookupSecurity() error = %v", err)
	}
	if sec.SecID != "SBER" || sec.Board != "TQBR" || sec.Engine != "stock" || sec.Market != "shares" {
		t.Fatalf("unexpected security: %+v", sec)
	}
}

func TestCandles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/engines/stock/markets/shares/boards/TQBR/securities/SBER/candles.json" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("interval") != "24" {
			t.Fatalf("unexpected interval %q", r.URL.Query().Get("interval"))
		}
		payload := map[string]any{
			"candles": map[string]any{
				"columns": []string{"open", "close", "high", "low", "value", "volume", "begin", "end"},
				"data": [][]any{
					{270.1, 271.5, 272.0, 269.8, 1000000, 3700, "2024-01-09 10:00:00", "2024-01-09 18:00:00"},
					{271.5, 272.2, 273.0, 271.0, 1100000, 4100, "2024-01-10 10:00:00", "2024-01-10 18:00:00"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer server.Close()

	client := NewClient(server.URL, nil)
	from := time.Date(2024, 1, 9, 0, 0, 0, 0, time.UTC)
	sec := Security{SecID: "SBER", Board: "TQBR", Engine: "stock", Market: "shares"}
	candles, err := client.Candles(context.Background(), sec, 24, from, time.Time{})
	if err != nil {
		t.Fatalf("Candles() error = %v", err)
	}
	if len(candles) != 2 {
		t.Fatalf("expected 2 candles, got %d", len(candles))
	}
	if !candles[0].Close.Equal(decimal.NewFromFloat(271.5)) {
		t.Fatalf("unexpected first close %s", candles[0].Close)
	}
	if !candles[1].Open.Equal(decimal.NewFromFloat(271.5)) {
		t.Fatalf("unexpected second open %s", candles[1].Open)
	}
	if candles[0].Begin.Format("2006-01-02") != "2024-01-09" {
		t.Fatalf("unexpected begin %v", candles[0].Begin)
	}
}

func TestLastPrice(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/engines/stock/markets/shares/securities/SBER.json" {
			http.NotFound(w, r)
			return
		}
		payload := map[string]any{
			"marketdata": map[string]any{
				"columns": []string{"SECID", "BOARDID", "LAST"},
				"data":    [][]any{{"SBER", "TQBR", 272.25}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer server.Close()

	client := NewClient(server.URL, nil)
	price, err := client.LastPrice(context.Background(), Security{SecID: "SBER", Engine: "stock", Market: "shares"})
	if err != nil {
		t.Fatalf("LastPrice() error = %v", err)
	}
	if !price.Equal(decimal.NewFromFloat(272.25)) {
		t.Fatalf("unexpected price %s", price)
	}
}

func TestHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewClient(server.URL, nil)
	_, err := client.LastPrice(context.Background(), Security{SecID: "SBER", Engine: "stock", Market: "shares"})
	if err == nil {
		t.Fatal("expected error for HTTP error response")
	}
	if !strings.Contains(err.Error(), "503") {
		t.Fatalf("expected 503 in error, got %v", err)
	}
}

func TestMalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"marketdata": {"columns": [`)
	}))
	defer server.Close()

	client := NewClient(server.URL, nil)
	_, err := client.LastPrice(context.Background(), Security{SecID: "SBER", Engine: "stock", Market: "shares"})
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "parse moex JSON") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()

	client := NewClient(server.URL, &http.Client{Timeout: 20 * time.Millisecond})
	_, err := client.LastPrice(context.Background(), Security{SecID: "SBER", Engine: "stock", Market: "shares"})
	if err == nil {
		t.Fatal("expected error for request timeout")
	}
}
