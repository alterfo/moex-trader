// Package drift provides a population stability index (PSI) check for
// monitoring whether live feature values have drifted away from the
// distribution the model was trained on. It is diagnostics-only: it emits
// warnings and never blocks a trade.
package drift

import (
	"fmt"
	"log"
	"math"
	"sort"
	"sync"
)

// Distribution is a univariate reference distribution described by bins of
// equal expected probability. Edges holds Bins-1 ascending cut points; the
// first bin is (-inf, Edges[0]) and the last is (Edges[Bins-2], +inf).
type Distribution struct {
	Bins  int       `json:"bins"`
	Edges []float64 `json:"edges"`
}

// QuantileDistribution builds a reference distribution with equal-probability
// bins from a training sample. Values that are NaN or infinite are ignored.
func QuantileDistribution(training []float64, bins int) Distribution {
	if bins < 2 {
		bins = 2
	}
	values := make([]float64, 0, len(training))
	for _, v := range training {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		values = append(values, v)
	}
	sort.Float64s(values)

	edges := make([]float64, 0, bins-1)
	for i := 1; i < bins; i++ {
		edges = append(edges, sampleQuantile(values, float64(i)/float64(bins)))
	}
	return Distribution{Bins: bins, Edges: edges}
}

// NormalDistribution builds a reference distribution from a mean and standard
// deviation using normal quantiles as the equal-probability bin edges.
func NormalDistribution(mean, std float64, bins int) Distribution {
	if bins < 2 {
		bins = 2
	}
	edges := make([]float64, 0, bins-1)
	for i := 1; i < bins; i++ {
		edges = append(edges, mean+std*normalQuantile(float64(i)/float64(bins)))
	}
	return Distribution{Bins: bins, Edges: edges}
}

// PSI returns the population stability index of an observed sample against the
// reference distribution. It uses the equal-expected-probability binning, so
// the reference must have been built with either QuantileDistribution or
// NormalDistribution. An empty observed sample returns zero.
func PSI(ref Distribution, observed []float64) float64 {
	if ref.Bins < 2 || len(ref.Edges) != ref.Bins-1 || len(observed) == 0 {
		return 0
	}

	counts := make([]float64, ref.Bins)
	for _, value := range observed {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}
		counts[ref.bin(value)]++
	}

	total := 0.0
	for _, count := range counts {
		total += count
	}
	if total == 0 {
		return 0
	}

	// Zero-count bins would produce log(0); smooth them to a tiny non-zero
	// proportion so the resulting PSI stays finite while still signalling a
	// near-complete loss of mass in that bin.
	const epsilon = 1e-6
	expected := 1.0 / float64(ref.Bins)
	psi := 0.0
	for _, count := range counts {
		actual := count / total
		if actual <= 0 {
			actual = epsilon
		}
		psi += (actual - expected) * math.Log(actual/expected)
	}
	return psi
}

func (d Distribution) bin(value float64) int {
	return sort.SearchFloat64s(d.Edges, value)
}

func sampleQuantile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}
	position := p * float64(len(sorted)-1)
	low := int(math.Floor(position))
	high := int(math.Ceil(position))
	if low == high {
		return sorted[low]
	}
	frac := position - float64(low)
	return sorted[low]*(1-frac) + sorted[high]*frac
}

// NormalReference builds a per-feature reference distribution from training
// mean and standard deviation arrays. Features with a non-positive or
// non-finite standard deviation are skipped because a degenerate reference
// cannot be compared with PSI.
func NormalReference(features []string, mean, std []float64, bins int) (map[string]Distribution, error) {
	if len(features) != len(mean) || len(features) != len(std) {
		return nil, fmt.Errorf("drift: feature/mean/std length mismatch: %d/%d/%d", len(features), len(mean), len(std))
	}
	if bins < 2 {
		bins = 2
	}
	ref := make(map[string]Distribution, len(features))
	for i, name := range features {
		if !(std[i] > 0) || math.IsNaN(std[i]) || math.IsInf(std[i], 0) || math.IsNaN(mean[i]) || math.IsInf(mean[i], 0) {
			continue
		}
		ref[name] = NormalDistribution(mean[i], std[i], bins)
	}
	return ref, nil
}

// Warning describes one feature whose recent observed distribution drifted
// beyond the configured PSI threshold.
type Warning struct {
	Feature string  `json:"feature"`
	PSI     float64 `json:"psi"`
}

