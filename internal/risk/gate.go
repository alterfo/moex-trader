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
	// LotSize is the number of underlying shares per 1 lot for the ticker the
	// order is about. 1 when the engine/subsystem works in share-sized units
	// (backtest), the real exchange lot size in live execution.
	LotSize decimal.Decimal
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

	// ExposureDeltaLots is the signed lot change THIS order applies to its own
	// ticker, positive = buy. It is separate from Signal.TargetLots because
	// the target-position path carries the absolute target position there while
	// incremental callers carry the order size. Exposure must be projected
	// from the delta: reading the absolute target as a delta double-counts the
	// position already held, and under a gross cap that makes the order which
	// flattens a position look like a fresh full-size entry. Nil means
	// Signal.TargetLots is the order size (incremental callers).
	ExposureDeltaLots *int
}

type OrderCanceller interface {
	CancelOpenOrders(ctx context.Context) error
}

type PositionReader interface {
	CurrentLots(ctx context.Context, ticker string) (int, error)
}

// ExposureReader reports the signed position notional in RUB per ticker, keyed
// by upper-cased ticker. Pricing basis is the reader's own: the backtest engine
// uses average entry price, the live fill ledger average cost. A ticker absent
// from the map is flat. One snapshot backs both the gross book size and the
// per-ticker projection, so the two never disagree on basis.
type ExposureReader interface {
	ExposureByTicker(ctx context.Context) (map[string]decimal.Decimal, error)
}

