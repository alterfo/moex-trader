package main

import (
	"fmt"
	"math"
	"sort"
)

type ctx3 struct {
	c     *ctx
	e     *eqData
	eb    *eqBlock
	hi    hi2Map
	eqFo  map[string][]float64
	expWk []float64
}

func newCtx3(c *ctx, e *eqData, eb *eqBlock, hi hi2Map) *ctx3 {
	x := &ctx3{c: c, e: e, eb: eb, hi: hi, eqFo: map[string][]float64{}}
	for _, n := range eqNames {
		x.eqFo[n] = alignToFo(c, e, eb.series[n])
	}
	x.expWk = expiryWeek(c.fo.dates)
	return x
}

func (x *ctx3) series(sp rowSpec) []float64 {
	switch sp.kind {
	case "e2f", "e2fx":
		return x.eqFo[sp.feat]
	case "hi2ts":
		return x.hi.series(x.c.fo, sp.group, sp.feat)
	}
	return nil
}

func (x *ctx3) split(sp rowSpec) int {
	c := x.c
	var ds []int
	switch sp.kind {
	case "e2f", "hi2ts":
		ser := x.series(sp)
		for d := c.lo; d <= c.hi; d++ {
			if !isNaN(z60(ser, d)) && c.fo.provOK(sp.group, d) {
				ds = append(ds, d)
			}
		}
	case "e2fx":
		ser := x.series(sp)
		for d := c.lo; d <= c.hi; d++ {
			fd, ok := c.fx.dIdx[c.fo.dates[d]]
			if ok && !isNaN(z60(ser, d)) && c.fx.provOK(sp.group, fd) {
				ds = append(ds, d)
			}
		}
	case "hi2fm":
		for d := c.lo; d <= c.hi; d++ {
			L := c.fo.liquid(d, 30)
			k := 0
			for _, g := range c.fo.groups {
				if L[g] && c.fo.provOK(g, d) && !isNaN(x.hi.at(c.fo, g, d, sp.feat)) {
					k++
				}
			}
			if k >= 20 {
				ds = append(ds, d)
			}
		}
	}
	return medianDay(ds)
}

func (x *ctx3) run(sp rowSpec) res {
	c := x.c
	fo := c.fo
	switch sp.kind {
	case "e2f", "hi2ts":
		ser := x.series(sp)
		var xs, ys []float64
		var cs [][]float64
		var ds []int
		for d := c.lo; d <= c.hi; d++ {
			z := z60(ser, d)
			var y float64
			if sp.kind == "hi2ts" || sp.group2 == "oc1" {
				y = fo.sretOC(sp.group, d)
			} else {
				y = fo.sret(sp.group, d, sp.hor)
			}
			ct := fo.ctrls(sp.group, d)
			xv := z
			if ct != nil && sp.inter[0] == "expiryweek" {
				w := x.expWk[d]
				xv = z * w
				ct = append([]float64{z, w}, ct...)
			}
			xs, ys, cs, ds = append(xs, xv), append(ys, y), append(cs, ct), append(ds, d)
		}
		return tsRun(xs, ys, cs, ds, x.split(sp), sp.hor)
	case "e2fx":
		ser := x.series(sp)
		fx := c.fx
		var xs, ys []float64
		var cs [][]float64
		var ds []int
		for d := c.lo; d <= c.hi; d++ {
			fd, ok := fx.dIdx[fo.dates[d]]
			if !ok {
				continue
			}
			xs, ys, cs, ds = append(xs, z60(ser, d)), append(ys, fx.sret(sp.group, fd, 1)), append(cs, fx.ctrls(sp.group, fd)), append(ds, d)
		}
		return tsRun(xs, ys, cs, ds, x.split(sp), 1)
	case "hi2fm":
		var coefs []float64
		var days []int
		for d := c.lo; d <= c.hi; d++ {
			L := fo.liquid(d, 30)
			var fv, ys []float64
			var X [][]float64
			for _, g := range fo.groups {
				if !L[g] {
					continue
				}
				f := x.hi.at(fo, g, d, sp.feat)
				y := fo.sretOC(g, d)
				ct := fo.ctrls(g, d)
				tz := fo.turnZ(g, d)
				if isNaN(f) || isNaN(y) || ct == nil || isNaN(tz) {
					continue
				}
				fv, ys, X = append(fv, f), append(ys, y), append(X, append(append([]float64{}, ct...), clip(tz)))
			}
			if len(fv) < 20 {
				continue
			}
			rf := rankScale(fv)
			my := mean(ys)
			M := make([][]float64, len(ys))
			Y := make([]float64, len(ys))
			for k := range ys {
				Y[k] = ys[k] - my
				M[k] = append([]float64{1, rf[k]}, X[k]...)
			}
			b, ok := ols(M, Y)
			if !ok {
				continue
			}
			coefs, days = append(coefs, b[1]), append(days, d)
		}
		r := res{n: len(coefs)}
		if len(coefs) < 80 {
			r.t, r.p = nan, nan
			return r
		}
		r.hc, r.ht, r.coef, r.t = halfStats(coefs, days, x.split(sp), lagsFor(sp.hor))
		r.p = normP2(r.t)
		return r
	}
	return res{t: nan, p: nan}
}

