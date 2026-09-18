package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	pb "github.com/tinkoff/invest-api-go-sdk/proto"

	"github.com/olegsidorkin/moex-trader/internal/borrowcost"
	brokertinkoff "github.com/olegsidorkin/moex-trader/internal/broker/tinkoff"
	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/drift"
	"github.com/olegsidorkin/moex-trader/internal/executor"
	"github.com/olegsidorkin/moex-trader/internal/features"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/tinkoff"
	"github.com/olegsidorkin/moex-trader/internal/model"
	"github.com/olegsidorkin/moex-trader/internal/orchestrator"
	"github.com/olegsidorkin/moex-trader/internal/risk"
	"github.com/olegsidorkin/moex-trader/internal/storage"
)

type traderTestIngestor struct{}

func (traderTestIngestor) Ingest(context.Context, string) (features.Input, error) {
	return features.Input{}, nil
}

type traderTestGate struct{}

func (traderTestGate) Approve(context.Context, risk.Request) (bool, error) {
	return true, nil
}

type traderTestExecutor struct{}

func (traderTestExecutor) Execute(context.Context, domain.TradeSignal, decimal.Decimal) (executor.Fill, error) {
	return executor.Fill{}, nil
}

type traderTestAudit struct{}

func (traderTestAudit) InsertAuditEvent(context.Context, domain.AuditEvent) error {
	return nil
}

func openTraderTestStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "trader.db"))
	if err != nil {
		t.Fatalf("storage.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("store.Close() error = %v", err)
		}
	})
	return store
}

type fakeSandboxClient struct {
	closed         bool
	resolveUID     func(ctx context.Context, ticker string) (string, error)
	tradingStatus  func(ctx context.Context, instrumentID string) (*pb.GetTradingStatusResponse, error)
	resolveLotSize func(ctx context.Context, instrumentUID string) (int32, error)
	operations     func(ctx context.Context, accountID string, from, to time.Time) ([]*pb.Operation, error)
}

func (f *fakeSandboxClient) ResolveInstrumentUID(ctx context.Context, ticker string) (string, error) {
	if f.resolveUID != nil {
		return f.resolveUID(ctx, ticker)
	}
	return "", fmt.Errorf("unexpected ResolveInstrumentUID call")
}

func (f *fakeSandboxClient) TradingStatus(ctx context.Context, instrumentID string) (*pb.GetTradingStatusResponse, error) {
	if f.tradingStatus != nil {
		return f.tradingStatus(ctx, instrumentID)
	}
	return nil, fmt.Errorf("unexpected TradingStatus call")
}

func (f *fakeSandboxClient) ResolveLotSize(ctx context.Context, instrumentUID string) (int32, error) {
	if f.resolveLotSize != nil {
		return f.resolveLotSize(ctx, instrumentUID)
	}
	return 0, fmt.Errorf("unexpected ResolveLotSize call")
}

func (f *fakeSandboxClient) SandboxAccounts(context.Context) ([]*pb.Account, error) {
	return []*pb.Account{{Id: "acc-1"}}, nil
}

func (f *fakeSandboxClient) OpenSandboxAccount(context.Context) (string, error) {
	return "", fmt.Errorf("unexpected OpenSandboxAccount call")
}

func (f *fakeSandboxClient) SandboxPayIn(context.Context, string, decimal.Decimal) (decimal.Decimal, error) {
	return decimal.Zero, fmt.Errorf("unexpected SandboxPayIn call")
}

func (f *fakeSandboxClient) PostSandboxOrder(context.Context, *pb.PostOrderRequest) (*pb.PostOrderResponse, error) {
	return nil, fmt.Errorf("unexpected PostSandboxOrder call")
}

func (f *fakeSandboxClient) GetSandboxPortfolio(context.Context, string) (*pb.PortfolioResponse, error) {
	return nil, fmt.Errorf("unexpected GetSandboxPortfolio call")
}

func (f *fakeSandboxClient) GetSandboxOrders(context.Context, string) ([]*pb.OrderState, error) {
	return nil, fmt.Errorf("unexpected GetSandboxOrders call")
}

func (f *fakeSandboxClient) SandboxOperations(ctx context.Context, accountID string, from, to time.Time) ([]*pb.Operation, error) {
	if f.operations != nil {
		return f.operations(ctx, accountID, from, to)
	}
	return nil, fmt.Errorf("unexpected SandboxOperations call")
}

func (f *fakeSandboxClient) CancelSandboxOrder(context.Context, string, string) error {
	return fmt.Errorf("unexpected CancelSandboxOrder call")
}

func (f *fakeSandboxClient) Close() error {
	f.closed = true
	return nil
}

func TestNewBrokerRuntimePicksPaper(t *testing.T) {
	store := openTraderTestStore(t)
	cfg := &config.Config{
		Broker:         config.BrokerPaper,
		IsPaperTrading: true,
		Commission:     config.Commission{Rate: decimal.New(1, -4)},
	}

	runtime, err := newBrokerRuntime(context.Background(), cfg, store, time.Now, brokerDeps{})
	if err != nil {
		t.Fatalf("newBrokerRuntime() error = %v", err)
	}
	if _, ok := runtime.exec.(*executor.TargetPositionExecutor); !ok {
		t.Fatalf("newBrokerRuntime() type = %T, want *executor.TargetPositionExecutor", runtime.exec)
	}
	if _, ok := runtime.exec.(*executor.TargetPositionExecutor).Inner().(*executor.PaperExecutor); !ok {
		t.Fatalf("newBrokerRuntime() inner type = %T, want *executor.PaperExecutor", runtime.exec.(*executor.TargetPositionExecutor).Inner())
	}
	if runtime.accountSource != nil || runtime.canceller != nil {
		t.Fatalf("paper runtime should not wire account source or canceller: %+v", runtime)
	}
}

func TestNewBrokerRuntimeEmptyBrokerDefaultsToPaper(t *testing.T) {
	store := openTraderTestStore(t)
	cfg := &config.Config{IsPaperTrading: true}

	runtime, err := newBrokerRuntime(context.Background(), cfg, store, time.Now, brokerDeps{})
	if err != nil {
		t.Fatalf("newBrokerRuntime() error = %v", err)
	}
	if _, ok := runtime.exec.(*executor.TargetPositionExecutor); !ok {
		t.Fatalf("newBrokerRuntime() type = %T, want *executor.TargetPositionExecutor", runtime.exec)
	}
	if _, ok := runtime.exec.(*executor.TargetPositionExecutor).Inner().(*executor.PaperExecutor); !ok {
		t.Fatalf("newBrokerRuntime() inner type = %T, want *executor.PaperExecutor", runtime.exec.(*executor.TargetPositionExecutor).Inner())
	}
}

func TestNewBrokerRuntimeRefusesLiveBrokers(t *testing.T) {
	for _, broker := range []string{config.BrokerTinkoff, config.BrokerFinam} {
		cfg := &config.Config{Broker: broker, IsPaperTrading: false}
		_, err := newBrokerRuntime(context.Background(), cfg, nil, time.Now, brokerDeps{})
		if err == nil {
			t.Fatalf("newBrokerRuntime(%q) error = nil, want refusal", broker)
		}
		if !strings.Contains(err.Error(), broker) {
			t.Fatalf("newBrokerRuntime(%q) error = %v, want broker name in message", broker, err)
		}
		if !strings.Contains(err.Error(), "live trading mode is not wired") {
			t.Fatalf("newBrokerRuntime(%q) error = %v, want live wiring refusal", broker, err)
		}
	}
}

func TestNewBrokerRuntimeRefusesLiveBrokerInPaperMode(t *testing.T) {
	for _, broker := range []string{config.BrokerTinkoff, config.BrokerFinam} {
		cfg := &config.Config{Broker: broker, IsPaperTrading: true}
		_, err := newBrokerRuntime(context.Background(), cfg, nil, time.Now, brokerDeps{})
		if err == nil {
			t.Fatalf("newBrokerRuntime(%q) error = nil, want refusal", broker)
		}
		if !strings.Contains(err.Error(), broker) {
			t.Fatalf("newBrokerRuntime(%q) error = %v, want broker name in message", broker, err)
		}
	}
}

