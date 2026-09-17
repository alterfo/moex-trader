package strategyvalidation

import (
	"math"
	"testing"
)

func TestDefaultRegistryAttemptCount(t *testing.T) {
	registry := DefaultRegistry()
	if registry.AttemptCount() < 40 {
		t.Fatalf("AttemptCount = %d, want >= 40", registry.AttemptCount())
	}
	summary := registry.Summary()
	if summary.Attempts != registry.AttemptCount() {
		t.Fatalf("summary.Attempts = %d, want %d", summary.Attempts, registry.AttemptCount())
	}
	if summary.Selected+summary.Rejected+summary.Other != summary.Attempts {
		t.Fatalf("status counts %d+%d+%d do not sum to %d", summary.Selected, summary.Rejected, summary.Other, summary.Attempts)
	}
}

func TestDefaultRegistryContainsDocumentedKeyValues(t *testing.T) {
	registry := DefaultRegistry()
	assertAttempt(t, registry, "abs10d_baseline", 110742)
	assertAttempt(t, registry, "news_aware_retrain", 56379)
	assertAttempt(t, registry, "max_hold_bars_10", 52498)
	assertAttempt(t, registry, "xsec_08_3000_realized", -5068)
	assertAttempt(t, registry, "tail_precision_pooled", 42.4)
	assertAttempt(t, registry, "task4_six_quarter_total_realized", 153482.21)
}

func TestDefaultRegistryRealizedReturnSeries(t *testing.T) {
	registry := DefaultRegistry()
	pnl := registry.Series["abs10d_quarterly_realized_pnl"]
	returns := registry.Series["abs10d_quarterly_realized_return"]
	if len(pnl) != 6 || len(returns) != 6 {
		t.Fatalf("series lengths = %d/%d, want 6/6", len(pnl), len(returns))
	}
	for i := range pnl {
		if math.Abs(returns[i]-pnl[i]/defaultDeposit) > 1e-12 {
			t.Fatalf("return[%d] = %.12f, want %.12f", i, returns[i], pnl[i]/defaultDeposit)
		}
	}
	if math.Abs(SharpeRatio(returns)-1.179) > 0.01 {
		t.Fatalf("six-quarter realized Sharpe = %.4f, want ~1.18", SharpeRatio(returns))
	}
}

func TestRegistryPBONotComputableFromSummaryOnly(t *testing.T) {
	registry := DefaultRegistry()
	if registry.PBOComputable() {
		t.Fatal("summary-only registry reported PBO computable")
	}
	if _, ok := registry.ComputePBO("missing", 10); ok {
		t.Fatal("ComputePBO returned ok for missing matrix")
	}
}

func assertAttempt(t *testing.T, registry Registry, name string, want float64) {
	t.Helper()
	for _, attempt := range registry.Attempts {
		if attempt.Name == name {
			if math.Abs(attempt.Value-want) > 1e-9 {
				t.Fatalf("%s value = %.6f, want %.6f", name, attempt.Value, want)
			}
			return
		}
	}
	t.Fatalf("attempt %q not found", name)
}
