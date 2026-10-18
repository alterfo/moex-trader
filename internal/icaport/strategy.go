package icaport

import (
	"errors"
	"math"
)

type ICAConfig struct {
	Horizon  int
	Rho      float64
	Higher   bool
	UseMean  bool
	Seed     int64
	Restarts int
}

func EqualWeights(n int) []float64 {
	w := make([]float64, n)
	for i := range w {
		w[i] = 1 / float64(n)
	}
	return w
}

func InverseVolWeights(returns Mat, lookback int) []float64 {
	t := len(returns)
	n := len(returns[0])
	from := t - lookback
	if from < 0 {
		from = 0
	}
	w := make([]float64, n)
	var sum float64
	for j := 0; j < n; j++ {
		var m, v float64
		count := float64(t - from)
		for i := from; i < t; i++ {
			m += returns[i][j]
		}
		m /= count
		for i := from; i < t; i++ {
			d := returns[i][j] - m
			v += d * d
		}
		std := math.Sqrt(v / (count - 1))
		if std < 1e-12 {
			std = 1e-12
		}
		w[j] = 1 / std
		sum += w[j]
	}
	for j := range w {
		w[j] /= sum
	}
	return w
}

func MinVarianceWeights(returns Mat, seed int64) []float64 {
	cov := Covariance(returns)
	n := len(cov)
	starts := DefaultStarts(n, seed, 3, InverseVolWeights(returns, len(returns)))
	return MinimizeSimplex(Quadratic{Sigma: cov}, starts, 2000)
}

type ICAFit struct {
	Objective CARA
	Phi       Mat
}

func BuildICAObjective(returns Mat, cfg ICAConfig) (CARA, error) {
	if len(returns) < 100 {
		return CARA{}, errors.New("icaport: window too short")
	}
	n := len(returns[0])
	y, _ := Demean(returns)
	var1, resid, err := FitVAR1(y)
	if err != nil {
		return CARA{}, err
	}
	ica, err := FastICA(resid, cfg.Seed)
	if err != nil {
		return CARA{}, err
	}
	comps := Components(resid, ica.U)
	cum := Cumulants{K2: make([]float64, n), K3: make([]float64, n), K4: make([]float64, n)}
	for l := 0; l < n; l++ {
		x2 := make([]float64, len(comps))
		x3 := make([]float64, len(comps))
		x4 := make([]float64, len(comps))
		for t := range comps {
			v := comps[t][l]
			x2[t] = v * v
			x3[t] = v * v * v
			x4[t] = v * v * v * v
		}
		h := FitRecursion(x2, 2)
		cum.K2[l] = h.ForecastAverage(cfg.Horizon)
		if cum.K2[l] < 1e-12 {
			cum.K2[l] = 1e-12
		}
		if !cfg.Higher {
			continue
		}
		s := FitRecursion(x3, 3)
		k := FitRecursion(x4, 4)
		k3 := s.ForecastAverage(cfg.Horizon)
		capS := Quantile(absAll(s.Fitted), 0.99)
		cum.K3[l] = clamp(k3, -capS, capS)
		k4 := k.ForecastAverage(cfg.Horizon) - 3*cum.K2[l]*cum.K2[l]
		excess := make([]float64, len(k.Fitted))
		for t := range k.Fitted {
			excess[t] = k.Fitted[t] - 3*h.Fitted[t]*h.Fitted[t]
		}
		capK := math.Max(Quantile(excess, 0.99), 0)
		cum.K4[l] = clamp(k4, 0, capK)
	}
	var mu []float64
	if cfg.UseMean {
		mu = var1.Forecast(y[len(y)-1])
	}
	return CARA{B: ica.B, Mu: mu, C: cum, Rho: cfg.Rho}, nil
}

func ICAWeights(returns Mat, cfg ICAConfig) ([]float64, error) {
	obj, err := BuildICAObjective(returns, cfg)
	if err != nil {
		return nil, err
	}
	n := len(returns[0])
	restarts := cfg.Restarts
	if restarts <= 0 {
		restarts = 3
	}
	starts := DefaultStarts(n, cfg.Seed, restarts, InverseVolWeights(returns, 63))
	return MinimizeSimplex(obj, starts, 1500), nil
}

func absAll(v []float64) []float64 {
	out := make([]float64, len(v))
	for i, x := range v {
		out[i] = math.Abs(x)
	}
	return out
}

func clamp(v, lo, hi float64) float64 {
	return math.Max(lo, math.Min(hi, v))
}
