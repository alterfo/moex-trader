package model

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type TrainingMetadata struct {
	TrainFrom         time.Time `json:"train_from"`
	TrainTill         time.Time `json:"train_till"`
	ValFrom           time.Time `json:"val_from"`
	ValTill           time.Time `json:"val_till"`
	Tickers           []string  `json:"tickers"`
	TrainSamples      int       `json:"train_samples"`
	ValSamples        int       `json:"val_samples"`
	TrainAccuracy     float64   `json:"train_accuracy"`
	ValAccuracy       float64   `json:"val_accuracy"`
	ValSharpe         float64   `json:"val_sharpe"`
	ValHitRate        float64   `json:"val_hit_rate"`
	ValMaxDrawdownPct float64   `json:"val_max_drawdown_pct"`
}

type Weights struct {
	FeatureOrder  []string         `json:"feature_order"`
	Mean          []float64        `json:"mean"`
	Std           []float64        `json:"std"`
	Coef          []float64        `json:"coef"`
	Bias          float64          `json:"bias"`
	BuyThreshold  float64          `json:"buy_threshold"`
	SellThreshold float64          `json:"sell_threshold"`
	HorizonDays   int              `json:"horizon_days"`
	DeadbandPct   float64          `json:"deadband_pct"`
	TrainedAt     time.Time        `json:"trained_at"`
	Training      TrainingMetadata `json:"training"`
}

func (w *Weights) Save(path string) error {
	if err := w.validate(); err != nil {
		return fmt.Errorf("model: validate weights before save: %w", err)
	}
	data, err := json.MarshalIndent(w, "", "  ")
	if err != nil {
		return fmt.Errorf("model: marshal weights: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("model: create dir for %q: %w", path, err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("model: write weights %q: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("model: replace weights %q: %w", path, err)
	}
	return nil
}

func LoadWeights(path string) (*Weights, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("model: read weights %q: %w", path, err)
	}
	var w Weights
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, fmt.Errorf("model: parse weights %q: %w", path, err)
	}
	if err := w.validate(); err != nil {
		return nil, fmt.Errorf("model: invalid weights %q: %w", path, err)
	}
	return &w, nil
}

func (w *Weights) validate() error {
	if len(w.Coef) != len(w.FeatureOrder) || len(w.Coef) != len(w.Mean) || len(w.Coef) != len(w.Std) {
		return fmt.Errorf(
			"dimension mismatch: coef=%d feature_order=%d mean=%d std=%d",
			len(w.Coef), len(w.FeatureOrder), len(w.Mean), len(w.Std),
		)
	}
	if w.BuyThreshold <= w.SellThreshold {
		return fmt.Errorf("buy_threshold %v must be greater than sell_threshold %v", w.BuyThreshold, w.SellThreshold)
	}
	if w.BuyThreshold <= 0 || w.BuyThreshold >= 1 || w.SellThreshold <= 0 || w.SellThreshold >= 1 {
		return fmt.Errorf("thresholds must be in (0,1), got buy=%v sell=%v", w.BuyThreshold, w.SellThreshold)
	}
	return nil
}
