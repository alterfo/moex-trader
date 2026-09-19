package tinkoff

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	pb "github.com/tinkoff/invest-api-go-sdk/proto"

	"github.com/olegsidorkin/moex-trader/internal/borrowcost"
	ingestion "github.com/olegsidorkin/moex-trader/internal/ingestion/tinkoff"
	"github.com/olegsidorkin/moex-trader/internal/risk"
)

type Client interface {
	ResolveInstrumentUID(ctx context.Context, ticker string) (string, error)
	SandboxAccounts(ctx context.Context) ([]*pb.Account, error)
	OpenSandboxAccount(ctx context.Context) (string, error)
	SandboxPayIn(ctx context.Context, accountID string, amount decimal.Decimal) (decimal.Decimal, error)
	PostSandboxOrder(ctx context.Context, request *pb.PostOrderRequest) (*pb.PostOrderResponse, error)
	GetSandboxPortfolio(ctx context.Context, accountID string) (*pb.PortfolioResponse, error)
	GetSandboxOrders(ctx context.Context, accountID string) ([]*pb.OrderState, error)
	SandboxOperations(ctx context.Context, accountID string, from, to time.Time) ([]*pb.Operation, error)
	CancelSandboxOrder(ctx context.Context, accountID, orderID string) error
	TradingStatus(ctx context.Context, instrumentID string) (*pb.GetTradingStatusResponse, error)
	ResolveLotSize(ctx context.Context, instrumentUID string) (int32, error)
	Close() error
}

type Config struct {
	AccountID string
	PayIn     decimal.Decimal
	Now       func() time.Time
}

type Sandbox struct {
	client    Client
	accountID string
	deposit   decimal.Decimal
	now       func() time.Time

	mu          sync.Mutex
	instruments map[string]string
	lotSizes    map[string]int32
	day         string
	dayStart    decimal.Decimal
}

func NewSandbox(client Client, cfg Config) (*Sandbox, error) {
	if client == nil {
		return nil, errors.New("sandbox: client is nil")
	}
	if !cfg.PayIn.IsPositive() {
		return nil, errors.New("sandbox: pay in must be positive")
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Sandbox{
		client:      client,
		accountID:   strings.TrimSpace(cfg.AccountID),
		deposit:     cfg.PayIn,
		now:         now,
		instruments: make(map[string]string),
		lotSizes:    make(map[string]int32),
	}, nil
}

func (s *Sandbox) AccountID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.accountID
}

func (s *Sandbox) EnsureAccount(ctx context.Context) (string, error) {
	configured := s.AccountID()
	accounts, err := s.client.SandboxAccounts(ctx)
	if err != nil {
		return "", err
	}
	if configured != "" {
		for _, account := range accounts {
			if strings.TrimSpace(account.GetId()) == configured {
				return configured, nil
			}
		}
		return "", fmt.Errorf("sandbox: configured account %q not found; clear tinkoff.account_id to open a new sandbox account", configured)
	}
	for _, account := range accounts {
		if accountID := strings.TrimSpace(account.GetId()); accountID != "" {
			s.setAccountID(accountID)
			return accountID, nil
		}
	}

	accountID, err := s.client.OpenSandboxAccount(ctx)
	if err != nil {
		return "", err
	}
	if _, err := s.client.SandboxPayIn(ctx, accountID, s.deposit); err != nil {
		return "", fmt.Errorf("sandbox: pay in account %q: %w", accountID, err)
	}
	s.setAccountID(accountID)
	return accountID, nil
}

func (s *Sandbox) setAccountID(accountID string) {
	s.mu.Lock()
	s.accountID = accountID
	s.mu.Unlock()
}

func (s *Sandbox) ResolveInstrumentID(ctx context.Context, ticker string) (string, error) {
	key := strings.ToUpper(strings.TrimSpace(ticker))
	if key == "" {
		return "", errors.New("sandbox: ticker must not be empty")
	}

	s.mu.Lock()
	uid, ok := s.instruments[key]
	s.mu.Unlock()
	if ok {
		return uid, nil
	}

	uid, err := s.client.ResolveInstrumentUID(ctx, key)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.instruments[key] = uid
	s.mu.Unlock()
	return uid, nil
}

