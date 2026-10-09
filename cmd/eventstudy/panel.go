package main

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/features"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
	"github.com/olegsidorkin/moex-trader/internal/model"
)

type panel struct {
	tickers    []string
	keys       []int
	dates      []string
	ar         [][]float64
	mkt        []float64
	studyStart int
	artCount   [][]int
}

type ev struct {
	tk int
	t0 int
}

type topicData struct {
	name    string
	flagged [][]bool
	events  []ev
	rawDays []int
}

type topicDef struct {
	name    string
	primary bool
	match   func(title string) bool
}

func topicDefs() []topicDef {
	return []topicDef{
		{"sanctions", true, func(t string) bool { return features.DetectEvents(t).Sanctions == 1 }},
		{"negotiations", true, func(t string) bool { return features.DetectEvents(t).Negotiations == 1 }},
		{"incident", true, features.DetectIncident},
		{"dividend", false, func(t string) bool { return features.DetectEvents(t).Dividend == 1 }},
		{"report", false, func(t string) bool { return features.DetectEvents(t).Report == 1 }},
		{"buyback", false, func(t string) bool { return features.DetectEvents(t).Buyback == 1 }},
		{"mna", false, func(t string) bool { return features.DetectEvents(t).MNA == 1 }},
	}
}

func dateKey(t time.Time) int {
	y, m, d := t.Date()
	return y*10000 + int(m)*100 + d
}

func toF(d decimal.Decimal) float64 {
	f, _ := d.Float64()
	return f
}

func dropIncompleteTrailing(candles []moex.Candle, now time.Time) []moex.Candle {
	if len(candles) == 0 {
		return candles
	}
	last := candles[len(candles)-1]
	if dateKey(last.Begin) == dateKey(now) && now.Hour() < 19 {
		return candles[:len(candles)-1]
	}
	return candles
}

func loadPanel(ctx context.Context, baseURL string, tickers []string, histFrom, till, studyFrom time.Time, arMode string) (*panel, error) {
	client := moex.NewClient(baseURL, nil)
	src := backtest.NewISSSource(baseURL, client)
	msk, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		return nil, err
	}
	now := time.Now().In(msk)

	imoex, err := src.History(ctx, "IMOEX", histFrom, till)
	if err != nil {
		return nil, fmt.Errorf("imoex history: %w", err)
	}
	imoex = dropIncompleteTrailing(imoex, now)
	p := &panel{tickers: tickers}
	imoexClose := make([]float64, len(imoex))
	for i, c := range imoex {
		p.keys = append(p.keys, dateKey(c.Begin))
		p.dates = append(p.dates, c.Begin.Format("2006-01-02"))
		imoexClose[i] = toF(c.Close)
	}
	n := len(p.keys)
	p.mkt = make([]float64, n)
	p.mkt[0] = math.NaN()
	for i := 1; i < n; i++ {
		p.mkt[i] = imoexClose[i]/imoexClose[i-1] - 1
	}
	studyKey := dateKey(studyFrom)
	p.studyStart = sort.SearchInts(p.keys, studyKey)

	keyIdx := make(map[int]int, n)
	for i, k := range p.keys {
		keyIdx[k] = i
	}
	for _, tk := range tickers {
		candles, err := src.History(ctx, tk, histFrom, till)
		if err != nil {
			return nil, fmt.Errorf("%s history: %w", tk, err)
		}
		candles = dropIncompleteTrailing(candles, now)
		closes := make([]float64, n)
		for i := range closes {
			closes[i] = math.NaN()
		}
		for _, c := range candles {
			if i, ok := keyIdx[dateKey(c.Begin)]; ok {
				closes[i] = toF(c.Close)
			}
		}
		r := make([]float64, n)
		r[0] = math.NaN()
		for i := 1; i < n; i++ {
			r[i] = closes[i]/closes[i-1] - 1
		}
		alpha, beta := 0.0, 1.0
		if arMode == "beta" {
			var xs [][]float64
			var ys []float64
			for i := 1; i < p.studyStart; i++ {
				if !math.IsNaN(r[i]) && !math.IsNaN(p.mkt[i]) {
					xs = append(xs, []float64{1, p.mkt[i]})
					ys = append(ys, r[i])
				}
			}
			if len(xs) >= 60 {
				if coef, ok := olsFit(xs, ys, 0); ok {
					alpha, beta = coef[0], coef[1]
				}
			}
		}
		ar := make([]float64, n)
		for i := range ar {
			if math.IsNaN(r[i]) || math.IsNaN(p.mkt[i]) {
				ar[i] = math.NaN()
				continue
			}
			ar[i] = r[i] - alpha - beta*p.mkt[i]
		}
		p.ar = append(p.ar, ar)
	}
	return p, nil
}

