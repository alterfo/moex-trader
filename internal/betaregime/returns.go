package betaregime

import (
	"sort"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

// DailyReturnsFromCurve converts an equity curve into close-to-close daily
// returns keyed by the day each return is realised (the end of the interval).
// Days with a non-positive previous equity are skipped rather than emitting an
// infinite return.
func DailyReturnsFromCurve(curve []backtest.EquityPoint) map[time.Time]float64 {
	points := append([]backtest.EquityPoint(nil), curve...)
	sort.Slice(points, func(i, j int) bool { return points[i].Date.Before(points[j].Date) })
	out := make(map[time.Time]float64, len(points))
	for i := 1; i < len(points); i++ {
		prev, _ := points[i-1].Equity.Float64()
		cur, _ := points[i].Equity.Float64()
		if prev <= 0 {
			continue
		}
		out[points[i].Date] = cur/prev - 1
	}
	return out
}

// DailyReturnsFromCandles converts candle closes into close-to-close daily
// returns keyed by each candle's Begin date.
func DailyReturnsFromCandles(candles []moex.Candle) map[time.Time]float64 {
	cs := append([]moex.Candle(nil), candles...)
	sort.Slice(cs, func(i, j int) bool { return cs[i].Begin.Before(cs[j].Begin) })
	out := make(map[time.Time]float64, len(cs))
	for i := 1; i < len(cs); i++ {
		prev, _ := cs[i-1].Close.Float64()
		cur, _ := cs[i].Close.Float64()
		if prev <= 0 {
			continue
		}
		out[cs[i].Begin] = cur/prev - 1
	}
	return out
}

// CompoundReturn compounds daily returns over the half-open interval
// (from, to]: a position opened at the close of "from" and closed at the
// close of "to" earns every daily return strictly after "from" up to and
// including "to".
func CompoundReturn(returns map[time.Time]float64, from, to time.Time) (float64, bool) {
	acc := 1.0
	found := false
	for day, ret := range returns {
		if day.After(from) && !day.After(to) {
			acc *= 1 + ret
			found = true
		}
	}
	if !found {
		return 0, false
	}
	return acc - 1, true
}
