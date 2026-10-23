package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"time"
)

type dump struct {
	dates []time.Time
	raw   []string
	cols  map[string][]float64
}

func loadDump(path string) (*dump, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(bufio.NewReader(f))
	hdr, err := r.Read()
	if err != nil {
		return nil, err
	}
	d := &dump{cols: map[string][]float64{}}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		t, err := time.Parse(dayFmt, rec[0])
		if err != nil {
			return nil, err
		}
		d.dates = append(d.dates, t)
		d.raw = append(d.raw, rec[0])
		for i := 1; i < len(hdr); i++ {
			v, err := strconv.ParseFloat(rec[i], 64)
			if err != nil {
				v = math.NaN()
			}
			d.cols[hdr[i]] = append(d.cols[hdr[i]], v)
		}
	}
	return d, nil
}

type mrow struct {
	secid    string
	f10, u10 float64
	div      bool
}

type matrix struct {
	dates []string
	byDay map[string][]mrow
	ret   map[string]map[string]float64
}

func loadMatrix(path string) (*matrix, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(bufio.NewReaderSize(f, 1<<22))
	hdr, err := r.Read()
	if err != nil {
		return nil, err
	}
	ix := map[string]int{}
	for i, h := range hdr {
		ix[h] = i
	}
	m := &matrix{byDay: map[string][]mrow{}, ret: map[string]map[string]float64{}}
	pf := func(s string) float64 {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return math.NaN()
		}
		return v
	}
	seen := map[string]bool{}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		d, s := rec[ix["date"]], rec[ix["secid"]]
		if !seen[d] {
			seen[d] = true
			m.dates = append(m.dates, d)
		}
		m.byDay[d] = append(m.byDay[d], mrow{secid: s, f10: pf(rec[ix["f10"]]), u10: pf(rec[ix["u10"]]), div: rec[ix["div_drop"]] == "1"})
		if m.ret[s] == nil {
			m.ret[s] = map[string]float64{}
		}
		m.ret[s][d] = pf(rec[ix["rb1"]])
	}
	sort.Strings(m.dates)
	return m, nil
}

func rankScale(v []float64) []float64 {
	n := len(v)
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return v[idx[a]] < v[idx[b]] })
	out := make([]float64, n)
	for i := 0; i < n; {
		j := i
		for j+1 < n && v[idx[j+1]] == v[idx[i]] {
			j++
		}
		r := float64(i+j)/2/float64(n-1) - 0.5
		for k := i; k <= j; k++ {
			out[idx[k]] = r
		}
		i = j + 1
	}
	return out
}

func slopeOn(day []mrow, beta []float64) float64 {
	var y, x []float64
	for i, r := range day {
		if r.div || math.IsNaN(r.f10) || math.IsNaN(r.u10) || math.IsNaN(beta[i]) {
			continue
		}
		y = append(y, r.f10-r.u10)
		x = append(x, beta[i])
	}
	if len(y) < 30 {
		return math.NaN()
	}
	ry, rx := rankScale(y), rankScale(x)
	var sxy, sxx float64
	for i := range ry {
		sxy += ry[i] * rx[i]
		sxx += rx[i] * rx[i]
	}
	return sxy / sxx
}

func rollBeta(m *matrix, f *factor, di int, secid string) float64 {
	var xs, ys []float64
	for k := di - 60; k < di; k++ {
		if k < 0 {
			continue
		}
		d := m.dates[k]
		c, ok := f.chg[d]
		if !ok {
			continue
		}
		r, ok := m.ret[secid][d]
		if !ok || math.IsNaN(r) {
			continue
		}
		xs = append(xs, c)
		ys = append(ys, r)
	}
	if len(xs) < 40 {
		return math.NaN()
	}
	mx, my := mean(xs), mean(ys)
	var sxy, sxx float64
	for i := range xs {
		sxy += (xs[i] - mx) * (ys[i] - my)
		sxx += (xs[i] - mx) * (xs[i] - mx)
	}
	if sxx == 0 {
		return math.NaN()
	}
	return sxy / sxx
}

type testRow struct {
	id, use, kind string
	d, d1, d2, dA float64
	n1, n0        int
	p, nullSD     float64
	shifts        int
	ok            bool
	thr           float64
	w2            bool
}

func diffMeans(s, y []float64) (float64, int, int) {
	var a, b float64
	var na, nb int
	for i := range s {
		if math.IsNaN(s[i]) || math.IsNaN(y[i]) {
			continue
		}
		if s[i] == 1 {
			a += y[i]
			na++
		} else {
			b += y[i]
			nb++
		}
	}
	if na == 0 || nb == 0 {
		return math.NaN(), na, nb
	}
	return a/float64(na) - b/float64(nb), na, nb
}

