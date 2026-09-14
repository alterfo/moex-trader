package news

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

const defaultHTTPTimeout = 10 * time.Second

type Fetcher struct {
	httpClient *http.Client
}

func NewFetcher(httpClient *http.Client) *Fetcher {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}
	return &Fetcher{httpClient: httpClient}
}

type Article struct {
	Title       string
	Link        string
	Description string
	PublishedAt time.Time
	Source      Source
}

func (f *Fetcher) Fetch(ctx context.Context, source Source) ([]Article, error) {
	if source.Type != "" && source.Type != "rss" {
		return nil, fmt.Errorf("unsupported source type %q", source.Type)
	}
	if strings.TrimSpace(source.URL) == "" {
		return nil, fmt.Errorf("source %q has no URL", source.Name)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("build news request %q: %w", source.URL, err)
	}
	req.Header.Set("Accept", "application/rss+xml, application/xml, text/xml")
	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get news %q: %w", source.URL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("get news %q: unexpected status %d", source.URL, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read news %q: %w", source.URL, err)
	}
	return parseRSS(body, source)
}

func parseRSS(data []byte, source Source) ([]Article, error) {
	var feed rssFeed
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("parse RSS %q: %w", source.URL, err)
	}
	articles := make([]Article, 0, len(feed.Channel.Items))
	for _, item := range feed.Channel.Items {
		publishedAt, err := parseRSSDate(item.PubDate)
		if err != nil {
			return nil, fmt.Errorf("parse RSS date %q from %q: %w", item.PubDate, source.URL, err)
		}
		articles = append(articles, Article{
			Title:       strings.TrimSpace(item.Title),
			Link:        strings.TrimSpace(item.Link),
			Description: strings.TrimSpace(item.Description),
			PublishedAt: publishedAt,
			Source:      source,
		})
	}
	return articles, nil
}

func parseRSSDate(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	layouts := []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC822Z,
		time.RFC822,
		time.RFC3339,
		"Mon, 2 Jan 2006 15:04:05 -0700",
		"Mon, 02 Jan 2006 15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported RSS date format %q", raw)
}

type rssFeed struct {
	Channel struct {
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
}

type MatchedArticle struct {
	Ticker      string
	Title       string
	Link        string
	Description string
	PublishedAt time.Time
	SourceName  string
	TrustWeight decimal.Decimal
}

type Matcher struct {
	aliases map[string][]string
}

func NewMatcher(aliases map[string][]string) *Matcher {
	normalized := make(map[string][]string, len(aliases))
	for ticker, aliasList := range aliases {
		all := make([]string, 0, len(aliasList)+1)
		if len(ticker) >= 2 {
			all = append(all, ticker)
		}
		for _, alias := range aliasList {
			alias = strings.TrimSpace(alias)
			if alias != "" && !containsFold(all, alias) {
				all = append(all, alias)
			}
		}
		normalized[ticker] = all
	}
	return &Matcher{aliases: normalized}
}

func (m *Matcher) Match(articles []Article) []MatchedArticle {
	matches := make([]MatchedArticle, 0)
	for _, article := range articles {
		haystack := strings.ToLower(article.Title + " " + article.Description)
		for ticker, aliasList := range m.aliases {
			for _, alias := range aliasList {
				if strings.Contains(haystack, strings.ToLower(alias)) {
					matches = append(matches, MatchedArticle{
						Ticker:      ticker,
						Title:       article.Title,
						Link:        article.Link,
						Description: article.Description,
						PublishedAt: article.PublishedAt,
						SourceName:  article.Source.Name,
						TrustWeight: article.Source.TrustWeight,
					})
					break
				}
			}
		}
	}
	return matches
}

func containsFold(items []string, candidate string) bool {
	for _, item := range items {
		if strings.EqualFold(item, candidate) {
			return true
		}
	}
	return false
}
