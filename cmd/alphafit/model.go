package main

import (
	"math"
)

type gram struct {
	p   int
	g   []float64
	xty []float64
	n   float64
}

func newGram(p int) *gram { return &gram{p: p, g: make([]float64, p*p), xty: make([]float64, p)} }

func (m *gram) addRow(x []float32, y float64) {
	p := m.p
	for i := 0; i < p; i++ {
		xi := float64(x[i])
		if xi == 0 {
			continue
		}
		m.xty[i] += xi * y
		row := m.g[i*p : (i+1)*p]
		for j := i; j < p; j++ {
			row[j] += xi * float64(x[j])
		}
	}
	m.n++
}

func (m *gram) merge(o *gram) {
	for i := range m.g {
		m.g[i] += o.g[i]
	}
	for i := range m.xty {
		m.xty[i] += o.xty[i]
	}
	m.n += o.n
}

func (m *gram) sym() []float64 {
	p := m.p
	out := make([]float64, p*p)
	copy(out, m.g)
	for i := 0; i < p; i++ {
		for j := 0; j < i; j++ {
			out[i*p+j] = out[j*p+i]
		}
	}
	return out
}

func solveRidge(m *gram, lambda float64, active []int) []float64 {
	p := m.p
	G := m.sym()
	k := len(active)
	A := make([][]float64, k)
	for a := range A {
		A[a] = make([]float64, k+1)
		for b := range active {
			A[a][b] = G[active[a]*p+active[b]]
		}
		A[a][a] += lambda * m.n
		A[a][k] = m.xty[active[a]]
	}
	for c := 0; c < k; c++ {
		piv := c
		for r := c + 1; r < k; r++ {
			if math.Abs(A[r][c]) > math.Abs(A[piv][c]) {
				piv = r
			}
		}
		A[c], A[piv] = A[piv], A[c]
		d := A[c][c]
		if math.Abs(d) < 1e-12 {
			continue
		}
		for j := c; j <= k; j++ {
			A[c][j] /= d
		}
		for r := 0; r < k; r++ {
			if r == c {
				continue
			}
			f := A[r][c]
			if f == 0 {
				continue
			}
			for j := c; j <= k; j++ {
				A[r][j] -= f * A[c][j]
			}
		}
	}
	beta := make([]float64, p)
	for a := range active {
		beta[active[a]] = A[a][k]
	}
	return beta
}

func solveEN(m *gram, lambda, alpha float64) []float64 {
	p := m.p
	G := m.sym()
	n := m.n
	beta := make([]float64, p)
	c := make([]float64, p)
	for i := 0; i < p; i++ {
		c[i] = m.xty[i] / n
	}
	diag := make([]float64, p)
	for i := 0; i < p; i++ {
		diag[i] = G[i*p+i] / n
	}
	for it := 0; it < 500; it++ {
		maxd := 0.0
		for i := 0; i < p; i++ {
			if diag[i] == 0 {
				continue
			}
			r := c[i]
			row := G[i*p : (i+1)*p]
			for j := 0; j < p; j++ {
				if j != i && beta[j] != 0 {
					r -= row[j] / n * beta[j]
				}
			}
			var nb float64
			switch {
			case r > lambda*alpha:
				nb = (r - lambda*alpha) / (diag[i] + lambda*(1-alpha))
			case r < -lambda*alpha:
				nb = (r + lambda*alpha) / (diag[i] + lambda*(1-alpha))
			}
			if d := math.Abs(nb - beta[i]); d > maxd {
				maxd = d
			}
			beta[i] = nb
		}
		if maxd < 1e-7 {
			break
		}
	}
	return beta
}

func predict(beta []float64, x []float32) float64 {
	s := 0.0
	for i, b := range beta {
		if b != 0 {
			s += b * float64(x[i])
		}
	}
	return s
}
