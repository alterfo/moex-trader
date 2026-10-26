package main

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
)

type tfmSeries struct {
	di   []int
	ab   []float64
	flow [3][]float64
}

type tfmCtx struct {
	ds    *dataset
	ser   map[string]*tfmSeries
	pos   map[string]map[int]int
	names []string
	cv    map[string][]float64
	col   map[string]int
}

func buildTfmCtx(pp *pipe, path string) *tfmCtx {
	ds := pp.ds
	var names []string
	cv := map[string][]float64{}
	if path != "" {
		names, cv = readCandidates(path)
	}
	t := &tfmCtx{ds: ds, ser: map[string]*tfmSeries{}, pos: map[string]map[int]int{}, names: names, cv: cv, col: map[string]int{}}
	for i, n := range names {
		t.col[n] = i
	}
	fidx := map[string]int{}
	for i, n := range ds.names {
		fidx[n] = i
	}
	flowNames := []string{"A2", "LevAsym", "SpLv"}
	for d := range ds.dates {
		var vals []float64
		for _, x := range ds.byDate[d] {
			if !math.IsNaN(x.rb1) {
				vals = append(vals, x.rb1)
			}
		}
		mk := nan
		if len(vals) >= 30 {
			mk = mean(vals)
		}
		for _, x := range ds.byDate[d] {
			s := t.ser[x.secid]
			if s == nil {
				s = &tfmSeries{}
				t.ser[x.secid] = s
				t.pos[x.secid] = map[int]int{}
			}
			ab := 0.0
			if !math.IsNaN(x.rb1) && !math.IsNaN(mk) {
				ab = x.rb1 - mk
			}
			t.pos[x.secid][d] = len(s.di)
			s.di = append(s.di, d)
			s.ab = append(s.ab, ab)
			for j, fn := range flowNames {
				s.flow[j] = append(s.flow[j], x.raw[fidx[fn]])
			}
		}
	}
	return t
}

func sumRange(v []float64, lo, hi int) float64 {
	s := 0.0
	for i := lo; i <= hi; i++ {
		s += v[i]
	}
	return s
}

func (t *tfmCtx) file(d int, secid string, name string) float64 {
	v, ok := t.cv[t.ds.dates[d]+"|"+secid]
	if !ok {
		return nan
	}
	return v[t.col[name]]
}

var tfmCandNames = []string{"u256", "surprise_lag", "d512-64", "d512-256", "d256-64", "flA2", "flLev", "flSp", "shapeA", "shapeB", "drop"}

type tfmDay struct {
	rows  []*row
	cand  [][]float64
	twin  [][]float64
	mom   [4][]float64
	vol20 []float64
	sp    []float64
}

