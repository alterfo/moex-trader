package newsstudy

import (
	"math"
	"math/rand"
	"sort"
)

type Obs struct {
	Date  string
	Score float64
	Label float64
	Aux   float64
}

func AUC(rows []Obs) float64 {
	n := len(rows)
	if n == 0 {
		return math.NaN()
	}
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return rows[idx[a]].Score < rows[idx[b]].Score })
	ranks := make([]float64, n)
	for i := 0; i < n; {
		j := i
		for j+1 < n && rows[idx[j+1]].Score == rows[idx[i]].Score {
			j++
		}
		mid := float64(i+j)/2 + 1
		for k := i; k <= j; k++ {
			ranks[idx[k]] = mid
		}
		i = j + 1
	}
	var pos, neg, sum float64
	for i, r := range rows {
		if r.Label > 0 {
			pos++
			sum += ranks[i]
		} else {
			neg++
		}
	}
	if pos == 0 || neg == 0 {
		return math.NaN()
	}
	return (sum - pos*(pos+1)/2) / (pos * neg)
}

func ByDate(rows []Obs) ([]string, map[string][]Obs) {
	groups := make(map[string][]Obs)
	for _, r := range rows {
		groups[r.Date] = append(groups[r.Date], r)
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys, groups
}

func Resample(keys []string, groups map[string][]Obs, rng *rand.Rand) []Obs {
	var out []Obs
	for range keys {
		out = append(out, groups[keys[rng.Intn(len(keys))]]...)
	}
	return out
}

func Quantile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return math.NaN()
	}
	pos := q * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	return sorted[lo] + (sorted[hi]-sorted[lo])*(pos-float64(lo))
}

func AUCCI(rows []Obs, reps int, rng *rand.Rand) (float64, float64) {
	keys, groups := ByDate(rows)
	draws := make([]float64, 0, reps)
	for r := 0; r < reps; r++ {
		a := AUC(Resample(keys, groups, rng))
		if !math.IsNaN(a) {
			draws = append(draws, a)
		}
	}
	sort.Float64s(draws)
	return Quantile(draws, 0.025), Quantile(draws, 0.975)
}

func AUCDiffCI(a, b []Obs, reps int, rng *rand.Rand) (float64, float64) {
	ka, ga := ByDate(a)
	kb, gb := ByDate(b)
	draws := make([]float64, 0, reps)
	for r := 0; r < reps; r++ {
		x := AUC(Resample(kb, gb, rng))
		y := AUC(Resample(ka, ga, rng))
		if !math.IsNaN(x) && !math.IsNaN(y) {
			draws = append(draws, x-y)
		}
	}
	sort.Float64s(draws)
	return Quantile(draws, 0.025), Quantile(draws, 0.975)
}

func rankOf(v []float64) []float64 {
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
		mid := float64(i+j)/2 + 1
		for k := i; k <= j; k++ {
			out[idx[k]] = mid
		}
		i = j + 1
	}
	return out
}

func Spearman(x, y []float64) float64 {
	if len(x) < 3 || len(x) != len(y) {
		return math.NaN()
	}
	rx, ry := rankOf(x), rankOf(y)
	var mx, my float64
	for i := range rx {
		mx += rx[i]
		my += ry[i]
	}
	mx /= float64(len(rx))
	my /= float64(len(ry))
	var sxy, sxx, syy float64
	for i := range rx {
		sxy += (rx[i] - mx) * (ry[i] - my)
		sxx += (rx[i] - mx) * (rx[i] - mx)
		syy += (ry[i] - my) * (ry[i] - my)
	}
	if sxx == 0 || syy == 0 {
		return math.NaN()
	}
	return sxy / math.Sqrt(sxx*syy)
}

func TercileAUC(rows []Obs) float64 {
	if len(rows) < 30 {
		return math.NaN()
	}
	sorted := append([]Obs(nil), rows...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Aux < sorted[j].Aux })
	var total, weight float64
	for t := 0; t < 3; t++ {
		part := sorted[t*len(sorted)/3 : (t+1)*len(sorted)/3]
		a := AUC(part)
		if math.IsNaN(a) {
			continue
		}
		total += a * float64(len(part))
		weight += float64(len(part))
	}
	if weight == 0 {
		return math.NaN()
	}
	return total / weight
}

func drawBlocks(v []float64, block int, rng *rand.Rand) float64 {
	n := len(v)
	sum, cnt := 0.0, 0
	for cnt < n {
		start := rng.Intn(n)
		for k := 0; k < block && cnt < n; k++ {
			sum += v[(start+k)%n]
			cnt++
		}
	}
	return sum / float64(n)
}

func BlockBootstrapMean(vals []float64, block, reps int, rng *rand.Rand) (float64, float64) {
	if len(vals) == 0 {
		return math.NaN(), math.NaN()
	}
	if block < 1 {
		block = 1
	}
	draws := make([]float64, 0, reps)
	for r := 0; r < reps; r++ {
		draws = append(draws, drawBlocks(vals, block, rng))
	}
	sort.Float64s(draws)
	return Quantile(draws, 0.025), Quantile(draws, 0.975)
}

func BlockBootstrapDiff(a, b []float64, block, reps int, rng *rand.Rand) (float64, float64) {
	if len(a) == 0 || len(b) == 0 {
		return math.NaN(), math.NaN()
	}
	if block < 1 {
		block = 1
	}
	draws := make([]float64, 0, reps)
	for r := 0; r < reps; r++ {
		draws = append(draws, drawBlocks(a, block, rng)-drawBlocks(b, block, rng))
	}
	sort.Float64s(draws)
	return Quantile(draws, 0.025), Quantile(draws, 0.975)
}

func Mean(v []float64) float64 {
	if len(v) == 0 {
		return math.NaN()
	}
	var s float64
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}
