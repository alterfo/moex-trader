package main

import (
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
