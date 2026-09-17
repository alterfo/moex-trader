package tailprecision

import (
	"math"
	"math/rand"
	"sort"
	"time"
)

type Direction int

const (
	DirectionHold Direction = 0
	DirectionBuy  Direction = 1
	DirectionSell Direction = -1
)

func (d Direction) String() string {
	switch d {
	case DirectionBuy:
		return "BUY"
	case DirectionSell:
		return "SELL"
	default:
		return "HOLD"
	}
}

type Decision struct {
	Ticker           string
	Date             time.Time
	Direction        Direction
	Probability      float64
	ForwardReturnPct float64
}

type BaseRate struct {
	Up      float64
	Down    float64
	Samples int
}

func ComputeBaseRate(returns []float64, deadbandPct float64) BaseRate {
	rate := BaseRate{Samples: len(returns)}
	if len(returns) == 0 {
		return rate
	}
	var up, down int
	for _, r := range returns {
		switch {
		case r > deadbandPct:
			up++
		case r < -deadbandPct:
			down++
		}
	}
	rate.Up = float64(up) / float64(len(returns))
	rate.Down = float64(down) / float64(len(returns))
	return rate
}

func OutcomeInDirection(d Decision, deadbandPct float64) bool {
	switch d.Direction {
	case DirectionBuy:
		return d.ForwardReturnPct > deadbandPct
	case DirectionSell:
		return d.ForwardReturnPct < -deadbandPct
	default:
		return false
	}
}

func PooledPrecision(decisions []Decision, deadbandPct float64) (precision float64, wins int) {
	if len(decisions) == 0 {
		return 0, 0
	}
	for _, d := range decisions {
		if OutcomeInDirection(d, deadbandPct) {
			wins++
		}
	}
	return float64(wins) / float64(len(decisions)), wins
}

func PooledBaseRate(decisions []Decision, base BaseRate) float64 {
	if len(decisions) == 0 {
		return 0
	}
	buys, sells := 0, 0
	for _, d := range decisions {
		switch d.Direction {
		case DirectionBuy:
			buys++
		case DirectionSell:
			sells++
		}
	}
	total := buys + sells
	if total == 0 {
		return 0
	}
	return (float64(buys)*base.Up + float64(sells)*base.Down) / float64(total)
}

func Episodes(decisions []Decision) [][]Decision {
	if len(decisions) == 0 {
		return nil
	}
	sorted := append([]Decision(nil), decisions...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Ticker != sorted[j].Ticker {
			return sorted[i].Ticker < sorted[j].Ticker
		}
		return sorted[i].Date.Before(sorted[j].Date)
	})
	var episodes [][]Decision
	var current []Decision
	for _, d := range sorted {
		if len(current) > 0 {
			last := current[len(current)-1]
			if last.Ticker != d.Ticker || last.Direction != d.Direction {
				episodes = append(episodes, current)
				current = nil
			}
		}
		current = append(current, d)
	}
	if len(current) > 0 {
		episodes = append(episodes, current)
	}
	return episodes
}

type BootstrapResult struct {
	Trials        int
	Observed      float64
	BaseRate      float64
	BootstrapMean float64
	BootstrapSD   float64
	PValue        float64
}

func BootstrapPValue(decisions []Decision, baseRate, deadbandPct float64, rng *rand.Rand, trials int) BootstrapResult {
	result := BootstrapResult{Trials: trials, BaseRate: baseRate}
	observed, _ := PooledPrecision(decisions, deadbandPct)
	result.Observed = observed
	if rng == nil {
		rng = rand.New(rand.NewSource(1))
	}
	episodes := Episodes(decisions)
	if len(episodes) == 0 || trials <= 0 {
		result.PValue = 1
		return result
	}
	stats := make([]float64, trials)
	for i := 0; i < trials; i++ {
		var sampled []Decision
		for j := 0; j < len(episodes); j++ {
			sampled = append(sampled, episodes[rng.Intn(len(episodes))]...)
		}
		p, _ := PooledPrecision(sampled, deadbandPct)
		stats[i] = p
	}
	result.BootstrapMean = mean(stats)
	result.BootstrapSD = stddev(stats, result.BootstrapMean)

	dObserved := observed - baseRate
	exceed := 0
	for _, s := range stats {
		if s-result.BootstrapMean >= dObserved {
			exceed++
		}
	}
	result.PValue = float64(exceed+1) / float64(trials+1)
	return result
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func stddev(values []float64, meanValue float64) float64 {
	if len(values) < 2 {
		return 0
	}
	var sum float64
	for _, v := range values {
		diff := v - meanValue
		sum += diff * diff
	}
	return math.Sqrt(sum / float64(len(values)-1))
}
