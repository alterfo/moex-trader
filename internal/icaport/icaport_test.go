package icaport

import (
	"math"
	"math/rand"
	"testing"
)

func TestJacobiEigenReconstructsMatrix(t *testing.T) {
	a := Mat{{4, 1, 0.5}, {1, 3, 0.2}, {0.5, 0.2, 2}}
	vals, vecs := JacobiEigen(a)
	for i := range a {
		for j := range a {
			var s float64
			for k := range vals {
				s += vecs[i][k] * vals[k] * vecs[j][k]
			}
			if math.Abs(s-a[i][j]) > 1e-9 {
				t.Fatalf("reconstruction mismatch at %d,%d: %.12f vs %.12f", i, j, s, a[i][j])
			}
		}
	}
}

func TestInverseTimesMatrixIsIdentity(t *testing.T) {
	a := Mat{{2, 1, 0}, {1, 3, 1}, {0, 1, 4}}
	inv, err := Inverse(a)
	if err != nil {
		t.Fatal(err)
	}
	prod := MatMul(a, inv)
	for i := range prod {
		for j := range prod {
			want := 0.0
			if i == j {
				want = 1
			}
			if math.Abs(prod[i][j]-want) > 1e-12 {
				t.Fatalf("not identity at %d,%d: %.12f", i, j, prod[i][j])
			}
		}
	}
	if _, err := Inverse(Mat{{1, 2}, {2, 4}}); err == nil {
		t.Fatal("singular matrix must fail")
	}
}

func TestFastICARecoversIndependentSources(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	const n = 6000
	sources := NewMat(n, 3)
	for i := 0; i < n; i++ {
		sources[i][0] = rng.Float64()*2*math.Sqrt(3) - math.Sqrt(3)
		u := rng.Float64() - 0.5
		sources[i][1] = -math.Copysign(1, u) * math.Log(1-2*math.Abs(u)) / math.Sqrt2
		e := -math.Log(1 - rng.Float64())
		sources[i][2] = e - 1
	}
	mix := Mat{{1, 0.6, 0.3}, {0.4, 1, 0.5}, {0.2, 0.7, 1}}
	x := NewMat(n, 3)
	for i := range x {
		copy(x[i], MatVec(mix, sources[i]))
	}
	res, err := FastICA(x, 1)
	if err != nil {
		t.Fatal(err)
	}
	comps := Components(x, res.U)
	for s := 0; s < 3; s++ {
		best := 0.0
		for c := 0; c < 3; c++ {
			var sx, sy, sxy, sxx, syy float64
			for i := 0; i < n; i++ {
				a, b := sources[i][s], comps[i][c]
				sx += a
				sy += b
				sxy += a * b
				sxx += a * a
				syy += b * b
			}
			fn := float64(n)
			corr := (sxy/fn - sx/fn*sy/fn) / math.Sqrt((sxx/fn-sx*sx/fn/fn)*(syy/fn-sy*sy/fn/fn))
			best = math.Max(best, math.Abs(corr))
		}
		if best < 0.95 {
			t.Fatalf("source %d recovered with |corr| %.3f", s, best)
		}
	}
	prod := MatMul(res.U, res.B)
	for i := range prod {
		if math.Abs(prod[i][i]-1) > 1e-9 {
			t.Fatalf("U*B is not identity: %v", prod)
		}
	}
}

func TestRecursionRecoversPersistence(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	const n = 4000
	h := 1.0
	x := make([]float64, n)
	for i := 0; i < n; i++ {
		e := math.Sqrt(h) * rng.NormFloat64()
		x[i] = e * e
		h = 0.1 + 0.15*x[i] + 0.75*h
	}
	r := FitRecursion(x, 2)
	if math.Abs(r.Persistence()-0.9) > 0.15 {
		t.Fatalf("persistence %.3f, want about 0.9", r.Persistence())
	}
	if f := r.ForecastAverage(21); f <= 0 || math.IsNaN(f) {
		t.Fatalf("forecast %.4f is not positive", f)
	}
}