func TestNewBrokerRuntimeRefusesLiveModeForPaperBroker(t *testing.T) {
	cfg := &config.Config{Broker: config.BrokerPaper, IsPaperTrading: false}
	_, err := newBrokerRuntime(context.Background(), cfg, nil, time.Now, brokerDeps{})
	if err == nil {
		t.Fatal("newBrokerRuntime() error = nil, want refusal")
	}
	if !strings.Contains(err.Error(), "live trading mode is not wired") {
		t.Fatalf("newBrokerRuntime() error = %v, want live wiring refusal", err)
	}
}

func TestNewBrokerRuntimeUnknownBroker(t *testing.T) {
	cfg := &config.Config{Broker: "alfa", IsPaperTrading: true}
	_, err := newBrokerRuntime(context.Background(), cfg, nil, time.Now, brokerDeps{})
	if err == nil {
		t.Fatal("newBrokerRuntime() error = nil, want unknown broker error")
	}
	if !strings.Contains(err.Error(), "unknown broker") {
		t.Fatalf("newBrokerRuntime() error = %v, want unknown broker error", err)
	}
}

func TestNewBrokerRuntimeNilConfig(t *testing.T) {
	_, err := newBrokerRuntime(context.Background(), nil, nil, time.Now, brokerDeps{})
	if err == nil {
		t.Fatal("newBrokerRuntime() error = nil, want nil config error")
	}
}

func TestNewBrokerRuntimeSandboxWiresLiveExecutor(t *testing.T) {
	store := openTraderTestStore(t)
	cfg := &config.Config{
		Broker:         config.BrokerTinkoff,
		IsPaperTrading: false,
		Commission:     config.Commission{Rate: decimal.New(5, -4)},
		Tinkoff: config.Tinkoff{
			Sandbox:   true,
			AccountID: "acc-1",
			PayIn:     decimal.RequireFromString("100000"),
			OrderType: config.OrderTypeMarket,
		},
	}
	client := &fakeSandboxClient{}
	deps := brokerDeps{
		dialTinkoff: func(context.Context, tinkoff.Config) (brokertinkoff.Client, error) {
			return client, nil
		},
	}

	runtime, err := newBrokerRuntime(context.Background(), cfg, store, time.Now, deps)
	if err != nil {
		t.Fatalf("newBrokerRuntime() error = %v", err)
	}
	if _, ok := runtime.exec.(*executor.TargetPositionExecutor); !ok {
		t.Fatalf("newBrokerRuntime() executor type = %T, want *executor.TargetPositionExecutor", runtime.exec)
	}
	windowed, ok := runtime.exec.(*executor.TargetPositionExecutor).Inner().(*tradingWindowExecutor)
	if !ok {
		t.Fatalf("newBrokerRuntime() inner executor type = %T, want *tradingWindowExecutor", runtime.exec.(*executor.TargetPositionExecutor).Inner())
	}
	guarded, ok := windowed.inner.(*marketHoursExecutor)
	if !ok {
		t.Fatalf("newBrokerRuntime() windowed inner executor type = %T, want *marketHoursExecutor", windowed.inner)
	}
	if _, ok := guarded.inner.(*executor.LiveExecutor); !ok {
		t.Fatalf("newBrokerRuntime() guarded inner executor type = %T, want *executor.LiveExecutor", guarded.inner)
	}
	if runtime.accountSource == nil {
		t.Fatal("sandbox runtime account source is nil")
	}
	if runtime.canceller == nil {
		t.Fatal("sandbox runtime canceller is nil")
	}
	if runtime.closeFn == nil {
		t.Fatal("sandbox runtime close function is nil")
	}
	if err := runtime.closeFn(); err != nil {
		t.Fatalf("closeFn() error = %v", err)
	}
	if !client.closed {
		t.Fatal("closeFn() did not close the sandbox client")
	}
}

func TestNewBrokerRuntimeSandboxDialError(t *testing.T) {
	dialErr := fmt.Errorf("dial failed")
	deps := brokerDeps{
		dialTinkoff: func(context.Context, tinkoff.Config) (brokertinkoff.Client, error) {
			return nil, dialErr
		},
	}
	cfg := &config.Config{
		Broker:         config.BrokerTinkoff,
		IsPaperTrading: false,
		Tinkoff:        config.Tinkoff{Sandbox: true, PayIn: decimal.RequireFromString("100000")},
	}
	if _, err := newBrokerRuntime(context.Background(), cfg, nil, time.Now, deps); !errors.Is(err, dialErr) {
		t.Fatalf("newBrokerRuntime() error = %v, want %v", err, dialErr)
	}
}

type fakeHistorySource struct {
	candles map[string][]moex.Candle
	err     error
	calls   int
}

func (f *fakeHistorySource) History(_ context.Context, ticker string, from, till time.Time) ([]moex.Candle, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	out := make([]moex.Candle, 0, len(f.candles[ticker]))
	for _, candle := range f.candles[ticker] {
		if candle.Begin.Before(from) || candle.Begin.After(till) {
			continue
		}
		out = append(out, candle)
	}
	return out, nil
}

type fixedSignalSource struct {
	action domain.Action
}

func (f fixedSignalSource) Generate(_ context.Context, feature domain.FeatureContext) (domain.TradeSignal, error) {
	return domain.TradeSignal{
		Ticker:      feature.Ticker,
		Action:      f.action,
		Confidence:  decimal.RequireFromString("0.9"),
		TargetLots:  1,
		Reasoning:   "preflight test",
		GeneratedAt: time.Now(),
	}, nil
}

type alternatingSignalSource struct {
	calls int
}

func (a *alternatingSignalSource) Generate(_ context.Context, feature domain.FeatureContext) (domain.TradeSignal, error) {
	a.calls++
	action := domain.ActionHold
	switch a.calls {
	case 1:
		action = domain.ActionBuy
	case 2:
		action = domain.ActionSell
	}
	return domain.TradeSignal{
		Ticker:      feature.Ticker,
		Action:      action,
		Confidence:  decimal.RequireFromString("0.9"),
		TargetLots:  1,
		Reasoning:   "preflight test",
		GeneratedAt: time.Now(),
	}, nil
}

func testCandles(start time.Time, count int, step float64) []moex.Candle {
	return testCandlesFrom(start, count, 100, step)
}

func testCandlesFrom(start time.Time, count int, startPrice, step float64) []moex.Candle {
	candles := make([]moex.Candle, 0, count)
	price := startPrice
	for i := 0; i < count; i++ {
		open := price
		price += step
		closePrice := price
		candles = append(candles, moex.Candle{
			Open:   decimal.NewFromFloat(open),
			High:   decimal.NewFromFloat(math.Max(open, closePrice) + 0.5),
			Low:    decimal.NewFromFloat(math.Min(open, closePrice) - 0.5),
			Close:  decimal.NewFromFloat(closePrice),
			Volume: decimal.NewFromInt(1000),
			Begin:  start.AddDate(0, 0, i),
			End:    start.AddDate(0, 0, i),
		})
	}
	return candles
}

func preflightConfig() *config.Config {
	return &config.Config{
		Tickers:    []string{"SBER"},
		Risk:       config.Risk{MaxLots: 1},
		Commission: config.Commission{Rate: decimal.New(5, -4)},
		Preflight: config.Preflight{
			Enabled:   true,
			Days:      30,
			Deposit:   decimal.NewFromInt(100000),
			MinNetPnL: decimal.Zero,
		},
	}
}

func TestPreflightDisabledSkipsBacktest(t *testing.T) {
	history := &fakeHistorySource{err: fmt.Errorf("history must not be called")}
	cfg := preflightConfig()
	cfg.Preflight.Enabled = false

	p := newPreflight(cfg, fixedSignalSource{action: domain.ActionBuy}, history, time.Now)
	if err := p.check(context.Background()); err != nil {
		t.Fatalf("check() error = %v", err)
	}
	if history.calls != 0 {
		t.Fatalf("history calls = %d, want 0", history.calls)
	}
}

