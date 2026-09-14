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
	lastPrices func(context.Context, *pb.GetLastPricesRequest) (*pb.GetLastPricesResponse, error)
	candles    func(context.Context, *pb.GetCandlesRequest) (*pb.GetCandlesResponse, error)
	orderBook  func(context.Context, *pb.GetOrderBookRequest) (*pb.GetOrderBookResponse, error)
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
				LastPrices: []*pb.LastPrice{{Price: quotationFromString("42.5"), Time: timestamppb.Now()}},
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
