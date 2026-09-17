package tinkoff

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	pb "github.com/tinkoff/invest-api-go-sdk/proto"

	"github.com/olegsidorkin/moex-trader/internal/risk"
)

type fakeClient struct {
	resolveUID    func(ctx context.Context, ticker string) (string, error)
	accounts      func(ctx context.Context) ([]*pb.Account, error)
	open          func(ctx context.Context) (string, error)
	payIn         func(ctx context.Context, accountID string, amount decimal.Decimal) (decimal.Decimal, error)
	postOrder     func(ctx context.Context, request *pb.PostOrderRequest) (*pb.PostOrderResponse, error)
	portfolio     func(ctx context.Context, accountID string) (*pb.PortfolioResponse, error)
	orders        func(ctx context.Context, accountID string) ([]*pb.OrderState, error)
	operations    func(ctx context.Context, accountID string, from, to time.Time) ([]*pb.Operation, error)
	cancel        func(ctx context.Context, accountID, orderID string) error
	tradingStatus func(ctx context.Context, instrumentID string) (*pb.GetTradingStatusResponse, error)
	lotSize       func(ctx context.Context, instrumentUID string) (int32, error)

	closed bool
}

func (f *fakeClient) ResolveInstrumentUID(ctx context.Context, ticker string) (string, error) {
	if f.resolveUID != nil {
		return f.resolveUID(ctx, ticker)
	}
	return "", errors.New("unexpected ResolveInstrumentUID call")
}

func (f *fakeClient) SandboxAccounts(ctx context.Context) ([]*pb.Account, error) {
	if f.accounts != nil {
		return f.accounts(ctx)
	}
	return nil, errors.New("unexpected SandboxAccounts call")
}

func (f *fakeClient) OpenSandboxAccount(ctx context.Context) (string, error) {
	if f.open != nil {
		return f.open(ctx)
	}
	return "", errors.New("unexpected OpenSandboxAccount call")
}

func (f *fakeClient) SandboxPayIn(ctx context.Context, accountID string, amount decimal.Decimal) (decimal.Decimal, error) {
	if f.payIn != nil {
		return f.payIn(ctx, accountID, amount)
	}
	return decimal.Zero, errors.New("unexpected SandboxPayIn call")
}

func (f *fakeClient) PostSandboxOrder(ctx context.Context, request *pb.PostOrderRequest) (*pb.PostOrderResponse, error) {
	if f.postOrder != nil {
		return f.postOrder(ctx, request)
	}
	return nil, errors.New("unexpected PostSandboxOrder call")
}

func (f *fakeClient) GetSandboxPortfolio(ctx context.Context, accountID string) (*pb.PortfolioResponse, error) {
	if f.portfolio != nil {
		return f.portfolio(ctx, accountID)
	}
	return nil, errors.New("unexpected GetSandboxPortfolio call")
}

func (f *fakeClient) GetSandboxOrders(ctx context.Context, accountID string) ([]*pb.OrderState, error) {
	if f.orders != nil {
		return f.orders(ctx, accountID)
	}
	return nil, errors.New("unexpected GetSandboxOrders call")
}

func (f *fakeClient) SandboxOperations(ctx context.Context, accountID string, from, to time.Time) ([]*pb.Operation, error) {
	if f.operations != nil {
		return f.operations(ctx, accountID, from, to)
	}
	return nil, errors.New("unexpected SandboxOperations call")
}

func (f *fakeClient) CancelSandboxOrder(ctx context.Context, accountID, orderID string) error {
	if f.cancel != nil {
		return f.cancel(ctx, accountID, orderID)
	}
	return errors.New("unexpected CancelSandboxOrder call")
}

func (f *fakeClient) TradingStatus(ctx context.Context, instrumentID string) (*pb.GetTradingStatusResponse, error) {
	if f.tradingStatus != nil {
		return f.tradingStatus(ctx, instrumentID)
	}
	return nil, errors.New("unexpected TradingStatus call")
}

