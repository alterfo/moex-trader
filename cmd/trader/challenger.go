package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/model"
	"github.com/olegsidorkin/moex-trader/internal/orchestrator"
)

const defaultChallengerLogPath = "artifacts/challenger/decisions.jsonl"

type challengerSide struct {
	Action      string `json:"action,omitempty"`
	Probability string `json:"probability,omitempty"`
	Lots        int    `json:"lots,omitempty"`
	Error       string `json:"error,omitempty"`
}

type challengerRecord struct {
	Time       time.Time      `json:"ts"`
	Ticker     string         `json:"ticker"`
	Price      string         `json:"price"`
	Champion   challengerSide `json:"champion"`
	Challenger challengerSide `json:"challenger"`
}

type challengerSignalSource struct {
	champion   orchestrator.SignalSource
	challenger orchestrator.SignalSource
	file       *os.File
	mu         sync.Mutex
	now        func() time.Time
	logger     *log.Logger
}

func newChallengerSignalSource(champion, challenger orchestrator.SignalSource, logPath string, now func() time.Time, logger *log.Logger) (*challengerSignalSource, error) {
	if logger == nil {
		logger = log.Default()
	}
	if now == nil {
		now = time.Now
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, fmt.Errorf("challenger log dir: %w", err)
	}
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open challenger log: %w", err)
	}
	return &challengerSignalSource{champion: champion, challenger: challenger, file: f, now: now, logger: logger}, nil
}

func (c *challengerSignalSource) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.file.Close()
}

func (c *challengerSignalSource) Generate(ctx context.Context, feature domain.FeatureContext) (domain.TradeSignal, error) {
	signal, err := c.champion.Generate(ctx, feature)
	if err != nil {
		return signal, err
	}
	c.record(ctx, feature, signal)
	return signal, nil
}

func (c *challengerSignalSource) VolScaleEnabled() bool {
	if aware, ok := c.champion.(interface{ VolScaleEnabled() bool }); ok {
		return aware.VolScaleEnabled()
	}
	return false
}

func (c *challengerSignalSource) record(ctx context.Context, feature domain.FeatureContext, champion domain.TradeSignal) {
	rec := challengerRecord{
		Time:     c.now().UTC(),
		Ticker:   feature.Ticker,
		Price:    feature.LastPrice.String(),
		Champion: sideOf(champion, nil),
	}
	other, err := c.safeGenerate(ctx, feature)
	rec.Challenger = sideOf(other, err)
	line, mErr := json.Marshal(rec)
	if mErr != nil {
		c.logger.Printf("trader: challenger marshal %s: %v", feature.Ticker, mErr)
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, wErr := c.file.Write(append(line, '\n')); wErr != nil {
		c.logger.Printf("trader: challenger write %s: %v", feature.Ticker, wErr)
	}
}

func (c *challengerSignalSource) safeGenerate(ctx context.Context, feature domain.FeatureContext) (signal domain.TradeSignal, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("challenger panic: %v", r)
		}
	}()
	return c.challenger.Generate(ctx, feature)
}

func sideOf(signal domain.TradeSignal, err error) challengerSide {
	if err != nil {
		return challengerSide{Error: err.Error()}
	}
	side := challengerSide{Action: string(signal.Action), Lots: signal.TargetLots}
	if signal.Probability.IsPositive() {
		side.Probability = signal.Probability.StringFixed(4)
	}
	return side
}

func newChallengerModelSource(cfg *config.Config) (orchestrator.SignalSource, error) {
	path := cfg.Model.ChallengerEnsemblePath
	if path == "" {
		return nil, nil
	}
	m, err := model.LoadEnsembleModel(path)
	if err != nil {
		return nil, fmt.Errorf("load challenger ensemble model: %w", err)
	}
	return &model.EnsembleSignalSource{
		Model:          m,
		MaxLots:        cfg.Risk.MaxLots,
		TargetNotional: cfg.Risk.TargetNotional,
		VolScale: model.VolScale{
			Enabled: cfg.Risk.VolScale.Enabled,
			MinMult: cfg.Risk.VolScale.MinMult,
			MaxMult: cfg.Risk.VolScale.MaxMult,
		},
	}, nil
}

func challengerLogPath(cfg *config.Config) string {
	if cfg.Model.ChallengerLogPath != "" {
		return cfg.Model.ChallengerLogPath
	}
	return defaultChallengerLogPath
}