func (t *tfmCtx) day(d int) *tfmDay {
	rows := t.ds.byDate[d]
	n := len(rows)
	nc := len(tfmCandNames)
	out := &tfmDay{rows: rows}
	out.cand = make([][]float64, nc)
	out.twin = make([][]float64, nc)
	for c := range out.cand {
		out.cand[c] = filled(n)
		out.twin[c] = filled(n)
	}
	for k := range out.mom {
		out.mom[k] = filled(n)
	}
	out.vol20 = filled(n)
	out.sp = filled(n)
	get := func(name string) []float64 {
		v := filled(n)
		for i, x := range rows {
			v[i] = t.file(d, x.secid, name)
		}
		return v
	}
	u64, u256, u512 := get("u64"), get("u256"), get("u512")
	sh := get("sh256")
	sa, sb := get("shapeA"), get("shapeB")
	fl := [3][]float64{get("flA2"), get("flLev"), get("flSp")}
	out.sp = get("sp")
	m64, m120, m256 := filled(n), filled(n), filled(n)
	twA, twB, twD := filled(n), filled(n), filled(n)
	sur, ar1 := filled(n), filled(n)
	var nv [3][]float64
	for j := range nv {
		nv[j] = filled(n)
	}
	for i, x := range rows {
		s := t.ser[x.secid]
		p, ok := t.pos[x.secid][d]
		if !ok || p < 255 {
			continue
		}
		out.mom[0][i] = sumRange(s.ab, p-19, p)
		m64[i] = sumRange(s.ab, p-63, p)
		m120[i] = sumRange(s.ab, p-119, p)
		m256[i] = sumRange(s.ab, p-255, p)
		out.mom[1][i], out.mom[2][i], out.mom[3][i] = m64[i], m120[i], m256[i]
		mu := sumRange(s.ab, p-19, p) / 20
		v := 0.0
		for q := p - 19; q <= p; q++ {
			v += (s.ab[q] - mu) * (s.ab[q] - mu)
		}
		out.vol20[i] = math.Sqrt(v / 19)
		twA[i] = sumRange(s.ab, p-2, p)/3 - sumRange(s.ab, p-9, p-3)/7
		if p >= 59 {
			twB[i] = sumRange(s.ab, p-19, p)/20 - sumRange(s.ab, p-59, p-20)/40
		}
		twD[i] = out.mom[0][i] - m256[i]/12.8
		if p >= 10+256 {
			prev := s.di[p-10]
			real := sumRange(s.ab, p-9, p)
			sur[i] = real - t.file(prev, x.secid, "u256")
			var sxy, sxx float64
			for q := p - 10 - 254; q <= p-10; q++ {
				sxy += s.ab[q] * s.ab[q-1]
				sxx += s.ab[q-1] * s.ab[q-1]
			}
			if sxx > 0 {
				phi := sxy / sxx
				acc, pw := 0.0, 1.0
				for k := 1; k <= 10; k++ {
					pw *= phi
					acc += pw
				}
				ar1[i] = real - s.ab[p-10]*acc
			}
		}
		for j := 0; j < 3; j++ {
			var vals []float64
			for q := p - 9; q <= p; q++ {
				if !math.IsNaN(s.flow[j][q]) {
					vals = append(vals, s.flow[j][q])
				}
			}
			if len(vals) >= 5 {
				nv[j][i] = mean(vals)
			}
		}
	}
	rsub := func(v []float64) []float64 {
		idx := []int{}
		for i := range v {
			if !math.IsNaN(v[i]) {
				idx = append(idx, i)
			}
		}
		sub := make([]float64, len(idx))
		for k, i := range idx {
			sub[k] = v[i]
		}
		r := rankScale(sub)
		o := filled(len(v))
		for k, i := range idx {
			o[i] = r[k]
		}
		return o
	}
	dif := func(a, b []float64) []float64 {
		ra, rb := rsub(a), rsub(b)
		o := filled(n)
		for i := range o {
			if !math.IsNaN(ra[i]) && !math.IsNaN(rb[i]) {
				o[i] = ra[i] - rb[i]
			}
		}
		return o
	}
	out.cand[0] = u256
	out.cand[1], out.twin[1] = sur, ar1
	out.cand[2], out.twin[2] = dif(u512, u64), dif(m256, m64)
	out.cand[3], out.twin[3] = dif(u512, u256), dif(m256, m120)
	out.cand[4], out.twin[4] = dif(u256, u64), dif(m256, m64)
	for j := 0; j < 3; j++ {
		out.cand[5+j], out.twin[5+j] = fl[j], nv[j]
	}
	out.cand[8], out.twin[8] = sa, twA
	out.cand[9], out.twin[9] = sb, twB
	d10 := filled(n)
	for i := range d10 {
		if !math.IsNaN(u256[i]) && !math.IsNaN(sh[i]) {
			d10[i] = u256[i] - sh[i]
		}
	}
	out.cand[10], out.twin[10] = d10, twD
	return out
}

func filled(n int) []float64 {
	o := make([]float64, n)
	for i := range o {
		o[i] = nan
	}
	return o
}

