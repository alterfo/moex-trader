package main

import (
	"fmt"
	"math"
	"sort"
)

const (
	costSide   = 0.0003
	costStress = 0.0010
	splitDate  = "2026-01-05"
)

var winLo, winHi = "2025-01-03", "2026-10-16"

var capRet = map[int]float64{1: 0.25, 5: 0.45}

type ctx struct {
	fo, fx      *store
	foF, fxF    featMat
	lo, hi, spl int
	fxLo, fxHi  int
	fxSpl       int
}

func dateWin(s *store) (int, int, int) {
	lo, hi, sp := -1, -1, -1
	for i, d := range s.dates {
		if lo < 0 && d >= winLo {
			lo = i
		}
		if d <= winHi {
			hi = i
		}
		if sp < 0 && d >= splitDate {
			sp = i
		}
	}
	return lo, hi, sp
}

func (s *store) sret(g string, d, h int) float64 {
	r := s.ret(g, d, h)
	if isNaN(r) || math.Abs(r) > capRet[h] {
		return nan
	}
	sg := s.sigma60(g, d)
	if isNaN(sg) || sg <= 0 {
		return nan
	}
	return r / (sg * math.Sqrt(float64(h)))
}

func clip(x float64) float64 {
	if x > 5 {
		return 5
	}
	if x < -5 {
		return -5
	}
	return x
}

func (s *store) ctrls(g string, d int) []float64 {
	sg := s.sigma60(g, d)
	if isNaN(sg) || sg <= 0 {
		return nil
	}
	r0, r1 := s.ret1Back(g, d), s.ret1Back(g, d-1)
	var v []float64
	for k := d - 5; k <= d-2; k++ {
		if r := s.ret1Back(g, k); !isNaN(r) {
			v = append(v, r)
		}
	}
	if isNaN(r0) || isNaN(r1) || len(v) < 3 {
		return nil
	}
	return []float64{clip(r0 / sg), clip(r1 / sg), clip(mean(v) / sg)}
}

type res struct {
	coef, t, p   float64
	hc, ht       [2]float64
	n            int
	net          [2]float64
	netAll       float64
	hasNet       bool
	netStress    [2]float64
	netStressAll float64
	dr2          float64
	note         string
}

func z60(series []float64, d int) float64 {
	var v []float64
	for k := d - 60; k < d; k++ {
		if k >= 0 && !isNaN(series[k]) {
			v = append(v, series[k])
		}
	}
	if len(v) < 40 || isNaN(series[d]) {
		return nan
	}
	sd := stdOf(v)
	if !(sd > 0) {
		return nan
	}
	return (series[d] - mean(v)) / sd
}

func medianDay(days []int) int {
	d := append([]int(nil), days...)
	sort.Ints(d)
	if len(d) == 0 {
		return 0
	}
	return d[len(d)/2]
}

func (s *store) provOK(g string, d int) bool {
	x := s.byGroup[g][d]
	return x != nil && (allowLate || x.lateTS <= 0.2)
}

