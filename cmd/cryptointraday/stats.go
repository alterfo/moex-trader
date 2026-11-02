package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

type cellResult struct {
	Cell       string  `json:"cell"`
	Session    string  `json:"session"`
	Trades     int     `json:"trades"`
	Days       int     `json:"days"`
	TradesDay  float64 `json:"trades_per_day"`
	HitRate    float64 `json:"hit_rate"`
	HoldMin    float64 `json:"hold_minutes"`
	GrossTrade float64 `json:"gross_per_trade"`
	CostTrade  float64 `json:"cost_per_trade"`
	FundTrade  float64 `json:"funding_per_trade"`
	NetTrade   float64 `json:"net_per_trade"`
	TTrade     float64 `json:"t_per_trade"`
	GrossDay   float64 `json:"gross_per_day"`
	CostDay    float64 `json:"cost_per_day"`
	FundDay    float64 `json:"funding_per_day"`
	NetDay     float64 `json:"net_per_day"`
	NetDayT    float64 `json:"net_per_day_t"`
	Sharpe     float64 `json:"sharpe"`
	H1         float64 `json:"h1"`
	H2         float64 `json:"h2"`
}

func statsFromTrades(cell, session string, trades []tradeRec) cellResult {
	byDate := map[string][]tradeRec{}
	var dates []string
	for _, t := range trades {
		if _, ok := byDate[t.date]; !ok {
			dates = append(dates, t.date)
		}
		byDate[t.date] = append(byDate[t.date], t)
	}
	sort.Strings(dates)
	var daily []float64
	for _, d := range dates {
		ts := byDate[d]
		s := 0.0
		for _, t := range ts {
			s += t.net
		}
		daily = append(daily, s/float64(len(ts)))
	}
	var nets, holds []float64
	wins := 0
	for _, t := range trades {
		nets = append(nets, t.net)
		holds = append(holds, float64(t.hold))
		if t.net > 0 {
			wins++
		}
	}
	r := cellResult{
		Cell: cell, Session: session, Trades: len(trades), Days: len(dates),
		TradesDay:  float64(len(trades)) / float64(len(dates)),
		HitRate:    100 * float64(wins) / float64(len(trades)),
		HoldMin:    mean(holds),
		GrossTrade: meanField(trades, func(t tradeRec) float64 { return t.gross }),
		CostTrade:  meanField(trades, func(t tradeRec) float64 { return t.cost }),
		FundTrade:  meanField(trades, func(t tradeRec) float64 { return t.fund }),
		NetTrade:   mean(nets),
		TTrade:     nwT(nets, 5),
		NetDay:     mean(daily),
		NetDayT:    nwT(daily, 5),
		Sharpe:     sharpe(daily),
		GrossDay:   meanField(trades, func(t tradeRec) float64 { return t.gross }) * float64(len(trades)) / float64(len(dates)),
		CostDay:    meanField(trades, func(t tradeRec) float64 { return t.cost }) * float64(len(trades)) / float64(len(dates)),
		FundDay:    meanField(trades, func(t tradeRec) float64 { return t.fund }) * float64(len(trades)) / float64(len(dates)),
	}
	if len(daily) >= 2 {
		r.H1 = mean(daily[:len(daily)/2])
		r.H2 = mean(daily[len(daily)/2:])
	}
	return r
}

func writeCellJSON(path string, r cellResult) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dirOf(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("cell json -> %s\n", path)
	return nil
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}

func printCell(r cellResult) {
	if r.Trades == 0 {
		fmt.Printf("cell %s %s: days %d, gross %.2f, cost %.2f, funding %.2f, net %.2f bps/day, t %.2f, sharpe %.2f, halves %.2f / %.2f\n",
			r.Cell, r.Session, r.Days, r.GrossDay, r.CostDay, r.FundDay, r.NetDay, r.NetDayT, r.Sharpe, r.H1, r.H2)
		return
	}
	fmt.Printf("cell %s %s: trades %d over %d days, net %.1f bps/trade (t %.2f), hit %.1f%%, hold %.0f min, %.1f trades/day\n",
		r.Cell, r.Session, r.Trades, r.Days, r.NetTrade, r.TTrade, r.HitRate, r.HoldMin, r.TradesDay)
	fmt.Printf("  gross %.1f, cost %.1f, funding %.1f bps/trade; daily %.2f bps/day, t %.2f, sharpe %.2f, halves %.2f / %.2f\n",
		r.GrossTrade, r.CostTrade, r.FundTrade, r.NetDay, r.NetDayT, r.Sharpe, r.H1, r.H2)
}