func (f *fakeClient) ResolveLotSize(ctx context.Context, instrumentUID string) (int32, error) {
	if f.lotSize != nil {
		return f.lotSize(ctx, instrumentUID)
	}
	return 0, errors.New("unexpected ResolveLotSize call")
}

func (f *fakeClient) Close() error {
	f.closed = true
	return nil
}

func money(units int64, nano int32) *pb.MoneyValue {
	return &pb.MoneyValue{Currency: "rub", Units: units, Nano: nano}
}

func quotation(units int64, nano int32) *pb.Quotation {
	return &pb.Quotation{Units: units, Nano: nano}
}

func newSandboxForTest(t *testing.T, client Client, cfg Config) *Sandbox {
	t.Helper()
	if cfg.PayIn.IsZero() {
		cfg.PayIn = decimal.NewFromInt(100000)
	}
	sandbox, err := NewSandbox(client, cfg)
	if err != nil {
		t.Fatalf("NewSandbox() error = %v", err)
	}
	return sandbox
}

func TestNewSandboxValidatesConfig(t *testing.T) {
	if _, err := NewSandbox(nil, Config{PayIn: decimal.NewFromInt(1)}); err == nil {
		t.Fatal("NewSandbox() error = nil for nil client")
	}
	if _, err := NewSandbox(&fakeClient{}, Config{}); err == nil {
		t.Fatal("NewSandbox() error = nil for non-positive pay in")
	}
}

func TestEnsureAccountUsesConfiguredID(t *testing.T) {
	var opened bool
	client := &fakeClient{
		accounts: func(context.Context) ([]*pb.Account, error) {
			return []*pb.Account{{Id: "configured"}}, nil
		},
		open: func(context.Context) (string, error) {
			opened = true
			return "acc-new", nil
		},
	}
	sandbox := newSandboxForTest(t, client, Config{AccountID: " configured "})

	accountID, err := sandbox.EnsureAccount(context.Background())
	if err != nil {
		t.Fatalf("EnsureAccount() error = %v", err)
	}
	if accountID != "configured" {
		t.Fatalf("EnsureAccount() = %q, want configured", accountID)
	}
	if opened {
		t.Fatal("EnsureAccount() opened a new account for an existing configured one")
	}
}

func TestEnsureAccountConfiguredIDNotFound(t *testing.T) {
	client := &fakeClient{
		accounts: func(context.Context) ([]*pb.Account, error) {
			return []*pb.Account{{Id: "other-account"}}, nil
		},
	}
	sandbox := newSandboxForTest(t, client, Config{AccountID: "missing"})

	_, err := sandbox.EnsureAccount(context.Background())
	if err == nil {
		t.Fatal("EnsureAccount() error = nil for missing configured account")
	}
	if !strings.Contains(err.Error(), "missing") || !strings.Contains(err.Error(), "tinkoff.account_id") {
		t.Fatalf("EnsureAccount() error = %v, want missing account id and remediation hint", err)
	}
}

func TestEnsureAccountReusesExistingSandboxAccount(t *testing.T) {
	var opened bool
	client := &fakeClient{
		accounts: func(context.Context) ([]*pb.Account, error) {
			return []*pb.Account{{Id: ""}, {Id: " acc-1 "}}, nil
		},
		open: func(context.Context) (string, error) {
			opened = true
			return "acc-new", nil
		},
	}
	sandbox := newSandboxForTest(t, client, Config{})

	accountID, err := sandbox.EnsureAccount(context.Background())
	if err != nil {
		t.Fatalf("EnsureAccount() error = %v", err)
	}
	if accountID != "acc-1" {
		t.Fatalf("EnsureAccount() = %q, want acc-1", accountID)
	}
	if opened {
		t.Fatal("EnsureAccount() opened a new account despite an existing one")
	}
	if sandbox.AccountID() != "acc-1" {
		t.Fatalf("AccountID() = %q, want acc-1", sandbox.AccountID())
	}
}

