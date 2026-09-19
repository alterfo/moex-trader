package pnl

import (
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/dailysummary"
	"github.com/olegsidorkin/moex-trader/internal/domain"
)

type Fill = dailysummary.Fill

type Position struct {
	Ticker   string
	Lots     int
	AvgEntry decimal.Decimal
	Realized decimal.Decimal
}

type Snapshot struct {
	Positions        map[string]*Position
	RealizedTotal    decimal.Decimal
	CommissionsTotal decimal.Decimal
	Fills            int
	ClosedTrades     int
	WinningTrades    int
	MaxDrawdown      decimal.Decimal
	LastFillAt       time.Time
}

func Replay(fills []Fill) Snapshot {
	sorted := append([]Fill(nil), fills...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].ExecutedAt.Before(sorted[j].ExecutedAt)
	})

	snap := Snapshot{Positions: make(map[string]*Position)}
	cumulative := decimal.Zero
	peak := decimal.Zero

	for _, f := range sorted {
		if f.Lots <= 0 || f.Price.Sign() <= 0 {
			continue
		}
		snap.Fills++
		snap.CommissionsTotal = snap.CommissionsTotal.Add(f.Commission)
		if f.ExecutedAt.After(snap.LastFillAt) {
			snap.LastFillAt = f.ExecutedAt
		}

		pos := snap.Positions[f.Ticker]
		if pos == nil {
			pos = &Position{Ticker: f.Ticker}
			snap.Positions[f.Ticker] = pos
		}

		realized := pos.record(f)
		if realized.IsZero() {
			continue
		}
		snap.RealizedTotal = snap.RealizedTotal.Add(realized)
		cumulative = cumulative.Add(realized)
		if cumulative.GreaterThan(peak) {
			peak = cumulative
		}
		if drawdown := peak.Sub(cumulative); drawdown.GreaterThan(snap.MaxDrawdown) {
			snap.MaxDrawdown = drawdown
		}
		snap.ClosedTrades++
		if realized.GreaterThan(decimal.Zero) {
			snap.WinningTrades++
		}
	}
	return snap
}

func (p *Position) record(f Fill) decimal.Decimal {
	delta := f.Lots
	if f.Action == domain.ActionSell {
		delta = -delta
	}
	if delta == 0 {
		return decimal.Zero
	}

	if p.Lots == 0 {
		p.Lots = delta
		p.AvgEntry = entryBasis(f.Action, f.Price, f.Commission.Div(decimal.NewFromInt(int64(f.Lots))))
		return decimal.Zero
	}

	if (p.Lots > 0) == (delta > 0) {
		oldAbs := absInt(p.Lots)
		addAbs := absInt(delta)
		newAbs := oldAbs + addAbs
		entryCost := entryBasis(f.Action, f.Price, f.Commission.Div(decimal.NewFromInt(int64(addAbs))))
		p.AvgEntry = p.AvgEntry.Mul(decimal.NewFromInt(int64(oldAbs))).
			Add(entryCost.Mul(decimal.NewFromInt(int64(addAbs)))).
			Div(decimal.NewFromInt(int64(newAbs)))
		p.Lots += delta
		return decimal.Zero
	}

	openAbs := absInt(p.Lots)
	deltaAbs := absInt(delta)
	closeLots := openAbs
	if deltaAbs < openAbs {
		closeLots = deltaAbs
	}

	var realized decimal.Decimal
	if p.Lots > 0 {
		realized = f.Price.Sub(p.AvgEntry).Mul(decimal.NewFromInt(int64(closeLots)))
	} else {
		realized = p.AvgEntry.Sub(f.Price).Mul(decimal.NewFromInt(int64(closeLots)))
	}
	perLotCommission := f.Commission.Div(decimal.NewFromInt(int64(f.Lots)))
	realized = realized.Sub(perLotCommission.Mul(decimal.NewFromInt(int64(closeLots))))

	p.Lots -= closeLots
	if p.Lots == 0 {
		p.AvgEntry = decimal.Zero
	}

	remainder := deltaAbs - closeLots
	if remainder > 0 {
		p.Lots = remainder
		if delta < 0 {
			p.Lots = -p.Lots
		}
		p.AvgEntry = entryBasis(f.Action, f.Price, perLotCommission)
	}

	p.Realized = p.Realized.Add(realized)
	return realized
}