func TestNewPreflightNilConfigIsDisabled(t *testing.T) {
	p := newPreflight(nil, fixedSignalSource{action: domain.ActionBuy}, &fakeHistorySource{}, nil)
	if p.enabled {
		t.Fatal("newPreflight(nil) enabled = true, want false")
	}
	if err := p.check(context.Background()); err != nil {
		t.Fatalf("check() error = %v", err)
	}
}

func TestPreflightAppliesBorrowStress(t *testing.T) {
	p := newPreflight(preflightConfig(), fixedSignalSource{action: domain.ActionBuy}, &fakeHistorySource{}, time.Now)
	if !p.borrowPctPerDay.Equal(borrowcost.StressRatePerDay()) {
		t.Fatalf("preflight borrow rate = %s, want stress rate %s", p.borrowPctPerDay, borrowcost.StressRatePerDay())
	}
}

func TestPreflightPassesPositiveResult(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	history := &fakeHistorySource{candles: map[string][]moex.Candle{
		"SBER": testCandles(now.AddDate(0, 0, -250), 260, 0.5),
	}}
	p := newPreflight(preflightConfig(), fixedSignalSource{action: domain.ActionBuy}, history, func() time.Time { return now })

	if err := p.check(context.Background()); err != nil {
		t.Fatalf("check() error = %v", err)
	}
	if history.calls == 0 {
		t.Fatal("history was not called")
	}
}

func TestPreflightRejectsNegativeResult(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	history := &fakeHistorySource{candles: map[string][]moex.Candle{
		"SBER": testCandlesFrom(now.AddDate(0, 0, -250), 260, 200, -0.5),
	}}
	p := newPreflight(preflightConfig(), &alternatingSignalSource{}, history, func() time.Time { return now })

	err := p.check(context.Background())
	if err == nil {
		t.Fatal("check() error = nil, want rejection")
	}
	if !strings.Contains(err.Error(), "preflight rejected") {
		t.Fatalf("check() error = %v, want preflight rejection", err)
	}
	if !strings.Contains(err.Error(), "refusing to start") {
		t.Fatalf("check() error = %v, want refusing to start", err)
	}
}

func TestPreflightRespectsMinNetPnL(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	history := &fakeHistorySource{candles: map[string][]moex.Candle{
		"SBER": testCandles(now.AddDate(0, 0, -250), 260, 0.5),
	}}
	cfg := preflightConfig()
	cfg.Preflight.MinNetPnL = decimal.NewFromInt(1_000_000)
	p := newPreflight(cfg, fixedSignalSource{action: domain.ActionBuy}, history, func() time.Time { return now })

	err := p.check(context.Background())
	if err == nil || !strings.Contains(err.Error(), "preflight rejected") {
		t.Fatalf("check() error = %v, want rejection below min net pnl", err)
	}
}

func TestPreflightThresholdUsesRealizedPnlNotMTM(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	history := &fakeHistorySource{candles: map[string][]moex.Candle{
		"SBER": testCandles(now.AddDate(0, 0, -250), 260, 0.5),
	}}
	cfg := preflightConfig()
	cfg.Preflight.MinNetPnL = decimal.NewFromInt(1)
	cfg.Preflight.MinClosedTrades = 0
	p := newPreflight(cfg, fixedSignalSource{action: domain.ActionBuy}, history, func() time.Time { return now })

	err := p.check(context.Background())
	if err == nil {
		t.Fatal("check() error = nil, want realized-P&L rejection despite positive MTM")
	}
	if !strings.Contains(err.Error(), "realized P&L") {
		t.Fatalf("check() error = %v, want realized P&L rejection", err)
	}
}

func TestPreflightRequiresMinimumClosedTrades(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	history := &fakeHistorySource{candles: map[string][]moex.Candle{
		"SBER": testCandles(now.AddDate(0, 0, -250), 260, 0.5),
	}}
	cfg := preflightConfig()
	cfg.Preflight.MinClosedTrades = 1
	p := newPreflight(cfg, fixedSignalSource{action: domain.ActionBuy}, history, func() time.Time { return now })

	err := p.check(context.Background())
	if err == nil {
		t.Fatal("check() error = nil, want minimum-closed-trades rejection")
	}
	if !strings.Contains(err.Error(), "closed trades") {
		t.Fatalf("check() error = %v, want closed-trades rejection", err)
	}
}

func TestPreflightConfigHashChangesWithInputs(t *testing.T) {
	a := preflightConfigHash(preflightConfig())
	b := preflightConfigHash(preflightConfig())
	if a == "" {
		t.Fatal("preflightConfigHash() returned empty hash")
	}
	if a != b {
		t.Fatal("preflightConfigHash() is not deterministic for identical config")
	}

	changed := preflightConfig()
	changed.Tickers[0] = "LKOH"
	if preflightConfigHash(changed) == a {
		t.Fatal("preflightConfigHash() did not change after ticker change")
	}
}

func TestPreflightFailsWhenHistoryUnavailable(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	history := &fakeHistorySource{err: fmt.Errorf("iss is down")}
	p := newPreflight(preflightConfig(), fixedSignalSource{action: domain.ActionBuy}, history, func() time.Time { return now })

	err := p.check(context.Background())
	if err == nil {
		t.Fatal("check() error = nil, want no-decisions failure")
	}
	if !strings.Contains(err.Error(), "no decisions") {
		t.Fatalf("check() error = %v, want no decisions failure", err)
	}
}

type failingSignalSource struct {
	err error
}

func (f failingSignalSource) Generate(context.Context, domain.FeatureContext) (domain.TradeSignal, error) {
	return domain.TradeSignal{}, f.err
}

func TestPreflightRejectsWhenAllDecisionsFail(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	history := &fakeHistorySource{candles: map[string][]moex.Candle{
		"SBER": testCandles(now.AddDate(0, 0, -250), 260, 0.5),
	}}
	source := failingSignalSource{err: fmt.Errorf("ensemble: feature order has 18 fields, want 14")}
	p := newPreflight(preflightConfig(), source, history, func() time.Time { return now })

	err := p.check(context.Background())
	if err == nil {
		t.Fatal("check() error = nil, want rejection")
	}
	if !strings.Contains(err.Error(), "all") || !strings.Contains(err.Error(), "decisions failed") {
		t.Fatalf("check() error = %v, want all-decisions-failed rejection", err)
	}
}

func TestPreflightRequiresSources(t *testing.T) {
	p := newPreflight(preflightConfig(), nil, nil, time.Now)
	if err := p.check(context.Background()); err == nil {
		t.Fatal("check() error = nil, want missing source error")
	}
}

type recordingExecutor struct {
	calls []domain.TradeSignal
	err   error
}

func (r *recordingExecutor) Execute(_ context.Context, signal domain.TradeSignal, _ decimal.Decimal) (executor.Fill, error) {
	r.calls = append(r.calls, signal)
	if r.err != nil {
		return executor.Fill{}, r.err
	}
	return executor.Fill{ID: "fill", Ticker: signal.Ticker, Action: signal.Action, Lots: signal.TargetLots}, nil
}

func newSandboxWithTradingStatus(t *testing.T, status *pb.GetTradingStatusResponse, statusErr error) *brokertinkoff.Sandbox {
	t.Helper()
	client := &fakeSandboxClient{
		resolveUID: func(_ context.Context, ticker string) (string, error) { return "uid-" + ticker, nil },
		tradingStatus: func(context.Context, string) (*pb.GetTradingStatusResponse, error) {
			return status, statusErr
		},
	}
	sandbox, err := brokertinkoff.NewSandbox(client, brokertinkoff.Config{
		AccountID: "acc-1",
		PayIn:     decimal.RequireFromString("100000"),
		Now:       time.Now,
	})
	if err != nil {
		t.Fatalf("NewSandbox() error = %v", err)
	}
	return sandbox
}

func marketSignal(action domain.Action) domain.TradeSignal {
	return domain.TradeSignal{
		Ticker:      "SBER",
		Action:      action,
		Confidence:  decimal.RequireFromString("0.9"),
		TargetLots:  1,
		Reasoning:   "market hours test",
		GeneratedAt: time.Now(),
	}
}

