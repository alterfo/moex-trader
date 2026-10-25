package main

import (
	"fmt"
	"math"
	"strings"
)

type result struct {
	id             string
	coef, t, p     float64
	tPre, tRt      float64
	ok             bool
	signal         map[int]float64
	sign           float64
	price, twoSide bool
}

func pairCorr(a, b map[int]float64) float64 {
	var xs, ys []float64
	for k, v := range a {
		if w, ok := b[k]; ok {
			xs = append(xs, v)
			ys = append(ys, w)
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

func jacobi(a [][]float64) []float64 {
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

func nyholt(sigs []map[int]float64) float64 {
	m := len(sigs)
	c := make([][]float64, m)
	for i := range c {
		c[i] = make([]float64, m)
		for j := range c[i] {
			if i == j {
				c[i][j] = 1
			} else {
				c[i][j] = pairCorr(sigs[i], sigs[j])
			}
		}
	}
	e := jacobi(c)
	mu := mean(e)
	var v float64
	for _, x := range e {
		v += (x - mu) * (x - mu)
	}
	v /= float64(m - 1)
	return 1 + float64(m-1)*(1-v/float64(m))
}

func verdict(rs []result) {
	var fam []result
	var sigs []map[int]float64
	for _, r := range rs {
		if r.ok && (strings.HasPrefix(r.id, "PR") || strings.HasPrefix(r.id, "PM") || strings.HasPrefix(r.id, "PF") || strings.HasPrefix(r.id, "PX")) {
			fam = append(fam, r)
			sigs = append(sigs, r.signal)
		}
	}
	meff := nyholt(sigs)
	thr := 0.05 / meff
	fmt.Printf("\nfamily: %d rows, Nyholt M_eff %.2f, permutation threshold p <= %.4f\n", len(fam), meff, thr)
	hits := 0
	for _, r := range fam {
		signOK := r.twoSide || r.coef*r.sign > 0
		halves := true
		if r.price {
			halves = r.tPre*r.coef > 0 && r.tRt*r.coef > 0
		}
		hit := r.p <= thr && signOK && halves
		if hit {
			hits++
		}
		fmt.Printf("  %-5s p %.4f sign-ok %v halves-ok %v %s\n", r.id, r.p, signOK, halves, map[bool]string{true: "HIT", false: ""}[hit])
	}
	fmt.Printf("hits: %d of %d\n", hits, len(fam))
}