func olsCoef(cols [][]float64, y []float64) []float64 {
	k := len(cols)
	n := len(y)
	my := mean(y)
	mc := make([]float64, k)
	for j := range cols {
		mc[j] = mean(cols[j])
	}
	a := make([][]float64, k)
	b := make([]float64, k)
	for i := range a {
		a[i] = make([]float64, k+1)
		for j := 0; j < k; j++ {
			s := 0.0
			for r := 0; r < n; r++ {
				s += (cols[i][r] - mc[i]) * (cols[j][r] - mc[j])
			}
			a[i][j] = s
		}
		for r := 0; r < n; r++ {
			b[i] += (cols[i][r] - mc[i]) * (y[r] - my)
		}
		a[i][k] = b[i]
	}
	for i := 0; i < k; i++ {
		pv := i
		for r := i + 1; r < k; r++ {
			if math.Abs(a[r][i]) > math.Abs(a[pv][i]) {
				pv = r
			}
		}
		if math.Abs(a[pv][i]) < 1e-12 {
			return nil
		}
		a[i], a[pv] = a[pv], a[i]
		for r := 0; r < k; r++ {
			if r == i {
				continue
			}
			f := a[r][i] / a[i][i]
			for c := i; c <= k; c++ {
				a[r][c] -= f * a[i][c]
			}
		}
	}
	out := make([]float64, k)
	for i := range out {
		out[i] = a[i][k] / a[i][i]
	}
	return out
}

func slopeOf(y []float64, ctrl [][]float64, cand []float64) float64 {
	idx := []int{}
	for i := range y {
		if math.IsNaN(y[i]) || math.IsNaN(cand[i]) {
			continue
		}
		ok := true
		for _, c := range ctrl {
			if math.IsNaN(c[i]) {
				ok = false
				break
			}
		}
		if ok {
			idx = append(idx, i)
		}
	}
	if len(idx) < 30 {
		return nan
	}
	sel := func(v []float64) []float64 {
		s := make([]float64, len(idx))
		for k, i := range idx {
			s[k] = v[i]
		}
		return rankScale(s)
	}
	ys := make([]float64, len(idx))
	for k, i := range idx {
		ys[k] = y[i]
	}
	var cols [][]float64
	for _, c := range ctrl {
		cols = append(cols, sel(c))
	}
	cols = append(cols, sel(cand))
	co := olsCoef(cols, ys)
	if co == nil {
		return nan
	}
	return co[len(co)-1]
}

func tfmBFlags(mb, tb, mi, h1, h2 float64) bool {
	return math.Abs(tb) >= 3.0 && h1*mb > 0 && h2*mb > 0 && mi*mb > 0
}

