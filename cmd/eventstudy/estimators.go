package main

import (
	"math"
	"math/rand"
	"sort"
)

type window struct {
	a, b int
}

func (p *panel) cleanControl(td topicData, tk, t0 int) bool {
	for t := t0 - 10; t <= t0+10; t++ {
		if t < 0 || t >= len(p.keys) {
			continue
		}
		if td.flagged[tk][t] {
			return false
		}
	}
	return true
}

func (p *panel) didValue(td topicData, tk, t0 int, w window) (float64, bool) {
	width := float64(w.b - w.a + 1)
	post, ok := p.car(tk, t0, w.a, w.b)
	if !ok {
		return 0, false
	}
	pre, ok := p.meanAR(tk, t0-5, t0-1)
	if !ok {
		return 0, false
	}
	treated := post - pre*width
	sum, cnt := 0.0, 0
	for c := range p.tickers {
		if c == tk || !p.cleanControl(td, c, t0) {
			continue
		}
		cp, ok1 := p.car(c, t0, w.a, w.b)
		cpre, ok2 := p.meanAR(c, t0-5, t0-1)
		if !ok1 || !ok2 {
			continue
		}
		sum += cp - cpre*width
		cnt++
	}
	if cnt < 3 {
		return 0, false
	}
	return treated - sum/float64(cnt), true
}

type evStats struct {
	car interval
	did interval
}

func (p *panel) eventStats(td topicData, w window, reps int, rng *rand.Rand) evStats {
	var cv, dv []float64
	var ck, dk []int
	for _, e := range td.events {
		if v, ok := p.car(e.tk, e.t0, w.a, w.b); ok {
			cv = append(cv, v)
			ck = append(ck, e.t0)
		}
		if v, ok := p.didValue(td, e.tk, e.t0, w); ok {
			dv = append(dv, v)
			dk = append(dk, e.t0)
		}
	}
	return evStats{
		car: dateBootstrap(cv, ck, reps, rng),
		did: dateBootstrap(dv, dk, reps, rng),
	}
}

func (p *panel) placeboP(td topicData, w window, observed float64, reps int, rng *rand.Rand) float64 {
	if len(td.events) == 0 {
		return math.NaN()
	}
	n := len(p.keys)
	count := 0
	valid := 0
	for r := 0; r < reps; r++ {
		sum, cnt := 0.0, 0
		for _, e := range td.events {
			for try := 0; try < 6; try++ {
				shift := 20 + rng.Intn(41)
				if rng.Intn(2) == 0 {
					shift = -shift
				}
				t0 := e.t0 + shift
				if t0 < p.studyStart || t0+w.b >= n {
					continue
				}
				if v, ok := p.didValue(td, e.tk, t0, w); ok {
					sum += v
					cnt++
					break
				}
			}
		}
		if cnt == 0 {
			continue
		}
		valid++
		if math.Abs(sum/float64(cnt)) >= math.Abs(observed) {
			count++
		}
	}
	if valid == 0 {
		return math.NaN()
	}
	return float64(count+1) / float64(valid+1)
}

type twfeResult struct {
	ks   []int
	coef []float64
	se   []float64
}

func (p *panel) twfe(td topicData, ks []int) (twfeResult, bool) {
	n := len(p.keys)
	type obs struct {
		tk, t int
		y     float64
		x     []float64
	}
	treat := make([]map[int]bool, len(p.tickers))
	for i := range treat {
		treat[i] = make(map[int]bool)
	}
	for _, e := range td.events {
		treat[e.tk][e.t0] = true
	}
	var rows []obs
	for tk := range p.tickers {
		for t := p.studyStart; t < n; t++ {
			if math.IsNaN(p.ar[tk][t]) {
				continue
			}
			x := make([]float64, len(ks))
			for j, k := range ks {
				if treat[tk][t-k] {
					x[j] = 1
				}
			}
			rows = append(rows, obs{tk: tk, t: t, y: p.ar[tk][t], x: x})
		}
	}
	if len(rows) == 0 {
		return twfeResult{}, false
	}
	cols := len(ks) + 1
	data := make([][]float64, cols)
	for c := range data {
		data[c] = make([]float64, len(rows))
	}
	for i, r := range rows {
		data[0][i] = r.y
		for j := range ks {
			data[j+1][i] = r.x[j]
		}
	}
	for c := range data {
		demeanTwoWay(data[c], rows, func(o obs) (int, int) { return o.tk, o.t })
	}
	x := make([][]float64, len(rows))
	y := make([]float64, len(rows))
	for i := range rows {
		x[i] = make([]float64, len(ks))
		for j := range ks {
			x[i][j] = data[j+1][i]
		}
		y[i] = data[0][i]
	}
	xtx := gram(x, 0)
	for i := range xtx {
		xtx[i][i] += 1e-9
	}
	xty := make([]float64, len(ks))
	for r := range x {
		for j := range ks {
			xty[j] += x[r][j] * y[r]
		}
	}
	beta, ok := solve(xtx, xty)
	if !ok {
		return twfeResult{}, false
	}
	inv, ok := invert(xtx)
	if !ok {
		return twfeResult{}, false
	}
	scores := make(map[int][]float64)
	for r, o := range rows {
		resid := y[r] - dot(x[r], beta)
		s := scores[o.t]
		if s == nil {
			s = make([]float64, len(ks))
			scores[o.t] = s
		}
		for j := range ks {
			s[j] += x[r][j] * resid
		}
	}
	meat := make([][]float64, len(ks))
	for i := range meat {
		meat[i] = make([]float64, len(ks))
	}
	for _, s := range scores {
		for i := range ks {
			for j := range ks {
				meat[i][j] += s[i] * s[j]
			}
		}
	}
	g := float64(len(scores))
	scale := g / math.Max(g-1, 1)
	se := make([]float64, len(ks))
	for i := range ks {
		v := 0.0
		for a := range ks {
			for b := range ks {
				v += inv[i][a] * meat[a][b] * inv[b][i]
			}
		}
		se[i] = math.Sqrt(math.Max(v, 0) * scale)
	}
	return twfeResult{ks: ks, coef: beta, se: se}, true
}

