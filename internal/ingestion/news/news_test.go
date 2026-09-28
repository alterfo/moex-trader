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
	if len(sources) != 17 {
		t.Fatalf("expected 17 sources, got %d", len(sources))
	}
	rssCount := 0
	telegramCount := 0
	siteNewsCount := 0
	for _, source := range sources {
		switch source.Type {
		case "telegram":
			telegramCount++
			continue
		case SourceTypeMOEXSiteNews:
			siteNewsCount++
		default:
			rssCount++
		}
		if source.URL == "" {
			t.Fatalf("source %q has empty URL", source.Name)
		}
		if source.TrustWeight.IsZero() {
			t.Fatalf("source %q has zero trust weight", source.Name)
		}
	}
	if rssCount != 14 || telegramCount != 2 || siteNewsCount != 1 {
		t.Fatalf("expected 14 RSS, 2 telegram, 1 moex_sitenews source, got %d, %d, %d", rssCount, telegramCount, siteNewsCount)
	}
}

func TestGoogleNewsSources(t *testing.T) {
	sources := GoogleNewsSources([]string{"sber", " gazp ", "unknownticker", "gldrub_tom"})
	if len(sources) != 3 {
		t.Fatalf("expected 3 sources (unknown ticker skipped), got %d: %+v", len(sources), sources)
	}
	if sources[0].Name != "Google News: SBER" {
		t.Fatalf("unexpected name %q", sources[0].Name)
	}
	if !strings.Contains(sources[0].URL, "q=%D0%A1%D0%B1%D0%B5%D1%80%D0%B1%D0%B0%D0%BD%D0%BA+%D0%B0%D0%BA%D1%86%D0%B8%D0%B8") {
		t.Fatalf("expected URL-encoded %q query, got %q", "Сбербанк акции", sources[0].URL)
	}
	if !strings.HasPrefix(sources[1].URL, "https://news.google.com/rss/search?") {
		t.Fatalf("unexpected URL %q", sources[1].URL)
	}
	if sources[2].Name != "Google News: GLDRUB_TOM" || !strings.Contains(sources[2].URL, "%D1%86%D0%B5%D0%BD%D0%B0") {
		t.Fatalf("expected GLDRUB_TOM to use the non-equity %q suffix, got %+v", "цена", sources[2])
	}
	for _, s := range sources {
		if s.TrustWeight.IsZero() {
			t.Fatalf("source %q has zero trust weight", s.Name)
		}
	}
}

func TestAllSourcesCombinesDefaultAndGoogleNews(t *testing.T) {
	sources := AllSources([]string{"SBER", "GAZP"})
	if len(sources) != len(DefaultSources())+2 {
		t.Fatalf("expected DefaultSources()+2, got %d", len(sources))
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

func TestFetchWindows1251RSS(t *testing.T) {
	body := []byte("<?xml version=\"1.0\" encoding=\"windows-1251\"?>" +
		"<rss version=\"2.0\"><channel><item>" +
		"<title>\xcf\xf0\xe8\xe2\xe5\xf2</title>" +
		"<link>https://example.com/cp1251</link>" +
		"<description>\xd1\xee\xe1\xfb\xf2\xe8\xff</description>" +
		"<pubDate>Mon, 09 Jan 2024 10:00:00 +0300</pubDate>" +
		"</item></channel></rss>")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml; charset=windows-1251")
		_, _ = w.Write(body)
	}))
	defer server.Close()

	fetcher := NewFetcher(nil)
	articles, err := fetcher.Fetch(context.Background(), Source{Name: "finmarket", URL: server.URL})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(articles) != 1 {
		t.Fatalf("expected 1 article, got %d", len(articles))
	}
	if articles[0].Title != "Привет" {
		t.Fatalf("title = %q, want Привет", articles[0].Title)
	}
	if articles[0].Description != "События" {
		t.Fatalf("description = %q, want События", articles[0].Description)
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

func TestFetchSetsBrowserUserAgent(t *testing.T) {
	var gotUA string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, rssFixture)
	}))
	defer server.Close()

	fetcher := NewFetcher(nil)
	if _, err := fetcher.Fetch(context.Background(), Source{Name: "test", URL: server.URL}); err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if gotUA == "" || gotUA == "Go-http-client/1.1" {
		t.Fatalf("expected a browser-like User-Agent, got %q", gotUA)
	}
}