func TestEnsureAccountOpensAndFundsNewAccount(t *testing.T) {
	var opened string
	var paidAccount string
	var paidAmount decimal.Decimal
	client := &fakeClient{
		accounts: func(context.Context) ([]*pb.Account, error) {
			if opened == "" {
				return nil, nil
			}
			return []*pb.Account{{Id: opened}}, nil
		},
		open: func(context.Context) (string, error) {
			opened = "acc-new"
			return opened, nil
		},
		payIn: func(_ context.Context, accountID string, amount decimal.Decimal) (decimal.Decimal, error) {
			paidAccount = accountID
			paidAmount = amount
			return amount, nil
		},
	}
	sandbox := newSandboxForTest(t, client, Config{PayIn: decimal.RequireFromString("50000")})

	accountID, err := sandbox.EnsureAccount(context.Background())
	if err != nil {
		t.Fatalf("EnsureAccount() error = %v", err)
	}
	if accountID != "acc-new" {
		t.Fatalf("EnsureAccount() = %q, want acc-new", accountID)
	}
	if paidAccount != "acc-new" {
		t.Fatalf("pay in account = %q, want acc-new", paidAccount)
	}
	if !paidAmount.Equal(decimal.RequireFromString("50000")) {
		t.Fatalf("pay in amount = %s, want 50000", paidAmount)
	}

	second, err := sandbox.EnsureAccount(context.Background())
	if err != nil {
		t.Fatalf("second EnsureAccount() error = %v", err)
	}
	if second != "acc-new" {
		t.Fatalf("second EnsureAccount() = %q, want acc-new", second)
	}
}

func TestEnsureAccountPropagatesErrors(t *testing.T) {
	accountErr := errors.New("accounts unavailable")
	sandbox := newSandboxForTest(t, &fakeClient{
		accounts: func(context.Context) ([]*pb.Account, error) { return nil, accountErr },
	}, Config{})
	if _, err := sandbox.EnsureAccount(context.Background()); !errors.Is(err, accountErr) {
		t.Fatalf("EnsureAccount() error = %v, want %v", err, accountErr)
	}

	openErr := errors.New("open failed")
	sandbox = newSandboxForTest(t, &fakeClient{
		accounts: func(context.Context) ([]*pb.Account, error) { return nil, nil },
		open:     func(context.Context) (string, error) { return "", openErr },
	}, Config{})
	if _, err := sandbox.EnsureAccount(context.Background()); !errors.Is(err, openErr) {
		t.Fatalf("EnsureAccount() error = %v, want %v", err, openErr)
	}

	payInErr := errors.New("pay in failed")
	sandbox = newSandboxForTest(t, &fakeClient{
		accounts: func(context.Context) ([]*pb.Account, error) { return nil, nil },
		open:     func(context.Context) (string, error) { return "acc-1", nil },
		payIn: func(context.Context, string, decimal.Decimal) (decimal.Decimal, error) {
			return decimal.Zero, payInErr
		},
	}, Config{})
	if _, err := sandbox.EnsureAccount(context.Background()); !errors.Is(err, payInErr) {
		t.Fatalf("EnsureAccount() error = %v, want %v", err, payInErr)
	}
}

func TestResolveInstrumentIDCachesResults(t *testing.T) {
	calls := 0
	client := &fakeClient{
		resolveUID: func(_ context.Context, ticker string) (string, error) {
			calls++
			if ticker != "SBER" {
				t.Fatalf("ticker = %q, want SBER", ticker)
			}
			return "uid-sber", nil
		},
	}
	sandbox := newSandboxForTest(t, client, Config{})

	for _, ticker := range []string{"SBER", " sber ", "SBER"} {
		uid, err := sandbox.ResolveInstrumentID(context.Background(), ticker)
		if err != nil {
			t.Fatalf("ResolveInstrumentID(%q) error = %v", ticker, err)
		}
		if uid != "uid-sber" {
			t.Fatalf("ResolveInstrumentID(%q) = %q, want uid-sber", ticker, uid)
		}
	}
	if calls != 1 {
		t.Fatalf("resolve calls = %d, want 1", calls)
	}

	if _, err := sandbox.ResolveInstrumentID(context.Background(), "  "); err == nil {
		t.Fatal("ResolveInstrumentID() error = nil for empty ticker")
	}
}

