package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/olegsidorkin/moex-trader/internal/strategyvalidation"
)

func TestLoadMatricesInto_MergesArchive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "matrices.json")
	if err := os.WriteFile(path, []byte(`{"walkforward_candidates": [[0.01, 0.02], [0.02, 0.01], [0.015, 0.005], [0.005, 0.015]]}`), 0o644); err != nil {
		t.Fatalf("write matrices file: %v", err)
	}
	registry := strategyvalidation.DefaultRegistry()
	if registry.PBOComputable() {
		t.Fatal("expected default registry to have no archived matrices")
	}
	if err := loadMatricesInto(&registry, path); err != nil {
		t.Fatalf("loadMatricesInto() error = %v", err)
	}
	if !registry.PBOComputable() {
		t.Fatal("expected registry to be PBO-computable after loading matrices")
	}
	matrix, ok := registry.Matrices["walkforward_candidates"]
	if !ok || len(matrix) != 4 {
		t.Fatalf("matrix = %+v, ok=%v, want 4 rows", matrix, ok)
	}
}

func TestLoadMatricesInto_RejectsEmptyArchive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "matrices.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write matrices file: %v", err)
	}
	registry := strategyvalidation.DefaultRegistry()
	if err := loadMatricesInto(&registry, path); err == nil {
		t.Fatal("expected error for empty archive")
	}
}

func TestLoadMatricesInto_MissingFile(t *testing.T) {
	registry := strategyvalidation.DefaultRegistry()
	if err := loadMatricesInto(&registry, filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("expected error for missing matrices file")
	}
}