func (x *ctx3) runThin(sp rowSpec) res {
	e, eb := x.e, x.eb
	var feat map[string][]float64
	switch sp.feat {
	case "a2":
		feat = eb.a2
	case "a2m5":
		feat = eb.a2m5
	case "tz":
		feat = eb.tz
	case "sr":
		feat = eb.sr
	}
	lo := e.dateIdxAtOrAfter(winLo)
	hi := e.dateIdxAtOrAfter(winHi)
	if hi >= len(e.dates) || e.dates[hi] > winHi {
		hi--
	}
	var coefs []float64
	var days []int
	var elig []int
	for d := lo; d <= hi; d++ {
		var names []string
		for t := range eb.band2 {
			if eb.band2[t][d] && !isNaN(feat[t][d]) {
				names = append(names, t)
			}
		}
		sort.Strings(names)
		if len(names) >= 40 {
			elig = append(elig, d)
		}
		type obs struct {
			f, y, ex float64
			ct       []float64
		}
		var os []obs
		for _, t := range names {
			r := e.ret(t, d, sp.hor)
			sg := e.sigma60(t, d)
			ct := e.ctrls(t, d)
			if isNaN(r) || isNaN(sg) || sg <= 0 || ct == nil {
				continue
			}
			ex := 0.0
			if fl := eb.exWk[t]; fl != nil && fl[d] {
				ex = 1
			}
			os = append(os, obs{feat[t][d], r / (sg * math.Sqrt(float64(sp.hor))), ex, ct})
		}
		if len(os) < 40 {
			continue
		}
		fv := make([]float64, len(os))
		for k := range os {
			fv[k] = os[k].f
		}
		rf := rankScale(fv)
		M := make([][]float64, len(os))
		Y := make([]float64, len(os))
		my := 0.0
		for k := range os {
			Y[k] = os[k].y
			my += Y[k]
		}
		my /= float64(len(Y))
		for k := range os {
			Y[k] -= my
			if sp.inter[0] == "exweek" {
				M[k] = append([]float64{1, rf[k] * os[k].ex, rf[k], os[k].ex}, os[k].ct...)
			} else {
				M[k] = append([]float64{1, rf[k]}, os[k].ct...)
			}
		}
		b, ok := ols(M, Y)
		if !ok {
			continue
		}
		coefs, days = append(coefs, b[1]), append(days, d)
	}
	r := res{n: len(coefs)}
	if len(coefs) < 80 {
		r.t, r.p = nan, nan
		return r
	}
	r.hc, r.ht, r.coef, r.t = halfStats(coefs, days, medianDay(elig), lagsFor(sp.hor))
	r.p = normP2(r.t)
	return r
}

func (x *ctx3) prep() {
	c := x.c
	var names []string
	var sers [][]float64
	for _, n := range eqNames {
		names = append(names, n)
		sers = append(sers, x.eqFo[n])
	}
	for _, p := range [][2]string{{"MIX", "hhi_agressive"}, {"MIX", "hhi_passive"}, {"MIX", "agr_diff"}, {"MIX", "net_diff"}, {"Si", "hhi_agressive"}, {"Si", "agr_diff"}} {
		names = append(names, "hi2:"+p[0]+":"+p[1])
		sers = append(sers, x.hi.series(c.fo, p[0], p[1]))
	}
	k := len(sers)
	zs := make([][]float64, k)
	for i := range sers {
		zs[i] = make([]float64, len(c.fo.dates))
		for d := range zs[i] {
			zs[i][d] = z60(sers[i], d)
		}
	}
	cut := func(v []float64) []float64 {
		o := make([]float64, len(v))
		for i := range o {
			o[i] = nan
			if i >= c.lo && i <= c.hi && c.fo.provOK("MIX", i) {
				o[i] = v[i]
			}
		}
		return o
	}
	R := make([][]float64, k)
	for i := range R {
		R[i] = make([]float64, k)
		for j := range R[i] {
			R[i][j] = corr(cut(zs[i]), cut(zs[j]))
			if i == j {
				R[i][j] = 1
			}
		}
	}
	ev := jacobiEigen(R)
	mu := 0.0
	for _, v := range ev {
		mu += v
	}
	mu /= float64(k)
	vv := 0.0
	for _, v := range ev {
		vv += (v - mu) * (v - mu)
	}
	vv /= float64(k - 1)
	mEff := 1 + float64(k-1)*(1-vv/float64(k))
	thr := math.Max(3.0, normQuantile(0.05/mEff))
	fmt.Printf("prep3: window %s..%s, %d date-level z60 predictor series\n", c.fo.dates[c.lo], c.fo.dates[c.hi], k)
	for i := range R {
		row := make([]string, k)
		for j := range R[i] {
			row[j] = fmt.Sprintf("%+.2f", R[i][j])
		}
		fmt.Printf("  %-22s %s\n", names[i], joinStr(row))
	}
	fmt.Printf("prep3: M %d, Nyholt M_eff %.3f, threshold t >= %.3f\n", k, mEff, thr)
}

