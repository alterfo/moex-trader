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

func TestLoadReturnsDirsMatrixJoinsVariants(t *testing.T) {
	base := t.TempDir()
	variantA := filepath.Join(base, "variant_a")
	variantB := filepath.Join(base, "variant_b")
	writeWindowReturns(t, variantA, "2025-04-01_2025-06-30", "date,realized_net\n2025-04-01,100\n2025-04-02,-20\n")
	writeWindowReturns(t, variantA, "2025-07-01_2025-09-30", "date,realized_net\n2025-07-01,40\n")
	writeWindowReturns(t, variantB, "2025-04-01_2025-06-30", "date,realized_net\n2025-04-01,50\n2025-04-02,10\n")
	writeWindowReturns(t, variantB, "2025-07-01_2025-09-30", "date,realized_net\n2025-07-01,-30\n")

	matrix, err := loadReturnsDirsMatrix([]string{variantA, variantB})
	if err != nil {
		t.Fatal(err)
	}
	if len(matrix) != 3 {
		t.Fatalf("len(matrix) = %d, want 3 trading dates", len(matrix))
	}
	if len(matrix[0]) != 2 {
		t.Fatalf("len(matrix[0]) = %d, want 2 variants", len(matrix[0]))
	}
	if matrix[0][0] != 100 || matrix[0][1] != 50 {
		t.Fatalf("matrix[0] = %v, want [100 50]", matrix[0])
	}
	if matrix[1][0] != -20 || matrix[1][1] != 10 {
		t.Fatalf("matrix[1] = %v, want [-20 10]", matrix[1])
	}
	if matrix[2][0] != 40 || matrix[2][1] != -30 {
		t.Fatalf("matrix[2] = %v, want [40 -30]", matrix[2])
	}
}

func TestLoadReturnsDirsMatrixRejectsEmptyVariant(t *testing.T) {
	base := t.TempDir()
	empty := filepath.Join(base, "empty")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	populated := filepath.Join(base, "populated")
	writeWindowReturns(t, populated, "2025-04-01_2025-06-30", "date,realized_net\n2025-04-01,100\n")

	if _, err := loadReturnsDirsMatrix([]string{empty, populated}); err == nil {
		t.Fatal("expected error for a variant with no period_returns.csv files")
	}
}

func writeWindowReturns(t *testing.T, parent, windowID, csv string) {
	t.Helper()
	dir := filepath.Join(parent, windowID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "period_returns.csv"), []byte(csv), 0o644); err != nil {
		t.Fatal(err)
	}
}
