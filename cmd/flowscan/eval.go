package main

import (
	"math"
	"sort"
)

const (
	nCtrl     = 8
	costLive  = 0.0015
	costLow   = 0.0005
	minPerDay = 40
)

type okind int

const (
	kH1 okind = iota
	kH3
	kH5
	kH10
	kIntraSame
	kIntraNext
	kOvernight
	kOpenH1
	kOpenH5
	nKinds
)

func holdOf(k okind) int {
	switch k {
	case kH3:
		return 3
	case kH5, kOpenH5:
		return 5
	case kH10:
		return 10
	}
	return 1
}

func (p *panel) dayClean(i, d int) bool {
	return d >= 0 && d < len(p.dates) && !p.excluded[d] && !p.divDrop[i][d]
}

func (p *panel) rawOutcome(k okind, i, d int) float64 {
	switch k {
	case kH1:
		return p.ret(i, d, d+1)
	case kH3:
		return p.ret(i, d, d+3)
	case kH5:
		return p.ret(i, d, d+5)
	case kH10:
		return p.ret(i, d, d+10)
	case kIntraSame, kIntraNext:
		day := d
		if k == kIntraNext {
			day = d + 1
		}
		if !p.dayClean(i, day) {
			return nan
		}
		a, b := p.v(i, day, bClose10), p.v(i, day, bClose1830)
		if !(a > 0) || !(b > 0) || !p.simpleOK(b/a-1) {
			return nan
		}
		return math.Log(b / a)
	case kOvernight:
		if !p.spanClean(i, d, d+1) {
			return nan
		}
		a, b := p.v(i, d, bCloseDay), p.v(i, d+1, bOpen10)
		if !(a > 0) || !(b > 0) || !p.simpleOK(b/a-1) {
			return nan
		}
		return math.Log(b / a)
	case kOpenH1, kOpenH5:
		n := holdOf(k)
		if !p.spanClean(i, d, d+n) {
			return nan
		}
		a, b := p.v(i, d+1, bOpen10), p.v(i, d+n, bCloseDay)
		if !(a > 0) || !(b > 0) || !p.simpleOK(b/a-1) {
			return nan
		}
		return math.Log(b / a)
	}
	return nan
}

type yvec struct {
	ok  bool
	y   []float64
	raw []float64
}

func (p *panel) yAt(k okind, d int) *yvec {
	if p.ycache == nil {
		p.ycache = make([][]*yvec, nKinds)
		for i := range p.ycache {
			p.ycache[i] = make([]*yvec, len(p.dates))
		}
	}
	if c := p.ycache[k][d]; c != nil {
		return c
	}
	out := &yvec{}
	p.ycache[k][d] = out
	if d < 61 {
		return out
	}
	adm := p.admittedAt(d)
	raw := make([]float64, len(adm))
	s, n := 0.0, 0
	for j, i := range adm {
		raw[j] = p.rawOutcome(k, i, d)
		if !math.IsNaN(raw[j]) {
			s += raw[j]
			n++
		}
	}
	if n < minPerDay {
		return out
	}
	m := s / float64(n)
	y := make([]float64, len(adm))
	for j := range adm {
		y[j] = raw[j] - m
	}
	out.ok, out.y, out.raw = true, y, raw
	return out
}

type cvec struct {
	built bool
	c     [][nCtrl]float64
	ok    []bool
}

func (p *panel) ctrlAt(d, sh int) *cvec {
	if p.ccache[0] == nil {
		p.ccache = [2][]*cvec{make([]*cvec, len(p.dates)), make([]*cvec, len(p.dates))}
	}
	if c := p.ccache[sh][d]; c != nil {
		return c
	}
	out := &cvec{built: true}
	p.ccache[sh][d] = out
	if d < 61 {
		return out
	}
	adm := p.admittedAt(d)
	out.c = make([][nCtrl]float64, len(adm))
	out.ok = make([]bool, len(adm))
	for j, i := range adm {
		var c [nCtrl]float64
		dd := d - sh
		good := !p.divDrop[i][d]
		for l := 0; l <= 5 && good; l++ {
			c[l] = p.logRet(i, dd-l)
			good = !math.IsNaN(c[l])
		}
		if good {
			c[6] = p.turnZ(i, dd)
			c[7] = p.v(i, dd, bSpLv)
			good = !math.IsNaN(c[6]) && !math.IsNaN(c[7])
		}
		out.c[j], out.ok[j] = c, good
	}
	return out
}

type window struct {
	lo, hi, split int
}

type dayPort struct {
	d       int
	members []int
	w       map[int]float64
	ys      []float64
	gross   float64
	net     float64
	turn    float64
}

type evalOut struct {
	n            int
	coef, t      float64
	halfT        [2]float64
	halfCoef     [2]float64
	net, gross   [2]float64
	days         [2]int
	netAll       float64
	grossAll     float64
	turnAll      float64
	yearT        map[string]float64
	yearFlip     bool
	betas        []float64
	betaDays     []int
	ports        []dayPort
	netSeries    []float64
	netSeriesDay []int
}

func lagsFor(h int) int {
	if h > 5 {
		return h
	}
	return 5
}

type tradeOpts struct {
	hyst float64
	minN int
	frac float64
	hold int
	keep func(ti, d int) bool
}