func (c *ctx) split(sp rowSpec) int {
	var ds []int
	s := c.fo
	lo, hi := c.lo, c.hi
	switch sp.kind {
	case "fxts", "fxvol":
		s, lo, hi = c.fx, c.fxLo, c.fxHi
	}
	featSeries := func(st *store, F featMat, g, f string) []float64 {
		gi := st.gi(g)
		if gi < 0 {
			return nil
		}
		if f == "aBI" {
			v := make([]float64, len(st.dates))
			for k := range v {
				v[k] = math.Abs(F["BI"][gi][k])
			}
			return v
		}
		return F[f][gi]
	}
	switch sp.kind {
	case "fm", "vol", "disp", "breadth", "eqfm":
		for d := lo; d <= hi; d++ {
			L := s.liquid(d, 30)
			k := 0
			for gi, g := range s.groups {
				if sp.kind == "eqfm" {
					if _, ok := stockMap[g]; !ok {
						continue
					}
				} else if !L[g] {
					continue
				}
				if !s.provOK(g, d) {
					continue
				}
				ok := false
				if sp.inter[0] != "" {
					ok = !isNaN(c.foF[sp.inter[0]][gi][d]) && !isNaN(c.foF[sp.inter[1]][gi][d])
				} else if sp.feat == "aBI" {
					ok = !isNaN(c.foF["BI"][gi][d])
				} else {
					ok = !isNaN(c.foF[sp.feat][gi][d])
				}
				if ok {
					k++
				}
			}
			need := 20
			if sp.kind == "eqfm" {
				need = 8
			}
			if k >= need {
				ds = append(ds, d)
			}
		}
	case "agg":
		for d := lo; d <= hi; d++ {
			k := 0
			for _, g := range sp.groups {
				if gi := s.gi(g); gi >= 0 && s.provOK(g, d) && !isNaN(c.foF[sp.feat][gi][d]) {
					k++
				}
			}
			if k >= 3 {
				ds = append(ds, d)
			}
		}
	case "basis":
		b := c.basisSeries(sp.group, sp.group2)
		for d := lo; d <= hi; d++ {
			if !isNaN(b[d]) && s.provOK(sp.group, d) {
				ds = append(ds, d)
			}
		}
	case "fut2spot":
		series := featSeries(c.fo, c.foF, sp.group, sp.feat)
		for d := lo; d <= hi; d++ {
			fd, ok := c.fx.dIdx[c.fo.dates[d]]
			if ok && series != nil && !isNaN(z60(series, d)) && c.fx.provOK(sp.group2, fd) {
				ds = append(ds, d)
			}
		}
	case "eqts", "eqagg":
		series := featSeries(c.fo, c.foF, sp.group, sp.feat)
		for d := lo; d <= hi; d++ {
			if series != nil && !isNaN(z60(series, d)) && c.fo.provOK(sp.group, d) {
				ds = append(ds, d)
			}
		}
	default:
		F := c.foF
		if s == c.fx {
			F = c.fxF
		}
		series := featSeries(s, F, sp.group, sp.feat)
		for d := lo; d <= hi; d++ {
			if series != nil && !isNaN(z60(series, d)) && s.provOK(sp.group, d) {
				ds = append(ds, d)
			}
		}
	}
	return medianDay(ds)
}

func halfStats(coefs []float64, days []int, spl, lags int) ([2]float64, [2]float64, float64, float64) {
	var h0, h1 []float64
	for i, c := range coefs {
		if days[i] < spl {
			h0 = append(h0, c)
		} else {
			h1 = append(h1, c)
		}
	}
	m, t := neweyWest(coefs, lags)
	m0, t0 := neweyWest(h0, lags)
	m1, t1 := neweyWest(h1, lags)
	return [2]float64{m0, m1}, [2]float64{t0, t1}, m, t
}

func (c *ctx) featAt(featName string, gi, d int) float64 {
	if featName == "aBI" {
		v := c.foF["BI"][gi][d]
		return math.Abs(v)
	}
	return c.foF[featName][gi][d]
}

