package strategyvalidation

import "math"

const HarveyLiuT = 3.5

const eulerMascheroni = 0.5772156649015328606

type SharpeDecomposition struct {
	Mean                float64
	StdDev              float64
	Sharpe              float64
	Skewness            float64
	Kurtosis            float64
	TStat               float64
	ProbabilisticSharpe float64
	PValue              float64
}

type DeflatedSharpeResult struct {
	Sharpe            float64
	ExpectedMaxSharpe float64
	Benchmark         float64
	DeflatedSharpe    float64
	PValue            float64
}

func meanSampleStd(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}
	var sum float64
	for _, value := range values {
		sum += value
	}
	mean := sum / float64(len(values))
	if len(values) < 2 {
		return mean, 0
	}
	var squares float64
	for _, value := range values {
		diff := value - mean
		squares += diff * diff
	}
	return mean, math.Sqrt(squares / float64(len(values)-1))
}

func SharpeRatio(returns []float64) float64 {
	if len(returns) < 2 {
		return 0
	}
	mean, std := meanSampleStd(returns)
	if std < 1e-12 {
		return 0
	}
	return mean / std
}

func skewness(returns []float64, mean, std float64) float64 {
	if len(returns) == 0 || std < 1e-12 {
		return 0
	}
	var sum float64
	for _, value := range returns {
		z := (value - mean) / std
		sum += z * z * z
	}
	return sum / float64(len(returns))
}

func kurtosis(returns []float64, mean, std float64) float64 {
	if len(returns) == 0 || std < 1e-12 {
		return 0
	}
	var sum float64
	for _, value := range returns {
		z := (value - mean) / std
		sum += z * z * z * z
	}
	return sum / float64(len(returns))
}

func normCDF(x float64) float64 {
	return 0.5 * math.Erfc(-x/math.Sqrt2)
}

func normQuantile(p float64) float64 {
	if p <= 0 {
		return math.Inf(-1)
	}
	if p >= 1 {
		return math.Inf(1)
	}
	if p < 0.5 {
		return math.Sqrt2 * math.Erfinv(2*p-1)
	}
	return -math.Sqrt2 * math.Erfcinv(2*p)
}

func ExpectedMaxSharpe(nTrials, nObservations int) float64 {
	if nTrials <= 1 || nObservations < 2 {
		return 0
	}
	variance := 1 / float64(nObservations-1)
	dispersion := math.Sqrt(variance)
	trials := float64(nTrials)
	z1 := normQuantile(1 - 1/trials)
	z2 := normQuantile(1 - 1/(trials*math.E))
	return dispersion * ((1-eulerMascheroni)*z1 + eulerMascheroni*z2)
}

func HarveyLiuSharpeThreshold(nObservations int) float64 {
	if nObservations < 2 {
		return 0
	}
	return HarveyLiuT / math.Sqrt(float64(nObservations-1))
}

func ProbabilisticSharpe(returns []float64, benchmark float64) SharpeDecomposition {
	result := SharpeDecomposition{}
	if len(returns) < 2 {
		return result
	}
	mean, std := meanSampleStd(returns)
	result.Mean = mean
	result.StdDev = std
	if std < 1e-12 {
		result.PValue = 1
		return result
	}
	result.Sharpe = mean / std
	result.Skewness = skewness(returns, mean, std)
	result.Kurtosis = kurtosis(returns, mean, std)
	term := 1 - result.Skewness*result.Sharpe + ((result.Kurtosis-1)/4)*result.Sharpe*result.Sharpe
	if term < 1e-12 {
		term = 1e-12
	}
	result.TStat = (result.Sharpe - benchmark) * math.Sqrt(float64(len(returns)-1)) / math.Sqrt(term)
	result.ProbabilisticSharpe = normCDF(result.TStat)
	result.PValue = 1 - result.ProbabilisticSharpe
	return result
}

func DeflatedSharpe(returns []float64, nTrials int, nullMean float64) DeflatedSharpeResult {
	expectedMax := ExpectedMaxSharpe(nTrials, len(returns))
	benchmark := nullMean + expectedMax
	psr := ProbabilisticSharpe(returns, benchmark)
	return DeflatedSharpeResult{
		Sharpe:            psr.Sharpe,
		ExpectedMaxSharpe: expectedMax,
		Benchmark:         benchmark,
		DeflatedSharpe:    psr.ProbabilisticSharpe,
		PValue:            psr.PValue,
	}
}
