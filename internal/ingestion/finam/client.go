package finam

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

const (
	DefaultBaseURL         = "https://api.finam.ru"
	tokenLifetime          = 15 * time.Minute
	refreshBeforeExpiry    = time.Minute
	defaultHTTPTimeout     = 10 * time.Second
	maxResponseBytes       = 4 << 20
	MaxClientOrderIDLength = 20
)

type Timeframe string

const (
	TimeframeUnspecified Timeframe = "TIME_FRAME_UNSPECIFIED"
	TimeframeM1          Timeframe = "TIME_FRAME_M1"
	TimeframeM5          Timeframe = "TIME_FRAME_M5"
	TimeframeM15         Timeframe = "TIME_FRAME_M15"
	TimeframeM30         Timeframe = "TIME_FRAME_M30"
	TimeframeH1          Timeframe = "TIME_FRAME_H1"
	TimeframeH2          Timeframe = "TIME_FRAME_H2"
	TimeframeH4          Timeframe = "TIME_FRAME_H4"
	TimeframeH8          Timeframe = "TIME_FRAME_H8"
	TimeframeD           Timeframe = "TIME_FRAME_D"
	TimeframeW           Timeframe = "TIME_FRAME_W"
	TimeframeMN          Timeframe = "TIME_FRAME_MN"
	TimeframeQR          Timeframe = "TIME_FRAME_QR"
)

type Side string

const (
	SideUnspecified Side = "SIDE_UNSPECIFIED"
	SideBuy         Side = "SIDE_BUY"
	SideSell        Side = "SIDE_SELL"
)

type OrderType string

const (
	OrderTypeUnspecified OrderType = "ORDER_TYPE_UNSPECIFIED"
	OrderTypeMarket      OrderType = "ORDER_TYPE_MARKET"
	OrderTypeLimit       OrderType = "ORDER_TYPE_LIMIT"
)

type TimeInForce string

const (
	TimeInForceUnspecified TimeInForce = "TIME_IN_FORCE_UNSPECIFIED"
	TimeInForceDay         TimeInForce = "TIME_IN_FORCE_DAY"
)

type OrderStatus string

const (
	OrderStatusUnspecified     OrderStatus = "ORDER_STATUS_UNSPECIFIED"
	OrderStatusNew             OrderStatus = "ORDER_STATUS_NEW"
	OrderStatusPartiallyFilled OrderStatus = "ORDER_STATUS_PARTIALLY_FILLED"
	OrderStatusFilled          OrderStatus = "ORDER_STATUS_FILLED"
	OrderStatusExecuted        OrderStatus = "ORDER_STATUS_EXECUTED"
	OrderStatusRejected        OrderStatus = "ORDER_STATUS_REJECTED"
	OrderStatusRejectedByExch  OrderStatus = "ORDER_STATUS_REJECTED_BY_EXCHANGE"
	OrderStatusDeniedByBroker  OrderStatus = "ORDER_STATUS_DENIED_BY_BROKER"
	OrderStatusCanceled        OrderStatus = "ORDER_STATUS_CANCELED"
	OrderStatusExpired         OrderStatus = "ORDER_STATUS_EXPIRED"
	OrderStatusFailed          OrderStatus = "ORDER_STATUS_FAILED"
	OrderStatusPendingNew      OrderStatus = "ORDER_STATUS_PENDING_NEW"
)

type Config struct {
	BaseURL     string
	SecretToken string
}

type Client struct {
	baseURL     string
	secretToken string
	httpClient  *http.Client
	now         func() time.Time

	mu             sync.Mutex
	token          string
	tokenExpiresAt time.Time
}

type Quote struct {
	Symbol string
	Price  decimal.Decimal
	Time   time.Time
}

type Candle struct {
	Open   decimal.Decimal
	High   decimal.Decimal
	Low    decimal.Decimal
	Close  decimal.Decimal
	Volume decimal.Decimal
	Begin  time.Time
	End    time.Time
}

type PlaceOrderRequest struct {
	Symbol        string
	Quantity      decimal.Decimal
	Side          Side
	Type          OrderType
	TimeInForce   TimeInForce
	ClientOrderID string
}