// NetExposureReader reports the signed aggregate net position notional in RUB
// across the whole book (sum over open positions of signed lots * price).
// Positive = net long, negative = net short.
//
// Kept as a fallback for readers that predate ExposureReader. It cannot bound
// risk on its own: longs and shorts cancel, so a book long 500k and short 500k
// reads as zero. Prefer ExposureReader for the cap.
type NetExposureReader interface {
	NetExposure(ctx context.Context) (decimal.Decimal, error)
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
	MaxLots         int
	MaxDailyLossPct decimal.Decimal
	FatFingerPct    decimal.Decimal
	MaxDrawdownPct  decimal.Decimal
	// MaxNetExposure caps the book's GROSS position notional (sum of |net|
	// per ticker), not the signed net. 0 disables the cap. Kept under its
	// historical name for config compatibility; see exceedsMaxNetExposure.
	MaxNetExposure decimal.Decimal
	// SectorCaps caps gross position notional per sector (sum of |net|
	// across tickers assigned to that sector). A sector absent from the map is
	// uncapped; a present zero cap allows only flatten-to-flat orders in that
	// sector. Sectors maps an upper-cased ticker to its sector name.
	SectorCaps            map[string]decimal.Decimal
	Sectors               map[string]string
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
	maxNetExposure        decimal.Decimal
	sectorCaps            map[string]decimal.Decimal
	sectors               map[string]string
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
	if cfg.MaxNetExposure.IsNegative() {
		return nil, fmt.Errorf("risk gate: max net exposure must be non-negative")
	}
	for sector, cap := range cfg.SectorCaps {
		if cap.IsNegative() {
			return nil, fmt.Errorf("risk gate: sector cap %q must be non-negative", sector)
		}
	}
	return &HardenedGate{
		maxLots:               cfg.MaxLots,
		maxDailyLossPct:       cfg.MaxDailyLossPct,
		fatFingerPct:          cfg.FatFingerPct,
		maxDrawdownPct:        cfg.MaxDrawdownPct,
		maxNetExposure:        cfg.MaxNetExposure,
		sectorCaps:            cloneSectorCaps(cfg.SectorCaps),
		sectors:               cloneSectors(cfg.Sectors),
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
	if g.exceedsMaxNetExposure(ctx, request) {
		return Decision{Reason: "max_net_exposure_exceeded"}, nil
	}
	if g.exceedsSectorCap(ctx, request) {
		return Decision{Reason: "sector_cap"}, nil
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

// exceedsMaxNetExposure rejects orders that would push the book's exposure
// beyond MaxNetExposure. 0 disables the cap.
//
// The bound is GROSS exposure — the sum over tickers of |signed position
// notional| — not signed net. Net cannot bound risk: a long 500k and a short
// 500k net to zero and would sail past a 60k cap while carrying 1M of gross
// risk, purely because the two sides happened to be equal size. Gross also
// removes the cancellation-driven order dependence: under a net cap, which
// names got filled depended on how much opposing exposure happened to be
// booked first, not on signal quality.
//
// Projection is per ticker, not book-wide addition: this order changes one
// name, so only that name's leg is replaced. Treating every order as purely
// additive to gross would make the cap block the very orders that REDUCE risk
// (closing or trimming a position), which would strand open positions. What is
// checked is |other names| + |this name after the order|.
//
// A reader exposing only the legacy NetExposureReader still works, with the
// weaker cancelling semantics retained for compatibility; it cannot see
// per-ticker legs and so stays conservative on reductions. A configured cap
// fails closed when neither is available, because an unverifiable exposure
// bound must not silently allow an order.
func (g *HardenedGate) exceedsMaxNetExposure(ctx context.Context, request Request) bool {
	if g.maxNetExposure.Sign() <= 0 {
		return false
	}
	if request.Signal.Action != domain.ActionBuy && request.Signal.Action != domain.ActionSell {
		return false
	}
	if request.Signal.TargetLots == 0 || request.Market.OrderPrice.Sign() <= 0 {
		return false
	}
	deltaLots := signedLots(request.Signal.Action, request.Signal.TargetLots)
	if request.ExposureDeltaLots != nil {
		deltaLots = *request.ExposureDeltaLots
	}
	if deltaLots == 0 {
		return false
	}
	delta := decimal.NewFromInt(int64(deltaLots))
	lotSize := request.Market.LotSize
	if lotSize.IsZero() {
		lotSize = decimal.NewFromInt(1)
	}
	delta = delta.Mul(lotSize).Mul(request.Market.OrderPrice)

	reader, ok := g.positions.(ExposureReader)
	if !ok {
		return g.exceedsMaxNetExposureLegacy(ctx, delta)
	}
	book, err := reader.ExposureByTicker(ctx)
	if err != nil {
		return true
	}
	ticker := normalizeTicker(request.Signal.Ticker)
	thisLeg := book[ticker]
	others := decimal.Zero
	for name, notional := range book {
		if name == ticker {
			continue
		}
		others = others.Add(notional.Abs())
	}
	projected := others.Add(thisLeg.Add(delta).Abs())
	return projected.GreaterThan(g.maxNetExposure)
}

// exceedsMaxNetExposureLegacy keeps the cap working for readers that only
// expose an aggregate. Without per-ticker legs it cannot tell an exit from an
// entry, so it adds the order to the signed book and takes the absolute value —
// the original behaviour, conservative on reductions.
func (g *HardenedGate) exceedsMaxNetExposureLegacy(ctx context.Context, delta decimal.Decimal) bool {
	reader, ok := g.positions.(NetExposureReader)
	if !ok {
		return true
	}
	current, err := reader.NetExposure(ctx)
	if err != nil {
		return true
	}
	return current.Add(delta).Abs().GreaterThan(g.maxNetExposure)
}

// exceedsSectorCap rejects orders that would push a sector's gross position
// notional beyond its cap. The projection is per ticker, exactly like
// exceedsMaxNetExposure: this order changes one name, so only that name's leg
// is replaced, and the other names in the same sector are summed by |notional|.
// A ticker without a sector, or a sector without a cap, is never blocked. A
// cap of zero allows only flatten-to-flat orders in that sector. A configured
// cap fails closed when per-ticker exposure is unavailable.
func (g *HardenedGate) exceedsSectorCap(ctx context.Context, request Request) bool {
	if len(g.sectorCaps) == 0 {
		return false
	}
	if request.Signal.Action != domain.ActionBuy && request.Signal.Action != domain.ActionSell {
		return false
	}
	if request.Signal.TargetLots == 0 || request.Market.OrderPrice.Sign() <= 0 {
		return false
	}
	ticker := normalizeTicker(request.Signal.Ticker)
	sector, assigned := g.sectors[ticker]
	if !assigned || sector == "" {
		return false
	}
	cap, capped := g.sectorCaps[sector]
	if !capped {
		return false
	}
	deltaLots := signedLots(request.Signal.Action, request.Signal.TargetLots)
	if request.ExposureDeltaLots != nil {
		deltaLots = *request.ExposureDeltaLots
	}
	if deltaLots == 0 {
		return false
	}
	delta := decimal.NewFromInt(int64(deltaLots))
	lotSize := request.Market.LotSize
	if lotSize.IsZero() {
		lotSize = decimal.NewFromInt(1)
	}
	delta = delta.Mul(lotSize).Mul(request.Market.OrderPrice)

	reader, ok := g.positions.(ExposureReader)
	if !ok {
		return true
	}
	book, err := reader.ExposureByTicker(ctx)
	if err != nil {
		return true
	}
	thisLeg := book[ticker]
	others := decimal.Zero
	for name, notional := range book {
		if name == ticker {
			continue
		}
		if g.sectors[name] != sector {
			continue
		}
		others = others.Add(notional.Abs())
	}
	projected := others.Add(thisLeg.Add(delta).Abs())
	return projected.GreaterThan(cap)
}

func cloneSectorCaps(in map[string]decimal.Decimal) map[string]decimal.Decimal {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]decimal.Decimal, len(in))
	for sector, cap := range in {
		out[sector] = cap
	}
	return out
}

func cloneSectors(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for ticker, sector := range in {
		out[ticker] = sector
	}
	return out
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
