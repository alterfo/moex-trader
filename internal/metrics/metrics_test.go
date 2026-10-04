package metrics

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/shopspring/decimal"
)

func TestNewRegistersPrometheusMetrics(t *testing.T) {
	m := New()
	touchVecs(m)

	families, err := m.Registry.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	if len(families) != 10 {
		t.Fatalf("Gather() returned %d metric families, want 10", len(families))
	}

	want := map[string]bool{
		"moex_trader_inference_duration_seconds": false,
		"signals_generated_total":                false,
		"risk_rejections_total":                  false,
		"executor_skips_total":                   false,
		"signal_probability":                     false,
		"candle_age_seconds":                     false,
		"feature_psi":                            false,
		"position_notional":                      false,
		"gross_exposure":                         false,
		"lease_held":                             false,
	}
	for _, family := range families {
		if _, ok := want[family.GetName()]; ok {
			want[family.GetName()] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Fatalf("metric %q was not registered", name)
		}
	}
}

func TestMetricsUpdateValues(t *testing.T) {
	m := New()

	m.ObserveInference(250 * time.Millisecond)
	m.IncSignalsGenerated()
	m.IncSignalsGenerated()
	m.IncRiskRejections("max_lots")

	if got := prometheusValue(t, m.SignalsGenerated); got != 2 {
		t.Fatalf("signals_generated_total = %v, want 2", got)
	}
	if got := testutil.ToFloat64(m.RiskRejections.WithLabelValues("max_lots")); got != 1 {
		t.Fatalf("risk_rejections_total{reason=max_lots} = %v, want 1", got)
	}

	sample := histogramSample(t, m.InferenceDuration)
	if sample.Count != 1 {
		t.Fatalf("moex_trader_inference_duration_seconds sample count = %d, want 1", sample.Count)
	}
	if sample.Sum < 0.2 || sample.Sum > 0.3 {
		t.Fatalf("moex_trader_inference_duration_seconds sample sum = %v, want ~0.25", sample.Sum)
	}
}

func TestRiskRejectionsByReason(t *testing.T) {
	m := New()
	m.IncRiskRejections("max_lots")
	m.IncRiskRejections("max_lots")
	m.IncRiskRejections("daily_loss")

	if got := testutil.ToFloat64(m.RiskRejections.WithLabelValues("max_lots")); got != 2 {
		t.Fatalf("risk_rejections_total{reason=max_lots} = %v, want 2", got)
	}
	if got := testutil.ToFloat64(m.RiskRejections.WithLabelValues("daily_loss")); got != 1 {
		t.Fatalf("risk_rejections_total{reason=daily_loss} = %v, want 1", got)
	}
}

func TestExecutorSkipsByReason(t *testing.T) {
	m := New()
	m.IncExecutorSkips("model")
	m.IncExecutorSkips("news")
	m.IncExecutorSkips("model")

	if got := testutil.ToFloat64(m.ExecutorSkips.WithLabelValues("model")); got != 2 {
		t.Fatalf("executor_skips_total{reason=model} = %v, want 2", got)
	}
	if got := testutil.ToFloat64(m.ExecutorSkips.WithLabelValues("news")); got != 1 {
		t.Fatalf("executor_skips_total{reason=news} = %v, want 1", got)
	}
}

func TestSignalProbabilityHistogram(t *testing.T) {
	m := New()
	m.ObserveSignalProbability(decimal.NewFromFloat(0.51))
	m.ObserveSignalProbability(decimal.NewFromFloat(0.9))

	sample := histogramSample(t, m.SignalProbability)
	if sample.Count != 2 {
		t.Fatalf("signal_probability sample count = %d, want 2", sample.Count)
	}
	if sample.Sum < 1.4 || sample.Sum > 1.42 {
		t.Fatalf("signal_probability sample sum = %v, want ~1.41", sample.Sum)
	}
}

func TestCandleAgeGauge(t *testing.T) {
	m := New()
	m.SetCandleAge("SBER", 3600)

	if got := testutil.ToFloat64(m.CandleAge.WithLabelValues("SBER")); got != 3600 {
		t.Fatalf("candle_age_seconds{ticker=SBER} = %v, want 3600", got)
	}
}

