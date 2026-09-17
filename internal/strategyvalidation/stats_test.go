package strategyvalidation

import (
	"math"
	"testing"
)

func TestExpectedMaxSharpeSingleTrialIsZero(t *testing.T) {
	if got := ExpectedMaxSharpe(1, 100); got != 0 {
		t.Fatalf("ExpectedMaxSharpe(1,100) = %.6f, want 0", got)
	}
	if got := ExpectedMaxSharpe(5, 1); got != 0 {
		t.Fatalf("ExpectedMaxSharpe(5,1) = %.6f, want 0", got)
	}
}

func TestExpectedMaxSharpeIncreasesWithTrials(t *testing.T) {
	small := ExpectedMaxSharpe(2, 100)
	large := ExpectedMaxSharpe(50, 100)
	if small <= 0 || large <= small {
		t.Fatalf("ExpectedMaxSharpe(2,100)=%.4f, ExpectedMaxSharpe(50,100)=%.4f, want positive and increasing", small, large)
	}
}

func TestHarveyLiuSharpeThreshold(t *testing.T) {
	if got := HarveyLiuSharpeThreshold(1); got != 0 {
		t.Fatalf("HarveyLiuSharpeThreshold(1) = %.4f, want 0", got)
	}
	want := HarveyLiuT / math.Sqrt(5)
	if got := HarveyLiuSharpeThreshold(6); math.Abs(got-want) > 1e-12 {
		t.Fatalf("HarveyLiuSharpeThreshold(6) = %.6f, want %.6f", got, want)
	}
}

func TestSharpeRatioKnown(t *testing.T) {
	returns := []float64{0.02, -0.01, 0.03, 0.01}
	mean := 0.0125
	var squares float64
	for _, value := range returns {
		diff := value - mean
		squares += diff * diff
	}
	want := mean / math.Sqrt(squares/3)
	if got := SharpeRatio(returns); math.Abs(got-want) > 1e-12 {
		t.Fatalf("SharpeRatio = %.6f, want %.6f", got, want)
	}
}

func TestConstantReturnsHaveZeroSharpeAndPSR(t *testing.T) {
	returns := []float64{0.02, 0.02, 0.02, 0.02}
	if got := SharpeRatio(returns); got != 0 {
		t.Fatalf("SharpeRatio constant = %.6f, want 0", got)
	}
	psr := ProbabilisticSharpe(returns, 0)
	if psr.Sharpe != 0 || psr.ProbabilisticSharpe != 0 || psr.PValue != 1 {
		t.Fatalf("constant PSR = %+v, want zero Sharpe/PSR and p-value 1", psr)
	}
	dsr := DeflatedSharpe(returns, 20, 0)
	if dsr.DeflatedSharpe != 0 {
		t.Fatalf("constant DSR = %.6f, want 0", dsr.DeflatedSharpe)
	}
}

func TestSmallSamplesReturnZero(t *testing.T) {
	for _, returns := range [][]float64{nil, {}, {0.01}} {
		if got := SharpeRatio(returns); got != 0 {
			t.Fatalf("SharpeRatio(%v) = %.6f, want 0", returns, got)
		}
		if got := ProbabilisticSharpe(returns, 0).ProbabilisticSharpe; got != 0 {
			t.Fatalf("ProbabilisticSharpe(%v) = %.6f, want 0", returns, got)
		}
	}
	if got := DeflatedSharpe(nil, 20, 0).DeflatedSharpe; got != 0 {
		t.Fatalf("DeflatedSharpe(nil) = %.6f, want 0", got)
	}
}

func TestNormQuantileKnown(t *testing.T) {
	want := 1.959963984540054
	if got := normQuantile(0.975); math.Abs(got-want) > 1e-9 {
		t.Fatalf("normQuantile(0.975) = %.12f, want %.12f", got, want)
	}
}

func TestDeflatedSharpeNeverExceedsProbabilistic(t *testing.T) {
	returns := []float64{0.01, -0.005, 0.02, -0.01, 0.015, 0.005, 0.02, -0.01, 0.01, 0.005}
	probabilistic := ProbabilisticSharpe(returns, 0).ProbabilisticSharpe
	deflated := DeflatedSharpe(returns, 100, 0).DeflatedSharpe
	if deflated > probabilistic+1e-12 {
		t.Fatalf("deflated Sharpe %.6f exceeds probabilistic Sharpe %.6f", deflated, probabilistic)
	}
	if DeflatedSharpe(returns, 100, 0).Benchmark <= 0 {
		t.Fatalf("deflated benchmark = %.6f, want positive", DeflatedSharpe(returns, 100, 0).Benchmark)
	}
}

func TestNegativeSkewFatTailsLowerPSR(t *testing.T) {
	symmetric := []float64{0.02, -0.01, 0.03, -0.02, 0.02, -0.01, 0.03, -0.02, 0.02, -0.01, 0.03, -0.02}
	negativeSkew := []float64{0.02, 0.01, 0.03, 0.02, 0.02, 0.01, 0.03, 0.02, 0.02, 0.01, 0.03, -0.25}
	good := ProbabilisticSharpe(symmetric, 0)
	bad := ProbabilisticSharpe(negativeSkew, 0)
	if bad.Skewness >= good.Skewness {
		t.Fatalf("negative-skew series skewness %.4f is not below symmetric %.4f", bad.Skewness, good.Skewness)
	}
	if bad.Kurtosis <= good.Kurtosis {
		t.Fatalf("negative-skew series kurtosis %.4f is not above symmetric %.4f", bad.Kurtosis, good.Kurtosis)
	}
	if bad.ProbabilisticSharpe >= good.ProbabilisticSharpe {
		t.Fatalf("negative-skew PSR %.6f is not below symmetric PSR %.6f", bad.ProbabilisticSharpe, good.ProbabilisticSharpe)
	}
}
