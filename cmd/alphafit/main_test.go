package main

import (
	"math"
	"math/rand"
	"testing"
)

func synthGram(n int, beta []float64, noise float64, seed int64) *gram {
	rng := rand.New(rand.NewSource(seed))
	g := newGram(len(beta))
	for i := 0; i < n; i++ {
		x := make([]float32, len(beta))
		y := 0.0
		for j := range beta {
			x[j] = float32(rng.Float64() - 0.5)
			y += beta[j] * float64(x[j])
		}
		g.addRow(x, y+noise*rng.NormFloat64())
	}
	return g
}

func TestRidgeRecoversCoefficients(t *testing.T) {
	want := []float64{0.8, -0.5, 0, 0.3}
	g := synthGram(20000, want, 0.05, 1)
	b := solveRidge(g, 1e-6, []int{0, 1, 2, 3})
	for i := range want {
		if math.Abs(b[i]-want[i]) > 0.02 {
			t.Fatalf("beta[%d] %v want %v", i, b[i], want[i])
		}
	}
}

func TestElasticNetZeroesNoiseColumns(t *testing.T) {
	g := synthGram(20000, []float64{0.8, 0, 0, 0, 0}, 0.05, 2)
	b := solveEN(g, 0.01, 0.5)
	if b[0] < 0.5 {
		t.Fatalf("signal coefficient shrunk too far: %v", b[0])
	}
	for i := 1; i < 5; i++ {
		if math.Abs(b[i]) > 0.02 {
			t.Fatalf("noise coefficient %d is %v", i, b[i])
		}
	}
}

func TestAimWeightsDollarNeutralLegs(t *testing.T) {
	var rows []*row
	var pred []float64
	for i := 0; i < 50; i++ {
		rows = append(rows, &row{secid: string(rune('A'+i/26)) + string(rune('a'+i%26)), sig: 0.01 + 0.001*float64(i%5)})
		pred = append(pred, float64(i))
	}
	w := aimWeights(rows, pred)
	long, short := 0.0, 0.0
	for _, v := range w {
		if v > 0 {
			long += v
		} else {
			short += -v
		}
	}
	if math.Abs(long-1) > 1e-9 || math.Abs(short-1) > 1e-9 {
		t.Fatalf("legs %v %v", long, short)
	}
}

func TestPartialAdjustmentCutsTurnover(t *testing.T) {
	rows := []*row{{secid: "A", f1: 0.01}, {secid: "B", f1: -0.01}}
	full, part := newBook(1), newBook(0.25)
	a1 := map[string]float64{"A": 1, "B": -1}
	a2 := map[string]float64{"A": -1, "B": 1}
	var cf, cp float64
	for i := 0; i < 6; i++ {
		a := a1
		if i%2 == 1 {
			a = a2
		}
		cf += full.step(rows, a).cost
		cp += part.step(rows, a).cost
	}
	if !(cp < cf) {
		t.Fatalf("partial adjustment cost %v not below full %v", cp, cf)
	}
}

func TestBorrowChargedOnShortNotional(t *testing.T) {
	rows := []*row{{secid: "A", f1: 0}, {secid: "B", f1: 0}}
	b := newBook(1)
	pl := b.step(rows, map[string]float64{"A": 1, "B": -1})
	if math.Abs(pl.short-1) > 1e-12 {
		t.Fatalf("short notional %v", pl.short)
	}
	if math.Abs(pl.cost-2*0.0015) > 1e-12 {
		t.Fatalf("opening cost %v", pl.cost)
	}
}

func TestRankScaleCentered(t *testing.T) {
	r := rankScale([]float64{5, 1, 3, 3, 9})
	s := 0.0
	for _, v := range r {
		s += v
	}
	if math.Abs(s) > 1e-9 {
		t.Fatalf("ranks not centred: %v", r)
	}
}