func (c *ctx) runFM(sp rowSpec) res {
	s := c.fo
	var coefs []float64
	var days []int
	var netv, netS []float64
	var netDays []int
	spl := c.split(sp)
	for d := c.lo; d <= c.hi; d++ {
		if s.excl[d] {
			continue
		}
		L := s.liquid(d, 30)
		var gs []int
		var fv, ys, raw []float64
		var X [][]float64
		for gi, g := range s.groups {
			if !L[g] {
				continue
			}
			var f float64
			if sp.inter[0] != "" {
				f = c.foF[sp.inter[0]][gi][d]
				if isNaN(c.foF[sp.inter[1]][gi][d]) {
					f = nan
				}
			} else {
				f = c.featAt(sp.feat, gi, d)
			}
			y := s.sret(g, d, sp.hor)
			ct := s.ctrls(g, d)
			tz := s.turnZ(g, d)
			if isNaN(f) || isNaN(y) || ct == nil || isNaN(tz) {
				continue
			}
			gs = append(gs, gi)
			fv = append(fv, f)
			ys = append(ys, y)
			raw = append(raw, s.ret(g, d, sp.hor))
			X = append(X, append(append([]float64{}, ct...), clip(tz)))
		}
		if len(gs) < 20 {
			continue
		}
		if sp.inter[0] != "" {
			a, b := make([]float64, len(gs)), make([]float64, len(gs))
			for k, gi := range gs {
				a[k], b[k] = c.foF[sp.inter[0]][gi][d], c.foF[sp.inter[1]][gi][d]
			}
			ra, rb := rankScale(a), rankScale(b)
			for k := range fv {
				fv[k] = ra[k] * (rb[k] + 0.5)
			}
		}
		rf := rankScale(fv)
		rawMean := mean(raw)
		for k := range raw {
			raw[k] -= rawMean
		}
		my := mean(ys)
		Y := make([]float64, len(ys))
		M := make([][]float64, len(ys))
		for k := range ys {
			Y[k] = ys[k] - my
			M[k] = append([]float64{1, rf[k]}, X[k]...)
		}
		b, ok := ols(M, Y)
		if !ok {
			continue
		}
		coefs = append(coefs, b[1])
		days = append(days, d)
		n := len(gs)
		idx := make([]int, n)
		for k := range idx {
			idx[k] = k
		}
		sg := sp.sign
		sort.SliceStable(idx, func(a, b int) bool { return sg*rf[idx[a]] > sg*rf[idx[b]] })
		q := n / 3
		var top, bot float64
		for k := 0; k < q; k++ {
			top += raw[idx[k]]
			bot += raw[idx[n-1-k]]
		}
		netv = append(netv, (top-bot)/float64(q)-4*costSide)
		netS = append(netS, (top-bot)/float64(q)-4*costStress)
		netDays = append(netDays, d)
	}
	r := res{n: len(coefs)}
	if len(coefs) < 80 {
		r.t, r.p = nan, nan
		return r
	}
	lags := lagsFor(sp.hor)
	r.hc, r.ht, r.coef, r.t = halfStats(coefs, days, spl, lags)
	r.p = normP2(r.t)
	nh, _, m, _ := halfStats(netv, netDays, spl, lags)
	r.net, r.netAll, r.hasNet = nh, m, true
	ns, _, ms, _ := halfStats(netS, netDays, spl, lags)
	r.netStress, r.netStressAll = ns, ms
	return r
}

