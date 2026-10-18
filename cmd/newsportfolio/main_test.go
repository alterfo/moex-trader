package main

import (
	"fmt"
	"testing"
)

func TestBuildSplitsQuintilesAndSpread(t *testing.T) {
	day := map[string]*dayScore{}
	for i := 0; i < 10; i++ {
		day[fmt.Sprintf("T%02d", i)] = &dayScore{score: float64(i), ret: float64(i), n: 1}
	}
	tr := build(map[string]map[string]*dayScore{"2025-01-02": day}, 10)
	if len(tr) != 1 {
		t.Fatalf("tranches = %d, want 1", len(tr))
	}
	if tr[0].spread != 8 {
		t.Fatalf("spread = %v, want 8 (top mean 8.5 minus bottom mean 0.5)", tr[0].spread)
	}
	for q := 1; q < 5; q++ {
		if tr[0].quintile[q] <= tr[0].quintile[q-1] {
			t.Fatalf("quintiles not increasing: %v", tr[0].quintile)
		}
	}
}

func TestBuildSkipsThinDates(t *testing.T) {
	day := map[string]*dayScore{"A": {score: 1, ret: 1, n: 1}}
	if got := build(map[string]map[string]*dayScore{"2025-01-02": day}, 10); len(got) != 0 {
		t.Fatalf("thin date kept: %v", got)
	}
}

func TestBootstrapIntervalCoversMeanOfConstantSeries(t *testing.T) {
	xs := make([]float64, 50)
	for i := range xs {
		xs[i] = 2
	}
	lo, hi := bootstrap(xs, 10, 200, 1)
	if lo != 2 || hi != 2 {
		t.Fatalf("interval = [%v,%v], want [2,2]", lo, hi)
	}
}
