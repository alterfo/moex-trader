package walkforward

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

func day(year int, month int, d int) time.Time {
	return time.Date(year, time.Month(month), d, 0, 0, 0, 0, time.UTC)
}

func testConfig() Config {
	return Config{
		WindowStart:     day(2025, 4, 1),
		WindowEnd:       day(2025, 6, 30),
		SignalSource:    "ensemble",
		ModelPath:       "ensemble_model.json",
		Deposit:         "1000000",
		MaxLots:         1000,
		CommissionRate:  "0.0005",
		SpreadPct:       "0.0005",
		SlippagePct:     "0.0005",
		BorrowPctPerDay: "0.00005",
		TargetNotional:  "15000",
		Tickers:         []string{"SBER", "GAZP"},
		BuyThreshold:    0.60,
		SellThreshold:   0.40,
		FeatureOrder:    []string{"return_pct", "rsi_14"},
	}
}

func testDecision() Decision {
	return Decision{
		ID:          "decision-1",
		Ticker:      "SBER",
		Date:        day(2025, 4, 1),
		Action:      string(domain.ActionBuy),
		Probability: 0.62,
		Confidence:  "0.24",
		TargetLots:  1,
		FeatureHash: "feature-hash-1",
		ConfigID:    "config-hash-1",
	}
}

type fakeSignalSource struct {
	signal domain.TradeSignal
}

func (f fakeSignalSource) Generate(_ context.Context, feature domain.FeatureContext) (domain.TradeSignal, error) {
	signal := f.signal
	if signal.Ticker == "" {
		signal.Ticker = feature.Ticker
	}
	signal.GeneratedAt = feature.GeneratedAt
	return signal, nil
}

type fakeProbability struct {
	p float64
}

func (p fakeProbability) RawProbability(_ domain.FeatureContext) (float64, error) {
	return p.p, nil
}

func TestFeatureHashDeterministicAndSensitive(t *testing.T) {
	base := domain.FeatureContext{
		Ticker:      "SBER",
		ReturnPct:   decimal.NewFromFloat(1.25),
		GeneratedAt: day(2025, 4, 1),
	}
	first := FeatureHash(base)
	second := FeatureHash(base)
	if first != second {
		t.Fatalf("feature hash is not deterministic: %q vs %q", first, second)
	}
	if first == "" {
		t.Fatal("feature hash must not be empty")
	}

	changed := base
	changed.ReturnPct = decimal.NewFromFloat(-1.25)
	if FeatureHash(changed) == first {
		t.Fatal("feature hash should change when the feature vector changes")
	}
}

func TestFeatureHashIgnoresTimestamp(t *testing.T) {
	base := domain.FeatureContext{
		Ticker:    "SBER",
		ReturnPct: decimal.NewFromFloat(1.25),
	}
	a := FeatureHash(base)
	base.GeneratedAt = day(2025, 4, 1)
	b := FeatureHash(base)
	if a != b {
		t.Fatalf("feature hash should depend on the feature vector, not the generated timestamp: %q vs %q", a, b)
	}
}

func TestConfigHashChangesWithConfig(t *testing.T) {
	a := testConfig()
	b := testConfig()
	if ConfigHash(a) != ConfigHash(b) {
		t.Fatal("identical configs should hash identically")
	}
	b.TargetNotional = "20000"
	if ConfigHash(a) == ConfigHash(b) {
		t.Fatal("config hash should change when the config changes")
	}
}

func TestDecisionIDIsStableAndSensitive(t *testing.T) {
	id := DecisionID("SBER", day(2025, 4, 1), "hash", "BUY")
	if id == "" || len(id) != 16 {
		t.Fatalf("unexpected decision ID %q", id)
	}
	if DecisionID("SBER", day(2025, 4, 1), "hash", "BUY") != id {
		t.Fatal("decision ID should be deterministic")
	}
	if DecisionID("SBER", day(2025, 4, 1), "other-hash", "BUY") == id {
		t.Fatal("decision ID should change when the feature hash changes")
	}
}

func TestNewWindowID(t *testing.T) {
	got := NewWindowID(day(2025, 4, 1), day(2025, 6, 30))
	if got != "2025-04-01_2025-06-30" {
		t.Fatalf("window ID = %q", got)
	}
}

func TestRecorderLogsExactProbability(t *testing.T) {
	source := fakeSignalSource{signal: domain.TradeSignal{
		Action:      domain.ActionBuy,
		Confidence:  decimal.NewFromFloat(0.24),
		TargetLots:  2,
		GeneratedAt: day(2025, 4, 1),
	}}
	recorder := NewRecorder(source, fakeProbability{p: 0.62}, "config-hash")
	feature := domain.FeatureContext{Ticker: "SBER", ReturnPct: decimal.NewFromFloat(1), GeneratedAt: day(2025, 4, 1)}

	signal, err := recorder.Generate(context.Background(), feature)
	if err != nil {
		t.Fatal(err)
	}
	if signal.Action != domain.ActionBuy {
		t.Fatalf("action = %s", signal.Action)
	}
	decisions := recorder.Decisions()
	if len(decisions) != 1 {
		t.Fatalf("decisions = %d, want 1", len(decisions))
	}
	if decisions[0].Probability != 0.62 {
		t.Fatalf("probability = %v, want 0.62", decisions[0].Probability)
	}
	if decisions[0].FeatureHash == "" || decisions[0].ConfigID != "config-hash" || decisions[0].ID == "" {
		t.Fatalf("incomplete decision record: %+v", decisions[0])
	}
}

