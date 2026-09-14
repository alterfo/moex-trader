package risk

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

type Gate interface {
	Approve(ctx context.Context, request Request) (bool, error)
}

type Market struct {
	OrderPrice decimal.Decimal
	Bid        decimal.Decimal
	Ask        decimal.Decimal
	PrevClose  decimal.Decimal
}

type Account struct {
	Deposit        decimal.Decimal
	DayStartEquity decimal.Decimal
	CurrentEquity  decimal.Decimal
}

type Request struct {
	Signal  domain.TradeSignal
	Market  Market
	Account Account
}

type OrderCanceller interface {
	CancelOpenOrders(ctx context.Context) error
}

type KillSwitchStore interface {
	IsKillSwitchActive(ctx context.Context) (bool, error)
	SetKillSwitchActive(ctx context.Context, active bool) error
}

type KillSwitchAlerter interface {
	KillSwitchTriggered(ctx context.Context, reason string) error
}

type Config struct {
	MaxLots         int
	MaxDailyLossPct decimal.Decimal
	FatFingerPct    decimal.Decimal
	MaxDrawdownPct  decimal.Decimal
	Canceller       OrderCanceller
	Store           KillSwitchStore
	Alerter         KillSwitchAlerter
}

type HardenedGate struct {
	maxLots         int
	maxDailyLossPct decimal.Decimal
	fatFingerPct    decimal.Decimal
	maxDrawdownPct  decimal.Decimal
	canceller       OrderCanceller
	store           KillSwitchStore
	alerter         KillSwitchAlerter

	mu         sync.RWMutex
	killSwitch bool
}

func DefaultConfig() Config {
	return Config{
		MaxLots:         1,
		MaxDailyLossPct: decimal.RequireFromString("0.5"),
		FatFingerPct:    decimal.RequireFromString("2"),
		MaxDrawdownPct:  decimal.RequireFromString("3"),
	}
}

func NewHardenedGate(cfg Config) (*HardenedGate, error) {
	if cfg.MaxLots <= 0 {
		return nil, fmt.Errorf("risk gate: max lots must be positive")
	}
	if cfg.MaxDailyLossPct.Sign() <= 0 {
		return nil, fmt.Errorf("risk gate: max daily loss percent must be positive")
	}
	if cfg.FatFingerPct.Sign() <= 0 {
		return nil, fmt.Errorf("risk gate: fat finger percent must be positive")
	}
	if cfg.MaxDrawdownPct.Sign() <= 0 {
		return nil, fmt.Errorf("risk gate: max drawdown percent must be positive")
	}
	return &HardenedGate{
		maxLots:         cfg.MaxLots,
		maxDailyLossPct: cfg.MaxDailyLossPct,
		fatFingerPct:    cfg.FatFingerPct,
		maxDrawdownPct:  cfg.MaxDrawdownPct,
		canceller:       cfg.Canceller,
		store:           cfg.Store,
		alerter:         cfg.Alerter,
	}, nil
}

func NewLotLimitGate(maxLots int) (*HardenedGate, error) {
	cfg := DefaultConfig()
	cfg.MaxLots = maxLots
	return NewHardenedGate(cfg)
}

func (g *HardenedGate) Approve(ctx context.Context, request Request) (bool, error) {
	if err := request.Signal.Validate(); err != nil {
		return false, fmt.Errorf("risk gate: invalid signal: %w", err)
	}
	if request.Signal.TargetLots < 0 {
		return false, fmt.Errorf("risk gate: target lots must be non-negative")
	}
	active, err := g.killSwitchActive(ctx)
	if err != nil {
		return false, fmt.Errorf("risk gate: read kill switch: %w", err)
	}
	if active {
		return false, nil
	}
	if g.canceller != nil && (request.Signal.Action == domain.ActionBuy || request.Signal.Action == domain.ActionSell) && !g.hasAccountData(request.Account) {
		return false, fmt.Errorf("risk gate: live trading requires account deposit and equity data")
	}

	if g.exceedsDrawdown(request.Account) {
		if err := g.triggerKillSwitch(ctx, "drawdown limit exceeded"); err != nil {
			return false, fmt.Errorf("risk gate: trigger kill switch: %w", err)
		}
		return false, nil
	}
	if request.Signal.TargetLots > g.maxLots {
		return false, nil
	}
	if request.Signal.Action == domain.ActionBuy || request.Signal.Action == domain.ActionSell {
		if request.Signal.TargetLots > 0 && !g.fatFingerOK(request.Market) {
			return false, nil
		}
	}
	if g.exceedsDailyLoss(request.Account) {
		return false, nil
	}
	return true, nil
}