func TestResolveLotSizeCachesResults(t *testing.T) {
	calls := 0
	client := &fakeClient{
		resolveUID: func(_ context.Context, ticker string) (string, error) {
			if ticker != "SBER" {
				t.Fatalf("ticker = %q, want SBER", ticker)
			}
			return "uid-sber", nil
		},
		lotSize: func(_ context.Context, uid string) (int32, error) {
			calls++
			if uid != "uid-sber" {
				t.Fatalf("uid = %q, want uid-sber", uid)
			}
			return 10, nil
		},
	}
	sandbox := newSandboxForTest(t, client, Config{})

	for _, ticker := range []string{"SBER", " sber ", "SBER"} {
		lot, err := sandbox.ResolveLotSize(context.Background(), ticker)
		if err != nil {
			t.Fatalf("ResolveLotSize(%q) error = %v", ticker, err)
		}
		if !lot.Equal(decimal.NewFromInt(10)) {
			t.Fatalf("ResolveLotSize(%q) = %s, want 10", ticker, lot)
		}
	}
	if calls != 1 {
		t.Fatalf("resolve calls = %d, want 1", calls)
	}
}

func TestResolveLotSizePropagatesError(t *testing.T) {
	resolveErr := errors.New("lot lookup failed")
	client := &fakeClient{
		resolveUID: func(context.Context, string) (string, error) { return "uid-sber", nil },
		lotSize:    func(context.Context, string) (int32, error) { return 0, resolveErr },
	}
	sandbox := newSandboxForTest(t, client, Config{})

	if _, err := sandbox.ResolveLotSize(context.Background(), "SBER"); !errors.Is(err, resolveErr) {
		t.Fatalf("ResolveLotSize() error = %v, want %v", err, resolveErr)
	}
}

func TestResolveInstrumentIDPropagatesError(t *testing.T) {
	resolveErr := errors.New("instrument not found")
	client := &fakeClient{
		resolveUID: func(context.Context, string) (string, error) { return "", resolveErr },
	}
	sandbox := newSandboxForTest(t, client, Config{})

	if _, err := sandbox.ResolveInstrumentID(context.Background(), "SBER"); !errors.Is(err, resolveErr) {
		t.Fatalf("ResolveInstrumentID() error = %v, want %v", err, resolveErr)
	}
}

func TestPostOrderValidatesAccount(t *testing.T) {
	var got *pb.PostOrderRequest
	client := &fakeClient{
		postOrder: func(_ context.Context, request *pb.PostOrderRequest) (*pb.PostOrderResponse, error) {
			got = request
			return &pb.PostOrderResponse{OrderId: "broker-order"}, nil
		},
	}
	sandbox := newSandboxForTest(t, client, Config{AccountID: "acc-1"})

	if _, err := sandbox.PostOrder(context.Background(), nil); err == nil {
		t.Fatal("PostOrder() error = nil for nil request")
	}
	if _, err := sandbox.PostOrder(context.Background(), &pb.PostOrderRequest{AccountId: "other"}); err == nil {
		t.Fatal("PostOrder() error = nil for mismatched account")
	}

	response, err := sandbox.PostOrder(context.Background(), &pb.PostOrderRequest{AccountId: "acc-1", OrderId: "order-1"})
	if err != nil {
		t.Fatalf("PostOrder() error = %v", err)
	}
	if response.GetOrderId() != "broker-order" {
		t.Fatalf("PostOrder() response = %+v", response)
	}
	if got.GetOrderId() != "order-1" {
		t.Fatalf("delegated request = %+v", got)
	}

	uninitialized := newSandboxForTest(t, &fakeClient{}, Config{})
	if _, err := uninitialized.PostOrder(context.Background(), &pb.PostOrderRequest{AccountId: "acc-1"}); err == nil {
		t.Fatal("PostOrder() error = nil for uninitialized account")
	}
}

