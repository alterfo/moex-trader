package tinkoff

import (
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	pb "github.com/tinkoff/invest-api-go-sdk/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type fakeMarketDataServer struct {
	pb.UnimplementedMarketDataServiceServer
	lastPrices    func(context.Context, *pb.GetLastPricesRequest) (*pb.GetLastPricesResponse, error)
	candles       func(context.Context, *pb.GetCandlesRequest) (*pb.GetCandlesResponse, error)
	orderBook     func(context.Context, *pb.GetOrderBookRequest) (*pb.GetOrderBookResponse, error)
	tradingStatus func(context.Context, *pb.GetTradingStatusRequest) (*pb.GetTradingStatusResponse, error)
}

func (s *fakeMarketDataServer) GetLastPrices(ctx context.Context, req *pb.GetLastPricesRequest) (*pb.GetLastPricesResponse, error) {
	if s.lastPrices != nil {
		return s.lastPrices(ctx, req)
	}
	return nil, status.Error(codes.Unimplemented, "unimplemented")
}

func (s *fakeMarketDataServer) GetCandles(ctx context.Context, req *pb.GetCandlesRequest) (*pb.GetCandlesResponse, error) {
	if s.candles != nil {
		return s.candles(ctx, req)
	}
	return nil, status.Error(codes.Unimplemented, "unimplemented")
}

func (s *fakeMarketDataServer) GetOrderBook(ctx context.Context, req *pb.GetOrderBookRequest) (*pb.GetOrderBookResponse, error) {
	if s.orderBook != nil {
		return s.orderBook(ctx, req)
	}
	return nil, status.Error(codes.Unimplemented, "unimplemented")
}

func (s *fakeMarketDataServer) GetTradingStatus(ctx context.Context, req *pb.GetTradingStatusRequest) (*pb.GetTradingStatusResponse, error) {
	if s.tradingStatus != nil {
		return s.tradingStatus(ctx, req)
	}
	return nil, status.Error(codes.Unimplemented, "unimplemented")
}

type fakeStreamServer struct {
	pb.UnimplementedMarketDataStreamServiceServer
	mu    sync.Mutex
	calls int
}

func (s *fakeStreamServer) MarketDataServerSideStream(req *pb.MarketDataServerSideStreamRequest, stream pb.MarketDataStreamService_MarketDataServerSideStreamServer) error {
	s.mu.Lock()
	s.calls++
	call := s.calls
	s.mu.Unlock()

	if call == 1 {
		if err := stream.Send(lastPriceResponse("100.5")); err != nil {
			return err
		}
		return status.Error(codes.Unavailable, "stream disconnected")
	}
	if err := stream.Send(lastPriceResponse("101.25")); err != nil {
		return err
	}
	<-stream.Context().Done()
	return nil
}

func startTestServer(t *testing.T, register func(*grpc.Server)) *grpc.ClientConn {
	t.Helper()
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	register(server)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})

	conn, err := grpc.DialContext(context.Background(), "bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial bufconn: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func quotation(units int64, nano int32) *pb.Quotation {
	return &pb.Quotation{Units: units, Nano: nano}
}

func lastPriceResponse(value string) *pb.MarketDataResponse {
	return &pb.MarketDataResponse{
		Payload: &pb.MarketDataResponse_LastPrice{
			LastPrice: &pb.LastPrice{
				InstrumentUid: "SBER",
				Price:         quotationFromString(value),
				Time:          timestamppb.New(time.Unix(1700000000, 0)),
			},
		},
	}
}

func quotationFromString(value string) *pb.Quotation {
	d := decimal.RequireFromString(value)
	units := d.IntPart()
	nano := d.Sub(decimal.NewFromInt(units)).Mul(decimal.NewFromInt(1000000000)).IntPart()
	return &pb.Quotation{Units: units, Nano: int32(nano)}
}

func requireDecimal(t *testing.T, got decimal.Decimal, want string) {
	t.Helper()
	wantDecimal, err := decimal.NewFromString(want)
	if err != nil {
		t.Fatalf("parse want %q: %v", want, err)
	}
	if !got.Equal(wantDecimal) {
		t.Fatalf("got %s, want %s", got.String(), wantDecimal.String())
	}
}

