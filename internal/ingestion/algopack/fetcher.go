package algopack

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultHTTPTimeout = 10 * time.Second

type Fetcher interface {
	FetchOrderBook(ctx context.Context, ticker string) (OrderBook, error)
}

type HTTPFetcher struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewHTTPFetcher(baseURL, token string, httpClient *http.Client) *HTTPFetcher {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}
	return &HTTPFetcher{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      strings.TrimSpace(token),
		httpClient: httpClient,
	}
}

func (f *HTTPFetcher) FetchOrderBook(ctx context.Context, ticker string) (OrderBook, error) {
	var empty OrderBook
	requestURL := f.baseURL + "/algopack/eq/obstats/" + url.PathEscape(ticker) + ".json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return empty, fmt.Errorf("build algopack request %q: %w", requestURL, err)
	}
	req.Header.Set("Accept", "application/json")
	if f.token != "" {
		req.Header.Set("Authorization", "Bearer "+f.token)
	}
	resp, err := f.httpClient.Do(req)
	if err != nil {
		return empty, fmt.Errorf("get algopack %q: %w", requestURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return empty, fmt.Errorf("get algopack %q: unexpected status %d", requestURL, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return empty, fmt.Errorf("read algopack %q: %w", requestURL, err)
	}
	book, err := ParseOrderBook(body)
	if err != nil {
		return empty, fmt.Errorf("parse algopack %q: %w", requestURL, err)
	}
	if strings.TrimSpace(book.Ticker) == "" {
		book.Ticker = ticker
	}
	return book, nil
}
