package main

import (
	"math"
	"testing"
)

func TestSumRangeAndOLS(t *testing.T) {
	v := make([]float64, 300)
	for i := range v {
		v[i] = float64(i)
	}
	if got := sumRange(v, 44, 299); got != 256*(44+299)/2 {
		t.Fatalf("mom256 sum %v", got)
	}
	if got := sumRange(v, 44, 279); got != 236*(44+279)/2 {
		t.Fatalf("mom121 sum %v", got)
	}
	x := []float64{0, 1, 2, 3, 4, 5}
	y := []float64{1, 3, 5, 7, 9, 11}
	co := olsCoef([][]float64{x}, y)
	if co == nil || math.Abs(co[0]-2) > 1e-9 {
		t.Fatalf("ols slope %v", co)
	}
}