func tfmB(pp *pipe, frzPath string) {
	ds := pp.ds
	t := buildTfmCtx(pp, candFile)
	raw, err := readFile(frzPath)
	if err != nil {
		fatal(err)
	}
	var fz frozen
	unmarshal(raw, &fz)
	t0 := sortSearch(ds.dates, "2020-04-01")
	r := newRunner(pp, fz, []string{"blend"})
	r.keep = map[int][]float64{}
	blocks := [][2]string{{"2025-01-03", "2025-03-31"}, {"2025-04-01", "2025-06-30"}, {"2025-07-01", "2025-09-30"}, {"2025-10-01", "2025-12-31"}, {"2026-01-01", "2026-03-31"}, {"2026-04-01", "2026-06-30"}, {"2026-07-01", "2026-10-16"}}
	for _, b := range blocks {
		lo, hi := pp.dateRange(b[0], b[1])
		r.block(lo, hi, t0, nil, false)
	}
	lo, hi := pp.dateRange("2025-01-03", "2026-10-16")
	split := sortSearch(ds.dates, "2026-01-05") - lo
	nc := len(tfmCandNames)
	total := nc + 5
	label := append([]string{}, tfmCandNames...)
	for k := 0; k < 5; k++ {
		label = append(label, fmt.Sprintf("placebo%d (u256 permuted)", k+1))
	}
	nd := hi - lo + 1
	mk := func() [][]float64 {
		o := make([][]float64, total)
		for c := range o {
			o[c] = filled(nd)
		}
		return o
	}
	ic, bs, bt, cr := mk(), mk(), mk(), mk()
	var e2 [4][]float64
	for k := range e2 {
		e2[k] = filled(nd)
	}
	var skill [3][2][]float64
	for j := range skill {
		for k := range skill[j] {
			skill[j][k] = filled(nd)
		}
	}
	for d := lo; d <= hi; d++ {
		pr := r.keep[d]
		if pr == nil {
			continue
		}
		dy := t.day(d)
		rows := dy.rows
		y := make([]float64, len(rows))
		for i, x := range rows {
			y[i] = x.y10
		}
		who := []int{}
		for i := range rows {
			if !math.IsNaN(dy.cand[0][i]) {
				who = append(who, i)
			}
		}
		cols := make([][]float64, total)
		for c := 0; c < nc; c++ {
			cols[c] = dy.cand[c]
		}
		for k := 0; k < 5; k++ {
			rng := rand.New(rand.NewSource(int64(20261031+k)*1000003 + int64(d)))
			perm := rng.Perm(len(who))
			cc := filled(len(rows))
			for j, i := range who {
				cc[i] = dy.cand[0][who[perm[j]]]
			}
			cols[nc+k] = cc
		}
		m120, m256 := dy.mom[2], dy.mom[3]
		for c := 0; c < total; c++ {
			var ys, ps, ss, ms []float64
			for i := range rows {
				if math.IsNaN(y[i]) || math.IsNaN(pr[i]) || math.IsNaN(cols[c][i]) || math.IsNaN(m256[i]) {
					continue
				}
				ys, ps, ss, ms = append(ys, y[i]), append(ps, pr[i]), append(ss, cols[c][i]), append(ms, m256[i])
			}
			if len(ys) < 30 {
				continue
			}
			ic[c][d-lo] = pearson(rankScale(ss), ys)
			cr[c][d-lo] = pearson(rankScale(ss), rankScale(ms))
			bs[c][d-lo] = slopeOf(y, [][]float64{pr, m120, m256}, cols[c])
			if c < nc && dy.twin[c] != nil {
				tw := dy.twin[c]
				if !allNaN(tw) {
					bt[c][d-lo] = slopeOfTwin(y, [][]float64{pr, m120, m256}, tw, cols[c])
				}
			}
		}
		tfmE2(dy, pr, y, e2, d-lo)
		tfmSkill(t, d, dy, skill, d-lo)
	}
	fmt.Printf("tfmb: %d candidates plus 5 placebos, baseline composite + mom120 + mom256 ranks, window %s..%s\n", nc, ds.dates[lo], ds.dates[hi])
	fmt.Printf("%-26s %8s %6s %9s %6s %8s %8s %7s %9s %s\n", "candidate", "IC", "t", "inc.slope", "t", "half1", "half2", "r_m256", "t|twin", "flags")
	hits, ctl := 0, 0
	for c := 0; c < total; c++ {
		icv, bsv := seqStat(ic[c], 0, nd-1), seqStat(bs[c], 0, nd-1)
		if len(bsv) < 100 {
			fmt.Printf("%-26s not testable (%d dates)\n", label[c], len(bsv))
			continue
		}
		mi, ti := neweyWest(icv, 10)
		mb, tb := neweyWest(bsv, 10)
		h1, h2 := halves(bs[c], split)
		rc := mean(seqStat(cr[c], 0, nd-1))
		tt := "-"
		flags := ""
		hit := tfmBFlags(mb, tb, mi, h1, h2)
		if c < nc {
			if v := seqStat(bt[c], 0, nd-1); len(v) >= 100 {
				_, ttw := neweyWest(v, 10)
				tt = fmt.Sprintf("%+.2f", ttw)
				if hit && math.Abs(ttw) < 3.0 {
					flags = "absorbed-by-twin"
				}
			}
		}
		if hit {
			flags = strings.TrimSpace("HIT " + flags)
			if c < nc {
				hits++
			} else {
				ctl++
			}
		}
		fmt.Printf("%-26s %+8.4f %+6.2f %+9.4f %+6.2f %+8.4f %+8.4f %+7.3f %9s %s\n", label[c], mi, ti, mb, tb, h1, h2, rc, tt, flags)
	}
	fmt.Printf("tfmb: hits %d of %d candidates, placebo hits %d of 5 (rule: |t| of the slope at least 3.0, same sign in both halves and for the IC; twin column is the slope t with the twin added to the baseline)\n", hits, nc, ctl)
	names := []string{"sp raw (top vs bottom tercile)", "sp stratified by |prediction|", "sp stratified by vol20", "vol20 raw (top vs bottom tercile)"}
	for k, nm := range names {
		v := seqStat(e2[k], 0, nd-1)
		if len(v) < 100 {
			fmt.Printf("tfmb E2: %s not testable\n", nm)
			continue
		}
		m, tv := neweyWest(v, 10)
		h1, h2 := halves(e2[k], split)
		fmt.Printf("tfmb E2: %-34s mean IC difference %+.4f (NW t %+.2f), halves %+.4f / %+.4f, %d dates\n", nm, m, tv, h1, h2, len(v))
	}
	for j, fn := range []string{"A2", "LevAsym", "SpLv"} {
		a, b := seqStat(skill[j][0], 0, nd-1), seqStat(skill[j][1], 0, nd-1)
		if len(a) < 100 {
			continue
		}
		d := make([]float64, 0)
		for i := range skill[j][0] {
			if !math.IsNaN(skill[j][0][i]) && !math.IsNaN(skill[j][1][i]) {
				d = append(d, skill[j][0][i]-skill[j][1][i])
			}
		}
		md, td := neweyWest(d, 10)
		fmt.Printf("tfmb E4 skill %-8s: forecast IC against realised mean flow d+1..d+10 %+.4f, naive last-10 mean IC %+.4f, difference %+.4f (NW t %+.2f)\n", fn, mean(a), mean(b), md, td)
	}
	tfmConfirm(t, pp)
	tfmCalibration(t, pp)
}

