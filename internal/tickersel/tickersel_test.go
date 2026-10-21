package tickersel

import (
	"math"
	"math/rand"
	"reflect"
	"testing"
	"time"
)

func day(s string) time.Time {
	d, _ := time.Parse("2006-01-02", s)
	return d
}

func TestBlocksCoverRangeWithoutGaps(t *testing.T) {
	got := Blocks(day("2025-01-01"), day("2025-04-10"), 30)
	if len(got) != 4 {
		t.Fatalf("blocks %d", len(got))
	}
	if !got[3].Till.Equal(day("2025-04-10")) {
		t.Fatalf("last till %v", got[3].Till)
	}
	for i := 1; i < len(got); i++ {
		if !got[i].From.Equal(got[i-1].Till.AddDate(0, 0, 1)) {
			t.Fatalf("gap at %d", i)
		}
	}
}

func TestWorstKDeterministicTieBreak(t *testing.T) {
	tickers := []string{"C", "A", "B", "D"}
	pnl := map[string]float64{"A": -5, "B": -5, "C": 3, "D": -9}
	got := WorstK(tickers, pnl, 2)
	if !reflect.DeepEqual(got, []string{"D", "A"}) {
		t.Fatalf("got %v", got)
	}
}

func TestKeepRemovesDropped(t *testing.T) {
	got := Keep([]string{"A", "B", "C"}, []string{"B"})
	if !reflect.DeepEqual(got, []string{"A", "C"}) {
		t.Fatalf("got %v", got)
	}
}

func TestRandomDropSizeAndDistinct(t *testing.T) {
	tickers := []string{"A", "B", "C", "D", "E"}
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 50; i++ {
		got := RandomDrop(tickers, 3, rng)
		seen := map[string]bool{}
		for _, g := range got {
			seen[g] = true
		}
		if len(got) != 3 || len(seen) != 3 {
			t.Fatalf("got %v", got)
		}
	}
}

func TestSpearmanExtremes(t *testing.T) {
	tickers := []string{"A", "B", "C", "D"}
	a := map[string]float64{"A": 1, "B": 2, "C": 3, "D": 4}
	same := map[string]float64{"A": 10, "B": 20, "C": 30, "D": 40}
	rev := map[string]float64{"A": 4, "B": 3, "C": 2, "D": 1}
	if math.Abs(Spearman(tickers, a, same)-1) > 1e-9 {
		t.Fatal("want 1")
	}
	if math.Abs(Spearman(tickers, a, rev)+1) > 1e-9 {
		t.Fatal("want -1")
	}
}

func TestOverlap(t *testing.T) {
	tickers := []string{"A", "B", "C", "D"}
	next := map[string]float64{"A": -9, "B": -1, "C": 5, "D": 7}
	if got := Overlap([]string{"A", "D"}, tickers, next); got != 0.5 {
		t.Fatalf("got %v", got)
	}
}
