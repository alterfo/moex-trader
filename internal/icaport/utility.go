package icaport

import "math"

type Cumulants struct {
	K2 []float64
	K3 []float64
	K4 []float64
}

func PortfolioMoments(a []float64, c Cumulants) (sigma2, s3, k4 float64) {
	for l := range a {
		a2 := a[l] * a[l]
		sigma2 += a2 * c.K2[l]
		s3 += a2 * a[l] * c.K3[l]
		k4 += a2 * a2 * c.K4[l]
	}
	k4 += 3 * sigma2 * sigma2
	return sigma2, s3, k4
}

type Objective interface {
	Value(w []float64) float64
	Gradient(w []float64) []float64
}

type CARA struct {
	B   Mat
	Mu  []float64
	C   Cumulants
	Rho float64
}

func (c CARA) loadings(w []float64) []float64 {
	n := len(c.C.K2)
	a := make([]float64, n)
	for l := 0; l < n; l++ {
		for i := range w {
			a[l] += c.B[i][l] * w[i]
		}
	}
	return a
}

func (c CARA) muP(w []float64) float64 {
	if len(c.Mu) == 0 {
		return 0
	}
	return Dot(w, c.Mu)
}

func (c CARA) Value(w []float64) float64 {
	a := c.loadings(w)
	sigma2, s3, k4 := PortfolioMoments(a, c.C)
	g := 1 + c.Rho*c.Rho/2*sigma2 - math.Pow(c.Rho, 3)/6*s3 + math.Pow(c.Rho, 4)/24*k4
	return math.Exp(-c.Rho*(1+c.muP(w))) * g
}

func (c CARA) Gradient(w []float64) []float64 {
	a := c.loadings(w)
	sigma2, s3, k4 := PortfolioMoments(a, c.C)
	rho := c.Rho
	g := 1 + rho*rho/2*sigma2 - math.Pow(rho, 3)/6*s3 + math.Pow(rho, 4)/24*k4
	scale := math.Exp(-rho * (1 + c.muP(w)))
	da := make([]float64, len(a))
	for l := range a {
		dSigma := 2 * a[l] * c.C.K2[l]
		dS := 3 * a[l] * a[l] * c.C.K3[l]
		dK := 4*a[l]*a[l]*a[l]*c.C.K4[l] + 12*sigma2*a[l]*c.C.K2[l]
		da[l] = rho*rho/2*dSigma - math.Pow(rho, 3)/6*dS + math.Pow(rho, 4)/24*dK
	}
	grad := make([]float64, len(w))
	for i := range w {
		var s float64
		for l := range a {
			s += c.B[i][l] * da[l]
		}
		grad[i] = scale * s
		if len(c.Mu) > 0 {
			grad[i] += -rho * c.Mu[i] * scale * g
		}
	}
	return grad
}

type Quadratic struct {
	Sigma Mat
}

func (q Quadratic) Value(w []float64) float64 {
	return Dot(w, MatVec(q.Sigma, w))
}

func (q Quadratic) Gradient(w []float64) []float64 {
	g := MatVec(q.Sigma, w)
	for i := range g {
		g[i] *= 2
	}
	return g
}
