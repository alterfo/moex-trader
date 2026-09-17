package risk

import (
	"strings"
	"sync"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

// Fill is the minimal fill information the ticker circuit breaker needs to
// compute realized P&L. It is kept independent of the executor package so the
// risk layer does not depend on the execution layer.
type Fill struct {
	Ticker     string
	Action     domain.Action
	Lots       int
	Price      decimal.Decimal
	Commission decimal.Decimal
}

// TickerBreakerConfig configures the per-ticker circuit breaker.
//
// MaxCumulativeLossPct is a fraction of Notional (0.05 = 5%), matching the
// convention used by RebalanceMinDeviationPct in the risk config. When
// Notional is not positive, the cumulative-loss trip is disabled and only the
// consecutive-loss trip applies.
type TickerBreakerConfig struct {
	MaxConsecutiveLosses int
	MaxCumulativeLossPct decimal.Decimal
	Notional             decimal.Decimal
}

// DefaultTickerBreakerConfig returns the fallback thresholds: three
// consecutive losing round-trips or a cumulative realized loss of 5% of
// notional.
func DefaultTickerBreakerConfig() TickerBreakerConfig {
	return TickerBreakerConfig{
		MaxConsecutiveLosses: 3,
		MaxCumulativeLossPct: decimal.New(5, -2),
	}
}

type tickerState struct {
	openLots          int
	avgEntry          decimal.Decimal
	realized          decimal.Decimal
	consecutiveLosses int
	tripped           bool
	reason            string
}

// TickerBreaker stops trading individual names after repeated or outsized
// realized losses. It is intentionally diagnostics/safety machinery and never
// retrains or changes the model. A tripped ticker stays tripped until Reset is
// called.
type TickerBreaker struct {
	mu    sync.Mutex
	cfg   TickerBreakerConfig
	state map[string]*tickerState
}

func NewTickerBreaker(cfg TickerBreakerConfig) *TickerBreaker {
	if cfg.MaxConsecutiveLosses <= 0 {
		cfg.MaxConsecutiveLosses = DefaultTickerBreakerConfig().MaxConsecutiveLosses
	}
	if !cfg.MaxCumulativeLossPct.IsPositive() {
		cfg.MaxCumulativeLossPct = DefaultTickerBreakerConfig().MaxCumulativeLossPct
	}
	return &TickerBreaker{
		cfg:   cfg,
		state: make(map[string]*tickerState),
	}
}

// RecordFill updates the realized P&L accounting for one executed fill using
// average-cost position tracking.
func (b *TickerBreaker) RecordFill(f Fill) {
	if f.Lots <= 0 || f.Price.Sign() <= 0 {
		return
	}
	key := normalizeTicker(f.Ticker)
	if key == "" {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	st := b.state[key]
	if st == nil {
		st = &tickerState{}
		b.state[key] = st
	}
	st.record(f, b.cfg)
}

// Blocked reports whether the named ticker has tripped its circuit breaker.
func (b *TickerBreaker) Blocked(ticker string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	st := b.state[normalizeTicker(ticker)]
	return st != nil && st.tripped
}

// Reason returns the trip reason for a blocked ticker, or the empty string
// when the ticker is not blocked.
func (b *TickerBreaker) Reason(ticker string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	st := b.state[normalizeTicker(ticker)]
	if st == nil || !st.tripped {
		return ""
	}
	return st.reason
}

// Reset clears the trip flag for one ticker without discarding its realized
// P&L or open-position accounting.
func (b *TickerBreaker) Reset(ticker string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	st := b.state[normalizeTicker(ticker)]
	if st == nil {
		return
	}
	st.tripped = false
	st.reason = ""
	st.consecutiveLosses = 0
}

// State exposes the accounting for one ticker for tests and diagnostics.
func (b *TickerBreaker) State(ticker string) (openLots int, realized decimal.Decimal, consecutiveLosses int, tripped bool, reason string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	st := b.state[normalizeTicker(ticker)]
	if st == nil {
		return 0, decimal.Zero, 0, false, ""
	}
	return st.openLots, st.realized, st.consecutiveLosses, st.tripped, st.reason
}

func (s *tickerState) record(f Fill, cfg TickerBreakerConfig) {
	delta := f.Lots
	if f.Action == domain.ActionSell {
		delta = -delta
	}
	if delta == 0 {
		return
	}

	if s.openLots == 0 {
		s.openLots = delta
		s.avgEntry = entryBasis(f.Action, f.Price, f.Commission.Div(decimal.NewFromInt(int64(f.Lots))))
		return
	}

	// Adding to the existing position in the same direction.
	if (s.openLots > 0) == (delta > 0) {
		oldAbs := absInt(s.openLots)
		addAbs := absInt(delta)
		newAbs := oldAbs + addAbs
		entryCost := entryBasis(f.Action, f.Price, f.Commission.Div(decimal.NewFromInt(int64(addAbs))))
		s.avgEntry = s.avgEntry.Mul(decimal.NewFromInt(int64(oldAbs))).
			Add(entryCost.Mul(decimal.NewFromInt(int64(addAbs)))).
			Div(decimal.NewFromInt(int64(newAbs)))
		s.openLots += delta
		return
	}

	// Opposite direction: close the smaller magnitude, realize P&L, then open
	// any remainder in the new direction.
	openAbs := absInt(s.openLots)
	deltaAbs := absInt(delta)
	closeLots := openAbs
	if deltaAbs < openAbs {
		closeLots = deltaAbs
	}

	var pnl decimal.Decimal
	if s.openLots > 0 {
		pnl = f.Price.Sub(s.avgEntry).Mul(decimal.NewFromInt(int64(closeLots)))
	} else {
		pnl = s.avgEntry.Sub(f.Price).Mul(decimal.NewFromInt(int64(closeLots)))
	}
	perLotCommission := f.Commission.Div(decimal.NewFromInt(int64(f.Lots)))
	closedCommission := perLotCommission.Mul(decimal.NewFromInt(int64(closeLots)))
	pnl = pnl.Sub(closedCommission)

	s.realized = s.realized.Add(pnl)
	if pnl.IsNegative() {
		s.consecutiveLosses++
	} else {
		s.consecutiveLosses = 0
	}

	if s.openLots > 0 {
		s.openLots -= closeLots
	} else {
		s.openLots += closeLots
	}
	if s.openLots == 0 {
		s.avgEntry = decimal.Zero
	}

	s.evaluateTrip(cfg)

	remainder := deltaAbs - closeLots
	if remainder > 0 {
		s.openLots = remainder
		if delta < 0 {
			s.openLots = -s.openLots
		}
		s.avgEntry = entryBasis(f.Action, f.Price, perLotCommission)
	}
}

func entryBasis(action domain.Action, price, perLotCommission decimal.Decimal) decimal.Decimal {
	if action == domain.ActionSell {
		return price.Sub(perLotCommission)
	}
	return price.Add(perLotCommission)
}

func (s *tickerState) evaluateTrip(cfg TickerBreakerConfig) {
	if s.tripped {
		return
	}
	if s.consecutiveLosses >= cfg.MaxConsecutiveLosses {
		s.tripped = true
		s.reason = "consecutive losing round-trips exceeded limit"
		return
	}
	if cfg.Notional.IsPositive() {
		limit := cfg.Notional.Mul(cfg.MaxCumulativeLossPct)
		if s.realized.IsNegative() && s.realized.Abs().GreaterThanOrEqual(limit) {
			s.tripped = true
			s.reason = "cumulative realized loss exceeded limit"
			return
		}
	}
}

func normalizeTicker(ticker string) string {
	return strings.ToUpper(strings.TrimSpace(ticker))
}
