package tickersel

import (
	"math"
	"math/rand"
	"sort"
	"time"
)

type Block struct {
	From time.Time
	Till time.Time
}

func Blocks(from, till time.Time, days int) []Block {
	var out []Block
	if days <= 0 {
		return out
	}
	for start := from; !start.After(till); start = start.AddDate(0, 0, days) {
		end := start.AddDate(0, 0, days-1)
		if end.After(till) {
			end = till
		}
		out = append(out, Block{From: start, Till: end})
	}
	return out
}

func rank(tickers []string, pnl map[string]float64) []string {
	order := append([]string(nil), tickers...)
	sort.Slice(order, func(i, j int) bool {
		a, b := pnl[order[i]], pnl[order[j]]
		if a != b {
			return a < b
		}
		return order[i] < order[j]
	})
	return order
}

func WorstK(tickers []string, pnl map[string]float64, k int) []string {
	if k <= 0 {
		return nil
	}
	order := rank(tickers, pnl)
	if k > len(order) {
		k = len(order)
	}
	return order[:k]
}

func Keep(tickers, drop []string) []string {
	skip := make(map[string]bool, len(drop))
	for _, t := range drop {
		skip[t] = true
	}
	out := make([]string, 0, len(tickers))
	for _, t := range tickers {
		if !skip[t] {
			out = append(out, t)
		}
	}
	return out
}

func RandomDrop(tickers []string, k int, rng *rand.Rand) []string {
	if k <= 0 {
		return nil
	}
	if k > len(tickers) {
		k = len(tickers)
	}
	perm := rng.Perm(len(tickers))
	out := make([]string, k)
	for i := 0; i < k; i++ {
		out[i] = tickers[perm[i]]
	}
	return out
}

func ranks(values []float64) []float64 {
	idx := make([]int, len(values))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return values[idx[a]] < values[idx[b]] })
	out := make([]float64, len(values))
	for i := 0; i < len(idx); {
		j := i
		for j+1 < len(idx) && values[idx[j+1]] == values[idx[i]] {
			j++
		}
		mid := float64(i+j)/2 + 1
		for q := i; q <= j; q++ {
			out[idx[q]] = mid
		}
		i = j + 1
	}
	return out
}

func Spearman(tickers []string, a, b map[string]float64) float64 {
	n := len(tickers)
	if n < 3 {
		return 0
	}
	x := make([]float64, n)
	y := make([]float64, n)
	for i, t := range tickers {
		x[i], y[i] = a[t], b[t]
	}
	rx, ry := ranks(x), ranks(y)
	var mx, my float64
	for i := 0; i < n; i++ {
		mx += rx[i]
		my += ry[i]
	}
	mx /= float64(n)
	my /= float64(n)
	var sxy, sxx, syy float64
	for i := 0; i < n; i++ {
		dx, dy := rx[i]-mx, ry[i]-my
		sxy += dx * dy
		sxx += dx * dx
		syy += dy * dy
	}
	if sxx == 0 || syy == 0 {
		return 0
	}
	return sxy / (math.Sqrt(sxx) * math.Sqrt(syy))
}

func Overlap(drop []string, tickers []string, pnl map[string]float64) float64 {
	if len(drop) == 0 {
		return 0
	}
	worst := make(map[string]bool, len(drop))
	for _, t := range WorstK(tickers, pnl, len(drop)) {
		worst[t] = true
	}
	hits := 0
	for _, t := range drop {
		if worst[t] {
			hits++
		}
	}
	return float64(hits) / float64(len(drop))
}
