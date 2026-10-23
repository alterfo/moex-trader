package main

import (
	"math"
	"sort"
)

func mean(v []float64) float64 {
	s := 0.0
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}

func stdOf(v []float64) float64 {
	if len(v) < 2 {
		return nan
	}
	m := mean(v)
	s := 0.0
	for _, x := range v {
		s += (x - m) * (x - m)
	}
	return math.Sqrt(s / float64(len(v)-1))
}

func neweyWest(x []float64, lags int) (float64, float64) {
	n := len(x)
	if n < lags+3 {
		return mean(x), nan
	}
	m := mean(x)
	g0 := 0.0
	for _, v := range x {
		g0 += (v - m) * (v - m)
	}
	g0 /= float64(n)
	variance := g0
	for l := 1; l <= lags; l++ {
		g := 0.0
		for i := l; i < n; i++ {
			g += (x[i] - m) * (x[i-l] - m)
		}
		g /= float64(n)
		variance += 2 * (1 - float64(l)/float64(lags+1)) * g
	}
	if variance <= 0 {
		return m, nan
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
			for j := 0; j < k; j++ {
				a[i][j] += x[r][i] * x[r][j]
			}
			a[i][k] += x[r][i] * y[r]
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
	b := make([]float64, k)
	for i := range b {
		b[i] = a[i][k] / a[i][i]
	}
	return b, true
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

func normCDF(x float64) float64 { return 0.5 * math.Erfc(-x/math.Sqrt2) }

func normP2(t float64) float64 { return math.Erfc(math.Abs(t) / math.Sqrt2) }

func normQuantile(alpha float64) float64 {
	lo, hi := 0.0, 10.0
	for k := 0; k < 100; k++ {
		mid := (lo + hi) / 2
		if math.Erfc(mid/math.Sqrt2) > alpha {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2
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
		w := 1.0
		if l > 0 {
			w = 1 - float64(l)/float64(lags+1)
		}
		for t := l; t < n; t++ {
			for i := 0; i < k; i++ {
				for j := 0; j < k; j++ {
					v := u[t][i] * u[t-l][j] * w
					S[i][j] += v
					if l > 0 {
						S[j][i] += v
					}
				}
			}
		}
	}
	V00 := 0.0
	for a := 0; a < k; a++ {
		for c := 0; c < k; c++ {
			V00 += inv[1][a] * S[a][c] * inv[c][1]
		}
	}
	se := math.Sqrt(V00)
	if !(se > 0) {
		return b[1], 0, false
	}
	return b[1], b[1] / se, true
}

func lagsFor(h int) int {
	if h > 5 {
		return h
	}
	return 5
}
