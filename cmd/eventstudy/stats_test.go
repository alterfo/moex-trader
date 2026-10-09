package main

import (
	"math"
	"math/rand"
	"testing"
)

func TestSolveRecoversLinearSystem(t *testing.T) {
	x, ok := solve([][]float64{{2, 1}, {1, 3}}, []float64{5, 10})
	if !ok || math.Abs(x[0]-1) > 1e-9 || math.Abs(x[1]-3) > 1e-9 {
		t.Fatalf("got %v ok=%v", x, ok)
	}
}

func TestHolmIsMonotoneAndCapped(t *testing.T) {
	got := holm([]float64{0.01, 0.04, 0.03})
	want := []float64{0.03, 0.06, 0.06}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-12 {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestWeekdayOf(t *testing.T) {
	if got := weekdayOf(2026, 10, 9); got != 5 {
		t.Fatalf("2026-10-09 weekday = %d, want 5 (Friday)", got)
	}
}

func TestLogisticFitSeparatesSignal(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	var x [][]float64
	var d []float64
	for i := 0; i < 2000; i++ {
		v := rng.NormFloat64()
		p := 1 / (1 + math.Exp(-(0.5 + 1.5*v)))
		y := 0.0
		if rng.Float64() < p {
			y = 1
		}
		x = append(x, []float64{1, v})
		d = append(d, y)
	}
	beta, ok := logisticFit(x, d, 0.01)
	if !ok || math.Abs(beta[1]-1.5) > 0.25 {
		t.Fatalf("beta=%v ok=%v", beta, ok)
	}
}

func TestDateBootstrapCoversMean(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	vals := []float64{0.01, 0.02, -0.01, 0.03, 0.0, 0.02}
	keys := []int{1, 1, 2, 3, 4, 4}
	iv := dateBootstrap(vals, keys, 500, rng)
	if iv.Lo > iv.Mean || iv.Hi < iv.Mean {
		t.Fatalf("interval %+v does not cover its mean", iv)
	}
}

func TestTwoWayDemeanRemovesFixedEffects(t *testing.T) {
	type row struct{ a, b int }
	rows := []row{{0, 0}, {0, 1}, {1, 0}, {1, 1}}
	v := []float64{1 + 10, 1 + 20, 2 + 10, 2 + 20}
	demeanTwoWay(v, rows, func(r row) (int, int) { return r.a, r.b })
	for _, x := range v {
		if math.Abs(x) > 1e-9 {
			t.Fatalf("residual %v", v)
		}
	}
}
