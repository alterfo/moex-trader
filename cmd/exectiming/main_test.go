package main

import (
	"math"
	"testing"
)

func TestSlotOfBoundaries(t *testing.T) {
	cases := map[string]int{"10:00:00": -1, "10:05:00": 0, "10:30:00": 0, "10:35:00": 1, "18:00:00": 15, "18:30:00": 16, "18:35:00": -1, "09:55:00": -1}
	for in, want := range cases {
		if got := slotOf(in); got != want {
			t.Fatalf("%s: got %d want %d", in, got, want)
		}
	}
}

func TestSpearmanMonotone(t *testing.T) {
	a := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}
	b := []float64{2, 4, 6, 8, 10, 12, 14, 16, 18, 20, 22}
	if math.Abs(spearman(a, b)-1) > 1e-12 {
		t.Fatal("rank correlation of a monotone pair must be 1")
	}
}
