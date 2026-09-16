package tinkoff

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	pb "github.com/tinkoff/invest-api-go-sdk/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var ErrPaperTrading = errors.New("tinkoff market data is disabled while paper trading")

const (
	defaultEndpoint    = "invest-public-api.tinkoff.ru:443"
	defaultAppName     = "moex-trader"
	streamBaseBackoff  = 100 * time.Millisecond
	streamMaxBackoff   = time.Second
	quotationPrecision = 1000000000
)

type Config struct {
	Endpoint       string
	Token          string
	AppName        string
	IsPaperTrading bool
}

type Client struct {
	conn        *grpc.ClientConn
	md          pb.MarketDataServiceClient
	stream      pb.MarketDataStreamServiceClient
	instruments pb.InstrumentsServiceClient
	sandbox     pb.SandboxServiceClient
	enabled     bool
}

type Quote struct {
	InstrumentID string
	Price        decimal.Decimal
	Time         time.Time
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

type OrderBookLevel struct {
	Price    decimal.Decimal
	Quantity int64
}

type OrderBook struct {
	InstrumentID string
	Depth        int32
	IsConsistent bool
	Bids         []OrderBookLevel
	Asks         []OrderBookLevel
	Time         time.Time
}

func New(ctx context.Context, cfg Config) (*Client, error) {
	if cfg.IsPaperTrading {
		return &Client{enabled: false}, nil
	}
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, errors.New("tinkoff: token must not be empty")
	}
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	appName := strings.TrimSpace(cfg.AppName)
	if appName == "" {
		appName = defaultAppName
	}

	conn, err := dial(ctx, endpoint, cfg.Token, appName, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})))
	if err != nil {
		return nil, fmt.Errorf("tinkoff dial %q: %w", endpoint, err)
	}
	return newClientFromConn(conn), nil
}

func dial(ctx context.Context, endpoint, token, appName string, baseOpts ...grpc.DialOption) (*grpc.ClientConn, error) {
	opts := make([]grpc.DialOption, 0, len(baseOpts)+2)
	opts = append(opts, baseOpts...)
	opts = append(opts,
		grpc.WithUnaryInterceptor(authUnaryInterceptor(token, appName)),
		grpc.WithStreamInterceptor(authStreamInterceptor(token, appName)),
	)
	return grpc.DialContext(ctx, endpoint, opts...)
}

func newClientFromConn(conn *grpc.ClientConn) *Client {
	return &Client{
		conn:        conn,
		md:          pb.NewMarketDataServiceClient(conn),
		stream:      pb.NewMarketDataStreamServiceClient(conn),
		instruments: pb.NewInstrumentsServiceClient(conn),
		sandbox:     pb.NewSandboxServiceClient(conn),
		enabled:     true,
	}
}

func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *Client) Candles(ctx context.Context, instrumentID string, interval pb.CandleInterval, from, to time.Time) ([]Candle, error) {
	if !c.enabled {
		return nil, ErrPaperTrading
	}
	if err := validateInstrumentID(instrumentID); err != nil {
		return nil, err
	}
	if interval == pb.CandleInterval_CANDLE_INTERVAL_UNSPECIFIED {
		interval = pb.CandleInterval_CANDLE_INTERVAL_DAY
	}
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		return nil, errors.New("tinkoff: candles from must not be after to")
	}

	request := &pb.GetCandlesRequest{
		InstrumentId: instrumentID,
		Interval:     interval,
	}
	if !from.IsZero() {
		request.From = timestamppb.New(from)
	}
	if !to.IsZero() {
		request.To = timestamppb.New(to)
	}

	response, err := c.md.GetCandles(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("tinkoff get candles %q: %w", instrumentID, err)
	}

	candles := make([]Candle, 0, len(response.GetCandles()))
	for _, historic := range response.GetCandles() {
		candle, err := convertCandle(historic)
		if err != nil {
			return nil, fmt.Errorf("tinkoff convert candle for %q: %w", instrumentID, err)
		}
		candles = append(candles, candle)
	}
	return candles, nil
}

func (c *Client) LastPrice(ctx context.Context, instrumentID string) (Quote, error) {
	if !c.enabled {
		return Quote{}, ErrPaperTrading
	}
	if err := validateInstrumentID(instrumentID); err != nil {
		return Quote{}, err
	}

	response, err := c.md.GetLastPrices(ctx, &pb.GetLastPricesRequest{InstrumentId: []string{instrumentID}})
	if err != nil {
		return Quote{}, fmt.Errorf("tinkoff get last price %q: %w", instrumentID, err)
	}
	for _, lastPrice := range response.GetLastPrices() {
		if lastPrice == nil {
			continue
		}
		if !lastPriceMatchesInstrument(lastPrice, instrumentID) {
			continue
		}
		quote, err := convertLastPrice(lastPrice)
		if err != nil {
			return Quote{}, fmt.Errorf("tinkoff convert last price %q: %w", instrumentID, err)
		}
		quote.InstrumentID = instrumentID
		return quote, nil
	}
	return Quote{}, fmt.Errorf("tinkoff get last price %q: no data", instrumentID)
}

