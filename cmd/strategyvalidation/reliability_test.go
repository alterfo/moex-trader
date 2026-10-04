package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeReliabilityWindow(t *testing.T, parent, windowID, csv string) {
	t.Helper()
	dir := filepath.Join(parent, windowID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "reliability.csv"), []byte(csv), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadReliabilityPairs(t *testing.T) {
	base := t.TempDir()
	writeReliabilityWindow(t, base, "2025-04-01_2025-06-30", "date,ticker,probability,label\n2025-04-01,SBER,0.7,1\n2025-04-02,GAZP,0.3,0\n")
	writeReliabilityWindow(t, base, "2025-07-01_2025-09-30", "date,ticker,probability,label\n2025-07-01,SBER,0.6,1\n")

	pairs, err := loadReliabilityPairs(base)
	if err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 3 {
		t.Fatalf("len(pairs) = %d, want 3", len(pairs))
	}
	if pairs[0].Probability != 0.7 || pairs[0].Label != 1 {
		t.Fatalf("pairs[0] = %+v", pairs[0])
	}
}

func TestLoadReliabilityPairsRejectsEmpty(t *testing.T) {
	if _, err := loadReliabilityPairs(t.TempDir()); err == nil {
		t.Fatal("loadReliabilityPairs() error = nil, want error for empty directory")
	}
}

func TestLoadReliabilityPairsRejectsMalformedWindow(t *testing.T) {
	base := t.TempDir()
	writeReliabilityWindow(t, base, "2025-04-01_2025-06-30", "date,ticker,probability,label\n2025-04-01,SBER,0.7,1\n")
	writeReliabilityWindow(t, base, "2025-07-01_2025-09-30", "date,ticker,probability\n2025-07-01,SBER,0.6\n")
	if _, err := loadReliabilityPairs(base); err == nil {
		t.Fatal("loadReliabilityPairs() error = nil, want error for malformed window")
	}
}
