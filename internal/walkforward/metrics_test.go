package walkforward

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
)

func TestMetricsFromResultAndSaveLoadRoundTrip(t *testing.T) {
	spec := WindowSpec{
		From:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Split: time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC),
		Till:  time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC),
	}
	r := backtest.Result{
		ClosedTrades:   40,
		WinningTrades:  22,
		HitRate:        0.55,
		NetPnl:         decimal.NewFromInt(1234),
		RealizedPnl:    decimal.NewFromInt(1000),
		Sharpe:         1.1,
		Sortino:        1.4,
		Calmar:         0.9,
		CAGR:           12.5,
		MaxDrawdownPct: 3.2,
		MaxDrawdownRub: decimal.NewFromInt(5000),
	}
	m := MetricsFromResult("2024-04-01_2024-05-01", spec, r)
	if !m.Significant {
		t.Fatalf("expected Significant=true for %d closed trades", r.ClosedTrades)
	}
	if m.NetPnl != "1234.00" || m.RealizedPnl != "1000.00" {
		t.Fatalf("NetPnl/RealizedPnl = %q/%q, want 1234.00/1000.00", m.NetPnl, m.RealizedPnl)
	}

	dir := filepath.Join(t.TempDir(), m.WindowID)
	if err := SaveMetrics(dir, m); err != nil {
		t.Fatalf("SaveMetrics() error = %v", err)
	}
	loaded, err := LoadMetrics(dir)
	if err != nil {
		t.Fatalf("LoadMetrics() error = %v", err)
	}
	if loaded != m {
		t.Fatalf("round-tripped metrics = %+v, want %+v", loaded, m)
	}
}

func TestMetricsFromResult_BelowThresholdNotSignificant(t *testing.T) {
	spec := WindowSpec{}
	r := backtest.Result{ClosedTrades: backtest.MinTradesForSignificance - 1}
	m := MetricsFromResult("w", spec, r)
	if m.Significant {
		t.Fatalf("expected Significant=false for %d closed trades", r.ClosedTrades)
	}
}