func (g *HardenedGate) fatFingerOK(market Market) bool {
	if market.OrderPrice.Sign() <= 0 {
		return false
	}
	if market.Bid.Sign() <= 0 && market.Ask.Sign() <= 0 {
		if market.PrevClose.Sign() <= 0 {
			return false
		}
		return g.withinFatFingerBand(market.OrderPrice, market.PrevClose)
	}
	if market.Bid.Sign() > 0 && !g.withinFatFingerBand(market.OrderPrice, market.Bid) {
		return false
	}
	if market.Ask.Sign() > 0 && !g.withinFatFingerBand(market.OrderPrice, market.Ask) {
		return false
	}
	return true
}

func (g *HardenedGate) withinFatFingerBand(orderPrice, reference decimal.Decimal) bool {
	deviation := orderPrice.Sub(reference).Abs().Div(reference).Mul(decimal.NewFromInt(100))
	return !deviation.GreaterThan(g.fatFingerPct)
}

func (g *HardenedGate) exceedsDailyLoss(account Account) bool {
	if account.Deposit.Sign() <= 0 || account.DayStartEquity.Sign() <= 0 {
		return false
	}
	loss := account.DayStartEquity.Sub(account.CurrentEquity)
	if loss.Sign() <= 0 {
		return false
	}
	limit := account.Deposit.Mul(g.maxDailyLossPct).Div(decimal.NewFromInt(100))
	return loss.GreaterThan(limit)
}

func (g *HardenedGate) exceedsDrawdown(account Account) bool {
	if account.Deposit.Sign() <= 0 {
		return false
	}
	drawdown := account.Deposit.Sub(account.CurrentEquity).Div(account.Deposit).Mul(decimal.NewFromInt(100))
	return drawdown.GreaterThan(g.maxDrawdownPct)
}

func (g *HardenedGate) triggerKillSwitch(ctx context.Context, reason string) error {
	g.mu.Lock()
	if g.killSwitch {
		g.mu.Unlock()
		return nil
	}
	g.killSwitch = true
	canceller := g.canceller
	store := g.store
	alerter := g.alerter
	g.mu.Unlock()

	var errs []error
	if store != nil {
		if err := store.SetKillSwitchActive(ctx, true); err != nil {
			errs = append(errs, fmt.Errorf("risk gate: persist kill switch: %w", err))
		}
	}
	if canceller != nil {
		if err := canceller.CancelOpenOrders(ctx); err != nil {
			errs = append(errs, fmt.Errorf("risk gate: cancel open orders: %w", err))
		}
	}
	if alerter != nil {
		if err := alerter.KillSwitchTriggered(ctx, reason); err != nil {
			errs = append(errs, fmt.Errorf("risk gate: alert kill switch: %w", err))
		}
	}
	return errors.Join(errs...)
}

func (g *HardenedGate) hasAccountData(account Account) bool {
	return account.Deposit.Sign() > 0 && account.DayStartEquity.Sign() > 0 && account.CurrentEquity.Sign() > 0
}

func (g *HardenedGate) killSwitchActive(ctx context.Context) (bool, error) {
	g.mu.RLock()
	local := g.killSwitch
	g.mu.RUnlock()
	if local {
		return true, nil
	}
	if g.store != nil {
		active, err := g.store.IsKillSwitchActive(ctx)
		if err != nil {
			return false, err
		}
		if active {
			g.mu.Lock()
			g.killSwitch = true
			g.mu.Unlock()
		}
		return active, nil
	}
	return false, nil
}

func (g *HardenedGate) TripKillSwitch(ctx context.Context) error {
	return g.triggerKillSwitch(ctx, "manual trigger")
}

func (g *HardenedGate) ResetKillSwitch() {
	g.mu.Lock()
	g.killSwitch = false
	g.mu.Unlock()
}

func (g *HardenedGate) ResetKillSwitchContext(ctx context.Context) error {
	g.mu.Lock()
	g.killSwitch = false
	store := g.store
	g.mu.Unlock()
	if store == nil {
		return nil
	}
	return store.SetKillSwitchActive(ctx, false)
}

func (g *HardenedGate) IsKillSwitchActive() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.killSwitch
}
