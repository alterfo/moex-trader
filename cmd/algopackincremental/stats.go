package main

import (
	"math"
	"math/rand"
	"sort"
)

type ranked struct {
	order []int
	tie   []int
}

func rankOrder(score []float64) ranked {
	n := len(score)
	o := make([]int, n)
	for i := range o {
		o[i] = i
	}
	sort.Slice(o, func(a, b int) bool { return score[o[a]] < score[o[b]] })
	t := make([]int, n)
	i := 0
	for i < n {
		j := i
		for j+1 < n && score[o[j+1]] == score[o[i]] {
			j++
		}
		for k := i; k <= j; k++ {
			t[k] = j
		}
		i = j + 1
	}
	return ranked{o, t}
}

func weightedAUC(rk ranked, y []bool, w []float64) float64 {
	var wp, wn float64
	for i := range y {
		if y[i] {
			wp += w[i]
		} else {
			wn += w[i]
		}
	}
	if wp == 0 || wn == 0 {
		return math.NaN()
	}
	n := len(rk.order)
	var cumNeg, sum float64
	i := 0
	for i < n {
		j := rk.tie[i]
		var gp, gn float64
		for k := i; k <= j; k++ {
			idx := rk.order[k]
			if y[idx] {
				gp += w[idx]
			} else {
				gn += w[idx]
			}
		}
		sum += gp * (cumNeg + 0.5*gn)
		cumNeg += gn
		i = j + 1
	}
	return sum / (wp * wn)
}

func mean(v []float64) float64 {
	s := 0.0
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}

func neweyWest(x []float64, lags int) (float64, float64) {
	n := len(x)
	if n < lags+3 {
		return math.NaN(), math.NaN()
	}
	m := mean(x)
	var g0 float64
	for _, v := range x {
		g0 += (v - m) * (v - m)
	}
	g0 /= float64(n)
	v := g0
	for l := 1; l <= lags; l++ {
		var g float64
		for i := l; i < n; i++ {
			g += (x[i] - m) * (x[i-l] - m)
		}
		g /= float64(n)
		v += 2 * (1 - float64(l)/float64(lags+1)) * g
	}
	se := math.Sqrt(v / float64(n))
	return m, m / se
}

func spearman(a, b []float64) float64 {
	n := len(a)
	ra, rb := avgRank(a), avgRank(b)
	ma, mb := mean(ra), mean(rb)
	var sab, saa, sbb float64
	for i := 0; i < n; i++ {
		sab += (ra[i] - ma) * (rb[i] - mb)
		saa += (ra[i] - ma) * (ra[i] - ma)
		sbb += (rb[i] - mb) * (rb[i] - mb)
	}
	if saa == 0 || sbb == 0 {
		return math.NaN()
	}
	return sab / math.Sqrt(saa*sbb)
}

func avgRank(v []float64) []float64 {
	n := len(v)
	o := make([]int, n)
	for i := range o {
		o[i] = i
	}
	sort.Slice(o, func(a, b int) bool { return v[o[a]] < v[o[b]] })
	r := make([]float64, n)
	i := 0
	for i < n {
		j := i
		for j+1 < n && v[o[j+1]] == v[o[i]] {
			j++
		}
		for k := i; k <= j; k++ {
			r[o[k]] = float64(i+j)/2 + 1
		}
		i = j + 1
	}
	return r
}

func blockWeights(dateOf []int, nDates, block int, rng *rand.Rand) []float64 {
	cnt := make([]float64, nDates)
	need := (nDates + block - 1) / block
	for b := 0; b < need; b++ {
		s := rng.Intn(nDates - block + 1)
		for k := 0; k < block; k++ {
			cnt[s+k]++
		}
	}
	w := make([]float64, len(dateOf))
	for i, d := range dateOf {
		w[i] = cnt[d]
	}
	return w
}

func percentile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return math.NaN()
	}
	pos := q * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	return sorted[lo] + (sorted[hi]-sorted[lo])*(pos-float64(lo))
}
