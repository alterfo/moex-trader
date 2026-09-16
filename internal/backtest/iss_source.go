package backtest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

const (
	heapDefaultLimit = 1000
	heapMaxPages     = 200
)

// HistoricalSource fetches the full candle history for a ticker within
// [from, till]. It is self-contained: only the shared moex.LookupSecurity is
// reused, candle pages are fetched with explicit ISS start/limit paging so the
// default 100-row/or no-pagination behaviour of the live client never truncates
// the history. The fetched granularity is controlled by the source's Interval
// (see NewISSSourceInterval): 24 = daily, 60/10/1 = intraday minutes.
type HistoricalSource interface {
	// History returns candles sorted by Begin in ascending order.
	History(ctx context.Context, ticker string, from, till time.Time) ([]moex.Candle, error)
}

// ISSSource reads candle history from the MOEX ISS API.
type ISSSource struct {
	client   *moex.Client
	baseURL  string
	http     *http.Client
	interval int
}

func NewISSSource(baseURL string, client *moex.Client) *ISSSource {
	return NewISSSourceInterval(baseURL, client, 24)
}

// NewISSSourceInterval returns an ISSSource fetching candles at the given ISS
// interval (24 = daily, 60/10/1 = intraday minutes). interval <= 0 defaults to
// daily so a misconfigured value degrades to the previous behaviour instead of
// returning garbage.
func NewISSSourceInterval(baseURL string, client *moex.Client, interval int) *ISSSource {
	if client == nil {
		client = moex.NewClient(baseURL, nil)
	}
	if interval <= 0 {
		interval = 24
	}
	return &ISSSource{client: client, baseURL: strings.TrimRight(baseURL, "/"), http: &http.Client{Timeout: 30 * time.Second}, interval: interval}
}

func (s *ISSSource) History(ctx context.Context, ticker string, from, till time.Time) ([]moex.Candle, error) {
	sec, err := s.client.LookupSecurity(ctx, ticker)
	if err != nil {
		return nil, fmt.Errorf("backtest: lookup %s: %w", ticker, err)
	}
	path := fmt.Sprintf("/engines/%s/markets/%s/boards/%s/securities/%s/candles.json",
		url.PathEscape(sec.Engine), url.PathEscape(sec.Market), url.PathEscape(sec.Board), url.PathEscape(sec.SecID))

	var all []moex.Candle
	start, limit := 0, heapDefaultLimit
	for page := 0; page < heapMaxPages; page++ {
		query := url.Values{}
		query.Set("interval", strconv.Itoa(s.interval))
		query.Set("start", strconv.Itoa(start))
		query.Set("limit", strconv.Itoa(limit))
		query.Set("iss.meta", "off")
		if !from.IsZero() {
			query.Set("from", from.Format("2006-01-02"))
		}
		if !till.IsZero() {
			query.Set("till", till.Format("2006-01-02"))
		}
		resp, err := s.get(ctx, path+"?"+query.Encode())
		if err != nil {
			return nil, err
		}
		if len(resp.Candles.Columns) == 0 {
			return nil, fmt.Errorf("backtest: candles for %s: no candles block", ticker)
		}
		rows := parseCandles(resp.Candles.Columns, resp.Candles.Data)
		if len(rows) == 0 {
			break
		}
		all = append(all, rows...)
		start += len(rows)
	}
	return sortCandles(all), nil
}

func (s *ISSSource) get(ctx context.Context, path string) (*issCandlesResponse, error) {
	requestURL := s.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("backtest: build request %q: %w", requestURL, err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("backtest: get %q: %w", requestURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("backtest: get %q: unexpected status %d", requestURL, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("backtest: read %q: %w", requestURL, err)
	}
	var parsed issCandlesResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("backtest: parse candles JSON from %q: %w", requestURL, err)
	}
	return &parsed, nil
}

type issCandlesResponse struct {
	Candles issCandleBlock `json:"candles"`
}

type issCandleBlock struct {
	Columns []string `json:"columns"`
	Data    [][]any  `json:"data"`
}

func parseCandles(columns []string, rows [][]any) []moex.Candle {
	out := make([]moex.Candle, 0, len(rows))
	for _, row := range rows {
		c := moex.Candle{Open: decAt(columns, row, "open"), Close: decAt(columns, row, "close"),
			High: decAt(columns, row, "high"), Low: decAt(columns, row, "low"),
			Volume: decAt(columns, row, "volume"),
			Begin:  timeAt(columns, row, "begin"), End: timeAt(columns, row, "end")}
		out = append(out, c)
	}
	return out
}

func decAt(columns []string, row []any, name string) decimal.Decimal {
	raw := cell(columns, row, name)
	if raw == "" {
		return decimal.Zero
	}
	value, err := decimal.NewFromString(raw)
	if err != nil {
		return decimal.Zero
	}
	return value
}

func timeAt(columns []string, row []any, name string) time.Time {
	raw := cell(columns, row, name)
	if raw == "" {
		return time.Time{}
	}
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339, "2006-01-02"} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func cell(columns []string, row []any, name string) string {
	idx := -1
	for i, col := range columns {
		if col == name {
			idx = i
			break
		}
	}
	if idx < 0 || idx >= len(row) {
		return ""
	}
	switch value := row[idx].(type) {
	case nil:
		return ""
	case string:
		return value
	case float64:
		if value == float64(int64(value)) {
			return strconv.FormatInt(int64(value), 10)
		}
		return strconv.FormatFloat(value, 'f', -1, 64)
	default:
		return fmt.Sprint(value)
	}
}

func sortCandles(candles []moex.Candle) []moex.Candle {
	for i := 1; i < len(candles); i++ {
		for j := i; j > 0 && candles[j].Begin.Before(candles[j-1].Begin); j-- {
			candles[j], candles[j-1] = candles[j-1], candles[j]
		}
	}
	return candles
}