func allNaN(v []float64) bool {
	for _, x := range v {
		if !math.IsNaN(x) {
			return false
		}
	}
	return true
}

func slopeOfTwin(y []float64, ctrl [][]float64, twin, cand []float64) float64 {
	idx := []int{}
	for i := range y {
		if math.IsNaN(y[i]) || math.IsNaN(cand[i]) || math.IsNaN(twin[i]) {
			continue
		}
		ok := true
		for _, c := range ctrl {
			if math.IsNaN(c[i]) {
				ok = false
				break
			}
		}
		if ok {
			idx = append(idx, i)
		}
	}
	if len(idx) < 30 {
		return nan
	}
	sel := func(v []float64) []float64 {
		s := make([]float64, len(idx))
		for k, i := range idx {
			s[k] = v[i]
		}
		return rankScale(s)
	}
	ys := make([]float64, len(idx))
	for k, i := range idx {
		ys[k] = y[i]
	}
	var cols [][]float64
	for _, c := range ctrl {
		cols = append(cols, sel(c))
	}
	cols = append(cols, sel(twin), sel(cand))
	co := olsCoef(cols, ys)
	if co == nil {
		return nan
	}
	return co[len(co)-1]
}

func termIdx(v []float64, ok []bool, lo, hi float64) []int {
	type iv struct {
		i int
		v float64
	}
	var a []iv
	for i := range v {
		if ok[i] && !math.IsNaN(v[i]) {
			a = append(a, iv{i, v[i]})
		}
	}
	sort.Slice(a, func(p, q int) bool { return a[p].v < a[q].v })
	var o []int
	n := len(a)
	for k := int(lo * float64(n)); k < int(hi*float64(n)); k++ {
		o = append(o, a[k].i)
	}
	return o
}