func TestSnapshotTracksDayStartEquity(t *testing.T) {
	equity := decimal.RequireFromString("100000")
	current := time.Date(2024, 3, 1, 10, 0, 0, 0, time.UTC)
	client := &fakeClient{
		portfolio: func(_ context.Context, accountID string) (*pb.PortfolioResponse, error) {
			if accountID != "acc-1" {
				t.Fatalf("accountID = %q, want acc-1", accountID)
			}
			value := equity
			units := value.IntPart()
			nano := value.Sub(decimal.NewFromInt(units)).Mul(decimal.NewFromInt(1000000000)).IntPart()
			return &pb.PortfolioResponse{TotalAmountPortfolio: money(units, int32(nano))}, nil
		},
	}
	sandbox := newSandboxForTest(t, client, Config{
		AccountID: "acc-1",
		PayIn:     decimal.RequireFromString("150000"),
		Now:       func() time.Time { return current },
	})

	snapshot, err := sandbox.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if !snapshot.Deposit.Equal(decimal.RequireFromString("150000")) {
		t.Fatalf("Deposit = %s, want 150000", snapshot.Deposit)
	}
	if !snapshot.DayStartEquity.Equal(equity) || !snapshot.CurrentEquity.Equal(equity) {
		t.Fatalf("snapshot = %+v, want both equities %s", snapshot, equity)
	}

	equity = decimal.RequireFromString("97000")
	snapshot, err = sandbox.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if !snapshot.DayStartEquity.Equal(decimal.RequireFromString("100000")) {
		t.Fatalf("DayStartEquity = %s, want 100000", snapshot.DayStartEquity)
	}
	if !snapshot.CurrentEquity.Equal(equity) {
		t.Fatalf("CurrentEquity = %s, want %s", snapshot.CurrentEquity, equity)
	}

	current = current.AddDate(0, 0, 1)
	equity = decimal.RequireFromString("98000")
	snapshot, err = sandbox.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if !snapshot.DayStartEquity.Equal(equity) {
		t.Fatalf("DayStartEquity after day change = %s, want %s", snapshot.DayStartEquity, equity)
	}
}

func TestSnapshotErrors(t *testing.T) {
	uninitialized := newSandboxForTest(t, &fakeClient{}, Config{})
	if _, err := uninitialized.Snapshot(context.Background()); err == nil {
		t.Fatal("Snapshot() error = nil for uninitialized account")
	}

	portfolioErr := errors.New("portfolio unavailable")
	sandbox := newSandboxForTest(t, &fakeClient{
		portfolio: func(context.Context, string) (*pb.PortfolioResponse, error) {
			return nil, portfolioErr
		},
	}, Config{AccountID: "acc-1"})
	if _, err := sandbox.Snapshot(context.Background()); !errors.Is(err, portfolioErr) {
		t.Fatalf("Snapshot() error = %v, want %v", err, portfolioErr)
	}

	sandbox = newSandboxForTest(t, &fakeClient{
		portfolio: func(context.Context, string) (*pb.PortfolioResponse, error) {
			return &pb.PortfolioResponse{}, nil
		},
	}, Config{AccountID: "acc-1"})
	if _, err := sandbox.Snapshot(context.Background()); err == nil {
		t.Fatal("Snapshot() error = nil for missing portfolio amount")
	}

	sandbox = newSandboxForTest(t, &fakeClient{
		portfolio: func(context.Context, string) (*pb.PortfolioResponse, error) {
			return &pb.PortfolioResponse{TotalAmountPortfolio: money(0, 0)}, nil
		},
	}, Config{AccountID: "acc-1"})
	if _, err := sandbox.Snapshot(context.Background()); err == nil {
		t.Fatal("Snapshot() error = nil for non-positive equity")
	}
}

