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

func stddev(v []float64) float64 {
	if len(v) < 2 {
		return 0
	}
	m := mean(v)
	s := 0.0
	for _, x := range v {
		s += (x - m) * (x - m)
	}
	return math.Sqrt(s / float64(len(v)-1))
}

func percentile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return math.NaN()
	}
	pos := q * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return sorted[lo]
	}
	frac := pos - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

func normalTwoSidedP(z float64) float64 {
	return math.Erfc(math.Abs(z) / math.Sqrt2)
}

func solve(a [][]float64, b []float64) ([]float64, bool) {
	n := len(b)
	m := make([][]float64, n)
	for i := range m {
		m[i] = make([]float64, n+1)
		copy(m[i], a[i])
		m[i][n] = b[i]
	}
	for c := 0; c < n; c++ {
		p := c
		for r := c + 1; r < n; r++ {
			if math.Abs(m[r][c]) > math.Abs(m[p][c]) {
				p = r
			}
		}
		if math.Abs(m[p][c]) < 1e-12 {
			return nil, false
		}
		m[c], m[p] = m[p], m[c]
		for r := 0; r < n; r++ {
			if r == c {
				continue
			}
			f := m[r][c] / m[c][c]
			for k := c; k <= n; k++ {
				m[r][k] -= f * m[c][k]
			}
		}
	}
	x := make([]float64, n)
	for i := range x {
		x[i] = m[i][n] / m[i][i]
	}
	return x, true
}

func invert(a [][]float64) ([][]float64, bool) {
	n := len(a)
	inv := make([][]float64, n)
	for i := range inv {
		inv[i] = make([]float64, n)
	}
	for j := 0; j < n; j++ {
		e := make([]float64, n)
		e[j] = 1
		col, ok := solve(a, e)
		if !ok {
			return nil, false
		}
		for i := 0; i < n; i++ {
			inv[i][j] = col[i]
		}
	}
	return inv, true
}

func gram(x [][]float64, ridge float64) [][]float64 {
	k := len(x[0])
	g := make([][]float64, k)
	for i := range g {
		g[i] = make([]float64, k)
	}
	for _, row := range x {
		for i := 0; i < k; i++ {
			for j := 0; j < k; j++ {
				g[i][j] += row[i] * row[j]
			}
		}
	}
	for i := 1; i < k; i++ {
		g[i][i] += ridge
	}
	return g
}

func olsFit(x [][]float64, y []float64, ridge float64) ([]float64, bool) {
	if len(x) == 0 {
		return nil, false
	}
	k := len(x[0])
	xty := make([]float64, k)
	for r, row := range x {
		for i := 0; i < k; i++ {
			xty[i] += row[i] * y[r]
		}
	}
	return solve(gram(x, ridge), xty)
}

func dot(a, b []float64) float64 {
	s := 0.0
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}

func logisticFit(x [][]float64, d []float64, ridge float64) ([]float64, bool) {
	k := len(x[0])
	beta := make([]float64, k)
	for iter := 0; iter < 40; iter++ {
		g := make([][]float64, k)
		for i := range g {
			g[i] = make([]float64, k)
		}
		grad := make([]float64, k)
		for r, row := range x {
			p := 1 / (1 + math.Exp(-dot(row, beta)))
			w := p * (1 - p)
			if w < 1e-9 {
				w = 1e-9
			}
			for i := 0; i < k; i++ {
				grad[i] += row[i] * (d[r] - p)
				for j := 0; j < k; j++ {
					g[i][j] += w * row[i] * row[j]
				}
			}
		}
		for i := 1; i < k; i++ {
			g[i][i] += ridge
			grad[i] -= ridge * beta[i]
		}
		step, ok := solve(g, grad)
		if !ok {
			return nil, false
		}
		maxStep := 0.0
		for i := range beta {
			beta[i] += step[i]
			maxStep = math.Max(maxStep, math.Abs(step[i]))
		}
		if maxStep < 1e-7 {
			break
		}
	}
	return beta, true
}

type interval struct {
	Mean float64
	Lo   float64
	Hi   float64
	N    int
	SE   float64
}

func (iv interval) excludesZero() bool {
	return !math.IsNaN(iv.Lo) && (iv.Lo > 0 || iv.Hi < 0)
}

func dateBootstrap(vals []float64, keys []int, reps int, rng *rand.Rand) interval {
	if len(vals) == 0 {
		return interval{Mean: math.NaN(), Lo: math.NaN(), Hi: math.NaN()}
	}
	groups := make(map[int][]float64)
	for i, v := range vals {
		groups[keys[i]] = append(groups[keys[i]], v)
	}
	gl := make([][]float64, 0, len(groups))
	ks := make([]int, 0, len(groups))
	for k := range groups {
		ks = append(ks, k)
	}
	sort.Ints(ks)
	for _, k := range ks {
		gl = append(gl, groups[k])
	}
	draws := make([]float64, 0, reps)
	for r := 0; r < reps; r++ {
		sum, cnt := 0.0, 0
		for range gl {
			g := gl[rng.Intn(len(gl))]
			for _, v := range g {
				sum += v
			}
			cnt += len(g)
		}
		draws = append(draws, sum/float64(cnt))
	}
	sort.Float64s(draws)
	return interval{
		Mean: mean(vals),
		Lo:   percentile(draws, 0.025),
		Hi:   percentile(draws, 0.975),
		N:    len(vals),
		SE:   stddev(draws),
	}
}

func holm(ps []float64) []float64 {
	m := len(ps)
	idx := make([]int, m)
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return ps[idx[a]] < ps[idx[b]] })
	out := make([]float64, m)
	running := 0.0
	for rank, i := range idx {
		adj := math.Min(1, float64(m-rank)*ps[i])
		if adj < running {
			adj = running
		}
		running = adj
		out[i] = adj
	}
	return out
}
