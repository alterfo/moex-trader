package main

import (
	"math"
	"math/rand"
	"sort"
)

func mean(v []float64) float64 {
	if len(v) == 0 {
		return math.NaN()
	}
	s := 0.0
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}

func neweyWest(x []float64, lags int) (float64, float64) {
	n := len(x)
	if n < lags+3 {
		return mean(x), math.NaN()
	}
	m := mean(x)
	gamma0 := 0.0
	for _, v := range x {
		gamma0 += (v - m) * (v - m)
	}
	gamma0 /= float64(n)
	variance := gamma0
	for l := 1; l <= lags; l++ {
		g := 0.0
		for i := l; i < n; i++ {
			g += (x[i] - m) * (x[i-l] - m)
		}
		g /= float64(n)
		variance += 2 * (1 - float64(l)/float64(lags+1)) * g
	}
	if variance <= 0 {
		return m, math.NaN()
	}
	return m, m / math.Sqrt(variance/float64(n))
}

func ols(x [][]float64, y []float64) ([]float64, bool) {
	n := len(y)
	if n == 0 {
		return nil, false
	}
	k := len(x[0])
	if n <= k {
		return nil, false
	}
	a := make([][]float64, k)
	for i := range a {
		a[i] = make([]float64, k+1)
	}
	for r := 0; r < n; r++ {
		for i := 0; i < k; i++ {
			xi := x[r][i]
			for j := 0; j < k; j++ {
				a[i][j] += xi * x[r][j]
			}
			a[i][k] += xi * y[r]
		}
	}
	for i := 0; i < k; i++ {
		a[i][i] += 1e-9
	}
	for col := 0; col < k; col++ {
		piv := col
		for r := col + 1; r < k; r++ {
			if math.Abs(a[r][col]) > math.Abs(a[piv][col]) {
				piv = r
			}
		}
		if math.Abs(a[piv][col]) < 1e-14 {
			return nil, false
		}
		a[col], a[piv] = a[piv], a[col]
		for r := 0; r < k; r++ {
			if r == col {
				continue
			}
			f := a[r][col] / a[col][col]
			for c := col; c <= k; c++ {
				a[r][c] -= f * a[col][c]
			}
		}
	}
	beta := make([]float64, k)
	for i := range beta {
		beta[i] = a[i][k] / a[i][i]
	}
	return beta, true
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
		r := float64(i+j)/2 + 0.5
		for k := i; k <= j; k++ {
			out[idx[k]] = r/float64(n) - 0.5
		}
		i = j + 1
	}
	return out
}

func quantile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return math.NaN()
	}
	pos := q * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	return sorted[lo] + (sorted[hi]-sorted[lo])*(pos-float64(lo))
}

type sample struct {
	day int
	val float64
}

func groupByDay(s []sample) map[int][]float64 {
	g := map[int][]float64{}
	for _, e := range s {
		g[e.day] = append(g[e.day], e.val)
	}
	return g
}

func bootstrapDiff(top, bottom []sample, reps int, seed int64, lo, hi float64) (float64, float64, float64) {
	gt, gb := groupByDay(top), groupByDay(bottom)
	daySet := map[int]bool{}
	for d := range gt {
		daySet[d] = true
	}
	for d := range gb {
		daySet[d] = true
	}
	days := make([]int, 0, len(daySet))
	for d := range daySet {
		days = append(days, d)
	}
	sort.Ints(days)
	type agg struct{ st, nt, sb, nb float64 }
	per := make([]agg, len(days))
	for i, d := range days {
		for _, v := range gt[d] {
			per[i].st += v
			per[i].nt++
		}
		for _, v := range gb[d] {
			per[i].sb += v
			per[i].nb++
		}
	}
	diffOf := func(pick func(i int) agg, n int) float64 {
		var st, nt, sb, nb float64
		for i := 0; i < n; i++ {
			a := pick(i)
			st += a.st
			nt += a.nt
			sb += a.sb
			nb += a.nb
		}
		if nt == 0 || nb == 0 {
			return math.NaN()
		}
		return st/nt - sb/nb
	}
	point := diffOf(func(i int) agg { return per[i] }, len(per))
	rng := rand.New(rand.NewSource(seed))
	draws := make([]float64, 0, reps)
	for r := 0; r < reps; r++ {
		pickIdx := make([]int, len(per))
		for i := range pickIdx {
			pickIdx[i] = rng.Intn(len(per))
		}
		d := diffOf(func(i int) agg { return per[pickIdx[i]] }, len(per))
		if !math.IsNaN(d) {
			draws = append(draws, d)
		}
	}
	sort.Float64s(draws)
	return point, quantile(draws, lo), quantile(draws, hi)
}

func tercileCuts(v []float64) (float64, float64) {
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	return quantile(s, 1.0/3), quantile(s, 2.0/3)
}