func (c *ctx) runVolFM(sp rowSpec) res {
	s := c.fo
	logRV := func(g string, d int) float64 {
		x := s.byGroup[g][d]
		if x == nil || !(x.rv > 0) || (!allowLate && x.lateTS > 0.2) {
			return nan
		}
		return math.Log(x.rv)
	}
	avgRV := func(g string, d, n int) float64 {
		var v []float64
		for k := d - n + 1; k <= d; k++ {
			if k >= 0 {
				if x := logRV(g, k); !isNaN(x) {
					v = append(v, x)
				}
			}
		}
		if len(v) < n-1 {
			return nan
		}
		return mean(v)
	}
	var coefs []float64
	var days []int
	var dr2 []float64
	for d := c.lo; d <= c.hi; d++ {
		if s.excl[d] || d+sp.hor >= len(s.dates) {
			continue
		}
		L := s.liquid(d, 30)
		var fv, ys []float64
		var X [][]float64
		for gi, g := range s.groups {
			if !L[g] {
				continue
			}
			f := c.featAt(sp.feat, gi, d)
			if isNaN(f) {
				continue
			}
			var yv []float64
			bad := false
			for k := 1; k <= sp.hor; k++ {
				if s.excl[d+k] {
					bad = true
				}
				if x := logRV(g, d+k); isNaN(x) {
					bad = true
				} else {
					yv = append(yv, x)
				}
			}
			if bad {
				continue
			}
			c0, c5, c20, tz := logRV(g, d), avgRV(g, d, 5), avgRV(g, d, 20), s.turnZ(g, d)
			if isNaN(c0) || isNaN(c5) || isNaN(c20) || isNaN(tz) {
				continue
			}
			fv = append(fv, f)
			ys = append(ys, mean(yv))
			X = append(X, []float64{c0, c5, c20, tz})
		}
		if len(fv) < 20 {
			continue
		}
		n := len(fv)
		cols := make([][]float64, 5)
		cols[0] = rankScale(fv)
		for j := 0; j < 4; j++ {
			v := make([]float64, n)
			for k := range v {
				v[k] = X[k][j]
			}
			cols[j+1] = rankScale(v)
		}
		Y := rankScale(ys)
		M1 := make([][]float64, n)
		M0 := make([][]float64, n)
		for k := 0; k < n; k++ {
			M1[k] = []float64{1, cols[0][k], cols[1][k], cols[2][k], cols[3][k], cols[4][k]}
			M0[k] = []float64{1, cols[1][k], cols[2][k], cols[3][k], cols[4][k]}
		}
		b1, ok1 := ols(M1, Y)
		b0, ok0 := ols(M0, Y)
		if !ok1 || !ok0 {
			continue
		}
		coefs = append(coefs, b1[1])
		days = append(days, d)
		r2 := func(M [][]float64, b []float64) float64 {
			var sse, sst float64
			for k := range Y {
				p := 0.0
				for j := range b {
					p += b[j] * M[k][j]
				}
				sse += (Y[k] - p) * (Y[k] - p)
				sst += Y[k] * Y[k]
			}
			return 1 - sse/sst
		}
		dr2 = append(dr2, r2(M1, b1)-r2(M0, b0))
	}
	r := res{n: len(coefs)}
	if len(coefs) < 80 {
		r.t, r.p = nan, nan
		return r
	}
	lags := lagsFor(sp.hor)
	r.hc, r.ht, r.coef, r.t = halfStats(coefs, days, c.split(sp), lags)
	r.p = normP2(r.t)
	r.dr2 = mean(dr2)
	return r
}

func tsRun(x, y []float64, ctl [][]float64, days []int, spl, hor int) res {
	var X [][]float64
	var Y []float64
	var D []int
	for i := range x {
		if isNaN(x[i]) || isNaN(y[i]) || ctl[i] == nil {
			continue
		}
		X = append(X, append([]float64{1, x[i]}, ctl[i]...))
		Y = append(Y, y[i])
		D = append(D, days[i])
	}
	r := res{n: len(Y)}
	if len(Y) < 80 {
		r.t, r.p = nan, nan
		return r
	}
	lags := lagsFor(hor)
	b, t, ok := tsHAC(X, Y, lags)
	if !ok {
		r.t, r.p = nan, nan
		return r
	}
	r.coef, r.t, r.p = b, t, normP2(t)
	for h := 0; h < 2; h++ {
		var xh [][]float64
		var yh []float64
		for i := range Y {
			if (D[i] >= spl) == (h == 1) {
				xh = append(xh, X[i])
				yh = append(yh, Y[i])
			}
		}
		if len(yh) >= 40 {
			if bh, th, ok := tsHAC(xh, yh, lags); ok {
				r.hc[h], r.ht[h] = bh, th
			}
		}
	}
	return r
}