func TestMarketHoursExecutorSkipsOrdersWhenClosed(t *testing.T) {
	status := &pb.GetTradingStatusResponse{
		TradingStatus:            pb.SecurityTradingStatus_SECURITY_TRADING_STATUS_NOT_AVAILABLE_FOR_TRADING,
		MarketOrderAvailableFlag: false,
		LimitOrderAvailableFlag:  false,
	}
	inner := &recordingExecutor{}
	exec, err := newMarketHoursExecutor(inner, newSandboxWithTradingStatus(t, status, nil), time.Now, log.Default())
	if err != nil {
		t.Fatalf("newMarketHoursExecutor() error = %v", err)
	}

	if _, err := exec.Execute(context.Background(), marketSignal(domain.ActionBuy), decimal.NewFromInt(100)); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(inner.calls) != 0 {
		t.Fatalf("inner executor calls = %d, want 0", len(inner.calls))
	}
}

func TestMarketHoursExecutorPlacesOrdersWhenOpen(t *testing.T) {
	status := &pb.GetTradingStatusResponse{MarketOrderAvailableFlag: true}
	inner := &recordingExecutor{}
	exec, err := newMarketHoursExecutor(inner, newSandboxWithTradingStatus(t, status, nil), time.Now, log.Default())
	if err != nil {
		t.Fatalf("newMarketHoursExecutor() error = %v", err)
	}

	if _, err := exec.Execute(context.Background(), marketSignal(domain.ActionBuy), decimal.NewFromInt(100)); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(inner.calls) != 1 {
		t.Fatalf("inner executor calls = %d, want 1", len(inner.calls))
	}
}

func TestMarketHoursExecutorPassesHoldThrough(t *testing.T) {
	inner := &recordingExecutor{}
	sandbox := newSandboxWithTradingStatus(t, nil, fmt.Errorf("trading status must not be called for HOLD"))
	exec, err := newMarketHoursExecutor(inner, sandbox, time.Now, log.Default())
	if err != nil {
		t.Fatalf("newMarketHoursExecutor() error = %v", err)
	}

	if _, err := exec.Execute(context.Background(), marketSignal(domain.ActionHold), decimal.NewFromInt(100)); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(inner.calls) != 1 {
		t.Fatalf("inner executor calls = %d, want 1", len(inner.calls))
	}
}

func TestMarketHoursExecutorPropagatesStatusError(t *testing.T) {
	statusErr := fmt.Errorf("trading status unavailable")
	inner := &recordingExecutor{}
	exec, err := newMarketHoursExecutor(inner, newSandboxWithTradingStatus(t, nil, statusErr), time.Now, log.Default())
	if err != nil {
		t.Fatalf("newMarketHoursExecutor() error = %v", err)
	}

	if _, err := exec.Execute(context.Background(), marketSignal(domain.ActionBuy), decimal.NewFromInt(100)); !errors.Is(err, statusErr) {
		t.Fatalf("Execute() error = %v, want %v", err, statusErr)
	}
	if len(inner.calls) != 0 {
		t.Fatalf("inner executor calls = %d, want 0", len(inner.calls))
	}
}

func TestMarketHoursExecutorLogsClosedOncePerDay(t *testing.T) {
	status := &pb.GetTradingStatusResponse{}
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	now := time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC)
	exec, err := newMarketHoursExecutor(&recordingExecutor{}, newSandboxWithTradingStatus(t, status, nil), func() time.Time { return now }, logger)
	if err != nil {
		t.Fatalf("newMarketHoursExecutor() error = %v", err)
	}

	for i := 0; i < 3; i++ {
		if _, err := exec.Execute(context.Background(), marketSignal(domain.ActionBuy), decimal.NewFromInt(100)); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	}
	if got := strings.Count(buf.String(), "not available for trading"); got != 1 {
		t.Fatalf("closed-market log lines = %d, want 1; log: %q", got, buf.String())
	}
}

func TestMarketHoursExecutorValidatesConfig(t *testing.T) {
	if _, err := newMarketHoursExecutor(nil, newSandboxWithTradingStatus(t, &pb.GetTradingStatusResponse{}, nil), time.Now, nil); err == nil {
		t.Fatal("newMarketHoursExecutor() error = nil for nil inner executor")
	}
	if _, err := newMarketHoursExecutor(&recordingExecutor{}, nil, time.Now, nil); err == nil {
		t.Fatal("newMarketHoursExecutor() error = nil for nil sandbox")
	}
}

func TestTradingWindowExecutorBlocksDuringOpeningCooldown(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Fatalf("LoadLocation() error = %v", err)
	}
	inner := &recordingExecutor{}
	now := time.Date(2026, 9, 17, 10, 5, 0, 0, loc)
	exec, err := newTradingWindowExecutor(inner, func() time.Time { return now }, 15, nil, log.Default())
	if err != nil {
		t.Fatalf("newTradingWindowExecutor() error = %v", err)
	}
	if _, err := exec.Execute(context.Background(), marketSignal(domain.ActionBuy), decimal.NewFromInt(100)); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(inner.calls) != 0 {
		t.Fatalf("inner executor calls = %d, want 0 (inside 15-minute opening cooldown)", len(inner.calls))
	}
}

func TestTradingWindowExecutorAllowsAfterCooldown(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Fatalf("LoadLocation() error = %v", err)
	}
	inner := &recordingExecutor{}
	now := time.Date(2026, 9, 17, 10, 16, 0, 0, loc)
	exec, err := newTradingWindowExecutor(inner, func() time.Time { return now }, 15, nil, log.Default())
	if err != nil {
		t.Fatalf("newTradingWindowExecutor() error = %v", err)
	}
	if _, err := exec.Execute(context.Background(), marketSignal(domain.ActionBuy), decimal.NewFromInt(100)); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(inner.calls) != 1 {
		t.Fatalf("inner executor calls = %d, want 1 (past the 15-minute opening cooldown)", len(inner.calls))
	}
}

func TestTradingWindowExecutorBlocksDuringBlackoutWindow(t *testing.T) {
	inner := &recordingExecutor{}
	now := time.Date(2026, 10, 24, 13, 30, 0, 0, time.UTC)
	exec, err := newTradingWindowExecutor(inner, func() time.Time { return now }, 0,
		[]string{"2026-10-24T13:00:00Z/2026-10-24T14:00:00Z"}, log.Default())
	if err != nil {
		t.Fatalf("newTradingWindowExecutor() error = %v", err)
	}
	if _, err := exec.Execute(context.Background(), marketSignal(domain.ActionSell), decimal.NewFromInt(100)); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(inner.calls) != 0 {
		t.Fatalf("inner executor calls = %d, want 0 (inside blackout window)", len(inner.calls))
	}
}

func TestTradingWindowExecutorPassesHoldThrough(t *testing.T) {
	inner := &recordingExecutor{}
	loc, _ := time.LoadLocation("Europe/Moscow")
	now := time.Date(2026, 9, 17, 10, 5, 0, 0, loc)
	exec, err := newTradingWindowExecutor(inner, func() time.Time { return now }, 15, nil, log.Default())
	if err != nil {
		t.Fatalf("newTradingWindowExecutor() error = %v", err)
	}
	if _, err := exec.Execute(context.Background(), marketSignal(domain.ActionHold), decimal.NewFromInt(100)); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(inner.calls) != 1 {
		t.Fatalf("inner executor calls = %d, want 1 (HOLD bypasses the trading window gate)", len(inner.calls))
	}
}

func TestTradingWindowExecutorRejectsInvalidBlackoutSpec(t *testing.T) {
	if _, err := newTradingWindowExecutor(&recordingExecutor{}, time.Now, 0, []string{"not-a-window"}, log.Default()); err == nil {
		t.Fatal("newTradingWindowExecutor() error = nil for invalid blackout spec")
	}
}