type PlaceOrderResponse struct {
	OrderID          string
	ExecID           string
	Status           OrderStatus
	ExecutedQuantity decimal.Decimal
}

type decimalValue struct {
	Value string `json:"value"`
}

type lastQuoteResponse struct {
	Symbol string     `json:"symbol"`
	Quote  finamQuote `json:"quote"`
}

type finamQuote struct {
	Symbol    string       `json:"symbol"`
	Timestamp string       `json:"timestamp"`
	Last      decimalValue `json:"last"`
}

type barsResponse struct {
	Symbol string     `json:"symbol"`
	Bars   []finamBar `json:"bars"`
}

type finamBar struct {
	Timestamp string       `json:"timestamp"`
	Open      decimalValue `json:"open"`
	High      decimalValue `json:"high"`
	Low       decimalValue `json:"low"`
	Close     decimalValue `json:"close"`
	Volume    decimalValue `json:"volume"`
}

type finamPlaceOrderRequest struct {
	Symbol        string       `json:"symbol,omitempty"`
	Quantity      decimalValue `json:"quantity,omitempty"`
	Side          Side         `json:"side,omitempty"`
	Type          OrderType    `json:"type,omitempty"`
	TimeInForce   TimeInForce  `json:"time_in_force,omitempty"`
	ClientOrderID string       `json:"client_order_id,omitempty"`
}

type finamPlaceOrderResponse struct {
	OrderID          string       `json:"order_id"`
	ExecID           string       `json:"exec_id"`
	Status           OrderStatus  `json:"status"`
	ExecutedQuantity decimalValue `json:"executed_quantity"`
}

type authRequest struct {
	Secret string `json:"secret"`
}

type authResponse struct {
	Token string `json:"token"`
}

func New(ctx context.Context, cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.SecretToken) == "" {
		return nil, errors.New("finam: secret token must not be empty")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	client := &Client{
		baseURL:     baseURL,
		secretToken: strings.TrimSpace(cfg.SecretToken),
		httpClient:  &http.Client{Timeout: defaultHTTPTimeout},
		now:         time.Now,
	}
	if err := client.authenticate(ctx); err != nil {
		return nil, err
	}
	return client, nil
}

func (c *Client) LastQuote(ctx context.Context, symbol string) (Quote, error) {
	var empty Quote
	if err := validateSymbol(symbol); err != nil {
		return empty, err
	}
	var response lastQuoteResponse
	path := "/v1/instruments/" + url.PathEscape(symbol) + "/quotes/latest"
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &response); err != nil {
		return empty, err
	}
	if response.Quote.Last.Value == "" {
		return empty, fmt.Errorf("finam: last quote for %q: missing last price", symbol)
	}
	price, err := decimal.NewFromString(response.Quote.Last.Value)
	if err != nil {
		return empty, fmt.Errorf("finam: parse last quote price %q: %w", response.Quote.Last.Value, err)
	}
	quoteTime, err := parseTimestamp(response.Quote.Timestamp)
	if err != nil {
		return empty, fmt.Errorf("finam: parse last quote timestamp %q: %w", response.Quote.Timestamp, err)
	}
	return Quote{Symbol: symbol, Price: price, Time: quoteTime}, nil
}

func (c *Client) Bars(ctx context.Context, symbol string, timeframe Timeframe, from, to time.Time) ([]Candle, error) {
	if err := validateSymbol(symbol); err != nil {
		return nil, err
	}
	if timeframe == "" || timeframe == TimeframeUnspecified {
		timeframe = TimeframeD
	}
	if !validTimeframes[timeframe] {
		return nil, fmt.Errorf("finam: unsupported timeframe %q", timeframe)
	}
	if !from.IsZero() && !to.IsZero() && !from.Before(to) {
		return nil, errors.New("finam: bars from must be before to")
	}

	query := url.Values{}
	query.Set("timeframe", string(timeframe))
	if !from.IsZero() {
		query.Set("interval.start_time", formatTimestamp(from))
	}
	if !to.IsZero() {
		query.Set("interval.end_time", formatTimestamp(to))
	}

	var response barsResponse
	path := "/v1/instruments/" + url.PathEscape(symbol) + "/bars"
	if err := c.doJSON(ctx, http.MethodGet, path, query, &response); err != nil {
		return nil, err
	}

	candles := make([]Candle, 0, len(response.Bars))
	for index, bar := range response.Bars {
		candle, err := convertBar(bar)
		if err != nil {
			return nil, fmt.Errorf("finam: convert bar %d for %q: %w", index, symbol, err)
		}
		candles = append(candles, candle)
	}
	return candles, nil
}