func lastPriceMatchesInstrument(lastPrice *pb.LastPrice, instrumentID string) bool {
	uid := lastPrice.GetInstrumentUid()
	figi := lastPrice.GetFigi()
	return (uid != "" && uid == instrumentID) || (figi != "" && figi == instrumentID)
}

func (c *Client) OrderBook(ctx context.Context, instrumentID string, depth int32) (OrderBook, error) {
	if !c.enabled {
		return OrderBook{}, ErrPaperTrading
	}
	if err := validateInstrumentID(instrumentID); err != nil {
		return OrderBook{}, err
	}
	if depth <= 0 {
		depth = 1
	}

	response, err := c.md.GetOrderBook(ctx, &pb.GetOrderBookRequest{InstrumentId: instrumentID, Depth: depth})
	if err != nil {
		return OrderBook{}, fmt.Errorf("tinkoff get order book %q: %w", instrumentID, err)
	}

	bids, err := convertOrderLevels(response.GetBids())
	if err != nil {
		return OrderBook{}, fmt.Errorf("tinkoff convert order book bids %q: %w", instrumentID, err)
	}
	asks, err := convertOrderLevels(response.GetAsks())
	if err != nil {
		return OrderBook{}, fmt.Errorf("tinkoff convert order book asks %q: %w", instrumentID, err)
	}

	return OrderBook{
		InstrumentID: instrumentID,
		Depth:        response.GetDepth(),
		Bids:         bids,
		Asks:         asks,
		Time:         timestampTime(response.GetOrderbookTs()),
	}, nil
}

func (c *Client) TradingStatus(ctx context.Context, instrumentID string) (*pb.GetTradingStatusResponse, error) {
	if !c.enabled {
		return nil, ErrPaperTrading
	}
	if err := validateInstrumentID(instrumentID); err != nil {
		return nil, err
	}
	response, err := c.md.GetTradingStatus(ctx, &pb.GetTradingStatusRequest{InstrumentId: instrumentID})
	if err != nil {
		return nil, fmt.Errorf("tinkoff get trading status %q: %w", instrumentID, err)
	}
	if response == nil {
		return nil, errors.New("tinkoff get trading status: nil response")
	}
	return response, nil
}

func (c *Client) ResolveInstrumentUID(ctx context.Context, ticker string) (string, error) {
	if !c.enabled {
		return "", ErrPaperTrading
	}
	ticker = strings.TrimSpace(ticker)
	if ticker == "" {
		return "", errors.New("tinkoff: ticker must not be empty")
	}

	response, err := c.instruments.FindInstrument(ctx, &pb.FindInstrumentRequest{
		Query:                 ticker,
		ApiTradeAvailableFlag: true,
	})
	if err != nil {
		return "", fmt.Errorf("tinkoff find instrument %q: %w", ticker, err)
	}

	var fallback *pb.InstrumentShort
	for _, instrument := range response.GetInstruments() {
		if instrument == nil || strings.TrimSpace(instrument.GetUid()) == "" {
			continue
		}
		if !strings.EqualFold(instrument.GetTicker(), ticker) {
			continue
		}
		if instrument.GetInstrumentKind() == pb.InstrumentType_INSTRUMENT_TYPE_SHARE {
			return instrument.GetUid(), nil
		}
		if fallback == nil {
			fallback = instrument
		}
	}
	if fallback != nil {
		return fallback.GetUid(), nil
	}
	return "", fmt.Errorf("tinkoff find instrument %q: no match", ticker)
}

func (c *Client) SandboxAccounts(ctx context.Context) ([]*pb.Account, error) {
	if !c.enabled {
		return nil, ErrPaperTrading
	}
	response, err := c.sandbox.GetSandboxAccounts(ctx, &pb.GetAccountsRequest{})
	if err != nil {
		return nil, fmt.Errorf("tinkoff get sandbox accounts: %w", err)
	}
	return response.GetAccounts(), nil
}

func (c *Client) OpenSandboxAccount(ctx context.Context) (string, error) {
	if !c.enabled {
		return "", ErrPaperTrading
	}
	response, err := c.sandbox.OpenSandboxAccount(ctx, &pb.OpenSandboxAccountRequest{})
	if err != nil {
		return "", fmt.Errorf("tinkoff open sandbox account: %w", err)
	}
	accountID := strings.TrimSpace(response.GetAccountId())
	if accountID == "" {
		return "", errors.New("tinkoff open sandbox account: empty account id")
	}
	return accountID, nil
}