func (p *panel) eventDay(ts time.Time, msk *time.Location) int {
	local := ts.In(msk)
	target := dateKey(local)
	if local.Hour() >= 19 {
		target = dateKey(local.AddDate(0, 0, 1))
	}
	i := sort.SearchInts(p.keys, target)
	if i >= len(p.keys) {
		return -1
	}
	return i
}

func buildTopics(p *panel, records []model.HistoricalNewsRecord, defs []topicDef, dedupGap int) ([]topicData, error) {
	msk, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		return nil, err
	}
	tkIdx := make(map[string]int, len(p.tickers))
	for i, t := range p.tickers {
		tkIdx[t] = i
	}
	n := len(p.keys)
	p.artCount = make([][]int, len(p.tickers))
	for i := range p.artCount {
		p.artCount[i] = make([]int, n)
	}
	out := make([]topicData, len(defs))
	for d := range defs {
		out[d].name = defs[d].name
		out[d].flagged = make([][]bool, len(p.tickers))
		for i := range out[d].flagged {
			out[d].flagged[i] = make([]bool, n)
		}
	}
	for d := range out {
		out[d].rawDays = make([]int, n)
	}
	seen := make([]map[string]bool, len(defs))
	for i := range seen {
		seen[i] = make(map[string]bool)
	}
	for _, r := range records {
		ti, ok := tkIdx[r.Ticker]
		if !ok || r.TrustWeight <= 0 || r.Title == "" {
			continue
		}
		t0 := p.eventDay(r.PublishedAt, msk)
		if t0 < p.studyStart {
			continue
		}
		p.artCount[ti][t0]++
		for d, def := range defs {
			if !def.match(r.Title) {
				continue
			}
			out[d].flagged[ti][t0] = true
			if !seen[d][r.ArticleID] {
				seen[d][r.ArticleID] = true
				out[d].rawDays[t0]++
			}
		}
	}
	for d := range out {
		for ti := range p.tickers {
			last := -1000
			for t := p.studyStart; t < n; t++ {
				if !out[d].flagged[ti][t] {
					continue
				}
				if t-last <= dedupGap {
					continue
				}
				out[d].events = append(out[d].events, ev{tk: ti, t0: t})
				last = t
			}
		}
	}
	return out, nil
}

func (p *panel) car(tk, t0, a, b int) (float64, bool) {
	if t0+a < 0 || t0+b >= len(p.keys) {
		return 0, false
	}
	s := 0.0
	for t := t0 + a; t <= t0+b; t++ {
		v := p.ar[tk][t]
		if math.IsNaN(v) {
			return 0, false
		}
		s += v
	}
	return s, true
}

func (p *panel) meanAR(tk, from, to int) (float64, bool) {
	if from < 0 || to >= len(p.keys) || from > to {
		return 0, false
	}
	s := 0.0
	for t := from; t <= to; t++ {
		v := p.ar[tk][t]
		if math.IsNaN(v) {
			return 0, false
		}
		s += v
	}
	return s / float64(to-from+1), true
}