func (c *ctx) seriesFor(s *store, F featMat, g string, feat string, lo, hi int) ([]float64, []float64, [][]float64, []int) {
	gi := s.gi(g)
	var xs, ys []float64
	var cs [][]float64
	var ds []int
	if gi < 0 {
		return nil, nil, nil, nil
	}
	for d := lo; d <= hi; d++ {
		var raw []float64
		if feat == "aBI" {
			raw = make([]float64, len(s.dates))
			for k := range raw {
				raw[k] = math.Abs(F["BI"][gi][k])
			}
		} else {
			raw = F[feat][gi]
		}
		xs = append(xs, z60(raw, d))
		ds = append(ds, d)
		cs = append(cs, s.ctrls(g, d))
	}
	return xs, ys, cs, ds
}

func (c *ctx) runTS(sp rowSpec) res {
	s, F := c.fo, c.foF
	lo, hi := c.lo, c.hi
	xs, _, cs, ds := c.seriesFor(s, F, sp.group, sp.feat, lo, hi)
	if xs == nil {
		return res{t: nan, p: nan}
	}
	ys := make([]float64, len(ds))
	for i, d := range ds {
		ys[i] = s.sret(sp.group, d, sp.hor)
	}
	return tsRun(xs, ys, cs, ds, c.split(sp), sp.hor)
}

func (c *ctx) runFxTS(sp rowSpec) res {
	s, F := c.fx, c.fxF
	xs, _, cs, ds := c.seriesFor(s, F, sp.group, sp.feat, c.fxLo, c.fxHi)
	if xs == nil {
		return res{t: nan, p: nan}
	}
	ys := make([]float64, len(ds))
	for i, d := range ds {
		ys[i] = s.sret(sp.group, d, sp.hor)
	}
	return tsRun(xs, ys, cs, ds, c.split(sp), sp.hor)
}

func (c *ctx) runFxVol(sp rowSpec) res {
	s, F := c.fx, c.fxF
	g := sp.group
	gi := s.gi(g)
	if gi < 0 {
		return res{t: nan, p: nan}
	}
	logRV := func(d int) float64 {
		x := s.byGroup[g][d]
		if x == nil || !(x.rv > 0) || (!allowLate && x.lateTS > 0.2) {
			return nan
		}
		return math.Log(x.rv)
	}
	ab := make([]float64, len(s.dates))
	for k := range ab {
		ab[k] = math.Abs(F["BI"][gi][k])
	}
	var xs, ys []float64
	var cs [][]float64
	var ds []int
	for d := c.fxLo; d <= c.fxHi && d+1 < len(s.dates); d++ {
		var a5, a20 []float64
		for k := d - 4; k <= d; k++ {
			if x := logRV(k); !isNaN(x) {
				a5 = append(a5, x)
			}
		}
		for k := d - 19; k <= d; k++ {
			if k >= 0 {
				if x := logRV(k); !isNaN(x) {
					a20 = append(a20, x)
				}
			}
		}
		xs = append(xs, z60(ab, d))
		ys = append(ys, logRV(d+1))
		var ct []float64
		if len(a5) >= 4 && len(a20) >= 18 && !isNaN(logRV(d)) {
			ct = []float64{logRV(d), mean(a5), mean(a20)}
		}
		cs = append(cs, ct)
		ds = append(ds, d)
	}
	return tsRun(xs, ys, cs, ds, c.split(rowSpec{kind: "fxvol", feat: "aBI", group: sp.group, hor: 1}), 1)
}

func (c *ctx) runFut2Spot(sp rowSpec) res {
	fo, fx := c.fo, c.fx
	gi := fo.gi(sp.group)
	if gi < 0 || fx.gi(sp.group2) < 0 {
		return res{t: nan, p: nan}
	}
	var xs, ys []float64
	var cs [][]float64
	var ds []int
	raw := c.foF[sp.feat][gi]
	for d := c.lo; d <= c.hi; d++ {
		fd, ok := fx.dIdx[fo.dates[d]]
		if !ok {
			continue
		}
		xs = append(xs, z60(raw, d))
		ys = append(ys, fx.sret(sp.group2, fd, 1))
		cs = append(cs, fx.ctrls(sp.group2, fd))
		ds = append(ds, d)
	}
	return tsRun(xs, ys, cs, ds, c.split(sp), 1)
}

