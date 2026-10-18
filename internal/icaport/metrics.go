package icaport

import (
	"math"
	"math/rand"
	"sort"
)

type Performance struct {
	AnnReturn float64
	AnnVol    float64
	CVaR95    float64
	MaxDD     float64
	Sharpe    float64
}

func Evaluate(daily []float64) Performance {
	n := len(daily)
	if n == 0 {
		return Performance{}
	}
	wealth := 1.0
	peak := 1.0
	var maxDD, mean float64
	for _, r := range daily {
		wealth *= 1 + r
		if wealth > peak {
			peak = wealth
		}
		if dd := 1 - wealth/peak; dd > maxDD {
			maxDD = dd
		}
		mean += r
	}
	mean /= float64(n)
	var v float64
	for _, r := range daily {
		d := r - mean
		v += d * d
	}
	std := math.Sqrt(v / float64(n-1))
	p := Performance{
		AnnReturn: math.Pow(wealth, 252/float64(n)) - 1,
		AnnVol:    std * math.Sqrt(252),
		CVaR95:    CVaR(daily, 0.95),
		MaxDD:     maxDD,
	}
	if std > 0 {
		p.Sharpe = mean / std * math.Sqrt(252)
	}
	return p
}

func CVaR(daily []float64, level float64) float64 {
	losses := make([]float64, len(daily))
	for i, r := range daily {
		losses[i] = -r
	}
	sort.Sort(sort.Reverse(sort.Float64Slice(losses)))
	k := int(math.Ceil(float64(len(losses)) * (1 - level)))
	if k < 1 {
		k = 1
	}
	var s float64
	for i := 0; i < k; i++ {
		s += losses[i]
	}
	return s / float64(k)
}

func StationaryIndices(n int, meanBlock float64, rng *rand.Rand) []int {
	idx := make([]int, n)
	p := 1 / meanBlock
	cur := rng.Intn(n)
	for i := 0; i < n; i++ {
		if i > 0 && rng.Float64() < p {
			cur = rng.Intn(n)
		} else if i > 0 {
			cur = (cur + 1) % n
		}
		idx[i] = cur
	}
	return idx
}

type RatioCI struct {
	Point float64
	Lo    float64
	Hi    float64
}

func BootstrapRatios(strategy, control []float64, repeats int, meanBlock float64, seed int64, stat func([]float64) float64) RatioCI {
	rng := rand.New(rand.NewSource(seed))
	n := len(strategy)
	ratios := make([]float64, 0, repeats)
	bs := make([]float64, n)
	bc := make([]float64, n)
	for r := 0; r < repeats; r++ {
		idx := StationaryIndices(n, meanBlock, rng)
		for i, k := range idx {
			bs[i] = strategy[k]
			bc[i] = control[k]
		}
		den := stat(bc)
		if den == 0 {
			continue
		}
		ratios = append(ratios, stat(bs)/den)
	}
	return RatioCI{
		Point: stat(strategy) / stat(control),
		Lo:    Quantile(ratios, 0.025),
		Hi:    Quantile(ratios, 0.975),
	}
}
