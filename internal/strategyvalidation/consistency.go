package strategyvalidation

import (
	"fmt"
	"math"
	"sort"
)

type WindowConsistency struct {
	Windows       int
	Positive      int
	AllPositive   bool
	Worst         float64
	Best          float64
	Median        float64
	SmoothedShare float64
	PassAtK       map[int]float64
}

func ComputeWindowConsistency(windowPnL []float64, horizons []int) (WindowConsistency, error) {
	if len(windowPnL) == 0 {
		return WindowConsistency{}, fmt.Errorf("no windows")
	}
	sorted := append([]float64(nil), windowPnL...)
	sort.Float64s(sorted)
	positive := 0
	for _, v := range windowPnL {
		if v > 0 {
			positive++
		}
	}
	n := len(windowPnL)
	share := float64(positive+1) / float64(n+2)
	passAtK := make(map[int]float64, len(horizons))
	for _, k := range horizons {
		if k < 1 {
			return WindowConsistency{}, fmt.Errorf("horizon %d must be >= 1", k)
		}
		passAtK[k] = math.Pow(share, float64(k))
	}
	median := sorted[n/2]
	if n%2 == 0 {
		median = (sorted[n/2-1] + sorted[n/2]) / 2
	}
	return WindowConsistency{
		Windows:       n,
		Positive:      positive,
		AllPositive:   positive == n,
		Worst:         sorted[0],
		Best:          sorted[n-1],
		Median:        median,
		SmoothedShare: share,
		PassAtK:       passAtK,
	}, nil
}
