package main

import (
	"math"
	"testing"
)

func TestRankScaleMaskedKeepsMissingAndCentres(t *testing.T) {
	in := []float64{1, math.NaN(), 3, 2, 4}
	got := rankScaleMasked(in)
	if !math.IsNaN(got[1]) {
		t.Fatalf("missing value must stay missing, got %v", got[1])
	}
	if got[0] >= got[3] || got[3] >= got[2] || got[2] >= got[4] {
		t.Fatalf("ranks are not monotone: %v", got)
	}
	if got[0] < -0.5 || got[4] > 0.5 {
		t.Fatalf("ranks outside [-0.5,0.5]: %v", got)
	}
	if math.Abs(got[0]+0.375) > 1e-12 || math.Abs(got[4]-0.375) > 1e-12 {
		t.Fatalf("rank positions = %v", got)
	}
}

func TestRankResidualRemovesTheControl(t *testing.T) {
	ctrl := []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0, 1.1, 1.2}
	y := make([]float64, len(ctrl))
	for i, x := range ctrl {
		y[i] = 3*x + 1
	}
	res := rankResidual(y, ctrl)
	for i := range res {
		if math.IsNaN(res[i]) {
			t.Fatalf("row %d is missing", i)
		}
		if math.Abs(res[i]) > 1e-9 {
			t.Fatalf("exactly spanned signal left residual %v at %d", res[i], i)
		}
	}
	y[5] += 1
	res = rankResidual(y, ctrl)
	if math.Abs(res[5]) < 0.5 {
		t.Fatalf("injected deviation not kept: %v", res[5])
	}
	var sum float64
	for _, v := range res {
		sum += v
	}
	if math.Abs(sum) > 1e-9 {
		t.Fatalf("residual is not centred: %v", sum)
	}
}

func TestRankResidualKeepsMissingRows(t *testing.T) {
	y := []float64{1, 2, 3, math.NaN(), 5, 6, 7, 8, 9, 10, 11, 12}
	ctrl := []float64{1, 2, 3, 4, math.NaN(), 6, 7, 8, 9, 10, 11, 12}
	res := rankResidual(y, ctrl)
	if !math.IsNaN(res[3]) || !math.IsNaN(res[4]) {
		t.Fatalf("missing inputs must give missing residuals: %v", res)
	}
	if math.IsNaN(res[0]) {
		t.Fatal("defined row lost its residual")
	}
}

func TestSeriesDiffSkipsMissing(t *testing.T) {
	a := []float64{1, math.NaN(), 3}
	b := []float64{0.5, 1, math.NaN()}
	d := seriesDiff(a, b)
	if math.Abs(d[0]-0.5) > 1e-12 || !math.IsNaN(d[1]) || !math.IsNaN(d[2]) {
		t.Fatalf("diff = %v", d)
	}
}
