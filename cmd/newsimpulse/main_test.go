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

func TestOlsSlopeRecoversCoefficient(t *testing.T) {
	var y []float64
	x := [][]float64{nil, nil}
	for i := 0; i < 50; i++ {
		a, b := float64(i%7)-3, float64((i*5)%11)-5
		x[0], x[1] = append(x[0], a), append(x[1], b)
		y = append(y, 2+3*a-1.5*b)
	}
	if g := olsSlope(y, x, 0); math.Abs(g-3) > 1e-9 {
		t.Fatalf("slope %v", g)
	}
	if g := olsSlope(y, x, 1); math.Abs(g+1.5) > 1e-9 {
		t.Fatalf("slope %v", g)
	}
}

func TestGroupsOfSplitsTrustAndAggregation(t *testing.T) {
	it := item{id: "a", trust: 2, title: "компания объявила дивиденды и прибыль"}
	g := groupsOf(it, 1, map[string]int{"a": 4})
	has := func(x int) bool {
		for _, v := range g {
			if v == x {
				return true
			}
		}
		return false
	}
	if !has(0) || !has(1) || has(2) || !has(4) || has(3) || !has(5) || !has(7) || has(6) {
		t.Fatalf("groups %v", g)
	}
	low := groupsOf(item{id: "b", trust: 0.5, title: "санкции"}, 1, map[string]int{"b": 1})
	if len(low) != 4 || low[1] != 2 || low[2] != 3 || low[3] != 6 {
		t.Fatalf("groups %v", low)
	}
}

func TestMedianTrust(t *testing.T) {
	if m := medianTrust([]item{{trust: 3}, {trust: 1}, {trust: 2}}); m != 2 {
		t.Fatal(m)
	}
}
