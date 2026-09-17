package walkforward

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/model"
)

const (
	windowFileName = "window.json"
	modelFileName  = "model.json"
)

type Config struct {
	WindowStart     time.Time `json:"window_start"`
	WindowEnd       time.Time `json:"window_end"`
	SignalSource    string    `json:"signal_source"`
	ModelPath       string    `json:"model_path,omitempty"`
	Deposit         string    `json:"deposit"`
	MaxLots         int       `json:"max_lots"`
	CommissionRate  string    `json:"commission_rate"`
	SpreadPct       string    `json:"spread_pct"`
	SlippagePct     string    `json:"slippage_pct"`
	BorrowPctPerDay string    `json:"borrow_pct_per_day"`
	TargetNotional  string    `json:"target_notional,omitempty"`
	Tickers         []string  `json:"tickers"`
	BuyThreshold    float64   `json:"buy_threshold"`
	SellThreshold   float64   `json:"sell_threshold"`
	FeatureOrder    []string  `json:"feature_order"`
	SpreadMinObs    int       `json:"spread_min_obs"`
	SpreadDBPath    string    `json:"spread_db_path,omitempty"`
}

type Decision struct {
	ID          string    `json:"id"`
	Ticker      string    `json:"ticker"`
	Date        time.Time `json:"date"`
	Action      string    `json:"action"`
	Probability float64   `json:"probability"`
	Confidence  string    `json:"confidence"`
	TargetLots  int       `json:"target_lots"`
	FeatureHash string    `json:"feature_hash"`
	ConfigID    string    `json:"config_id"`
}