func TestRecorderFallsBackToConfidence(t *testing.T) {
	source := fakeSignalSource{signal: domain.TradeSignal{
		Action:      domain.ActionSell,
		Confidence:  decimal.NewFromFloat(0.2),
		GeneratedAt: day(2025, 4, 1),
	}}
	recorder := NewRecorder(source, nil, "config-hash")
	feature := domain.FeatureContext{Ticker: "SBER", ReturnPct: decimal.NewFromFloat(1), GeneratedAt: day(2025, 4, 1)}

	if _, err := recorder.Generate(context.Background(), feature); err != nil {
		t.Fatal(err)
	}
	decisions := recorder.Decisions()
	if len(decisions) != 1 {
		t.Fatalf("decisions = %d, want 1", len(decisions))
	}
	if decisions[0].Probability != 0.4 {
		t.Fatalf("probability = %v, want 0.4 (0.5 - confidence/2)", decisions[0].Probability)
	}
}

func TestSaveLoadRoundTripWithModel(t *testing.T) {
	modelPath := filepath.Join(t.TempDir(), "model.json")
	modelData := []byte(`{"feature_order":["return_pct","rsi_14"]}`)
	if err := os.WriteFile(modelPath, modelData, 0o644); err != nil {
		t.Fatal(err)
	}

	w := Window{
		ID:         NewWindowID(day(2025, 4, 1), day(2025, 6, 30)),
		Config:     testConfig(),
		Decisions:  []Decision{testDecision()},
		ConfigHash: ConfigHash(testConfig()),
	}
	dir := filepath.Join(t.TempDir(), "2025-04-01_2025-06-30")
	if err := Save(dir, w, modelPath); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != w.ID {
		t.Fatalf("ID = %q, want %q", loaded.ID, w.ID)
	}
	if loaded.ModelSHA256 != HashBytes(modelData) {
		t.Fatal("model SHA256 mismatch")
	}
	if loaded.ModelFile != modelFileName {
		t.Fatalf("model file = %q, want %q", loaded.ModelFile, modelFileName)
	}
	if len(loaded.Decisions) != 1 || loaded.Decisions[0].ID != "decision-1" {
		t.Fatalf("decisions did not round-trip: %+v", loaded.Decisions)
	}
	if _, err := os.Stat(filepath.Join(dir, modelFileName)); err != nil {
		t.Fatalf("model file was not persisted: %v", err)
	}
}

func TestSaveLoadRoundTripWithoutModel(t *testing.T) {
	w := Window{
		ID:         NewWindowID(day(2025, 4, 1), day(2025, 6, 30)),
		Config:     testConfig(),
		ConfigHash: ConfigHash(testConfig()),
	}
	dir := filepath.Join(t.TempDir(), "2025-04-01_2025-06-30")
	if err := Save(dir, w, ""); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ModelSHA256 != "" || loaded.ModelFile != "" {
		t.Fatalf("unexpected model fields: %+v", loaded)
	}
}

func TestSaveRejectsConfigHashMismatch(t *testing.T) {
	w := Window{
		ID:         NewWindowID(day(2025, 4, 1), day(2025, 6, 30)),
		Config:     testConfig(),
		ConfigHash: "wrong-hash",
	}
	if err := Save(filepath.Join(t.TempDir(), "window"), w, ""); err == nil {
		t.Fatal("Save should reject a config hash mismatch")
	}
}

func TestLoadDetectsModelTampering(t *testing.T) {
	modelPath := filepath.Join(t.TempDir(), "model.json")
	if err := os.WriteFile(modelPath, []byte("original-model"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := Window{
		ID:         NewWindowID(day(2025, 4, 1), day(2025, 6, 30)),
		Config:     testConfig(),
		ConfigHash: ConfigHash(testConfig()),
	}
	dir := filepath.Join(t.TempDir(), "2025-04-01_2025-06-30")
	if err := Save(dir, w, modelPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, modelFileName), []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatal("Load should detect model file tampering")
	}
}

func TestLoadRejectsDuplicateDecisionIDs(t *testing.T) {
	d := testDecision()
	w := Window{
		ID:         NewWindowID(day(2025, 4, 1), day(2025, 6, 30)),
		Config:     testConfig(),
		ConfigHash: ConfigHash(testConfig()),
		Decisions:  []Decision{d, d},
		CreatedAt:  time.Now().UTC(),
	}
	if err := w.Validate(); err == nil {
		t.Fatal("Validate should reject duplicate decision IDs")
	}
}
