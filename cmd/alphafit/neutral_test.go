package main

import (
	"math"
	"testing"
)

func TestNormInvRoundTrip(t *testing.T) {
	for _, p := range []float64{0.01, 0.5, 0.9, 0.995} {
		if math.Abs(normCDF(normInv(p))-p) > 1e-9 {
			t.Fatalf("p %v", p)
		}
	}
}

func TestSectorNeutralDemeansWithinSector(t *testing.T) {
	rows := []*row{{secid: "SBER"}, {secid: "VTBR"}, {secid: "T"}, {secid: "LKOH"}}
	out := sectorNeutral(rows, []float64{1, 2, 3, 10})
	if math.Abs(out[0]+1) > 1e-12 || math.Abs(out[2]-1) > 1e-12 || out[3] != 10 {
		t.Fatalf("%v", out)
	}
}

func TestMomentsNormalish(t *testing.T) {
	x := []float64{-1, 1, -1, 1}
	m, sd, sk, ku := moments(x)
	if m != 0 || sd != 1 || sk != 0 || ku != 1 {
		t.Fatalf("%v %v %v %v", m, sd, sk, ku)
	}
}
