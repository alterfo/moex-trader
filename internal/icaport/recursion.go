package icaport

import (
	"math"
	"sort"
)

type Recursion struct {
	Omega  float64
	Alpha  float64
	Beta   float64
	LastX  float64
	LastXi float64
	Fitted []float64
}

func (r Recursion) Persistence() float64 {
	return r.Alpha + r.Beta
}

func filter(x []float64, omega, alpha, beta float64) ([]float64, float64) {
	xi := make([]float64, len(x))
	var mean float64
	for _, v := range x {
		mean += v
	}
	mean /= float64(len(x))
	xi[0] = mean
	var sse float64
	for t := 1; t < len(x); t++ {
		xi[t] = omega + alpha*x[t-1] + beta*xi[t-1]
		d := x[t] - xi[t]
		sse += d * d
	}
	return xi, sse
}

func feasible(power int, omega, alpha, beta float64) bool {
	if beta < 0 || beta >= 0.999 || math.Abs(alpha)+beta >= 0.999 {
		return false
	}
	if power%2 == 0 {
		return omega > 0 && alpha >= 0
	}
	return math.Abs(alpha) < 1
}

func FitRecursion(x []float64, power int) Recursion {
	var mean float64
	for _, v := range x {
		mean += v
	}
	mean /= float64(len(x))
	start := []float64{mean * 0.05, 0.05, 0.9}
	if power%2 == 0 && start[0] <= 0 {
		start[0] = 1e-6
	}
	objective := func(p []float64) float64 {
		if !feasible(power, p[0], p[1], p[2]) {
			return math.Inf(1)
		}
		_, sse := filter(x, p[0], p[1], p[2])
		return sse
	}
	best := NelderMead(objective, start, 0.3, 600)
	if math.IsInf(objective(best), 1) {
		best = []float64{mean * 0.05, 0, 0}
		if power%2 == 0 && best[0] <= 0 {
			best[0] = 1e-6
		}
	}
	xi, _ := filter(x, best[0], best[1], best[2])
	return Recursion{
		Omega: best[0], Alpha: best[1], Beta: best[2],
		LastX: x[len(x)-1], LastXi: xi[len(xi)-1], Fitted: xi,
	}
}

func (r Recursion) ForecastAverage(horizon int) float64 {
	xi1 := r.Omega + r.Alpha*r.LastX + r.Beta*r.LastXi
	rho := r.Persistence()
	if rho >= 1 || horizon <= 0 {
		return xi1
	}
	mu := r.Omega / (1 - rho)
	var sum float64
	decay := 1.0
	for h := 1; h <= horizon; h++ {
		sum += mu + decay*(xi1-mu)
		decay *= rho
	}
	return sum / float64(horizon)
}

func Quantile(values []float64, q float64) float64 {
	if len(values) == 0 {
		return 0
	}
	s := append([]float64(nil), values...)
	sort.Float64s(s)
	pos := q * float64(len(s)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return s[lo]
	}
	return s[lo] + (s[hi]-s[lo])*(pos-float64(lo))
}