func TestCancelOpenOrders(t *testing.T) {
	var cancelled []string
	cancelErr := errors.New("cancel rejected")
	client := &fakeClient{
		resolveUID: func(_ context.Context, ticker string) (string, error) {
			return "uid-" + strings.ToLower(ticker), nil
		},
		orders: func(_ context.Context, accountID string) ([]*pb.OrderState, error) {
			if accountID != "acc-1" {
				t.Fatalf("accountID = %q, want acc-1", accountID)
			}
			return []*pb.OrderState{
				{OrderId: "order-1", InstrumentUid: "uid-sber"},
				{OrderId: " ", InstrumentUid: "uid-sber"},
				{OrderId: "order-2", InstrumentUid: "uid-sber"},
				{OrderId: "ofz-order", InstrumentUid: "uid-ofz"},
				{OrderId: "unknown-order"},
			}, nil
		},
		cancel: func(_ context.Context, accountID, orderID string) error {
			if accountID != "acc-1" {
				t.Fatalf("cancel accountID = %q, want acc-1", accountID)
			}
			cancelled = append(cancelled, orderID)
			if orderID == "order-2" {
				return cancelErr
			}
			return nil
		},
	}
	sandbox := newSandboxForTest(t, client, Config{AccountID: "acc-1"})
	if _, err := sandbox.ResolveInstrumentID(context.Background(), "SBER"); err != nil {
		t.Fatalf("ResolveInstrumentID() error = %v", err)
	}

	err := sandbox.CancelOpenOrders(context.Background())
	if !errors.Is(err, cancelErr) {
		t.Fatalf("CancelOpenOrders() error = %v, want %v", err, cancelErr)
	}
	if strings.Join(cancelled, ",") != "order-1,order-2" {
		t.Fatalf("cancelled orders = %v, want only the bot's own orders order-1,order-2", cancelled)
	}

	ordersErr := errors.New("orders unavailable")
	sandbox = newSandboxForTest(t, &fakeClient{
		orders: func(context.Context, string) ([]*pb.OrderState, error) { return nil, ordersErr },
	}, Config{AccountID: "acc-1"})
	if err := sandbox.CancelOpenOrders(context.Background()); !errors.Is(err, ordersErr) {
		t.Fatalf("CancelOpenOrders() error = %v, want %v", err, ordersErr)
	}

	uninitialized := newSandboxForTest(t, &fakeClient{}, Config{})
	if err := uninitialized.CancelOpenOrders(context.Background()); err == nil {
		t.Fatal("CancelOpenOrders() error = nil for uninitialized account")
	}
}

func TestCancelOpenOrdersLeavesForeignOrdersAlone(t *testing.T) {
	var cancelled []string
	client := &fakeClient{
		orders: func(context.Context, string) ([]*pb.OrderState, error) {
			return []*pb.OrderState{
				{OrderId: "ofz-order", InstrumentUid: "uid-ofz"},
				{OrderId: "posi-order", InstrumentUid: "uid-posi"},
			}, nil
		},
		cancel: func(_ context.Context, _, orderID string) error {
			cancelled = append(cancelled, orderID)
			return nil
		},
	}
	sandbox := newSandboxForTest(t, client, Config{AccountID: "acc-1"})

	if err := sandbox.CancelOpenOrders(context.Background()); err != nil {
		t.Fatalf("CancelOpenOrders() error = %v", err)
	}
	if len(cancelled) != 0 {
		t.Fatalf("cancelled orders = %v, want none for instruments the bot does not trade", cancelled)
	}
}

