package main

import (
	"math"
	"sort"
)

type featFn func(p *panel, d int, adm []int) []float64

type rowSpec struct {
	id      string
	tier    int
	sign    float64
	kind    okind
	fn      featFn
	special bool
}

func per(f func(p *panel, i, d int) float64) featFn {
	return func(p *panel, d int, adm []int) []float64 {
		out := make([]float64, len(adm))
		for k, i := range adm {
			out[k] = f(p, i, d)
		}
		return out
	}
}

func col(k int) featFn {
	return per(func(p *panel, i, d int) float64 { return p.v(i, d, k) })
}

func prevVals(p *panel, i, d, k, n int, cur bool) []float64 {
	lo, hi := d-n, d-1
	if cur {
		lo, hi = d-n+1, d
	}
	var out []float64
	for s := lo; s <= hi; s++ {
		if v := p.v(i, s, k); !math.IsNaN(v) {
			out = append(out, v)
		}
	}
	return out
}

func m5(k int) featFn {
	return per(func(p *panel, i, d int) float64 {
		v := prevVals(p, i, d, k, 5, true)
		if len(v) < 3 || math.IsNaN(p.v(i, d, k)) {
			return nan
		}
		return mean(v)
	})
}

func logRatioM(k, n, minValid int) featFn {
	return per(func(p *panel, i, d int) float64 {
		cur := p.v(i, d, k)
		v := prevVals(p, i, d, k, n, false)
		if len(v) < minValid || !(cur > 0) {
			return nan
		}
		m := mean(v)
		if !(m > 0) {
			return nan
		}
		return math.Log(cur / m)
	})
}

func logRatioMedian(k, n, minValid int) featFn {
	return per(func(p *panel, i, d int) float64 {
		cur := p.v(i, d, k)
		v := prevVals(p, i, d, k, n, false)
		if len(v) < minValid || !(cur > 0) {
			return nan
		}
		sort.Float64s(v)
		m := v[len(v)/2]
		if !(m > 0) {
			return nan
		}
		return math.Log(cur / m)
	})
}

func zPrev(k int, tf func(float64) float64) featFn {
	return per(func(p *panel, i, d int) float64 {
		cur := p.v(i, d, k)
		if math.IsNaN(cur) {
			return nan
		}
		raw := prevVals(p, i, d, k, 20, false)
		var v []float64
		for _, x := range raw {
			if t := tf(x); !math.IsNaN(t) && !math.IsInf(t, 0) {
				v = append(v, t)
			}
		}
		c := tf(cur)
		if len(v) < 10 || math.IsNaN(c) || math.IsInf(c, 0) {
			return nan
		}
		sd := stdOf(v)
		if !(sd > 0) {
			return nan
		}
		return (c - mean(v)) / sd
	})
}

func ident(x float64) float64 { return x }
func lg(x float64) float64 {
	if x > 0 {
		return math.Log(x)
	}
	return nan
}

func minus(a, b int) featFn {
	return per(func(p *panel, i, d int) float64 { return p.v(i, d, a) - p.v(i, d, b) })
}

func minusM(k, n, minValid int) featFn {
	return per(func(p *panel, i, d int) float64 {
		cur := p.v(i, d, k)
		v := prevVals(p, i, d, k, n, false)
		if len(v) < minValid || math.IsNaN(cur) {
			return nan
		}
		return cur - mean(v)
	})
}

func logRatioCols(a, b int) featFn {
	return per(func(p *panel, i, d int) float64 {
		x, y := p.v(i, d, a), p.v(i, d, b)
		if !(x > 0) || !(y > 0) {
			return nan
		}
		return math.Log(x / y)
	})
}

func rankOf(v []float64) []float64 {
	var idx []int
	var vals []float64
	for k, x := range v {
		if !math.IsNaN(x) && !math.IsInf(x, 0) {
			idx = append(idx, k)
			vals = append(vals, x)
		}
	}
	out := make([]float64, len(v))
	for k := range out {
		out[k] = nan
	}
	if len(vals) < minPerDay {
		return out
	}
	rk := rankScale(vals)
	for n, k := range idx {
		out[k] = rk[n]
	}
	return out
}

func inter(fa, fb featFn) featFn {
	return func(p *panel, d int, adm []int) []float64 {
		ra, rb := rankOf(fa(p, d, adm)), rankOf(fb(p, d, adm))
		out := make([]float64, len(adm))
		for k := range out {
			out[k] = ra[k] * (rb[k] + 0.5)
		}
		return out
	}
}

