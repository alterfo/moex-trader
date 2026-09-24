package walkforward

import (
	"math"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
)

func AggregateEquityCurve(results []backtest.Result, deposit decimal.Decimal) []backtest.EquityPoint {
	var combined []backtest.EquityPoint
	level, _ := deposit.Float64()
	for _, r := range results {
		pts := r.EquityCurve
		if len(pts) == 0 {
			continue
		}
		base, _ := pts[0].Equity.Float64()
		if base <= 0 {
			continue
		}
		for i, p := range pts {
			if i == 0 && len(combined) > 0 {
				continue
			}
			v, _ := p.Equity.Float64()
			idx := level * (v / base)
			combined = append(combined, backtest.EquityPoint{Date: p.Date, Equity: decimal.NewFromFloat(idx)})
		}
		last, _ := pts[len(pts)-1].Equity.Float64()
		level = level * (last / base)
	}
	return combined
}

func Aggregate(tickers []string, deposit decimal.Decimal, results []backtest.Result) backtest.Result {
	var netPnl, realizedPnl, realizedNetBorrow, grossPnl, totalCommission, totalBorrow decimal.Decimal
	var closedTrades, winningTrades int
	var allTrades []backtest.Trade
	var lastOpen []backtest.OpenPosition
	var lastUnrealized decimal.Decimal
	var start, end time.Time

	for i, r := range results {
		netPnl = netPnl.Add(r.NetPnl)
		realizedPnl = realizedPnl.Add(r.RealizedPnl)
		realizedNetBorrow = realizedNetBorrow.Add(r.RealizedPnlNetBorrow)
		grossPnl = grossPnl.Add(r.GrossPnl)
		totalCommission = totalCommission.Add(r.TotalCommission)
		totalBorrow = totalBorrow.Add(r.TotalBorrow)
		closedTrades += r.ClosedTrades
		winningTrades += r.WinningTrades
		allTrades = append(allTrades, r.Trades...)
		if i == 0 {
			start = r.Start
		}
		if i == len(results)-1 {
			end = r.End
			lastOpen = r.OpenPositions
			lastUnrealized = r.UnrealizedPnl
		}
	}

	curve := AggregateEquityCurve(results, deposit)
	sharpe, sortino, cagr, maxDDPct, maxDDRub := backtest.CurveStats(curve)
	finalEquity := deposit
	if len(curve) > 0 {
		finalEquity = curve[len(curve)-1].Equity
	}
	attribution := backtest.ComputeAttribution(allTrades, lastOpen, backtest.DefaultAttributionTopN)

	result := backtest.Result{
		Tickers:              append([]string(nil), tickers...),
		Start:                start,
		End:                  end,
		Deposit:              deposit,
		FinalEquity:          finalEquity,
		NetPnl:               netPnl,
		RealizedPnl:          realizedPnl,
		RealizedPnlNetBorrow: realizedNetBorrow,
		UnrealizedPnl:        lastUnrealized,
		GrossPnl:             grossPnl,
		TotalCommission:      totalCommission,
		TotalBorrow:          totalBorrow,
		ClosedTrades:         closedTrades,
		WinningTrades:        winningTrades,
		Trades:               allTrades,
		OpenPositions:        lastOpen,
		Attribution:          attribution,
		EquityCurve:          curve,
		Sharpe:               sharpe,
		Sortino:              sortino,
		CAGR:                 cagr,
		MaxDrawdownPct:       maxDDPct,
		MaxDrawdownRub:       maxDDRub,
	}
	if closedTrades > 0 {
		result.HitRate = float64(winningTrades) / float64(closedTrades)
	}
	if maxDDPct != 0 {
		result.Calmar = cagr / math.Abs(maxDDPct)
	}
	return result
}
