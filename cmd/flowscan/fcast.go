package main

import (
	"fmt"
	"math"
	"sort"
)

type fcFeature struct {
	id string
	fn featFn
}

func absOf(f featFn) featFn {
	return func(p *panel, d int, adm []int) []float64 {
		v := f(p, d, adm)
		for i := range v {
			v[i] = math.Abs(v[i])
		}
		return v
	}
}

func fcFeatures() []fcFeature {
	return []fcFeature{
		{"A2abs", absOf(col(bA2))},
		{"H002", logRatioM(bDepth, 20, 10)},
		{"H015", logRatioM(bSpLv, 20, 10)},
		{"A5", col(bA5)},
		{"H047", zPrev(bPutOverVal, ident)},
		{"H025", zPrev(bTicket, lg)},
		{"H003abs", absOf(col(bLevAsym))},
	}
}

type fcTarget struct {
	name  string
	hor   int
	value func(p *panel, i, d int) float64
	ctrls []func(p *panel, i, d int) float64
}

func logPos(x float64) float64 {
	if !(x > 0) {
		return nan
	}
	return math.Log(x)
}

func avgOver(f func(p *panel, i, d int) float64, lo, hi int) func(p *panel, i, d int) float64 {
	return func(p *panel, i, d int) float64 {
		s, n := 0.0, 0
		for k := d + lo; k <= d+hi; k++ {
			if v := f(p, i, k); !math.IsNaN(v) {
				s += v
				n++
			}
		}
		need := (hi - lo + 1)
		if need > 1 {
			need = need - 1
		}
		if n < need {
			return nan
		}
		return s / float64(n)
	}
}

func logRV(p *panel, i, d int) float64 { return logPos(p.v(i, d, bRV)) }
func logSp(p *panel, i, d int) float64 { return logPos(p.v(i, d, bSpLv)) }

func volTargets() []fcTarget {
	ctr := []func(p *panel, i, d int) float64{
		logRV,
		avgOver(logRV, -4, 0),
		avgOver(logRV, -19, 0),
		func(p *panel, i, d int) float64 { return p.turnZ(i, d) },
	}
	return []fcTarget{
		{"B1 logRV d+1", 1, func(p *panel, i, d int) float64 { return logRV(p, i, d+1) }, ctr},
		{"B2 logRV mean d+1..d+5", 5, avgOver(logRV, 1, 5), ctr},
	}
}

func spreadTargets() []fcTarget {
	ctr := []func(p *panel, i, d int) float64{
		logSp,
		avgOver(logSp, -19, 0),
		func(p *panel, i, d int) float64 { return p.turnZ(i, d) },
	}
	return []fcTarget{
		{"C1 logSpread d+1", 1, func(p *panel, i, d int) float64 { return logSp(p, i, d+1) }, ctr},
	}
}

type fcResult struct {
	label    string
	coef, t  float64
	halfCoef [2]float64
	halfT    [2]float64
	n        int
	p        float64
}

func (r *run) fcastFM(tg fcTarget, ft fcFeature) fcResult {
	p := r.p
	var all, h0, h1 []float64
	for d := r.win.lo; d <= r.win.hi && d+tg.hor < len(p.dates); d++ {
		if d < 61 || p.excluded[d] {
			continue
		}
		adm := p.admittedAt(d)
		fv := ft.fn(p, d, adm)
		var ys []float64
		var rows [][]float64
		var cols [][]float64
		for k, i := range adm {
			if math.IsNaN(fv[k]) || math.IsInf(fv[k], 0) {
				continue
			}
			bad := false
			for s := d; s <= d+tg.hor; s++ {
				if p.excluded[s] || p.divDrop[i][s] {
					bad = true
				}
			}
			if bad {
				continue
			}
			y := tg.value(p, i, d)
			if math.IsNaN(y) {
				continue
			}
			row := []float64{fv[k]}
			for _, c := range tg.ctrls {
				v := c(p, i, d)
				if math.IsNaN(v) || math.IsInf(v, 0) {
					row = nil
					break
				}
				row = append(row, v)
			}
			if row == nil {
				continue
			}
			ys = append(ys, y)
			rows = append(rows, row)
		}
		if len(ys) < 40 {
			continue
		}
		nc := len(rows[0])
		cols = make([][]float64, nc)
		for c := 0; c < nc; c++ {
			v := make([]float64, len(rows))
			for i := range rows {
				v[i] = rows[i][c]
			}
			cols[c] = rankScale(v)
		}
		yr := rankScale(ys)
		X := make([][]float64, len(rows))
		for i := range rows {
			x := make([]float64, nc+1)
			x[0] = 1
			for c := 0; c < nc; c++ {
				x[c+1] = cols[c][i]
			}
			X[i] = x
		}
		beta, ok := ols(X, yr)
		if !ok {
			continue
		}
		all = append(all, beta[1])
		if d < r.win.split {
			h0 = append(h0, beta[1])
		} else {
			h1 = append(h1, beta[1])
		}
	}
	lags := lagsFor(tg.hor)
	m, t := neweyWest(all, lags)
	m0, t0 := neweyWest(h0, lags)
	m1, t1 := neweyWest(h1, lags)
	return fcResult{label: tg.name + " / " + ft.id, coef: m, t: t, halfCoef: [2]float64{m0, m1}, halfT: [2]float64{t0, t1}, n: len(all), p: 2 * (1 - normCDF(math.Abs(t)))}
}

func normCDF(x float64) float64 { return 0.5 * math.Erfc(-x/math.Sqrt2) }

func (r *run) forecastFamily(name string, targets []fcTarget, q float64) {
	w := r.w
	feats := fcFeatures()
	var res []fcResult
	for _, tg := range targets {
		for _, ft := range feats {
			x := r.fcastFM(tg, ft)
			res = append(res, x)
			fmt.Fprintf(w, "%s coef %+.5f NW t %+.2f halves t %+.2f / %+.2f coef %+.5f / %+.5f dates %d\n", x.label, x.coef, x.t, x.halfT[0], x.halfT[1], x.halfCoef[0], x.halfCoef[1], x.n)
		}
	}
	idx := make([]int, len(res))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return res[idx[a]].p < res[idx[b]].p })
	m := len(res)
	cut := -1
	for k, i := range idx {
		if !math.IsNaN(res[i].p) && res[i].p <= q*float64(k+1)/float64(m) {
			cut = k
		}
	}
	pass := 0
	fmt.Fprintf(w, "\n== Family %s: pooled |t| >= 3.0, same sign in both halves, BH q=%.2f over %d tests\n", name, q, m)
	for k, i := range idx {
		x := res[i]
		same := x.halfCoef[0]*x.halfCoef[1] > 0 && x.halfCoef[0]*x.coef > 0
		ok := math.Abs(x.t) >= 3.0 && same && k <= cut
		if ok {
			pass++
		}
		fmt.Fprintf(w, "%s p %.5f BH %v same-sign %v -> %s\n", x.label, x.p, k <= cut, same, passWord(ok))
	}
	fmt.Fprintf(w, "family %s passed %d of %d\n", name, pass, m)
}
