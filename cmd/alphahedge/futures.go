package main

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
)

type mxContract struct {
	secID     string
	lastTrade time.Time
}

func defaultMXSchedule() []mxContract {
	d := func(y int, m time.Month, day int) time.Time {
		return time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
	}
	return []mxContract{
		{"MXH5", d(2025, 3, 20)},
		{"MXM5", d(2025, 6, 19)},
		{"MXU5", d(2025, 9, 18)},
		{"MXZ5", d(2025, 12, 18)},
		{"MXH6", d(2026, 3, 19)},
		{"MXM6", d(2026, 6, 18)},
		{"MXU6", d(2026, 9, 17)},
		{"MXZ6", d(2026, 12, 17)},
	}
}

type futuresConfig struct {
	Share             float64
	RollDays          int
	GOFinancingPctDay float64
	InitialMargin     float64
	UnitValue         float64
	CostRate          float64
	FeePerContract    float64
	RoundToZero       bool
}

type futuresResult struct {
	Curve          []backtest.EquityPoint
	LegPnl         float64
	TradeCosts     float64
	FinancingCosts float64
	Rolls          int
	AvgContracts   float64
	MaxContracts   int
}

func fetchFuturesCloses(ctx context.Context, source backtest.HistoricalSource, contracts []mxContract, from, till time.Time) (map[string]map[time.Time]decimal.Decimal, error) {
	out := make(map[string]map[time.Time]decimal.Decimal, len(contracts))
	for _, c := range contracts {
		candles, err := source.History(ctx, c.secID, from, till)
		if err != nil {
			return nil, fmt.Errorf("futures %s: %w", c.secID, err)
		}
		byDay := make(map[time.Time]decimal.Decimal, len(candles))
		for _, candle := range candles {
			day := time.Date(candle.Begin.Year(), candle.Begin.Month(), candle.Begin.Day(), 0, 0, 0, 0, time.UTC)
			byDay[day] = candle.Close
		}
		out[c.secID] = byDay
	}
	return out, nil
}

func activeContract(contracts []mxContract, day time.Time, rollDays int) mxContract {
	best := contracts[0]
	found := false
	for _, c := range contracts {
		if day.Before(c.lastTrade.AddDate(0, 0, -rollDays)) {
			if !found || c.lastTrade.Before(best.lastTrade) {
				best = c
				found = true
			}
		}
	}
	if !found {
		return contracts[len(contracts)-1]
	}
	return best
}

func futuresOverlayCurve(curve []backtest.EquityPoint, exposure map[time.Time]float64, closes map[string]map[time.Time]decimal.Decimal, contracts []mxContract, cfg futuresConfig) futuresResult {
	sorted := append([]backtest.EquityPoint(nil), curve...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Date.Before(sorted[j].Date) })
	out := make([]backtest.EquityPoint, len(sorted))

	res := futuresResult{}
	heldContract := ""
	held := 0
	cumLeg, cumTrade, cumFin := 0.0, 0.0, 0.0
	contractDays := 0

	for i := 0; i < len(sorted); i++ {
		day := sorted[i].Date
		if i > 0 && heldContract != "" {
			prevClose, okPrev := closes[heldContract][sorted[i-1].Date]
			curClose, okCur := closes[heldContract][day]
			if okPrev && okCur {
				cumLeg += float64(held) * toFloat(curClose.Sub(prevClose)) * cfg.UnitValue
			}
			cumFin += float64(absInt(held)) * cfg.InitialMargin * cfg.GOFinancingPctDay
			if absInt(held) > 0 {
				contractDays += absInt(held)
			}
		}

		target := activeContract(contracts, day, cfg.RollDays)
		notional := 0.0
		if price, ok := closes[target.secID][day]; ok {
			notional = toFloat(price) * cfg.UnitValue
		}
		targetContracts := 0
		if notional > 0 {
			raw := -cfg.Share * exposureAt(exposure, day) / notional
			if cfg.RoundToZero {
				targetContracts = int(math.Trunc(raw))
			} else {
				targetContracts = int(math.Round(raw))
			}
		}

		if target.secID != heldContract {
			if heldContract == "" {
				cumTrade += math.Abs(float64(targetContracts))*notional*cfg.CostRate + float64(absInt(targetContracts))*cfg.FeePerContract
			} else {
				tradeNotional := (math.Abs(float64(held)) + math.Abs(float64(targetContracts))) * notional
				cumTrade += tradeNotional*cfg.CostRate + float64(absInt(held)+absInt(targetContracts))*cfg.FeePerContract
				res.Rolls++
			}
		} else if delta := targetContracts - held; delta != 0 {
			cumTrade += math.Abs(float64(delta))*notional*cfg.CostRate + float64(absInt(delta))*cfg.FeePerContract
		}

		if absInt(targetContracts) > res.MaxContracts {
			res.MaxContracts = absInt(targetContracts)
		}
		held = targetContracts
		heldContract = target.secID

		out[i] = backtest.EquityPoint{Date: day, Equity: sorted[i].Equity.Add(decimal.NewFromFloat(cumLeg - cumTrade - cumFin))}
	}

	res.Curve = out
	res.LegPnl = cumLeg
	res.TradeCosts = cumTrade
	res.FinancingCosts = cumFin
	if len(sorted) > 0 {
		res.AvgContracts = float64(contractDays) / float64(len(sorted))
	}
	return res
}

func exposureAt(exposure map[time.Time]float64, day time.Time) float64 {
	if v, ok := exposure[day]; ok {
		return v
	}
	return 0
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