func TestNewDisabledInPaperTrading(t *testing.T) {
	client, err := New(context.Background(), Config{IsPaperTrading: true})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer client.Close()

	if _, err := client.LastPrice(context.Background(), "SBER"); err != ErrPaperTrading {
		t.Fatalf("LastPrice error = %v, want ErrPaperTrading", err)
	}
	if _, err := client.Candles(context.Background(), "SBER", pb.CandleInterval_CANDLE_INTERVAL_DAY, time.Time{}, time.Time{}); err != ErrPaperTrading {
		t.Fatalf("Candles error = %v, want ErrPaperTrading", err)
	}
	if _, err := client.OrderBook(context.Background(), "SBER", 1); err != ErrPaperTrading {
		t.Fatalf("OrderBook error = %v, want ErrPaperTrading", err)
	}
}

func TestNewRejectsMissingToken(t *testing.T) {
	if _, err := New(context.Background(), Config{IsPaperTrading: false}); err == nil {
		t.Fatal("New returned nil error for empty token")
	}
}

func TestLastPriceSuccess(t *testing.T) {
	server := &fakeMarketDataServer{
		lastPrices: func(ctx context.Context, req *pb.GetLastPricesRequest) (*pb.GetLastPricesResponse, error) {
			return &pb.GetLastPricesResponse{
				LastPrices: []*pb.LastPrice{
					{
						InstrumentUid: "SBER",
						Price:         quotationFromString("123.45"),
						Time:          timestamppb.New(time.Unix(1700000000, 0)),
					},
				},
			}, nil
		},
	}
	conn := startTestServer(t, func(s *grpc.Server) { pb.RegisterMarketDataServiceServer(s, server) })
	client := newClientFromConn(conn)

	quote, err := client.LastPrice(context.Background(), "SBER")
	if err != nil {
		t.Fatalf("LastPrice returned error: %v", err)
	}
	requireDecimal(t, quote.Price, "123.45")
	if quote.InstrumentID != "SBER" {
		t.Fatalf("InstrumentID = %q, want SBER", quote.InstrumentID)
	}
	if quote.Time.IsZero() {
		t.Fatal("quote time is zero")
	}
}

func TestLastPriceAcceptsFigiWhenUIDIsEmpty(t *testing.T) {
	server := &fakeMarketDataServer{
		lastPrices: func(ctx context.Context, req *pb.GetLastPricesRequest) (*pb.GetLastPricesResponse, error) {
			return &pb.GetLastPricesResponse{
				LastPrices: []*pb.LastPrice{
					{
						Figi:  "SBER",
						Price: quotationFromString("123.45"),
						Time:  timestamppb.New(time.Unix(1700000000, 0)),
					},
				},
			}, nil
		},
	}
	conn := startTestServer(t, func(s *grpc.Server) { pb.RegisterMarketDataServiceServer(s, server) })
	client := newClientFromConn(conn)

	quote, err := client.LastPrice(context.Background(), "SBER")
	if err != nil {
		t.Fatalf("LastPrice returned error: %v", err)
	}
	if quote.InstrumentID != "SBER" {
		t.Fatalf("InstrumentID = %q, want SBER", quote.InstrumentID)
	}
}

func TestLastPriceRejectsEmptyUIDAndMismatchedFigi(t *testing.T) {
	server := &fakeMarketDataServer{
		lastPrices: func(ctx context.Context, req *pb.GetLastPricesRequest) (*pb.GetLastPricesResponse, error) {
			return &pb.GetLastPricesResponse{
				LastPrices: []*pb.LastPrice{
					{
						Figi:  "GAZP",
						Price: quotationFromString("123.45"),
						Time:  timestamppb.New(time.Unix(1700000000, 0)),
					},
				},
			}, nil
		},
	}
	conn := startTestServer(t, func(s *grpc.Server) { pb.RegisterMarketDataServiceServer(s, server) })
	client := newClientFromConn(conn)

	if _, err := client.LastPrice(context.Background(), "SBER"); err == nil {
		t.Fatal("LastPrice returned nil error for mismatched instrument")
	}
}

func TestLastPriceAuthError(t *testing.T) {
	server := &fakeMarketDataServer{
		lastPrices: func(ctx context.Context, req *pb.GetLastPricesRequest) (*pb.GetLastPricesResponse, error) {
			return nil, status.Error(codes.Unauthenticated, "invalid token")
		},
	}
	conn := startTestServer(t, func(s *grpc.Server) { pb.RegisterMarketDataServiceServer(s, server) })
	client := newClientFromConn(conn)

	if _, err := client.LastPrice(context.Background(), "SBER"); err == nil || !strings.Contains(err.Error(), "Unauthenticated") {
		t.Fatalf("LastPrice error = %v, want Unauthenticated", err)
	}
}

