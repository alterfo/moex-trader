package orchestrator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/ingestion/algopack"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/news"
)

type fakeAlgoPackFetcher struct {
	book algopack.OrderBook
	err  error
}

func (f fakeAlgoPackFetcher) FetchOrderBook(_ context.Context, _ string) (algopack.OrderBook, error) {
	return f.book, f.err
}

func TestMOEXIngestorCachesNewsPerCycleAndPopulatesQuote(t *testing.T) {
	var newsCalls atomic.Int32
	newsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		newsCalls.Add(1)
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><item>
<title>Сбербанк отчитался о прибыли</title>
<link>https://example.com/sber</link>
<description>Прибыль выросла.</description>
<pubDate>Mon, 09 Jan 2024 10:00:00 +0300</pubDate>
</item></channel></rss>`))
	}))
	defer newsServer.Close()

	moexServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/securities/SBER.json":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"description": map[string]any{
					"columns": []string{"name", "title", "value", "type", "sort_order", "is_hidden", "precision"},
					"data":    [][]any{{"SECID", "Идентификатор инструмента", "SBER", "string", 0, 0, nil}},
				},
				"boards": map[string]any{
					"columns": []string{"secid", "boardid", "engine", "market", "is_traded", "is_primary"},
					"data":    [][]any{{"SBER", "TQBR", "stock", "shares", 1, 1}},
				},
			})
		case r.URL.Path == "/engines/stock/markets/shares/boards/TQBR/securities/SBER.json":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"marketdata": map[string]any{
					"columns": []string{"SECID", "BOARDID", "LAST", "BID", "OFFER"},
					"data":    [][]any{{"SBER", "TQBR", 272.25, 272.2, 272.3}},
				},
			})
		case strings.Contains(r.URL.Path, "/candles.json"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"candles": map[string]any{
					"columns": []string{"open", "close", "high", "low", "value", "volume", "begin", "end"},
					"data": [][]any{
						{270.1, 271.5, 272.0, 269.8, 1000000, 3700, "2024-01-09 10:00:00", "2024-01-09 18:00:00"},
						{271.5, 272.2, 273.0, 271.0, 1100000, 4100, "2024-01-10 10:00:00", "2024-01-10 18:00:00"},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer moexServer.Close()

	moexClient := moex.NewClient(moexServer.URL, nil)
	matcher := news.NewMatcher(map[string][]string{"SBER": {"сбербанк", "сбер"}})
	ingestor := NewMOEXIngestor(moexClient, news.NewFetcher(nil), matcher, []news.Source{
		{Name: "test", URL: newsServer.URL, Type: "rss"},
	}, fakeAlgoPackFetcher{book: algopack.OrderBook{
		Ticker: "SBER",
		Bids:   []algopack.Level{{Price: decimal.NewFromFloat(270.1), Quantity: decimal.NewFromFloat(1000)}},
		Asks:   []algopack.Level{{Price: decimal.NewFromFloat(270.3), Quantity: decimal.NewFromFloat(500)}},
	}})

	ingestor.ResetCycle()
	first, err := ingestor.Ingest(context.Background(), "SBER")
	if err != nil {
		t.Fatalf("first Ingest() error = %v", err)
	}
	second, err := ingestor.Ingest(context.Background(), "SBER")
	if err != nil {
		t.Fatalf("second Ingest() error = %v", err)
	}
	if newsCalls.Load() != 1 {
		t.Fatalf("news requests = %d, want 1 within one cycle", newsCalls.Load())
	}
	if len(first.News) == 0 || len(second.News) == 0 {
		t.Fatalf("expected matched news for SBER, got first=%d second=%d", len(first.News), len(second.News))
	}
	if first.Price.Bid.String() != "272.2" || first.Price.Ask.String() != "272.3" {
		t.Fatalf("unexpected quote bid/ask: %s/%s", first.Price.Bid, first.Price.Ask)
	}
	if !first.OrderBookImbalance.Equal(decimal.NewFromInt(1).Div(decimal.NewFromInt(3))) {
		t.Fatalf("unexpected order book imbalance %s", first.OrderBookImbalance)
	}

	ingestor.ResetCycle()
	if _, err := ingestor.Ingest(context.Background(), "SBER"); err != nil {
		t.Fatalf("third Ingest() error = %v", err)
	}
	if newsCalls.Load() != 2 {
		t.Fatalf("news requests = %d, want 2 after cycle reset", newsCalls.Load())
	}
}