func (p *panel) evaluate(X [][]float32, sign float64, kind okind, w window, drop []int, topts tradeOpts) evalOut {
	hold := holdOf(kind)
	if topts.hold > 0 {
		hold = topts.hold
	}
	frac := topts.frac
	if frac == 0 {
		frac = 0.2
	}
	dropSet := map[int]bool{}
	for _, c := range drop {
		dropSet[c] = true
	}
	var out evalOut
	out.yearT = map[string]float64{}
	yearBetas := map[string][]float64{}
	portAt := map[int]*dayPort{}
	stored := map[int]float64{}
	var h0, h1 []float64
	var net0, net1, gr0, gr1 float64
	var allNet, allGross, allTurn []float64
	for d := w.lo; d <= w.hi && d < len(p.dates); d++ {
		yv := p.yAt(kind, d)
		if !yv.ok {
			continue
		}
		adm := p.admittedAt(d)
		sh := 0
		if kind == kIntraSame {
			sh = 1
		}
		cv := p.ctrlAt(d, sh)
		if !cv.built || len(cv.ok) != len(adm) {
			continue
		}
		type pick struct {
			j  int
			x  float64
			ti int
		}
		var ps []pick
		for j, ti := range adm {
			x := float64(X[ti][d])
			if math.IsNaN(x) || math.IsInf(x, 0) || !cv.ok[j] || math.IsNaN(yv.y[j]) || p.divDrop[ti][d] {
				continue
			}
			if topts.keep != nil && !topts.keep(ti, d) {
				continue
			}
			ps = append(ps, pick{j, sign * x, ti})
		}
		minN := minPerDay
		if topts.minN > 0 {
			minN = topts.minN
		}
		if len(ps) < minN {
			continue
		}
		xs := make([]float64, len(ps))
		for k, q := range ps {
			xs[k] = q.x
		}
		rk := rankScale(xs)
		design := make([][]float64, len(ps))
		ys := make([]float64, len(ps))
		for k, q := range ps {
			row := make([]float64, 0, 2+nCtrl)
			row = append(row, 1, rk[k])
			for c := 0; c < nCtrl; c++ {
				if !dropSet[c] {
					row = append(row, cv.c[q.j][c])
				}
			}
			design[k] = row
			ys[k] = yv.y[q.j]
		}
		if beta, ok := ols(design, ys); ok {
			out.betas = append(out.betas, beta[1])
			out.betaDays = append(out.betaDays, d)
			if d < w.split {
				h0 = append(h0, beta[1])
			} else {
				h1 = append(h1, beta[1])
			}
			yr := p.dates[d][:4]
			yearBetas[yr] = append(yearBetas[yr], beta[1])
		}
		order := make([]int, len(ps))
		for k := range order {
			order[k] = k
		}
		score := make([]float64, len(ps))
		for k, q := range ps {
			score[k] = rk[k]
			if topts.hyst > 0 {
				if old, ok := stored[q.ti]; !ok || math.Abs(rk[k]-old) > topts.hyst {
					stored[q.ti] = rk[k]
				}
				score[k] = stored[q.ti]
			}
			_ = q
		}
		sort.Slice(order, func(a, b int) bool {
			if score[order[a]] != score[order[b]] {
				return score[order[a]] > score[order[b]]
			}
			return ps[order[a]].ti < ps[order[b]].ti
		})
		m := int(float64(len(ps)) * frac)
		if m < 3 {
			continue
		}
		dp := &dayPort{d: d, w: map[int]float64{}}
		g := 0.0
		for _, k := range order[:m] {
			dp.members = append(dp.members, ps[k].ti)
			dp.w[ps[k].ti] = 1 / float64(m)
			dp.ys = append(dp.ys, ys[k])
			g += ys[k]
		}
		g /= float64(m)
		prev := portAt[d-hold]
		turn := 0.0
		if prev == nil {
			turn = 1
		} else {
			for t, wt := range dp.w {
				turn += math.Abs(wt - prev.w[t])
			}
			for t, wt := range prev.w {
				if _, ok := dp.w[t]; !ok {
					turn += wt
				}
			}
		}
		dp.gross = g / float64(hold)
		dp.net = (g - costLive*turn) / float64(hold)
		dp.turn = turn
		portAt[d] = dp
		out.ports = append(out.ports, *dp)
		out.netSeries = append(out.netSeries, dp.net)
		out.netSeriesDay = append(out.netSeriesDay, d)
		allNet = append(allNet, dp.net)
		allGross = append(allGross, dp.gross)
		allTurn = append(allTurn, turn)
		if d < w.split {
			net0 += dp.net
			gr0 += dp.gross
			out.days[0]++
		} else {
			net1 += dp.net
			gr1 += dp.gross
			out.days[1]++
		}
	}
	lags := lagsFor(hold)
	out.n = len(out.betas)
	out.coef, out.t = neweyWest(out.betas, lags)
	m0, t0 := neweyWest(h0, lags)
	m1, t1 := neweyWest(h1, lags)
	out.halfCoef, out.halfT = [2]float64{m0, m1}, [2]float64{t0, t1}
	if out.days[0] > 0 {
		out.net[0], out.gross[0] = net0/float64(out.days[0]), gr0/float64(out.days[0])
	}
	if out.days[1] > 0 {
		out.net[1], out.gross[1] = net1/float64(out.days[1]), gr1/float64(out.days[1])
	}
	out.netAll, out.grossAll, out.turnAll = mean(allNet), mean(allGross), mean(allTurn)
	for yr, bs := range yearBetas {
		if len(bs) >= 60 {
			_, t := neweyWest(bs, lags)
			out.yearT[yr] = t
			if t <= -2.0 {
				out.yearFlip = true
			}
		}
	}
	return out
}
