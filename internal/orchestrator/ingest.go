package orchestrator

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/features"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/algopack"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/news"
)

type MOEXIngestor struct {
	moex           *moex.Client
	news           *news.Fetcher
	matcher        *news.Matcher
	sources        []news.Source
	candleInterval int
	candleLookback time.Duration
	logger         *log.Logger
	algopack       algopack.Fetcher
	algopackWorker *algopack.Worker

	mu              sync.Mutex
	newsCache       []news.Article
	newsCacheFilled bool

	cycleMu      sync.Mutex
	cycleResults map[string]chan algopack.Result
}

const (
	newsWindow         = 2 * 24 * time.Hour
	telegramLiveMaxPages = 3
)

func NewMOEXIngestor(moexClient *moex.Client, fetcher *news.Fetcher, matcher *news.Matcher, sources []news.Source, algopackFetcher algopack.Fetcher) *MOEXIngestor {
	if sources == nil {
		sources = news.DefaultSources()
	}
	ingestor := &MOEXIngestor{
		moex:           moexClient,
		news:           fetcher,
		matcher:        matcher,
		sources:        sources,
		candleInterval: 24,
		candleLookback: 10 * 24 * time.Hour,
		logger:         log.Default(),
		algopack:       algopackFetcher,
	}
	if algopackFetcher != nil {
		ingestor.algopackWorker = algopack.NewWorker(algopackFetcher, 2, 10*time.Second)
	}
	return ingestor
}

func (i *MOEXIngestor) Ingest(ctx context.Context, ticker string) (features.Input, error) {
	var empty features.Input
	security, err := i.moex.LookupSecurity(ctx, ticker)
	if err != nil {
		return empty, fmt.Errorf("lookup security %s: %w", ticker, err)
	}
	quote, err := i.moex.Quote(ctx, security)
	if err != nil {
		return empty, fmt.Errorf("quote %s: %w", ticker, err)
	}

	now := time.Now()
	candles, err := i.moex.Candles(ctx, security, i.candleInterval, now.Add(-i.candleLookback), now)
	if err != nil {
		return empty, fmt.Errorf("candles %s: %w", ticker, err)
	}

	articles := i.cycleArticles(ctx)
	matches := i.matcher.Match(articles)

	input := features.Input{
		Ticker: ticker,
		Price: features.PriceSnapshot{
			LastPrice: quote.Last,
			Bid:       quote.Bid,
			Ask:       quote.Ask,
			PrevClose: previousClose(candles, now),
			AsOf:      now,
		},
		Candles: candles,
		News:    filterMatches(matches, ticker),
	}

	if i.algopack != nil {
		if result, ok := i.algopackResult(ctx, ticker); ok {
			if result.Err != nil {
				i.logger.Printf("ingest: algopack order book for %s: %v", ticker, result.Err)
			} else {
				input.OrderBookImbalance = result.Imbalance
			}
		} else {
			book, err := i.algopack.FetchOrderBook(ctx, ticker)
			if err != nil {
				i.logger.Printf("ingest: algopack order book for %s: %v", ticker, err)
			} else {
				input.OrderBookImbalance = book.Imbalance()
			}
		}
	}

	return input, nil
}

func (i *MOEXIngestor) ResetCycle() {
	i.mu.Lock()
	i.newsCacheFilled = false
	i.newsCache = nil
	i.mu.Unlock()
	i.cycleMu.Lock()
	i.cycleResults = nil
	i.cycleMu.Unlock()
}

func (i *MOEXIngestor) PrepareCycle(ctx context.Context, tickers []string) {
	if i.algopackWorker == nil {
		return
	}
	seen := make(map[string]struct{}, len(tickers))
	unique := make([]string, 0, len(tickers))
	for _, ticker := range tickers {
		key := strings.ToUpper(strings.TrimSpace(ticker))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, key)
	}
	if len(unique) == 0 {
		return
	}

	results := make(map[string]chan algopack.Result, len(unique))
	for _, ticker := range unique {
		results[ticker] = make(chan algopack.Result, 1)
	}
	i.cycleMu.Lock()
	i.cycleResults = results
	i.cycleMu.Unlock()

	jobs := make(chan string, len(unique))
	workerResults := i.algopackWorker.Start(ctx, jobs)
	go func() {
		for result := range workerResults {
			key := strings.ToUpper(strings.TrimSpace(result.Ticker))
			if ch, ok := results[key]; ok {
				ch <- result
			}
		}
		for _, ch := range results {
			close(ch)
		}
	}()
	for _, ticker := range unique {
		jobs <- ticker
	}
	close(jobs)
}

func (i *MOEXIngestor) algopackResult(ctx context.Context, ticker string) (algopack.Result, bool) {
	key := strings.ToUpper(strings.TrimSpace(ticker))
	i.cycleMu.Lock()
	ch, ok := i.cycleResults[key]
	i.cycleMu.Unlock()
	if !ok {
		return algopack.Result{}, false
	}
	select {
	case result, open := <-ch:
		if !open {
			return algopack.Result{}, false
		}
		return result, true
	case <-ctx.Done():
		return algopack.Result{Ticker: key, Err: ctx.Err()}, true
	}
}

func (i *MOEXIngestor) cycleArticles(ctx context.Context) []news.Article {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.newsCacheFilled {
		return i.newsCache
	}
	i.newsCache = i.fetchArticles(ctx)
	i.newsCacheFilled = true
	return i.newsCache
}

func (i *MOEXIngestor) fetchArticles(ctx context.Context) []news.Article {
	articles := make([]news.Article, 0)
	attempted := 0
	failures := make([]string, 0)
	for _, source := range i.sources {
		if source.Type == "telegram" {
			attempted++
			items, err := i.news.FetchTelegram(ctx, source, time.Now().Add(-newsWindow), telegramLiveMaxPages)
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s: %v", source.Name, err))
				continue
			}
			for j := range items {
				articles = append(articles, items[j])
			}
			continue
		}
		if strings.TrimSpace(source.URL) == "" {
			continue
		}
		attempted++
		items, err := i.news.Fetch(ctx, source)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", source.Name, err))
			continue
		}
		articles = append(articles, items...)
	}
	if len(failures) > 0 && ctx.Err() == nil {
		i.logger.Printf("ingest: news sources failed (%d of %d): %s", len(failures), attempted, strings.Join(failures, "; "))
	}
	return articles
}

func previousClose(candles []moex.Candle, now time.Time) decimal.Decimal {
	if len(candles) == 0 {
		return decimal.Zero
	}
	last := candles[len(candles)-1]
	if isSameDate(last.Begin, now) && len(candles) >= 2 {
		return candles[len(candles)-2].Close
	}
	return last.Close
}

func isSameDate(candleTime, now time.Time) bool {
	if candleTime.IsZero() {
		return false
	}
	candleYear, candleMonth, candleDay := candleTime.Date()
	nowYear, nowMonth, nowDay := now.Date()
	return candleYear == nowYear && candleMonth == nowMonth && candleDay == nowDay
}

func filterMatches(matches []news.MatchedArticle, ticker string) []news.MatchedArticle {
	out := make([]news.MatchedArticle, 0, len(matches))
	for _, match := range matches {
		if strings.EqualFold(match.Ticker, ticker) {
			out = append(out, match)
		}
	}
	return out
}
