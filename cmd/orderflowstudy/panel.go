package main

import (
	"math"
	"sort"
)

const (
	exclFrom = "2022-02-25"
	exclTill = "2022-03-28"
)

type panel struct {
	dates    []string
	excluded []bool
	tickers  []string
	tIdx     map[string]int
	recs     [][]*dayRec
	close    [][]float32
	val      [][]float32
	admitted map[int][][]int
}

func newPanel(results []dayResult) *panel {
	p := &panel{tIdx: map[string]int{}, admitted: map[int][][]int{}}
	for _, r := range results {
		p.dates = append(p.dates, r.date)
		p.excluded = append(p.excluded, r.date >= exclFrom && r.date <= exclTill)
		for _, rec := range r.recs {
			if _, ok := p.tIdx[rec.secid]; !ok {
				p.tIdx[rec.secid] = len(p.tickers)
				p.tickers = append(p.tickers, rec.secid)
			}
		}
	}
	n := len(p.dates)
	p.recs = make([][]*dayRec, len(p.tickers))
	p.close = make([][]float32, len(p.tickers))
	p.val = make([][]float32, len(p.tickers))
	for i := range p.tickers {
		p.recs[i] = make([]*dayRec, n)
		p.close[i] = make([]float32, n)
		p.val[i] = make([]float32, n)
	}
	for d, r := range results {
		for _, rec := range r.recs {
			i := p.tIdx[rec.secid]
			rec.day = d
			p.recs[i][d] = rec
			if !math.IsNaN(rec.closeDay) {
				p.close[i][d] = float32(rec.closeDay)
			}
			p.val[i][d] = float32(rec.valDay)
		}
	}
	return p
}

func (p *panel) simpleOK(r float64) bool {
	return r > -0.45 && r < 0.80
}

func (p *panel) logRet(i, d int) float64 {
	if d < 1 || d >= len(p.dates) || p.excluded[d] || p.excluded[d-1] {
		return nan
	}
	a, b := float64(p.close[i][d-1]), float64(p.close[i][d])
	if a <= 0 || b <= 0 || !p.simpleOK(b/a-1) {
		return nan
	}
	return math.Log(b / a)
}

func (p *panel) turnZ(i, d int) float64 {
	if d < 21 {
		return nan
	}
	sum, n := 0.0, 0
	for k := d - 20; k < d; k++ {
		if v := float64(p.val[i][k]); v > 0 {
			sum += v
			n++
		}
	}
	cur := float64(p.val[i][d])
	if n < 10 || cur <= 0 {
		return nan
	}
	return math.Log(cur / (sum / float64(n)))
}

func (p *panel) admittedAt(d, topN int) []int {
	if byN, ok := p.admitted[topN]; ok && byN != nil {
		return byN[d]
	}
	out := make([][]int, len(p.dates))
	for day := 60; day < len(p.dates); day++ {
		type cand struct {
			i   int
			med float64
		}
		var cs []cand
		for i := range p.tickers {
			var vs []float64
			for k := day - 60; k < day; k++ {
				if v := float64(p.val[i][k]); v > 0 {
					vs = append(vs, v)
				}
			}
			if len(vs) < 40 {
				continue
			}
			sort.Float64s(vs)
			cs = append(cs, cand{i, vs[len(vs)/2]})
		}
		sort.Slice(cs, func(a, b int) bool {
			if cs[a].med != cs[b].med {
				return cs[a].med > cs[b].med
			}
			return cs[a].i < cs[b].i
		})
		if len(cs) > topN {
			cs = cs[:topN]
		}
		ids := make([]int, len(cs))
		for k, c := range cs {
			ids[k] = c.i
		}
		out[day] = ids
	}
	p.admitted[topN] = out
	return out[d]
}

func (p *panel) half(d int) int {
	if p.dates[d] <= "2023-06-30" {
		return 0
	}
	return 1
}
