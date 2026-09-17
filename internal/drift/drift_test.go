package drift

import (
	"io"
	"log"
	"math"
	"testing"
)

func TestPSIZeroWhenObservedMatchesReference(t *testing.T) {
	ref := NormalDistribution(0, 1, 10)
	observed := make([]float64, 1000)
	for i := range observed {
		observed[i] = normalQuantile((float64(i) + 0.5) / 1000)
	}
	psi := PSI(ref, observed)
	if psi > 0.05 {
		t.Fatalf("PSI() = %f, want near zero for a matching sample", psi)
	}
}

func TestPSIRisesWithShiftedDistribution(t *testing.T) {
	ref := NormalDistribution(0, 1, 10)
	observed := make([]float64, 1000)
	for i := range observed {
		observed[i] = 2 + normalQuantile((float64(i)+0.5)/1000)
	}
	shifted := PSI(ref, observed)

	matched := make([]float64, 1000)
	for i := range matched {
		matched[i] = normalQuantile((float64(i) + 0.5) / 1000)
	}
	base := PSI(ref, matched)

	if shifted <= base || shifted < 0.5 {
		t.Fatalf("PSI() shifted = %f, matched = %f; want shifted meaningfully higher", shifted, base)
	}
}

func TestPSIEmptyOrDegenerate(t *testing.T) {
	if got := PSI(Distribution{}, nil); got != 0 {
		t.Fatalf("PSI() empty = %f, want 0", got)
	}
	if got := PSI(Distribution{Bins: 2, Edges: []float64{0}}, nil); got != 0 {
		t.Fatalf("PSI() no observed = %f, want 0", got)
	}
	ref := NormalDistribution(0, 1, 4)
	nan := []float64{math.NaN(), math.NaN(), math.NaN()}
	if got := PSI(ref, nan); got != 0 {
		t.Fatalf("PSI() all NaN = %f, want 0", got)
	}
}

func TestQuantileDistributionHasEqualProbabilityBins(t *testing.T) {
	sample := make([]float64, 0, 1000)
	for i := 0; i < 1000; i++ {
		sample = append(sample, float64(i))
	}
	ref := QuantileDistribution(sample, 10)
	if len(ref.Edges) != 9 {
		t.Fatalf("QuantileDistribution() edges = %d, want 9", len(ref.Edges))
	}
	if got := PSI(ref, sample); got > 0.05 {
		t.Fatalf("PSI() of training sample against its own reference = %f, want near zero", got)
	}
}

func TestNormalReferenceSkipsDegenerateStd(t *testing.T) {
	ref, err := NormalReference([]string{"a", "b"}, []float64{0, 1}, []float64{0, 2}, 10)
	if err != nil {
		t.Fatalf("NormalReference() error = %v", err)
	}
	if _, ok := ref["a"]; ok {
		t.Fatal("NormalReference() kept a feature with zero std")
	}
	if _, ok := ref["b"]; !ok {
		t.Fatal("NormalReference() dropped a feature with positive std")
	}
}

func TestNormalReferenceLengthMismatch(t *testing.T) {
	if _, err := NormalReference([]string{"a"}, []float64{0, 1}, []float64{2}, 10); err == nil {
		t.Fatal("NormalReference() error = nil, want length mismatch error")
	}
}

func normalWindow(n int) []float64 {
	values := make([]float64, n)
	for i := range values {
		values[i] = normalQuantile((float64(i) + 0.5) / float64(n))
	}
	return values
}

func TestMonitorWarnsOncePerDriftEpisode(t *testing.T) {
	ref := map[string]Distribution{"f": NormalDistribution(0, 1, 8)}
	logger := log.New(io.Discard, "", 0)
	monitor := NewMonitor(ref, 0.1, 20, 20, logger)

	var warnings []Warning
	for _, value := range normalWindow(20) {
		warnings = append(warnings, monitor.Observe([]float64{value}, []string{"f"})...)
	}
	if len(warnings) != 0 {
		t.Fatalf("Observe() matched-sample warnings = %d, want 0", len(warnings))
	}

	// Shift every value far from the reference. Once the rolling window is
	// dominated by shifted values the monitor must warn exactly once, and it
	// must keep suppressing the warning while the feature stays drifted.
	var driftWarnings []Warning
	for i := 0; i < 25; i++ {
		driftWarnings = append(driftWarnings, monitor.Observe([]float64{10}, []string{"f"})...)
	}
	if len(driftWarnings) != 1 {
		t.Fatalf("Observe() drift warning count = %d, want 1 (deduplicated)", len(driftWarnings))
	}
	if driftWarnings[0].Feature != "f" || driftWarnings[0].PSI <= 0.1 {
		t.Fatalf("Observe() warning = %+v, want feature f with PSI > 0.1", driftWarnings[0])
	}
}

func TestMonitorIgnoresUnknownFeaturesAndLengthMismatch(t *testing.T) {
	ref := map[string]Distribution{"f": NormalDistribution(0, 1, 8)}
	monitor := NewMonitor(ref, 0.1, 5, 10, log.New(io.Discard, "", 0))
	if got := monitor.Observe([]float64{1}, []string{"unknown"}); len(got) != 0 {
		t.Fatalf("Observe() unknown feature warnings = %d, want 0", len(got))
	}
	if got := monitor.Observe([]float64{1, 2}, []string{"f"}); len(got) != 0 {
		t.Fatalf("Observe() length mismatch warnings = %d, want 0", len(got))
	}
}

func TestNormalQuantileSymmetryAndMedian(t *testing.T) {
	if got := normalQuantile(0.5); math.Abs(got) > 1e-9 {
		t.Fatalf("normalQuantile(0.5) = %f, want 0", got)
	}
	if got := normalQuantile(0.975); math.Abs(got-1.959964) > 1e-4 {
		t.Fatalf("normalQuantile(0.975) = %f, want ~1.959964", got)
	}
	if got := normalQuantile(0.025); math.Abs(got+1.959964) > 1e-4 {
		t.Fatalf("normalQuantile(0.025) = %f, want ~-1.959964", got)
	}
}