// Monitor tracks a rolling window of live feature values and compares each
// feature's observed distribution to its training reference.
type Monitor struct {
	mu sync.Mutex

	reference  map[string]Distribution
	threshold  float64
	minSamples int
	maxWindow  int
	logger     *log.Logger
	windows    map[string][]float64
	warned     map[string]bool
}

// NewMonitor returns a monitor over the given per-feature reference
// distributions. minSamples controls how many observations must accumulate
// before the first PSI is computed; maxWindow caps the rolling window size.
// A nil logger falls back to the standard logger.
func NewMonitor(reference map[string]Distribution, threshold float64, minSamples, maxWindow int, logger *log.Logger) *Monitor {
	if threshold <= 0 {
		threshold = 0.2
	}
	if minSamples <= 0 {
		minSamples = 64
	}
	if maxWindow < minSamples {
		maxWindow = minSamples
	}
	if logger == nil {
		logger = log.Default()
	}
	return &Monitor{
		reference:  reference,
		threshold:  threshold,
		minSamples: minSamples,
		maxWindow:  maxWindow,
		logger:     logger,
		windows:    make(map[string][]float64),
		warned:     make(map[string]bool),
	}
}

// Observe ingests one feature vector and returns warnings for features whose
// current PSI exceeds the threshold. Each drifting feature is reported once
// per drift episode: the warning is suppressed until the feature returns below
// the threshold and drifts above it again.
func (m *Monitor) Observe(vector []float64, featureOrder []string) []Warning {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(vector) != len(featureOrder) {
		return nil
	}
	for i, name := range featureOrder {
		if _, ok := m.reference[name]; !ok {
			continue
		}
		window := m.windows[name]
		window = append(window, vector[i])
		if len(window) > m.maxWindow {
			window = window[len(window)-m.maxWindow:]
		}
		m.windows[name] = window
	}

	var warnings []Warning
	for name, ref := range m.reference {
		window := m.windows[name]
		if len(window) < m.minSamples {
			continue
		}
		psi := PSI(ref, window)
		if psi > m.threshold {
			if !m.warned[name] {
				m.warned[name] = true
				warnings = append(warnings, Warning{Feature: name, PSI: psi})
				m.logger.Printf("drift: feature %s PSI %.3f exceeds threshold %.3f", name, psi, m.threshold)
			}
		} else {
			m.warned[name] = false
		}
	}
	return warnings
}

// normalQuantile is the inverse standard normal CDF (Acklam's rational
// approximation). It is accurate to about 1.15e-9 in the body of the
// distribution, which is far more than PSI binning needs.
func normalQuantile(p float64) float64 {
	const (
		a1 = -3.969683028665376e+01
		a2 = 2.209460984245205e+02
		a3 = -2.759285104469687e+02
		a4 = 1.383577518672690e+02
		a5 = -3.066479806614716e+01
		a6 = 2.506628277459239e+00

		b1 = -5.447609879822406e+01
		b2 = 1.615858368580409e+02
		b3 = -1.556989798598866e+02
		b4 = 6.680131188771972e+01
		b5 = -1.328068155288572e+01

		c1 = -7.784894002430293e-03
		c2 = -3.223964580411365e-01
		c3 = -2.400758277161838e+00
		c4 = -2.549732539343734e+00
		c5 = 4.374664141464968e+00
		c6 = 2.938163982698783e+00

		d1 = 7.784695709041462e-03
		d2 = 3.224671290700398e-01
		d3 = 2.445134137142996e+00
		d4 = 3.754408661907416e+00

		pLow  = 0.02425
		pHigh = 1 - pLow
	)

	if p <= 0 {
		return math.Inf(-1)
	}
	if p >= 1 {
		return math.Inf(1)
	}
	if p < pLow {
		q := math.Sqrt(-2 * math.Log(p))
		return (((((c1*q+c2)*q+c3)*q+c4)*q+c5)*q + c6) /
			((((d1*q+d2)*q+d3)*q+d4)*q + 1)
	}
	if p > pHigh {
		q := math.Sqrt(-2 * math.Log(1-p))
		return -(((((c1*q+c2)*q+c3)*q+c4)*q+c5)*q + c6) /
			((((d1*q+d2)*q+d3)*q+d4)*q + 1)
	}
	q := p - 0.5
	r := q * q
	return (((((a1*r+a2)*r+a3)*r+a4)*r+a5)*r + a6) * q /
		(((((b1*r+b2)*r+b3)*r+b4)*r+b5)*r + 1)
}
