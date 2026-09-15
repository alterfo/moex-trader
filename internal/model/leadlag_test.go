package model

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

func syntheticCandles(n int, price func(i int) float64) []moex.Candle {
	candles := make([]moex.Candle, n)
	for i := range n {
		p := price(i)
		candles[i] = moex.Candle{
			Open:   decimal.NewFromFloat(p),
			Close:  decimal.NewFromFloat(p),
			High:   decimal.NewFromFloat(p + 1),
			Low:    decimal.NewFromFloat(p - 1),
			Volume: decimal.NewFromInt(1000),
			Begin:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i),
			End:    time.Date(2024, 1, 1, 18, 0, 0, 0, time.UTC).AddDate(0, 0, i),
		}
	}
	return candles
}

func TestLogReturnSeries(t *testing.T) {
	candles := syntheticCandles(3, func(i int) float64 { return 100 * math.Pow(1.01, float64(i)) })
	returns := LogReturnSeries(candles)
	if len(returns) != 2 {
		t.Fatalf("got %d returns, want 2", len(returns))
	}
	for _, r := range returns {
		if math.Abs(r-math.Log(1.01)) > 1e-9 {
			t.Fatalf("return = %v, want %v", r, math.Log(1.01))
		}
	}
}

func lcgSeries(n int, seed uint64) []float64 {
	out := make([]float64, n)
	for i := range n {
		seed = seed*6364136223846793005 + 1442695040888963407
		out[i] = (float64(seed>>11) / (1 << 53)) - 0.5
	}
	return out
}

func TestCrossCorrelationDetectsLead(t *testing.T) {
	n := 250
	signal := lcgSeries(n, 42)
	noise := lcgSeries(n, 7)
	x := make(map[string]float64, n)
	y := make(map[string]float64, n)
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range n {
		x[dateKey(base.AddDate(0, 0, i))] = signal[i]
	}
	for i := 2; i < n; i++ {
		// y mostly tracks x from 2 days earlier, plus independent noise, so
		// the relationship is detectable but not perfectly collinear.
		y[dateKey(base.AddDate(0, 0, i))] = 0.8*signal[i-2] + 0.2*noise[i]
	}

	cc := CrossCorrelation(x, y, 5, 30)
	leadCorr := cc.Lead[2]
	if leadCorr < 0.7 {
		t.Fatalf("lead correlation at k=2 = %v, want > 0.7 (y mostly tracks x shifted forward by 2 days)", leadCorr)
	}
	if math.Abs(cc.Lag[2]) > 0.3 {
		t.Fatalf("reverse correlation at k=2 = %v, want close to 0 (no relationship in the reverse direction)", cc.Lag[2])
	}
}

func TestFSurvivalMatchesKnownCriticalValues(t *testing.T) {
	tests := []struct {
		f, d1, d2, wantP float64
	}{
		{f: 4.96, d1: 1, d2: 10, wantP: 0.05},
		{f: 3.49, d1: 2, d2: 20, wantP: 0.05},
		{f: 1.0, d1: 5, d2: 50, wantP: 0.427},
	}
	for _, tt := range tests {
		got := fSurvival(tt.f, tt.d1, tt.d2)
		if math.Abs(got-tt.wantP) > 0.01 {
			t.Fatalf("fSurvival(%v, %v, %v) = %v, want ~%v", tt.f, tt.d1, tt.d2, got, tt.wantP)
		}
	}
}

func TestFSurvivalMonotonicallyDecreasing(t *testing.T) {
	prev := 1.0
	for f := 0.5; f <= 10; f += 0.5 {
		p := fSurvival(f, 3, 40)
		if p > prev {
			t.Fatalf("fSurvival not monotonically decreasing: f=%v p=%v > prev=%v", f, p, prev)
		}
		prev = p
	}
}

func TestGrangerFTestDetectsCausality(t *testing.T) {
	n := 300
	x := make(map[string]float64, n)
	y := make(map[string]float64, n)
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	signal := lcgSeries(n, 111)
	noise := lcgSeries(n, 222)
	for i := range n {
		x[dateKey(base.AddDate(0, 0, i))] = signal[i]
	}
	for i := 1; i < n; i++ {
		// y mostly tracks x from 1 day earlier, plus independent noise, so
		// the regression isn't exactly collinear (a real causal relationship
		// always has some idiosyncratic noise; a noise-free deterministic
		// lag makes the unrestricted design matrix singular).
		y[dateKey(base.AddDate(0, 0, i))] = 0.9*signal[i-1] + 0.1*noise[i]
	}

	result, ok := GrangerFTest(x, y, 2, 30)
	if !ok {
		t.Fatal("GrangerFTest() ok = false, want true")
	}
	if result.PValue > 0.01 {
		t.Fatalf("GrangerFTest() p-value = %v, want < 0.01 (x's past strongly predicts y)", result.PValue)
	}
}

func TestGrangerFTestNoRelationship(t *testing.T) {
	n := 200
	x := make(map[string]float64, n)
	y := make(map[string]float64, n)
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	seed := uint64(12345)
	nextRand := func() float64 {
		seed = seed*6364136223846793005 + 1442695040888963407
		return (float64(seed>>11) / (1 << 53)) - 0.5
	}
	for i := range n {
		x[dateKey(base.AddDate(0, 0, i))] = nextRand()
		y[dateKey(base.AddDate(0, 0, i))] = nextRand()
	}

	result, ok := GrangerFTest(x, y, 2, 30)
	if !ok {
		t.Fatal("GrangerFTest() ok = false, want true")
	}
	if result.PValue < 0.01 {
		t.Fatalf("GrangerFTest() p-value = %v on independent noise series, want a large p-value (no real causality)", result.PValue)
	}
}

