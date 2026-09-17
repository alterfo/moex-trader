package betaregime

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/domain"
)

// TradeSample is one closed trade aligned to the IMOEX, equal-weight and
// momentum benchmark returns earned over the same holding window. ReturnPct
// is the realised P&L divided by entry notional, so it is scale-free and
// comparable with the benchmark returns.
type TradeSample struct {
	Ticker         string
	Side           int
	OpenedAt       time.Time
	ClosedAt       time.Time
	Notional       decimal.Decimal
	NetPnl         decimal.Decimal
	ReturnPct      float64
	ImoexReturn    float64
	BenchReturn    float64
	MomentumReturn float64
}

// BuildTradeSamples aligns each closed trade to the three return series.
// Side is +1 for a long trade and -1 for a short trade.
func BuildTradeSamples(trades []backtest.Trade, imoex, bench, momentum map[time.Time]float64) []TradeSample {
	samples := make([]TradeSample, 0, len(trades))
	for _, tr := range trades {
		notional := tr.EntryPrice.Mul(decimal.NewFromInt(int64(tr.Lots)))
		if !notional.IsPositive() {
			continue
		}
		side := 1
		if tr.Action == domain.ActionSell {
			side = -1
		}
		net, _ := tr.NetPnl.Float64()
		not, _ := notional.Float64()
		returnPct := 0.0
		if not > 0 {
			returnPct = net / not
		}
		imoexRet, _ := CompoundReturn(imoex, tr.OpenedAt, tr.ClosedAt)
		benchRet, _ := CompoundReturn(bench, tr.OpenedAt, tr.ClosedAt)
		momRet, _ := CompoundReturn(momentum, tr.OpenedAt, tr.ClosedAt)
		samples = append(samples, TradeSample{
			Ticker:         tr.Ticker,
			Side:           side,
			OpenedAt:       tr.OpenedAt,
			ClosedAt:       tr.ClosedAt,
			Notional:       notional,
			NetPnl:         tr.NetPnl,
			ReturnPct:      returnPct,
			ImoexReturn:    imoexRet,
			BenchReturn:    benchRet,
			MomentumReturn: momRet,
		})
	}
	return samples
}

// LegStats aggregates realised P&L, notional, the IMOEX (beta) component and
// the residual (alpha) component for one side of the book.
type LegStats struct {
	Trades           int
	RealizedPnl      decimal.Decimal
	Notional         decimal.Decimal
	ImoexPnl         decimal.Decimal
	ExcessPnl        decimal.Decimal
	ReturnOnNotional float64
}

// Decomposition splits realised P&L into a market (IMOEX) component and an
// excess component, for long and short legs separately, and reports the
// aggregate return on gross exposure instead of return on the 1M deposit.
type Decomposition struct {
	Long                  LegStats
	Short                 LegStats
	TotalRealizedPnl      decimal.Decimal
	GrossExposure         decimal.Decimal
	ImoexPnl              decimal.Decimal
	ExcessPnl             decimal.Decimal
	ReturnOnGrossExposure float64
}

// Decompose aggregates per-trade samples. For a long trade the IMOEX
// component is imoexReturn*notional; for a short trade it is
// -imoexReturn*notional, so the market component is side*imoexReturn*notional
// and the excess is netPnl minus that component.
func Decompose(samples []TradeSample) Decomposition {
	var d Decomposition
	for _, s := range samples {
		imoexComponent := decimal.NewFromFloat(s.ImoexReturn).Mul(s.Notional)
		if s.Side < 0 {
			imoexComponent = imoexComponent.Neg()
		}
		excess := s.NetPnl.Sub(imoexComponent)

		leg := &d.Long
		if s.Side < 0 {
			leg = &d.Short
		}
		leg.Trades++
		leg.RealizedPnl = leg.RealizedPnl.Add(s.NetPnl)
		leg.Notional = leg.Notional.Add(s.Notional)
		leg.ImoexPnl = leg.ImoexPnl.Add(imoexComponent)
		leg.ExcessPnl = leg.ExcessPnl.Add(excess)

		d.TotalRealizedPnl = d.TotalRealizedPnl.Add(s.NetPnl)
		d.GrossExposure = d.GrossExposure.Add(s.Notional)
		d.ImoexPnl = d.ImoexPnl.Add(imoexComponent)
		d.ExcessPnl = d.ExcessPnl.Add(excess)
	}

	if f, _ := d.Long.Notional.Float64(); f > 0 {
		pnl, _ := d.Long.RealizedPnl.Float64()
		d.Long.ReturnOnNotional = pnl / f
	}
	if f, _ := d.Short.Notional.Float64(); f > 0 {
		pnl, _ := d.Short.RealizedPnl.Float64()
		d.Short.ReturnOnNotional = pnl / f
	}
	if f, _ := d.GrossExposure.Float64(); f > 0 {
		pnl, _ := d.TotalRealizedPnl.Float64()
		d.ReturnOnGrossExposure = pnl / f
	}
	return d
}
