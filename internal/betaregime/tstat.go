package betaregime

import "math"

const (
	tstatMaxIter = 200
	tstatEPS     = 3.0e-14
	tstatFPMin   = 1.0e-300
)

var lanczosCoeffs = [9]float64{
	0.99999999999980993,
	676.5203681218851,
	-1259.1392167224028,
	771.32342877765313,
	-176.61502916214059,
	12.507343278686905,
	-0.13857109526572012,
	9.9843695780195716e-6,
	1.5056327351493116e-7,
}

func lgamma(x float64) float64 {
	if x < 0.5 {
		return math.Log(math.Pi/math.Sin(math.Pi*x)) - lgamma(1-x)
	}
	x -= 1
	base := 0.99999999999980993
	series := 0.0
	for i := 1; i < len(lanczosCoeffs); i++ {
		series += lanczosCoeffs[i] / (x + float64(i))
	}
	t := x + 7.5
	return 0.5*math.Log(2*math.Pi) + (x+0.5)*math.Log(t) - t + math.Log(base+series)
}

// betacf evaluates the continued fraction used by the regularised incomplete
// beta function (Numerical Recipes formulation).
func betacf(a, b, x float64) float64 {
	qab := a + b
	qap := a + 1
	qam := a - 1
	c := 1.0
	d := 1 - qab*x/qap
	if math.Abs(d) < tstatFPMin {
		d = tstatFPMin
	}
	d = 1 / d
	h := d
	for m := 1; m <= tstatMaxIter; m++ {
		m2 := 2 * m
		aa := float64(m) * (b - float64(m)) * x / ((qam + float64(m2)) * (a + float64(m2)))
		d = 1 + aa*d
		if math.Abs(d) < tstatFPMin {
			d = tstatFPMin
		}
		c = 1 + aa/c
		if math.Abs(c) < tstatFPMin {
			c = tstatFPMin
		}
		d = 1 / d
		h *= d * c

		aa = -(a + float64(m)) * (qab + float64(m)) * x / ((a + float64(m2)) * (qap + float64(m2)))
		d = 1 + aa*d
		if math.Abs(d) < tstatFPMin {
			d = tstatFPMin
		}
		c = 1 + aa/c
		if math.Abs(c) < tstatFPMin {
			c = tstatFPMin
		}
		d = 1 / d
		del := d * c
		h *= del
		if math.Abs(del-1) < tstatEPS {
			return h
		}
	}
	return h
}

// RegularizedIncompleteBeta returns I_x(a,b) = B_x(a,b)/B(a,b).
func RegularizedIncompleteBeta(a, b, x float64) float64 {
	if x <= 0 {
		return 0
	}
	if x >= 1 {
		return 1
	}
	bt := math.Exp(lgamma(a+b) - lgamma(a) - lgamma(b) + a*math.Log(x) + b*math.Log(1-x))
	if x < (a+1)/(a+b+2) {
		return bt * betacf(a, b, x) / a
	}
	return 1 - bt*betacf(b, a, 1-x)/b
}

// StudentTCDF returns P(T_df <= t). It is two-sided-safe: PValue for a
// two-sided test of |t| is 2*(1 - StudentTCDF(|t|, df)).
func StudentTCDF(t, df float64) float64 {
	if df <= 0 || math.IsNaN(t) {
		return math.NaN()
	}
	if math.IsInf(t, 1) {
		return 1
	}
	if math.IsInf(t, -1) {
		return 0
	}
	x := df / (df + t*t)
	ib := RegularizedIncompleteBeta(df/2, 0.5, x)
	if t > 0 {
		return 1 - 0.5*ib
	}
	return 0.5 * ib
}
