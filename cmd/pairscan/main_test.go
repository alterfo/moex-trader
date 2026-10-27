package main

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
)

func TestRegressRecoversSlopeWithFixedEffects(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	var pts []point
	for k := 0; k < 3; k++ {
		for tt := 0; tt < 400; tt++ {
			x := rng.NormFloat64()
			pts = append(pts, point{k: k, t: tt, x: []float64{x}, y: 5*float64(k) + 0.5*x + 0.1*rng.NormFloat64()})
		}
	}
	f := regress(pts, 400, 5)
	if f.coef == nil || math.Abs(f.coef[0]-0.5) > 0.02 || f.t[0] < 20 {
		t.Fatalf("%v %v", f.coef, f.t)
	}
}

func TestSideCostFallbackAndSpread(t *testing.T) {
	if sideCost(nan) != 0.0015 {
		t.Fatal("fallback")
	}
	if math.Abs(sideCost(20)-0.0015) > 1e-12 {
		t.Fatalf("%v", sideCost(20))
	}
}

func TestBuildSkipsDividendDays(t *testing.T) {
	p := &panel{rows: map[string]map[string]obs{}}
	for i, d := range []string{"d1", "d2", "d3"} {
		p.dates = append(p.dates, d)
		p.rows[d] = map[string]obs{
			"A": {rb1: 0.01, div: i == 1, f5: nan, f1: nan, f10: nan, a2: nan, sp: 10},
			"B": {rb1: 0.0, f5: nan, f1: nan, f10: nan, a2: nan, sp: 10},
		}
	}
	s := build(p, pairSpec{id: "x", ord: "A", pref: "B"})
	if s.n != 3 {
		t.Fatalf("n %d", s.n)
	}
}

func TestGroupDiffNeedsEnoughTradesAndSigns(t *testing.T) {
	tr := make([]trade, 20)
	grp := make([]int, 20)
	for i := range tr {
		tr[i].net = float64(i)
		if i < 10 {
			grp[i] = -1
		} else {
			grp[i] = 1
		}
	}
	if d := groupDiff(tr, grp); d != 10 {
		t.Fatalf("difference %v", d)
	}
	if !math.IsNaN(groupDiff(tr[:6], grp[:6])) {
		t.Fatal("too few trades per group must give NaN")
	}
}

func TestTradesAreNonOverlappingAndCostMultiplierScales(t *testing.T) {
	n := 40
	s := &series{spec: pairSpec{id: "X"}, n: n}
	for i := 0; i < n; i++ {
		s.dates = append(s.dates, fmt.Sprintf("2021-01-%02d", i%28+1))
		s.z = append(s.z, 2.0)
		s.y1 = append(s.y1, 0.001)
		s.y5 = append(s.y5, 0.01)
		s.y10 = append(s.y10, 0.02)
		s.spOrd = append(s.spOrd, 10)
		s.spPref = append(s.spPref, 10)
	}
	a := tradesOf([]*series{s}, 5, 1, false)
	if len(a) != n/5 {
		t.Fatalf("trades %d", len(a))
	}
	b := tradesOf([]*series{s}, 5, 2, false)
	if !(b[0].cost > a[0].cost*1.99 && b[0].cost < a[0].cost*2.01) {
		t.Fatalf("cost multiplier %v %v", a[0].cost, b[0].cost)
	}
}
