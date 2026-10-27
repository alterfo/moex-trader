package main

import (
	"math"
	"testing"
)

func TestTercilesPickExtremes(t *testing.T) {
	mod := make([]float64, 30)
	valid := make([]int, 30)
	for i := range mod {
		mod[i] = float64((i * 7) % 30)
		valid[i] = i
	}
	lo, hi := terciles(mod, valid)
	if len(lo) != 10 || len(hi) != 10 {
		t.Fatalf("sizes %d %d", len(lo), len(hi))
	}
	for _, i := range lo {
		if mod[i] >= 10 {
			t.Fatal("low tercile contains a high value")
		}
	}
	for _, i := range hi {
		if mod[i] < 20 {
			t.Fatal("high tercile contains a low value")
		}
	}
}

func TestOrthResidualRemovesBetaRank(t *testing.T) {
	n := 60
	sig, beta := make([]float64, n), make([]float64, n)
	for i := range sig {
		beta[i] = float64(i)
		sig[i] = float64(i) + math.Sin(float64(i)*3)
	}
	res := orthResidual(sig, beta)
	rb := rankScale(beta)
	var c, v float64
	for i := range res {
		c += (res[i] - mean(res)) * (rb[i] - mean(rb))
		v += (rb[i] - mean(rb)) * (rb[i] - mean(rb))
	}
	if math.Abs(c/v) > 1e-9 {
		t.Fatalf("residual still loads on beta: %v", c/v)
	}
}

func TestRegimeBootstrapDetectsShift(t *testing.T) {
	n := 600
	ic := make([]float64, n)
	on := make([]bool, n)
	for i := range ic {
		on[i] = (i/40)%2 == 0
		ic[i] = 0.01 * math.Sin(float64(i))
		if on[i] {
			ic[i] += 0.05
		}
	}
	p, lo, hi := regimeBootLevel(ic, on, 20, 1000, 7, 0.05)
	if p < 0.04 || lo <= 0 || hi < lo {
		t.Fatalf("point %v interval [%v, %v]", p, lo, hi)
	}
}

func TestSignFlipBlocksKeepsMagnitude(t *testing.T) {
	x := []float64{1, 2, 3, 4, 5, 6, 7, 8}
	y := signFlipBlocks(x, 4, 3)
	for i := range x {
		if math.Abs(y[i]) != x[i] {
			t.Fatal("magnitudes must be kept")
		}
	}
}

func TestOutcomeSkipsDividendDrop(t *testing.T) {
	x := &row{f1: 0.02, u1: 0.01, divDrop: true}
	if !math.IsNaN(outcomeOf(x, 1)) {
		t.Fatal("dividend-drop rows have no outcome")
	}
	x.divDrop = false
	if math.Abs(outcomeOf(x, 1)-0.01) > 1e-12 {
		t.Fatal("abnormal one-day return")
	}
}

func TestTrendSlopeRecoversDrift(t *testing.T) {
	n := 400
	y, x := make([]float64, n), make([]float64, n)
	for i := range y {
		x[i] = float64(i) / 100
		y[i] = 0.02 - 0.005*x[i] + 0.01*math.Sin(float64(i)*1.7)
	}
	b, tv := trendSlope(y, x, 5)
	if math.Abs(b+0.005) > 0.001 || tv > -3 {
		t.Fatalf("slope %v t %v", b, tv)
	}
}

func TestMedianOfIgnoresNaN(t *testing.T) {
	if m := medianOf([]float64{3, nan, 1, 2}); m != 2 {
		t.Fatal(m)
	}
}