func demeanTwoWay[T any](v []float64, rows []T, key func(T) (int, int)) {
	for iter := 0; iter < 60; iter++ {
		sumA := make(map[int]float64)
		cntA := make(map[int]int)
		for i, r := range rows {
			a, _ := key(r)
			sumA[a] += v[i]
			cntA[a]++
		}
		for i, r := range rows {
			a, _ := key(r)
			v[i] -= sumA[a] / float64(cntA[a])
		}
		sumB := make(map[int]float64)
		cntB := make(map[int]int)
		for i, r := range rows {
			_, b := key(r)
			sumB[b] += v[i]
			cntB[b]++
		}
		maxShift := 0.0
		for i, r := range rows {
			_, b := key(r)
			shift := sumB[b] / float64(cntB[b])
			v[i] -= shift
			maxShift = math.Max(maxShift, math.Abs(shift))
		}
		if maxShift < 1e-12 {
			break
		}
	}
}

type aipwResult struct {
	ATT      float64
	SE       float64
	NTreated int
	NControl int
	MeanProp float64
}

func (p *panel) covariates(tk, t int) []float64 {
	arPrev := 0.0
	if v := p.ar[tk][t-1]; !math.IsNaN(v) {
		arPrev = v
	}
	ar5 := 0.0
	for s := t - 5; s < t; s++ {
		if v := p.ar[tk][s]; !math.IsNaN(v) {
			ar5 += v
		}
	}
	var hist []float64
	for s := t - 20; s < t; s++ {
		if v := p.ar[tk][s]; !math.IsNaN(v) {
			hist = append(hist, v)
		}
	}
	mk := 0.0
	if v := p.mkt[t-1]; !math.IsNaN(v) {
		mk = v
	}
	news5 := 0.0
	for s := t - 5; s < t; s++ {
		if s >= 0 {
			news5 += float64(p.artCount[tk][s])
		}
	}
	return []float64{arPrev, ar5, stddev(hist), mk, math.Log1p(news5)}
}

func weekdayDummies(p *panel, t int) []float64 {
	k := p.keys[t]
	y, m, d := k/10000, (k/100)%100, k%100
	wd := weekdayOf(y, m, d)
	mon, fri := 0.0, 0.0
	if wd == 1 {
		mon = 1
	}
	if wd == 5 {
		fri = 1
	}
	return []float64{mon, fri}
}

func weekdayOf(y, m, d int) int {
	t := []int{0, 3, 2, 5, 0, 3, 5, 1, 4, 6, 2, 4}
	if m < 3 {
		y--
	}
	return (y + y/4 - y/100 + y/400 + t[m-1] + d) % 7
}

