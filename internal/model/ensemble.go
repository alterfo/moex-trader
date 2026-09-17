package model

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/orchestrator"
)

var _ orchestrator.SignalSource = (*EnsembleSignalSource)(nil)
var _ backtest.SignalSource = (*EnsembleSignalSource)(nil)

type treeNode struct {
	SplitIndices    []int     `json:"si"`
	SplitConditions []float64 `json:"sc"`
	LeftChildren    []int     `json:"lc"`
	RightChildren   []int     `json:"rc"`
	DefaultLeft     []int     `json:"dl"`
}

type logisticWeights struct {
	Mean []float64 `json:"mean"`
	Std  []float64 `json:"std"`
	Coef []float64 `json:"coef"`
	Bias float64   `json:"bias"`
}

type EnsembleModel struct {
	FeatureOrder  []string        `json:"feature_order"`
	BuyThreshold  float64         `json:"buy_threshold"`
	SellThreshold float64         `json:"sell_threshold"`
	LGBBaseLogit  float64         `json:"lgb_base_logit"`
	XGBBaseLogit  float64         `json:"xgb_base_logit"`
	Logistic      logisticWeights `json:"logistic"`
	LGBTrees      []treeNode      `json:"lgb_trees"`
	XGBTrees      []treeNode      `json:"xgb_trees"`
}

func LoadEnsembleModel(path string) (*EnsembleModel, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ensemble: read %q: %w", path, err)
	}
	var m EnsembleModel
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("ensemble: parse %q: %w", path, err)
	}
	if err := m.validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

func (m *EnsembleModel) validate() error {
	if len(m.FeatureOrder) == 0 {
		return fmt.Errorf("ensemble: feature_order is empty")
	}
	if m.BuyThreshold < 0 || m.BuyThreshold > 1 || m.SellThreshold < 0 || m.SellThreshold > 1 {
		return fmt.Errorf("ensemble: thresholds out of range: buy=%v sell=%v", m.BuyThreshold, m.SellThreshold)
	}
	if len(m.Logistic.Coef) != len(m.FeatureOrder) || len(m.Logistic.Mean) != len(m.FeatureOrder) || len(m.Logistic.Std) != len(m.FeatureOrder) {
		return fmt.Errorf("ensemble: logistic weights length mismatch: coef=%d mean=%d std=%d features=%d",
			len(m.Logistic.Coef), len(m.Logistic.Mean), len(m.Logistic.Std), len(m.FeatureOrder))
	}
	for _, trees := range [][]treeNode{m.LGBTrees, m.XGBTrees} {
		for i, t := range trees {
			if err := validateTree(&t, len(m.FeatureOrder)); err != nil {
				return fmt.Errorf("ensemble: tree %d: %w", i, err)
			}
		}
	}
	return nil
}

func validateTree(t *treeNode, features int) error {
	n := len(t.SplitIndices)
	if n == 0 {
		return fmt.Errorf("empty tree")
	}
	if len(t.SplitConditions) != n || len(t.LeftChildren) != n || len(t.RightChildren) != n || len(t.DefaultLeft) != n {
		return fmt.Errorf("array length mismatch: si=%d sc=%d lc=%d rc=%d dl=%d",
			n, len(t.SplitConditions), len(t.LeftChildren), len(t.RightChildren), len(t.DefaultLeft))
	}
	for node := 0; node < n; node++ {
		if t.LeftChildren[node] == -1 {
			if t.RightChildren[node] != -1 {
				return fmt.Errorf("leaf node %d has right child %d", node, t.RightChildren[node])
			}
			continue
		}
		if t.LeftChildren[node] < 0 || t.LeftChildren[node] >= n || t.RightChildren[node] < 0 || t.RightChildren[node] >= n {
			return fmt.Errorf("node %d child index out of range: left=%d right=%d", node, t.LeftChildren[node], t.RightChildren[node])
		}
		if t.SplitIndices[node] < 0 || t.SplitIndices[node] >= features {
			return fmt.Errorf("node %d split index %d out of range [0,%d)", node, t.SplitIndices[node], features)
		}
	}
	return nil
}

type EnsembleSignalSource struct {
	Model          *EnsembleModel
	MaxLots        int
	TargetNotional decimal.Decimal
}