func (s Snapshot) OpenTickers() []string {
	out := make([]string, 0, len(s.Positions))
	for ticker, pos := range s.Positions {
		if pos.Lots != 0 {
			out = append(out, ticker)
		}
	}
	sort.Strings(out)
	return out
}

func (s Snapshot) OpenPositions() int {
	count := 0
	for _, pos := range s.Positions {
		if pos.Lots != 0 {
			count++
		}
	}
	return count
}

func (s Snapshot) Unrealized(closes map[string]decimal.Decimal) decimal.Decimal {
	total := decimal.Zero
	for ticker, pos := range s.Positions {
		if pos.Lots == 0 {
			continue
		}
		close, ok := closes[normTicker(ticker)]
		if !ok || !close.IsPositive() {
			continue
		}
		total = total.Add(close.Sub(pos.AvgEntry).Mul(signedLots(pos.Lots)))
	}
	return total
}

func (s Snapshot) PositionPnL(closes map[string]decimal.Decimal) map[string]decimal.Decimal {
	out := make(map[string]decimal.Decimal, len(s.Positions))
	for ticker, pos := range s.Positions {
		mark := markPrice(pos, closes)
		out[ticker] = pos.Realized.Add(mark.Sub(pos.AvgEntry).Mul(signedLots(pos.Lots)))
	}
	return out
}

func (s Snapshot) UnrealizedByTicker(closes map[string]decimal.Decimal) map[string]decimal.Decimal {
	out := make(map[string]decimal.Decimal, len(s.Positions))
	for ticker, pos := range s.Positions {
		if pos.Lots == 0 {
			out[ticker] = decimal.Zero
			continue
		}
		out[ticker] = markPrice(pos, closes).Sub(pos.AvgEntry).Mul(signedLots(pos.Lots))
	}
	return out
}

func (s Snapshot) GrossExposure(closes map[string]decimal.Decimal) decimal.Decimal {
	total := decimal.Zero
	for _, pos := range s.Positions {
		if pos.Lots == 0 {
			continue
		}
		total = total.Add(markPrice(pos, closes).Mul(decimal.NewFromInt(int64(absInt(pos.Lots)))))
	}
	return total
}

func (s Snapshot) NetExposure(closes map[string]decimal.Decimal) decimal.Decimal {
	total := decimal.Zero
	for _, pos := range s.Positions {
		if pos.Lots == 0 {
			continue
		}
		total = total.Add(markPrice(pos, closes).Mul(signedLots(pos.Lots)))
	}
	return total
}

func (s Snapshot) ExposureByTicker(closes map[string]decimal.Decimal) map[string]decimal.Decimal {
	out := make(map[string]decimal.Decimal, len(s.Positions))
	for ticker, pos := range s.Positions {
		if pos.Lots == 0 {
			out[ticker] = decimal.Zero
			continue
		}
		out[ticker] = markPrice(pos, closes).Mul(decimal.NewFromInt(int64(absInt(pos.Lots))))
	}
	return out
}

func markPrice(pos *Position, closes map[string]decimal.Decimal) decimal.Decimal {
	if close, ok := closes[normTicker(pos.Ticker)]; ok && close.IsPositive() {
		return close
	}
	return pos.AvgEntry
}

func signedLots(lots int) decimal.Decimal {
	value := decimal.NewFromInt(int64(absInt(lots)))
	if lots < 0 {
		return value.Neg()
	}
	return value
}

func entryBasis(action domain.Action, price, perLotCommission decimal.Decimal) decimal.Decimal {
	if action == domain.ActionSell {
		return price.Sub(perLotCommission)
	}
	return price.Add(perLotCommission)
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func normTicker(ticker string) string {
	out := make([]rune, 0, len(ticker))
	for _, r := range ticker {
		if r >= 'a' && r <= 'z' {
			r -= 'a' - 'A'
		}
		out = append(out, r)
	}
	return string(out)
}
