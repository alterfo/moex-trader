package strategyvalidation

type Attempt struct {
	Name   string
	Source string
	Metric string
	Value  float64
	Unit   string
	Status string
	Notes  string
}

type Registry struct {
	Attempts []Attempt
	Series   map[string][]float64
	Matrices map[string][][]float64
}

const (
	StatusSelected  = "selected"
	StatusRejected  = "rejected"
	StatusControl   = "control"
	StatusReference = "reference"
	StatusBenchmark = "benchmark"
)

const defaultDeposit = 1_000_000.0

type RegistrySummary struct {
	Attempts int
	Selected int
	Rejected int
	Other    int
	Sources  map[string]int
}

func DefaultRegistry() Registry {
	attempts := []Attempt{
		{Name: "abs10d_baseline", Source: "AGENTS.md:strategy-research", Metric: "realized_pnl_6q", Value: 110742, Unit: "RUB", Status: StatusSelected, Notes: "18 tickers, 15000 RUB notional, thresholds 0.60/0.40, hold-until-flip, max DD 2.91%"},
		{Name: "excess_label_baseline", Source: "AGENTS.md:strategy-research", Metric: "realized_pnl_6q", Value: 235, Unit: "RUB", Status: StatusRejected, Notes: "excess-to-IMOEX label, 2/6 quarters positive"},
		{Name: "fx_tickers_included", Source: "AGENTS.md:strategy-research", Metric: "worst_single_ticker_pnl", Value: -1500, Unit: "RUB", Status: StatusRejected, Notes: "GLDRUB_TOM, SLVRUB_TOM, CNYRUB_TOM"},
		{Name: "horizon_1d", Source: "AGENTS.md:strategy-research", Metric: "relative_pnl_vs_10d", Value: 0, Unit: "RUB", Status: StatusRejected, Notes: "weaker than 10d"},
		{Name: "horizon_3d", Source: "AGENTS.md:strategy-research", Metric: "relative_pnl_vs_10d", Value: 0, Unit: "RUB", Status: StatusRejected, Notes: "weaker than 10d"},
		{Name: "horizon_5d", Source: "AGENTS.md:strategy-research", Metric: "relative_pnl_vs_10d", Value: 0, Unit: "RUB", Status: StatusRejected, Notes: "weaker than 10d"},
		{Name: "thresholds_065_035", Source: "AGENTS.md:strategy-research", Metric: "relative_pnl_vs_060_040", Value: 0, Unit: "RUB", Status: StatusRejected, Notes: "worse than 0.60/0.40"},
		{Name: "confidence_scaled_sizing", Source: "AGENTS.md:strategy-research", Metric: "realized_pnl_18mo", Value: 0, Unit: "RUB", Status: StatusRejected, Notes: "min(1, |p-0.5|/0.2), 300+ trades"},
		{Name: "notional_20000", Source: "AGENTS.md:strategy-research", Metric: "max_drawdown_pct", Value: 3.8, Unit: "%", Status: StatusRejected, Notes: "breaches 3% kill-switch limit"},
		{Name: "universe_42_tickers", Source: "AGENTS.md:strategy-research", Metric: "max_drawdown_pct", Value: 3.9, Unit: "%", Status: StatusRejected, Notes: "breaches 3% kill-switch limit"},
		{Name: "in_sample_365d", Source: "AGENTS.md:model-quality", Metric: "realized_pnl", Value: 8566, Unit: "RUB", Status: StatusRejected, Notes: "data-scarcity overfitting symptom"},
		{Name: "out_of_sample_validation", Source: "AGENTS.md:model-quality", Metric: "realized_pnl", Value: -497, Unit: "RUB", Status: StatusRejected, Notes: "validation window"},
		{Name: "baseline_classification_auc", Source: "AGENTS.md:model-quality", Metric: "auc", Value: 0.51, Unit: "AUC", Status: StatusReference, Notes: "lgbm/xgb/logreg near noise"},
		{Name: "intraday_10min", Source: "AGENTS.md:strategy-research", Metric: "validation_pnl", Value: -12800, Unit: "RUB", Status: StatusRejected, Notes: "AUC 0.5173, kill switch tripped"},
		{Name: "features_26_colsample_08", Source: "AGENTS.md:strategy-research", Metric: "realized_pnl_6q", Value: 80716, Unit: "RUB", Status: StatusRejected, Notes: "zero event features dilute sampled columns"},
		{Name: "features_26_colsample_10", Source: "AGENTS.md:strategy-research", Metric: "realized_pnl_6q", Value: 87823, Unit: "RUB", Status: StatusRejected, Notes: "26-column recipe"},
		{Name: "max_hold_bars_10", Source: "AGENTS.md:strategy-research", Metric: "realized_pnl_6q", Value: 52498, Unit: "RUB", Status: StatusRejected, Notes: "exit at label horizon"},
		{Name: "hold_until_flip", Source: "AGENTS.md:strategy-research", Metric: "realized_pnl_6q", Value: 87823, Unit: "RUB", Status: StatusSelected, Notes: "daily strategy comparison baseline"},
		{Name: "news_aware_retrain", Source: "AGENTS.md:strategy-research", Metric: "realized_pnl_3mo", Value: 56379, Unit: "RUB", Status: StatusSelected, Notes: "real Telegram news backfill, 253 trades, DD 1.60%"},
		{Name: "no_news_control", Source: "AGENTS.md:strategy-research", Metric: "realized_pnl_3mo", Value: 40102, Unit: "RUB", Status: StatusControl, Notes: "identical window control, 205 trades, DD 1.09%"},
		{Name: "event_features_on_news", Source: "AGENTS.md:strategy-research", Metric: "realized_pnl_mtm", Value: 31161, Unit: "RUB", Status: StatusRejected, Notes: "sparse event features still underperform"},
		{Name: "event_features_no_news_mtm", Source: "AGENTS.md:strategy-research", Metric: "realized_pnl_mtm", Value: 33395, Unit: "RUB", Status: StatusControl, Notes: "pre-correction comparison"},
		{Name: "no_news_90d_preflight", Source: "AGENTS.md:strategy-research", Metric: "realized_pnl_90d", Value: 25699, Unit: "RUB", Status: StatusSelected, Notes: "matches live startup gate, no news injected"},
		{Name: "spread_slippage_0", Source: "AGENTS.md:strategy-research", Metric: "realized_pnl_18mo", Value: 43891, Unit: "RUB", Status: StatusReference, Notes: "no fill-cost reference"},
		{Name: "spread_slippage_05_05", Source: "AGENTS.md:strategy-research", Metric: "realized_pnl_18mo", Value: 43082, Unit: "RUB", Status: StatusReference, Notes: "0.05% spread + 0.05% slippage"},
		{Name: "spread_slippage_10_10", Source: "AGENTS.md:strategy-research", Metric: "realized_pnl_18mo", Value: 42270, Unit: "RUB", Status: StatusReference, Notes: "0.1% spread + 0.1% slippage"},
		{Name: "retrained_artifact_preflight", Source: "AGENTS.md:strategy-research", Metric: "realized_pnl_preflight", Value: 79453, Unit: "RUB", Status: StatusSelected, Notes: "DD 0.84%, kill switch off"},
		{Name: "xsec_full_mtm", Source: "AGENTS.md:strategy-research", Metric: "mtm_pnl", Value: 8061, Unit: "RUB", Status: StatusRejected, Notes: "cross-sectional ranking"},
		{Name: "xsec_full_realized", Source: "AGENTS.md:strategy-research", Metric: "realized_pnl", Value: 621.80, Unit: "RUB", Status: StatusRejected, Notes: "2 years, spread not modeled"},
		{Name: "xsec_08_3000_mtm", Source: "AGENTS.md:strategy-research", Metric: "mtm_pnl", Value: 2020, Unit: "RUB", Status: StatusRejected, Notes: "cross-sectional ranking"},
		{Name: "xsec_08_3000_realized", Source: "AGENTS.md:strategy-research", Metric: "realized_pnl", Value: -5068, Unit: "RUB", Status: StatusRejected, Notes: "realized is negative"},
		{Name: "tail_precision_pooled", Source: "docs/metrics.md:task-1", Metric: "pooled_precision", Value: 42.4, Unit: "%", Status: StatusRejected, Notes: "212/500 tail decisions"},
		{Name: "tail_precision_base_rate", Source: "docs/metrics.md:task-1", Metric: "pooled_base_rate", Value: 47.7, Unit: "%", Status: StatusControl, Notes: "tail does not beat base rate"},
		{Name: "tail_precision_bootstrap_p", Source: "docs/metrics.md:task-1", Metric: "bootstrap_p_value", Value: 0.9123, Unit: "p", Status: StatusRejected, Notes: "83 episodes, 10000 trials"},
		{Name: "momentum_ensemble_20260619_0917", Source: "docs/metrics.md:task-2", Metric: "realized_pnl", Value: 34690.64, Unit: "RUB", Status: StatusBenchmark, Notes: "deployed ensemble on 81-day window"},
		{Name: "momentum_long_only", Source: "docs/metrics.md:task-2", Metric: "realized_pnl", Value: -6697.58, Unit: "RUB", Status: StatusRejected, Notes: "top-5 mom_21d, 10-day rebalance"},
		{Name: "momentum_long_short", Source: "docs/metrics.md:task-2", Metric: "realized_pnl", Value: -13111.02, Unit: "RUB", Status: StatusRejected, Notes: "top/bottom-5 mom_21d"},
		{Name: "task4_six_quarter_total_realized", Source: "docs/metrics.md:task-4", Metric: "realized_pnl", Value: 153482.21, Unit: "RUB", Status: StatusSelected, Notes: "797 closed trades, kill-frozen days 0"},
		{Name: "task4_six_quarter_total_mtm", Source: "docs/metrics.md:task-4", Metric: "mtm_pnl", Value: 163901.86, Unit: "RUB", Status: StatusReference, Notes: "MTM is shown separately"},
		{Name: "task5_preflight_realized", Source: "docs/metrics.md:task-5", Metric: "realized_pnl", Value: 43040.38, Unit: "RUB", Status: StatusSelected, Notes: "90-day preflight, 197 closed trades, DD 1.07%"},
		{Name: "task5_preflight_mtm", Source: "docs/metrics.md:task-5", Metric: "mtm_pnl", Value: 38818.21, Unit: "RUB", Status: StatusReference, Notes: "90-day preflight MTM"},
	}
	series := map[string][]float64{
		"abs10d_quarterly_realized_pnl":    []float64{42453.50, 9171.72, 19600.27, -6655.63, 41695.40, 47216.95},
		"abs10d_quarterly_realized_return": realizedReturns([]float64{42453.50, 9171.72, 19600.27, -6655.63, 41695.40, 47216.95}),
	}
	return Registry{Attempts: attempts, Series: series}
}

func realizedReturns(pnl []float64) []float64 {
	returns := make([]float64, len(pnl))
	for i, value := range pnl {
		returns[i] = value / defaultDeposit
	}
	return returns
}

func (r Registry) Summary() RegistrySummary {
	summary := RegistrySummary{Sources: make(map[string]int)}
	for _, attempt := range r.Attempts {
		summary.Attempts++
		switch attempt.Status {
		case StatusSelected:
			summary.Selected++
		case StatusRejected:
			summary.Rejected++
		default:
			summary.Other++
		}
		summary.Sources[attempt.Source]++
	}
	return summary
}

func (r Registry) AttemptCount() int {
	return len(r.Attempts)
}

func (r Registry) PBOComputable() bool {
	return len(r.Matrices) > 0
}

func (r Registry) ComputePBO(name string, s int) (PBOResult, bool) {
	matrix, ok := r.Matrices[name]
	if !ok {
		return PBOResult{}, false
	}
	return ProbabilityOfBacktestOverfitting(matrix, s), true
}