func (s *EnsembleSignalSource) Generate(ctx context.Context, feature domain.FeatureContext) (domain.TradeSignal, error) {
	probability, err := s.RawProbability(feature)
	if err != nil {
		return domain.TradeSignal{}, err
	}

	signal := domain.TradeSignal{
		Ticker:      feature.Ticker,
		Action:      domain.ActionHold,
		Confidence:  decimal.NewFromFloat(math.Abs(probability-0.5) * 2),
		TargetLots:  0,
		Reasoning:   fmt.Sprintf("ensemble:p=%.4f", probability),
		GeneratedAt: feature.GeneratedAt,
	}

	switch {
	case probability >= s.Model.BuyThreshold:
		signal.Action = domain.ActionBuy
		signal.TargetLots = s.targetLots(feature)
	case probability <= s.Model.SellThreshold:
		signal.Action = domain.ActionSell
		signal.TargetLots = s.targetLots(feature)
	default:
		signal.HoldReason = domain.HoldReasonModel
	}

	return signal, nil
}

func (s *EnsembleSignalSource) RawProbability(feature domain.FeatureContext) (float64, error) {
	if s == nil || s.Model == nil {
		return 0, fmt.Errorf("ensemble: model is required")
	}
	if strings.TrimSpace(feature.Ticker) == "" {
		return 0, fmt.Errorf("ensemble: feature ticker must not be empty")
	}
	vector, names := ToVector(feature)
	if err := checkFeatureOrder(s.Model.FeatureOrder, names); err != nil {
		return 0, err
	}
	if len(vector) != len(names) {
		return 0, fmt.Errorf("ensemble: feature vector length %d does not match order %d", len(vector), len(names))
	}

	probability := s.Model.Probability(vector)
	if math.IsNaN(probability) || math.IsInf(probability, 0) {
		return 0, fmt.Errorf("ensemble: computed probability is not finite for %s", feature.Ticker)
	}
	return probability, nil
}

func (s *EnsembleSignalSource) targetLots(feature domain.FeatureContext) int {
	if !s.TargetNotional.IsPositive() {
		return s.MaxLots
	}
	if !feature.LastPrice.IsPositive() {
		return s.MaxLots
	}
	perUnit := feature.LastPrice
	if feature.LotSize.IsPositive() {
		perUnit = feature.LastPrice.Mul(feature.LotSize)
	}
	lots := s.TargetNotional.Div(perUnit).Round(0).IntPart()
	if lots < 1 {
		return 1
	}
	if lots > math.MaxInt32 {
		return math.MaxInt32
	}
	return int(lots)
}

func (m *EnsembleModel) Probability(vector []float64) float64 {
	pLGB := probabilityFromFloat64Trees(m.LGBTrees, m.LGBBaseLogit, vector)
	pXGB := probabilityFromFloat32Trees(m.XGBTrees, m.XGBBaseLogit, vector)
	pLogReg := m.logisticProbability(vector)
	return (pLGB + pXGB + pLogReg) / 3.0
}

func probabilityFromFloat32Trees(trees []treeNode, baseLogit float64, input []float64) float64 {
	margin := baseLogit
	for i := range trees {
		margin += predictFloat32Tree(&trees[i], input)
	}
	return sigmoid(margin)
}

func probabilityFromFloat64Trees(trees []treeNode, baseLogit float64, input []float64) float64 {
	margin := baseLogit
	for i := range trees {
		margin += predictFloat64Tree(&trees[i], input)
	}
	return sigmoid(margin)
}

func predictFloat32Tree(t *treeNode, input []float64) float64 {
	node := 0
	for {
		if t.LeftChildren[node] == -1 {
			return t.SplitConditions[node]
		}
		feature := float32(input[t.SplitIndices[node]])
		if feature < float32(t.SplitConditions[node]) {
			node = t.LeftChildren[node]
		} else {
			node = t.RightChildren[node]
		}
	}
}

func predictFloat64Tree(t *treeNode, input []float64) float64 {
	node := 0
	for {
		if t.LeftChildren[node] == -1 {
			return t.SplitConditions[node]
		}
		if input[t.SplitIndices[node]] < t.SplitConditions[node] {
			node = t.LeftChildren[node]
		} else {
			node = t.RightChildren[node]
		}
	}
}

func (m *EnsembleModel) logisticProbability(input []float64) float64 {
	logit := m.Logistic.Bias
	for i := range input {
		std := m.Logistic.Std[i]
		if std == 0 {
			std = 1
		}
		logit += m.Logistic.Coef[i] * (input[i] - m.Logistic.Mean[i]) / std
	}
	return sigmoid(logit)
}

func checkFeatureOrder(want, got []string) error {
	if len(want) != len(got) {
		return fmt.Errorf("ensemble: feature order has %d fields, want %d", len(got), len(want))
	}
	for i := range want {
		if want[i] != got[i] {
			return fmt.Errorf("ensemble: feature order mismatch at index %d: got %q, want %q", i, got[i], want[i])
		}
	}
	return nil
}