// ResolveLotSize returns the exchange lot size for ticker (the number of
// shares per lot Tinkoff order quantities are denominated in) as a decimal
// suitable for notional-to-lots sizing math.
func (s *Sandbox) ResolveLotSize(ctx context.Context, ticker string) (decimal.Decimal, error) {
	uid, err := s.ResolveInstrumentID(ctx, ticker)
	if err != nil {
		return decimal.Zero, err
	}

	key := strings.ToUpper(strings.TrimSpace(ticker))
	s.mu.Lock()
	lot, ok := s.lotSizes[key]
	s.mu.Unlock()
	if ok {
		return decimal.NewFromInt32(lot), nil
	}

	lot, err = s.client.ResolveLotSize(ctx, uid)
	if err != nil {
		return decimal.Zero, err
	}
	s.mu.Lock()
	s.lotSizes[key] = lot
	s.mu.Unlock()
	return decimal.NewFromInt32(lot), nil
}

func (s *Sandbox) MarketOpen(ctx context.Context, ticker string) (bool, error) {
	uid, err := s.ResolveInstrumentID(ctx, ticker)
	if err != nil {
		return false, err
	}
	status, err := s.client.TradingStatus(ctx, uid)
	if err != nil {
		return false, err
	}
	return status.GetMarketOrderAvailableFlag() || status.GetLimitOrderAvailableFlag(), nil
}

// MarginFees returns the actual short-borrow charges observed in the sandbox
// operation history between from and to, summed over margin-fee operations.
// This is the live counterpart to the Task 11 stress rate and is logged
// periodically by the trader so the two can be cross-referenced.
func (s *Sandbox) MarginFees(ctx context.Context, from, to time.Time) (decimal.Decimal, int, error) {
	accountID := s.AccountID()
	if accountID == "" {
		return decimal.Zero, 0, errors.New("sandbox: account is not initialized")
	}
	operations, err := s.client.SandboxOperations(ctx, accountID, from, to)
	if err != nil {
		return decimal.Zero, 0, err
	}
	fees, count := borrowcost.SumMarginFees(operations)
	return fees, count, nil
}

func (s *Sandbox) PostOrder(ctx context.Context, request *pb.PostOrderRequest) (*pb.PostOrderResponse, error) {
	if request == nil {
		return nil, errors.New("sandbox: post order request is nil")
	}
	accountID := s.AccountID()
	if accountID == "" {
		return nil, errors.New("sandbox: account is not initialized")
	}
	if request.GetAccountId() != accountID {
		return nil, fmt.Errorf("sandbox: order account %q does not match sandbox account %q", request.GetAccountId(), accountID)
	}
	return s.client.PostSandboxOrder(ctx, request)
}

func (s *Sandbox) Snapshot(ctx context.Context) (risk.Account, error) {
	accountID := s.AccountID()
	if accountID == "" {
		return risk.Account{}, errors.New("sandbox: account is not initialized")
	}
	portfolio, err := s.client.GetSandboxPortfolio(ctx, accountID)
	if err != nil {
		return risk.Account{}, err
	}
	equity, err := ingestion.MoneyValueToDecimal(portfolio.GetTotalAmountPortfolio())
	if err != nil {
		return risk.Account{}, fmt.Errorf("sandbox: portfolio equity: %w", err)
	}
	if equity.Sign() <= 0 {
		return risk.Account{}, fmt.Errorf("sandbox: portfolio equity %s is not positive", equity)
	}

	day := s.now().Format("2006-01-02")
	s.mu.Lock()
	if s.day != day {
		s.day = day
		s.dayStart = equity
	}
	dayStart := s.dayStart
	s.mu.Unlock()

	return risk.Account{
		Deposit:        s.deposit,
		DayStartEquity: dayStart,
		CurrentEquity:  equity,
	}, nil
}