func TestPortfolioMomentsMatchSampleMoments(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	const n = 600000
	a := []float64{0.7, -0.4, 0.5}
	cum := Cumulants{K2: []float64{1, 1, 1}, K3: make([]float64, 3), K4: make([]float64, 3)}
	draws := make([]float64, n)
	for i := range draws {
		e := -math.Log(1-rng.Float64()) - 1
		u := rng.Float64() - 0.5
		l := -math.Copysign(1, u) * math.Log(1-2*math.Abs(u)) / math.Sqrt2
		g := rng.NormFloat64()
		draws[i] = a[0]*e + a[1]*l + a[2]*g
	}
	cum.K3[0], cum.K4[0] = 2, 6
	cum.K3[1], cum.K4[1] = 0, 3
	var m2, m3, m4 float64
	for _, v := range draws {
		m2 += v * v
		m3 += v * v * v
		m4 += v * v * v * v
	}
	m2 /= n
	m3 /= n
	m4 /= n
	s2, s3, k4 := PortfolioMoments(a, cum)
	if math.Abs(s2-m2)/m2 > 0.02 || math.Abs(s3-m3)/math.Abs(m3) > 0.05 || math.Abs(k4-m4)/m4 > 0.05 {
		t.Fatalf("formula (%.4f, %.4f, %.4f) vs sample (%.4f, %.4f, %.4f)", s2, s3, k4, m2, m3, m4)
	}
}

func TestCARAGradientMatchesFiniteDifference(t *testing.T) {
	b := Mat{{1, 0.3, -0.2}, {0.1, 1, 0.4}, {-0.3, 0.2, 1}}
	obj := CARA{
		B:   b,
		Mu:  []float64{0.02, -0.01, 0.03},
		C:   Cumulants{K2: []float64{1.2, 0.8, 1.5}, K3: []float64{0.3, -0.2, 0.1}, K4: []float64{2, 1, 3}},
		Rho: 1,
	}
	w := []float64{0.5, 0.3, 0.2}
	g := obj.Gradient(w)
	for i := range w {
		const h = 1e-6
		up := append([]float64(nil), w...)
		dn := append([]float64(nil), w...)
		up[i] += h
		dn[i] -= h
		fd := (obj.Value(up) - obj.Value(dn)) / (2 * h)
		if math.Abs(fd-g[i]) > 1e-6 {
			t.Fatalf("gradient %d: analytic %.8f vs finite %.8f", i, g[i], fd)
		}
	}
}

func TestMinimizeSimplexFeasibleAndOptimal(t *testing.T) {
	sigma := Mat{{4, 0, 0}, {0, 1, 0}, {0, 0, 2}}
	w := MinimizeSimplex(Quadratic{Sigma: sigma}, DefaultStarts(3, 42, 3), 3000)
	var sum float64
	for _, v := range w {
		if v < -1e-12 {
			t.Fatalf("negative weight %v", w)
		}
		sum += v
	}
	if math.Abs(sum-1) > 1e-9 {
		t.Fatalf("weights sum to %.12f", sum)
	}
	want := []float64{1.0 / 4 / (1.0/4 + 1 + 1.0/2), 1 / (1.0/4 + 1 + 1.0/2), 1.0 / 2 / (1.0/4 + 1 + 1.0/2)}
	for i := range w {
		if math.Abs(w[i]-want[i]) > 1e-4 {
			t.Fatalf("weights %v, want %v", w, want)
		}
	}
}

func TestEvaluateKnownSeries(t *testing.T) {
	daily := []float64{0.1, -0.2, 0.05, -0.1, 0.0}
	p := Evaluate(daily)
	wantDD := 1 - (1.1*0.8*1.05*0.9)/(1.1)
	if math.Abs(p.MaxDD-wantDD) > 1e-12 {
		t.Fatalf("max drawdown %.6f, want %.6f", p.MaxDD, wantDD)
	}
	if got := CVaR(daily, 0.8); math.Abs(got-0.2) > 1e-12 {
		t.Fatalf("CVaR %.6f, want 0.2", got)
	}
}

func TestICAWeightsAreOnSimplexAndDeterministic(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	const n, assets = 400, 4
	returns := NewMat(n, assets)
	vols := []float64{1, 1.5, 2, 3}
	for i := range returns {
		common := rng.NormFloat64()
		for j := 0; j < assets; j++ {
			e := -math.Log(1-rng.Float64()) - 1
			returns[i][j] = vols[j] * (0.4*common + 0.6*rng.NormFloat64() + 0.3*e)
		}
	}
	cfg := ICAConfig{Horizon: 21, Rho: 1, Higher: true, UseMean: true, Seed: 42}
	w1, err := ICAWeights(returns, cfg)
	if err != nil {
		t.Fatal(err)
	}
	w2, _ := ICAWeights(returns, cfg)
	var sum float64
	for i := range w1 {
		if w1[i] < -1e-12 || math.Abs(w1[i]-w2[i]) > 1e-12 {
			t.Fatalf("not deterministic or negative: %v vs %v", w1, w2)
		}
		sum += w1[i]
	}
	if math.Abs(sum-1) > 1e-9 {
		t.Fatalf("weights sum %.12f", sum)
	}
	if w1[0] <= w1[3] {
		t.Fatalf("the lowest-volatility asset should outweigh the highest: %v", w1)
	}
}