func slopeXY(x, y []float64) float64 {
	var xs, ys []float64
	for i := range x {
		if !math.IsNaN(x[i]) && !math.IsNaN(y[i]) {
			xs = append(xs, x[i])
			ys = append(ys, y[i])
		}
	}
	if len(xs) < 60 {
		return math.NaN()
	}
	mx, my := mean(xs), mean(ys)
	var sxy, sxx float64
	for i := range xs {
		sxy += (xs[i] - mx) * (ys[i] - my)
		sxx += (xs[i] - mx) * (xs[i] - mx)
	}
	return sxy / sxx
}

func shifted(s []float64, k int) []float64 {
	n := len(s)
	o := make([]float64, n)
	for i := range o {
		o[i] = s[(i+k)%n]
	}
	return o
}

func permTest(stat func(s []float64) float64, s []float64, minSide func(s []float64) bool) (float64, float64, int) {
	obs := stat(s)
	n := len(s)
	ge, cnt := 0, 0
	var ss, s1 float64
	for k := 20; k <= n-20; k++ {
		sh := shifted(s, k)
		v := stat(sh)
		if math.IsNaN(v) {
			continue
		}
		cnt++
		s1 += v
		ss += v * v
		if math.Abs(v) >= math.Abs(obs) {
			ge++
		}
	}
	if cnt == 0 {
		return math.NaN(), math.NaN(), 0
	}
	mu := s1 / float64(cnt)
	sd := math.Sqrt(math.Max(ss/float64(cnt)-mu*mu, 0))
	return float64(1+ge) / float64(1+cnt), sd, cnt
}

func jacobiEigen(a [][]float64) []float64 {
	n := len(a)
	m := make([][]float64, n)
	for i := range m {
		m[i] = append([]float64(nil), a[i]...)
	}
	for sweep := 0; sweep < 100; sweep++ {
		off := 0.0
		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				off += m[i][j] * m[i][j]
			}
		}
		if off < 1e-18 {
			break
		}
		for p := 0; p < n; p++ {
			for q := p + 1; q < n; q++ {
				if math.Abs(m[p][q]) < 1e-15 {
					continue
				}
				th := (m[q][q] - m[p][p]) / (2 * m[p][q])
				t := math.Copysign(1, th) / (math.Abs(th) + math.Sqrt(th*th+1))
				c := 1 / math.Sqrt(t*t+1)
				s := t * c
				for k := 0; k < n; k++ {
					kp, kq := m[k][p], m[k][q]
					m[k][p] = c*kp - s*kq
					m[k][q] = s*kp + c*kq
				}
				for k := 0; k < n; k++ {
					pk, qk := m[p][k], m[q][k]
					m[p][k] = c*pk - s*qk
					m[q][k] = s*pk + c*qk
				}
			}
		}
	}
	e := make([]float64, n)
	for i := range e {
		e[i] = m[i][i]
	}
	return e
}

func pairCorr(a, b []float64) float64 {
	var xs, ys []float64
	for i := range a {
		if !math.IsNaN(a[i]) && !math.IsNaN(b[i]) {
			xs = append(xs, a[i])
			ys = append(ys, b[i])
		}
	}
	if len(xs) < 30 {
		return 0
	}
	mx, my := mean(xs), mean(ys)
	var sxy, sxx, syy float64
	for i := range xs {
		sxy += (xs[i] - mx) * (ys[i] - my)
		sxx += (xs[i] - mx) * (xs[i] - mx)
		syy += (ys[i] - my) * (ys[i] - my)
	}
	if sxx == 0 || syy == 0 {
		return 0
	}
	return sxy / math.Sqrt(sxx*syy)
}

func nyholt(series [][]float64) float64 {
	m := len(series)
	c := make([][]float64, m)
	for i := range c {
		c[i] = make([]float64, m)
		for j := range c[i] {
			if i == j {
				c[i][j] = 1
			} else {
				c[i][j] = pairCorr(series[i], series[j])
			}
		}
	}
	e := jacobiEigen(c)
	mu := mean(e)
	var v float64
	for _, x := range e {
		v += (x - mu) * (x - mu)
	}
	v /= float64(m - 1)
	return 1 + float64(m-1)*(1-v/float64(m))
}

