package oosfreeze

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const MinimumCollectionDays = 183

type Recipe struct {
	ModelSHA256              string   `json:"model_sha256"`
	ConfigSHA256             string   `json:"config_sha256"`
	FeatureOrder             []string `json:"feature_order"`
	BuyThreshold             float64  `json:"buy_threshold"`
	SellThreshold            float64  `json:"sell_threshold"`
	Tickers                  []string `json:"tickers"`
	EnsemblePath             string   `json:"ensemble_path"`
	TargetNotional           string   `json:"target_notional"`
	CommissionRate           string   `json:"commission_rate"`
	MaxSlippagePct           string   `json:"max_slippage_pct"`
	RebalanceMinDeviationPct string   `json:"rebalance_min_deviation_pct"`
	NoTradeAfterOpenMinutes  int      `json:"no_trade_after_open_minutes"`
	BlackoutWindows          []string `json:"blackout_windows"`
	PollInterval             string   `json:"poll_interval"`
	OrderType                string   `json:"order_type"`
	Broker                   string   `json:"broker"`
	IsPaperTrading           bool     `json:"is_paper_trading"`
}

type Manifest struct {
	FrozenAt         time.Time `json:"frozen_at"`
	CodeSHA          string    `json:"code_sha"`
	RecipeHash       string    `json:"recipe_hash"`
	Recipe           Recipe    `json:"recipe"`
	RealMoneyEnabled bool      `json:"real_money_enabled"`
}

type CheckResult struct {
	CheckedAt         time.Time `json:"checked_at"`
	Changed           bool      `json:"changed"`
	CollectionDays    int       `json:"collection_days"`
	RequiredDays      int       `json:"required_days"`
	Complete          bool      `json:"complete"`
	CurrentRecipeHash string    `json:"current_recipe_hash"`
}

func HashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func HashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %q: %w", path, err)
	}
	return HashBytes(data), nil
}

func HashRecipe(recipe Recipe) string {
	canonical := recipe
	canonical.FeatureOrder = append([]string(nil), recipe.FeatureOrder...)
	canonical.Tickers = sortedCopy(recipe.Tickers)
	canonical.BlackoutWindows = sortedCopy(recipe.BlackoutWindows)
	data, err := json.Marshal(canonical)
	if err != nil {
		panic(err)
	}
	return HashBytes(data)
}

func NewManifest(frozenAt time.Time, codeSHA string, recipe Recipe) Manifest {
	return Manifest{
		FrozenAt:         frozenAt.UTC(),
		CodeSHA:          strings.TrimSpace(codeSHA),
		RecipeHash:       HashRecipe(recipe),
		Recipe:           recipe,
		RealMoneyEnabled: false,
	}
}

func (m Manifest) Validate() error {
	if strings.TrimSpace(m.CodeSHA) == "" {
		return fmt.Errorf("oos freeze: code SHA is required")
	}
	if strings.TrimSpace(m.RecipeHash) == "" {
		return fmt.Errorf("oos freeze: recipe hash is required")
	}
	if m.FrozenAt.IsZero() {
		return fmt.Errorf("oos freeze: frozen_at is required")
	}
	if m.RealMoneyEnabled {
		return fmt.Errorf("oos freeze: real-money execution must remain disabled during collection")
	}
	if strings.TrimSpace(m.Recipe.ModelSHA256) == "" {
		return fmt.Errorf("oos freeze: model SHA256 is required")
	}
	if strings.TrimSpace(m.Recipe.ConfigSHA256) == "" {
		return fmt.Errorf("oos freeze: config SHA256 is required")
	}
	if HashRecipe(m.Recipe) != m.RecipeHash {
		return fmt.Errorf("oos freeze: recipe hash does not match recipe")
	}
	return nil
}

func WriteManifest(path string, manifest Manifest) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("oos freeze: marshal manifest: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("oos freeze: create manifest directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("oos freeze: write manifest: %w", err)
	}
	return nil
}

func LoadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("oos freeze: read manifest %q: %w", path, err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("oos freeze: parse manifest %q: %w", path, err)
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, fmt.Errorf("oos freeze: %w", err)
	}
	return manifest, nil
}

func Check(manifest Manifest, current Recipe, now time.Time) CheckResult {
	now = now.UTC()
	currentHash := HashRecipe(current)
	changed := currentHash != manifest.RecipeHash
	days := 0
	if !changed && now.After(manifest.FrozenAt) {
		days = int(now.Sub(manifest.FrozenAt).Hours() / 24)
	}
	return CheckResult{
		CheckedAt:         now,
		Changed:           changed,
		CollectionDays:    days,
		RequiredDays:      MinimumCollectionDays,
		Complete:          !changed && days >= MinimumCollectionDays,
		CurrentRecipeHash: currentHash,
	}
}

func sortedCopy(values []string) []string {
	out := make([]string, len(values))
	copy(out, values)
	sort.Strings(out)
	return out
}
