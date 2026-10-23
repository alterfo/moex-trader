package main

import (
	"math"
	"math/rand"
	"testing"
)

func TestAddDaysAcrossMonthAndLeapYear(t *testing.T) {
	cases := map[[2]interface{}]string{
		{"2024-02-28", 2}:  "2024-03-01",
		{"2025-12-31", 1}:  "2026-01-01",
		{"2026-10-23", 30}: "2026-11-22",
	}
	for k, want := range cases {
		if got := addDays(k[0].(string), k[1].(int)); got != want {
			t.Fatalf("addDays(%v) = %s want %s", k, got, want)
		}
	}
}

func TestLateDaysUsesThirtyDayRule(t *testing.T) {
	if lateDays("2026-10-30 10:00:00", "2026-10-23", 30) {
		t.Fatal("a row published a week later is not late")
	}
	if !lateDays("2027-01-10 10:00:00", "2026-10-23", 30) {
		t.Fatal("a row published months later is late")
	}
}

func TestKSDistance(t *testing.T) {
	a := []float64{1, 2, 3, 4, 5}
	b := []float64{1, 2, 3, 4, 5}
	if d := ksDist(a, b); d != 0 {
		t.Fatalf("identical samples distance %v", d)
	}
	if d := ksDist([]float64{1, 2, 3}, []float64{10, 11, 12}); math.Abs(d-1) > 1e-12 {
		t.Fatalf("disjoint samples distance %v", d)
	}
}

func TestTsHACRecoversSlope(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	n := 400
	X := make([][]float64, n)
	y := make([]float64, n)
	for i := range X {
		x := rng.NormFloat64()
		X[i] = []float64{1, x, rng.NormFloat64()}
		y[i] = 0.5*x + 0.1*rng.NormFloat64()
	}
	b, tt, ok := tsHAC(X, y, 5)
	if !ok || math.Abs(b-0.5) > 0.05 || tt < 20 {
		t.Fatalf("slope %v t %v ok %v", b, tt, ok)
	}
}

func TestEligibleRespectsPerDatasetLateShare(t *testing.T) {
	allowLate = false
	x := &rec{lateTS: 0, lateOB: 0, lateOS: 1}
	if !eligible(x, "to") {
		t.Fatal("tradestats and obstats realtime must be eligible")
	}
	if eligible(x, "st") {
		t.Fatal("a feature needing orderstats must not be eligible while orderstats is regenerated")
	}
}

func TestFeatureNaNOnRegeneratedOrderstats(t *testing.T) {
	allowLate = false
	s := &store{dates: []string{"2026-01-02"}, groups: []string{"G"}, excl: []bool{false}, byGroup: map[string][]*rec{"G": {{secid: "GH6", bi: 0.2, ti: 0.1, nof: 0.3, ci: 0.4, lateTS: 0, lateOB: 0, lateOS: 1}}}, closes: map[string][]float64{}, lastIdx: map[string]int{}}
	F := buildFeat(s)
	if math.IsNaN(F["BI"][0][0]) || math.IsNaN(F["TI"][0][0]) {
		t.Fatal("BI and TI must exist when tradestats and obstats are realtime")
	}
	if !math.IsNaN(F["NOF"][0][0]) || !math.IsNaN(F["CI"][0][0]) {
		t.Fatal("NOF and CI must be NaN while orderstats is regenerated")
	}
}