func (c *Client) SandboxPayIn(ctx context.Context, accountID string, amount decimal.Decimal) (decimal.Decimal, error) {
	if !c.enabled {
		return decimal.Zero, ErrPaperTrading
	}
	if err := validateAccountID(accountID); err != nil {
		return decimal.Zero, err
	}
	if !amount.IsPositive() {
		return decimal.Zero, errors.New("tinkoff: pay in amount must be positive")
	}
	response, err := c.sandbox.SandboxPayIn(ctx, &pb.SandboxPayInRequest{
		AccountId: accountID,
		Amount:    decimalToMoneyValue(amount, "rub"),
	})
	if err != nil {
		return decimal.Zero, fmt.Errorf("tinkoff sandbox pay in %q: %w", accountID, err)
	}
	balance, err := MoneyValueToDecimal(response.GetBalance())
	if err != nil {
		return decimal.Zero, fmt.Errorf("tinkoff sandbox pay in %q: %w", accountID, err)
	}
	return balance, nil
}

func (c *Client) PostSandboxOrder(ctx context.Context, request *pb.PostOrderRequest) (*pb.PostOrderResponse, error) {
	if !c.enabled {
		return nil, ErrPaperTrading
	}
	if request == nil {
		return nil, errors.New("tinkoff: post sandbox order request is nil")
	}
	if err := validateAccountID(request.GetAccountId()); err != nil {
		return nil, err
	}
	response, err := c.sandbox.PostSandboxOrder(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("tinkoff post sandbox order: %w", err)
	}
	if response == nil {
		return nil, errors.New("tinkoff post sandbox order: nil response")
	}
	return response, nil
}

func (c *Client) GetSandboxPortfolio(ctx context.Context, accountID string) (*pb.PortfolioResponse, error) {
	if !c.enabled {
		return nil, ErrPaperTrading
	}
	if err := validateAccountID(accountID); err != nil {
		return nil, err
	}
	response, err := c.sandbox.GetSandboxPortfolio(ctx, &pb.PortfolioRequest{AccountId: accountID})
	if err != nil {
		return nil, fmt.Errorf("tinkoff get sandbox portfolio %q: %w", accountID, err)
	}
	if response == nil {
		return nil, errors.New("tinkoff get sandbox portfolio: nil response")
	}
	return response, nil
}

func (c *Client) GetSandboxOrders(ctx context.Context, accountID string) ([]*pb.OrderState, error) {
	if !c.enabled {
		return nil, ErrPaperTrading
	}
	if err := validateAccountID(accountID); err != nil {
		return nil, err
	}
	response, err := c.sandbox.GetSandboxOrders(ctx, &pb.GetOrdersRequest{AccountId: accountID})
	if err != nil {
		return nil, fmt.Errorf("tinkoff get sandbox orders %q: %w", accountID, err)
	}
	return response.GetOrders(), nil
}

func (c *Client) CancelSandboxOrder(ctx context.Context, accountID, orderID string) error {
	if !c.enabled {
		return ErrPaperTrading
	}
	if err := validateAccountID(accountID); err != nil {
		return err
	}
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return errors.New("tinkoff: order id must not be empty")
	}
	if _, err := c.sandbox.CancelSandboxOrder(ctx, &pb.CancelOrderRequest{AccountId: accountID, OrderId: orderID}); err != nil {
		return fmt.Errorf("tinkoff cancel sandbox order %q: %w", orderID, err)
	}
	return nil
}

func (c *Client) StreamLastPrices(ctx context.Context, instrumentID string) (<-chan Quote, <-chan error) {
	quotes := make(chan Quote)
	errs := make(chan error, 1)

	if !c.enabled {
		errs <- ErrPaperTrading
		close(quotes)
		close(errs)
		return quotes, errs
	}
	if err := validateInstrumentID(instrumentID); err != nil {
		errs <- err
		close(quotes)
		close(errs)
		return quotes, errs
	}

	go c.runLastPriceStream(ctx, instrumentID, quotes, errs)
	return quotes, errs
}

