package newsstudy

import (
	"math"
	"math/rand"
	"testing"
)

func TestAUCPerfectAndTies(t *testing.T) {
	rows := []Obs{{Score: 1, Label: 0}, {Score: 2, Label: 0}, {Score: 3, Label: 1}, {Score: 4, Label: 1}}
	if got := AUC(rows); got != 1 {
		t.Fatalf("perfect separation: got %v", got)
	}
	tied := []Obs{{Score: 1, Label: 0}, {Score: 1, Label: 1}}
	if got := AUC(tied); got != 0.5 {
		t.Fatalf("all ties: got %v", got)
	}
	if !math.IsNaN(AUC([]Obs{{Score: 1, Label: 1}})) {
		t.Fatal("single class must be NaN")
	}
}

func TestSpearman(t *testing.T) {
	if got := Spearman([]float64{1, 2, 3, 4}, []float64{10, 20, 30, 40}); math.Abs(got-1) > 1e-9 {
		t.Fatalf("got %v", got)
	}
	if got := Spearman([]float64{1, 2, 3, 4}, []float64{4, 3, 2, 1}); math.Abs(got+1) > 1e-9 {
		t.Fatalf("got %v", got)
	}
}

func TestAUCCIBracketsNoise(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	var rows []Obs
	for d := 0; d < 60; d++ {
		for k := 0; k < 8; k++ {
			rows = append(rows, Obs{Date: string(rune('a' + d)), Score: rng.Float64(), Label: float64(rng.Intn(2))})
		}
	}
	lo, hi := AUCCI(rows, 400, rand.New(rand.NewSource(2)))
	if lo > 0.5 || hi < 0.5 {
		t.Fatalf("noise CI should contain 0.5: [%v,%v]", lo, hi)
	}
}
