package betaregime

import (
	"math"
	"testing"
)

func TestRegularizedIncompleteBetaHalfHalf(t *testing.T) {
	x := 0.25
	got := RegularizedIncompleteBeta(0.5, 0.5, x)
	// I_x(1/2,1/2) = (2/pi)*asin(sqrt(x)); sqrt(0.25)=0.5, asin(0.5)=pi/6.
	want := 2 / math.Pi * math.Asin(math.Sqrt(x))
	if math.Abs(got-want) > 1e-10 {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestRegularizedIncompleteBetaBoundariesAndSymmetry(t *testing.T) {
	if got := RegularizedIncompleteBeta(2, 3, 0); got != 0 {
		t.Fatalf("I_0 = %v, want 0", got)
	}
	if got := RegularizedIncompleteBeta(2, 3, 1); got != 1 {
		t.Fatalf("I_1 = %v, want 1", got)
	}
	// Symmetry: I_x(a,b) = 1 - I_{1-x}(b,a).
	x := 0.3
	got := RegularizedIncompleteBeta(2.5, 4.0, x) + RegularizedIncompleteBeta(4.0, 2.5, 1-x)
	if math.Abs(got-1) > 1e-10 {
		t.Fatalf("symmetry sum = %v, want 1", got)
	}
}

func TestStudentTCDKnownQuantile(t *testing.T) {
	// t_{0.95,5} = 2.015048.
	got := StudentTCDF(2.015048, 5)
	if math.Abs(got-0.95) > 1e-4 {
		t.Fatalf("TCDF(2.015048, 5) = %v, want 0.95", got)
	}
}

func TestStudentTCDApproachesNormal(t *testing.T) {
	got := StudentTCDF(3.0, 10000)
	// Phi(3) = 0.5*(1+erf(3/sqrt(2))).
	want := 0.5 * (1 + math.Erf(3/math.Sqrt2))
	if math.Abs(got-want) > 1e-3 {
		t.Fatalf("TCDF(3, 10000) = %v, want ~%v", got, want)
	}
}

func TestStudentTCDSymmetry(t *testing.T) {
	got := StudentTCDF(1.7, 12) + StudentTCDF(-1.7, 12)
	if math.Abs(got-1) > 1e-12 {
		t.Fatalf("symmetry sum = %v, want 1", got)
	}
}
