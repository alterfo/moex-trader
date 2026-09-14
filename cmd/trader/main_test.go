package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/executor"
	"github.com/olegsidorkin/moex-trader/internal/storage"
)

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