func TestFetchMOEXSiteNews(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"sitenews":{"columns":["id","tag","title","published_at","modified_at"],`+
			`"data":[[104631,"site","О начале торгов","2026-09-28 18:58:38","2026-09-28 18:58:25"],`+
			`[104630,"site","Информация &amp; уведомление","2026-09-28 18:02:35","2026-09-28 18:31:54"]]}}`)
	}))
	defer server.Close()

	fetcher := NewFetcher(nil)
	source := Source{Name: "MOEX сайт-новости", URL: server.URL, Type: SourceTypeMOEXSiteNews, TrustWeight: decimal.NewFromFloat(1.0)}
	articles, err := fetcher.Fetch(context.Background(), source)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(articles) != 2 {
		t.Fatalf("expected 2 articles, got %d", len(articles))
	}
	if articles[0].Title != "О начале торгов" {
		t.Fatalf("unexpected first title %q", articles[0].Title)
	}
	if articles[0].Link != "https://www.moex.com/n104631" {
		t.Fatalf("unexpected link %q", articles[0].Link)
	}
	if articles[1].Title != "Информация & уведомление" {
		t.Fatalf("expected HTML entity unescaped, got %q", articles[1].Title)
	}
	if articles[0].PublishedAt.IsZero() {
		t.Fatal("expected parsed publish date")
	}
}

func TestFetchMOEXSiteNewsMalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"sitenews":`)
	}))
	defer server.Close()

	fetcher := NewFetcher(nil)
	_, err := fetcher.Fetch(context.Background(), Source{URL: server.URL, Type: SourceTypeMOEXSiteNews})
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "parse moex sitenews") {
		t.Fatalf("expected parse moex sitenews error, got %v", err)
	}
}

func TestFetchMOEXSiteNewsMissingColumns(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"sitenews":{"columns":["id","title"],"data":[[1,"x"]]}}`)
	}))
	defer server.Close()

	fetcher := NewFetcher(nil)
	_, err := fetcher.Fetch(context.Background(), Source{URL: server.URL, Type: SourceTypeMOEXSiteNews})
	if err == nil {
		t.Fatal("expected error for missing published_at column")
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

func TestMatcherDoesNotMatchDataTickerOnOrdinaryWordData(t *testing.T) {
	article := Article{
		Title:       "Market data shows improving liquidity",
		Description: "Big data analytics is used by many companies.",
		Source:      Source{Name: "test", TrustWeight: decimal.NewFromFloat(0.7)},
	}
	matcher := NewMatcher(DefaultAliases())
	if matches := matcher.Match([]Article{article}); len(matches) != 0 {
		t.Fatalf("expected no DATA match for ordinary word data, got %+v", matches)
	}
}

func TestDefaultAliasesCoverEveryTicker(t *testing.T) {
	aliases := DefaultAliases()
	for _, ticker := range []string{"YDEX", "OZON", "SBER", "LKOH", "GAZP", "GMKN", "ROSN", "NVTK", "TATN", "MTSS", "MGNT", "PLZL", "CHMF", "DATA", "T", "VTBR", "RUAL", "GLDRUB_TOM", "SLVRUB_TOM", "CNYRUB_TOM", "POSI", "SNGSP", "SBERP", "NLMK", "MAGN", "AFLT", "ALRS", "SIBN", "RASP", "TRMK", "MTLR", "PHOR", "MOEX", "AFKS", "HYDR", "IRAO", "PIKK", "FEES", "ENPG", "SVCB", "SFIN", "SMLT", "VKCO", "TATNP", "BANEP"} {
		if len(aliases[ticker]) == 0 {
			t.Fatalf("ticker %q has no aliases", ticker)
		}
	}
}