func (c *Client) runLastPriceStream(ctx context.Context, instrumentID string, quotes chan<- Quote, errs chan<- error) {
	defer close(quotes)
	defer close(errs)

	backoff := streamBaseBackoff
	for {
		if ctx.Err() != nil {
			return
		}
		err := c.consumeLastPriceStream(ctx, instrumentID, quotes)
		if err == nil || ctx.Err() != nil {
			return
		}
		select {
		case errs <- err:
		default:
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = nextStreamBackoff(backoff)
	}
}

func nextStreamBackoff(current time.Duration) time.Duration {
	next := current * 2
	if next > streamMaxBackoff {
		return streamMaxBackoff
	}
	return next
}

func (c *Client) consumeLastPriceStream(ctx context.Context, instrumentID string, quotes chan<- Quote) error {
	stream, err := c.stream.MarketDataServerSideStream(ctx, &pb.MarketDataServerSideStreamRequest{
		SubscribeLastPriceRequest: &pb.SubscribeLastPriceRequest{
			SubscriptionAction: pb.SubscriptionAction_SUBSCRIPTION_ACTION_SUBSCRIBE,
			Instruments: []*pb.LastPriceInstrument{
				{InstrumentId: instrumentID},
			},
		},
	})
	if err != nil {
		return err
	}

	for {
		response, err := stream.Recv()
		if err != nil {
			return err
		}
		if lastPrice := response.GetLastPrice(); lastPrice != nil {
			quote, err := convertLastPrice(lastPrice)
			if err != nil {
				return err
			}
			quote.InstrumentID = instrumentID
			select {
			case quotes <- quote:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

func validateInstrumentID(instrumentID string) error {
	if strings.TrimSpace(instrumentID) == "" {
		return errors.New("tinkoff: instrument id must not be empty")
	}
	return nil
}

func validateAccountID(accountID string) error {
	if strings.TrimSpace(accountID) == "" {
		return errors.New("tinkoff: account id must not be empty")
	}
	return nil
}

func decimalToMoneyValue(value decimal.Decimal, currency string) *pb.MoneyValue {
	units := value.IntPart()
	remainder := value.Sub(decimal.NewFromInt(units))
	nano := remainder.Mul(decimal.NewFromInt(quotationPrecision)).Round(0).IntPart()
	return &pb.MoneyValue{
		Currency: currency,
		Units:    units,
		Nano:     int32(nano),
	}
}

func MoneyValueToDecimal(value *pb.MoneyValue) (decimal.Decimal, error) {
	if value == nil {
		return decimal.Zero, errors.New("tinkoff: nil money value")
	}
	whole := decimal.NewFromInt(value.GetUnits())
	fraction := decimal.NewFromInt(int64(value.GetNano())).Div(decimal.NewFromInt(quotationPrecision))
	return whole.Add(fraction), nil
}

func convertCandle(candle *pb.HistoricCandle) (Candle, error) {
	var empty Candle
	if candle == nil {
		return empty, errors.New("tinkoff: nil historic candle")
	}
	open, err := quotationToDecimal(candle.GetOpen())
	if err != nil {
		return empty, err
	}
	high, err := quotationToDecimal(candle.GetHigh())
	if err != nil {
		return empty, err
	}
	low, err := quotationToDecimal(candle.GetLow())
	if err != nil {
		return empty, err
	}
	closePrice, err := quotationToDecimal(candle.GetClose())
	if err != nil {
		return empty, err
	}
	return Candle{
		Open:   open,
		High:   high,
		Low:    low,
		Close:  closePrice,
		Volume: decimal.NewFromInt(candle.GetVolume()),
		Begin:  timestampTime(candle.GetTime()),
		End:    time.Time{},
	}, nil
}

func convertLastPrice(lastPrice *pb.LastPrice) (Quote, error) {
	var empty Quote
	if lastPrice == nil {
		return empty, errors.New("tinkoff: nil last price")
	}
	price, err := quotationToDecimal(lastPrice.GetPrice())
	if err != nil {
		return empty, err
	}
	return Quote{
		InstrumentID: lastPrice.GetInstrumentUid(),
		Price:        price,
		Time:         timestampTime(lastPrice.GetTime()),
	}, nil
}

func convertOrderLevels(levels []*pb.Order) ([]OrderBookLevel, error) {
	result := make([]OrderBookLevel, 0, len(levels))
	for _, level := range levels {
		if level == nil {
			continue
		}
		price, err := quotationToDecimal(level.GetPrice())
		if err != nil {
			return nil, err
		}
		result = append(result, OrderBookLevel{
			Price:    price,
			Quantity: level.GetQuantity(),
		})
	}
	return result, nil
}

func quotationToDecimal(quotation *pb.Quotation) (decimal.Decimal, error) {
	if quotation == nil {
		return decimal.Zero, errors.New("tinkoff: nil quotation")
	}
	whole := decimal.NewFromInt(quotation.GetUnits())
	fraction := decimal.NewFromInt(int64(quotation.GetNano())).Div(decimal.NewFromInt(quotationPrecision))
	return whole.Add(fraction), nil
}

func authUnaryInterceptor(token, appName string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		return invoker(withAuthMetadata(ctx, token, appName), method, req, reply, cc, opts...)
	}
}

func authStreamInterceptor(token, appName string) grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		return streamer(withAuthMetadata(ctx, token, appName), desc, cc, method, opts...)
	}
}

func withAuthMetadata(ctx context.Context, token, appName string) context.Context {
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
	if appName != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "x-app-name", appName)
	}
	return ctx
}

func timestampTime(value *timestamppb.Timestamp) time.Time {
	if value == nil {
		return time.Time{}
	}
	return value.AsTime()
}
