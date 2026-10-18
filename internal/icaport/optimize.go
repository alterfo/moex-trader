package icaport

import (
	"math"
	"math/rand"
	"sort"
)

func ProjectSimplex(v []float64) []float64 {
	n := len(v)
	u := append([]float64(nil), v...)
	sort.Sort(sort.Reverse(sort.Float64Slice(u)))
	var cum float64
	var theta float64
	for i := 0; i < n; i++ {
		cum += u[i]
		t := (cum - 1) / float64(i+1)
		if u[i]-t > 0 {
			theta = t
		}
	}
	out := make([]float64, n)
	for i := range v {
		out[i] = math.Max(v[i]-theta, 0)
	}
	return out
}

func MinimizeSimplex(obj Objective, starts [][]float64, maxIter int) []float64 {
	var best []float64
	bestValue := math.Inf(1)
	for _, start := range starts {
		w := ProjectSimplex(start)
		step := 1.0
		for iter := 0; iter < maxIter; iter++ {
			f := obj.Value(w)
			g := obj.Gradient(w)
			var next []float64
			accepted := false
			for try := 0; try < 40; try++ {
				cand := make([]float64, len(w))
				for i := range w {
					cand[i] = w[i] - step*g[i]
				}
				cand = ProjectSimplex(cand)
				var lin, sq float64
				for i := range w {
					d := cand[i] - w[i]
					lin += g[i] * d
					sq += d * d
				}
				if obj.Value(cand) <= f+lin+sq/(2*step)+1e-14 {
					next = cand
					accepted = true
					break
				}
				step /= 2
			}
			if !accepted {
				break
			}
			var move float64
			for i := range w {
				move += math.Abs(next[i] - w[i])
			}
			w = next
			step = math.Min(step*1.5, 100)
			if move < 1e-10 {
				break
			}
		}
		if v := obj.Value(w); v < bestValue {
			bestValue = v
			best = w
		}
	}
	return best
}

func DefaultStarts(n int, seed int64, random int, extra ...[]float64) [][]float64 {
	equal := make([]float64, n)
	for i := range equal {
		equal[i] = 1 / float64(n)
	}
	starts := [][]float64{equal}
	starts = append(starts, extra...)
	rng := rand.New(rand.NewSource(seed))
	for k := 0; k < random; k++ {
		v := make([]float64, n)
		var s float64
		for i := range v {
			v[i] = -math.Log(1 - rng.Float64())
			s += v[i]
		}
		for i := range v {
			v[i] /= s
		}
		starts = append(starts, v)
	}
	return starts
}
