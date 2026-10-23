package main

import (
	"fmt"
	"io"
	"math"
	"sort"
)

const (
	nCtrl     = 8
	tThresh   = 3.0
	costLive  = 0.0015
	costLow   = 0.0005
	nwLags    = 5
	minPerDay = 40
)

type fmRow struct {
	ti    int
	y     float64
	yAlt  float64
	raw   float64
	ctrl  [nCtrl]float64
	extra []float64
	feats []float64
}

type fmResult struct {
	days  []int
	betas []float64
}

func validCtrl(c [nCtrl]float64) bool {
	for _, v := range c {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

func runFM(rows [][]fmRow, feat int, useAlt bool) fmResult {
	var res fmResult
	for d, rs := range rows {
		var pick []fmRow
		for _, r := range rs {
			if !math.IsNaN(r.feats[feat]) && !math.IsInf(r.feats[feat], 0) {
				pick = append(pick, r)
			}
		}
		if len(pick) < minPerDay {
			continue
		}
		raw := make([]float64, len(pick))
		for i, r := range pick {
			raw[i] = r.feats[feat]
		}
		rk := rankScale(raw)
		x := make([][]float64, len(pick))
		y := make([]float64, len(pick))
		for i, r := range pick {
			row := make([]float64, 0, 2+nCtrl)
			row = append(row, 1, rk[i])
			row = append(row, r.ctrl[:]...)
			row = append(row, r.extra...)
			x[i] = row
			if useAlt {
				y[i] = r.yAlt
			} else {
				y[i] = r.y
			}
		}
		beta, ok := ols(x, y)
		if !ok {
			continue
		}
		res.days = append(res.days, d)
		res.betas = append(res.betas, beta[1])
	}
	return res
}

type fmSummary struct {
	n        int
	mean, t  float64
	m0, t0   float64
	m1, t1   float64
	n0, n1   int
	sameSign bool
}

func (p *panel) summarize(r fmResult) fmSummary {
	var s fmSummary
	var h0, h1 []float64
	for i, d := range r.days {
		if p.half(d) == 0 {
			h0 = append(h0, r.betas[i])
		} else {
			h1 = append(h1, r.betas[i])
		}
	}
	s.n = len(r.betas)
	s.mean, s.t = neweyWest(r.betas, nwLags)
	s.m0, s.t0 = neweyWest(h0, nwLags)
	s.m1, s.t1 = neweyWest(h1, nwLags)
	s.n0, s.n1 = len(h0), len(h1)
	s.sameSign = s.m0*s.m1 > 0 && s.m0*s.mean > 0
	return s
}

type dailyPoint struct {
	day              int
	gross, net       float64
	rawTop, turnover float64
}

type tradeStats struct {
	daily             []dailyPoint
	gross, net, net05 [2]float64
	days              [2]int
	grossAll, netAll  float64
	spreadGross       [2]float64
	spreadDays        [2]int
	spreadAll         float64
}

func (p *panel) tradeForm(rows [][]fmRow, feat int, sign float64, frac float64) tradeStats {
	var ts tradeStats
	prev := map[int]float64{}
	var nets []float64
	var grossAll float64
	var nAll int
	for d, rs := range rows {
		var pick []fmRow
		for _, r := range rs {
			if !math.IsNaN(r.feats[feat]) && !math.IsInf(r.feats[feat], 0) {
				pick = append(pick, r)
			}
		}
		if len(pick) < minPerDay {
			continue
		}
		sort.Slice(pick, func(a, b int) bool {
			fa, fb := sign*pick[a].feats[feat], sign*pick[b].feats[feat]
			if fa != fb {
				return fa > fb
			}
			return pick[a].ti < pick[b].ti
		})
		m := int(float64(len(pick)) * frac)
		if m < 3 {
			continue
		}
		cur := map[int]float64{}
		g, rawTop := 0.0, 0.0
		for _, r := range pick[:m] {
			cur[r.ti] = 1 / float64(m)
			g += r.y
			rawTop += math.Expm1(r.raw)
		}
		g /= float64(m)
		rawTop /= float64(m)
		low := 0.0
		for _, r := range pick[len(pick)-m:] {
			low += r.y
		}
		low /= float64(m)
		turn := 0.0
		for k, w := range cur {
			turn += math.Abs(w - prev[k])
		}
		for k, w := range prev {
			if _, ok := cur[k]; !ok {
				turn += w
			}
		}
		prev = cur
		h := p.half(d)
		net := g - costLive*turn
		ts.daily = append(ts.daily, dailyPoint{day: d, gross: g, net: net, rawTop: rawTop, turnover: turn})
		net05 := g - costLow*turn
		ts.gross[h] += g
		ts.net[h] += net
		ts.net05[h] += net05
		ts.days[h]++
		ts.spreadGross[h] += g - low
		ts.spreadDays[h]++
		grossAll += g
		ts.spreadAll += g - low
		nets = append(nets, net)
		nAll++
	}
	for h := 0; h < 2; h++ {
		if ts.days[h] > 0 {
			ts.gross[h] /= float64(ts.days[h])
			ts.net[h] /= float64(ts.days[h])
			ts.net05[h] /= float64(ts.days[h])
			ts.spreadGross[h] /= float64(ts.spreadDays[h])
		}
	}
	if nAll > 0 {
		ts.grossAll = grossAll / float64(nAll)
		ts.netAll = mean(nets)
		ts.spreadAll /= float64(nAll)
	}
	return ts
}

func pct(v float64) string {
	if math.IsNaN(v) {
		return "n/a"
	}
	return fmt.Sprintf("%+.4f%%", v*100)
}

func (p *panel) baseRowsA(topN int) [][]fmRow {
	return p.baseRowsAh(topN, 1)
}

func (p *panel) baseRowsAh(topN, h int) [][]fmRow {
	out := make([][]fmRow, len(p.dates))
	for d := 7; d+h < len(p.dates); d++ {
		if p.excluded[d] || p.excluded[d+h] {
			continue
		}
		adm := p.admittedAt(d, topN)
		if len(adm) == 0 {
			continue
		}
		next := make([]float64, len(adm))
		sum, cnt := 0.0, 0
		for k, i := range adm {
			next[k] = p.logRet(i, d+h)
			if !math.IsNaN(next[k]) {
				sum += next[k]
				cnt++
			}
		}
		if cnt < minPerDay {
			continue
		}
		uni := sum / float64(cnt)
		nextAlt := make([]float64, len(adm))
		sumAlt, cntAlt := 0.0, 0
		for k, i := range adm {
			nextAlt[k] = nan
			ro := p.recs[i][d+h]
			if h == 1 && ro != nil && ro.openDay > 0 && ro.closeDay > 0 && p.simpleOK(ro.closeDay/ro.openDay-1) {
				nextAlt[k] = math.Log(ro.closeDay / ro.openDay)
				sumAlt += nextAlt[k]
				cntAlt++
			}
		}
		uniAlt := nan
		if cntAlt > 0 {
			uniAlt = sumAlt / float64(cntAlt)
		}
		for k, i := range adm {
			rec := p.recs[i][d]
			if rec == nil || math.IsNaN(next[k]) {
				continue
			}
			var c [nCtrl]float64
			for l := 0; l <= 5; l++ {
				c[l] = p.logRet(i, d-l)
			}
			c[6] = p.turnZ(i, d)
			c[7] = rec.spread
			if !validCtrl(c) {
				continue
			}
			f := make([]float64, 5)
			copy(f, rec.feat[:])
			out[d] = append(out[d], fmRow{ti: i, y: next[k] - uni, yAlt: nextAlt[k] - uniAlt, raw: next[k], ctrl: c, feats: f})
		}
	}
	return out
}

func (p *panel) baseRowsB(topN int, late bool) [][]fmRow {
	out := make([][]fmRow, len(p.dates))
	cpSlot := slotMid
	if late {
		cpSlot = slotLate
	}
	for d := 7; d < len(p.dates); d++ {
		if p.excluded[d] || p.excluded[d-1] {
			continue
		}
		adm := p.admittedAt(d, topN)
		if len(adm) == 0 {
			continue
		}
		type cand struct {
			i   int
			ret float64
			cut cut
			rec *dayRec
		}
		var cs []cand
		sum := 0.0
		for _, i := range adm {
			rec := p.recs[i][d]
			if rec == nil {
				continue
			}
			c := rec.mid
			if late {
				c = rec.late
			}
			if c.price <= 0 || c.priceSlot < cpSlot-6 || rec.end <= 0 || rec.endSlot < 95 || rec.endSlot <= c.priceSlot {
				continue
			}
			if !p.simpleOK(rec.end/c.price - 1) {
				continue
			}
			r := math.Log(rec.end / c.price)
			cs = append(cs, cand{i, r, c, rec})
			sum += r
		}
		if len(cs) < minPerDay {
			continue
		}
		uni := sum / float64(len(cs))
		for _, e := range cs {
			prevClose := float64(p.close[e.i][d-1])
			if prevClose <= 0 || !p.simpleOK(e.cut.price/prevClose-1) {
				continue
			}
			var c [nCtrl]float64
			c[0] = math.Log(e.cut.price / prevClose)
			for l := 1; l <= 5; l++ {
				c[l] = p.logRet(e.i, d-l)
			}
			c[6] = p.turnZ(e.i, d-1)
			c[7] = e.cut.spread
			if !validCtrl(c) {
				continue
			}
			out[d] = append(out[d], fmRow{ti: e.i, y: e.ret - uni, yAlt: nan, ctrl: c, feats: []float64{e.cut.tradeImb, e.cut.bookImb}})
		}
	}
	return out
}

type verdict struct {
	name   string
	pass   bool
	detail string
}

func (p *panel) reportFM(w io.Writer, name string, rows [][]fmRow, feat int, decileHurdle float64) verdict {
	res := runFM(rows, feat, false)
	s := p.summarize(res)
	fmt.Fprintf(w, "%s: dates %d, coef %+.5f, NW t %+.2f (halves %+.2f / %+.2f, mean %+.5f / %+.5f), same sign %v\n", name, s.n, s.mean, s.t, s.t0, s.t1, s.m0, s.m1, s.sameSign)
	v := verdict{name: name}
	if math.IsNaN(s.t) || math.Abs(s.t) < tThresh || !s.sameSign {
		fmt.Fprintf(w, "  -> FAIL (statistic)\n")
		v.detail = "statistic"
		return v
	}
	sign := 1.0
	if s.mean < 0 {
		sign = -1
	}
	if decileHurdle > 0 {
		ts := p.tradeForm(rows, feat, sign, 0.1)
		ok := ts.spreadAll > decileHurdle && ts.spreadGross[0] > 0 && ts.spreadGross[1] > 0
		fmt.Fprintf(w, "  top-minus-bottom decile gross %s (halves %s / %s), hurdle %s -> %s\n", pct(ts.spreadAll), pct(ts.spreadGross[0]), pct(ts.spreadGross[1]), pct(decileHurdle), passWord(ok))
		v.pass = ok
		return v
	}
	ts := p.tradeForm(rows, feat, sign, 0.2)
	ok := ts.net[0] > 0 && ts.net[1] > 0
	net05 := (ts.net05[0]*float64(ts.days[0]) + ts.net05[1]*float64(ts.days[1])) / float64(ts.days[0]+ts.days[1])
	fmt.Fprintf(w, "  top quintile minus universe per day: gross %s, net 0.15%% %s (halves %s / %s), net 0.05%% %s -> %s\n", pct(ts.grossAll), pct(ts.netAll), pct(ts.net[0]), pct(ts.net[1]), pct(net05), passWord(ok))
	v.pass = ok
	return v
}

func passWord(ok bool) string {
	if ok {
		return "PASS"
	}
	return "fail (trading form)"
}
