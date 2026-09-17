package oosfreeze

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testRecipe() Recipe {
	return Recipe{
		ModelSHA256:              "model-hash",
		ConfigSHA256:             "config-hash",
		FeatureOrder:             []string{"return_pct", "rsi_14"},
		BuyThreshold:             0.60,
		SellThreshold:            0.40,
		Tickers:                  []string{"SBER", "GAZP"},
		EnsemblePath:             "ensemble_model.json",
		TargetNotional:           "15000",
		CommissionRate:           "0.0005",
		MaxSlippagePct:           "0.003",
		RebalanceMinDeviationPct: "0.05",
		NoTradeAfterOpenMinutes:  15,
		BlackoutWindows:          []string{},
		PollInterval:             "5m0s",
		OrderType:                "limit",
		Broker:                   "tinkoff",
		IsPaperTrading:           false,
	}
}

func TestHashRecipeIgnoresTickerOrder(t *testing.T) {
	a := testRecipe()
	a.Tickers = []string{"SBER", "GAZP", "LKOH"}
	b := testRecipe()
	b.Tickers = []string{"LKOH", "GAZP", "SBER"}
	if HashRecipe(a) != HashRecipe(b) {
		t.Fatal("recipe hash should be order-insensitive for tickers")
	}
}

func TestHashRecipeDetectsModelChange(t *testing.T) {
	a := testRecipe()
	b := testRecipe()
	b.ModelSHA256 = "changed-model-hash"
	if HashRecipe(a) == HashRecipe(b) {
		t.Fatal("recipe hash should change when the model hash changes")
	}
}

func TestNewManifestDisablesRealMoney(t *testing.T) {
	manifest := NewManifest(day(2026, 9, 17), "abc123", testRecipe())
	if manifest.RealMoneyEnabled {
		t.Fatal("real-money execution must be disabled")
	}
	if manifest.RecipeHash == "" || manifest.CodeSHA != "abc123" {
		t.Fatalf("unexpected manifest: %+v", manifest)
	}
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestManifestRoundTrip(t *testing.T) {
	manifest := NewManifest(day(2026, 9, 17), "abc123", testRecipe())
	path := filepath.Join(t.TempDir(), "freeze.json")
	if err := WriteManifest(path, manifest); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RecipeHash != manifest.RecipeHash || loaded.CodeSHA != manifest.CodeSHA {
		t.Fatalf("round-trip mismatch: %+v", loaded)
	}
}

func TestLoadManifestRejectsRealMoney(t *testing.T) {
	manifest := NewManifest(day(2026, 9, 17), "abc123", testRecipe())
	manifest.RealMoneyEnabled = true
	manifest.RecipeHash = HashRecipe(manifest.Recipe)
	if err := manifest.Validate(); err == nil {
		t.Fatal("expected validation error for real-money execution")
	}
	path := filepath.Join(t.TempDir(), "freeze.json")
	if err := WriteManifest(path, manifest); err == nil {
		t.Fatal("expected write to fail for real-money execution")
	}
}

func TestCheckUnchangedCountsDays(t *testing.T) {
	recipe := testRecipe()
	manifest := NewManifest(day(2026, 1, 1), "abc123", recipe)
	check := Check(manifest, recipe, day(2026, 9, 17))
	if check.Changed {
		t.Fatal("unchanged recipe should not report a change")
	}
	if check.CollectionDays <= 0 || check.RequiredDays != MinimumCollectionDays {
		t.Fatalf("unexpected collection days: %+v", check)
	}
	if !check.Complete {
		t.Fatal("183-day window should be complete by September")
	}
}

func TestCheckChangedResetsCount(t *testing.T) {
	manifest := NewManifest(day(2026, 1, 1), "abc123", testRecipe())
	changed := testRecipe()
	changed.ModelSHA256 = "changed-model-hash"
	check := Check(manifest, changed, day(2026, 9, 17))
	if !check.Changed {
		t.Fatal("changed recipe should be detected")
	}
	if check.CollectionDays != 0 || check.Complete {
		t.Fatalf("changed recipe must reset the OOS count: %+v", check)
	}
}

func TestCheckClampsNegativeElapsed(t *testing.T) {
	recipe := testRecipe()
	manifest := NewManifest(day(2026, 9, 17), "abc123", recipe)
	check := Check(manifest, recipe, day(2026, 9, 1))
	if check.CollectionDays != 0 {
		t.Fatalf("negative elapsed time should clamp to zero, got %d", check.CollectionDays)
	}
}

func TestLoadManifestRejectsCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "freeze.json")
	if err := os.WriteFile(path, []byte("not-json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(path); err == nil {
		t.Fatal("expected error for corrupt manifest")
	}
}

func day(year int, month int, d int) time.Time {
	return time.Date(year, time.Month(month), d, 0, 0, 0, 0, time.UTC)
}