func TestOrderTypeFromConfig(t *testing.T) {
	tests := []struct {
		value string
		want  pb.OrderType
	}{
		{value: "", want: pb.OrderType_ORDER_TYPE_LIMIT},
		{value: config.OrderTypeLimit, want: pb.OrderType_ORDER_TYPE_LIMIT},
		{value: config.OrderTypeMarket, want: pb.OrderType_ORDER_TYPE_MARKET},
	}
	for _, tt := range tests {
		got, err := orderTypeFromConfig(tt.value)
		if err != nil {
			t.Fatalf("orderTypeFromConfig(%q) error = %v", tt.value, err)
		}
		if got != tt.want {
			t.Fatalf("orderTypeFromConfig(%q) = %s, want %s", tt.value, got, tt.want)
		}
	}
	if _, err := orderTypeFromConfig("stop"); err == nil {
		t.Fatal("orderTypeFromConfig() error = nil for unknown type")
	}
}

func TestNewModelSignalSourceWiresIntoOrchestrator(t *testing.T) {
	_, names := model.ToVector(domain.FeatureContext{})
	weights := &model.Weights{
		FeatureOrder:  names,
		Mean:          make([]float64, len(names)),
		Std:           make([]float64, len(names)),
		Coef:          make([]float64, len(names)),
		Bias:          0,
		BuyThreshold:  0.55,
		SellThreshold: 0.45,
		HorizonDays:   5,
		DeadbandPct:   0.5,
		TrainedAt:     time.Now(),
	}
	for i := range weights.Std {
		weights.Std[i] = 1
	}
	modelPath := filepath.Join(t.TempDir(), "model.json")
	if err := weights.Save(modelPath); err != nil {
		t.Fatalf("weights.Save() error = %v", err)
	}

	cfg := &config.Config{
		Model: config.Model{Path: modelPath},
		Risk:  config.Risk{MaxLots: 3},
	}
	source, err := newModelSignalSource(cfg)
	if err != nil {
		t.Fatalf("newModelSignalSource() error = %v", err)
	}
	if source == nil {
		t.Fatal("newModelSignalSource() returned nil source")
	}
	logisticSource, ok := source.(*model.SignalSource)
	if !ok {
		t.Fatalf("newModelSignalSource() returned %T, want *model.SignalSource", source)
	}
	if logisticSource.Weights == nil {
		t.Fatal("newModelSignalSource() returned nil weights")
	}
	if logisticSource.MaxLots != cfg.Risk.MaxLots {
		t.Fatalf("newModelSignalSource() max lots = %d, want %d", logisticSource.MaxLots, cfg.Risk.MaxLots)
	}

	_, err = orchestrator.New(orchestrator.Options{
		Tickers:  []string{"SBER"},
		Ingestor: traderTestIngestor{},
		Source:   source,
		Gate:     traderTestGate{},
		Executor: traderTestExecutor{},
		Audit:    traderTestAudit{},
	})
	if err != nil {
		t.Fatalf("orchestrator.New() error = %v", err)
	}
}

