package main

import (
	"math"
	"testing"
)

func TestLexScoreSign(t *testing.T) {
	if lexScore("Компания объявила рекордную прибыль и дивиденд") <= 0 {
		t.Fatal("positive headline must score above zero")
	}
	if lexScore("Банкротство и крупный убыток, санкции") >= 0 {
		t.Fatal("negative headline must score below zero")
	}
}

func TestTrailingZExcludesToday(t *testing.T) {
	v := make([]float64, 30)
	for i := 0; i < 25; i++ {
		v[i] = 2
	}
	v[25] = 10
	z := trailingZ(v, 25)
	if math.Abs(z-(10-2)/math.Sqrt(3)) > 1e-9 {
		t.Fatalf("z %v", z)
	}
}