func joinStr(v []string) string {
	s := ""
	for i, x := range v {
		if i > 0 {
			s += " "
		}
		s += x
	}
	return s
}

func corr(a, b []float64) float64 {
	var x, y []float64
	for i := range a {
		if !isNaN(a[i]) && !isNaN(b[i]) {
			x, y = append(x, a[i]), append(y, b[i])
		}
	}
	if len(x) < 30 {
		return 0
	}
	mx, my := mean(x), mean(y)
	var sxy, sxx, syy float64
	for i := range x {
		sxy += (x[i] - mx) * (y[i] - my)
		sxx += (x[i] - mx) * (x[i] - mx)
		syy += (y[i] - my) * (y[i] - my)
	}
	return sxy / math.Sqrt(sxx*syy)
}

func (x *ctx3) scan(thr float64) {
	type out struct {
		sp rowSpec
		r  res
	}
	var book1 []out
	for _, sp := range append(specs3(), specs4()...) {
		if sp.kind == "deferred" {
			fmt.Printf("%s deferred (not scored)\n", sp.id)
			continue
		}
		r := x.run(sp)
		book1 = append(book1, out{sp, r})
		fmt.Println(r.String(sp))
	}
	idx := make([]int, len(book1))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool {
		pa, pb := book1[idx[a]].r.p, book1[idx[b]].r.p
		if math.IsNaN(pa) {
			return false
		}
		if math.IsNaN(pb) {
			return true
		}
		return pa < pb
	})
	m := len(book1)
	cut := -1
	for k, i := range idx {
		if !math.IsNaN(book1[i].r.p) && book1[i].r.p <= 0.05*float64(k+1)/float64(m) {
			cut = k
		}
	}
	fmt.Printf("\n== Book 1 (new predictor-outcome pairs): |t| >= %.3f with the registered sign, half coefficients with the registered and the pooled sign, BH q=0.05 over %d rows\n", thr, m)
	pass := 0
	for k, i := range idx {
		o := book1[i]
		tt := o.r.t * o.sp.sign
		same := o.r.hc[0]*o.r.hc[1] > 0 && o.r.hc[0]*o.r.coef > 0 && o.r.coef*o.sp.sign > 0
		ok := tt >= thr && same && k <= cut
		if ok {
			pass++
		}
		fmt.Printf("%s t %+.2f p %.5f n %d BH %v same-sign %v -> %s\n", o.sp.id, o.r.t, o.r.p, o.r.n, k <= cut, same, map[bool]string{true: "STAT-PASS", false: "fail"}[ok])
	}
	fmt.Printf("Book 1: %d of %d statistically pass\n", pass, m)
	var book2 []out
	for _, sp := range specs5() {
		r := x.runThin(sp)
		book2 = append(book2, out{sp, r})
		fmt.Println(r.String(sp))
	}
	idx = idx[:0]
	for i := range book2 {
		idx = append(idx, i)
	}
	sort.Slice(idx, func(a, b int) bool {
		pa, pb := book2[idx[a]].r.p, book2[idx[b]].r.p
		if math.IsNaN(pa) {
			return false
		}
		if math.IsNaN(pb) {
			return true
		}
		return pa < pb
	})
	m2 := len(book2)
	cut = -1
	for k, i := range idx {
		if !math.IsNaN(book2[i].r.p) && book2[i].r.p <= 0.10*float64(k+1)/float64(m2) {
			cut = k
		}
	}
	leads := 0
	fmt.Printf("\n== Book 2 (thin equities, spent outcomes, leads only): BH q=0.10 over %d rows, registered sign\n", m2)
	for k, i := range idx {
		if k <= cut && book2[i].r.t*book2[i].sp.sign > 0 {
			leads++
			fmt.Printf("LEAD %s t %+.2f p %.4f\n", book2[i].sp.id, book2[i].r.t, book2[i].r.p)
		}
	}
	fmt.Printf("Book 2 leads: %d of %d\n", leads, m2)
}
