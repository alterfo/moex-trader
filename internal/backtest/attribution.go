package backtest

import (
	"sort"

	"github.com/shopspring/decimal"
)

const DefaultAttributionTopN = 5

type TickerAttribution struct {
	Ticker        string
	Trades        int
	Wins          int
	Losses        int
	GrossPnl      decimal.Decimal
	Commission    decimal.Decimal
	RealizedPnl   decimal.Decimal
	UnrealizedPnl decimal.Decimal
}

type Attribution struct {
	Tickers         []TickerAttribution
	RealizedTotal   decimal.Decimal
	UnrealizedTotal decimal.Decimal
	TopN            int
	TopTradeShare   float64
	TopTickerShare  float64
	TradeCount      int
}

func computeAttribution(trades []Trade, open []OpenPosition, topN int) Attribution {
	if topN <= 0 {
		topN = DefaultAttributionTopN
	}

	byTicker := make(map[string]*TickerAttribution)
	var realizedTotal decimal.Decimal
	var unrealizedTotal decimal.Decimal
	var totalAbsTrade decimal.Decimal

	for _, t := range trades {
		entry := byTicker[t.Ticker]
		if entry == nil {
			entry = &TickerAttribution{Ticker: t.Ticker}
			byTicker[t.Ticker] = entry
		}
		entry.Trades++
		switch {
		case t.NetPnl.Sign() > 0:
			entry.Wins++
		case t.NetPnl.Sign() < 0:
			entry.Losses++
		}
		entry.GrossPnl = entry.GrossPnl.Add(t.GrossPnl)
		entry.Commission = entry.Commission.Add(t.Commission)
		entry.RealizedPnl = entry.RealizedPnl.Add(t.NetPnl)
		realizedTotal = realizedTotal.Add(t.NetPnl)
		totalAbsTrade = totalAbsTrade.Add(t.NetPnl.Abs())
	}

	for _, p := range open {
		entry := byTicker[p.Ticker]
		if entry == nil {
			entry = &TickerAttribution{Ticker: p.Ticker}
			byTicker[p.Ticker] = entry
		}
		entry.UnrealizedPnl = entry.UnrealizedPnl.Add(p.UnrealizedPnl)
		unrealizedTotal = unrealizedTotal.Add(p.UnrealizedPnl)
	}

	tickers := make([]TickerAttribution, 0, len(byTicker))
	for _, entry := range byTicker {
		tickers = append(tickers, *entry)
	}
	sort.Slice(tickers, func(i, j int) bool { return tickers[i].Ticker < tickers[j].Ticker })

	attr := Attribution{
		Tickers:         tickers,
		RealizedTotal:   realizedTotal,
		UnrealizedTotal: unrealizedTotal,
		TopN:            topN,
		TradeCount:      len(trades),
	}
	if totalAbsTrade.Sign() > 0 {
		attr.TopTradeShare = topTradeShare(trades, topN, totalAbsTrade)
		attr.TopTickerShare = topTickerShare(byTicker, topN, totalAbsTrade)
	}
	return attr
}

func topTradeShare(trades []Trade, topN int, totalAbs decimal.Decimal) float64 {
	if len(trades) == 0 || totalAbs.Sign() <= 0 {
		return 0
	}
	ranked := append([]Trade(nil), trades...)
	sort.Slice(ranked, func(i, j int) bool {
		ai := ranked[i].NetPnl.Abs()
		aj := ranked[j].NetPnl.Abs()
		if !ai.Equal(aj) {
			return ai.GreaterThan(aj)
		}
		if ranked[i].Ticker != ranked[j].Ticker {
			return ranked[i].Ticker < ranked[j].Ticker
		}
		return ranked[i].OpenedAt.Before(ranked[j].OpenedAt)
	})
	if topN > len(ranked) {
		topN = len(ranked)
	}
	var top decimal.Decimal
	for _, t := range ranked[:topN] {
		top = top.Add(t.NetPnl.Abs())
	}
	share, _ := top.Div(totalAbs).Float64()
	return share
}

func topTickerShare(byTicker map[string]*TickerAttribution, topN int, totalAbs decimal.Decimal) float64 {
	if totalAbs.Sign() <= 0 {
		return 0
	}
	ranked := make([]*TickerAttribution, 0, len(byTicker))
	for _, entry := range byTicker {
		if entry.RealizedPnl.IsZero() {
			continue
		}
		ranked = append(ranked, entry)
	}
	sort.Slice(ranked, func(i, j int) bool {
		ai := ranked[i].RealizedPnl.Abs()
		aj := ranked[j].RealizedPnl.Abs()
		if !ai.Equal(aj) {
			return ai.GreaterThan(aj)
		}
		return ranked[i].Ticker < ranked[j].Ticker
	})
	if len(ranked) == 0 {
		return 0
	}
	if topN > len(ranked) {
		topN = len(ranked)
	}
	var top decimal.Decimal
	for _, entry := range ranked[:topN] {
		top = top.Add(entry.RealizedPnl.Abs())
	}
	share, _ := top.Div(totalAbs).Float64()
	return share
}