func hiCol(k int) func(p *panel, i, d int) float64 {
	return func(p *panel, i, d int) float64 {
		if d < 0 || d >= len(p.dates) || p.hi[i][d] == nil {
			return nan
		}
		return float64(p.hi[i][d][k])
	}
}

func hiDiff(a, b int) featFn {
	fa, fb := hiCol(a), hiCol(b)
	return per(func(p *panel, i, d int) float64 { return fa(p, i, d) - fb(p, i, d) })
}

func hiZ(k int) featFn {
	f := hiCol(k)
	return per(func(p *panel, i, d int) float64 {
		cur := f(p, i, d)
		var v []float64
		for s := d - 20; s < d; s++ {
			if x := f(p, i, s); !math.IsNaN(x) {
				v = append(v, x)
			}
		}
		if len(v) < 10 || math.IsNaN(cur) {
			return nan
		}
		sd := stdOf(v)
		if !(sd > 0) {
			return nan
		}
		return (cur - mean(v)) / sd
	})
}

func hiRatioM(k int) featFn {
	f := hiCol(k)
	return per(func(p *panel, i, d int) float64 {
		cur := f(p, i, d)
		var v []float64
		for s := d - 20; s < d; s++ {
			if x := f(p, i, s); !math.IsNaN(x) {
				v = append(v, x)
			}
		}
		if len(v) < 10 || !(cur > 0) || !(mean(v) > 0) {
			return nan
		}
		return math.Log(cur / mean(v))
	})
}

func (p *panel) meanA2At(d int) float64 {
	if p.meanA2 == nil {
		p.meanA2 = make([]float64, len(p.dates))
		for k := range p.meanA2 {
			p.meanA2[k] = math.Inf(1)
		}
	}
	if !math.IsInf(p.meanA2[d], 1) {
		return p.meanA2[d]
	}
	p.meanA2[d] = nan
	if d < 61 {
		return nan
	}
	s, n := 0.0, 0
	for _, i := range p.admittedAt(d) {
		if v := p.v(i, d, bA2); !math.IsNaN(v) {
			s += v
			n++
		}
	}
	if n >= minPerDay {
		p.meanA2[d] = s / float64(n)
	}
	return p.meanA2[d]
}

func (p *panel) beta(i, d int) float64 {
	var xs, ys []float64
	for k := d - 60; k < d; k++ {
		if k < 1 || k >= len(p.dates) {
			continue
		}
		r, u := p.logRet(i, k), p.uniRet[k]
		if !math.IsNaN(r) && !math.IsNaN(u) {
			xs = append(xs, u)
			ys = append(ys, r)
		}
	}
	if len(xs) < 40 {
		return nan
	}
	mx, my := mean(xs), mean(ys)
	cov, vr := 0.0, 0.0
	for k := range xs {
		cov += (xs[k] - mx) * (ys[k] - my)
		vr += (xs[k] - mx) * (xs[k] - mx)
	}
	if vr <= 0 {
		return nan
	}
	return cov / vr
}

func marketTilt(top3 bool) featFn {
	return func(p *panel, d int, adm []int) []float64 {
		out := make([]float64, len(adm))
		sig := p.meanA2At(d)
		if top3 {
			s, n := 0.0, 0
			for k := 0; k < 3 && k < len(adm); k++ {
				if v := p.v(adm[k], d, bA2); !math.IsNaN(v) {
					s += v
					n++
				}
			}
			sig = nan
			if n >= 2 {
				sig = s / float64(n)
			}
		}
		for k, i := range adm {
			b := p.beta(i, d)
			out[k] = (b - 1) * sig
		}
		return out
	}
}

func neighborA2(p *panel, d int, adm []int) []float64 {
	n := len(adm)
	out := make([]float64, n)
	rets := make([][]float64, n)
	for k, i := range adm {
		r := make([]float64, 60)
		for s := 0; s < 60; s++ {
			r[s] = p.logRet(i, d-60+s)
		}
		rets[k] = r
	}
	for k := range out {
		out[k] = nan
	}
	for a := 0; a < n; a++ {
		own := p.v(adm[a], d, bA2)
		if math.IsNaN(own) {
			continue
		}
		type nb struct {
			c float64
			j int
		}
		var cs []nb
		for b := 0; b < n; b++ {
			if a == b || math.IsNaN(p.v(adm[b], d, bA2)) {
				continue
			}
			if c := corrPair(rets[a], rets[b]); !math.IsNaN(c) {
				cs = append(cs, nb{c, b})
			}
		}
		if len(cs) < 5 {
			continue
		}
		sort.Slice(cs, func(x, y int) bool { return cs[x].c > cs[y].c })
		s := 0.0
		for q := 0; q < 5; q++ {
			s += p.v(adm[cs[q].j], d, bA2)
		}
		out[a] = s/5 - own
	}
	return out
}