func TestMarketOpen(t *testing.T) {
	var statusCalls int
	client := &fakeClient{
		resolveUID: func(_ context.Context, ticker string) (string, error) {
			return "uid-" + strings.ToLower(ticker), nil
		},
		tradingStatus: func(_ context.Context, instrumentID string) (*pb.GetTradingStatusResponse, error) {
			statusCalls++
			if instrumentID != "uid-sber" {
				t.Fatalf("instrumentID = %q, want uid-sber", instrumentID)
			}
			return &pb.GetTradingStatusResponse{
				TradingStatus:            pb.SecurityTradingStatus_SECURITY_TRADING_STATUS_NORMAL_TRADING,
				MarketOrderAvailableFlag: true,
			}, nil
		},
	}
	sandbox := newSandboxForTest(t, client, Config{})

	open, err := sandbox.MarketOpen(context.Background(), "SBER")
	if err != nil {
		t.Fatalf("MarketOpen() error = %v", err)
	}
	if !open {
		t.Fatal("MarketOpen() = false, want true")
	}
	if statusCalls != 1 {
		t.Fatalf("trading status calls = %d, want 1", statusCalls)
	}
}

func TestMarketOpenFalseWhenNoOrdersAvailable(t *testing.T) {
	client := &fakeClient{
		resolveUID: func(context.Context, string) (string, error) { return "uid-1", nil },
		tradingStatus: func(context.Context, string) (*pb.GetTradingStatusResponse, error) {
			return &pb.GetTradingStatusResponse{
				TradingStatus:            pb.SecurityTradingStatus_SECURITY_TRADING_STATUS_NOT_AVAILABLE_FOR_TRADING,
				MarketOrderAvailableFlag: false,
				LimitOrderAvailableFlag:  false,
			}, nil
		},
	}
	sandbox := newSandboxForTest(t, client, Config{})

	open, err := sandbox.MarketOpen(context.Background(), "SBER")
	if err != nil {
		t.Fatalf("MarketOpen() error = %v", err)
	}
	if open {
		t.Fatal("MarketOpen() = true, want false")
	}
}

func TestMarketOpenAcceptsLimitOnlySession(t *testing.T) {
	client := &fakeClient{
		resolveUID: func(context.Context, string) (string, error) { return "uid-1", nil },
		tradingStatus: func(context.Context, string) (*pb.GetTradingStatusResponse, error) {
			return &pb.GetTradingStatusResponse{
				LimitOrderAvailableFlag:  true,
				MarketOrderAvailableFlag: false,
			}, nil
		},
	}
	sandbox := newSandboxForTest(t, client, Config{})

	open, err := sandbox.MarketOpen(context.Background(), "SBER")
	if err != nil {
		t.Fatalf("MarketOpen() error = %v", err)
	}
	if !open {
		t.Fatal("MarketOpen() = false, want true for a limit-only session")
	}
}

func TestMarketOpenPropagatesErrors(t *testing.T) {
	resolveErr := errors.New("instrument not found")
	sandbox := newSandboxForTest(t, &fakeClient{
		resolveUID: func(context.Context, string) (string, error) { return "", resolveErr },
	}, Config{})
	if _, err := sandbox.MarketOpen(context.Background(), "SBER"); !errors.Is(err, resolveErr) {
		t.Fatalf("MarketOpen() error = %v, want %v", err, resolveErr)
	}

	statusErr := errors.New("status unavailable")
	sandbox = newSandboxForTest(t, &fakeClient{
		resolveUID:    func(context.Context, string) (string, error) { return "uid-1", nil },
		tradingStatus: func(context.Context, string) (*pb.GetTradingStatusResponse, error) { return nil, statusErr },
	}, Config{})
	if _, err := sandbox.MarketOpen(context.Background(), "SBER"); !errors.Is(err, statusErr) {
		t.Fatalf("MarketOpen() error = %v, want %v", err, statusErr)
	}
}

