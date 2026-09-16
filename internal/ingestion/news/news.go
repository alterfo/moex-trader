package news

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
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
	decoder := xml.NewDecoder(bytes.NewReader(data))
	decoder.CharsetReader = charset.NewReaderLabel
	if err := decoder.Decode(&feed); err != nil {
		return nil, fmt.Errorf("parse RSS %q: %w", source.URL, err)
	}
	articles := make([]Article, 0, len(feed.Channel.Items))
	for _, item := range feed.Channel.Items {
		publishedAt, err := parseRSSDate(item.PubDate)
		if err != nil {
			continue
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
		all := make([]string, 0, len(aliasList))
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

// FetchTelegram fetches a channel's recent posts from its public web preview
// (https://t.me/s/{channel}), paginating backwards until posts older than
// since are reached or maxPages pages have been fetched. Pass maxPages <= 0
// for the backward-pagination retro default (bounded by client timeout).
func (f *Fetcher) FetchTelegram(ctx context.Context, source Source, since time.Time, maxPages int) ([]Article, error) {
	if strings.TrimSpace(source.Channel) == "" {
		return nil, fmt.Errorf("telegram source %q has no channel", source.Name)
	}
	if maxPages <= 0 {
		maxPages = 2000
	}
	var articles []Article
	before := ""
	for page := 0; page < maxPages; page++ {
		reqURL := "https://t.me/s/" + source.Channel
		if before != "" {
			reqURL += "?before=" + before
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return nil, fmt.Errorf("build telegram request %q: %w", reqURL, err)
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; moex-trader/1.0)")
		resp, err := f.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("get telegram channel %q: %w", source.Channel, err)
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return nil, fmt.Errorf("get telegram channel %q: unexpected status %d", source.Channel, resp.StatusCode)
		}
		if readErr != nil {
			return nil, fmt.Errorf("read telegram channel %q: %w", source.Channel, readErr)
		}
		posts, err := parseTelegramPage(body)
		if err != nil {
			return nil, fmt.Errorf("parse telegram channel %q page %d: %w", source.Channel, page, err)
		}
		if len(posts) == 0 {
			break
		}
		stale := false
		for _, p := range posts {
			if !since.IsZero() && p.publishedAt.Before(since) {
				stale = true
				break
			}
			articles = append(articles, Article{
				Title:       p.text,
				Link:        "https://t.me/" + p.postID,
				PublishedAt: p.publishedAt,
				Source:      source,
			})
		}
		if stale || len(posts) == 0 {
			break
		}
		nextID := posts[0].postID
		if i := strings.LastIndexByte(nextID, '/'); i >= 0 {
			nextID = nextID[i+1:]
		}
		if nextID == before {
			break
		}
		before = nextID
	}
	return articles, nil
}

type telegramPost struct {
	postID      string
	publishedAt time.Time
	text        string
}

func parseTelegramPage(data []byte) ([]telegramPost, error) {
	htmlDoc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}
	var posts []telegramPost
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "div" {
			postID := attr(n, "data-post")
			if strings.Contains(postID, "/") {
				var p telegramPost
				p.postID = postID
				extractPost(n, &p)
				if !p.publishedAt.IsZero() || p.text != "" {
					posts = append(posts, p)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(htmlDoc)
	return posts, nil
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func extractPost(n *html.Node, p *telegramPost) {
	if n.Type == html.ElementNode && n.Data == "time" {
		raw := attr(n, "datetime")
		if ts, err := time.Parse(time.RFC3339, raw); err == nil && p.publishedAt.IsZero() {
			p.publishedAt = ts
		}
	}
	if n.Type == html.ElementNode && n.Data == "div" && strings.Contains(attr(n, "class"), "tgme_widget_message_text") {
		p.text = strings.Join(strings.Fields(nodeText(n)), " ")
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		extractPost(c, p)
	}
}

func nodeText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			sb.WriteString(c.Data)
		} else if c.Type == html.ElementNode {
			sb.WriteString(" ")
			sb.WriteString(nodeText(c))
			sb.WriteString(" ")
		}
	}
	return sb.String()
}
