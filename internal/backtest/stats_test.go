package backtest

import (
	"math"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func mustCloseTo(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Fatalf("%s: got %v, want %v (tol %v)", name, got, want, tol)
	}
}

func TestCurveStats_FlatCurveHasZeroRiskMetrics(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	points := []EquityPoint{
		{Date: start, Equity: decimal.NewFromInt(1_000_000)},
		{Date: start.AddDate(0, 0, 1), Equity: decimal.NewFromInt(1_000_000)},
		{Date: start.AddDate(0, 0, 2), Equity: decimal.NewFromInt(1_000_000)},
	}
	sharpe, sortino, cagr, ddPct, ddRub := CurveStats(points)
	mustCloseTo(t, "sharpe", sharpe, 0, 1e-9)
	mustCloseTo(t, "sortino", sortino, 0, 1e-9)
	mustCloseTo(t, "cagr", cagr, 0, 1e-9)
	mustCloseTo(t, "maxDDPct", ddPct, 0, 1e-9)
	if !ddRub.IsZero() {
		t.Fatalf("maxDDRub: got %v, want 0", ddRub)
	}
}

func TestCurveStats_MonotonicGrowthPositiveSharpeAndCAGR(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	equity := []int64{1_000_000, 1_010_000, 1_020_100, 1_030_301, 1_040_604}
	points := make([]EquityPoint, len(equity))
	for i, v := range equity {
		points[i] = EquityPoint{Date: start.AddDate(0, 0, i), Equity: decimal.NewFromInt(v)}
	}
	sharpe, sortino, cagr, ddPct, ddRub := CurveStats(points)
	if sharpe <= 0 {
		t.Fatalf("sharpe: got %v, want > 0", sharpe)
	}
	if sortino != 0 {
		t.Fatalf("sortino: got %v, want 0 (no negative returns)", sortino)
	}
	if cagr <= 0 {
		t.Fatalf("cagr: got %v, want > 0", cagr)
	}
	mustCloseTo(t, "maxDDPct", ddPct, 0, 1e-9)
	if !ddRub.IsZero() {
		t.Fatalf("maxDDRub: got %v, want 0", ddRub)
	}
}

func TestCurveStats_DrawdownAndNegativeSortino(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	equity := []int64{1_000_000, 1_100_000, 900_000, 950_000}
	points := make([]EquityPoint, len(equity))
	for i, v := range equity {
		points[i] = EquityPoint{Date: start.AddDate(0, 0, i), Equity: decimal.NewFromInt(v)}
	}
	_, sortino, _, ddPct, ddRub := CurveStats(points)
	if sortino >= 0 {
		t.Fatalf("sortino: got %v, want < 0 (drawdown present)", sortino)
	}
	wantDDPct := 200000.0 / 1100000.0 * 100
	mustCloseTo(t, "maxDDPct", ddPct, wantDDPct, 1e-6)
	wantDDRub := decimal.NewFromInt(200000)
	if !ddRub.Equal(wantDDRub) {
		t.Fatalf("maxDDRub: got %v, want %v", ddRub, wantDDRub)
	}
}

func TestCurveStats_TooFewPointsReturnsZeroValues(t *testing.T) {
	sharpe, sortino, cagr, ddPct, ddRub := CurveStats(nil)
	if sharpe != 0 || sortino != 0 || cagr != 0 || ddPct != 0 || !ddRub.IsZero() {
		t.Fatalf("expected all-zero stats for empty curve, got sharpe=%v sortino=%v cagr=%v ddPct=%v ddRub=%v", sharpe, sortino, cagr, ddPct, ddRub)
	}
	single := []EquityPoint{{Date: time.Now(), Equity: decimal.NewFromInt(1000)}}
	sharpe, sortino, cagr, ddPct, ddRub = CurveStats(single)
	if sharpe != 0 || sortino != 0 || cagr != 0 || ddPct != 0 || !ddRub.IsZero() {
		t.Fatalf("expected all-zero stats for single-point curve, got sharpe=%v sortino=%v cagr=%v ddPct=%v ddRub=%v", sharpe, sortino, cagr, ddPct, ddRub)
	}
}

func TestComputeCAGR_KnownDoubling(t *testing.T) {
	start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	points := []EquityPoint{
		{Date: start, Equity: decimal.NewFromInt(1_000_000)},
		{Date: start.AddDate(1, 0, 0), Equity: decimal.NewFromInt(2_000_000)},
	}
	got := computeCAGR(points)
	mustCloseTo(t, "cagr", got, 100.0, 1.0)
}

func TestMeanStd_SampleConvention(t *testing.T) {
	mean, std := meanStd([]float64{1, 2, 3, 4})
	mustCloseTo(t, "mean", mean, 2.5, 1e-9)
	mustCloseTo(t, "std", std, math.Sqrt(5.0/3.0), 1e-9)

	mean, std = meanStd([]float64{7})
	mustCloseTo(t, "mean_single", mean, 7, 1e-9)
	mustCloseTo(t, "std_single", std, 0, 1e-9)

	mean, std = meanStd(nil)
	mustCloseTo(t, "mean_empty", mean, 0, 1e-9)
	mustCloseTo(t, "std_empty", std, 0, 1e-9)
}

func TestResult_StatisticallySignificant(t *testing.T) {
	r := Result{ClosedTrades: MinTradesForSignificance - 1}
	if r.StatisticallySignificant() {
		t.Fatalf("expected not significant at %d trades", r.ClosedTrades)
	}
	r.ClosedTrades = MinTradesForSignificance
	if !r.StatisticallySignificant() {
		t.Fatalf("expected significant at %d trades", r.ClosedTrades)
	}
}