type Window struct {
	ID          string     `json:"id"`
	Config      Config     `json:"config"`
	ConfigHash  string     `json:"config_hash"`
	ModelSHA256 string     `json:"model_sha256,omitempty"`
	ModelFile   string     `json:"model_file,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	Decisions   []Decision `json:"decisions"`
}

func NewWindowID(start, end time.Time) string {
	return start.Format("2006-01-02") + "_" + end.Format("2006-01-02")
}

func ConfigHash(cfg Config) string {
	return HashJSON(cfg)
}

func HashJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func HashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func FeatureHash(feature domain.FeatureContext) string {
	vector, order := model.ToVector(feature)
	var builder strings.Builder
	builder.WriteString(strings.ToUpper(strings.TrimSpace(feature.Ticker)))
	builder.WriteByte('|')
	for i, name := range order {
		if i > 0 {
			builder.WriteByte(';')
		}
		builder.WriteString(name)
		builder.WriteByte('=')
		builder.WriteString(strconv.FormatFloat(vector[i], 'g', -1, 64))
	}
	return HashBytes([]byte(builder.String()))
}

func DecisionID(ticker string, date time.Time, featureHash, action string) string {
	payload := strings.ToUpper(strings.TrimSpace(ticker)) + "|" + date.UTC().Format(time.RFC3339) + "|" + featureHash + "|" + action
	return HashBytes([]byte(payload))[:16]
}

type ProbabilityProvider interface {
	RawProbability(feature domain.FeatureContext) (float64, error)
}

type Recorder struct {
	inner       backtest.SignalSource
	probability ProbabilityProvider
	configID    string
	mu          sync.Mutex
	decisions   []Decision
}

func NewRecorder(inner backtest.SignalSource, probability ProbabilityProvider, configID string) *Recorder {
	return &Recorder{inner: inner, probability: probability, configID: configID}
}

func (r *Recorder) Generate(ctx context.Context, feature domain.FeatureContext) (domain.TradeSignal, error) {
	signal, err := r.inner.Generate(ctx, feature)
	if err != nil {
		return signal, err
	}
	hash := FeatureHash(feature)
	decision := Decision{
		ID:          DecisionID(feature.Ticker, feature.GeneratedAt, hash, string(signal.Action)),
		Ticker:      feature.Ticker,
		Date:        feature.GeneratedAt,
		Action:      string(signal.Action),
		Probability: r.rawProbability(feature, signal),
		Confidence:  signal.Confidence.String(),
		TargetLots:  signal.TargetLots,
		FeatureHash: hash,
		ConfigID:    r.configID,
	}
	r.mu.Lock()
	r.decisions = append(r.decisions, decision)
	r.mu.Unlock()
	return signal, nil
}

func (r *Recorder) rawProbability(feature domain.FeatureContext, signal domain.TradeSignal) float64 {
	if r.probability != nil {
		if p, err := r.probability.RawProbability(feature); err == nil && !math.IsNaN(p) && !math.IsInf(p, 0) {
			return p
		}
	}
	return confidenceProbability(signal)
}

func confidenceProbability(signal domain.TradeSignal) float64 {
	confidence, _ := signal.Confidence.Float64()
	switch signal.Action {
	case domain.ActionBuy:
		return 0.5 + confidence/2
	case domain.ActionSell:
		return 0.5 - confidence/2
	default:
		return 0.5
	}
}

func (r *Recorder) Decisions() []Decision {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Decision, len(r.decisions))
	copy(out, r.decisions)
	return out
}

func Save(dir string, w Window, modelPath string) error {
	w.CreatedAt = time.Now().UTC()
	if strings.TrimSpace(w.ID) == "" {
		return fmt.Errorf("walk-forward: window ID is required")
	}
	if w.ConfigHash != "" && w.ConfigHash != ConfigHash(w.Config) {
		return fmt.Errorf("walk-forward: config hash does not match config")
	}
	w.ConfigHash = ConfigHash(w.Config)
	if modelPath != "" {
		data, err := os.ReadFile(modelPath)
		if err != nil {
			return fmt.Errorf("walk-forward: read model %q: %w", modelPath, err)
		}
		w.ModelSHA256 = HashBytes(data)
		w.ModelFile = modelFileName
	} else {
		w.ModelSHA256 = ""
		w.ModelFile = ""
	}
	if err := w.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("walk-forward: create window dir: %w", err)
	}
	if modelPath != "" {
		if err := copyFile(modelPath, filepath.Join(dir, modelFileName)); err != nil {
			return err
		}
	}
	payload, err := json.MarshalIndent(w, "", "  ")
	if err != nil {
		return fmt.Errorf("walk-forward: marshal window: %w", err)
	}
	payload = append(payload, '\n')
	return writeFileAtomic(filepath.Join(dir, windowFileName), payload)
}

func Load(dir string) (Window, error) {
	data, err := os.ReadFile(filepath.Join(dir, windowFileName))
	if err != nil {
		return Window{}, fmt.Errorf("walk-forward: read window %q: %w", dir, err)
	}
	var w Window
	if err := json.Unmarshal(data, &w); err != nil {
		return Window{}, fmt.Errorf("walk-forward: parse window %q: %w", dir, err)
	}
	if err := w.Validate(); err != nil {
		return Window{}, fmt.Errorf("walk-forward: %w", err)
	}
	if w.ModelFile != "" {
		modelData, err := os.ReadFile(filepath.Join(dir, w.ModelFile))
		if err != nil {
			return Window{}, fmt.Errorf("walk-forward: read model %q: %w", filepath.Join(dir, w.ModelFile), err)
		}
		if HashBytes(modelData) != w.ModelSHA256 {
			return Window{}, fmt.Errorf("walk-forward: model SHA256 mismatch in %q", dir)
		}
	}
	return w, nil
}

func (w Window) Validate() error {
	if strings.TrimSpace(w.ID) == "" {
		return fmt.Errorf("walk-forward: window ID is required")
	}
	if w.CreatedAt.IsZero() {
		return fmt.Errorf("walk-forward: created_at is required")
	}
	if strings.TrimSpace(w.ConfigHash) == "" {
		return fmt.Errorf("walk-forward: config hash is required")
	}
	if w.ConfigHash != ConfigHash(w.Config) {
		return fmt.Errorf("walk-forward: config hash does not match config")
	}
	if strings.TrimSpace(w.Config.SignalSource) == "" {
		return fmt.Errorf("walk-forward: signal source is required")
	}
	if len(w.Config.Tickers) == 0 {
		return fmt.Errorf("walk-forward: tickers must not be empty")
	}
	if w.ModelFile == "" && w.ModelSHA256 != "" {
		return fmt.Errorf("walk-forward: model file is required when a SHA256 is present")
	}
	if w.ModelFile != "" && w.ModelSHA256 == "" {
		return fmt.Errorf("walk-forward: model SHA256 is required when a model file is present")
	}
	seen := make(map[string]struct{}, len(w.Decisions))
	for _, d := range w.Decisions {
		if err := d.validate(); err != nil {
			return err
		}
		if _, ok := seen[d.ID]; ok {
			return fmt.Errorf("walk-forward: duplicate decision ID %q", d.ID)
		}
		seen[d.ID] = struct{}{}
	}
	return nil
}

func (d Decision) validate() error {
	if strings.TrimSpace(d.ID) == "" {
		return fmt.Errorf("walk-forward: decision ID is required")
	}
	if strings.TrimSpace(d.Ticker) == "" {
		return fmt.Errorf("walk-forward: decision ticker is required")
	}
	if d.Date.IsZero() {
		return fmt.Errorf("walk-forward: decision date is required")
	}
	if !domain.Action(d.Action).IsValid() {
		return fmt.Errorf("walk-forward: invalid decision action %q", d.Action)
	}
	if d.Probability < 0 || d.Probability > 1 {
		return fmt.Errorf("walk-forward: decision probability out of range: %v", d.Probability)
	}
	if strings.TrimSpace(d.FeatureHash) == "" {
		return fmt.Errorf("walk-forward: decision feature hash is required")
	}
	if strings.TrimSpace(d.ConfigID) == "" {
		return fmt.Errorf("walk-forward: decision config ID is required")
	}
	return nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("walk-forward: read model %q: %w", src, err)
	}
	return writeFileAtomic(dst, data)
}

func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("walk-forward: write %q: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("walk-forward: replace %q: %w", path, err)
	}
	return nil
}
