package main

import (
	"math"
	"math/rand"
	"sort"
)

type point struct {
	k, t int
	x    []float64
	y    float64
}

func solve(a [][]float64, b []float64) []float64 {
	n := len(b)
	m := make([][]float64, n)
	for i := range m {
		m[i] = append(append([]float64{}, a[i]...), b[i])
	}
	for c := 0; c < n; c++ {
		p := c
		for r := c + 1; r < n; r++ {
			if math.Abs(m[r][c]) > math.Abs(m[p][c]) {
				p = r
			}
		}
		m[c], m[p] = m[p], m[c]
		if math.Abs(m[c][c]) < 1e-14 {
			return nil
		}
		for r := 0; r < n; r++ {
			if r == c {
				continue
			}
			f := m[r][c] / m[c][c]
			for q := c; q <= n; q++ {
				m[r][q] -= f * m[c][q]
			}
		}
	}
	out := make([]float64, n)
	for i := range out {
		out[i] = m[i][n] / m[i][i]
	}
	return out
}

func invert(a [][]float64) [][]float64 {
	n := len(a)
	out := make([][]float64, n)
	for j := 0; j < n; j++ {
		e := make([]float64, n)
		e[j] = 1
		col := solve(a, e)
		if col == nil {
			return nil
		}
		for i := 0; i < n; i++ {
			if out[i] == nil {
				out[i] = make([]float64, n)
			}
			out[i][j] = col[i]
		}
	}
	return out
}

type fit struct {
	coef, t []float64
	n       int
}

func regress(pts []point, nT, lags int) fit {
	if len(pts) < 30 {
		return fit{}
	}
	k := len(pts[0].x)
	sumX := map[int][]float64{}
	cnt := map[int]int{}
	sumY := map[int]float64{}
	for _, p := range pts {
		if sumX[p.k] == nil {
			sumX[p.k] = make([]float64, k)
		}
		for j := range p.x {
			sumX[p.k][j] += p.x[j]
		}
		sumY[p.k] += p.y
		cnt[p.k]++
	}
	xx := make([][]float64, k)
	for i := range xx {
		xx[i] = make([]float64, k)
	}
	xy := make([]float64, k)
	dx := make([][]float64, len(pts))
	dy := make([]float64, len(pts))
	for i, p := range pts {
		d := make([]float64, k)
		for j := range d {
			d[j] = p.x[j] - sumX[p.k][j]/float64(cnt[p.k])
		}
		dx[i] = d
		dy[i] = p.y - sumY[p.k]/float64(cnt[p.k])
		for a := 0; a < k; a++ {
			xy[a] += d[a] * dy[i]
			for b := 0; b < k; b++ {
				xx[a][b] += d[a] * d[b]
			}
		}
	}
	coef := solve(xx, xy)
	inv := invert(xx)
	if coef == nil || inv == nil {
		return fit{}
	}
	g := make([][]float64, nT)
	for t := range g {
		g[t] = make([]float64, k)
	}
	for i, p := range pts {
		e := dy[i]
		for a := 0; a < k; a++ {
			e -= coef[a] * dx[i][a]
		}
		for a := 0; a < k; a++ {
			g[p.t][a] += dx[i][a] * e
		}
	}
	meat := make([][]float64, k)
	for a := range meat {
		meat[a] = make([]float64, k)
	}
	for l := 0; l <= lags; l++ {
		w := 1.0
		if l > 0 {
			w = 2 * (1 - float64(l)/float64(lags+1))
		}
		for t := l; t < nT; t++ {
			for a := 0; a < k; a++ {
				for b := 0; b < k; b++ {
					meat[a][b] += w * g[t][a] * g[t-l][b]
				}
			}
		}
	}
	t := make([]float64, k)
	for a := 0; a < k; a++ {
		v := 0.0
		for b := 0; b < k; b++ {
			for c := 0; c < k; c++ {
				v += inv[a][b] * meat[b][c] * inv[c][a]
			}
		}
		if v > 0 {
			t[a] = coef[a] / math.Sqrt(v)
		} else {
			t[a] = nan
		}
	}
	return fit{coef: coef, t: t, n: len(pts)}
}

func percentile(a []float64, q float64) float64 {
	c := append([]float64(nil), a...)
	sort.Float64s(c)
	return c[int(q*float64(len(c)-1))]
}

func bootMean(x []float64, draws int, seed int64) (float64, float64) {
	if len(x) < 10 {
		return nan, nan
	}
	rng := rand.New(rand.NewSource(seed))
	ms := make([]float64, draws)
	for d := range ms {
		var s float64
		for i := 0; i < len(x); i++ {
			s += x[rng.Intn(len(x))]
		}
		ms[d] = s / float64(len(x))
	}
	return percentile(ms, 0.025), percentile(ms, 0.975)
}