func TestFeaturePSIGauge(t *testing.T) {
	m := New()
	m.SetFeaturePSI("mom_5d", 0.31)

	if got := testutil.ToFloat64(m.FeaturePSI.WithLabelValues("mom_5d")); got != 0.31 {
		t.Fatalf("feature_psi{feature=mom_5d} = %v, want 0.31", got)
	}
}

func TestPositionNotionalAndGrossExposure(t *testing.T) {
	m := New()
	m.SetPositionNotional("SBER", decimal.NewFromInt(15000))
	m.SetPositionNotional("VTBR", decimal.NewFromInt(-5000))
	m.SetGrossExposure(decimal.NewFromInt(20000))

	if got := testutil.ToFloat64(m.PositionNotional.WithLabelValues("SBER")); got != 15000 {
		t.Fatalf("position_notional{ticker=SBER} = %v, want 15000", got)
	}
	if got := testutil.ToFloat64(m.PositionNotional.WithLabelValues("VTBR")); got != -5000 {
		t.Fatalf("position_notional{ticker=VTBR} = %v, want -5000", got)
	}
	if got := testutil.ToFloat64(m.GrossExposure); got != 20000 {
		t.Fatalf("gross_exposure = %v, want 20000", got)
	}
}

func TestLeaseHeldGauge(t *testing.T) {
	m := New()
	m.SetLeaseHeld(true)
	if got := testutil.ToFloat64(m.LeaseHeld); got != 1 {
		t.Fatalf("lease_held = %v, want 1", got)
	}
	m.SetLeaseHeld(false)
	if got := testutil.ToFloat64(m.LeaseHeld); got != 0 {
		t.Fatalf("lease_held = %v, want 0", got)
	}
}

func TestHandlerExposesMetrics(t *testing.T) {
	m := New()
	m.IncSignalsGenerated()
	m.IncRiskRejections("max_lots")
	m.ObserveInference(time.Second)
	touchVecs(m)

	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("Handler status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, metric := range []string{
		"moex_trader_inference_duration_seconds",
		"signals_generated_total",
		"risk_rejections_total",
		"executor_skips_total",
		"signal_probability",
		"candle_age_seconds",
		"feature_psi",
		"position_notional",
		"gross_exposure",
		"lease_held",
	} {
		if !strings.Contains(body, metric) {
			t.Fatalf("metrics response does not contain %q", metric)
		}
	}
}

func touchVecs(m *Metrics) {
	m.IncRiskRejections("max_lots")
	m.IncExecutorSkips("model")
	m.SetCandleAge("SBER", 0)
	m.SetFeaturePSI("mom_5d", 0)
	m.SetPositionNotional("SBER", decimal.Zero)
}

type histogramSnapshot struct {
	Count uint64
	Sum   float64
}

func histogramSample(t *testing.T, histogram prometheus.Histogram) histogramSnapshot {
	t.Helper()

	metrics := make(chan prometheus.Metric, 1)
	histogram.Collect(metrics)
	metric := <-metrics

	var pb dto.Metric
	if err := metric.Write(&pb); err != nil {
		t.Fatalf("Metric.Write() error = %v", err)
	}
	histogramPB := pb.GetHistogram()
	if histogramPB == nil {
		t.Fatal("metric is not a histogram")
	}
	return histogramSnapshot{
		Count: histogramPB.GetSampleCount(),
		Sum:   histogramPB.GetSampleSum(),
	}
}

func prometheusValue(t *testing.T, collector prometheus.Collector) float64 {
	t.Helper()

	metrics := make(chan prometheus.Metric, 1)
	collector.Collect(metrics)
	metric := <-metrics

	var pb dto.Metric
	if err := metric.Write(&pb); err != nil {
		t.Fatalf("Metric.Write() error = %v", err)
	}
	switch {
	case pb.GetCounter() != nil:
		return pb.GetCounter().GetValue()
	case pb.GetGauge() != nil:
		return pb.GetGauge().GetValue()
	default:
		t.Fatalf("metric is not a counter or gauge: %s", pb.String())
		return 0
	}
}