func (s *Sandbox) PositionLots(ctx context.Context, tickers []string) (map[string]int, error) {
	accountID := s.AccountID()
	if accountID == "" {
		return nil, errors.New("sandbox: account is not initialized")
	}
	portfolio, err := s.client.GetSandboxPortfolio(ctx, accountID)
	if err != nil {
		return nil, err
	}
	byUID := make(map[string]*pb.PortfolioPosition, len(portfolio.GetPositions()))
	for _, position := range portfolio.GetPositions() {
		uid := strings.TrimSpace(position.GetInstrumentUid())
		if uid == "" {
			continue
		}
		byUID[uid] = position
	}
	lots := make(map[string]int, len(tickers))
	for _, ticker := range tickers {
		key := strings.ToUpper(strings.TrimSpace(ticker))
		if key == "" {
			continue
		}
		uid, err := s.ResolveInstrumentID(ctx, key)
		if err != nil {
			return nil, err
		}
		count, err := s.positionLots(ctx, key, byUID[uid])
		if err != nil {
			return nil, err
		}
		lots[key] = count
	}
	return lots, nil
}

func (s *Sandbox) positionLots(ctx context.Context, ticker string, position *pb.PortfolioPosition) (int, error) {
	if position == nil {
		return 0, nil
	}
	if quantityLots := position.GetQuantityLots(); quantityLots != nil {
		count, err := ingestion.QuotationToDecimal(quantityLots)
		if err != nil {
			return 0, fmt.Errorf("sandbox: position lots for %s: %w", ticker, err)
		}
		return int(count.Round(0).IntPart()), nil
	}
	quantity, err := ingestion.QuotationToDecimal(position.GetQuantity())
	if err != nil {
		return 0, fmt.Errorf("sandbox: position quantity for %s: %w", ticker, err)
	}
	if quantity.IsZero() {
		return 0, nil
	}
	lot, err := s.ResolveLotSize(ctx, ticker)
	if err != nil {
		return 0, err
	}
	if !lot.IsPositive() {
		return 0, fmt.Errorf("sandbox: lot size for %s is not positive", ticker)
	}
	return int(quantity.Div(lot).Round(0).IntPart()), nil
}

// MaxOpenPositionNotional returns the largest absolute notional across the
// account's open positions, computed as quantity times current price for each
// position. An account with no open positions returns zero.
func (s *Sandbox) MaxOpenPositionNotional(ctx context.Context) (decimal.Decimal, error) {
	accountID := s.AccountID()
	if accountID == "" {
		return decimal.Zero, errors.New("sandbox: account is not initialized")
	}
	portfolio, err := s.client.GetSandboxPortfolio(ctx, accountID)
	if err != nil {
		return decimal.Zero, err
	}
	maxNotional := decimal.Zero
	for _, position := range portfolio.GetPositions() {
		quantity, err := ingestion.QuotationToDecimal(position.GetQuantity())
		if err != nil {
			return decimal.Zero, fmt.Errorf("sandbox: position quantity: %w", err)
		}
		price, err := ingestion.MoneyValueToDecimal(position.GetCurrentPrice())
		if err != nil {
			return decimal.Zero, fmt.Errorf("sandbox: position price: %w", err)
		}
		notional := quantity.Mul(price).Abs()
		if notional.GreaterThan(maxNotional) {
			maxNotional = notional
		}
	}
	return maxNotional, nil
}

func (s *Sandbox) CancelOpenOrders(ctx context.Context) error {
	accountID := s.AccountID()
	if accountID == "" {
		return errors.New("sandbox: account is not initialized")
	}
	orders, err := s.client.GetSandboxOrders(ctx, accountID)
	if err != nil {
		return err
	}
	owned := s.knownInstrumentUIDs()
	var errs []error
	for _, order := range orders {
		orderID := strings.TrimSpace(order.GetOrderId())
		if orderID == "" {
			continue
		}
		if _, ok := owned[order.GetInstrumentUid()]; !ok {
			continue
		}
		if err := s.client.CancelSandboxOrder(ctx, accountID, orderID); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *Sandbox) knownInstrumentUIDs() map[string]struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	uids := make(map[string]struct{}, len(s.instruments))
	for _, uid := range s.instruments {
		uids[uid] = struct{}{}
	}
	return uids
}

func (s *Sandbox) Close() error {
	if s.client == nil {
		return nil
	}
	return s.client.Close()
}
