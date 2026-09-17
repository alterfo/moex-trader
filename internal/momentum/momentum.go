package momentum

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/domain"
)

type Variant int

const (
	VariantLongOnly Variant = iota
	VariantLongShort
)

func (v Variant) String() string {
	switch v {
	case VariantLongShort:
		return "long+short"
	default:
		return "long-only"
	}
}

type Rebalance struct {
	Date       time.Time
	Directions map[string]int
}

type Plan struct {
	Rebalances []Rebalance
}

func (p Plan) Len() int { return len(p.Rebalances) }

// BuildPlan ranks tickers by momentum value on each rebalance date and
// selects the top k (long-only) or the top k long plus bottom k short
// (long+short). dates is the sorted shared trading calendar; values maps
// ticker -> date -> momentum value. Rebalance dates are every rebalanceEvery
// trading days (index 0, rebalanceEvery, 2*rebalanceEvery, ...). Between
// rebalances the previous selection persists.
func BuildPlan(values map[string]map[time.Time]float64, dates []time.Time, k int, rebalanceEvery int, variant Variant) (Plan, error) {
	if k <= 0 {
		return Plan{}, fmt.Errorf("momentum: k must be positive")
	}
	if rebalanceEvery <= 0 {
		return Plan{}, fmt.Errorf("momentum: rebalanceEvery must be positive")
	}
	if variant != VariantLongOnly && variant != VariantLongShort {
		return Plan{}, fmt.Errorf("momentum: unknown variant %d", variant)
	}

	plan := Plan{}
	for i, date := range dates {
		if i%rebalanceEvery != 0 {
			continue
		}
		directions, err := selectDirections(values, normalizeDate(date), k, variant)
		if err != nil {
			return Plan{}, err
		}
		plan.Rebalances = append(plan.Rebalances, Rebalance{Date: normalizeDate(date), Directions: directions})
	}
	return plan, nil
}

func selectDirections(values map[string]map[time.Time]float64, date time.Time, k int, variant Variant) (map[string]int, error) {
	type entry struct {
		ticker string
		value  float64
	}
	var entries []entry
	for ticker, byDate := range values {
		v, ok := byDate[date]
		if !ok || math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		entries = append(entries, entry{ticker: ticker, value: v})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].value != entries[j].value {
			return entries[i].value > entries[j].value
		}
		return entries[i].ticker < entries[j].ticker
	})

	directions := make(map[string]int)
	top := k
	if top > len(entries) {
		top = len(entries)
	}
	for i := 0; i < top; i++ {
		directions[entries[i].ticker] = 1
	}
	if variant == VariantLongShort {
		bottom := k
		if bottom > len(entries) {
			bottom = len(entries)
		}
		for i := len(entries) - 1; i >= len(entries)-bottom; i-- {
			if _, alreadyLong := directions[entries[i].ticker]; alreadyLong {
				continue
			}
			directions[entries[i].ticker] = -1
		}
	}
	return directions, nil
}

// SignalSource implements both backtest.SignalSource and
// backtest.TargetPositionSource. It plans an absolute target position from a
// precomputed Plan: positive signed lots = long, negative = short, zero =
// flat, with position size derived from TargetNotional like the ensemble.
type SignalSource struct {
	Plan           Plan
	TargetNotional decimal.Decimal
	MaxLots        int
}

func (s *SignalSource) directionFor(ticker string, at time.Time) int {
	if s == nil {
		return 0
	}
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	date := normalizeDate(at)
	for i := len(s.Plan.Rebalances) - 1; i >= 0; i-- {
		if !s.Plan.Rebalances[i].Date.After(date) {
			return s.Plan.Rebalances[i].Directions[ticker]
		}
	}
	return 0
}

func (s *SignalSource) targetLots(feature domain.FeatureContext) int {
	if s == nil || !s.TargetNotional.IsPositive() {
		if s != nil && s.MaxLots > 0 {
			return s.MaxLots
		}
		return 1
	}
	if !feature.LastPrice.IsPositive() {
		if s.MaxLots > 0 {
			return s.MaxLots
		}
		return 1
	}
	perUnit := feature.LastPrice
	if feature.LotSize.IsPositive() {
		perUnit = feature.LastPrice.Mul(feature.LotSize)
	}
	lots := s.TargetNotional.Div(perUnit).Round(0).IntPart()
	if lots < 1 {
		return 1
	}
	if lots > math.MaxInt32 {
		return math.MaxInt32
	}
	return int(lots)
}

func (s *SignalSource) TargetPosition(feature domain.FeatureContext) (int, bool) {
	direction := s.directionFor(feature.Ticker, feature.GeneratedAt)
	if direction == 0 {
		return 0, true
	}
	return direction * s.targetLots(feature), true
}

func (s *SignalSource) Generate(_ context.Context, feature domain.FeatureContext) (domain.TradeSignal, error) {
	direction := s.directionFor(feature.Ticker, feature.GeneratedAt)
	lots := s.targetLots(feature)
	signal := domain.TradeSignal{
		Ticker:      feature.Ticker,
		Action:      domain.ActionHold,
		Confidence:  decimal.Zero,
		TargetLots:  0,
		GeneratedAt: feature.GeneratedAt,
		HoldReason:  domain.HoldReasonModel,
	}
	switch {
	case direction > 0:
		signal.Action = domain.ActionBuy
		signal.TargetLots = lots
		signal.Confidence = decimal.NewFromInt(1)
		signal.Reasoning = "momentum:top-k"
	case direction < 0:
		signal.Action = domain.ActionSell
		signal.TargetLots = lots
		signal.Confidence = decimal.NewFromInt(1)
		signal.Reasoning = "momentum:bottom-k"
	}
	return signal, nil
}

func normalizeDate(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

var _ backtest.SignalSource = (*SignalSource)(nil)
var _ backtest.TargetPositionSource = (*SignalSource)(nil)
