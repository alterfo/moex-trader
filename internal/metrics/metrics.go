package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/shopspring/decimal"
)

type Metrics struct {
	Registry          *prometheus.Registry
	InferenceDuration prometheus.Histogram
	SignalsGenerated  prometheus.Counter
	RiskRejections    *prometheus.CounterVec
	ExecutorSkips     *prometheus.CounterVec
	SignalProbability prometheus.Histogram
	CandleAge         *prometheus.GaugeVec
	FeaturePSI        *prometheus.GaugeVec
	PositionNotional  *prometheus.GaugeVec
	GrossExposure     prometheus.Gauge
	LeaseHeld         prometheus.Gauge
	Var95             prometheus.Gauge
	Var99             prometheus.Gauge
	ES95              prometheus.Gauge
	ES99              prometheus.Gauge
}

func New() *Metrics {
	registry := prometheus.NewRegistry()

	inferenceDuration := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "moex_trader_inference_duration_seconds",
		Help:    "Duration of model inference calls.",
		Buckets: prometheus.DefBuckets,
	})
	signalsGenerated := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "signals_generated_total",
		Help: "Total number of generated trade signals.",
	})
	riskRejections := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "risk_rejections_total",
		Help: "Total number of signals rejected by the risk gate, by reason.",
	}, []string{"reason"})
	executorSkips := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "executor_skips_total",
		Help: "Total number of approved decisions the executor skipped, by reason.",
	}, []string{"reason"})
	signalProbability := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "signal_probability",
		Help:    "Distribution of model signal probabilities.",
		Buckets: []float64{0, 0.1, 0.2, 0.3, 0.4, 0.45, 0.5, 0.55, 0.6, 0.7, 0.8, 0.9, 1},
	})
	candleAge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "candle_age_seconds",
		Help: "Age in seconds of the newest ingested candle per ticker.",
	}, []string{"ticker"})
	featurePSI := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "feature_psi",
		Help: "Latest population stability index per feature.",
	}, []string{"feature"})
	positionNotional := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "position_notional",
		Help: "Signed open-position notional in RUB per ticker.",
	}, []string{"ticker"})
	grossExposure := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gross_exposure",
		Help: "Gross open-position notional in RUB across all tickers.",
	})
	leaseHeld := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "lease_held",
		Help: "1 when this process holds the trading lease, 0 otherwise.",
	})
	var95 := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "var_95",
		Help: "Historical 1-day 95% value-at-risk of the current book in RUB.",
	})
	var99 := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "var_99",
		Help: "Historical 1-day 99% value-at-risk of the current book in RUB.",
	})
	es95 := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "es_95",
		Help: "Historical 1-day 95% expected shortfall of the current book in RUB.",
	})
	es99 := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "es_99",
		Help: "Historical 1-day 99% expected shortfall of the current book in RUB.",
	})

	registry.MustRegister(
		inferenceDuration,
		signalsGenerated,
		riskRejections,
		executorSkips,
		signalProbability,
		candleAge,
		featurePSI,
		positionNotional,
		grossExposure,
		leaseHeld,
		var95,
		var99,
		es95,
		es99,
	)

	return &Metrics{
		Registry:          registry,
		InferenceDuration: inferenceDuration,
		SignalsGenerated:  signalsGenerated,
		RiskRejections:    riskRejections,
		ExecutorSkips:     executorSkips,
		SignalProbability: signalProbability,
		CandleAge:         candleAge,
		FeaturePSI:        featurePSI,
		PositionNotional:  positionNotional,
		GrossExposure:     grossExposure,
		LeaseHeld:         leaseHeld,
		Var95:             var95,
		Var99:             var99,
		ES95:              es95,
		ES99:              es99,
	}
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{})
}

func (m *Metrics) ObserveInference(duration time.Duration) {
	m.InferenceDuration.Observe(duration.Seconds())
}

func (m *Metrics) IncSignalsGenerated() {
	m.SignalsGenerated.Inc()
}

func (m *Metrics) IncRiskRejections(reason string) {
	m.RiskRejections.WithLabelValues(reason).Inc()
}

func (m *Metrics) IncExecutorSkips(reason string) {
	m.ExecutorSkips.WithLabelValues(reason).Inc()
}

func (m *Metrics) ObserveSignalProbability(probability decimal.Decimal) {
	m.SignalProbability.Observe(probability.InexactFloat64())
}

func (m *Metrics) SetCandleAge(ticker string, seconds float64) {
	m.CandleAge.WithLabelValues(ticker).Set(seconds)
}

func (m *Metrics) SetFeaturePSI(feature string, psi float64) {
	m.FeaturePSI.WithLabelValues(feature).Set(psi)
}

func (m *Metrics) SetPositionNotional(ticker string, notional decimal.Decimal) {
	m.PositionNotional.WithLabelValues(ticker).Set(notional.InexactFloat64())
}

func (m *Metrics) SetGrossExposure(notional decimal.Decimal) {
	m.GrossExposure.Set(notional.InexactFloat64())
}

func (m *Metrics) SetLeaseHeld(held bool) {
	if held {
		m.LeaseHeld.Set(1)
		return
	}
	m.LeaseHeld.Set(0)
}

func (m *Metrics) SetVaRES(var95, var99, es95, es99 decimal.Decimal) {
	m.Var95.Set(var95.InexactFloat64())
	m.Var99.Set(var99.InexactFloat64())
	m.ES95.Set(es95.InexactFloat64())
	m.ES99.Set(es99.InexactFloat64())
}
