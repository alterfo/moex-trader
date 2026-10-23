package main

import (
	"fmt"
	"math"
	"sort"
)

type mktSeries struct {
	f1, f2, f3, v1, v2, v3 []float64
	retH1, retH5           []float64
	rv, rvH1, rvH5         []float64
	disp, dispH1           []float64
	turn                   []float64
}

func nanSlice(n int) []float64 {
	v := make([]float64, n)
	for i := range v {
		v[i] = nan
	}
	return v
}

func zTrailing(x []float64, d, n, minValid int) float64 {
	if math.IsNaN(x[d]) {
		return nan
	}
	var v []float64
	for k := d - n; k < d; k++ {
		if k >= 0 && !math.IsNaN(x[k]) {
			v = append(v, x[k])
		}
	}
	if len(v) < minValid {
		return nan
	}
	sd := stdOf(v)
	if !(sd > 0) {
		return nan
	}
	return (x[d] - mean(v)) / sd
}

func (r *run) buildMkt() *mktSeries {
	p := r.p
	n := len(p.dates)
	m := &mktSeries{}
	raw := [3][]float64{nanSlice(n), nanSlice(n), nanSlice(n)}
	absZ := nanSlice(n)
	stdZ := nanSlice(n)
	m.rv, m.disp, m.turn = nanSlice(n), nanSlice(n), nanSlice(n)
	for d := 61; d < n; d++ {
		if p.excluded[d] {
			continue
		}
		adm := p.admittedAt(d)
		var a2 []float64
		var top []float64
		pos := 0
		var zs []float64
		var rvs, rets, trn []float64
		for k, i := range adm {
			v := p.v(i, d, bA2)
			if !math.IsNaN(v) {
				a2 = append(a2, v)
				if hist := prevVals(p, i, d, bA2, 60, false); len(hist) >= 40 {
					if sd := stdOf(hist); sd > 0 {
						zs = append(zs, (v-mean(hist))/sd)
					}
				}
				if v > 0 {
					pos++
				}
				if k < 3 {
					top = append(top, v)
				}
			}
			if x := p.v(i, d, bRV); x > 0 {
				rvs = append(rvs, x)
			}
			if x := p.logRet(i, d); !math.IsNaN(x) {
				rets = append(rets, x)
			}
			if x := p.turnZ(i, d); !math.IsNaN(x) {
				trn = append(trn, x)
			}
		}
		if len(a2) >= 40 {
			raw[0][d] = mean(a2)
			raw[1][d] = float64(pos) / float64(len(a2))
			if len(top) == 3 {
				raw[2][d] = mean(top)
			}
			if len(zs) >= 40 {
				az := make([]float64, len(zs))
				for q, z := range zs {
					az[q] = math.Abs(z)
				}
				absZ[d] = mean(az)
				stdZ[d] = stdOf(zs)
			}
		}
		if len(rvs) >= 40 {
			m.rv[d] = math.Log(mean(rvs))
		}
		if len(rets) >= 40 {
			m.disp[d] = math.Log(stdOf(rets))
		}
		if len(trn) >= 40 {
			m.turn[d] = mean(trn)
		}
	}
	m.f1, m.f2, m.f3, m.v1, m.v2, m.v3 = nanSlice(n), nanSlice(n), nanSlice(n), nanSlice(n), nanSlice(n), nanSlice(n)
	for d := 0; d < n; d++ {
		m.f1[d] = zTrailing(raw[0], d, 60, 40)
		m.f2[d] = zTrailing(raw[1], d, 60, 40)
		m.f3[d] = zTrailing(raw[2], d, 60, 40)
		m.v1[d] = absZ[d]
		m.v3[d] = stdZ[d]
		m.v2[d] = math.Abs(m.f2[d])
	}
	m.retH1, m.retH5 = nanSlice(n), nanSlice(n)
	m.rvH1, m.rvH5, m.dispH1 = nanSlice(n), nanSlice(n), nanSlice(n)
	for d := 0; d+5 < n; d++ {
		span := true
		for s := d; s <= d+5; s++ {
			if p.excluded[s] {
				span = false
			}
		}
		if !span {
			continue
		}
		m.retH1[d] = p.uniRet[d+1]
		s5, ok5 := 0.0, true
		r5, ok5r := 0.0, 0
		for k := d + 1; k <= d+5; k++ {
			if math.IsNaN(p.uniRet[k]) {
				ok5 = false
			} else {
				s5 += p.uniRet[k]
			}
			if !math.IsNaN(m.rv[k]) {
				r5 += m.rv[k]
				ok5r++
			}
		}
		if ok5 {
			m.retH5[d] = s5
		}
		m.rvH1[d] = m.rv[d+1]
		if ok5r >= 4 {
			m.rvH5[d] = r5 / float64(ok5r)
		}
		m.dispH1[d] = m.disp[d+1]
	}
	return m
}

