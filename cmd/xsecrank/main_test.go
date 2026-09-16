package main

import (
	"math"
	"testing"
)

func TestPercentileRanksMonotonicAndBounded(t *testing.T) {
	ranks := percentileRanks([]float64{10, 20, 30, 40, 50})
	for i := 1; i < len(ranks); i++ {
		if ranks[i] <= ranks[i-1] {
			t.Fatalf("ranks must be strictly increasing for distinct inputs, got %v", ranks)
		}
	}
	if ranks[0] != 0 || ranks[len(ranks)-1] != 1 {
		t.Fatalf("expected min rank 0 and max rank 1, got %v", ranks)
	}
}

func TestPercentileRanksTiesShareMidpoint(t *testing.T) {
	ranks := percentileRanks([]float64{5, 5, 5})
	for i, r := range ranks {
		if math.Abs(r-0.5) > 1e-9 {
			t.Fatalf("all-equal values should rank 0.5, index %d got %v", i, r)
		}
	}
}

func TestPercentileRanksSingleValue(t *testing.T) {
	if got := percentileRanks([]float64{42})[0]; got != 0.5 {
		t.Fatalf("single value should rank 0.5, got %v", got)
	}
}

func TestCompositePercentileAveragesFeatureRanks(t *testing.T) {
	group := []row{
		{ticker: "A", values: map[string]float64{"x": 1, "y": 3}},
		{ticker: "B", values: map[string]float64{"x": 2, "y": 2}},
		{ticker: "C", values: map[string]float64{"x": 3, "y": 1}},
	}
	scores := compositePercentile(group, []string{"x", "y"})
	// A is best on x (rank 1) and worst on y (rank 0) -> 0.5, same for C.
	if math.Abs(scores["A"]-0.5) > 1e-9 || math.Abs(scores["C"]-0.5) > 1e-9 {
		t.Fatalf("opposing ranks should cancel to 0.5, got A=%v C=%v", scores["A"], scores["C"])
	}
	// B is middle on both features.
	if math.Abs(scores["B"]-0.5) > 1e-9 {
		t.Fatalf("middle ticker should score 0.5, got %v", scores["B"])
	}
}
