package algopack

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/features"
)

type fakeFetcher struct {
	books map[string]OrderBook
	errs  map[string]error
	delay map[string]time.Duration
}

func (f *fakeFetcher) FetchOrderBook(ctx context.Context, ticker string) (OrderBook, error) {
	if d := f.delay[ticker]; d > 0 {
		select {
		case <-time.After(d):
		case <-ctx.Done():
			return OrderBook{}, ctx.Err()
		}
	}
	if err := f.errs[ticker]; err != nil {
		return OrderBook{}, err
	}
	return f.books[ticker], nil
}

func runWorker(t *testing.T, worker *Worker, tickers ...string) map[string]Result {
	t.Helper()
	jobs := make(chan string)
	results := worker.Start(context.Background(), jobs)
	go func() {
		defer close(jobs)
		for _, ticker := range tickers {
			jobs <- ticker
		}
	}()
	collected := make(map[string]Result, len(tickers))
	for result := range results {
		collected[result.Ticker] = result
	}
	return collected
}

func TestWorkerSuccess(t *testing.T) {
	fetcher := &fakeFetcher{books: map[string]OrderBook{
		"SBER": {
			Bids: []Level{{Price: decimal.NewFromFloat(270), Quantity: decimal.NewFromFloat(1500)}},
			Asks: []Level{{Price: decimal.NewFromFloat(271), Quantity: decimal.NewFromFloat(1000)}},
		},
		"OZON": {
			Bids: []Level{{Price: decimal.NewFromFloat(210), Quantity: decimal.NewFromFloat(800)}},
			Asks: []Level{{Price: decimal.NewFromFloat(211), Quantity: decimal.NewFromFloat(200)}},
		},
	}}
	worker := NewWorker(fetcher, 2, time.Second)

	results := runWorker(t, worker, "SBER", "OZON")
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results["SBER"].Err != nil {
		t.Fatalf("unexpected SBER error %v", results["SBER"].Err)
	}
	if !results["SBER"].Imbalance.Equal(decimal.NewFromFloat(0.2)) {
		t.Fatalf("unexpected SBER imbalance %s", results["SBER"].Imbalance)
	}
	if !results["OZON"].Imbalance.Equal(decimal.NewFromFloat(0.6)) {
		t.Fatalf("unexpected OZON imbalance %s", results["OZON"].Imbalance)
	}
}

func TestWorkerPartialData(t *testing.T) {
	fetcher := &fakeFetcher{books: map[string]OrderBook{
		"SBER": {
			Bids: []Level{{Price: decimal.NewFromFloat(270), Quantity: decimal.NewFromFloat(900)}},
		},
	}}
	worker := NewWorker(fetcher, 1, time.Second)

	results := runWorker(t, worker, "SBER")
	result := results["SBER"]
	if result.Err != nil {
		t.Fatalf("unexpected error %v", result.Err)
	}
	if !result.Imbalance.Equal(decimal.NewFromInt(1)) {
		t.Fatalf("unexpected imbalance %s", result.Imbalance)
	}
}

func TestWorkerTimeout(t *testing.T) {
	fetcher := &fakeFetcher{delay: map[string]time.Duration{"SBER": 200 * time.Millisecond}}
	worker := NewWorker(fetcher, 1, 20*time.Millisecond)

	results := runWorker(t, worker, "SBER")
	result := results["SBER"]
	if result.Err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(result.Err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", result.Err)
	}
}

func TestWorkerFansIntoFeatureBuilder(t *testing.T) {
	fetcher := &fakeFetcher{books: map[string]OrderBook{
		"SBER": {
			Bids: []Level{{Price: decimal.NewFromFloat(270), Quantity: decimal.NewFromFloat(1500)}},
			Asks: []Level{{Price: decimal.NewFromFloat(271), Quantity: decimal.NewFromFloat(1000)}},
		},
	}}
	worker := NewWorker(fetcher, 1, time.Second)

	results := runWorker(t, worker, "SBER")
	result := results["SBER"]
	if result.Err != nil {
		t.Fatalf("unexpected error %v", result.Err)
	}
	enriched := features.EnrichWithAlgoPack(domain.FeatureContext{Ticker: "SBER"}, result.Imbalance)
	if !enriched.OrderBookImbalance.Equal(decimal.NewFromFloat(0.2)) {
		t.Fatalf("unexpected enriched imbalance %s", enriched.OrderBookImbalance)
	}
}