func corrPair(a, b []float64) float64 {
	var xs, ys []float64
	for k := range a {
		if !math.IsNaN(a[k]) && !math.IsNaN(b[k]) {
			xs = append(xs, a[k])
			ys = append(ys, b[k])
		}
	}
	if len(xs) < 40 {
		return nan
	}
	mx, my := mean(xs), mean(ys)
	sxy, sxx, syy := 0.0, 0.0, 0.0
	for k := range xs {
		sxy += (xs[k] - mx) * (ys[k] - my)
		sxx += (xs[k] - mx) * (xs[k] - mx)
		syy += (ys[k] - my) * (ys[k] - my)
	}
	if sxx <= 0 || syy <= 0 {
		return nan
	}
	return sxy / math.Sqrt(sxx*syy)
}

func residualA2(p *panel, d int, adm []int) []float64 {
	out := make([]float64, len(adm))
	md := p.meanA2At(d)
	for k, i := range adm {
		out[k] = nan
		own := p.v(i, d, bA2)
		if math.IsNaN(own) || math.IsNaN(md) {
			continue
		}
		var xs, ys []float64
		for s := d - 60; s < d; s++ {
			if s < 0 {
				continue
			}
			a, m := p.v(i, s, bA2), p.meanA2At(s)
			if !math.IsNaN(a) && !math.IsNaN(m) {
				xs = append(xs, m)
				ys = append(ys, a)
			}
		}
		if len(xs) < 40 {
			continue
		}
		mx, my := mean(xs), mean(ys)
		cov, vr := 0.0, 0.0
		for q := range xs {
			cov += (xs[q] - mx) * (ys[q] - my)
			vr += (xs[q] - mx) * (xs[q] - mx)
		}
		if vr <= 0 {
			continue
		}
		b := cov / vr
		out[k] = own - (my + b*(md-mx))
	}
	return out
}

func condTercile(cond func(p *panel, i, d int) float64, lowest bool) featFn {
	return func(p *panel, d int, adm []int) []float64 {
		out := make([]float64, len(adm))
		var cs []float64
		for _, i := range adm {
			c := cond(p, i, d)
			if !math.IsNaN(c) && !math.IsNaN(p.v(i, d, bA2)) {
				cs = append(cs, c)
			}
		}
		for k := range out {
			out[k] = nan
		}
		if len(cs) < minPerDay {
			return out
		}
		sort.Float64s(cs)
		cut := cs[len(cs)/3]
		for k, i := range adm {
			c := cond(p, i, d)
			if !math.IsNaN(c) && c <= cut {
				out[k] = p.v(i, d, bA2)
			}
		}
		return out
	}
}

func lowRangeA2(p *panel, d int, adm []int) []float64 {
	out := make([]float64, len(adm))
	rangeOf := func(i, s int) float64 {
		h, l, c := p.v(i, s, bHigh), p.v(i, s, bLow), p.v(i, s, bClose1830)
		if !(c > 0) || math.IsNaN(h) || math.IsNaN(l) {
			return nan
		}
		return (h - l) / c
	}
	for k, i := range adm {
		out[k] = nan
		cur := rangeOf(i, d)
		var v []float64
		for s := d - 20; s < d; s++ {
			if r := rangeOf(i, s); !math.IsNaN(r) {
				v = append(v, r)
			}
		}
		if len(v) < 10 || math.IsNaN(cur) {
			continue
		}
		sort.Float64s(v)
		if cur < v[len(v)/2] {
			out[k] = p.v(i, d, bA2)
		}
	}
	return out
}

func agreement(p *panel, d int, adm []int) []float64 {
	ra, rb := rankOf(col(bA1)(p, d, adm)), rankOf(col(bA2)(p, d, adm))
	out := make([]float64, len(adm))
	for k := range out {
		out[k] = nan
		if math.IsNaN(ra[k]) || math.IsNaN(rb[k]) {
			continue
		}
		if ra[k]*rb[k] > 0 {
			out[k] = (ra[k] + rb[k]) / 2
		} else {
			out[k] = 0
		}
	}
	return out
}

func rankDiffValSpread(p *panel, d int, adm []int) []float64 {
	rv, rs := rankOf(col(bValDay)(p, d, adm)), rankOf(col(bSpLv)(p, d, adm))
	out := make([]float64, len(adm))
	for k := range out {
		out[k] = rv[k] - rs[k]
	}
	return out
}