func (c *ctx) basisSeries(g1, g2 string) []float64 {
	fo, fx := c.fo, c.fx
	n := len(fo.dates)
	b := make([]float64, n)
	var ratios []float64
	for d := 0; d < n; d++ {
		b[d] = nan
		x := fo.byGroup[g1][d]
		fd, ok := fx.dIdx[fo.dates[d]]
		if x == nil || !ok {
			continue
		}
		y := fx.byGroup[g2][fd]
		if y == nil || !(y.closeMain > 0) {
			continue
		}
		ratios = append(ratios, x.closeMain/y.closeMain)
	}
	if len(ratios) < 100 {
		return b
	}
	k := math.Pow(10, math.Round(math.Log10(medianOf(ratios))))
	for d := 0; d < n; d++ {
		x := fo.byGroup[g1][d]
		fd, ok := fx.dIdx[fo.dates[d]]
		if x == nil || !ok {
			continue
		}
		y := fx.byGroup[g2][fd]
		if y == nil || !(y.closeMain > 0) {
			continue
		}
		b[d] = math.Log(x.closeMain / (k * y.closeMain))
	}
	return b
}

func (c *ctx) runBasis(sp rowSpec) res {
	fo := c.fo
	b := c.basisSeries(sp.group, sp.group2)
	fs := fo.frontSec[sp.group]
	n := len(b)
	var xs, ys []float64
	var cs [][]float64
	var ds []int
	for d := c.lo; d <= c.hi && d+1 < n; d++ {
		var v []float64
		for k := d - 20; k < d; k++ {
			if k >= 0 && fs[k] == fs[d] && !isNaN(b[k]) {
				v = append(v, b[k])
			}
		}
		x, y := nan, nan
		var ct []float64
		if len(v) >= 10 && !isNaN(b[d]) && fs[d] != "" && fs[d+1] == fs[d] && !isNaN(b[d+1]) && !fo.excl[d] && !fo.excl[d+1] && !fo.expired(fs[d], d) {
			if sd := stdOf(v); sd > 0 {
				x = (b[d] - mean(v)) / sd
				y = (b[d+1] - b[d]) * 100
			}
			if d >= 1 && fs[d-1] == fs[d] && !isNaN(b[d-1]) {
				ct = []float64{(b[d] - b[d-1]) * 100}
			}
		}
		xs, ys, cs, ds = append(xs, x), append(ys, y), append(cs, ct), append(ds, d)
	}
	return tsRun(xs, ys, cs, ds, c.split(sp), 1)
}

func (c *ctx) runAgg(sp rowSpec) res {
	s := c.fo
	var gis []int
	for _, g := range sp.groups {
		if i := s.gi(g); i >= 0 {
			gis = append(gis, i)
		}
	}
	nd := len(s.dates)
	xr := make([]float64, nd)
	for d := range xr {
		xr[d] = nan
		var v []float64
		for _, gi := range gis {
			if x := c.foF[sp.feat][gi][d]; !isNaN(x) {
				v = append(v, x)
			}
		}
		if len(v) >= 3 {
			xr[d] = mean(v)
		}
	}
	var xs, ys []float64
	var cs [][]float64
	var ds []int
	for d := c.lo; d <= c.hi; d++ {
		var yv []float64
		var c0 [][]float64
		for _, gi := range gis {
			g := s.groups[gi]
			if y := s.sret(g, d, sp.hor); !isNaN(y) {
				yv = append(yv, y)
				if ct := s.ctrls(g, d); ct != nil {
					c0 = append(c0, ct)
				}
			}
		}
		y := nan
		var ct []float64
		if len(yv) >= 3 && len(c0) >= 3 {
			y = mean(yv)
			ct = make([]float64, 3)
			for j := 0; j < 3; j++ {
				for _, cc := range c0 {
					ct[j] += cc[j]
				}
				ct[j] /= float64(len(c0))
			}
		}
		xs, ys, cs, ds = append(xs, z60(xr, d)), append(ys, y), append(cs, ct), append(ds, d)
	}
	return tsRun(xs, ys, cs, ds, c.split(sp), sp.hor)
}

