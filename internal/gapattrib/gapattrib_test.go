package gapattrib

import (
	"math"
	"testing"
	"time"
)

func TestSymDiff(t *testing.T) {
	a := map[int]bool{1: true, 2: true}
	b := map[int]bool{2: true, 3: true}
	if got := symDiff(a, b); got != 2 {
		t.Fatalf("symDiff = %d, want 2", got)
	}
	if got := symDiff(a, a); got != 0 {
		t.Fatalf("symDiff of equal sets = %d, want 0", got)
	}
}

func TestQuintilesPickExtremes(t *testing.T) {
	r := []float64{0.9, 0.1, 0.5, 0.3, 0.7, 0.2, 0.8, 0.4, 0.6, 0.0}
	names := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	top, bot := quintiles(names, r)
	if len(top) != 2 || len(bot) != 2 {
		t.Fatalf("quintile sizes %d/%d, want 2/2", len(top), len(bot))
	}
	if r[top[0]] < 0.8 || r[bot[len(bot)-1]] > 0.1 {
		t.Fatalf("quintiles not extreme: top %v bot %v", top, bot)
	}
}

func TestAdjustFoldsDividendIntoGap(t *testing.T) {
	panel := &Panel{
		Dates:   []time.Time{time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
		Tickers: []string{"A"},
		Open:    [][]float64{{100}, {90}},
		Close:   [][]float64{{100}, {90}},
		Value:   [][]float64{{1}, {1}},
	}
	div := [][]float64{{0}, {10}}
	adj := Adjust(panel, div)
	if math.Abs(adj.Close[1][0]/adj.Close[0][0]-1) > 1e-9 {
		t.Fatalf("a dividend equal to the price drop must give zero adjusted return, got %v", adj.Close[1][0]/adj.Close[0][0]-1)
	}
}

func TestAdjustIgnoresSplitLikeJump(t *testing.T) {
	panel := &Panel{
		Dates:   []time.Time{time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
		Tickers: []string{"A"},
		Open:    [][]float64{{100}, {10}},
		Close:   [][]float64{{100}, {10}},
		Value:   [][]float64{{1}, {1}},
	}
	adj := Adjust(panel, [][]float64{{0}, {0}})
	if adj.Close[1][0] != adj.Close[0][0] {
		t.Fatalf("a -90%% jump is treated as a corporate action and must not enter returns")
	}
}

func TestCarryUsesLaggedRate(t *testing.T) {
	kr := &KeyRate{
		days:  []time.Time{time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)},
		rates: []float64{10, 20},
	}
	got := kr.Carry(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC))
	want := (10.0 + 10.0 + 20.0) / 100 / 365
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("carry = %v, want %v", got, want)
	}
}

func TestNeweyWestTSign(t *testing.T) {
	up := []float64{0.01, 0.02, 0.015, 0.012, 0.018, 0.011, 0.02, 0.013}
	if neweyWestT(up, 3) <= 0 {
		t.Fatal("positive series must give positive t")
	}
	if neweyWestT([]float64{1, 1}, 3) != 0 {
		t.Fatal("too short series must give zero")
	}
}

func TestSummarizeEmpty(t *testing.T) {
	if s := Summarize(nil); s.Periods != 0 || s.MeanNet != 0 {
		t.Fatalf("empty summary must be zero, got %+v", s)
	}
}
