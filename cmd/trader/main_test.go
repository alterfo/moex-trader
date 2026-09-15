package main

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/executor"
	"github.com/olegsidorkin/moex-trader/internal/features"
	"github.com/olegsidorkin/moex-trader/internal/model"
	"github.com/olegsidorkin/moex-trader/internal/orchestrator"
	"github.com/olegsidorkin/moex-trader/internal/risk"
	"github.com/olegsidorkin/moex-trader/internal/storage"
)

type traderTestIngestor struct{}

func (traderTestIngestor) Ingest(context.Context, string) (features.Input, error) {
	return features.Input{}, nil
}

type traderTestGate struct{}

func (traderTestGate) Approve(context.Context, risk.Request) (bool, error) {
	return true, nil
}

type traderTestExecutor struct{}

func (traderTestExecutor) Execute(context.Context, domain.TradeSignal, decimal.Decimal) (executor.Fill, error) {
	return executor.Fill{}, nil
}

type traderTestAudit struct{}

func (traderTestAudit) InsertAuditEvent(context.Context, domain.AuditEvent) error {
	return nil
}

func openTraderTestStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "trader.db"))
	if err != nil {
		t.Fatalf("storage.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("store.Close() error = %v", err)
		}
	})
	return store
}

func TestSelectExecutorPicksPaper(t *testing.T) {
	store := openTraderTestStore(t)
	cfg := &config.Config{
		Broker:         config.BrokerPaper,
		IsPaperTrading: true,
		Commission:     config.Commission{Rate: decimal.New(1, -4)},
	}

	got, err := selectExecutor(cfg, store, time.Now)
	if err != nil {
		t.Fatalf("selectExecutor() error = %v", err)
	}
	if _, ok := got.(*executor.PaperExecutor); !ok {
		t.Fatalf("selectExecutor() type = %T, want *executor.PaperExecutor", got)
	}
}

func TestSelectExecutorEmptyBrokerDefaultsToPaper(t *testing.T) {
	store := openTraderTestStore(t)
	cfg := &config.Config{IsPaperTrading: true}

	got, err := selectExecutor(cfg, store, time.Now)
	if err != nil {
		t.Fatalf("selectExecutor() error = %v", err)
	}
	if _, ok := got.(*executor.PaperExecutor); !ok {
		t.Fatalf("selectExecutor() type = %T, want *executor.PaperExecutor", got)
	}
}

func TestSelectExecutorRefusesLiveBrokers(t *testing.T) {
	for _, broker := range []string{config.BrokerTinkoff, config.BrokerFinam} {
		cfg := &config.Config{Broker: broker, IsPaperTrading: false}
		_, err := selectExecutor(cfg, nil, time.Now)
		if err == nil {
			t.Fatalf("selectExecutor(%q) error = nil, want refusal", broker)
		}
		if !strings.Contains(err.Error(), broker) {
			t.Fatalf("selectExecutor(%q) error = %v, want broker name in message", broker, err)
		}
		if !strings.Contains(err.Error(), "live trading mode is not wired") {
			t.Fatalf("selectExecutor(%q) error = %v, want live wiring refusal", broker, err)
		}
	}
}

func TestSelectExecutorRefusesLiveBrokerInPaperMode(t *testing.T) {
	for _, broker := range []string{config.BrokerTinkoff, config.BrokerFinam} {
		cfg := &config.Config{Broker: broker, IsPaperTrading: true}
		_, err := selectExecutor(cfg, nil, time.Now)
		if err == nil {
			t.Fatalf("selectExecutor(%q) error = nil, want refusal", broker)
		}
		if !strings.Contains(err.Error(), broker) {
			t.Fatalf("selectExecutor(%q) error = %v, want broker name in message", broker, err)
		}
	}
}

func TestSelectExecutorRefusesLiveModeForPaperBroker(t *testing.T) {
	cfg := &config.Config{Broker: config.BrokerPaper, IsPaperTrading: false}
	_, err := selectExecutor(cfg, nil, time.Now)
	if err == nil {
		t.Fatal("selectExecutor() error = nil, want refusal")
	}
	if !strings.Contains(err.Error(), "live trading mode is not wired") {
		t.Fatalf("selectExecutor() error = %v, want live wiring refusal", err)
	}
}

func TestSelectExecutorUnknownBroker(t *testing.T) {
	cfg := &config.Config{Broker: "alfa", IsPaperTrading: true}
	_, err := selectExecutor(cfg, nil, time.Now)
	if err == nil {
		t.Fatal("selectExecutor() error = nil, want unknown broker error")
	}
	if !strings.Contains(err.Error(), "unknown broker") {
		t.Fatalf("selectExecutor() error = %v, want unknown broker error", err)
	}
}