func (c *Client) PlaceOrder(ctx context.Context, accountID string, request PlaceOrderRequest) (PlaceOrderResponse, error) {
	var empty PlaceOrderResponse
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return empty, errors.New("finam: account id must not be empty")
	}
	if len(request.ClientOrderID) > MaxClientOrderIDLength {
		return empty, fmt.Errorf("finam: client order id must be at most %d characters, got %d", MaxClientOrderIDLength, len(request.ClientOrderID))
	}
	if err := validateSymbol(request.Symbol); err != nil {
		return empty, err
	}
	if !request.Quantity.IsPositive() {
		return empty, errors.New("finam: order quantity must be positive")
	}
	if request.Side != SideBuy && request.Side != SideSell {
		return empty, fmt.Errorf("finam: order side must be %q or %q", SideBuy, SideSell)
	}
	orderType := request.Type
	if orderType == "" || orderType == OrderTypeUnspecified {
		orderType = OrderTypeMarket
	}
	timeInForce := request.TimeInForce
	if timeInForce == "" || timeInForce == TimeInForceUnspecified {
		timeInForce = TimeInForceDay
	}

	body := finamPlaceOrderRequest{
		Symbol:        request.Symbol,
		Quantity:      decimalValue{Value: request.Quantity.String()},
		Side:          request.Side,
		Type:          orderType,
		TimeInForce:   timeInForce,
		ClientOrderID: request.ClientOrderID,
	}
	var response finamPlaceOrderResponse
	path := "/v1/accounts/" + url.PathEscape(accountID) + "/orders"
	if err := c.doJSONBody(ctx, http.MethodPost, path, nil, body, &response); err != nil {
		return empty, err
	}

	executedQuantity := decimal.Zero
	if strings.TrimSpace(response.ExecutedQuantity.Value) != "" {
		parsed, err := decimal.NewFromString(response.ExecutedQuantity.Value)
		if err != nil {
			return empty, fmt.Errorf("finam: parse executed quantity %q: %w", response.ExecutedQuantity.Value, err)
		}
		executedQuantity = parsed
	}
	return PlaceOrderResponse{
		OrderID:          response.OrderID,
		ExecID:           response.ExecID,
		Status:           response.Status,
		ExecutedQuantity: executedQuantity,
	}, nil
}

var validTimeframes = map[Timeframe]bool{
	TimeframeM1:  true,
	TimeframeM5:  true,
	TimeframeM15: true,
	TimeframeM30: true,
	TimeframeH1:  true,
	TimeframeH2:  true,
	TimeframeH4:  true,
	TimeframeH8:  true,
	TimeframeD:   true,
	TimeframeW:   true,
	TimeframeMN:  true,
	TimeframeQR:  true,
}

func (c *Client) authenticate(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.authenticateLocked(ctx)
}