func eveningRank(p *panel, d int, adm []int) []float64 {
	ra, rb := rankOf(col(bEvA1)(p, d, adm)), rankOf(col(bEvA2)(p, d, adm))
	out := make([]float64, len(adm))
	for k := range out {
		out[k] = (ra[k] + rb[k]) / 2
	}
	return out
}

func gapOpen(p *panel, i, d int) float64 {
	o, c := p.v(i, d, bOpen10), p.v(i, d-1, bCloseDay)
	if !(o > 0) || !(c > 0) {
		return nan
	}
	return o/c - 1
}

func semiDiff(p *panel, i, d int) float64 {
	dn, up := p.v(i, d, bRSdn), p.v(i, d, bRSup)
	if math.IsNaN(dn) || math.IsNaN(up) || dn+up <= 0 {
		return nan
	}
	return (dn - up) / (dn + up)
}

func sdRV(p *panel, i, d int) float64 {
	v := prevVals(p, i, d, bRV, 20, true)
	if len(v) < 12 {
		return nan
	}
	return stdOf(v)
}

func rangePos(p *panel, i, d int) float64 {
	hi, lo := math.Inf(-1), math.Inf(1)
	n := 0
	for s := d - 19; s <= d; s++ {
		h, l := p.v(i, s, bHigh), p.v(i, s, bLow)
		if !math.IsNaN(h) && !math.IsNaN(l) {
			hi, lo = math.Max(hi, h), math.Min(lo, l)
			n++
		}
	}
	c := p.v(i, d, bClose1830)
	if n < 12 || !(hi > lo) || !(c > 0) {
		return nan
	}
	return (c - lo) / (hi - lo)
}

func logRV20(p *panel, i, d int) float64 {
	cur := p.v(i, d, bRV)
	v := prevVals(p, i, d, bRV, 20, false)
	if len(v) < 12 || !(cur > 0) || !(mean(v) > 0) {
		return nan
	}
	return math.Log(cur / mean(v))
}

func sumDiffLog(a, b int) func(p *panel, i, d int) float64 {
	return func(p *panel, i, d int) float64 {
		x, y := p.v(i, d, a), p.v(i, d, b)
		if !(x > 0) || !(y > 0) {
			return nan
		}
		return math.Log(x / y)
	}
}