func corr(a, b []float64) float64 {
	var x, y []float64
	for i := range a {
		if !math.IsNaN(a[i]) && !math.IsNaN(b[i]) {
			x = append(x, a[i])
			y = append(y, b[i])
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

func (r *run) mktPrep() {
	m := r.buildMkt()
	w := r.w
	ser := [][]float64{m.f1, m.f2, m.f3, m.v1, m.v3}
	names := []string{"F1", "F2", "F3", "V1", "V3"}
	lo, hi := r.win.lo, r.win.hi
	cut := func(v []float64) []float64 {
		o := make([]float64, len(v))
		for i := range o {
			o[i] = nan
			if i >= lo && i <= hi {
				o[i] = v[i]
			}
		}
		return o
	}
	k := len(ser)
	R := make([][]float64, k)
	for i := range R {
		R[i] = make([]float64, k)
		for j := range R[i] {
			R[i][j] = corr(cut(ser[i]), cut(ser[j]))
			if i == j {
				R[i][j] = 1
			}
		}
	}
	ev := jacobiEigen(R)
	mu := 0.0
	for _, e := range ev {
		mu += e
	}
	mu /= float64(k)
	v := 0.0
	for _, e := range ev {
		v += (e - mu) * (e - mu)
	}
	v /= float64(k - 1)
	mEff := 1 + float64(k-1)*(1-v/float64(k))
	thr := math.Max(3.0, normQuantile(0.05/mEff))
	fmt.Fprintf(w, "mktprep: window %s..%s, features %v\n", r.p.dates[lo], r.p.dates[hi], names)
	for i := range R {
		fmt.Fprintf(w, "  %s %v\n", names[i], fmtRow(R[i]))
	}
	fmt.Fprintf(w, "mktprep: M %d, Nyholt M_eff %.3f, T_M %.3f\n", k, mEff, thr)
}

func fmtRow(v []float64) string {
	s := ""
	for i, x := range v {
		if i > 0 {
			s += " "
		}
		s += fmt.Sprintf("%+.3f", x)
	}
	return s
}

func inverse(a [][]float64) ([][]float64, bool) {
	n := len(a)
	m := make([][]float64, n)
	for i := range m {
		m[i] = make([]float64, 2*n)
		copy(m[i], a[i])
		m[i][n+i] = 1
	}
	for c := 0; c < n; c++ {
		piv := c
		for r := c + 1; r < n; r++ {
			if math.Abs(m[r][c]) > math.Abs(m[piv][c]) {
				piv = r
			}
		}
		if math.Abs(m[piv][c]) < 1e-12 {
			return nil, false
		}
		m[c], m[piv] = m[piv], m[c]
		d := m[c][c]
		for j := range m[c] {
			m[c][j] /= d
		}
		for r := 0; r < n; r++ {
			if r == c {
				continue
			}
			f := m[r][c]
			for j := range m[r] {
				m[r][j] -= f * m[c][j]
			}
		}
	}
	out := make([][]float64, n)
	for i := range out {
		out[i] = m[i][n:]
	}
	return out, true
}

func tsHAC(X [][]float64, y []float64, lags int) (float64, float64, bool) {
	n, k := len(y), len(X[0])
	xtx := make([][]float64, k)
	for i := range xtx {
		xtx[i] = make([]float64, k)
	}
	xty := make([]float64, k)
	for t := 0; t < n; t++ {
		for i := 0; i < k; i++ {
			xty[i] += X[t][i] * y[t]
			for j := 0; j < k; j++ {
				xtx[i][j] += X[t][i] * X[t][j]
			}
		}
	}
	inv, ok := inverse(xtx)
	if !ok {
		return 0, 0, false
	}
	b := make([]float64, k)
	for i := 0; i < k; i++ {
		for j := 0; j < k; j++ {
			b[i] += inv[i][j] * xty[j]
		}
	}
	u := make([][]float64, n)
	for t := 0; t < n; t++ {
		e := y[t]
		for i := 0; i < k; i++ {
			e -= b[i] * X[t][i]
		}
		u[t] = make([]float64, k)
		for i := 0; i < k; i++ {
			u[t][i] = X[t][i] * e
		}
	}
	S := make([][]float64, k)
	for i := range S {
		S[i] = make([]float64, k)
	}
	for l := 0; l <= lags; l++ {
		wgt := 1.0
		if l > 0 {
			wgt = 1 - float64(l)/float64(lags+1)
		}
		for t := l; t < n; t++ {
			for i := 0; i < k; i++ {
				for j := 0; j < k; j++ {
					v := u[t][i] * u[t-l][j] * wgt
					S[i][j] += v
					if l > 0 {
						S[j][i] += v
					}
				}
			}
		}
	}
	V := make([][]float64, k)
	for i := range V {
		V[i] = make([]float64, k)
		for j := 0; j < k; j++ {
			for a := 0; a < k; a++ {
				for c := 0; c < k; c++ {
					V[i][j] += inv[i][a] * S[a][c] * inv[c][j]
				}
			}
		}
	}
	se := math.Sqrt(V[1][1])
	if !(se > 0) {
		return b[1], 0, false
	}
	return b[1], b[1] / se, true
}

type mktRow struct {
	name   string
	x      []float64
	y      []float64
	ctrls  [][]float64
	h      int
	gainOK bool
	res    struct {
		coef, t, p float64
		hc, ht     [2]float64
		n          int
		gain       float64
	}
}

func (r *run) mktRows(m *mktSeries) []*mktRow {
	rets := [][]float64{m.retH1, m.retH5}
	hs := []int{1, 5}
	var rows []*mktRow
	for _, f := range []struct {
		n string
		x []float64
	}{{"F1 mean A2", m.f1}, {"F2 breadth", m.f2}, {"F3 top3 A2", m.f3}} {
		for k := range rets {
			rows = append(rows, &mktRow{name: fmt.Sprintf("M %s -> ret h%d", f.n, hs[k]), x: f.x, y: rets[k], h: hs[k]})
		}
	}
	avg := func(x []float64, d, n int) float64 {
		s, c := 0.0, 0
		for k := d - n + 1; k <= d; k++ {
			if k >= 0 && !math.IsNaN(x[k]) {
				s += x[k]
				c++
			}
		}
		if c < n-1 {
			return nan
		}
		return s / float64(c)
	}
	ctl := func(base []float64) [][]float64 {
		n := len(base)
		c1, c2, c3 := nanSlice(n), nanSlice(n), nanSlice(n)
		for d := 0; d < n; d++ {
			c1[d] = avg(base, d, 5)
			c2[d] = avg(base, d, 20)
			c3[d] = m.turn[d]
		}
		return [][]float64{base, c1, c2, c3}
	}
	type vt struct {
		n string
		y []float64
		h int
		c [][]float64
	}
	targets := []vt{{"log RV h1", m.rvH1, 1, ctl(m.rv)}, {"log RV h5", m.rvH5, 5, ctl(m.rv)}, {"log dispersion h1", m.dispH1, 1, ctl(m.disp)}}
	for _, f := range []struct {
		n string
		x []float64
	}{{"V1 mean |A2|", m.v1}, {"V2 |breadth|", m.v2}, {"V3 std A2", m.v3}} {
		for _, t := range targets {
			rows = append(rows, &mktRow{name: fmt.Sprintf("V %s -> %s", f.n, t.n), x: f.x, y: t.y, ctrls: t.c, h: t.h, gainOK: true})
		}
	}
	return rows
}

func (r *run) designMkt(row *mktRow, d int, withX bool) ([]float64, bool) {
	v := []float64{1}
	if withX {
		if math.IsNaN(row.x[d]) {
			return nil, false
		}
		v = append(v, row.x[d])
	}
	for _, c := range row.ctrls {
		if math.IsNaN(c[d]) {
			return nil, false
		}
		v = append(v, c[d])
	}
	return v, true
}

func (r *run) mktTest(row *mktRow) {
	var X [][]float64
	var y []float64
	var half []int
	for d := r.win.lo; d <= r.win.hi && d < len(row.y); d++ {
		if math.IsNaN(row.y[d]) {
			continue
		}
		x, ok := r.designMkt(row, d, true)
		if !ok {
			continue
		}
		X = append(X, x)
		y = append(y, row.y[d])
		if d < r.win.split {
			half = append(half, 0)
		} else {
			half = append(half, 1)
		}
	}
	lags := lagsFor(row.h)
	row.res.n = len(y)
	if len(y) < 60 {
		row.res.t, row.res.p = nan, nan
		return
	}
	b, t, ok := tsHAC(X, y, lags)
	if !ok {
		row.res.t, row.res.p = nan, nan
		return
	}
	row.res.coef, row.res.t = b, t
	row.res.p = 2 * (1 - normCDF(math.Abs(t)))
	for h := 0; h < 2; h++ {
		var xh [][]float64
		var yh []float64
		for i := range y {
			if half[i] == h {
				xh = append(xh, X[i])
				yh = append(yh, y[i])
			}
		}
		if len(yh) >= 40 {
			if bh, th, ok := tsHAC(xh, yh, lags); ok {
				row.res.hc[h], row.res.ht[h] = bh, th
			}
		}
	}
}

func (r *run) mktGain(row *mktRow) float64 {
	var sseC, sseF, sst float64
	for _, e := range foldEdgesFlow() {
		lo := r.p.dateIndex(e[0])
		hi := r.p.dateIndex(e[1])
		if hi >= len(r.p.dates) || r.p.dates[hi] > e[1] {
			hi--
		}
		var X0, X1 [][]float64
		var y []float64
		for d := 0; d+row.h < lo-row.h && d < len(row.y); d++ {
			if math.IsNaN(row.y[d]) {
				continue
			}
			a, ok1 := r.designMkt(row, d, false)
			b, ok2 := r.designMkt(row, d, true)
			if !ok1 || !ok2 {
				continue
			}
			X0, X1, y = append(X0, a), append(X1, b), append(y, row.y[d])
		}
		if len(y) < 80 {
			continue
		}
		b0, ok0 := ols(X0, y)
		b1, ok1 := ols(X1, y)
		if !ok0 || !ok1 {
			continue
		}
		ym := mean(y)
		for d := lo; d <= hi && d < len(row.y); d++ {
			if math.IsNaN(row.y[d]) {
				continue
			}
			a, ok1 := r.designMkt(row, d, false)
			b, ok2 := r.designMkt(row, d, true)
			if !ok1 || !ok2 {
				continue
			}
			p0, p1 := 0.0, 0.0
			for j := range a {
				p0 += b0[j] * a[j]
			}
			for j := range b {
				p1 += b1[j] * b[j]
			}
			sseC += (row.y[d] - p0) * (row.y[d] - p0)
			sseF += (row.y[d] - p1) * (row.y[d] - p1)
			sst += (row.y[d] - ym) * (row.y[d] - ym)
		}
	}
	if sst == 0 {
		return nan
	}
	return (sseC - sseF) / sst
}

func (r *run) mktRun(tm float64) {
	m := r.buildMkt()
	rows := r.mktRows(m)
	w := r.w
	for _, row := range rows {
		r.mktTest(row)
		if row.gainOK {
			row.res.gain = r.mktGain(row)
		}
	}
	fam := func(prefix string) {
		var sel []*mktRow
		for _, row := range rows {
			if row.name[:1] == prefix {
				sel = append(sel, row)
			}
		}
		idx := make([]int, len(sel))
		for i := range idx {
			idx[i] = i
		}
		sort.Slice(idx, func(a, b int) bool { return sel[idx[a]].res.p < sel[idx[b]].res.p })
		cut := -1
		for k, i := range idx {
			if !math.IsNaN(sel[i].res.p) && sel[i].res.p <= 0.05*float64(k+1)/float64(len(sel)) {
				cut = k
			}
		}
		pass := 0
		fmt.Fprintf(w, "\n== Family %s: pooled t >= %.3f (T_M), same sign in both halves, BH q=0.05 over %d rows, one-sided registered sign + for M, two-sided for V\n", prefix, tm, len(sel))
		for k, i := range idx {
			x := sel[i]
			tt := x.res.t
			if prefix == "V" {
				tt = math.Abs(tt)
			}
			same := x.res.hc[0]*x.res.hc[1] > 0 && x.res.hc[0]*x.res.coef > 0
			if prefix == "M" {
				same = same && x.res.hc[0] > 0 && x.res.hc[1] > 0
			}
			p := x.res.p
			ok := tt >= tm && same && k <= cut
			extra := ""
			if x.gainOK {
				extra = fmt.Sprintf(" R2 gain %+.4f (material at 0.01: %v)", x.res.gain, x.res.gain >= 0.01)
			}
			if ok {
				pass++
			}
			fmt.Fprintf(w, "%s coef %+.5f t %+.2f halves t %+.2f / %+.2f p %.4f BH %v same-sign %v n %d%s -> %s\n", x.name, x.res.coef, x.res.t, x.res.ht[0], x.res.ht[1], p, k <= cut, same, x.res.n, extra, passWord(ok))
		}
		fmt.Fprintf(w, "family %s passed %d of %d\n", prefix, pass, len(sel))
	}
	fam("M")
	fam("V")
}