func (c *ctx) runCross(sp rowSpec) res {
	s := c.fo
	nd := len(s.dates)
	xr := make([]float64, nd)
	yr := make([]float64, nd)
	cr := make([][]float64, nd)
	for d := 0; d < nd; d++ {
		xr[d], yr[d] = nan, nan
		if d < c.lo-60 || d > c.hi {
			continue
		}
		L := s.liquid(d, 30)
		var bi, ys []float64
		var c0 [][]float64
		for gi, g := range s.groups {
			if !L[g] {
				continue
			}
			f := c.foF["BI"][gi][d]
			if isNaN(f) {
				continue
			}
			y := s.sret(g, d, 1)
			ct := s.ctrls(g, d)
			if isNaN(y) || ct == nil {
				continue
			}
			bi = append(bi, f)
			ys = append(ys, y)
			c0 = append(c0, ct)
		}
		if len(bi) < 20 {
			continue
		}
		if sp.kind == "disp" {
			xr[d] = stdOf(bi)
			a := make([]float64, len(ys))
			for k := range ys {
				a[k] = math.Abs(ys[k])
			}
			yr[d] = mean(a)
		} else {
			pos := 0.0
			for _, v := range bi {
				if v > 0 {
					pos++
				}
			}
			xr[d] = pos / float64(len(bi))
			yr[d] = mean(ys)
		}
		ct := make([]float64, 3)
		for j := 0; j < 3; j++ {
			for _, cc := range c0 {
				ct[j] += cc[j]
			}
			ct[j] /= float64(len(c0))
		}
		cr[d] = ct
	}
	var xs, ys []float64
	var cs [][]float64
	var ds []int
	for d := c.lo; d <= c.hi; d++ {
		xs, ys, cs, ds = append(xs, z60(xr, d)), append(ys, yr[d]), append(cs, cr[d]), append(ds, d)
	}
	return tsRun(xs, ys, cs, ds, c.split(sp), 1)
}

func (c *ctx) run(sp rowSpec) res {
	switch sp.kind {
	case "fm":
		return c.runFM(sp)
	case "vol":
		return c.runVolFM(sp)
	case "ts":
		return c.runTS(sp)
	case "fxts":
		return c.runFxTS(sp)
	case "fxvol":
		return c.runFxVol(sp)
	case "fut2spot":
		return c.runFut2Spot(sp)
	case "basis":
		return c.runBasis(sp)
	case "agg":
		return c.runAgg(sp)
	case "disp", "breadth":
		return c.runCross(sp)
	}
	return res{t: nan, p: nan}
}

func (r res) String(sp rowSpec) string {
	s := fmt.Sprintf("%s %s n %d coef %+.5f t %+.2f p %.5f halves t %+.2f / %+.2f coef %+.5f / %+.5f", sp.id, sp.kind, r.n, r.coef, r.t, r.p, r.ht[0], r.ht[1], r.hc[0], r.hc[1])
	if r.hasNet {
		s += fmt.Sprintf(" tercile L/S abnormal net@0.03%% %+.4f%% (halves %+.4f%% / %+.4f%%), @0.10%% %+.4f%% (halves %+.4f%% / %+.4f%%)", r.netAll*100, r.net[0]*100, r.net[1]*100, r.netStressAll*100, r.netStress[0]*100, r.netStress[1]*100)
	}
	if sp.kind == "vol" {
		s += fmt.Sprintf(" dR2 %+.4f", r.dr2)
	}
	return s
}