func (c *Client) authenticateLocked(ctx context.Context) error {
	payload, err := json.Marshal(authRequest{Secret: c.secretToken})
	if err != nil {
		return fmt.Errorf("finam: marshal auth request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/sessions", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("finam: build auth request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("finam: auth request: %w", err)
	}
	defer response.Body.Close()
	body, err := readBody(response.Body)
	if err != nil {
		return fmt.Errorf("finam: read auth response: %w", err)
	}
	if response.StatusCode == http.StatusUnauthorized {
		return errors.New("finam: auth failed: invalid or expired secret token")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("finam: auth failed: status %s: %s", response.Status, bodySummary(body))
	}

	var result authResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("finam: decode auth response: %w", err)
	}
	token := strings.TrimSpace(result.Token)
	if token == "" {
		return errors.New("finam: auth response missing token")
	}
	c.token = token
	c.tokenExpiresAt = c.now().Add(tokenLifetime)
	return nil
}

func (c *Client) ensureToken(ctx context.Context, force bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !force && c.token != "" && c.now().Add(refreshBeforeExpiry).Before(c.tokenExpiresAt) {
		return c.token, nil
	}
	if err := c.authenticateLocked(ctx); err != nil {
		return "", err
	}
	return c.token, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, query url.Values, out any) error {
	return c.doJSONBody(ctx, method, path, query, nil, out)
}

func (c *Client) doJSONBody(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	var bodyBytes []byte
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("finam: marshal request %s %s: %w", method, path, err)
		}
		bodyBytes = encoded
	}
	for attempt := 0; attempt < 2; attempt++ {
		token, err := c.ensureToken(ctx, attempt > 0)
		if err != nil {
			return err
		}
		var request *http.Request
		if body == nil {
			request, err = http.NewRequestWithContext(ctx, method, c.baseURL+path, nil)
		} else {
			request, err = http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(bodyBytes))
		}
		if err != nil {
			return fmt.Errorf("finam: build request %s %s: %w", method, path, err)
		}
		if query != nil {
			request.URL.RawQuery = query.Encode()
		}
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Accept", "application/json")
		if body != nil {
			request.Header.Set("Content-Type", "application/json")
		}

		response, err := c.httpClient.Do(request)
		if err != nil {
			return fmt.Errorf("finam: request %s %s: %w", method, path, err)
		}
		body, readErr := readBody(response.Body)
		_ = response.Body.Close()
		if readErr != nil {
			return fmt.Errorf("finam: read response %s %s: %w", method, path, readErr)
		}

		if response.StatusCode == http.StatusUnauthorized && attempt == 0 {
			continue
		}
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			return fmt.Errorf("finam: request %s %s: status %s: %s", method, path, response.Status, bodySummary(body))
		}
		if out == nil {
			return nil
		}
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("finam: decode response %s %s: %w", method, path, err)
		}
		return nil
	}
	return fmt.Errorf("finam: request %s %s failed after token refresh", method, path)
}

func validateSymbol(symbol string) error {
	parts := strings.Split(strings.TrimSpace(symbol), "@")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("finam: symbol must be in ticker@mic form, got %q", symbol)
	}
	return nil
}

func convertBar(bar finamBar) (Candle, error) {
	var empty Candle
	open, err := decimalFromValue(bar.Open)
	if err != nil {
		return empty, fmt.Errorf("open: %w", err)
	}
	high, err := decimalFromValue(bar.High)
	if err != nil {
		return empty, fmt.Errorf("high: %w", err)
	}
	low, err := decimalFromValue(bar.Low)
	if err != nil {
		return empty, fmt.Errorf("low: %w", err)
	}
	closePrice, err := decimalFromValue(bar.Close)
	if err != nil {
		return empty, fmt.Errorf("close: %w", err)
	}
	volume, err := decimalFromValue(bar.Volume)
	if err != nil {
		return empty, fmt.Errorf("volume: %w", err)
	}
	begin, err := parseTimestamp(bar.Timestamp)
	if err != nil {
		return empty, fmt.Errorf("timestamp %q: %w", bar.Timestamp, err)
	}
	return Candle{
		Open:   open,
		High:   high,
		Low:    low,
		Close:  closePrice,
		Volume: volume,
		Begin:  begin,
	}, nil
}

func decimalFromValue(value decimalValue) (decimal.Decimal, error) {
	if value.Value == "" {
		return decimal.Zero, nil
	}
	return decimal.NewFromString(value.Value)
}

func parseTimestamp(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid RFC3339 timestamp")
}

func formatTimestamp(value time.Time) string {
	return value.UTC().Format(time.RFC3339)
}

func readBody(body io.Reader) ([]byte, error) {
	return io.ReadAll(io.LimitReader(body, maxResponseBytes))
}

func bodySummary(body []byte) string {
	const limit = 256
	if len(body) > limit {
		return string(body[:limit]) + "..."
	}
	return string(body)
}
