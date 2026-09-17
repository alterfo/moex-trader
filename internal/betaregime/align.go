package betaregime

import (
	"sort"
	"time"
)

// AlignDaily joins three daily-return series on their common dates and maps
// each observation to the window (cluster) containing its date. Observations
// missing from any series, or outside every window, are counted in skipped and
// dropped. The returned slices are in ascending date order.
func AlignDaily(primary, bench, momentum map[time.Time]float64, windows []Window) (y []float64, x [][]float64, clusters []int, dates []time.Time, skipped int) {
	days := make(map[time.Time]struct{}, len(primary))
	for day := range primary {
		days[day] = struct{}{}
	}
	ordered := make([]time.Time, 0, len(days))
	for day := range days {
		ordered = append(ordered, day)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Before(ordered[j]) })

	for _, day := range ordered {
		p, okP := primary[day]
		b, okB := bench[day]
		m, okM := momentum[day]
		if !okP || !okB || !okM {
			skipped++
			continue
		}
		cluster := -1
		for i, w := range windows {
			if !day.Before(w.From) && !day.After(w.Till) {
				cluster = i
				break
			}
		}
		if cluster < 0 {
			skipped++
			continue
		}
		y = append(y, p)
		x = append(x, []float64{b, m})
		clusters = append(clusters, cluster)
		dates = append(dates, day)
	}
	return y, x, clusters, dates, skipped
}