func icOver(pr, y []float64, idx []int) float64 {
	if len(idx) < 8 {
		return nan
	}
	p, yy := make([]float64, len(idx)), make([]float64, len(idx))
	for k, i := range idx {
		p[k], yy[k] = pr[i], y[i]
	}
	return pearson(rankScale(p), yy)
}

func tfmE2(dy *tfmDay, pr, y []float64, e2 [4][]float64, k int) {
	n := len(dy.rows)
	ok := make([]bool, n)
	absp := filled(n)
	cnt := 0
	for i := range ok {
		ok[i] = !math.IsNaN(y[i]) && !math.IsNaN(pr[i]) && !math.IsNaN(dy.sp[i]) && !math.IsNaN(dy.vol20[i])
		if ok[i] {
			absp[i] = math.Abs(pr[i])
			cnt++
		}
	}
	if cnt < 45 {
		return
	}
	top := termIdx(dy.sp, ok, 2.0/3, 1)
	bot := termIdx(dy.sp, ok, 0, 1.0/3)
	e2[0][k] = icOver(pr, y, top) - icOver(pr, y, bot)
	strat := func(by []float64) float64 {
		var ds []float64
		for s := 0; s < 3; s++ {
			st := termIdx(by, ok, float64(s)/3, float64(s+1)/3)
			sub := make([]bool, n)
			for _, i := range st {
				sub[i] = true
			}
			hi := termIdx(dy.sp, sub, 0.5, 1)
			lw := termIdx(dy.sp, sub, 0, 0.5)
			a, b := icOver(pr, y, hi), icOver(pr, y, lw)
			if !math.IsNaN(a) && !math.IsNaN(b) {
				ds = append(ds, a-b)
			}
		}
		if len(ds) < 3 {
			return nan
		}
		return mean(ds)
	}
	e2[1][k] = strat(absp)
	e2[2][k] = strat(dy.vol20)
	vt := termIdx(dy.vol20, ok, 2.0/3, 1)
	vb := termIdx(dy.vol20, ok, 0, 1.0/3)
	e2[3][k] = icOver(pr, y, vt) - icOver(pr, y, vb)
}

func tfmSkill(t *tfmCtx, d int, dy *tfmDay, skill [3][2][]float64, k int) {
	for j := 0; j < 3; j++ {
		var f, real, nv []float64
		for i, x := range dy.rows {
			fc := dy.cand[5+j][i]
			tw := dy.twin[5+j][i]
			if math.IsNaN(fc) || math.IsNaN(tw) {
				continue
			}
			s := t.ser[x.secid]
			p, ok := t.pos[x.secid][d]
			if !ok || p+10 >= len(s.di) {
				continue
			}
			var vals []float64
			for q := p + 1; q <= p+10; q++ {
				if !math.IsNaN(s.flow[j][q]) {
					vals = append(vals, s.flow[j][q])
				}
			}
			if len(vals) < 5 {
				continue
			}
			f, real, nv = append(f, fc), append(real, mean(vals)), append(nv, tw)
		}
		if len(f) < 30 {
			continue
		}
		rr := rankScale(real)
		skill[j][0][k] = pearson(rankScale(f), rr)
		skill[j][1][k] = pearson(rankScale(nv), rr)
	}
}

