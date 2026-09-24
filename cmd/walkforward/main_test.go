package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/olegsidorkin/moex-trader/internal/strategyvalidation"
)

func writeCandidatesFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "candidates.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write candidates file: %v", err)
	}
	return path
}

func TestLoadCandidates_Valid(t *testing.T) {
	path := writeCandidatesFile(t, `[
		{"name": "c1", "buy_threshold": 0.6, "sell_threshold": 0.4, "horizon_days": 5, "deadband_pct": 0.5},
		{"name": "c2", "buy_threshold": 0.55, "sell_threshold": 0.45, "horizon_days": 10, "deadband_pct": 0}
	]`)
	candidates, err := loadCandidates(path)
	if err != nil {
		t.Fatalf("loadCandidates() error = %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("len(candidates) = %d, want 2", len(candidates))
	}
	if candidates[0].Name != "c1" || candidates[1].Name != "c2" {
		t.Fatalf("unexpected candidate names: %+v", candidates)
	}
}

func TestLoadCandidates_RejectsEmptyFile(t *testing.T) {
	path := writeCandidatesFile(t, `[]`)
	if _, err := loadCandidates(path); err == nil {
		t.Fatal("expected error for empty candidates list")
	}
}

func TestLoadCandidates_RejectsDuplicateNames(t *testing.T) {
	path := writeCandidatesFile(t, `[
		{"name": "dup", "buy_threshold": 0.6, "sell_threshold": 0.4, "horizon_days": 5},
		{"name": "dup", "buy_threshold": 0.55, "sell_threshold": 0.45, "horizon_days": 10}
	]`)
	if _, err := loadCandidates(path); err == nil {
		t.Fatal("expected error for duplicate candidate names")
	}
}

func TestLoadCandidates_RejectsInvalidThresholds(t *testing.T) {
	cases := []string{
		`[{"name": "a", "buy_threshold": 0, "sell_threshold": 0.4, "horizon_days": 5}]`,
		`[{"name": "a", "buy_threshold": 0.6, "sell_threshold": 0, "horizon_days": 5}]`,
		`[{"name": "a", "buy_threshold": 0.4, "sell_threshold": 0.6, "horizon_days": 5}]`,
		`[{"name": "a", "buy_threshold": 0.6, "sell_threshold": 0.4, "horizon_days": 0}]`,
		`[{"name": "a", "buy_threshold": 0.6, "sell_threshold": 0.4, "horizon_days": 5, "deadband_pct": -1}]`,
	}
	for i, c := range cases {
		path := writeCandidatesFile(t, c)
		if _, err := loadCandidates(path); err == nil {
			t.Fatalf("case %d: expected validation error for %s", i, c)
		}
	}
}

func TestLoadCandidates_MissingFile(t *testing.T) {
	if _, err := loadCandidates(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("expected error for missing candidates file")
	}
}

func TestDefaultPBOSplitsUsedByCLI(t *testing.T) {
	if got := strategyvalidation.DefaultSplits(40); got != 16 {
		t.Fatalf("strategyvalidation.DefaultSplits(40) = %d, want 16", got)
	}
}

func TestSignificanceSuffix(t *testing.T) {
	if significanceSuffix(true) != "" {
		t.Fatalf("significanceSuffix(true) = %q, want empty", significanceSuffix(true))
	}
	if significanceSuffix(false) == "" {
		t.Fatal("significanceSuffix(false) should not be empty")
	}
}

func TestSplitComma(t *testing.T) {
	got := splitComma(" SBER, GAZP ,,OZON")
	want := []string{"SBER", "GAZP", "OZON"}
	if len(got) != len(want) {
		t.Fatalf("splitComma() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("splitComma()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
