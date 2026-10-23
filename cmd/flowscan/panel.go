package main

import (
	"bufio"
	"encoding/json"
	"math"
	"os"
	"sort"
	"strings"
)

var admN = 100

const (
	exclFrom = "2022-02-25"
	exclTill = "2022-03-28"
)

type panel struct {
	dates    []string
	excluded []bool
	tickers  []string
	tIdx     map[string]int
	base     [][]*[nb]float32
	hi       [][]*hiRow
	divDrop  [][]bool
	adm      [][]int
	uniRet   []float64
	drops    []dropStat
	ycache   [][]*yvec
	meanA2   []float64
	ccache   [2][]*cvec
}

func newPanel(results []dayResult) *panel {
	p := &panel{tIdx: map[string]int{}}
	for _, r := range results {
		p.dates = append(p.dates, r.date)
		p.excluded = append(p.excluded, r.date >= exclFrom && r.date <= exclTill)
		p.drops = append(p.drops, r.drop)
		for sec := range r.recs {
			if _, ok := p.tIdx[sec]; !ok {
				p.tIdx[sec] = -1
			}
		}
	}
	secs := make([]string, 0, len(p.tIdx))
	for s := range p.tIdx {
		secs = append(secs, s)
	}
	sort.Strings(secs)
	p.tickers = secs
	for i, s := range secs {
		p.tIdx[s] = i
	}
	n := len(p.dates)
	p.base = make([][]*[nb]float32, len(secs))
	p.hi = make([][]*hiRow, len(secs))
	p.divDrop = make([][]bool, len(secs))
	for i := range secs {
		p.base[i] = make([]*[nb]float32, n)
		p.hi[i] = make([]*hiRow, n)
		p.divDrop[i] = make([]bool, n)
	}
	for d, r := range results {
		for sec, rec := range r.recs {
			i := p.tIdx[sec]
			p.base[i][d] = rec
			if h, ok := r.hi[sec]; ok {
				p.hi[i][d] = h
			}
		}
	}
	return p
}

func (p *panel) loadDividends(path string) int {
	fh, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer fh.Close()
	dateIdx := map[string]int{}
	for d, s := range p.dates {
		dateIdx[s] = d
	}
	nextDay := func(day string) int {
		for d, s := range p.dates {
			if s > day {
				return d
			}
		}
		return -1
	}
	cnt := 0
	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var rec struct {
			Ticker  string `json:"ticker"`
			LastBuy string `json:"last_buy_date"`
		}
		if json.Unmarshal([]byte(sc.Text()), &rec) != nil || len(rec.LastBuy) < 10 {
			continue
		}
		i, ok := p.tIdx[strings.TrimSpace(rec.Ticker)]
		if !ok {
			continue
		}
		ex := nextDay(rec.LastBuy[:10])
		if ex < 0 {
			continue
		}
		for _, d := range []int{ex - 1, ex, ex + 1} {
			if d >= 0 && d < len(p.dates) {
				p.divDrop[i][d] = true
			}
		}
		cnt++
	}
	return cnt
}

func (p *panel) v(i, d, k int) float64 {
	if d < 0 || d >= len(p.dates) {
		return nan
	}
	r := p.base[i][d]
	if r == nil {
		return nan
	}
	return float64(r[k])
}

func (p *panel) simpleOK(r float64) bool {
	return r > -0.45 && r < 0.80
}

func (p *panel) spanClean(i, d0, d1 int) bool {
	if d0 < 0 || d1 >= len(p.dates) || d0 > d1 {
		return false
	}
	for d := d0; d <= d1; d++ {
		if p.excluded[d] {
			return false
		}
		if d > d0 && p.divDrop[i][d] {
			return false
		}
	}
	return true
}

func (p *panel) ret(i, d0, d1 int) float64 {
	if !p.spanClean(i, d0, d1) {
		return nan
	}
	a, b := p.v(i, d0, bCloseDay), p.v(i, d1, bCloseDay)
	if !(a > 0) || !(b > 0) || !p.simpleOK(b/a-1) {
		return nan
	}
	return math.Log(b / a)
}

func (p *panel) logRet(i, d int) float64 {
	return p.ret(i, d-1, d)
}

func (p *panel) turnZ(i, d int) float64 {
	if d < 21 {
		return nan
	}
	sum, n := 0.0, 0
	for k := d - 20; k < d; k++ {
		if v := p.v(i, k, bValDay); v > 0 {
			sum += v
			n++
		}
	}
	cur := p.v(i, d, bValDay)
	if n < 10 || !(cur > 0) {
		return nan
	}
	return math.Log(cur / (sum / float64(n)))
}

func (p *panel) admittedAt(d int) []int {
	if p.adm == nil {
		p.adm = make([][]int, len(p.dates))
	}
	if p.adm[d] != nil || d < 60 {
		return p.adm[d]
	}
	type cand struct {
		i   int
		med float64
	}
	var cs []cand
	for i := range p.tickers {
		var vs []float64
		for k := d - 60; k < d; k++ {
			if v := p.v(i, k, bValDay); v > 0 {
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
	if len(cs) > admN {
		cs = cs[:admN]
	}
	ids := make([]int, len(cs))
	for k, c := range cs {
		ids[k] = c.i
	}
	p.adm[d] = ids
	return ids
}

func (p *panel) universeReturns() {
	p.uniRet = make([]float64, len(p.dates))
	for d := range p.dates {
		p.uniRet[d] = nan
		if d < 61 {
			continue
		}
		adm := p.admittedAt(d)
		s, n := 0.0, 0
		for _, i := range adm {
			if r := p.logRet(i, d); !math.IsNaN(r) {
				s += r
				n++
			}
		}
		if n >= 40 {
			p.uniRet[d] = s / float64(n)
		}
	}
}

func (p *panel) dateIndex(date string) int {
	return sort.SearchStrings(p.dates, date)
}
