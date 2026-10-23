package placebo

import (
	"fmt"
	"math"
	"sort"
	"testing"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/model"
)

func sample() Table {
	table := Table{}
	start := time.Date(2025, 3, 5, 0, 0, 0, 0, time.UTC)
	for t := 0; t < 4; t++ {
		ticker := fmt.Sprintf("T%d", t)
		for d := 0; d < 40; d++ {
			table[ticker] = append(table[ticker], model.ScorePoint{
				Date: start.AddDate(0, 0, d),
				P:    0.35 + float64((t*7+d*3)%30)/100,
			})
		}
	}
	return table
}

func values(series []model.ScorePoint) []float64 {
	out := make([]float64, len(series))
	for i, p := range series {
		out[i] = p.P
	}
	sort.Float64s(out)
	return out
}

func equal(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.Abs(a[i]-b[i]) > 1e-12 {
			return false
		}
	}
	return true
}

func TestShiftZeroIsIdentity(t *testing.T) {
	table := sample()
	shifted := Shift(table, 0)
	for ticker, series := range table {
		for i, point := range series {
			if shifted[ticker][i] != point {
				t.Fatalf("%s[%d] changed under k=0", ticker, i)
			}
		}
	}
}

func TestShiftPreservesMultisetAndDates(t *testing.T) {
	table := sample()
	shifted := Shift(table, 13)
	for ticker, series := range table {
		if !equal(values(series), values(shifted[ticker])) {
			t.Fatalf("%s multiset changed", ticker)
		}
		for i := range series {
			if !series[i].Date.Equal(shifted[ticker][i].Date) {
				t.Fatalf("%s date axis changed", ticker)
			}
		}
	}
}

func TestShiftIsCommonAndCyclic(t *testing.T) {
	table := sample()
	shifted := Shift(table, 5)
	for ticker, series := range table {
		for i := range series {
			want := series[(i+5)%len(series)].P
			if shifted[ticker][i].P != want {
				t.Fatalf("%s[%d] = %v want %v", ticker, i, shifted[ticker][i].P, want)
			}
		}
	}
	back := Shift(shifted, len(table["T0"])-5)
	for ticker, series := range table {
		for i, point := range series {
			if back[ticker][i] != point {
				t.Fatalf("%s[%d] round trip failed", ticker, i)
			}
		}
	}
}

func TestPermuteKeepsDateMultiset(t *testing.T) {
	table := sample()
	permuted := PermuteByDate(table, 7)
	for d := 0; d < 40; d++ {
		var before, after []float64
		for ticker := range table {
			before = append(before, table[ticker][d].P)
			after = append(after, permuted[ticker][d].P)
		}
		sort.Float64s(before)
		sort.Float64s(after)
		if !equal(before, after) {
			t.Fatalf("date %d multiset changed", d)
		}
	}
}

func TestPermuteDeterministicAndChangesTable(t *testing.T) {
	table := sample()
	a := PermuteByDate(table, 11)
	b := PermuteByDate(table, 11)
	c := PermuteByDate(table, 12)
	changed, differs := false, false
	for ticker := range table {
		for i := range table[ticker] {
			if a[ticker][i] != b[ticker][i] {
				t.Fatalf("same seed gave different tables")
			}
			if a[ticker][i].P != table[ticker][i].P {
				changed = true
			}
			if a[ticker][i].P != c[ticker][i].P {
				differs = true
			}
		}
	}
	if !changed || !differs {
		t.Fatalf("permutation did not move scores: changed=%v differs=%v", changed, differs)
	}
}

func TestPermuteDoesNotMutateInput(t *testing.T) {
	table := sample()
	before := Shift(table, 0)
	PermuteByDate(table, 3)
	for ticker := range table {
		for i := range table[ticker] {
			if table[ticker][i] != before[ticker][i] {
				t.Fatalf("input mutated")
			}
		}
	}
}

func TestOffsetsRespectHorizonAndLimit(t *testing.T) {
	table := Table{"A": nil}
	origin := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	for d := 0; d < 100; d++ {
		table["A"] = append(table["A"], model.ScorePoint{Date: origin.AddDate(0, 0, d), P: 0.5})
	}
	offsets := Offsets(table, 21, 200)
	if len(offsets) != 100-2*21+1 {
		t.Fatalf("got %d offsets", len(offsets))
	}
	for _, k := range offsets {
		if k < 21 || k > 100-21 {
			t.Fatalf("offset %d outside admissible range", k)
		}
	}
	long := Table{"A": nil}
	start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	for d := 0; d < 1000; d++ {
		long["A"] = append(long["A"], model.ScorePoint{Date: start.AddDate(0, 0, d), P: 0.5})
	}
	limited := Offsets(long, 21, 200)
	if len(limited) != 200 {
		t.Fatalf("got %d offsets, want 200", len(limited))
	}
	seen := map[int]bool{}
	for _, k := range limited {
		if seen[k] {
			t.Fatalf("duplicate offset %d", k)
		}
		seen[k] = true
	}
}

func TestPValue(t *testing.T) {
	if got := PValue(10, []float64{1, 2, 3, 4}); got != 0.2 {
		t.Fatalf("got %v", got)
	}
	if got := PValue(0, []float64{1, 2, 3, 4}); got != 1 {
		t.Fatalf("got %v", got)
	}
}
