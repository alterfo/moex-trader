package finam

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func requireDecimal(t *testing.T, got decimal.Decimal, want string) {
	t.Helper()
	wantDecimal, err := decimal.NewFromString(want)
	if err != nil {
		t.Fatalf("parse want %q: %v", want, err)
	}
	if !got.Equal(wantDecimal) {
		t.Fatalf("got %s, want %s", got.String(), wantDecimal.String())
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func TestNewRejectsMissingSecretToken(t *testing.T) {
	_, err := New(context.Background(), Config{BaseURL: "http://127.0.0.1", SecretToken: " "})
	if err == nil {
		t.Fatal("New returned nil error for empty secret token")
	}
}

func TestNewAuthFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"invalid token"}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := New(context.Background(), Config{BaseURL: server.URL, SecretToken: "bad-secret"})
	if err == nil || !strings.Contains(err.Error(), "auth failed") {
		t.Fatalf("New error = %v, want auth failed", err)
	}
}

func TestNewRejectsMalformedAuthResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"token": 42})
	}))
	defer server.Close()

	_, err := New(context.Background(), Config{BaseURL: server.URL, SecretToken: "secret"})
	if err == nil || !strings.Contains(err.Error(), "decode auth response") {
		t.Fatalf("New error = %v, want decode auth response", err)
	}
}

func TestLastQuoteSuccess(t *testing.T) {
	var authCalls atomic.Int32
	var quoteCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sessions":
			authCalls.Add(1)
			var request authRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode auth request: %v", err)
			}
			if request.Secret != "secret" {
				t.Errorf("auth secret = %q, want secret", request.Secret)
			}
			writeJSON(w, http.StatusOK, authResponse{Token: "jwt-1"})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/instruments/SBER@MISX/quotes/latest":
			quoteCalls.Add(1)
			if got := r.Header.Get("Authorization"); got != "Bearer jwt-1" {
				t.Errorf("Authorization = %q, want Bearer jwt-1", got)
			}
			writeJSON(w, http.StatusOK, lastQuoteResponse{
				Symbol: "SBER@MISX",
				Quote: finamQuote{
					Symbol:    "SBER@MISX",
					Timestamp: "2023-11-14T22:13:20Z",
					Last:      decimalValue{Value: "270.25"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := New(context.Background(), Config{BaseURL: server.URL, SecretToken: "secret"})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	quote, err := client.LastQuote(context.Background(), "SBER@MISX")
	if err != nil {
		t.Fatalf("LastQuote returned error: %v", err)
	}
	if quote.Symbol != "SBER@MISX" {
		t.Fatalf("Symbol = %q, want SBER@MISX", quote.Symbol)
	}
	requireDecimal(t, quote.Price, "270.25")
	wantTime := time.Date(2023, 11, 14, 22, 13, 20, 0, time.UTC)
	if !quote.Time.Equal(wantTime) {
		t.Fatalf("Time = %v, want %v", quote.Time, wantTime)
	}
	if authCalls.Load() != 1 {
		t.Fatalf("auth calls = %d, want 1", authCalls.Load())
	}
	if quoteCalls.Load() != 1 {
		t.Fatalf("quote calls = %d, want 1", quoteCalls.Load())
	}
}

func TestLastQuoteRejectsInvalidSymbol(t *testing.T) {
	client := &Client{}
	if _, err := client.LastQuote(context.Background(), "SBER"); err == nil {
		t.Fatal("LastQuote returned nil error for invalid symbol")
	}
	if _, err := client.Bars(context.Background(), "SBER", TimeframeD, time.Time{}, time.Time{}); err == nil {
		t.Fatal("Bars returned nil error for invalid symbol")
	}
}

func TestBarsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/sessions" {
			writeJSON(w, http.StatusOK, authResponse{Token: "jwt-bars"})
			return
		}
		if r.Method != http.MethodGet || r.URL.Path != "/v1/instruments/SBER@MISX/bars" {
			http.NotFound(w, r)
			return
		}
		query := r.URL.Query()
		if got := query.Get("timeframe"); got != string(TimeframeD) {
			t.Errorf("timeframe = %q, want %q", got, TimeframeD)
		}
		if got := query.Get("interval.start_time"); got != "2023-11-01T00:00:00Z" {
			t.Errorf("interval.start_time = %q", got)
		}
		if got := query.Get("interval.end_time"); got != "2023-11-02T00:00:00Z" {
			t.Errorf("interval.end_time = %q", got)
		}
		writeJSON(w, http.StatusOK, barsResponse{
			Symbol: "SBER@MISX",
			Bars: []finamBar{
				{
					Timestamp: "2023-11-01T00:00:00Z",
					Open:      decimalValue{Value: "200.1"},
					High:      decimalValue{Value: "201"},
					Low:       decimalValue{Value: "199.5"},
					Close:     decimalValue{Value: "200.75"},
					Volume:    decimalValue{Value: "1234"},
				},
			},
		})
	}))
	defer server.Close()

	client, err := New(context.Background(), Config{BaseURL: server.URL, SecretToken: "secret"})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	from := time.Date(2023, 11, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2023, 11, 2, 0, 0, 0, 0, time.UTC)
	candles, err := client.Bars(context.Background(), "SBER@MISX", TimeframeD, from, to)
	if err != nil {
		t.Fatalf("Bars returned error: %v", err)
	}
	if len(candles) != 1 {
		t.Fatalf("len(candles) = %d, want 1", len(candles))
	}
	requireDecimal(t, candles[0].Open, "200.1")
	requireDecimal(t, candles[0].High, "201")
	requireDecimal(t, candles[0].Low, "199.5")
	requireDecimal(t, candles[0].Close, "200.75")
	requireDecimal(t, candles[0].Volume, "1234")
	if !candles[0].Begin.Equal(from) {
		t.Fatalf("Begin = %v, want %v", candles[0].Begin, from)
	}
	if !candles[0].End.IsZero() {
		t.Fatalf("End = %v, want zero", candles[0].End)
	}
}