func specs() []rowSpec {
	var r []rowSpec
	add := func(id string, tier int, sign float64, kind okind, fn featFn) {
		r = append(r, rowSpec{id: id, tier: tier, sign: sign, kind: kind, fn: fn})
	}
	sp := func(id string, tier int) {
		r = append(r, rowSpec{id: id, tier: tier, special: true})
	}
	add("H001", 1, 1, kH5, m5(bA2))
	add("H002", 1, 1, kH1, logRatioM(bDepth, 20, 10))
	add("H003", 1, 1, kH1, col(bLevAsym))
	add("H004", 1, -1, kH1, col(bThinBid))
	add("H005", 2, 1, kH1, col(bImbVol))
	add("H006", 2, 1, kH1, minus(bImbBBOVol, bImbBBOVal))
	add("H007", 2, 1, kH1, col(bA2Late))
	add("H008", 2, 1, kH1, col(bA2Early))
	add("H009", 2, 1, kH1, minusM(bA2, 60, 30))
	add("H010", 2, 1, kH1, condTercile(func(p *panel, i, d int) float64 { return p.v(i, d, bSpLv) }, true))
	add("H011", 2, 1, kH1, lowRangeA2)
	add("H012", 2, 1, kH1, inter(col(bA2), logRatioM(bValDay, 20, 10)))
	add("H013", 2, -1, kH1, per(func(p *panel, i, d int) float64 { return p.v(i, d, bA2) - p.v(i, d-1, bA2) }))
	add("H014", 2, -1, kH1, col(bImbStd))
	add("H015", 1, 1, kH5, logRatioM(bSpLv, 20, 10))
	add("H016", 1, -1, kH1, col(bSpRatio))
	add("H017", 1, 1, kH5, logRatioMedian(bSp1m, 60, 30))
	add("H018", 2, 1, kH1, col(bSpStd))
	add("H019", 2, -1, kH1, col(bSpLast))
	add("H020", 2, 1, kH1, inter(col(bA2), col(bSpLv)))
	add("H021", 2, 1, kH5, minus(bSp1m, bSpLv))
	add("H022", 2, 1, kH1, rankDiffValSpread)
	add("H023", 2, -1, kH1, col(bSpEarlyOverMid))
	add("H024", 2, -1, kH1, col(bBBOoverStd))
	add("H025", 1, 1, kH1, zPrev(bTicket, lg))
	add("H026", 1, 1, kH1, logRatioCols(bTicketB, bTicketS))
	add("H027", 1, -1, kH1, col(bTop3))
	add("H028", 1, -1, kH1, logRatioM(bTrades, 20, 10))
	add("H029", 2, 1, kH1, col(bTrCntImb))
	add("H030", 2, 1, kH1, col(bTicketGapLate))
	add("H031", 2, 1, kH5, col(bAmihud))
	add("H032", 2, 1, kH1, col(bA1Trades))
	add("H033", 2, 1, kH5, col(bIdle))
	add("H034", 2, 1, kH1, zPrev(bTicketMed, ident))
	add("H035", 2, 1, kH1, col(bTicketStd))
	add("H036", 2, 1, kH1, col(bLogVolRatio))
	add("H037", 1, 1, kH1, col(bAggGap))
	add("H038", 1, -1, kH1, col(bCLV))
	add("H039", 2, -1, kH1, col(bVwapDev))
	add("H040", 2, -1, kH1, col(bRetLast))
	add("H041", 2, 1, kH1, col(bRetEarly))
	add("H042", 2, 1, kH1, col(bAggGapEarly))
	add("H043", 2, 1, kH5, m5(bA1))
	add("H044", 2, 1, kH1, agreement)
	add("H045", 2, -1, kH1, col(bShareNeg))
	add("H046", 2, 1, kH1, col(bA1Tail))
	add("H047", 1, -1, kH1, zPrev(bPutOverVal, ident))
	add("H048", 1, -1, kH5, col(bCanDiff))
	add("H049", 1, 1, kH1, col(bPlaceAgg))
	add("H050", 1, 1, kH1, col(bPutSize))
	add("H051", 1, 1, kH5, m5(bA5))
	add("H052", 2, 1, kH5, m5(bA4))
	add("H053", 2, 1, kH1, col(bCanVwGap))
	add("H054", 2, -1, kH1, col(bCanOrdRatio))
	add("H055", 2, -1, kH1, logRatioM(bPutOrders, 20, 10))
	add("H056", 2, -1, kH1, col(bPutOverBook))
	add("H057", 2, 1, kH1, col(bA4Late))
	add("H058", 2, 1, kH1, col(bA4Early))
	add("H059", 1, 1, kH1, minusM(bShareLast, 20, 10))
	add("H060", 1, -1, kIntraSame, per(gapOpen))
	add("H061", 2, 1, kH1, minus(bA2H1, bA2H2))
	add("H062", 2, 1, kH1, col(bDepthOpenClose))
	add("H063", 2, -1, kH1, col(bPeakMin))
	add("H064", 2, -1, kH1, col(bShareMid))
	add("H065", 1, 1, kIntraNext, eveningRank)
	add("H066", 2, 1, kIntraSame, col(bMornA1))
	add("H067", 2, 1, kH1, col(bMornShare))
	add("H068", 2, 1, kIntraNext, col(bEvSpRatio))
	sp("H069", 1)
	add("H070", 2, 1, kH5, per(logRV20))
	add("H071", 2, 1, kH1, col(bPrStd))
	add("H072", 2, -1, kH1, col(bMaxJump))
	add("H073", 2, 1, kH1, per(semiDiff))
	add("H074", 2, 1, kH5, col(bPark))
	add("H075", 2, 1, kH5, per(sdRV))
	add("H076", 2, -1, kH5, per(rangePos))
	add("H077", 1, 1, kH1, neighborA2)
	add("H078", 1, 1, kH1, residualA2)
	add("H079", 2, 1, kH1, marketTilt(false))
	sp("H080", 2)
	add("H081", 2, 1, kH1, marketTilt(true))
	sp("H082", 2)
	add("H083", 2, 1, kIntraNext, col(bEvRet))
	sp("H084", 2)
	sp("H085", 1)
	sp("H086", 1)
	sp("H087", 1)
	sp("H088", 1)
	sp("H089", 2)
	sp("H090", 2)
	sp("H091", 2)
	sp("H092", 2)
	sp("H093", 2)
	sp("H094", 2)
	add("H095", 2, 1, kH1, hiDiff(1, 2))
	add("H096", 2, 1, kH1, hiDiff(5, 6))
	add("H097", 2, -1, kH1, hiZ(7))
	add("H098", 2, -1, kH1, hiRatioM(10))
	add("H099", 2, 1, kH1, inter(col(bA2), per(func(p *panel, i, d int) float64 { return hiCol(1)(p, i, d) })))
	add("H100", 2, 1, kH1, hiDiff(3, 4))
	return r
}
