package risk

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

type Decision struct {
	Approved bool
	Reason   string
}

type Gate interface {
	Approve(ctx context.Context, request Request) (bool, error)
	ApproveReason(ctx context.Context, request Request) (Decision, error)
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

type PositionReader interface {
	CurrentLots(ctx context.Context, ticker string) (int, error)
}

type KillSwitchStore interface {
	IsKillSwitchActive(ctx context.Context) (bool, error)
	SetKillSwitchActive(ctx context.Context, active bool) error
}

type KillSwitchAlerter interface {
	KillSwitchTriggered(ctx context.Context, reason string) error
}

// TickerBlocker reports whether a single name has tripped its per-ticker
// circuit breaker and must not be traded.
type TickerBlocker interface {
	Blocked(ticker string) bool
}

type Config struct {
	MaxLots               int
	MaxDailyLossPct       decimal.Decimal
	FatFingerPct          decimal.Decimal
	MaxDrawdownPct        decimal.Decimal
	Canceller             OrderCanceller
	Positions             PositionReader
	Store                 KillSwitchStore
	Alerter               KillSwitchAlerter
	Breaker               TickerBlocker
	KillSwitchOnDailyLoss bool
}

type HardenedGate struct {
	maxLots               int
	maxDailyLossPct       decimal.Decimal
	fatFingerPct          decimal.Decimal
	maxDrawdownPct        decimal.Decimal
	canceller             OrderCanceller
	positions             PositionReader
	store                 KillSwitchStore
	alerter               KillSwitchAlerter
	breaker               TickerBlocker
	killSwitchOnDailyLoss bool

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
		maxLots:               cfg.MaxLots,
		maxDailyLossPct:       cfg.MaxDailyLossPct,
		fatFingerPct:          cfg.FatFingerPct,
		maxDrawdownPct:        cfg.MaxDrawdownPct,
		canceller:             cfg.Canceller,
		positions:             cfg.Positions,
		store:                 cfg.Store,
		alerter:               cfg.Alerter,
		breaker:               cfg.Breaker,
		killSwitchOnDailyLoss: cfg.KillSwitchOnDailyLoss,
	}, nil
}

func NewLotLimitGate(maxLots int) (*HardenedGate, error) {
	cfg := DefaultConfig()
	cfg.MaxLots = maxLots
	return NewHardenedGate(cfg)
}

func (g *HardenedGate) Approve(ctx context.Context, request Request) (bool, error) {
	decision, err := g.ApproveReason(ctx, request)
	return decision.Approved, err
}

func (g *HardenedGate) ApproveReason(ctx context.Context, request Request) (Decision, error) {
	if err := request.Signal.Validate(); err != nil {
		return Decision{}, fmt.Errorf("risk gate: invalid signal: %w", err)
	}
	if request.Signal.TargetLots < 0 {
		return Decision{}, fmt.Errorf("risk gate: target lots must be non-negative")
	}
	active, err := g.killSwitchActive(ctx)
	if err != nil {
		return Decision{}, fmt.Errorf("risk gate: read kill switch: %w", err)
	}
	if active {
		return Decision{Reason: "kill_switch_active"}, nil
	}
	if g.breaker != nil && g.breaker.Blocked(request.Signal.Ticker) {
		return Decision{Reason: "ticker_breaker_blocked"}, nil
	}
	if g.exceedsDrawdown(request.Account) {
		if err := g.triggerKillSwitch(ctx, "drawdown limit exceeded"); err != nil {
			return Decision{}, fmt.Errorf("risk gate: trigger kill switch: %w", err)
		}
		return Decision{Reason: "drawdown_limit"}, nil
	}
	currentLots := 0
	if g.positions != nil {
		var err error
		currentLots, err = g.positions.CurrentLots(ctx, request.Signal.Ticker)
		if err != nil {
			return Decision{}, fmt.Errorf("risk gate: read current position for %s: %w", request.Signal.Ticker, err)
		}
	}
	if request.Signal.TargetLots > g.maxLots {
		return Decision{Reason: "max_lots_exceeded"}, nil
	}
	if g.exceedsMaxPosition(request.Signal, currentLots) {
		return Decision{Reason: "max_position_exceeded"}, nil
	}
	if g.canceller != nil && (request.Signal.Action == domain.ActionBuy || request.Signal.Action == domain.ActionSell) && !g.hasAccountData(request.Account) {
		return Decision{}, fmt.Errorf("risk gate: live trading requires account deposit and equity data")
	}
	if request.Signal.Action == domain.ActionBuy || request.Signal.Action == domain.ActionSell {
		if request.Signal.TargetLots > 0 && !g.fatFingerOK(request.Market, request.Signal.Action) {
			return Decision{Reason: "fat_finger"}, nil
		}
	}
	if g.exceedsDailyLoss(request.Account) {
		if g.killSwitchOnDailyLoss {
			if err := g.triggerKillSwitch(ctx, "daily loss limit exceeded"); err != nil {
				return Decision{}, fmt.Errorf("risk gate: trigger kill switch: %w", err)
			}
		}
		return Decision{Reason: "daily_loss"}, nil
	}
	return Decision{Approved: true}, nil
}

func (g *HardenedGate) exceedsMaxPosition(signal domain.TradeSignal, _ int) bool {
	if signal.Action != domain.ActionBuy && signal.Action != domain.ActionSell {
		return false
	}
	desired := signedLots(signal.Action, signal.TargetLots)
	return absInt(desired) > g.maxLots
}

func signedLots(action domain.Action, lots int) int {
	if action == domain.ActionSell {
		return -lots
	}
	if action == domain.ActionBuy {
		return lots
	}
	return 0
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func (g *HardenedGate) fatFingerOK(market Market, action domain.Action) bool {
	if market.OrderPrice.Sign() <= 0 {
		return false
	}

	reference := market.PrevClose
	if action == domain.ActionBuy {
		reference = market.Ask
	} else if action == domain.ActionSell {
		reference = market.Bid
	}
	if reference.Sign() <= 0 {
		reference = market.PrevClose
	}
	if reference.Sign() <= 0 {
		return false
	}
	return g.withinFatFingerBand(market.OrderPrice, reference)
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