func TestAuthMetadataIsSent(t *testing.T) {
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	pb.RegisterMarketDataServiceServer(server, &fakeMarketDataServer{
		lastPrices: func(ctx context.Context, req *pb.GetLastPricesRequest) (*pb.GetLastPricesResponse, error) {
			md, ok := metadata.FromIncomingContext(ctx)
			if !ok || len(md.Get("authorization")) == 0 || md.Get("authorization")[0] != "Bearer test-token" {
				return nil, status.Error(codes.Unauthenticated, "missing bearer token")
			}
			return &pb.GetLastPricesResponse{
				LastPrices: []*pb.LastPrice{{InstrumentUid: "SBER", Price: quotationFromString("42.5"), Time: timestamppb.Now()}},
			}, nil
		},
	})
	go func() { _ = server.Serve(listener) }()
	defer server.Stop()
	defer listener.Close()

	conn, err := dial(context.Background(), "bufnet", "test-token", "moex-trader",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := newClientFromConn(conn)
	if _, err := client.LastPrice(context.Background(), "SBER"); err != nil {
		t.Fatalf("LastPrice returned error: %v", err)
	}
}

func TestCandlesSuccess(t *testing.T) {
	begin := time.Unix(1699990000, 0).UTC()
	lastTrade := time.Unix(1699993600, 0).UTC()
	server := &fakeMarketDataServer{
		candles: func(ctx context.Context, req *pb.GetCandlesRequest) (*pb.GetCandlesResponse, error) {
			return &pb.GetCandlesResponse{
				Candles: []*pb.HistoricCandle{
					{
						Open:   quotation(200, 100000000),
						High:   quotation(201, 0),
						Low:    quotation(199, 500000000),
						Close:  quotation(200, 750000000),
						Volume: 1234,
						Time:   timestamppb.New(begin),
					},
				},
			}, nil
		},
	}
	conn := startTestServer(t, func(s *grpc.Server) { pb.RegisterMarketDataServiceServer(s, server) })
	client := newClientFromConn(conn)

	candles, err := client.Candles(context.Background(), "SBER", pb.CandleInterval_CANDLE_INTERVAL_DAY, begin.Add(-time.Hour), lastTrade)
	if err != nil {
		t.Fatalf("Candles returned error: %v", err)
	}
	if len(candles) != 1 {
		t.Fatalf("len(candles) = %d, want 1", len(candles))
	}
	requireDecimal(t, candles[0].Open, "200.1")
	requireDecimal(t, candles[0].High, "201")
	requireDecimal(t, candles[0].Low, "199.5")
	requireDecimal(t, candles[0].Close, "200.75")
	if !candles[0].Volume.Equal(decimal.NewFromInt(1234)) {
		t.Fatalf("volume = %s, want 1234", candles[0].Volume.String())
	}
	if !candles[0].Begin.Equal(begin) {
		t.Fatalf("begin = %v, want %v", candles[0].Begin, begin)
	}
}

func TestOrderBookSuccess(t *testing.T) {
	server := &fakeMarketDataServer{
		orderBook: func(ctx context.Context, req *pb.GetOrderBookRequest) (*pb.GetOrderBookResponse, error) {
			return &pb.GetOrderBookResponse{
				Depth:       2,
				Bids:        []*pb.Order{{Price: quotation(100, 0), Quantity: 10}, {Price: quotation(99, 900000000), Quantity: 20}},
				Asks:        []*pb.Order{{Price: quotation(101, 0), Quantity: 30}},
				OrderbookTs: timestamppb.New(time.Unix(1700000000, 0)),
			}, nil
		},
	}
	conn := startTestServer(t, func(s *grpc.Server) { pb.RegisterMarketDataServiceServer(s, server) })
	client := newClientFromConn(conn)

	book, err := client.OrderBook(context.Background(), "SBER", 2)
	if err != nil {
		t.Fatalf("OrderBook returned error: %v", err)
	}
	if book.Depth != 2 || len(book.Bids) != 2 || len(book.Asks) != 1 {
		t.Fatalf("unexpected order book: %+v", book)
	}
	requireDecimal(t, book.Bids[0].Price, "100")
	requireDecimal(t, book.Bids[1].Price, "99.9")
	requireDecimal(t, book.Asks[0].Price, "101")
	if book.Time.IsZero() {
		t.Fatal("order book time is zero")
	}
}

func TestStreamLastPricesReconnectsAfterDisconnect(t *testing.T) {
	server := &fakeStreamServer{}
	conn := startTestServer(t, func(s *grpc.Server) { pb.RegisterMarketDataStreamServiceServer(s, server) })
	client := newClientFromConn(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	quotes, errs := client.StreamLastPrices(ctx, "SBER")
	var got []decimal.Decimal
	for len(got) < 2 {
		select {
		case quote, ok := <-quotes:
			if !ok {
				t.Fatalf("quotes channel closed after %d quotes", len(got))
			}
			got = append(got, quote.Price)
		case err := <-errs:
			if err != nil && status.Code(err) != codes.Unavailable {
				t.Fatalf("unexpected stream error: %v", err)
			}
		case <-ctx.Done():
			t.Fatalf("timed out after %d quotes", len(got))
		}
	}
	requireDecimal(t, got[0], "100.5")
	requireDecimal(t, got[1], "101.25")

	server.mu.Lock()
	calls := server.calls
	server.mu.Unlock()
	if calls < 2 {
		t.Fatalf("server calls = %d, want at least 2", calls)
	}
}

type fakeInstrumentsServer struct {
	pb.UnimplementedInstrumentsServiceServer
	findInstrument func(context.Context, *pb.FindInstrumentRequest) (*pb.FindInstrumentResponse, error)
	shareBy        func(context.Context, *pb.InstrumentRequest) (*pb.ShareResponse, error)
}

func (s *fakeInstrumentsServer) FindInstrument(ctx context.Context, req *pb.FindInstrumentRequest) (*pb.FindInstrumentResponse, error) {
	if s.findInstrument != nil {
		return s.findInstrument(ctx, req)
	}
	return nil, status.Error(codes.Unimplemented, "unimplemented")
}

func (s *fakeInstrumentsServer) ShareBy(ctx context.Context, req *pb.InstrumentRequest) (*pb.ShareResponse, error) {
	if s.shareBy != nil {
		return s.shareBy(ctx, req)
	}
	return nil, status.Error(codes.Unimplemented, "unimplemented")
}

type fakeSandboxServer struct {
	pb.UnimplementedSandboxServiceServer
	accounts    func(context.Context, *pb.GetAccountsRequest) (*pb.GetAccountsResponse, error)
	openAccount func(context.Context, *pb.OpenSandboxAccountRequest) (*pb.OpenSandboxAccountResponse, error)
	payIn       func(context.Context, *pb.SandboxPayInRequest) (*pb.SandboxPayInResponse, error)
	postOrder   func(context.Context, *pb.PostOrderRequest) (*pb.PostOrderResponse, error)
	portfolio   func(context.Context, *pb.PortfolioRequest) (*pb.PortfolioResponse, error)
	orders      func(context.Context, *pb.GetOrdersRequest) (*pb.GetOrdersResponse, error)
	cancel      func(context.Context, *pb.CancelOrderRequest) (*pb.CancelOrderResponse, error)
}

func (s *fakeSandboxServer) GetSandboxAccounts(ctx context.Context, req *pb.GetAccountsRequest) (*pb.GetAccountsResponse, error) {
	if s.accounts != nil {
		return s.accounts(ctx, req)
	}
	return nil, status.Error(codes.Unimplemented, "unimplemented")
}

func (s *fakeSandboxServer) OpenSandboxAccount(ctx context.Context, req *pb.OpenSandboxAccountRequest) (*pb.OpenSandboxAccountResponse, error) {
	if s.openAccount != nil {
		return s.openAccount(ctx, req)
	}
	return nil, status.Error(codes.Unimplemented, "unimplemented")
}

func (s *fakeSandboxServer) SandboxPayIn(ctx context.Context, req *pb.SandboxPayInRequest) (*pb.SandboxPayInResponse, error) {
	if s.payIn != nil {
		return s.payIn(ctx, req)
	}
	return nil, status.Error(codes.Unimplemented, "unimplemented")
}

func (s *fakeSandboxServer) PostSandboxOrder(ctx context.Context, req *pb.PostOrderRequest) (*pb.PostOrderResponse, error) {
	if s.postOrder != nil {
		return s.postOrder(ctx, req)
	}
	return nil, status.Error(codes.Unimplemented, "unimplemented")
}

func (s *fakeSandboxServer) GetSandboxPortfolio(ctx context.Context, req *pb.PortfolioRequest) (*pb.PortfolioResponse, error) {
	if s.portfolio != nil {
		return s.portfolio(ctx, req)
	}
	return nil, status.Error(codes.Unimplemented, "unimplemented")
}

func (s *fakeSandboxServer) GetSandboxOrders(ctx context.Context, req *pb.GetOrdersRequest) (*pb.GetOrdersResponse, error) {
	if s.orders != nil {
		return s.orders(ctx, req)
	}
	return nil, status.Error(codes.Unimplemented, "unimplemented")
}

func (s *fakeSandboxServer) CancelSandboxOrder(ctx context.Context, req *pb.CancelOrderRequest) (*pb.CancelOrderResponse, error) {
	if s.cancel != nil {
		return s.cancel(ctx, req)
	}
	return nil, status.Error(codes.Unimplemented, "unimplemented")
}

func startSandboxTestServer(t *testing.T, instruments *fakeInstrumentsServer, sandbox *fakeSandboxServer) *Client {
	t.Helper()
	conn := startTestServer(t, func(s *grpc.Server) {
		pb.RegisterInstrumentsServiceServer(s, instruments)
		pb.RegisterSandboxServiceServer(s, sandbox)
	})
	return newClientFromConn(conn)
}

func TestResolveInstrumentUIDPrefersShare(t *testing.T) {
	instruments := &fakeInstrumentsServer{
		findInstrument: func(ctx context.Context, req *pb.FindInstrumentRequest) (*pb.FindInstrumentResponse, error) {
			if req.GetQuery() != "SBER" || !req.GetApiTradeAvailableFlag() {
				t.Fatalf("unexpected request: %+v", req)
			}
			return &pb.FindInstrumentResponse{
				Instruments: []*pb.InstrumentShort{
					{Ticker: "SBER", Uid: "currency-uid", InstrumentKind: pb.InstrumentType_INSTRUMENT_TYPE_CURRENCY},
					{Ticker: "sber", Uid: "share-uid", InstrumentKind: pb.InstrumentType_INSTRUMENT_TYPE_SHARE},
					{Ticker: "SBERP", Uid: "other-uid", InstrumentKind: pb.InstrumentType_INSTRUMENT_TYPE_SHARE},
				},
			}, nil
		},
	}
	client := startSandboxTestServer(t, instruments, &fakeSandboxServer{})

	uid, err := client.ResolveInstrumentUID(context.Background(), " SBER ")
	if err != nil {
		t.Fatalf("ResolveInstrumentUID returned error: %v", err)
	}
	if uid != "share-uid" {
		t.Fatalf("uid = %q, want share-uid", uid)
	}
}

func TestResolveInstrumentUIDFallsBackToExactNonShare(t *testing.T) {
	instruments := &fakeInstrumentsServer{
		findInstrument: func(context.Context, *pb.FindInstrumentRequest) (*pb.FindInstrumentResponse, error) {
			return &pb.FindInstrumentResponse{
				Instruments: []*pb.InstrumentShort{
					{Ticker: "GLDRUB_TOM", Uid: "gold-uid", InstrumentKind: pb.InstrumentType_INSTRUMENT_TYPE_CURRENCY},
				},
			}, nil
		},
	}
	client := startSandboxTestServer(t, instruments, &fakeSandboxServer{})

	uid, err := client.ResolveInstrumentUID(context.Background(), "GLDRUB_TOM")
	if err != nil {
		t.Fatalf("ResolveInstrumentUID returned error: %v", err)
	}
	if uid != "gold-uid" {
		t.Fatalf("uid = %q, want gold-uid", uid)
	}
}

func TestResolveInstrumentUIDNoMatch(t *testing.T) {
	instruments := &fakeInstrumentsServer{
		findInstrument: func(context.Context, *pb.FindInstrumentRequest) (*pb.FindInstrumentResponse, error) {
			return &pb.FindInstrumentResponse{
				Instruments: []*pb.InstrumentShort{
					{Ticker: "SBERP", Uid: "other-uid", InstrumentKind: pb.InstrumentType_INSTRUMENT_TYPE_SHARE},
					{Ticker: "SBER", Uid: "", InstrumentKind: pb.InstrumentType_INSTRUMENT_TYPE_SHARE},
				},
			}, nil
		},
	}
	client := startSandboxTestServer(t, instruments, &fakeSandboxServer{})

	if _, err := client.ResolveInstrumentUID(context.Background(), "SBER"); err == nil {
		t.Fatal("ResolveInstrumentUID returned nil error for no match")
	}
	if _, err := client.ResolveInstrumentUID(context.Background(), "  "); err == nil {
		t.Fatal("ResolveInstrumentUID returned nil error for empty ticker")
	}
}

func TestResolveLotSize(t *testing.T) {
	instruments := &fakeInstrumentsServer{
		shareBy: func(ctx context.Context, req *pb.InstrumentRequest) (*pb.ShareResponse, error) {
			if req.GetIdType() != pb.InstrumentIdType_INSTRUMENT_ID_TYPE_UID || req.GetId() != "share-uid" {
				t.Fatalf("unexpected request: %+v", req)
			}
			return &pb.ShareResponse{Instrument: &pb.Share{Lot: 10}}, nil
		},
	}
	client := startSandboxTestServer(t, instruments, &fakeSandboxServer{})

	lot, err := client.ResolveLotSize(context.Background(), "share-uid")
	if err != nil {
		t.Fatalf("ResolveLotSize returned error: %v", err)
	}
	if lot != 10 {
		t.Fatalf("lot = %d, want 10", lot)
	}
}

func TestResolveLotSizeRejectsNonPositive(t *testing.T) {
	instruments := &fakeInstrumentsServer{
		shareBy: func(context.Context, *pb.InstrumentRequest) (*pb.ShareResponse, error) {
			return &pb.ShareResponse{Instrument: &pb.Share{Lot: 0}}, nil
		},
	}
	client := startSandboxTestServer(t, instruments, &fakeSandboxServer{})

	if _, err := client.ResolveLotSize(context.Background(), "share-uid"); err == nil {
		t.Fatal("ResolveLotSize returned nil error for non-positive lot")
	}
	if _, err := client.ResolveLotSize(context.Background(), "  "); err == nil {
		t.Fatal("ResolveLotSize returned nil error for empty uid")
	}
}

func TestSandboxAccountLifecycle(t *testing.T) {
	var opened bool
	var paidIn *pb.SandboxPayInRequest
	sandbox := &fakeSandboxServer{
		accounts: func(context.Context, *pb.GetAccountsRequest) (*pb.GetAccountsResponse, error) {
			return &pb.GetAccountsResponse{
				Accounts: []*pb.Account{{Id: "acc-1"}, {Id: "acc-2"}},
			}, nil
		},
		openAccount: func(ctx context.Context, req *pb.OpenSandboxAccountRequest) (*pb.OpenSandboxAccountResponse, error) {
			opened = true
			return &pb.OpenSandboxAccountResponse{AccountId: "acc-new"}, nil
		},
		payIn: func(ctx context.Context, req *pb.SandboxPayInRequest) (*pb.SandboxPayInResponse, error) {
			paidIn = req
			return &pb.SandboxPayInResponse{
				Balance: &pb.MoneyValue{Currency: "rub", Units: 100000},
			}, nil
		},
	}
	client := startSandboxTestServer(t, &fakeInstrumentsServer{}, sandbox)
	ctx := context.Background()

	accounts, err := client.SandboxAccounts(ctx)
	if err != nil {
		t.Fatalf("SandboxAccounts returned error: %v", err)
	}
	if len(accounts) != 2 || accounts[0].GetId() != "acc-1" {
		t.Fatalf("unexpected accounts: %+v", accounts)
	}

	accountID, err := client.OpenSandboxAccount(ctx)
	if err != nil {
		t.Fatalf("OpenSandboxAccount returned error: %v", err)
	}
	if !opened || accountID != "acc-new" {
		t.Fatalf("accountID = %q, opened = %v", accountID, opened)
	}

	balance, err := client.SandboxPayIn(ctx, "acc-new", decimal.RequireFromString("100000.5"))
	if err != nil {
		t.Fatalf("SandboxPayIn returned error: %v", err)
	}
	requireDecimal(t, balance, "100000")
	if paidIn.GetAccountId() != "acc-new" {
		t.Fatalf("pay in account = %q, want acc-new", paidIn.GetAccountId())
	}
	requireDecimal(t, moneyValueToDecimalMust(t, paidIn.GetAmount()), "100000.5")
	if paidIn.GetAmount().GetCurrency() != "rub" {
		t.Fatalf("pay in currency = %q, want rub", paidIn.GetAmount().GetCurrency())
	}
}

func moneyValueToDecimalMust(t *testing.T, value *pb.MoneyValue) decimal.Decimal {
	t.Helper()
	converted, err := MoneyValueToDecimal(value)
	if err != nil {
		t.Fatalf("MoneyValueToDecimal: %v", err)
	}
	return converted
}

func TestSandboxAccountErrors(t *testing.T) {
	client := startSandboxTestServer(t, &fakeInstrumentsServer{}, &fakeSandboxServer{
		openAccount: func(context.Context, *pb.OpenSandboxAccountRequest) (*pb.OpenSandboxAccountResponse, error) {
			return &pb.OpenSandboxAccountResponse{}, nil
		},
	})
	ctx := context.Background()

	if _, err := client.OpenSandboxAccount(ctx); err == nil {
		t.Fatal("OpenSandboxAccount returned nil error for empty account id")
	}
	if _, err := client.SandboxPayIn(ctx, "acc-1", decimal.Zero); err == nil {
		t.Fatal("SandboxPayIn returned nil error for non-positive amount")
	}
	if _, err := client.SandboxPayIn(ctx, "", decimal.NewFromInt(1)); err == nil {
		t.Fatal("SandboxPayIn returned nil error for empty account id")
	}
}

func TestSandboxOrdersAndPortfolio(t *testing.T) {
	var cancelled *pb.CancelOrderRequest
	sandbox := &fakeSandboxServer{
		postOrder: func(ctx context.Context, req *pb.PostOrderRequest) (*pb.PostOrderResponse, error) {
			if req.GetAccountId() != "acc-1" || req.GetOrderId() != "order-1" {
				t.Fatalf("unexpected post order request: %+v", req)
			}
			return &pb.PostOrderResponse{
				OrderId:               "broker-order-1",
				ExecutionReportStatus: pb.OrderExecutionReportStatus_EXECUTION_REPORT_STATUS_FILL,
				LotsExecuted:          1,
			}, nil
		},
		portfolio: func(ctx context.Context, req *pb.PortfolioRequest) (*pb.PortfolioResponse, error) {
			return &pb.PortfolioResponse{
				AccountId:            req.GetAccountId(),
				TotalAmountPortfolio: &pb.MoneyValue{Currency: "rub", Units: 99000, Nano: 500000000},
			}, nil
		},
		orders: func(ctx context.Context, req *pb.GetOrdersRequest) (*pb.GetOrdersResponse, error) {
			return &pb.GetOrdersResponse{
				Orders: []*pb.OrderState{{OrderId: "order-1"}, {OrderId: "order-2"}},
			}, nil
		},
		cancel: func(ctx context.Context, req *pb.CancelOrderRequest) (*pb.CancelOrderResponse, error) {
			cancelled = req
			return &pb.CancelOrderResponse{}, nil
		},
	}
	client := startSandboxTestServer(t, &fakeInstrumentsServer{}, sandbox)
	ctx := context.Background()

	response, err := client.PostSandboxOrder(ctx, &pb.PostOrderRequest{
		AccountId:    "acc-1",
		InstrumentId: "uid-1",
		Quantity:     1,
		OrderId:      "order-1",
	})
	if err != nil {
		t.Fatalf("PostSandboxOrder returned error: %v", err)
	}
	if response.GetExecutionReportStatus() != pb.OrderExecutionReportStatus_EXECUTION_REPORT_STATUS_FILL {
		t.Fatalf("unexpected status: %s", response.GetExecutionReportStatus())
	}

	portfolio, err := client.GetSandboxPortfolio(ctx, "acc-1")
	if err != nil {
		t.Fatalf("GetSandboxPortfolio returned error: %v", err)
	}
	requireDecimal(t, moneyValueToDecimalMust(t, portfolio.GetTotalAmountPortfolio()), "99000.5")

	orders, err := client.GetSandboxOrders(ctx, "acc-1")
	if err != nil {
		t.Fatalf("GetSandboxOrders returned error: %v", err)
	}
	if len(orders) != 2 {
		t.Fatalf("len(orders) = %d, want 2", len(orders))
	}

	if err := client.CancelSandboxOrder(ctx, "acc-1", "order-2"); err != nil {
		t.Fatalf("CancelSandboxOrder returned error: %v", err)
	}
	if cancelled.GetAccountId() != "acc-1" || cancelled.GetOrderId() != "order-2" {
		t.Fatalf("unexpected cancel request: %+v", cancelled)
	}
	if err := client.CancelSandboxOrder(ctx, "acc-1", " "); err == nil {
		t.Fatal("CancelSandboxOrder returned nil error for empty order id")
	}
	if _, err := client.PostSandboxOrder(ctx, nil); err == nil {
		t.Fatal("PostSandboxOrder returned nil error for nil request")
	}
}

func TestTradingStatus(t *testing.T) {
	server := &fakeMarketDataServer{
		tradingStatus: func(ctx context.Context, req *pb.GetTradingStatusRequest) (*pb.GetTradingStatusResponse, error) {
			if req.GetInstrumentId() != "uid-1" {
				t.Fatalf("instrument id = %q, want uid-1", req.GetInstrumentId())
			}
			return &pb.GetTradingStatusResponse{
				InstrumentUid:            "uid-1",
				TradingStatus:            pb.SecurityTradingStatus_SECURITY_TRADING_STATUS_NORMAL_TRADING,
				LimitOrderAvailableFlag:  true,
				MarketOrderAvailableFlag: true,
				ApiTradeAvailableFlag:    true,
			}, nil
		},
	}
	conn := startTestServer(t, func(s *grpc.Server) { pb.RegisterMarketDataServiceServer(s, server) })
	client := newClientFromConn(conn)

	response, err := client.TradingStatus(context.Background(), "uid-1")
	if err != nil {
		t.Fatalf("TradingStatus returned error: %v", err)
	}
	if response.GetTradingStatus() != pb.SecurityTradingStatus_SECURITY_TRADING_STATUS_NORMAL_TRADING {
		t.Fatalf("trading status = %s, want normal trading", response.GetTradingStatus())
	}
	if !response.GetMarketOrderAvailableFlag() || !response.GetLimitOrderAvailableFlag() {
		t.Fatalf("unexpected availability flags: %+v", response)
	}

	if _, err := client.TradingStatus(context.Background(), " "); err == nil {
		t.Fatal("TradingStatus returned nil error for empty instrument id")
	}
}

func TestTradingStatusDisabledInPaperTrading(t *testing.T) {
	client, err := New(context.Background(), Config{IsPaperTrading: true})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer client.Close()

	if _, err := client.TradingStatus(context.Background(), "uid-1"); err != ErrPaperTrading {
		t.Fatalf("TradingStatus error = %v, want ErrPaperTrading", err)
	}
}

func TestSandboxMethodsDisabledInPaperTrading(t *testing.T) {
	client, err := New(context.Background(), Config{IsPaperTrading: true})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer client.Close()
	ctx := context.Background()

	if _, err := client.ResolveInstrumentUID(ctx, "SBER"); err != ErrPaperTrading {
		t.Fatalf("ResolveInstrumentUID error = %v, want ErrPaperTrading", err)
	}
	if _, err := client.SandboxAccounts(ctx); err != ErrPaperTrading {
		t.Fatalf("SandboxAccounts error = %v, want ErrPaperTrading", err)
	}
	if _, err := client.OpenSandboxAccount(ctx); err != ErrPaperTrading {
		t.Fatalf("OpenSandboxAccount error = %v, want ErrPaperTrading", err)
	}
	if _, err := client.SandboxPayIn(ctx, "acc-1", decimal.NewFromInt(1)); err != ErrPaperTrading {
		t.Fatalf("SandboxPayIn error = %v, want ErrPaperTrading", err)
	}
	if _, err := client.PostSandboxOrder(ctx, &pb.PostOrderRequest{AccountId: "acc-1"}); err != ErrPaperTrading {
		t.Fatalf("PostSandboxOrder error = %v, want ErrPaperTrading", err)
	}
	if _, err := client.GetSandboxPortfolio(ctx, "acc-1"); err != ErrPaperTrading {
		t.Fatalf("GetSandboxPortfolio error = %v, want ErrPaperTrading", err)
	}
	if _, err := client.GetSandboxOrders(ctx, "acc-1"); err != ErrPaperTrading {
		t.Fatalf("GetSandboxOrders error = %v, want ErrPaperTrading", err)
	}
	if err := client.CancelSandboxOrder(ctx, "acc-1", "order-1"); err != ErrPaperTrading {
		t.Fatalf("CancelSandboxOrder error = %v, want ErrPaperTrading", err)
	}
}

func TestNextStreamBackoffCapsAtMax(t *testing.T) {
	tests := []struct {
		current time.Duration
		want    time.Duration
	}{
		{current: 100 * time.Millisecond, want: 200 * time.Millisecond},
		{current: 500 * time.Millisecond, want: time.Second},
		{current: time.Second, want: time.Second},
	}
	for _, tt := range tests {
		if got := nextStreamBackoff(tt.current); got != tt.want {
			t.Fatalf("nextStreamBackoff(%s) = %s, want %s", tt.current, got, tt.want)
		}
	}
}

func TestStreamLastPricesDisabled(t *testing.T) {
	client, err := New(context.Background(), Config{IsPaperTrading: true})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer client.Close()

	quotes, errs := client.StreamLastPrices(context.Background(), "SBER")
	if _, ok := <-quotes; ok {
		t.Fatal("quotes channel should be closed for disabled client")
	}
	err, ok := <-errs
	if !ok {
		t.Fatal("errs channel closed without an error")
	}
	if err != ErrPaperTrading {
		t.Fatalf("stream error = %v, want ErrPaperTrading", err)
	}
}
