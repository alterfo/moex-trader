package betaregime

import (
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

// Regime labels a quarterly IMOEX window as a directional trend, a decline,
// or flat. The threshold is a pre-registered flat band: |quarter return| <=
// flatBand is flat, above it is trend-up, below its negative is trend-down.
const (
	RegimeTrendUp   = "trend-up"
	RegimeFlat      = "flat"
	RegimeTrendDown = "trend-down"
)

// Window is a closed OOS quarter.
type Window struct {
	From time.Time
	Till time.Time
}

// WindowRegime is a Window annotated with the IMOEX return over it.
type WindowRegime struct {
	Window
	ImoexReturn float64
	Regime      string
	HasData     bool
}

// LabelRegime classifies a fractional return (for example 0.031) using the
// pre-registered flat band.
func LabelRegime(returnPct float64, flatBand float64) string {
	if returnPct > flatBand {
		return RegimeTrendUp
	}
	if returnPct < -flatBand {
		return RegimeTrendDown
	}
	return RegimeFlat
}

// ClassifyWindows computes the IMOEX return over each window using the close
// at or before the window end and the close at or before the window start,
// then labels the regime. A window without both marks gets HasData=false.
func ClassifyWindows(candles []moex.Candle, windows []Window, flatBand float64) []WindowRegime {
	cs := append([]moex.Candle(nil), candles...)
	sort.Slice(cs, func(i, j int) bool { return cs[i].Begin.Before(cs[j].Begin) })
	out := make([]WindowRegime, 0, len(windows))
	for _, w := range windows {
		start, okStart := closeAtOrBefore(cs, w.From)
		end, okEnd := closeAtOrBefore(cs, w.Till)
		wr := WindowRegime{Window: w}
		if !okStart || !okEnd {
			out = append(out, wr)
			continue
		}
		startF, _ := start.Float64()
		endF, _ := end.Float64()
		if startF <= 0 {
			out = append(out, wr)
			continue
		}
		ret := endF/startF - 1
		wr.ImoexReturn = ret
		wr.Regime = LabelRegime(ret, flatBand)
		wr.HasData = true
		out = append(out, wr)
	}
	return out
}

func closeAtOrBefore(candles []moex.Candle, day time.Time) (decimal.Decimal, bool) {
	idx := sort.Search(len(candles), func(i int) bool { return candles[i].Begin.After(day) })
	if idx == 0 {
		return decimal.Decimal{}, false
	}
	return candles[idx-1].Close, true
}
