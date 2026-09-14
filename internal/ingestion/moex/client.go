package moex

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
)

const defaultHTTPTimeout = 10 * time.Second

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), httpClient: httpClient}
}

type Security struct {
	SecID  string
	Board  string
	Engine string
	Market string
}

type Candle struct {
	Open   decimal.Decimal
	Close  decimal.Decimal
	High   decimal.Decimal
	Low    decimal.Decimal
	Volume decimal.Decimal
	Begin  time.Time
	End    time.Time
}

type issResponse struct {
	Description issBlock `json:"description"`
	Boards      issBlock `json:"boards"`
	Candles     issBlock `json:"candles"`
	Marketdata  issBlock `json:"marketdata"`
}

type issBlock struct {
	Columns []string `json:"columns"`
	Data    [][]any  `json:"data"`
}

func (c *Client) LookupSecurity(ctx context.Context, ticker string) (Security, error) {
	var empty Security
	resp, err := c.getJSON(ctx, "/securities/"+url.PathEscape(ticker)+".json")
	if err != nil {
		return empty, err
	}
	primary := resp.Description.firstString("primary_boardid")
	if primary == "" {
		primary = resp.Description.firstString("marketprice_boardid")
	}
	secid := resp.Description.firstString("secid")
	if secid == "" {
		secid = ticker
	}
	board, err := resp.Boards.findRow(map[string]string{"boardid": primary})
	if err != nil {
		return empty, err
	}
	engine := board.firstString("engine")
	market := board.firstString("market")
	boardID := board.firstString("boardid")
	if engine == "" || market == "" || boardID == "" {
		return empty, fmt.Errorf("moex lookup %q: missing engine/market/board in response", ticker)
	}
	return Security{SecID: secid, Board: boardID, Engine: engine, Market: market}, nil
}

func (c *Client) Candles(ctx context.Context, sec Security, interval int, from, till time.Time) ([]Candle, error) {
	path := fmt.Sprintf("/engines/%s/markets/%s/boards/%s/securities/%s/candles.json",
		url.PathEscape(sec.Engine), url.PathEscape(sec.Market), url.PathEscape(sec.Board), url.PathEscape(sec.SecID))
	if interval <= 0 {
		interval = 24
	}
	query := url.Values{}
	query.Set("interval", strconv.Itoa(interval))
	if !from.IsZero() {
		query.Set("from", from.Format("2006-01-02"))
	}
	if !till.IsZero() {
		query.Set("till", till.Format("2006-01-02"))
	}
	resp, err := c.getJSON(ctx, path+"?"+query.Encode())
	if err != nil {
		return nil, err
	}
	if len(resp.Candles.Columns) == 0 {
		return nil, fmt.Errorf("moex candles for %q: response has no candles block", sec.SecID)
	}
	candles := make([]Candle, 0, len(resp.Candles.Data))
	for _, row := range resp.Candles.Data {
		candle, err := parseCandle(resp.Candles.Columns, row)
		if err != nil {
			return nil, err
		}
		candles = append(candles, candle)
	}
	return candles, nil
}

func (c *Client) LastPrice(ctx context.Context, sec Security) (decimal.Decimal, error) {
	path := fmt.Sprintf("/engines/%s/markets/%s/securities/%s.json",
		url.PathEscape(sec.Engine), url.PathEscape(sec.Market), url.PathEscape(sec.SecID))
	resp, err := c.getJSON(ctx, path)
	if err != nil {
		return decimal.Zero, err
	}
	if len(resp.Marketdata.Data) == 0 {
		return decimal.Zero, fmt.Errorf("moex last price for %q: no marketdata rows", sec.SecID)
	}
	last := resp.Marketdata.firstString("LAST")
	if last == "" {
		return decimal.Zero, fmt.Errorf("moex last price for %q: no LAST column", sec.SecID)
	}
	price, err := decimal.NewFromString(last)
	if err != nil {
		return decimal.Zero, fmt.Errorf("parse moex last price %q: %w", last, err)
	}
	return price, nil
}

func (c *Client) getJSON(ctx context.Context, path string) (*issResponse, error) {
	requestURL := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build moex request %q: %w", requestURL, err)
	}
	req.Header.Set("Accept", "application/json")
	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get moex %q: %w", requestURL, err)
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode < 200 || httpResp.StatusCode > 299 {
		_, _ = io.Copy(io.Discard, httpResp.Body)
		return nil, fmt.Errorf("get moex %q: unexpected status %d", requestURL, httpResp.StatusCode)
	}
	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read moex %q: %w", requestURL, err)
	}
	var resp issResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse moex JSON from %q: %w", requestURL, err)
	}
	return &resp, nil
}

func parseCandle(columns []string, row []any) (Candle, error) {
	decimalAt := func(column string) (decimal.Decimal, error) {
		raw := stringifyRow(columns, row, column)
		if raw == "" {
			return decimal.Zero, nil
		}
		value, err := decimal.NewFromString(raw)
		if err != nil {
			return decimal.Zero, fmt.Errorf("parse candle %s %q: %w", column, raw, err)
		}
		return value, nil
	}
	open, err := decimalAt("open")
	if err != nil {
		return Candle{}, err
	}
	close, err := decimalAt("close")
	if err != nil {
		return Candle{}, err
	}
	high, err := decimalAt("high")
	if err != nil {
		return Candle{}, err
	}
	low, err := decimalAt("low")
	if err != nil {
		return Candle{}, err
	}
	volume, err := decimalAt("volume")
	if err != nil {
		return Candle{}, err
	}
	begin, err := parseISSDateTime(stringifyRow(columns, row, "begin"))
	if err != nil {
		return Candle{}, err
	}
	end, err := parseISSDateTime(stringifyRow(columns, row, "end"))
	if err != nil {
		return Candle{}, err
	}
	return Candle{Open: open, Close: close, High: high, Low: low, Volume: volume, Begin: begin, End: end}, nil
}

func (b issBlock) firstString(column string) string {
	index := b.columnIndex(column)
	if index < 0 || len(b.Data) == 0 {
		return ""
	}
	return stringify(b.Data[0], index)
}

func (b issBlock) columnIndex(column string) int {
	for i, name := range b.Columns {
		if name == column {
			return i
		}
	}
	return -1
}

func (b issBlock) findRow(criteria map[string]string) (issBlock, error) {
	for _, row := range b.Data {
		match := true
		for column, want := range criteria {
			if stringifyRow(b.Columns, row, column) != want {
				match = false
				break
			}
		}
		if match {
			return issBlock{Columns: b.Columns, Data: [][]any{row}}, nil
		}
	}
	return issBlock{}, fmt.Errorf("moex lookup: no row matching %v", criteria)
}

func parseISSDateTime(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	layouts := []string{"2006-01-02 15:04:05", time.RFC3339, "2006-01-02"}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("parse moex datetime %q", raw)
}

func stringifyRow(columns []string, row []any, column string) string {
	for i, name := range columns {
		if name == column {
			return stringify(row, i)
		}
	}
	return ""
}

func stringify(row []any, index int) string {
	if index < 0 || index >= len(row) {
		return ""
	}
	switch value := row[index].(type) {
	case nil:
		return ""
	case string:
		return value
	case float64:
		if value == float64(int64(value)) {
			return strconv.FormatInt(int64(value), 10)
		}
		return strconv.FormatFloat(value, 'f', -1, 64)
	case bool:
		if value {
			return "1"
		}
		return "0"
	default:
		return fmt.Sprint(value)
	}
}