func (p *panel) aipw(td topicData, w window) (aipwResult, bool) {
	n := len(p.keys)
	isEvent := make([]map[int]bool, len(p.tickers))
	for i := range isEvent {
		isEvent[i] = make(map[int]bool)
	}
	for _, e := range td.events {
		isEvent[e.tk][e.t0] = true
	}
	type unit struct {
		t int
		d float64
		y float64
		x []float64
	}
	var units []unit
	for tk := range p.tickers {
		for t := p.studyStart; t+w.b < n; t++ {
			y, ok := p.car(tk, t, w.a, w.b)
			if !ok {
				continue
			}
			d := 0.0
			if isEvent[tk][t] {
				d = 1
			} else {
				near := false
				for s := t - 5; s <= t+5; s++ {
					if s >= 0 && s < n && td.flagged[tk][s] {
						near = true
						break
					}
				}
				if near {
					continue
				}
			}
			x := append(p.covariates(tk, t), weekdayDummies(p, t)...)
			units = append(units, unit{t: p.keys[t], d: d, y: y, x: x})
		}
	}
	nt := 0
	for _, u := range units {
		if u.d == 1 {
			nt++
		}
	}
	nc := len(units) - nt
	if nt < 5 || nc < 50 {
		return aipwResult{NTreated: nt, NControl: nc}, false
	}
	k := len(units[0].x)
	mu := make([]float64, k)
	sd := make([]float64, k)
	for j := 0; j < k; j++ {
		col := make([]float64, len(units))
		for i, u := range units {
			col[i] = u.x[j]
		}
		mu[j] = mean(col)
		sd[j] = stddev(col)
		if sd[j] < 1e-12 {
			sd[j] = 1
		}
	}
	xs := make([][]float64, len(units))
	ds := make([]float64, len(units))
	for i, u := range units {
		row := make([]float64, k+1)
		row[0] = 1
		for j := 0; j < k; j++ {
			row[j+1] = (u.x[j] - mu[j]) / sd[j]
		}
		xs[i] = row
		ds[i] = u.d
	}
	prop, ok := logisticFit(xs, ds, 1.0)
	if !ok {
		return aipwResult{NTreated: nt, NControl: nc}, false
	}
	var cx [][]float64
	var cy []float64
	for i, u := range units {
		if u.d == 0 {
			cx = append(cx, xs[i])
			cy = append(cy, u.y)
		}
	}
	out, ok := olsFit(cx, cy, 1.0)
	if !ok {
		return aipwResult{NTreated: nt, NControl: nc}, false
	}
	terms := make([]float64, len(units))
	sum := 0.0
	propSum := 0.0
	for i, u := range units {
		e := 1 / (1 + math.Exp(-dot(xs[i], prop)))
		e = math.Min(0.99, math.Max(0.01, e))
		m0 := dot(xs[i], out)
		resid := u.y - m0
		if u.d == 1 {
			terms[i] = resid
			propSum += e
		} else {
			terms[i] = -e / (1 - e) * resid
		}
		sum += terms[i]
	}
	att := sum / float64(nt)
	byDate := make(map[int]float64)
	for i, u := range units {
		contrib := terms[i]
		if u.d == 1 {
			contrib -= att
		}
		byDate[u.t] += contrib
	}
	ss := 0.0
	for _, s := range byDate {
		ss += s * s
	}
	se := math.Sqrt(ss) / float64(nt)
	return aipwResult{ATT: att, SE: se, NTreated: nt, NControl: nc, MeanProp: propSum / float64(nt)}, true
}

type marketResult struct {
	Diff    interval
	NHigh   int
	NOthers int
}

func (p *panel) marketLevel(td topicData, reps int, rng *rand.Rand) marketResult {
	n := len(p.keys)
	var counts []int
	for t := p.studyStart; t < n; t++ {
		counts = append(counts, td.rawDays[t])
	}
	sorted := append([]int(nil), counts...)
	sort.Ints(sorted)
	threshold := sorted[int(0.9*float64(len(sorted)-1))]
	if threshold < 1 {
		threshold = 1
	}
	var hi, lo []float64
	for t := p.studyStart; t+1 < n; t++ {
		a, b := p.mkt[t], p.mkt[t+1]
		if math.IsNaN(a) || math.IsNaN(b) {
			continue
		}
		if td.rawDays[t] >= threshold {
			hi = append(hi, a+b)
		} else {
			lo = append(lo, a+b)
		}
	}
	if len(hi) < 3 || len(lo) < 3 {
		return marketResult{Diff: interval{Mean: math.NaN(), Lo: math.NaN(), Hi: math.NaN()}, NHigh: len(hi), NOthers: len(lo)}
	}
	obs := mean(hi) - mean(lo)
	draws := make([]float64, 0, reps)
	for r := 0; r < reps; r++ {
		sh, sl := 0.0, 0.0
		for range hi {
			sh += hi[rng.Intn(len(hi))]
		}
		for range lo {
			sl += lo[rng.Intn(len(lo))]
		}
		draws = append(draws, sh/float64(len(hi))-sl/float64(len(lo)))
	}
	sort.Float64s(draws)
	return marketResult{
		Diff:    interval{Mean: obs, Lo: percentile(draws, 0.025), Hi: percentile(draws, 0.975), N: len(hi), SE: stddev(draws)},
		NHigh:   len(hi),
		NOthers: len(lo),
	}
}
