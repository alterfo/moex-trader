package gapstress

import (
	"math"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

// Side is the direction of a position.
type Side int

const (
	SideLong Side = iota
	SideShort
)

func (s Side) String() string {
	if s == SideShort {
		return "short"
	}
	return "long"
}

// Pre-registered risk floors carried over from the source debate. The live
// kill switch blocks new entries at 3% drawdown / 0.5% daily loss but does
// not liquidate open positions, so a gap hits equity in full regardless.
const (
	DrawdownFloorPct  = 3.0
	DailyLossFloorPct = 0.5
)

// Position is a single open position. Notional is the absolute ruble
// exposure of the position (lots times current price).
type Position struct {
	Ticker   string
	Side     Side
	Notional decimal.Decimal
}

// Portfolio is the set of open positions plus the deposit the equity floors
// are measured against.
type Portfolio struct {
	Deposit   decimal.Decimal
	Positions []Position
}

// Scenario shocks each listed ticker by a fractional price move. A shock of
// -0.10 means the price falls 10%. Tickers absent from the map do not move.
type Scenario struct {
	Name   string
	Shocks map[string]float64
}

// Impact is the portfolio P&L impact of one scenario.
type Impact struct {
	Name                  string
	Pnl                   decimal.Decimal
	PnlPct                decimal.Decimal // P&L as a percentage of deposit
	DrawdownFloorPct      decimal.Decimal
	DrawdownFloorRub      decimal.Decimal
	BreachesDrawdownFloor bool
	BreachesDailyLoss     bool
}

// GrossExposure returns the sum of absolute position notionals.
func GrossExposure(p Portfolio) decimal.Decimal {
	total := decimal.Zero
	for _, pos := range p.Positions {
		total = total.Add(pos.Notional)
	}
	return total
}

// LongExposure returns the sum of notional on the long side.
func LongExposure(p Portfolio) decimal.Decimal {
	total := decimal.Zero
	for _, pos := range p.Positions {
		if pos.Side == SideLong {
			total = total.Add(pos.Notional)
		}
	}
	return total
}

// ShortExposure returns the sum of notional on the short side.
func ShortExposure(p Portfolio) decimal.Decimal {
	total := decimal.Zero
	for _, pos := range p.Positions {
		if pos.Side == SideShort {
			total = total.Add(pos.Notional)
		}
	}
	return total
}

// NetExposure returns long minus short notional.
func NetExposure(p Portfolio) decimal.Decimal {
	return LongExposure(p).Sub(ShortExposure(p))
}

// ImpactPnl returns the signed P&L of applying shocks to the portfolio. A
// long position gains notional*shock, a short position loses notional*shock.
func ImpactPnl(p Portfolio, shocks map[string]float64) decimal.Decimal {
	total := decimal.Zero
	for _, pos := range p.Positions {
		shock, ok := shocks[strings.ToUpper(strings.TrimSpace(pos.Ticker))]
		if !ok || math.IsNaN(shock) || math.IsInf(shock, 0) {
			continue
		}
		delta := pos.Notional.Mul(decimal.NewFromFloat(shock))
		if pos.Side == SideShort {
			delta = delta.Neg()
		}
		total = total.Add(delta)
	}
	return total
}

// Run evaluates each scenario against the portfolio and reports the impact
// versus the pre-registered drawdown and daily-loss floors.
func Run(p Portfolio, scenarios []Scenario) []Impact {
	drawdownPct := decimal.NewFromFloat(DrawdownFloorPct)
	drawdownRub := p.Deposit.Mul(drawdownPct).Div(decimal.NewFromInt(100))
	dailyLossRub := p.Deposit.Mul(decimal.NewFromFloat(DailyLossFloorPct)).Div(decimal.NewFromInt(100))

	out := make([]Impact, 0, len(scenarios))
	for _, scenario := range scenarios {
		pnl := ImpactPnl(p, scenario.Shocks)
		pnlPct := decimal.Zero
		if p.Deposit.IsPositive() {
			pnlPct = pnl.Div(p.Deposit).Mul(decimal.NewFromInt(100))
		}
		loss := pnl.Neg()
		out = append(out, Impact{
			Name:                  scenario.Name,
			Pnl:                   pnl,
			PnlPct:                pnlPct,
			DrawdownFloorPct:      drawdownPct,
			DrawdownFloorRub:      drawdownRub,
			BreachesDrawdownFloor: loss.GreaterThanOrEqual(drawdownRub),
			BreachesDailyLoss:     loss.GreaterThanOrEqual(dailyLossRub),
		})
	}
	return out
}

// UniformShockScenario builds a scenario applying the same fractional shock
// to every listed ticker.
func UniformShockScenario(tickers []string, name string, pct float64) Scenario {
	shocks := make(map[string]float64, len(tickers))
	for _, ticker := range tickers {
		shocks[strings.ToUpper(strings.TrimSpace(ticker))] = pct
	}
	return Scenario{Name: name, Shocks: shocks}
}

// OvernightGaps returns date -> open(t)/close(t-1)-1 for a daily candle
// series. Days without a valid prior close or open are skipped.
func OvernightGaps(candles []moex.Candle) map[time.Time]float64 {
	out := make(map[time.Time]float64, len(candles))
	for i := 1; i < len(candles); i++ {
		prevClose := candles[i-1].Close
		open := candles[i].Open
		if prevClose.Sign() <= 0 || open.Sign() <= 0 {
			continue
		}
		gap, _ := open.Div(prevClose).Sub(decimal.NewFromInt(1)).Float64()
		out[candles[i].Begin] = gap
	}
	return out
}

// OvernightGapsByTicker maps every ticker to its OvernightGaps series.
func OvernightGapsByTicker(candles map[string][]moex.Candle) map[string]map[time.Time]float64 {
	out := make(map[string]map[time.Time]float64, len(candles))
	for ticker, series := range candles {
		out[strings.ToUpper(strings.TrimSpace(ticker))] = OvernightGaps(series)
	}
	return out
}

// WorstHistoricalDayScenario finds the single historical trading day whose
// overnight gaps (across the tickers that traded that day) produce the worst
// portfolio P&L. ok is false when no aligned gap exists for any position.
func WorstHistoricalDayScenario(p Portfolio, gapsByTicker map[string]map[time.Time]float64) (Scenario, time.Time, bool) {
	dates := make(map[time.Time]struct{})
	for _, gaps := range gapsByTicker {
		for day := range gaps {
			dates[day] = struct{}{}
		}
	}
	ordered := make([]time.Time, 0, len(dates))
	for day := range dates {
		ordered = append(ordered, day)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Before(ordered[j]) })

	var best Scenario
	var bestDate time.Time
	bestPnl := decimal.Zero
	found := false
	for _, day := range ordered {
		shocks := make(map[string]float64, len(p.Positions))
		for _, pos := range p.Positions {
			ticker := strings.ToUpper(strings.TrimSpace(pos.Ticker))
			if gaps, ok := gapsByTicker[ticker]; ok {
				if gap, ok := gaps[day]; ok {
					shocks[ticker] = gap
				}
			}
		}
		if len(shocks) == 0 {
			continue
		}
		pnl := ImpactPnl(p, shocks)
		if !found || pnl.LessThan(bestPnl) {
			found = true
			bestPnl = pnl
			bestDate = day
			best = Scenario{Name: "worst-historical-day", Shocks: shocks}
		}
	}
	return best, bestDate, found
}

// WorstPerTickerScenario applies each position's most adverse historical
// overnight gap simultaneously. It is a synthetic composite stress, not a
// single historical event: longs take their most negative gap and shorts
// take their most positive gap.
func WorstPerTickerScenario(p Portfolio, gapsByTicker map[string]map[time.Time]float64) Scenario {
	shocks := make(map[string]float64, len(p.Positions))
	for _, pos := range p.Positions {
		ticker := strings.ToUpper(strings.TrimSpace(pos.Ticker))
		gap, ok := worstGap(gapsByTicker[ticker], pos.Side)
		if !ok {
			continue
		}
		shocks[ticker] = gap
	}
	return Scenario{Name: "worst-per-ticker-combined", Shocks: shocks}
}

func worstGap(gaps map[time.Time]float64, side Side) (float64, bool) {
	if len(gaps) == 0 {
		return 0, false
	}
	worst := 0.0
	first := true
	for _, gap := range gaps {
		if first {
			worst = gap
			first = false
			continue
		}
		if side == SideLong {
			if gap < worst {
				worst = gap
			}
		} else if gap > worst {
			worst = gap
		}
	}
	return worst, true
}