func TestBarsRejectsInvalidRange(t *testing.T) {
	client := &Client{}
	to := time.Now().Add(-time.Hour)
	if _, err := client.Bars(context.Background(), "SBER@MISX", TimeframeD, time.Now(), to); err == nil {
		t.Fatal("Bars returned nil error for reversed range")
	}
	if _, err := client.Bars(context.Background(), "SBER@MISX", Timeframe("TIME_FRAME_X"), time.Time{}, time.Time{}); err == nil {
		t.Fatal("Bars returned nil error for unsupported timeframe")
	}
}

func TestLastQuoteMalformedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/sessions" {
			writeJSON(w, http.StatusOK, authResponse{Token: "jwt-malformed"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"symbol":`))
	}))
	defer server.Close()

	client, err := New(context.Background(), Config{BaseURL: server.URL, SecretToken: "secret"})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if _, err := client.LastQuote(context.Background(), "SBER@MISX"); err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Fatalf("LastQuote error = %v, want decode response", err)
	}
}

func TestJWTRefreshBeforeExpiry(t *testing.T) {
	var authCalls atomic.Int32
	var quoteCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sessions":
			call := authCalls.Add(1)
			writeJSON(w, http.StatusOK, authResponse{Token: fmt.Sprintf("jwt-%d", call)})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/instruments/SBER@MISX/quotes/latest":
			quoteCalls.Add(1)
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer jwt-") {
				t.Errorf("Authorization = %q, want Bearer jwt-<n>", r.Header.Get("Authorization"))
			}
			writeJSON(w, http.StatusOK, lastQuoteResponse{
				Symbol: "SBER@MISX",
				Quote: finamQuote{
					Timestamp: "2023-11-14T22:13:20Z",
					Last:      decimalValue{Value: "270.25"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := New(context.Background(), Config{BaseURL: server.URL, SecretToken: "secret"})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	client.mu.Lock()
	client.tokenExpiresAt = client.now().Add(time.Second)
	client.mu.Unlock()

	if _, err := client.LastQuote(context.Background(), "SBER@MISX"); err != nil {
		t.Fatalf("LastQuote returned error: %v", err)
	}
	if authCalls.Load() != 2 {
		t.Fatalf("auth calls = %d, want 2", authCalls.Load())
	}
	if quoteCalls.Load() != 1 {
		t.Fatalf("quote calls = %d, want 1", quoteCalls.Load())
	}
}

func TestExpiredJWTTriggersRefreshAndRetry(t *testing.T) {
	var authCalls atomic.Int32
	var quoteCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sessions":
			call := authCalls.Add(1)
			writeJSON(w, http.StatusOK, authResponse{Token: fmt.Sprintf("jwt-%d", call)})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/instruments/SBER@MISX/quotes/latest":
			quoteCalls.Add(1)
			if r.Header.Get("Authorization") == "Bearer stale-token" {
				writeJSON(w, http.StatusUnauthorized, map[string]any{"message": "expired"})
				return
			}
			if r.Header.Get("Authorization") != "Bearer jwt-2" {
				t.Errorf("Authorization = %q, want Bearer jwt-2", r.Header.Get("Authorization"))
			}
			writeJSON(w, http.StatusOK, lastQuoteResponse{
				Symbol: "SBER@MISX",
				Quote: finamQuote{
					Timestamp: "2023-11-14T22:13:20Z",
					Last:      decimalValue{Value: "270.25"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := New(context.Background(), Config{BaseURL: server.URL, SecretToken: "secret"})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	client.mu.Lock()
	client.token = "stale-token"
	client.tokenExpiresAt = client.now().Add(10 * time.Minute)
	client.mu.Unlock()

	quote, err := client.LastQuote(context.Background(), "SBER@MISX")
	if err != nil {
		t.Fatalf("LastQuote returned error: %v", err)
	}
	requireDecimal(t, quote.Price, "270.25")
	if authCalls.Load() != 2 {
		t.Fatalf("auth calls = %d, want 2", authCalls.Load())
	}
	if quoteCalls.Load() != 2 {
		t.Fatalf("quote calls = %d, want 2", quoteCalls.Load())
	}
}
