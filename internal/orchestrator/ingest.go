package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/features"
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
}

func NewMOEXIngestor(moexClient *moex.Client, fetcher *news.Fetcher, matcher *news.Matcher, sources []news.Source) *MOEXIngestor {
	if sources == nil {
		sources = news.DefaultSources()
	}
	return &MOEXIngestor{
		moex:           moexClient,
		news:           fetcher,
		matcher:        matcher,
		sources:        sources,
		candleInterval: 24,
		candleLookback: 10 * 24 * time.Hour,
	}
}

func (i *MOEXIngestor) Ingest(ctx context.Context, ticker string) (features.Input, error) {
	var empty features.Input
	security, err := i.moex.LookupSecurity(ctx, ticker)
	if err != nil {
		return empty, fmt.Errorf("lookup security %s: %w", ticker, err)
	}
	lastPrice, err := i.moex.LastPrice(ctx, security)
	if err != nil {
		return empty, fmt.Errorf("last price %s: %w", ticker, err)
	}

	now := time.Now()
	candles, err := i.moex.Candles(ctx, security, i.candleInterval, now.Add(-i.candleLookback), now)
	if err != nil {
		return empty, fmt.Errorf("candles %s: %w", ticker, err)
	}

	articles := i.fetchArticles(ctx)
	matches := i.matcher.Match(articles)

	return features.Input{
		Ticker: ticker,
		Price: features.PriceSnapshot{
			LastPrice: lastPrice,
			PrevClose: previousClose(candles, now),
			AsOf:      now,
		},
		Candles: candles,
		News:    filterMatches(matches, ticker),
	}, nil
}

func (i *MOEXIngestor) fetchArticles(ctx context.Context) []news.Article {
	articles := make([]news.Article, 0)
	for _, source := range i.sources {
		if strings.TrimSpace(source.URL) == "" {
			continue
		}
		items, err := i.news.Fetch(ctx, source)
		if err != nil {
			continue
		}
		articles = append(articles, items...)
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