func TestNewModelSignalSourceWiresTargetNotionalForEnsemble(t *testing.T) {
	_, names := model.ToVector(domain.FeatureContext{})
	type logistic struct {
		Mean []float64 `json:"mean"`
		Std  []float64 `json:"std"`
		Coef []float64 `json:"coef"`
		Bias float64   `json:"bias"`
	}
	ensemble := struct {
		FeatureOrder  []string `json:"feature_order"`
		BuyThreshold  float64  `json:"buy_threshold"`
		SellThreshold float64  `json:"sell_threshold"`
		Logistic      logistic `json:"logistic"`
	}{
		FeatureOrder:  names,
		BuyThreshold:  0.6,
		SellThreshold: 0.4,
		Logistic: logistic{
			Mean: make([]float64, len(names)),
			Std:  make([]float64, len(names)),
			Coef: make([]float64, len(names)),
		},
	}
	raw, err := json.Marshal(ensemble)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	ensemblePath := filepath.Join(t.TempDir(), "ensemble_model.json")
	if err := os.WriteFile(ensemblePath, raw, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg := &config.Config{
		Model: config.Model{EnsemblePath: ensemblePath},
		Risk:  config.Risk{MaxLots: 100000, TargetNotional: decimal.NewFromInt(15000)},
	}
	source, err := newModelSignalSource(cfg)
	if err != nil {
		t.Fatalf("newModelSignalSource() error = %v", err)
	}
	ensembleSource, ok := source.(*model.EnsembleSignalSource)
	if !ok {
		t.Fatalf("newModelSignalSource() returned %T, want *model.EnsembleSignalSource", source)
	}
	if !ensembleSource.TargetNotional.Equal(cfg.Risk.TargetNotional) {
		t.Fatalf("newModelSignalSource() target notional = %s, want %s", ensembleSource.TargetNotional, cfg.Risk.TargetNotional)
	}
	if ensembleSource.MaxLots != cfg.Risk.MaxLots {
		t.Fatalf("newModelSignalSource() max lots = %d, want %d", ensembleSource.MaxLots, cfg.Risk.MaxLots)
	}
}

type fakeLotSizeResolver struct {
	lot decimal.Decimal
	err error
}

func (f fakeLotSizeResolver) ResolveLotSize(context.Context, string) (decimal.Decimal, error) {
	return f.lot, f.err
}

type featureCapturingSignalSource struct {
	feature domain.FeatureContext
}

func (s *featureCapturingSignalSource) Generate(_ context.Context, feature domain.FeatureContext) (domain.TradeSignal, error) {
	s.feature = feature
	return domain.TradeSignal{}, nil
}

func TestLotSizeSignalSourceFillsMissingLotSize(t *testing.T) {
	inner := &featureCapturingSignalSource{}
	source := newLotSizeSignalSource(inner, fakeLotSizeResolver{lot: decimal.NewFromInt(10)}, log.Default())

	if _, err := source.Generate(context.Background(), domain.FeatureContext{Ticker: "SBER"}); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if !inner.feature.LotSize.Equal(decimal.NewFromInt(10)) {
		t.Fatalf("LotSize = %s, want 10", inner.feature.LotSize)
	}
}

func TestLotSizeSignalSourceDoesNotOverwriteExistingLotSize(t *testing.T) {
	inner := &featureCapturingSignalSource{}
	resolver := fakeLotSizeResolver{lot: decimal.NewFromInt(10)}
	source := newLotSizeSignalSource(inner, resolver, log.Default())

	if _, err := source.Generate(context.Background(), domain.FeatureContext{Ticker: "SBER", LotSize: decimal.NewFromInt(1)}); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if !inner.feature.LotSize.Equal(decimal.NewFromInt(1)) {
		t.Fatalf("LotSize = %s, want unchanged 1", inner.feature.LotSize)
	}
}

func TestLotSizeSignalSourceContinuesOnResolveError(t *testing.T) {
	inner := &featureCapturingSignalSource{}
	source := newLotSizeSignalSource(inner, fakeLotSizeResolver{err: fmt.Errorf("resolve failed")}, log.Default())

	if _, err := source.Generate(context.Background(), domain.FeatureContext{Ticker: "SBER"}); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if inner.feature.LotSize.IsPositive() {
		t.Fatalf("LotSize = %s, want zero on resolve error", inner.feature.LotSize)
	}
}

type recordingSignalSource struct {
	signal domain.TradeSignal
	err    error
}

func (s recordingSignalSource) Generate(context.Context, domain.FeatureContext) (domain.TradeSignal, error) {
	return s.signal, s.err
}

type recordingAlerter struct {
	texts []string
	err   error
}

func (a *recordingAlerter) Send(_ context.Context, text string) error {
	a.texts = append(a.texts, text)
	return a.err
}

func TestAlertingSignalSourceAlertsOnFailure(t *testing.T) {
	generateErr := fmt.Errorf("model: computed probability is not finite for SBER")
	alerter := &recordingAlerter{}
	source := newAlertingSignalSource(recordingSignalSource{err: generateErr}, alerter, log.Default())

	_, err := source.Generate(context.Background(), domain.FeatureContext{Ticker: "SBER"})
	if err != generateErr {
		t.Fatalf("Generate() error = %v, want %v", err, generateErr)
	}
	if len(alerter.texts) != 1 {
		t.Fatalf("alert count = %d, want 1", len(alerter.texts))
	}
	if !strings.Contains(alerter.texts[0], "SBER") || !strings.Contains(alerter.texts[0], generateErr.Error()) {
		t.Fatalf("alert text = %q, want ticker and error", alerter.texts[0])
	}
}

func TestAlertingSignalSourceNoAlertOnSuccess(t *testing.T) {
	alerter := &recordingAlerter{}
	wantSignal := domain.TradeSignal{Ticker: "SBER", Action: domain.ActionBuy}
	source := newAlertingSignalSource(recordingSignalSource{signal: wantSignal}, alerter, log.Default())

	got, err := source.Generate(context.Background(), domain.FeatureContext{Ticker: "SBER"})
	if err != nil {
		t.Fatalf("Generate() error = %v, want nil", err)
	}
	if got != wantSignal {
		t.Fatalf("Generate() signal = %+v, want %+v", got, wantSignal)
	}
	if len(alerter.texts) != 0 {
		t.Fatalf("alert count = %d, want 0 for a signal without error", len(alerter.texts))
	}
}

func buyDecision(fillLots int) orchestrator.Decision {
	return orchestrator.Decision{
		Ticker:   "SBER",
		Price:    decimal.RequireFromString("270.5"),
		Approved: true,
		Signal: domain.TradeSignal{
			Ticker:     "SBER",
			Action:     domain.ActionBuy,
			Confidence: decimal.RequireFromString("0.8"),
			TargetLots: 1,
			Reasoning:  "ensemble:p=0.6234",
		},
		Fill: executor.Fill{Lots: fillLots, Price: decimal.RequireFromString("270.5")},
	}
}

func TestDecisionNotifierFilled(t *testing.T) {
	alerter := &recordingAlerter{}
	notifier := newDecisionNotifier(alerter, log.Default(), nil)

	notifier.Observe(context.Background(), buyDecision(1))

	if len(alerter.texts) != 1 {
		t.Fatalf("alert count = %d, want 1", len(alerter.texts))
	}
	text := alerter.texts[0]
	for _, want := range []string{"SBER → BUY", "270.5 ₽", "лоты: 1", "уверенность 0.80", "p=0.6234", "исполнено — 1 лот(а) @ 270.5"} {
		if !strings.Contains(text, want) {
			t.Fatalf("alert text = %q, want %q", text, want)
		}
	}
}

func TestDecisionNotifierRejected(t *testing.T) {
	alerter := &recordingAlerter{}
	notifier := newDecisionNotifier(alerter, log.Default(), nil)
	decision := buyDecision(0)
	decision.Approved = false

	notifier.Observe(context.Background(), decision)

	if len(alerter.texts) != 0 {
		t.Fatalf("alerts = %v, want none for an order that never went through (risk gate rejection)", alerter.texts)
	}
}

func TestDecisionNotifierSkippedWhenMarketClosed(t *testing.T) {
	alerter := &recordingAlerter{}
	notifier := newDecisionNotifier(alerter, log.Default(), nil)

	notifier.Observe(context.Background(), buyDecision(0))

	if len(alerter.texts) != 0 {
		t.Fatalf("alerts = %v, want none for an order skipped while the market is closed", alerter.texts)
	}
}

func TestDecisionNotifierError(t *testing.T) {
	alerter := &recordingAlerter{}
	notifier := newDecisionNotifier(alerter, log.Default(), nil)
	decision := buyDecision(0)
	decision.Err = fmt.Errorf("post order: rejected")

	notifier.Observe(context.Background(), decision)

	if len(alerter.texts) != 0 {
		t.Fatalf("alerts = %v, want none for an order that failed to execute", alerter.texts)
	}
}

func TestDecisionNotifierAlertsOnceOrderGoesThroughAfterFailing(t *testing.T) {
	alerter := &recordingAlerter{}
	notifier := newDecisionNotifier(alerter, log.Default(), nil)

	notifier.Observe(context.Background(), buyDecision(0))
	notifier.Observe(context.Background(), buyDecision(0))
	if len(alerter.texts) != 0 {
		t.Fatalf("alert count = %d, want 0 while the order keeps failing to go through", len(alerter.texts))
	}

	notifier.Observe(context.Background(), buyDecision(1))
	if len(alerter.texts) != 1 {
		t.Fatalf("alert count = %d, want 1 once the order is filled", len(alerter.texts))
	}
	if !strings.Contains(alerter.texts[0], "исполнено") {
		t.Fatalf("alert = %q, want filled status", alerter.texts[0])
	}
}

func TestDecisionNotifierDeduplicatesRepeatedFill(t *testing.T) {
	alerter := &recordingAlerter{}
	notifier := newDecisionNotifier(alerter, log.Default(), nil)

	notifier.Observe(context.Background(), buyDecision(1))
	notifier.Observe(context.Background(), buyDecision(1))
	if len(alerter.texts) != 1 {
		t.Fatalf("alert count = %d, want 1 for a repeated identical fill", len(alerter.texts))
	}
}

func TestDecisionNotifierSkipsHoldAndFiltersTickers(t *testing.T) {
	alerter := &recordingAlerter{}
	notifier := newDecisionNotifier(alerter, log.Default(), []string{"posi"})

	notifier.Observe(context.Background(), orchestrator.Decision{
		Ticker:   "SBER",
		Approved: true,
		Signal:   domain.TradeSignal{Ticker: "SBER", Action: domain.ActionHold},
	})
	notifier.Observe(context.Background(), buyDecision(1))
	if len(alerter.texts) != 0 {
		t.Fatalf("alerts = %v, want none for HOLD or filtered tickers", alerter.texts)
	}

	notifier.Observe(context.Background(), orchestrator.Decision{
		Ticker:   "POSI",
		Approved: true,
		Signal: domain.TradeSignal{
			Ticker:     "POSI",
			Action:     domain.ActionSell,
			Confidence: decimal.RequireFromString("0.7"),
			Reasoning:  "ensemble:p=0.31",
		},
		Fill: executor.Fill{Lots: 1, Price: decimal.RequireFromString("123.4")},
	})
	if len(alerter.texts) != 1 || !strings.Contains(alerter.texts[0], "POSI") {
		t.Fatalf("alerts = %v, want a POSI alert", alerter.texts)
	}
}

func TestNewTelegramHTTPClient(t *testing.T) {
	client, err := newTelegramHTTPClient("")
	if err != nil {
		t.Fatalf("newTelegramHTTPClient() error = %v", err)
	}
	if client != nil {
		t.Fatalf("newTelegramHTTPClient() = %v, want nil for empty proxy", client)
	}

	client, err = newTelegramHTTPClient("socks5://127.0.0.1:3333")
	if err != nil {
		t.Fatalf("newTelegramHTTPClient() error = %v", err)
	}
	if client == nil || client.Transport == nil {
		t.Fatal("newTelegramHTTPClient() did not configure a transport")
	}

	if _, err := newTelegramHTTPClient("://bad"); err == nil {
		t.Fatal("newTelegramHTTPClient() error = nil for invalid proxy URL")
	}
}

func TestNewModelSignalSourceMissingModelFails(t *testing.T) {
	cfg := &config.Config{
		Model: config.Model{Path: filepath.Join(t.TempDir(), "missing-model.json")},
	}
	_, err := newModelSignalSource(cfg)
	if err == nil {
		t.Fatal("newModelSignalSource() error = nil, want missing model failure")
	}
	if !strings.Contains(err.Error(), "load model") {
		t.Fatalf("newModelSignalSource() error = %v, want load model failure", err)
	}
}

func TestNewsGateVetoesBuyOnNegativeNews(t *testing.T) {
	newsCfg := config.News{
		VetoEnabled:   true,
		VetoSentiment: decimal.NewFromFloat(0.5),
		VetoMinCount:  1,
	}
	gate := newNewsGateSignalSource(fixedSignalSource{action: domain.ActionBuy}, newsCfg, nil, nil)
	signal, err := gate.Generate(context.Background(), domain.FeatureContext{
		Ticker:        "SBER",
		NewsSentiment: decimal.NewFromFloat(-0.8),
		NewsCount:     3,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if signal.Action != domain.ActionHold {
		t.Fatalf("action = %q, want HOLD (BUY vetoed)", signal.Action)
	}
	if signal.HoldReason != holdReasonNews {
		t.Fatalf("hold reason = %q, want %q", signal.HoldReason, holdReasonNews)
	}
	if signal.TargetLots != 0 {
		t.Fatalf("target lots = %d, want 0 after veto", signal.TargetLots)
	}
}

func TestNewsGateVetoesSellOnPositiveNews(t *testing.T) {
	newsCfg := config.News{
		VetoEnabled:   true,
		VetoSentiment: decimal.NewFromFloat(0.5),
		VetoMinCount:  1,
	}
	gate := newNewsGateSignalSource(fixedSignalSource{action: domain.ActionSell}, newsCfg, nil, nil)
	signal, err := gate.Generate(context.Background(), domain.FeatureContext{
		Ticker:        "SBER",
		NewsSentiment: decimal.NewFromFloat(0.9),
		NewsCount:     2,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if signal.Action != domain.ActionHold {
		t.Fatalf("action = %q, want HOLD (SELL vetoed)", signal.Action)
	}
	if signal.HoldReason != holdReasonNews {
		t.Fatalf("hold reason = %q, want %q", signal.HoldReason, holdReasonNews)
	}
}

func TestNewsGatePassesThroughWhenNoConflictingNews(t *testing.T) {
	newsCfg := config.News{
		VetoEnabled:   true,
		VetoSentiment: decimal.NewFromFloat(0.5),
		VetoMinCount:  1,
	}
	gate := newNewsGateSignalSource(fixedSignalSource{action: domain.ActionBuy}, newsCfg, nil, nil)
	signal, err := gate.Generate(context.Background(), domain.FeatureContext{
		Ticker:        "SBER",
		NewsSentiment: decimal.NewFromFloat(0.2),
		NewsCount:     1,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if signal.Action != domain.ActionBuy {
		t.Fatalf("action = %q, want BUY (no conflicting news)", signal.Action)
	}
}

func TestNewsGatePassesThroughWithoutNews(t *testing.T) {
	newsCfg := config.News{
		VetoEnabled:   true,
		VetoSentiment: decimal.NewFromFloat(0.5),
		VetoMinCount:  1,
	}
	gate := newNewsGateSignalSource(fixedSignalSource{action: domain.ActionBuy}, newsCfg, nil, nil)
	signal, err := gate.Generate(context.Background(), domain.FeatureContext{
		Ticker:        "SBER",
		NewsSentiment: decimal.NewFromFloat(-0.8),
		NewsCount:     0,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if signal.Action != domain.ActionBuy {
		t.Fatalf("action = %q, want BUY (no news present)", signal.Action)
	}
}

func TestNewsGateDisabled(t *testing.T) {
	newsCfg := config.News{
		VetoEnabled:   false,
		VetoSentiment: decimal.NewFromFloat(0.5),
		VetoMinCount:  1,
	}
	gate := newNewsGateSignalSource(fixedSignalSource{action: domain.ActionBuy}, newsCfg, nil, nil)
	signal, err := gate.Generate(context.Background(), domain.FeatureContext{
		Ticker:        "SBER",
		NewsSentiment: decimal.NewFromFloat(-0.8),
		NewsCount:     3,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if signal.Action != domain.ActionBuy {
		t.Fatalf("action = %q, want BUY (veto disabled)", signal.Action)
	}
}

func TestNewsGateRespectsMinCount(t *testing.T) {
	newsCfg := config.News{
		VetoEnabled:   true,
		VetoSentiment: decimal.NewFromFloat(0.5),
		VetoMinCount:  2,
	}
	gate := newNewsGateSignalSource(fixedSignalSource{action: domain.ActionBuy}, newsCfg, nil, nil)
	signal, err := gate.Generate(context.Background(), domain.FeatureContext{
		Ticker:        "SBER",
		NewsSentiment: decimal.NewFromFloat(-0.8),
		NewsCount:     1,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if signal.Action != domain.ActionBuy {
		t.Fatalf("action = %q, want BUY (below veto min count)", signal.Action)
	}
}

func writeDriftEnsembleModel(t *testing.T, names []string, coef []float64) string {
	t.Helper()
	std := make([]float64, len(names))
	for i := range std {
		std[i] = 1
	}
	ensemble := struct {
		FeatureOrder  []string `json:"feature_order"`
		BuyThreshold  float64  `json:"buy_threshold"`
		SellThreshold float64  `json:"sell_threshold"`
		LGBBaseLogit  float64  `json:"lgb_base_logit"`
		XGBBaseLogit  float64  `json:"xgb_base_logit"`
		Logistic      struct {
			Mean []float64 `json:"mean"`
			Std  []float64 `json:"std"`
			Coef []float64 `json:"coef"`
			Bias float64   `json:"bias"`
		} `json:"logistic"`
		LGBTrees []any `json:"lgb_trees"`
		XGBTrees []any `json:"xgb_trees"`
	}{
		FeatureOrder:  names,
		BuyThreshold:  0.6,
		SellThreshold: 0.4,
		Logistic: struct {
			Mean []float64 `json:"mean"`
			Std  []float64 `json:"std"`
			Coef []float64 `json:"coef"`
			Bias float64   `json:"bias"`
		}{Mean: make([]float64, len(names)), Std: std, Coef: coef},
		LGBTrees: []any{},
		XGBTrees: []any{},
	}
	raw, err := json.Marshal(ensemble)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "ensemble_model.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func TestNewDriftMonitorSkipsZeroCoefficientFeatures(t *testing.T) {
	path := writeDriftEnsembleModel(t, []string{"a", "b", "c", "d"}, []float64{1, 1, 0, 0})
	cfg := &config.Config{
		Model: config.Model{EnsemblePath: path},
		Risk:  config.Risk{DriftPSIThreshold: 0.1, DriftPSIWindow: 8},
	}
	monitor, err := newDriftMonitor(cfg, log.New(bytes.NewBuffer(nil), "", 0))
	if err != nil {
		t.Fatalf("newDriftMonitor() error = %v", err)
	}
	if monitor == nil {
		t.Fatal("newDriftMonitor() returned nil monitor")
	}

	for i := 0; i < 8; i++ {
		if got := monitor.Observe([]float64{10}, []string{"d"}); len(got) != 0 {
			t.Fatalf("Observe() zero-coef feature warnings = %d, want 0", len(got))
		}
	}

	var warnings []drift.Warning
	for i := 0; i < 8; i++ {
		warnings = append(warnings, monitor.Observe([]float64{10}, []string{"a"})...)
	}
	if len(warnings) != 1 {
		t.Fatalf("Observe() active-feature warnings = %d, want 1", len(warnings))
	}
}

func TestNewDriftMonitorReturnsNilWithoutEnsemblePath(t *testing.T) {
	cfg := &config.Config{Model: config.Model{Path: "model.json"}}
	monitor, err := newDriftMonitor(cfg, nil)
	if err != nil {
		t.Fatalf("newDriftMonitor() error = %v", err)
	}
	if monitor != nil {
		t.Fatalf("newDriftMonitor() = %v, want nil without ensemble path", monitor)
	}
}

func TestDriftSignalSourceObservesAndDelegates(t *testing.T) {
	var buf bytes.Buffer
	ref := map[string]drift.Distribution{"return_pct": drift.NormalDistribution(0, 1, 8)}
	monitor := drift.NewMonitor(ref, 0.1, 2, 2, log.New(&buf, "", 0))
	source := &driftSignalSource{source: fixedSignalSource{action: domain.ActionBuy}, monitor: monitor}

	feature := domain.FeatureContext{Ticker: "SBER", ReturnPct: decimal.NewFromInt(10)}
	for i := 0; i < 2; i++ {
		signal, err := source.Generate(context.Background(), feature)
		if err != nil {
			t.Fatalf("Generate() error = %v", err)
		}
		if signal.Action != domain.ActionBuy {
			t.Fatalf("Generate() action = %q, want BUY (delegated)", signal.Action)
		}
	}
	if !strings.Contains(buf.String(), "drift:") {
		t.Fatalf("drift monitor did not log a warning, got %q", buf.String())
	}
}

func TestBreakerFillObserverRecordsOnlyExecutedFills(t *testing.T) {
	breaker := risk.NewTickerBreaker(risk.TickerBreakerConfig{
		MaxConsecutiveLosses: 3,
		MaxCumulativeLossPct: decimal.NewFromFloat(0.05),
		Notional:             decimal.NewFromInt(1000),
	})
	observer := &breakerFillObserver{breaker: breaker, logger: log.New(bytes.NewBuffer(nil), "", 0)}

	observer.Observe(context.Background(), orchestrator.Decision{
		Approved: true,
		Fill: executor.Fill{
			Ticker: "SBER", Action: domain.ActionBuy, Lots: 1, Price: decimal.NewFromInt(100),
		},
	})
	if openLots, _, _, _, _ := breaker.State("SBER"); openLots != 1 {
		t.Fatalf("breaker openLots = %d, want 1 after approved fill", openLots)
	}

	observer.Observe(context.Background(), orchestrator.Decision{
		Approved: false,
		Fill: executor.Fill{
			Ticker: "SBER", Action: domain.ActionBuy, Lots: 1, Price: decimal.NewFromInt(100),
		},
	})
	observer.Observe(context.Background(), orchestrator.Decision{
		Approved: true,
		Err:      errors.New("rejected"),
		Fill: executor.Fill{
			Ticker: "SBER", Action: domain.ActionBuy, Lots: 1, Price: decimal.NewFromInt(100),
		},
	})
	observer.Observe(context.Background(), orchestrator.Decision{
		Approved: true,
		Fill: executor.Fill{
			Ticker: "SBER", Action: domain.ActionBuy, Lots: 0, Price: decimal.NewFromInt(100),
		},
	})
	if openLots, _, _, _, _ := breaker.State("SBER"); openLots != 1 {
		t.Fatalf("breaker openLots = %d after non-executed decisions, want 1", openLots)
	}
}

type fakeKillSwitchResetAccount struct {
	account     risk.Account
	maxNotional decimal.Decimal
	snapshotErr error
	notionalErr error
}

func (f *fakeKillSwitchResetAccount) Snapshot(context.Context) (risk.Account, error) {
	return f.account, f.snapshotErr
}

func (f *fakeKillSwitchResetAccount) MaxOpenPositionNotional(context.Context) (decimal.Decimal, error) {
	return f.maxNotional, f.notionalErr
}

func TestRunKillSwitchResetRefusesWhenEquityBelowThreshold(t *testing.T) {
	store := openTraderTestStore(t)
	ctx := context.Background()
	if err := store.SetKillSwitchActive(ctx, true); err != nil {
		t.Fatalf("SetKillSwitchActive(true) error = %v", err)
	}
	account := &fakeKillSwitchResetAccount{
		account:     risk.Account{CurrentEquity: decimal.NewFromInt(299)},
		maxNotional: decimal.NewFromInt(200),
	}
	runtime := &brokerRuntime{accountSource: account}

	if err := runKillSwitchReset(ctx, store, runtime); err == nil {
		t.Fatal("runKillSwitchReset() error = nil, want refusal")
	}
	active, err := store.IsKillSwitchActive(ctx)
	if err != nil {
		t.Fatalf("IsKillSwitchActive() error = %v", err)
	}
	if !active {
		t.Fatal("kill switch was reset despite equity below the 1.5x threshold")
	}
}

func TestRunKillSwitchResetAllowsWhenEquityMeetsThreshold(t *testing.T) {
	store := openTraderTestStore(t)
	ctx := context.Background()
	if err := store.SetKillSwitchActive(ctx, true); err != nil {
		t.Fatalf("SetKillSwitchActive(true) error = %v", err)
	}
	account := &fakeKillSwitchResetAccount{
		account:     risk.Account{CurrentEquity: decimal.NewFromInt(300)},
		maxNotional: decimal.NewFromInt(200),
	}
	runtime := &brokerRuntime{accountSource: account}

	if err := runKillSwitchReset(ctx, store, runtime); err != nil {
		t.Fatalf("runKillSwitchReset() error = %v", err)
	}
	active, err := store.IsKillSwitchActive(ctx)
	if err != nil {
		t.Fatalf("IsKillSwitchActive() error = %v", err)
	}
	if active {
		t.Fatal("kill switch still active after an allowed reset")
	}
}

func TestRunKillSwitchResetAllowsWithoutOpenPositions(t *testing.T) {
	store := openTraderTestStore(t)
	ctx := context.Background()
	if err := store.SetKillSwitchActive(ctx, true); err != nil {
		t.Fatalf("SetKillSwitchActive(true) error = %v", err)
	}
	account := &fakeKillSwitchResetAccount{account: risk.Account{CurrentEquity: decimal.NewFromInt(100)}}
	runtime := &brokerRuntime{accountSource: account}

	if err := runKillSwitchReset(ctx, store, runtime); err != nil {
		t.Fatalf("runKillSwitchReset() error = %v", err)
	}
	active, err := store.IsKillSwitchActive(ctx)
	if err != nil {
		t.Fatalf("IsKillSwitchActive() error = %v", err)
	}
	if active {
		t.Fatal("kill switch still active after reset with no open positions")
	}
}

func TestRunKillSwitchResetWithoutAccountSourceResetsForPaper(t *testing.T) {
	store := openTraderTestStore(t)
	ctx := context.Background()
	if err := store.SetKillSwitchActive(ctx, true); err != nil {
		t.Fatalf("SetKillSwitchActive(true) error = %v", err)
	}
	runtime := &brokerRuntime{}
	if err := runKillSwitchReset(ctx, store, runtime); err != nil {
		t.Fatalf("runKillSwitchReset() error = %v, want paper-mode reset", err)
	}
	active, err := store.IsKillSwitchActive(ctx)
	if err != nil {
		t.Fatalf("IsKillSwitchActive() error = %v", err)
	}
	if active {
		t.Fatal("kill switch still active after paper-mode reset")
	}
}

func TestLoadNewsPolarityEmptyPathFallsBackToLexicon(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)

	got := loadNewsPolarity(&config.Config{}, logger)
	if got != nil {
		t.Fatal("loadNewsPolarity() = non-nil scorer, want nil fallback for empty path")
	}
	if !strings.Contains(buf.String(), "falling back to keyword lexicon") {
		t.Fatalf("log output = %q, want fallback message", buf.String())
	}
}

func TestLoadNewsPolarityMissingFileFallsBackToLexicon(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	cfg := &config.Config{}
	cfg.News.ClassifierPath = filepath.Join(t.TempDir(), "missing.json")

	got := loadNewsPolarity(cfg, logger)
	if got != nil {
		t.Fatal("loadNewsPolarity() = non-nil scorer, want nil fallback for missing file")
	}
	if !strings.Contains(buf.String(), "falling back to keyword lexicon") {
		t.Fatalf("log output = %q, want fallback message", buf.String())
	}
}

func TestLoadNewsPolarityLoadsClassifierScorer(t *testing.T) {
	weights := &model.NewsClassifierWeights{
		Vocab:       []string{"abc"},
		Mean:        []float64{0},
		Std:         []float64{1},
		Coef:        []float64{0.5},
		Bias:        0,
		HorizonDays: 10,
		TrainedAt:   time.Now(),
	}
	path := filepath.Join(t.TempDir(), "news_classifier.json")
	if err := weights.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	cfg := &config.Config{}
	cfg.News.ClassifierPath = path

	scorer := loadNewsPolarity(cfg, logger)
	if scorer == nil {
		t.Fatal("loadNewsPolarity() = nil, want classifier scorer")
	}
	if !scorer("abc").IsPositive() {
		t.Fatalf("scorer(\"abc\") = %s, want positive", scorer("abc"))
	}
	if !strings.Contains(buf.String(), "loaded news classifier") {
		t.Fatalf("log output = %q, want loaded message", buf.String())
	}
}

func TestLoadNewsPolarityNilConfig(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	if got := loadNewsPolarity(nil, logger); got != nil {
		t.Fatal("loadNewsPolarity(nil) = non-nil scorer, want nil")
	}
}
