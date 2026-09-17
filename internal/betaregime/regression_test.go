package betaregime

import (
	"math"
	"testing"
)

func TestFitClusterOLSRecoversCoefficients(t *testing.T) {
	y := []float64{3, 5, 7, 9}
	x := [][]float64{{1}, {2}, {3}, {4}}
	clusters := []int{0, 0, 1, 1}
	res, err := FitClusterOLS(y, x, clusters)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(res.Coef[0]-1) > 1e-9 || math.Abs(res.Coef[1]-2) > 1e-9 {
		t.Fatalf("coef = %v, want [1 2]", res.Coef)
	}
	if math.Abs(res.R2-1) > 1e-9 {
		t.Fatalf("R2 = %v, want 1", res.R2)
	}
	if res.Clusters != 2 || res.Observations != 4 || res.Parameters != 2 {
		t.Fatalf("meta = %d obs, %d params, %d clusters", res.Observations, res.Parameters, res.Clusters)
	}
}

// When every observation is its own cluster (G=N), the CR1 cluster-robust
// variance equals HC0 scaled by N/(N-K). This cross-checks the meat and
// small-sample correction against an independently written HC0 formula.
func TestFitClusterOLSSingleObservationClustersMatchHC0(t *testing.T) {
	y := []float64{2.0, 3.1, 4.9, 6.2}
	x := [][]float64{{1.0}, {2.0}, {3.0}, {5.0}}
	clusters := []int{0, 1, 2, 3}

	res, err := FitClusterOLS(y, x, clusters)
	if err != nil {
		t.Fatal(err)
	}

	n := 4.0
	k := 2.0
	meanX := (1 + 2 + 3 + 5) / n
	meanY := (2.0 + 3.1 + 4.9 + 6.2) / n
	var sxx, sxy, syy float64
	resid := make([]float64, len(y))
	for i := range y {
		sxx += (x[i][0] - meanX) * (x[i][0] - meanX)
		sxy += (x[i][0] - meanX) * (y[i] - meanY)
		syy += (y[i] - meanY) * (y[i] - meanY)
	}
	slope := sxy / sxx
	intercept := meanY - slope*meanX
	for i := range y {
		resid[i] = y[i] - (intercept + slope*x[i][0])
	}

	// HC0 slope variance = sum((x_i - meanX)^2 * e_i^2) / sxx^2.
	var hc0Num float64
	for i := range y {
		hc0Num += (x[i][0] - meanX) * (x[i][0] - meanX) * resid[i] * resid[i]
	}
	hc0SlopeVar := hc0Num / (sxx * sxx)
	// CR1 with G=N multiplies HC0 by N/(N-K) = 2.
	wantClusterVar := hc0SlopeVar * (n / (n - k))

	if math.Abs(res.Coef[0]-intercept) > 1e-9 || math.Abs(res.Coef[1]-slope) > 1e-9 {
		t.Fatalf("coef = %v, want [%v %v]", res.Coef, intercept, slope)
	}
	if math.Abs(res.StdErr[1]*res.StdErr[1]-wantClusterVar) > 1e-9 {
		t.Fatalf("cluster var = %v, want %v", res.StdErr[1]*res.StdErr[1], wantClusterVar)
	}
}

func TestFitClusterOLSValidation(t *testing.T) {
	if _, err := FitClusterOLS([]float64{1, 2}, [][]float64{{1}, {2}}, []int{0, 0}); err == nil {
		t.Fatal("expected error when fewer than 2 clusters")
	}
	if _, err := FitClusterOLS([]float64{1}, [][]float64{{1}}, []int{0}); err == nil {
		t.Fatal("expected error when observations <= parameters")
	}
	if _, err := FitClusterOLS([]float64{1, 2, 3}, [][]float64{{1}, {2, 3}, {3}}, []int{0, 1, 2}); err == nil {
		t.Fatal("expected error for ragged predictor matrix")
	}
}