func TestFindLeadersRanksAsymmetricLeader(t *testing.T) {
	n := 250
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	signal := lcgSeries(n, 98765)
	idiosyncratic := lcgSeries(n, 13579)
	noise := lcgSeries(n, 24680)

	leaderReturns := make(map[string]float64, n)
	followerReturns := make(map[string]float64, n)
	noiseReturns := make(map[string]float64, n)
	for i := range n {
		d := dateKey(base.AddDate(0, 0, i))
		leaderReturns[d] = signal[i]
		if i >= 1 {
			// FOLLOWER mostly tracks LEADER from 1 day earlier, plus its own
			// idiosyncratic noise (a noise-free deterministic lag makes the
			// Granger regression exactly collinear).
			followerReturns[d] = 0.9*signal[i-1] + 0.1*idiosyncratic[i]
		}
		noiseReturns[d] = noise[i]
	}

	returns := map[string]map[string]float64{
		"LEADER":   leaderReturns,
		"FOLLOWER": followerReturns,
		"NOISE":    noiseReturns,
	}
	results, bonferroniAlpha, nTests := FindLeaders(returns, []string{"FOLLOWER"}, []string{"LEADER", "NOISE"}, 5, 2, 30, 0.0)
	if nTests != 2 {
		t.Fatalf("nTests = %d, want 2", nTests)
	}
	if bonferroniAlpha <= 0 || bonferroniAlpha >= 0.05 {
		t.Fatalf("bonferroniAlpha = %v, want in (0, 0.05)", bonferroniAlpha)
	}
	if len(results) == 0 {
		t.Fatal("got 0 results")
	}
	if results[0].Candidate != "LEADER" {
		t.Fatalf("top result candidate = %q, want LEADER (the true lead-lag relationship should rank first)", results[0].Candidate)
	}
	if !results[0].BonferroniSignificant && !results[0].FDRSignificant {
		t.Fatal("LEADER->FOLLOWER relationship did not survive any multiple-comparison correction")
	}
}

func TestWindowedRobustnessDetectsStableRelationship(t *testing.T) {
	n := 400
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	signal := lcgSeries(n, 555)
	idiosyncratic := lcgSeries(n, 666)

	x := make(map[string]float64, n)
	y := make(map[string]float64, n)
	for i := range n {
		x[dateKey(base.AddDate(0, 0, i))] = signal[i]
	}
	for i := 1; i < n; i++ {
		y[dateKey(base.AddDate(0, 0, i))] = 0.9*signal[i-1] + 0.1*idiosyncratic[i]
	}

	windows := WindowedRobustness(x, y, 1, 150, 2, 30)
	if len(windows) < 2 {
		t.Fatalf("got %d windows, want at least 2 non-overlapping windows", len(windows))
	}
	for _, w := range windows {
		if !w.HasLeadCorr || w.LeadCorr < 0.7 {
			t.Fatalf("window %s..%s: lead corr = %v (has=%v), want > 0.7 in every window for a stable relationship", w.Start, w.End, w.LeadCorr, w.HasLeadCorr)
		}
		if !w.HasGrangerP || w.GrangerP > 0.01 {
			t.Fatalf("window %s..%s: granger p = %v (has=%v), want < 0.01 in every window", w.Start, w.End, w.GrangerP, w.HasGrangerP)
		}
	}
}

func TestWindowedRobustnessNoWindowsWhenHistoryTooShort(t *testing.T) {
	n := 50
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	signal := lcgSeries(n, 1)
	x := make(map[string]float64, n)
	y := make(map[string]float64, n)
	for i := range n {
		x[dateKey(base.AddDate(0, 0, i))] = signal[i]
		y[dateKey(base.AddDate(0, 0, i))] = signal[i]
	}
	windows := WindowedRobustness(x, y, 1, 150, 2, 30)
	if len(windows) != 0 {
		t.Fatalf("got %d windows, want 0 (history shorter than one window)", len(windows))
	}
}

func TestBuildReturnUniverseSkipsFailingTickers(t *testing.T) {
	source := datasetSource{series: map[string][]moex.Candle{
		"GOOD":  syntheticCandles(120, func(i int) float64 { return 100 + float64(i) }),
		"IMOEX": syntheticCandles(120, func(i int) float64 { return 3000 }),
	}}
	returns, err := BuildReturnUniverse(context.Background(), source, []string{"GOOD", "MISSING"},
		time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("BuildReturnUniverse failed: %v", err)
	}
	if _, ok := returns["GOOD"]; !ok {
		t.Fatal("GOOD ticker missing from return universe")
	}
	if _, ok := returns["MISSING"]; ok {
		t.Fatal("MISSING ticker (no fixture data) should have been skipped, not present")
	}
	if _, ok := returns[benchmarkTicker]; !ok {
		t.Fatal("IMOEX benchmark missing from return universe")
	}
}

func TestBuildReturnUniverseFailsWithoutBenchmark(t *testing.T) {
	source := datasetSource{series: map[string][]moex.Candle{
		"GOOD": syntheticCandles(120, func(i int) float64 { return 100 + float64(i) }),
	}}
	_, err := BuildReturnUniverse(context.Background(), source, []string{"GOOD"},
		time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("missing IMOEX benchmark did not return an error")
	}
}
