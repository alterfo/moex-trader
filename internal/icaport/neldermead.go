package icaport

import (
	"math"
	"sort"
)

func NelderMead(f func([]float64) float64, start []float64, step float64, maxIter int) []float64 {
	n := len(start)
	simplex := make([][]float64, n+1)
	values := make([]float64, n+1)
	simplex[0] = append([]float64(nil), start...)
	for i := 1; i <= n; i++ {
		p := append([]float64(nil), start...)
		s := step
		if p[i-1] != 0 {
			s = step * math.Abs(p[i-1])
			if s < step*0.1 {
				s = step * 0.1
			}
		}
		p[i-1] += s
		simplex[i] = p
	}
	for i := range simplex {
		values[i] = f(simplex[i])
	}
	order := func() {
		idx := make([]int, n+1)
		for i := range idx {
			idx[i] = i
		}
		sort.SliceStable(idx, func(a, b int) bool { return values[idx[a]] < values[idx[b]] })
		ns := make([][]float64, n+1)
		nv := make([]float64, n+1)
		for i, k := range idx {
			ns[i] = simplex[k]
			nv[i] = values[k]
		}
		simplex, values = ns, nv
	}
	for iter := 0; iter < maxIter; iter++ {
		order()
		if math.Abs(values[n]-values[0]) < 1e-12*(1+math.Abs(values[0])) {
			break
		}
		centroid := make([]float64, n)
		for i := 0; i < n; i++ {
			for j := range centroid {
				centroid[j] += simplex[i][j] / float64(n)
			}
		}
		point := func(coef float64) []float64 {
			p := make([]float64, n)
			for j := range p {
				p[j] = centroid[j] + coef*(simplex[n][j]-centroid[j])
			}
			return p
		}
		reflected := point(-1)
		fr := f(reflected)
		switch {
		case fr < values[0]:
			expanded := point(-2)
			if fe := f(expanded); fe < fr {
				simplex[n], values[n] = expanded, fe
			} else {
				simplex[n], values[n] = reflected, fr
			}
		case fr < values[n-1]:
			simplex[n], values[n] = reflected, fr
		default:
			var cand []float64
			if fr < values[n] {
				cand = point(-0.5)
			} else {
				cand = point(0.5)
			}
			if fc := f(cand); fc < math.Min(fr, values[n]) {
				simplex[n], values[n] = cand, fc
			} else {
				for i := 1; i <= n; i++ {
					for j := range simplex[i] {
						simplex[i][j] = simplex[0][j] + 0.5*(simplex[i][j]-simplex[0][j])
					}
					values[i] = f(simplex[i])
				}
			}
		}
	}
	order()
	return simplex[0]
}