func tfmConfirm(t *tfmCtx, pp *pipe) {
	ds := pp.ds
	a, b := pp.dateRange("2022-04-01", "2024-12-31")
	nd := b - a + 1
	nc := len(tfmCandNames)
	sl := make([][]float64, nc)
	st := make([][]float64, nc)
	for c := range sl {
		sl[c] = filled(nd)
		st[c] = filled(nd)
	}
	for d := a; d <= b; d++ {
		dy := t.day(d)
		y := make([]float64, len(dy.rows))
		for i, x := range dy.rows {
			y[i] = x.y10
		}
		for c := 0; c < nc; c++ {
			sl[c][d-a] = slopeOf(y, [][]float64{dy.mom[3]}, dy.cand[c])
			if !allNaN(dy.twin[c]) {
				st[c][d-a] = slopeOfTwin(y, [][]float64{dy.mom[3]}, dy.twin[c], dy.cand[c])
			}
		}
	}
	fmt.Printf("tfmb confirmation (guard 3): per-date slope of the 10-day abnormal rank on mom256 rank and the candidate, %s..%s\n", ds.dates[a], ds.dates[b])
	fmt.Printf("%-26s %9s %6s %8s %8s %8s %9s %s\n", "candidate", "slope", "NW t", "2022", "2023", "2024", "t|twin", "flag")
	for c := 0; c < nc; c++ {
		v := seqStat(sl[c], 0, nd-1)
		if len(v) < 100 {
			fmt.Printf("%-26s not testable (%d dates)\n", tfmCandNames[c], len(v))
			continue
		}
		m, tv := neweyWest(v, 10)
		var yr [3][]float64
		for i := 0; i < nd; i++ {
			if math.IsNaN(sl[c][i]) {
				continue
			}
			switch ds.dates[a+i][:4] {
			case "2022":
				yr[0] = append(yr[0], sl[c][i])
			case "2023":
				yr[1] = append(yr[1], sl[c][i])
			case "2024":
				yr[2] = append(yr[2], sl[c][i])
			}
		}
		flag := ""
		ttw := "-"
		okTwin := true
		if tv2 := seqStat(st[c], 0, nd-1); len(tv2) >= 100 {
			mt, tt := neweyWest(tv2, 10)
			ttw = fmt.Sprintf("%+.2f", tt)
			okTwin = math.Abs(tt) >= 2.0 && mt*m > 0
		}
		if math.Abs(tv) >= 2.0 && mean(yr[1])*m > 0 && mean(yr[2])*m > 0 && okTwin {
			flag = "CONFIRMED"
		}
		fmt.Printf("%-26s %+9.4f %+6.2f %+8.4f %+8.4f %+8.4f %9s %s\n", tfmCandNames[c], m, tv, mean(yr[0]), mean(yr[1]), mean(yr[2]), ttw, flag)
	}
}

func tfmCalibration(t *tfmCtx, pp *pipe) {
	ds := pp.ds
	type acc struct{ n, in, below, above int }
	steps := []int{1, 5, 10}
	cols := [][2]string{{"q10s1", "q90s1"}, {"q10s5", "q90s5"}, {"q10s10", "q90s10"}}
	res := map[string]*acc{}
	for key, v := range t.cv {
		parts := strings.SplitN(key, "|", 2)
		di := sort.SearchStrings(ds.dates, parts[0])
		s := t.ser[parts[1]]
		if s == nil {
			continue
		}
		p, ok := t.pos[parts[1]][di]
		if !ok {
			continue
		}
		for k, st := range steps {
			if p+st >= len(s.ab) {
				continue
			}
			lo, hi := v[t.col[cols[k][0]]], v[t.col[cols[k][1]]]
			if math.IsNaN(lo) || math.IsNaN(hi) {
				continue
			}
			obs := s.ab[p+st]
			for _, tag := range []string{"all", parts[0][:4]} {
				name := fmt.Sprintf("step %2d %s", st, tag)
				a := res[name]
				if a == nil {
					a = &acc{}
					res[name] = a
				}
				a.n++
				switch {
				case obs < lo:
					a.below++
				case obs > hi:
					a.above++
				default:
					a.in++
				}
			}
		}
	}
	var keys []string
	for k := range res {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Println("tfmb E0 calibration of the 80% interval (q10..q90) against the realised single-day abnormal return at the step; usable if coverage lies in 0.72-0.88 at every step")
	for _, k := range keys {
		a := res[k]
		fmt.Printf("tfmb E0: %-16s n %7d coverage %.3f below q10 %.3f above q90 %.3f\n", k, a.n, float64(a.in)/float64(a.n), float64(a.below)/float64(a.n), float64(a.above)/float64(a.n))
	}
}