func run(dir, seriesPath, matrixPath, newsPath string, wave2 bool, exA2Path string) error {
	dp, err := loadDump(seriesPath)
	if err != nil {
		return err
	}
	n := len(dp.dates)
	meanOf := func(v []float64) float64 {
		var s float64
		k := 0
		for _, x := range v {
			if !math.IsNaN(x) {
				s += x
				k++
			}
		}
		return s / float64(k)
	}
	mIC, mNet := meanOf(dp.cols["ic_blend"]), meanOf(dp.cols["net_gp_blend"])
	fmt.Printf("dump check: mean ic_blend %.5f (verdict 0.0957), mean net_gp_blend %.5f%% (verdict 0.0298%%)\n", mIC, mNet*100)
	if math.Abs(mIC-0.0957) > 0.00006 || math.Abs(mNet*100-0.0298) > 0.00006 {
		return fmt.Errorf("dump does not reproduce the verdict series")
	}
	split := sort.SearchStrings(dp.raw, "2026-01-05")
	ss, fac, err := buildStates(dir, newsPath, dp.dates)
	if err != nil {
		return err
	}
	isW2 := map[string]bool{}
	nW1 := len(ss.ids)
	if wave2 {
		if err := addWave2States(dir, dp.dates, ss, fac); err != nil {
			return err
		}
		for _, id := range ss.ids[nW1:] {
			isW2[id] = true
		}
	}
	mx, err := loadMatrix(matrixPath)
	if err != nil {
		return err
	}
	dayIdx := map[string]int{}
	for i, d := range mx.dates {
		dayIdx[d] = i
	}
	type pair struct{ id, exp, cond string }
	pairs := []pair{{"I01", "brent", "brent"}, {"I02", "cny", "cny"}, {"I03", "gold", "gold"}, {"I04", "imoex", "imoex"}, {"I05", "imoex", "rvi"}}
	nP1 := len(pairs)
	if wave2 {
		pairs = append(pairs, pair{"J01", "moexog", "moexog"}, pair{"J02", "moexfn", "moexfn"}, pair{"J03", "moexmm", "moexmm"}, pair{"J04", "moexch", "moexch"}, pair{"J05", "moexeu", "moexeu"}, pair{"J06", "moextl", "moextl"})
		for _, pr := range pairs[nP1:] {
			isW2[pr.id] = true
		}
	}
	slopes := map[string][]float64{}
	zs := map[string][]float64{}
	for _, pr := range pairs {
		sl := make([]float64, n)
		z := make([]float64, n)
		for i, d := range dp.raw {
			sl[i] = math.NaN()
			z[i] = fac[pr.cond].tenDayZ(dp.dates[i])
			di, ok := dayIdx[d]
			if !ok {
				continue
			}
			day := mx.byDay[d]
			beta := make([]float64, len(day))
			for k, r := range day {
				beta[k] = rollBeta(mx, fac[pr.exp], di, r.secid)
			}
			sl[i] = slopeOn(day, beta)
		}
		slopes[pr.id] = sl
		zs[pr.id] = z
	}
	var meffIn, meffIn1 [][]float64
	for i, id := range ss.ids {
		meffIn = append(meffIn, ss.values[id])
		if i < nW1 {
			meffIn1 = append(meffIn1, ss.values[id])
		}
	}
	for i, pr := range pairs {
		meffIn = append(meffIn, zs[pr.id])
		if i < nP1 {
			meffIn1 = append(meffIn1, zs[pr.id])
		}
	}
	meff1 := nyholt(meffIn1)
	meff := nyholt(meffIn)
	thr1 := 0.05 / meff1
	thr := 0.05 / meff
	if wave2 {
		fmt.Printf("wave 1 M_eff %.2f threshold %.4f; enlarged M_eff %.2f threshold %.4f for wave-2 rows\n", meff1, thr1, meff, thr)
	} else {
		thr = thr1
		meff = meff1
	}
	fmt.Printf("realtime dates %d, halves split index %d (%s), M_eff %.2f, threshold p <= %.4f\n", n, split, dp.raw[split], meff, thr)

	outcomes := map[string][]float64{"R": dp.cols["ic_blend"], "S": dp.cols["net_gp_blend"], "T": dp.cols["u10"]}
	base := map[string][]float64{"R": dp.cols["ic_a2"], "S": dp.cols["net_gp_a2"]}
	var rows []testRow
	for _, id := range ss.ids {
		s := ss.values[id]
		for _, use := range []string{"R", "S", "T"} {
			y := outcomes[use]
			t := testRow{id: id, use: use, kind: "state", thr: thr1, w2: isW2[id]}
			if t.w2 {
				t.thr = thr
			}
			d, n1, n0 := diffMeans(s, y)
			t.d, t.n1, t.n0 = d, n1, n0
			if n1 < 40 || n0 < 40 {
				rows = append(rows, t)
				continue
			}
			t.ok = true
			t.d1, _, _ = diffMeans(s[:split], y[:split])
			t.d2, _, _ = diffMeans(s[split:], y[split:])
			if b, has := base[use]; has {
				t.dA, _, _ = diffMeans(s, b)
			} else {
				t.dA = math.NaN()
			}
			yy := y
			t.p, t.nullSD, t.shifts = permTest(func(sh []float64) float64 {
				v, a, b := diffMeans(sh, yy)
				if a < 10 || b < 10 {
					return math.NaN()
				}
				return v
			}, s, nil)
			rows = append(rows, t)
		}
	}
	for _, pr := range pairs {
		z := zs[pr.id]
		sl := slopes[pr.id]
		t := testRow{id: pr.id, use: "I", kind: "interaction", thr: thr1, w2: isW2[pr.id]}
		if t.w2 {
			t.thr = thr
		}
		t.d = slopeXY(z, sl)
		cnt := 0
		for i := range z {
			if !math.IsNaN(z[i]) && !math.IsNaN(sl[i]) {
				cnt++
			}
		}
		t.n1 = cnt
		if cnt < 100 {
			rows = append(rows, t)
			continue
		}
		t.ok = true
		t.d1 = slopeXY(z[:split], sl[:split])
		t.d2 = slopeXY(z[split:], sl[split:])
		t.dA = math.NaN()
		slc := sl
		t.p, t.nullSD, t.shifts = permTest(func(sh []float64) float64 { return slopeXY(sh, slc) }, z, nil)
		rows = append(rows, t)
	}
	var realP []float64
	for _, t := range rows {
		if t.ok && t.id[0] != 'C' {
			realP = append(realP, t.p)
		}
	}
	sort.Float64s(realP)
	bhCut := 0.0
	for i, p := range realP {
		if p <= 0.10*float64(i+1)/float64(len(realP)) {
			bhCut = p
		}
	}
	fmt.Printf("%-4s %-2s %5s %5s %10s %10s %10s %10s %8s %8s %6s  flags\n", "id", "use", "n1", "n0", "D", "D_h1", "D_h2", "D_a2", "nullSD", "p", "shifts")
	hits := map[string]int{}
	ctlHits := map[string]int{}
	for _, t := range rows {
		if !t.ok {
			fmt.Printf("%-4s %-2s %5d %5d  not testable\n", t.id, t.use, t.n1, t.n0)
			continue
		}
		flags := ""
		sameSign := t.d1*t.d > 0 && t.d2*t.d > 0
		a2ok := math.IsNaN(t.dA) || t.dA*t.d > 0
		hit := t.p <= t.thr && sameSign && a2ok
		if t.p <= t.thr {
			flags += " P"
		}
		if t.p <= bhCut && bhCut > 0 && t.id[0] != 'C' {
			flags += " BH"
		}
		if sameSign {
			flags += " H"
		}
		if hit {
			flags += " HIT"
			if t.id[0] == 'C' {
				ctlHits[t.use]++
			} else {
				hits[t.use]++
			}
		}
		fmt.Printf("%-4s %-2s %5d %5d %+10.5f %+10.5f %+10.5f %+10.5f %8.5f %8.4f %6d %s\n", t.id, t.use, t.n1, t.n0, t.d, t.d1, t.d2, t.dA, t.nullSD, t.p, t.shifts, flags)
	}
	for _, t := range rows {
		if !t.ok || t.id[0] == 'C' || t.use == "I" || t.p > t.thr {
			continue
		}
		s := ss.values[t.id]
		for _, u := range []string{"R", "S"} {
			if u != t.use {
				continue
			}
			for _, y := range []struct {
				name string
				v    []float64
			}{{"ic_blend", dp.cols["ic_blend"]}, {"ic_a2", dp.cols["ic_a2"]}, {"net_gp_blend", dp.cols["net_gp_blend"]}, {"net_gp25_blend", dp.cols["net_gp25_blend"]}} {
				var a, b float64
				var na, nb int
				for i := range s {
					if math.IsNaN(s[i]) || math.IsNaN(y.v[i]) {
						continue
					}
					if s[i] == 1 {
						a += y.v[i]
						na++
					} else {
						b += y.v[i]
						nb++
					}
				}
				fmt.Printf("describe %s %s: state=1 mean %+.5f (n=%d), state=0 mean %+.5f (n=%d)\n", t.id, y.name, a/float64(na), na, b/float64(nb), nb)
			}
		}
	}
	fmt.Printf("real hits by use: %v; control hits by use: %v; BH cut %.4f\n", hits, ctlHits, bhCut)
	if wave2 {
		return diagnostics(dir, dp, ss, mx, exA2Path)
	}
	return nil
}
