package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	Registry             *prometheus.Registry
	LLMInferenceDuration prometheus.Histogram
	SignalsGenerated     prometheus.Counter
	RiskRejections       prometheus.Counter
}

func New() *Metrics {
	registry := prometheus.NewRegistry()

	llmInferenceDuration := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "llm_inference_duration_seconds",
		Help:    "Duration of LLM inference calls.",
		Buckets: prometheus.DefBuckets,
	})
	signalsGenerated := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "signals_generated_total",
		Help: "Total number of generated trade signals.",
	})
	riskRejections := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "risk_rejections_total",
		Help: "Total number of signals rejected by the risk gate.",
	})

	registry.MustRegister(llmInferenceDuration, signalsGenerated, riskRejections)

	return &Metrics{
		Registry:             registry,
		LLMInferenceDuration: llmInferenceDuration,
		SignalsGenerated:     signalsGenerated,
		RiskRejections:       riskRejections,
	}
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{})
}

func (m *Metrics) ObserveLLMInference(duration time.Duration) {
	m.LLMInferenceDuration.Observe(duration.Seconds())
}

func (m *Metrics) IncSignalsGenerated() {
	m.SignalsGenerated.Inc()
}

func (m *Metrics) IncRiskRejections() {
	m.RiskRejections.Inc()
}
