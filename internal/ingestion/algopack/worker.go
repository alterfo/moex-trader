package algopack

import (
	"context"
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

type Result struct {
	Ticker    string
	Imbalance decimal.Decimal
	AsOf      time.Time
	Err       error
}

type Worker struct {
	fetcher     Fetcher
	concurrency int
	timeout     time.Duration
}

func NewWorker(fetcher Fetcher, concurrency int, timeout time.Duration) *Worker {
	if concurrency <= 0 {
		concurrency = 1
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Worker{fetcher: fetcher, concurrency: concurrency, timeout: timeout}
}

func (w *Worker) Start(ctx context.Context, jobs <-chan string) <-chan Result {
	results := make(chan Result)
	var wg sync.WaitGroup
	for i := 0; i < w.concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ticker := range jobs {
				result := w.enrich(ctx, ticker)
				select {
				case results <- result:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(results)
	}()
	return results
}

func (w *Worker) enrich(ctx context.Context, ticker string) Result {
	callCtx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()

	book, err := w.fetcher.FetchOrderBook(callCtx, ticker)
	if err != nil {
		return Result{Ticker: ticker, Err: err}
	}
	return Result{
		Ticker:    ticker,
		Imbalance: book.Imbalance(),
		AsOf:      book.Timestamp,
	}
}
