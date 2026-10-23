package main

import (
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestWeightedAUCKnownValues(t *testing.T) {
	score := []float64{0.1, 0.4, 0.35, 0.8}
	y := []bool{false, false, true, true}
	w := []float64{1, 1, 1, 1}
	got := weightedAUC(rankOrder(score), y, w)
	if math.Abs(got-0.75) > 1e-12 {
		t.Fatalf("auc %v want 0.75", got)
	}
	tied := weightedAUC(rankOrder([]float64{1, 1, 1, 1}), y, w)
	if math.Abs(tied-0.5) > 1e-12 {
		t.Fatalf("tied auc %v want 0.5", tied)
	}
	w2 := []float64{2, 0, 1, 1}
	if got := weightedAUC(rankOrder(score), y, w2); math.Abs(got-1.0) > 1e-12 {
		t.Fatalf("weighted auc %v want 1.0", got)
	}
}

func TestSpearmanMonotone(t *testing.T) {
	a := []float64{1, 2, 3, 4, 5}
	b := []float64{10, 20, 30, 40, 50}
	if got := spearman(a, b); math.Abs(got-1) > 1e-12 {
		t.Fatalf("spearman %v", got)
	}
}

func TestBlockWeightsCoverAllDates(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	dateOf := []int{0, 0, 1, 2, 3, 3, 4, 5, 6, 7, 8, 9, 10, 11}
	w := blockWeights(dateOf, 12, 4, rng)
	if len(w) != len(dateOf) {
		t.Fatal("length")
	}
	for _, x := range w {
		if x < 0 {
			t.Fatal("negative weight")
		}
	}
}

func TestMergeDropsIncompleteAndShufflePreservesVectors(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.csv")
	block := filepath.Join(dir, "block.csv")
	out := filepath.Join(dir, "out.csv")
	os.WriteFile(base, []byte("ticker,date,label,label_date,split,f1\nA,2025-01-03,1,2025-01-20,train,0.5\nB,2025-01-03,0,2025-01-20,train,0.6\nC,2025-01-03,1,2025-01-20,train,0.7\n"), 0o644)
	hdr := "date,secid,t18,adm100,realtime,div_drop,close," + strings.Join(apNames, ",") + "\n"
	row := func(sec string, v string) string {
		return "2025-01-03," + sec + ",1,1,1,0,100," + strings.TrimRight(strings.Repeat(v+",", 10), ",") + "\n"
	}
	os.WriteFile(block, []byte(hdr+row("A", "1")+row("B", "2")+"2025-01-03,C,1,1,1,0,100,,,,,,,,,,\n"), 0o644)
	if err := merge(base, block, out, mergeOpts{}); err != nil {
		t.Fatal(err)
	}
	_, rows, _ := readCSV(out)
	if len(rows) != 2 {
		t.Fatalf("rows %d want 2", len(rows))
	}
	out2 := filepath.Join(dir, "out2.csv")
	if err := merge(base, block, out2, mergeOpts{shuffleSeed: 7}); err != nil {
		t.Fatal(err)
	}
	h2, rows2, _ := readCSV(out2)
	ix := colIndex(h2)
	got := map[string]bool{}
	for _, r := range rows2 {
		got[r[ix["ap_a2"]]] = true
	}
	if !got["1"] || !got["2"] || len(rows2) != 2 {
		t.Fatalf("shuffle changed the vector multiset: %v", got)
	}
}

func TestMergeDemeanZeroMeanPerDate(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.csv")
	block := filepath.Join(dir, "block.csv")
	out := filepath.Join(dir, "out.csv")
	os.WriteFile(base, []byte("ticker,date,label,label_date,split,f1\nA,2025-01-03,1,2025-01-20,train,0.5\nB,2025-01-03,0,2025-01-20,train,0.6\nC,2025-01-03,1,2025-01-20,train,0.7\nD,2025-01-03,1,2025-01-20,train,0.7\nE,2025-01-03,1,2025-01-20,train,0.7\n"), 0o644)
	hdr := "date,secid,t18,adm100,realtime,div_drop,close," + strings.Join(apNames, ",") + "\n"
	row := func(sec string, v string) string {
		return "2025-01-03," + sec + ",1,1,1,0,100," + strings.TrimRight(strings.Repeat(v+",", 10), ",") + "\n"
	}
	os.WriteFile(block, []byte(hdr+row("A", "1")+row("B", "2")+row("C", "3")+row("D", "4")+row("E", "5")), 0o644)
	if err := merge(base, block, out, mergeOpts{demean: true}); err != nil {
		t.Fatal(err)
	}
	h, rows, _ := readCSV(out)
	ix := colIndex(h)
	sum := 0.0
	for _, r := range rows {
		v, _ := strconv.ParseFloat(r[ix["ap_a2"]], 64)
		sum += v
	}
	if len(rows) != 5 || math.Abs(sum) > 1e-9 {
		t.Fatalf("rows %d sum %v", len(rows), sum)
	}
}
