package strategyvalidation

import (
	"math"
	"testing"
)

func TestWindowConsistencyCountsPositiveAndWorst(t *testing.T) {
	got, err := ComputeWindowConsistency([]float64{100, -50, 30, 10, 20, 5}, []int{1, 4})
	if err != nil {
		t.Fatal(err)
	}
	if got.Windows != 6 || got.Positive != 5 || got.AllPositive {
		t.Fatalf("got %+v", got)
	}
	if got.Worst != -50 || got.Best != 100 {
		t.Fatalf("worst/best = %v/%v", got.Worst, got.Best)
	}
	if got.Median != 15 {
		t.Fatalf("median = %v, want 15", got.Median)
	}
	wantShare := 6.0 / 8.0
	if math.Abs(got.SmoothedShare-wantShare) > 1e-12 {
		t.Fatalf("share = %v, want %v", got.SmoothedShare, wantShare)
	}
	if math.Abs(got.PassAtK[4]-math.Pow(wantShare, 4)) > 1e-12 {
		t.Fatalf("pass^4 = %v", got.PassAtK[4])
	}
}

func TestWindowConsistencyAllPositiveStillSmoothed(t *testing.T) {
	got, err := ComputeWindowConsistency([]float64{1, 2, 3}, []int{3})
	if err != nil {
		t.Fatal(err)
	}
	if !got.AllPositive || got.SmoothedShare >= 1 {
		t.Fatalf("got %+v", got)
	}
}

func TestWindowConsistencyRejectsEmptyAndBadHorizon(t *testing.T) {
	if _, err := ComputeWindowConsistency(nil, []int{1}); err == nil {
		t.Fatal("empty windows must error")
	}
	if _, err := ComputeWindowConsistency([]float64{1}, []int{0}); err == nil {
		t.Fatal("horizon 0 must error")
	}
}
