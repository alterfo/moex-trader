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
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
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
	if got := testutil.ToFloat64(appMetrics.RiskRejections.WithLabelValues("max_lots_exceeded")); got != 2 {
		t.Fatalf("risk_rejections_total{reason=max_lots_exceeded} = %v, want 2", got)
	}
	if got := histogramSampleCount(t, appMetrics.InferenceDuration); got != 2 {
		t.Fatalf("moex_trader_inference_duration_seconds sample count = %d, want 2", got)
	}
	if got := histogramSampleCount(t, appMetrics.SignalProbability); got != 2 {
		t.Fatalf("signal_probability sample count = %d, want 2", got)
	}
}

func TestOrchestratorWiresExecutorSkips(t *testing.T) {
	store := openTestStore(t)
	now := func() time.Time { return time.Date(2024, 1, 11, 12, 30, 0, 0, time.UTC) }
	ingestor := &fakeIngestor{inputs: map[string]features.Input{
		"SBER": fixtureInput("SBER"),
		"YDEX": fixtureInput("YDEX"),
	}}
	source := &fakeSource{
		signal: domain.TradeSignal{
			Action:      domain.ActionHold,
			Confidence:  decimal.NewFromFloat(0.5),
			HoldReason:  domain.HoldReasonModel,
			Reasoning:   "fixture hold",
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

	if got := testutil.ToFloat64(appMetrics.ExecutorSkips.WithLabelValues(domain.HoldReasonModel)); got != 2 {
		t.Fatalf("executor_skips_total{reason=model} = %v, want 2", got)
	}
}

type recordingCandleObserver struct {
	ages map[string]time.Duration
}

func (o *recordingCandleObserver) ObserveCandle(_ context.Context, ticker string, age time.Duration) {
	if o.ages == nil {
		o.ages = make(map[string]time.Duration)
	}
	o.ages[ticker] = age
}

func TestOrchestratorWiresCandleAge(t *testing.T) {
	store := openTestStore(t)
	now := func() time.Time { return time.Date(2024, 1, 11, 12, 30, 0, 0, time.UTC) }
	sberInput := fixtureInput("SBER")
	sberInput.Candles = []moex.Candle{
		{Begin: now().Add(-2 * time.Hour), End: now().Add(-time.Hour), Close: decimal.NewFromFloat(100)},
	}
	ingestor := &fakeIngestor{inputs: map[string]features.Input{
		"SBER": sberInput,
		"YDEX": fixtureInput("YDEX"),
	}}
	source := &fakeSource{
		signal: domain.TradeSignal{
			Action:      domain.ActionHold,
			Confidence:  decimal.NewFromFloat(0.5),
			HoldReason:  domain.HoldReasonModel,
			Reasoning:   "fixture hold",
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
	observer := &recordingCandleObserver{}
	orch, err := New(Options{
		Tickers:        []string{"SBER", "YDEX"},
		Ingestor:       ingestor,
		Source:         source,
		Gate:           gate,
		Executor:       exec,
		Audit:          store,
		PollInterval:   time.Second,
		Now:            now,
		Metrics:        appMetrics,
		CandleObserver: observer,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	orch.RunOnce(context.Background())

	if got := testutil.ToFloat64(appMetrics.CandleAge.WithLabelValues("SBER")); got != 3600 {
		t.Fatalf("candle_age_seconds{ticker=SBER} = %v, want 3600", got)
	}
	if got := testutil.ToFloat64(appMetrics.CandleAge.WithLabelValues("YDEX")); got != 0 {
		t.Fatalf("candle_age_seconds{ticker=YDEX} = %v, want 0", got)
	}
	if got := observer.ages["SBER"]; got != time.Hour {
		t.Fatalf("observer age for SBER = %v, want 1h", got)
	}
	if got := observer.ages["YDEX"]; got != 0 {
		t.Fatalf("observer age for YDEX = %v, want 0", got)
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
