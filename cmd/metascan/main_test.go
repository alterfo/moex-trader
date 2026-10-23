package main

import (
	"math"
	"testing"
	"time"
)

func TestMoonAgeAtReferenceNewMoon(t *testing.T) {
	a := moonAge(time.Date(2000, 1, 6, 18, 14, 0, 0, time.UTC))
	if math.Abs(a) > 1e-6 {
		t.Fatalf("age %v", a)
	}
	full := moonAge(time.Date(2000, 1, 21, 4, 40, 0, 0, time.UTC))
	if math.Abs(full-14.765) > 0.6 {
		t.Fatalf("full moon age %v", full)
	}
}

func TestSaturnLongitudeKnownDates(t *testing.T) {
	l := saturnLongitude(time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC))
	if math.Abs(l-40.4) > 2 {
		t.Fatalf("saturn 2000-01-01 %v", l)
	}
	l = saturnLongitude(time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC))
	if math.Abs(l-292.3) > 2 {
		t.Fatalf("saturn 2020-01-01 %v", l)
	}
}

func TestCircularShiftKeepsRuns(t *testing.T) {
	s := []float64{1, 1, 0, 0, 0, 1}
	o := shifted(s, 2)
	want := []float64{0, 0, 0, 1, 1, 1}
	for i := range want {
		if o[i] != want[i] {
			t.Fatalf("shift %v", o)
		}
	}
}

func TestNyholtIdenticalSeriesCountOnce(t *testing.T) {
	a := make([]float64, 100)
	for i := range a {
		a[i] = float64(i % 7)
	}
	m := nyholt([][]float64{a, a, a, a})
	if m < 0.99 || m > 1.5 {
		t.Fatalf("meff %v", m)
	}
}

func TestPermTestDetectsAlignedState(t *testing.T) {
	n := 300
	s := make([]float64, n)
	y := make([]float64, n)
	for i := range s {
		if (i/40)%2 == 0 {
			s[i] = 1
			y[i] = 1
		}
	}
	p, _, _ := permTest(func(sh []float64) float64 {
		v, _, _ := diffMeans(sh, y)
		return v
	}, s, nil)
	if p > 0.02 {
		t.Fatalf("p %v", p)
	}
}
