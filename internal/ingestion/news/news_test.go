package news

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

const rssFixture = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Тестовые новости</title>
    <item>
      <title>Сбербанк отчитался о прибыли</title>
      <link>https://example.com/sber</link>
      <description>Крупнейший банк увеличил прибыль.</description>
      <pubDate>Mon, 09 Jan 2024 10:00:00 +0300</pubDate>
    </item>
    <item>
      <title>Цены на золото выросли</title>
      <link>https://example.com/gold</link>
      <description>Драгметалл обновил максимум.</description>
      <pubDate>Tue, 10 Jan 2024 11:30:00 +0300</pubDate>
    </item>
  </channel>
</rss>`

func TestDefaultSources(t *testing.T) {
	sources := DefaultSources()
	if len(sources) != 14 {
		t.Fatalf("expected 14 sources, got %d", len(sources))
	}
	rssCount := 0
	telegramCount := 0
	for _, source := range sources {
		if source.Type == "telegram" {
			telegramCount++
			continue
		}
		rssCount++
		if source.URL == "" {
			t.Fatalf("source %q has empty URL", source.Name)
		}
		if source.TrustWeight.IsZero() {
			t.Fatalf("source %q has zero trust weight", source.Name)
		}
	}
	if rssCount != 12 || telegramCount != 2 {
		t.Fatalf("expected 12 RSS and 2 telegram sources, got %d and %d", rssCount, telegramCount)
	}
}

func TestFetchValidRSS(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, rssFixture)
	}))
	defer server.Close()

	fetcher := NewFetcher(nil)
	source := Source{Name: "test", URL: server.URL, TrustWeight: decimal.NewFromFloat(0.5)}
	articles, err := fetcher.Fetch(context.Background(), source)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(articles) != 2 {
		t.Fatalf("expected 2 articles, got %d", len(articles))
	}
	if articles[0].Title != "Сбербанк отчитался о прибыли" {
		t.Fatalf("unexpected first title %q", articles[0].Title)
	}
	if articles[1].PublishedAt.IsZero() {
		t.Fatal("expected parsed publish date")
	}
}

func TestFetchSkipsMalformedPubDateItem(t *testing.T) {
	body := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <item>
      <title>Broken date item</title>
      <link>https://example.com/broken</link>
      <pubDate>not-a-real-date</pubDate>
    </item>
    <item>
      <title>Valid date item</title>
      <link>https://example.com/valid</link>
      <pubDate>Mon, 09 Jan 2024 10:00:00 +0300</pubDate>
    </item>
  </channel>
</rss>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, body)
	}))
	defer server.Close()

	fetcher := NewFetcher(nil)
	articles, err := fetcher.Fetch(context.Background(), Source{Name: "test", URL: server.URL})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(articles) != 1 || articles[0].Title != "Valid date item" {
		t.Fatalf("articles = %+v, want only the valid-date item", articles)
	}
}

func TestFetchHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusGone)
	}))
	defer server.Close()

	fetcher := NewFetcher(nil)
	_, err := fetcher.Fetch(context.Background(), Source{URL: server.URL})
	if err == nil {
		t.Fatal("expected error for HTTP error response")
	}
	if !strings.Contains(err.Error(), "410") {
		t.Fatalf("expected 410 in error, got %v", err)
	}
}

func TestFetchMalformedXML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, `<rss><channel><item><title>broken`)
	}))
	defer server.Close()

	fetcher := NewFetcher(nil)
	_, err := fetcher.Fetch(context.Background(), Source{URL: server.URL})
	if err == nil {
		t.Fatal("expected error for malformed XML")
	}
	if !strings.Contains(err.Error(), "parse RSS") {
		t.Fatalf("expected parse RSS error, got %v", err)
	}
}

func TestFetchTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()

	fetcher := NewFetcher(&http.Client{Timeout: 20 * time.Millisecond})
	_, err := fetcher.Fetch(context.Background(), Source{URL: server.URL})
	if err == nil {
		t.Fatal("expected error for request timeout")
	}
}

func TestFetchUnsupportedType(t *testing.T) {
	fetcher := NewFetcher(nil)
	_, err := fetcher.Fetch(context.Background(), Source{Type: "telegram", Channel: "markettwits"})
	if err == nil {
		t.Fatal("expected error for unsupported source type")
	}
}

func TestMatcherByAlias(t *testing.T) {
	article := Article{
		Title:       "Сбербанк отчитался о прибыли",
		Description: "Банк увеличил прибыль.",
		Source:      Source{Name: "test", TrustWeight: decimal.NewFromFloat(0.7)},
	}
	matcher := NewMatcher(map[string][]string{"SBER": {"сбербанк", "сбер"}})
	matches := matcher.Match([]Article{article})
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Ticker != "SBER" {
		t.Fatalf("unexpected ticker %q", matches[0].Ticker)
	}
	if !matches[0].TrustWeight.Equal(decimal.NewFromFloat(0.7)) {
		t.Fatalf("unexpected trust weight %s", matches[0].TrustWeight)
	}
}

func TestMatcherAvoidsSingleLetterTickerFalsePositive(t *testing.T) {
	article := Article{
		Title:       "Технологический сектор растет",
		Description: "Рынок продолжает торговаться.",
	}
	matcher := NewMatcher(map[string][]string{"T": {"т-технологии", "тинькофф"}})
	if matches := matcher.Match([]Article{article}); len(matches) != 0 {
		t.Fatalf("expected no matches, got %+v", matches)
	}
}

func TestDefaultAliasesCoverEveryTicker(t *testing.T) {
	aliases := DefaultAliases()
	for _, ticker := range []string{"YDEX", "OZON", "SBER", "LKOH", "GAZP", "GMKN", "ROSN", "NVTK", "TATN", "MTSS", "MGNT", "PLZL", "CHMF", "DATA", "T", "SBMM", "VTBR", "RUAL", "GLDRUB_TOM", "SLVRUB_TOM", "CNYRUB_TOM"} {
		if len(aliases[ticker]) == 0 {
			t.Fatalf("ticker %q has no aliases", ticker)
		}
	}
}
