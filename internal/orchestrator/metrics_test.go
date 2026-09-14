package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/features"
	"github.com/olegsidorkin/moex-trader/internal/metrics"
	"github.com/olegsidorkin/moex-trader/internal/risk"
)

func TestOrchestratorWiresMetrics(t *testing.T) {
	store := openTestStore(t)
	now := func() time.Time { return time.Date(2024, 1, 11, 12, 30, 0, 0, time.UTC) }
	ingestor := &fakeIngestor{inputs: map[string]features.Input{
		"SBER": fixtureInput("SBER"),
		"YDEX": fixtureInput("YDEX"),
	}}
	source := &fakeSource{
		signal: domain.TradeSignal{
			Action:      domain.ActionBuy,
			Confidence:  decimal.NewFromFloat(0.8),
			TargetLots:  2,
			Reasoning:   "fixture buy",
			GeneratedAt: now(),
		},
		now: now,
	}
	exec := &fakeExecutor{store: store, now: now}
	gate, err := risk.NewLotLimitGate(1)
	if err != nil {
		t.Fatalf("NewLotLimitGate() error = %v", err)
	}
	appMetrics := metrics.New()
	orch, err := New(Options{
		Tickers:      []string{"SBER", "YDEX"},
		Ingestor:     ingestor,
		Source:       source,
		Gate:         gate,
		Executor:     exec,
		Audit:        store,
		PollInterval: time.Second,
		Now:          now,
		Metrics:      appMetrics,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	orch.RunOnce(context.Background())

	if got := testutil.ToFloat64(appMetrics.SignalsGenerated); got != 2 {
		t.Fatalf("signals_generated_total = %v, want 2", got)
	}
	if got := testutil.ToFloat64(appMetrics.RiskRejections); got != 2 {
		t.Fatalf("risk_rejections_total = %v, want 2", got)
	}
	if got := histogramSampleCount(t, appMetrics.LLMInferenceDuration); got != 2 {
		t.Fatalf("llm_inference_duration_seconds sample count = %d, want 2", got)
	}
}

func histogramSampleCount(t *testing.T, histogram prometheus.Histogram) uint64 {
	t.Helper()

	metrics := make(chan prometheus.Metric, 1)
	histogram.Collect(metrics)
	metric := <-metrics

	var pb dto.Metric
	if err := metric.Write(&pb); err != nil {
		t.Fatalf("Metric.Write() error = %v", err)
	}
	if pb.GetHistogram() == nil {
		t.Fatal("metric is not a histogram")
	}
	return pb.GetHistogram().GetSampleCount()
}
