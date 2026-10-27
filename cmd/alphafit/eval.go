package main

import (
	"math"
	"math/rand"
	"sort"
)

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

func pearson(a, b []float64) float64 {
	ma, mb := mean(a), mean(b)
	var sab, saa, sbb float64
	for i := range a {
		sab += (a[i] - ma) * (b[i] - mb)
		saa += (a[i] - ma) * (a[i] - ma)
		sbb += (b[i] - mb) * (b[i] - mb)
	}
	if saa == 0 || sbb == 0 {
		return nan
	}
	return sab / math.Sqrt(saa*sbb)
}

func percentile(sorted []float64, q float64) float64 {
	pos := q * float64(len(sorted)-1)
	lo, hi := int(math.Floor(pos)), int(math.Ceil(pos))
	return sorted[lo] + (sorted[hi]-sorted[lo])*(pos-float64(lo))
}

func validIdx(series ...[]float64) []int {
	n := len(series[0])
	var out []int
	for i := 0; i < n; i++ {
		ok := true
		for _, s := range series {
			if math.IsNaN(s[i]) {
				ok = false
			}
		}
		if ok {
			out = append(out, i)
		}
	}
	return out
}

func bootCI(x []float64, draws int, seed int64) (float64, float64) {
	idx := validIdx(x)
	n := len(idx)
	block := bootBlock()
	if n < 2*block {
		return nan, nan
	}
	rng := rand.New(rand.NewSource(seed))
	ms := make([]float64, 0, draws)
	for b := 0; b < draws; b++ {
		s, c := 0.0, 0
		for q := 0; q < (n+block-1)/block; q++ {
			st := rng.Intn(n - block + 1)
			for k := 0; k < block; k++ {
				s += x[idx[st+k]]
				c++
			}
		}
		ms = append(ms, s/float64(c))
	}
	sort.Float64s(ms)
	return percentile(ms, 0.025), percentile(ms, 0.975)
}

func clean(x []float64) []float64 {
	var o []float64
	for _, v := range x {
		if !math.IsNaN(v) {
			o = append(o, v)
		}
	}
	return o
}

func halves(x []float64, split int) (float64, float64) {
	var a, b []float64
	for i, v := range x {
		if math.IsNaN(v) {
			continue
		}
		if i < split {
			a = append(a, v)
		} else {
			b = append(b, v)
		}
	}
	return mean(a), mean(b)
}

func diff(a, b []float64) []float64 {
	o := make([]float64, len(a))
	for i := range o {
		o[i] = nan
		if !math.IsNaN(a[i]) && !math.IsNaN(b[i]) {
			o[i] = a[i] - b[i]
		}
	}
	return o
}

type book struct {
	prev map[string]float64
	rate float64
}

func newBook(rate float64) *book { return &book{prev: map[string]float64{}, rate: rate} }

func aimWeights(rows []*row, pred []float64) map[string]float64 {
	var idx []int
	for i := range rows {
		if !math.IsNaN(pred[i]) {
			idx = append(idx, i)
		}
	}
	n := len(idx)
	out := map[string]float64{}
	if n < minAim {
		return out
	}
	sort.SliceStable(idx, func(a, b int) bool { return pred[idx[a]] > pred[idx[b]] })
	q := n / 5
	leg := func(sel []int, sign float64) {
		sum := 0.0
		w := make([]float64, len(sel))
		for k, i := range sel {
			s := rows[i].sig
			if equalWeight || math.IsNaN(s) || s <= 0 {
				w[k] = 1
			} else {
				w[k] = 1 / s
			}
			sum += w[k]
		}
		for k, i := range sel {
			out[rows[i].secid] = sign * w[k] / sum
		}
	}
	leg(idx[:q], 1)
	if !longOnly {
		leg(idx[n-q:], -1)
	}
	return out
}

type dayPL struct{ gross, cost, short, glong, gshort, turn, wlong float64 }

func (b *book) step(rows []*row, aim map[string]float64) dayPL {
	next := map[string]float64{}
	for k, v := range b.prev {
		next[k] = (1 - b.rate) * v
	}
	for k, v := range aim {
		next[k] += b.rate * v
	}
	var pl dayPL
	turn := 0.0
	seen := map[string]bool{}
	costOf := map[string]float64{}
	for _, r := range rows {
		seen[r.secid] = true
		costOf[r.secid] = r.sideCost()
		w := next[r.secid]
		if !math.IsNaN(r.f1) {
			contribution := w * r.f1
			if entryMode == "open" && !math.IsNaN(r.idy1) {
				contribution = b.prev[r.secid]*r.f1 + (w-b.prev[r.secid])*r.idy1
			}
			pl.gross += contribution
			if w > 0 {
				pl.glong += contribution
			} else {
				pl.gshort += contribution
			}
		}
	}
	if delistDay != nil && len(rows) > 0 {
		d := rows[0].di
		for k, w := range next {
			if dd, ok := delistDay[k]; ok && dd <= d && w != 0 {
				if w > 0 {
					pl.gross -= w
					pl.glong -= w
					delistHits++
				}
				next[k] = 0
			}
		}
	}
	costSum := 0.0
	costFor := func(k string) float64 {
		if c, ok := costOf[k]; ok {
			return c
		}
		return 0.0015
	}
	for _, w := range next {
		if w > 0 {
			pl.wlong += w
		}
	}
	for k, w := range next {
		turn += math.Abs(w - b.prev[k])
		costSum += math.Abs(w-b.prev[k]) * costFor(k)
		if w < 0 {
			pl.short += -w
		}
	}
	for k, w := range b.prev {
		if _, ok := next[k]; !ok {
			turn += math.Abs(w)
			costSum += math.Abs(w) * costFor(k)
		}
	}
	pl.turn = turn
	pl.cost = costSum
	b.prev = next
	for k, w := range b.prev {
		if math.Abs(w) < 1e-9 {
			delete(b.prev, k)
		}
	}
	return pl
}
