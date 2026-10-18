package newsstudy

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

const Benchmark = "IMOEX"

var Moscow = time.FixedZone("MSK", 3*3600)

type Article struct {
	Ticker    string
	ArticleID string
	At        time.Time
}

func (a Article) Key() string { return a.Ticker + "|" + a.ArticleID }

type Outcome struct {
	Excess     map[int]float64
	Prior5     float64
	Turnover20 float64
}

func pct(entry, exit float64) float64 { return (exit - entry) / entry * 100 }

func medianValue(candles []moex.Candle, end, window int) float64 {
	if end-window+1 < 0 {
		return 0
	}
	vals := make([]float64, 0, window)
	for i := end - window + 1; i <= end; i++ {
		v, _ := candles[i].Value.Float64()
		vals = append(vals, v)
	}
	sort.Float64s(vals)
	return Quantile(vals, 0.5)
}

func BuildOutcomes(ctx context.Context, src backtest.HistoricalSource, arts []Article, horizons []int) (map[string]Outcome, error) {
	if len(arts) == 0 {
		return nil, nil
	}
	maxH := 0
	for _, h := range horizons {
		if h > maxH {
			maxH = h
		}
	}
	minT, maxT := arts[0].At, arts[0].At
	byTicker := make(map[string][]Article)
	for _, a := range arts {
		if a.At.Before(minT) {
			minT = a.At
		}
		if a.At.After(maxT) {
			maxT = a.At
		}
		byTicker[a.Ticker] = append(byTicker[a.Ticker], a)
	}
	from := minT.AddDate(0, 0, -45)
	till := maxT.AddDate(0, 0, maxH*2+10)
	idx, err := src.History(ctx, Benchmark, from, till)
	if err != nil {
		return nil, fmt.Errorf("history %s: %w", Benchmark, err)
	}
	idxByDate := make(map[string]moex.Candle, len(idx))
	for _, c := range idx {
		idxByDate[c.Begin.Format("2006-01-02")] = c
	}
	out := make(map[string]Outcome)
	for ticker, list := range byTicker {
		raw, err := src.History(ctx, ticker, from, till)
		if err != nil {
			continue
		}
		candles := raw[:0:0]
		for _, c := range raw {
			if _, ok := idxByDate[c.Begin.Format("2006-01-02")]; ok {
				candles = append(candles, c)
			}
		}
		for _, a := range list {
			pubDate := a.At.In(Moscow).Format("2006-01-02")
			pub := -1
			for i, c := range candles {
				if c.Begin.Format("2006-01-02") <= pubDate {
					pub = i
				} else {
					break
				}
			}
			if pub < 20 || pub+1 >= len(candles) {
				continue
			}
			entry := candles[pub+1]
			ie, ok := idxByDate[entry.Begin.Format("2006-01-02")]
			if !ok {
				continue
			}
			e, _ := entry.Open.Float64()
			ieo, _ := ie.Open.Float64()
			if e <= 0 || ieo <= 0 {
				continue
			}
			o := Outcome{Excess: make(map[int]float64)}
			complete := true
			for _, h := range horizons {
				if pub+1+h >= len(candles) {
					complete = false
					break
				}
				exit := candles[pub+1+h]
				ix, ok := idxByDate[exit.Begin.Format("2006-01-02")]
				if !ok {
					complete = false
					break
				}
				x, _ := exit.Close.Float64()
				ixc, _ := ix.Close.Float64()
				if x <= 0 || ixc <= 0 {
					complete = false
					break
				}
				o.Excess[h] = pct(e, x) - pct(ieo, ixc)
			}
			if !complete {
				continue
			}
			c0, _ := candles[pub].Close.Float64()
			c5, _ := candles[pub-5].Close.Float64()
			if c0 <= 0 || c5 <= 0 {
				continue
			}
			o.Prior5 = pct(c5, c0)
			o.Turnover20 = medianValue(candles, pub, 20)
			out[a.Key()] = o
		}
	}
	return out, nil
}
