package metrics

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestNewRegistersPrometheusMetrics(t *testing.T) {
	m := New()

	families, err := m.Registry.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	if len(families) != 3 {
		t.Fatalf("Gather() returned %d metric families, want 3", len(families))
	}

	want := map[string]bool{
		"llm_inference_duration_seconds": false,
		"signals_generated_total":        false,
		"risk_rejections_total":          false,
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

	m.ObserveLLMInference(250 * time.Millisecond)
	m.IncSignalsGenerated()
	m.IncSignalsGenerated()
	m.IncRiskRejections()

	if got := prometheusValue(t, m.SignalsGenerated); got != 2 {
		t.Fatalf("signals_generated_total = %v, want 2", got)
	}
	if got := prometheusValue(t, m.RiskRejections); got != 1 {
		t.Fatalf("risk_rejections_total = %v, want 1", got)
	}

	sample := histogramSample(t, m.LLMInferenceDuration)
	if sample.Count != 1 {
		t.Fatalf("llm_inference_duration_seconds sample count = %d, want 1", sample.Count)
	}
	if sample.Sum < 0.2 || sample.Sum > 0.3 {
		t.Fatalf("llm_inference_duration_seconds sample sum = %v, want ~0.25", sample.Sum)
	}
}

func TestHandlerExposesMetrics(t *testing.T) {
	m := New()
	m.IncSignalsGenerated()
	m.IncRiskRejections()
	m.ObserveLLMInference(time.Second)

	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("Handler status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, metric := range []string{
		"llm_inference_duration_seconds",
		"signals_generated_total",
		"risk_rejections_total",
	} {
		if !strings.Contains(body, metric) {
			t.Fatalf("metrics response does not contain %q", metric)
		}
	}
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
