package main

import "math"

func normalCDF(x float64) float64 {
	return 0.5 * (1 + math.Erf(x/math.Sqrt2))
}

func normalInvCDF(p float64) float64 {
	return math.Sqrt2 * math.Erfinv(2*p-1)
}

func skewness(v []float64) float64 {
	n := float64(len(v))
	if n < 3 {
		return math.NaN()
	}
	m := mean(v)
	s := sd(v)
	var sum float64
	for _, x := range v {
		sum += math.Pow((x-m)/s, 3)
	}
	return sum / n
}

func kurtosis(v []float64) float64 {
	n := float64(len(v))
	if n < 4 {
		return math.NaN()
	}
	m := mean(v)
	s := sd(v)
	var sum float64
	for _, x := range v {
		sum += math.Pow((x-m)/s, 4)
	}
	return sum / n
}

func deflatedSharpe(v []float64, trials int) (float64, float64) {
	if len(v) < 30 || trials < 1 {
		return math.NaN(), math.NaN()
	}
	s := sd(v)
	if s == 0 {
		return math.NaN(), math.NaN()
	}
	sr := mean(v) / s
	n := float64(len(v))
	g3 := skewness(v)
	g4 := kurtosis(v)
	varSR := (1 - g3*sr + (g4-1)/4*sr*sr) / (n - 1)
	if varSR <= 0 {
		return math.NaN(), sr
	}
	e := math.E
	gamma := 0.5772156649
	nt := float64(trials)
	sr0 := math.Sqrt(varSR) * ((1-gamma)*normalInvCDF(1-1/nt) + gamma*normalInvCDF(1-1/(nt*e)))
	dsr := normalCDF((sr - sr0) / math.Sqrt(varSR))
	return dsr, sr
}
