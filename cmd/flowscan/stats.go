package main

import (
	"math"

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