func TestSelectExecutorNilConfig(t *testing.T) {
	_, err := selectExecutor(nil, nil, time.Now)
	if err == nil {
		t.Fatal("selectExecutor() error = nil, want nil config error")
	}
}

func TestNewModelSignalSourceWiresIntoOrchestrator(t *testing.T) {
	_, names := model.ToVector(domain.FeatureContext{})
	weights := &model.Weights{
		FeatureOrder:  names,
		Mean:          make([]float64, len(names)),
		Std:           make([]float64, len(names)),
		Coef:          make([]float64, len(names)),
		Bias:          0,
		BuyThreshold:  0.55,
		SellThreshold: 0.45,
		HorizonDays:   5,
		DeadbandPct:   0.5,
		TrainedAt:     time.Now(),
	}
	for i := range weights.Std {
		weights.Std[i] = 1
	}
	modelPath := filepath.Join(t.TempDir(), "model.json")
	if err := weights.Save(modelPath); err != nil {
		t.Fatalf("weights.Save() error = %v", err)
	}

	cfg := &config.Config{
		Model: config.Model{Path: modelPath},
		Risk:  config.Risk{MaxLots: 3},
	}
	source, err := newModelSignalSource(cfg)
	if err != nil {
		t.Fatalf("newModelSignalSource() error = %v", err)
	}
	if source == nil {
		t.Fatal("newModelSignalSource() returned nil source")
	}
	if source.Weights == nil {
		t.Fatal("newModelSignalSource() returned nil weights")
	}
	if source.MaxLots != cfg.Risk.MaxLots {
		t.Fatalf("newModelSignalSource() max lots = %d, want %d", source.MaxLots, cfg.Risk.MaxLots)
	}

	_, err = orchestrator.New(orchestrator.Options{
		Tickers:  []string{"SBER"},
		Ingestor: traderTestIngestor{},
		Source:   source,
		Gate:     traderTestGate{},
		Executor: traderTestExecutor{},
		Audit:    traderTestAudit{},
	})
	if err != nil {
		t.Fatalf("orchestrator.New() error = %v", err)
	}
}

type recordingSignalSource struct {
	signal domain.TradeSignal
	err    error
}

func (s recordingSignalSource) Generate(context.Context, domain.FeatureContext) (domain.TradeSignal, error) {
	return s.signal, s.err
}

type recordingAlerter struct {
	texts []string
	err   error
}

func (a *recordingAlerter) Send(_ context.Context, text string) error {
	a.texts = append(a.texts, text)
	return a.err
}

func TestAlertingSignalSourceAlertsOnFailure(t *testing.T) {
	generateErr := fmt.Errorf("model: computed probability is not finite for SBER")
	alerter := &recordingAlerter{}
	source := &alertingSignalSource{
		source:  recordingSignalSource{err: generateErr},
		alerter: alerter,
		logger:  log.Default(),
	}

	_, err := source.Generate(context.Background(), domain.FeatureContext{Ticker: "SBER"})
	if err != generateErr {
		t.Fatalf("Generate() error = %v, want %v", err, generateErr)
	}
	if len(alerter.texts) != 1 {
		t.Fatalf("alert count = %d, want 1", len(alerter.texts))
	}
	if !strings.Contains(alerter.texts[0], "SBER") || !strings.Contains(alerter.texts[0], generateErr.Error()) {
		t.Fatalf("alert text = %q, want ticker and error", alerter.texts[0])
	}
}

func TestAlertingSignalSourceNoAlertOnSuccess(t *testing.T) {
	alerter := &recordingAlerter{}
	wantSignal := domain.TradeSignal{Ticker: "SBER", Action: domain.ActionHold}
	source := &alertingSignalSource{
		source:  recordingSignalSource{signal: wantSignal},
		alerter: alerter,
		logger:  log.Default(),
	}

	got, err := source.Generate(context.Background(), domain.FeatureContext{Ticker: "SBER"})
	if err != nil {
		t.Fatalf("Generate() error = %v, want nil", err)
	}
	if got != wantSignal {
		t.Fatalf("Generate() signal = %+v, want %+v", got, wantSignal)
	}
	if len(alerter.texts) != 0 {
		t.Fatalf("alert count = %d, want 0", len(alerter.texts))
	}
}

func TestNewModelSignalSourceMissingModelFails(t *testing.T) {
	cfg := &config.Config{
		Model: config.Model{Path: filepath.Join(t.TempDir(), "missing-model.json")},
	}
	_, err := newModelSignalSource(cfg)
	if err == nil {
		t.Fatal("newModelSignalSource() error = nil, want missing model failure")
	}
	if !strings.Contains(err.Error(), "load model") {
		t.Fatalf("newModelSignalSource() error = %v, want load model failure", err)
	}
}