func TestCloseDelegates(t *testing.T) {
	client := &fakeClient{}
	sandbox := newSandboxForTest(t, client, Config{})
	if err := sandbox.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if !client.closed {
		t.Fatal("Close() did not close the client")
	}
}

func TestSnapshotSatisfiesRiskAccountContract(t *testing.T) {
	client := &fakeClient{
		portfolio: func(context.Context, string) (*pb.PortfolioResponse, error) {
			return &pb.PortfolioResponse{TotalAmountPortfolio: money(96000, 0)}, nil
		},
	}
	sandbox := newSandboxForTest(t, client, Config{
		AccountID: "acc-1",
		PayIn:     decimal.RequireFromString("100000"),
	})
	snapshot, err := sandbox.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	var _ risk.Account = snapshot
	drawdown := snapshot.Deposit.Sub(snapshot.CurrentEquity).Div(snapshot.Deposit).Mul(decimal.NewFromInt(100))
	if !drawdown.Equal(decimal.NewFromInt(4)) {
		t.Fatalf("drawdown = %s, want 4", drawdown)
	}
}

func TestMaxOpenPositionNotionalReturnsLargestPosition(t *testing.T) {
	client := &fakeClient{
		portfolio: func(context.Context, string) (*pb.PortfolioResponse, error) {
			return &pb.PortfolioResponse{Positions: []*pb.PortfolioPosition{
				{Quantity: quotation(10, 0), CurrentPrice: money(100, 0)},
				{Quantity: quotation(5, 0), CurrentPrice: money(500, 0)},
			}}, nil
		},
	}
	sandbox := newSandboxForTest(t, client, Config{AccountID: "acc-1"})

	got, err := sandbox.MaxOpenPositionNotional(context.Background())
	if err != nil {
		t.Fatalf("MaxOpenPositionNotional() error = %v", err)
	}
	if !got.Equal(decimal.NewFromInt(2500)) {
		t.Fatalf("MaxOpenPositionNotional() = %s, want 2500", got)
	}
}

func TestMaxOpenPositionNotionalReturnsZeroWithoutPositions(t *testing.T) {
	client := &fakeClient{
		portfolio: func(context.Context, string) (*pb.PortfolioResponse, error) {
			return &pb.PortfolioResponse{}, nil
		},
	}
	sandbox := newSandboxForTest(t, client, Config{AccountID: "acc-1"})

	got, err := sandbox.MaxOpenPositionNotional(context.Background())
	if err != nil {
		t.Fatalf("MaxOpenPositionNotional() error = %v", err)
	}
	if !got.IsZero() {
		t.Fatalf("MaxOpenPositionNotional() = %s, want zero", got)
	}
}

func TestMaxOpenPositionNotionalUsesAbsoluteNotional(t *testing.T) {
	client := &fakeClient{
		portfolio: func(context.Context, string) (*pb.PortfolioResponse, error) {
			return &pb.PortfolioResponse{Positions: []*pb.PortfolioPosition{
				{Quantity: quotation(-10, 0), CurrentPrice: money(100, 0)},
			}}, nil
		},
	}
	sandbox := newSandboxForTest(t, client, Config{AccountID: "acc-1"})

	got, err := sandbox.MaxOpenPositionNotional(context.Background())
	if err != nil {
		t.Fatalf("MaxOpenPositionNotional() error = %v", err)
	}
	if !got.Equal(decimal.NewFromInt(1000)) {
		t.Fatalf("MaxOpenPositionNotional() = %s, want 1000", got)
	}
}

func TestMaxOpenPositionNotionalRequiresAccount(t *testing.T) {
	sandbox := newSandboxForTest(t, &fakeClient{}, Config{})
	if _, err := sandbox.MaxOpenPositionNotional(context.Background()); err == nil {
		t.Fatal("MaxOpenPositionNotional() error = nil without account, want error")
	}
}
