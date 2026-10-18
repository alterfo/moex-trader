package icaport

import (
	"errors"
	"math"
	"math/rand"
)

type ICAResult struct {
	U Mat
	B Mat
}

func FastICA(x Mat, seed int64) (ICAResult, error) {
	t := len(x)
	if t < 10 {
		return ICAResult{}, errors.New("icaport: too few observations for ICA")
	}
	n := len(x[0])
	centered, _ := Demean(x)
	cov := NewMat(n, n)
	for _, row := range centered {
		for i := 0; i < n; i++ {
			for j := i; j < n; j++ {
				cov[i][j] += row[i] * row[j]
			}
		}
	}
	for i := 0; i < n; i++ {
		for j := i; j < n; j++ {
			cov[i][j] /= float64(t)
			cov[j][i] = cov[i][j]
		}
	}
	vals, vecs := JacobiEigen(cov)
	whiten := NewMat(n, n)
	for k := 0; k < n; k++ {
		if vals[k] <= 1e-12 {
			return ICAResult{}, errors.New("icaport: covariance is rank deficient")
		}
		scale := 1 / math.Sqrt(vals[k])
		for j := 0; j < n; j++ {
			whiten[k][j] = vecs[j][k] * scale
		}
	}
	z := NewMat(t, n)
	for i, row := range centered {
		copy(z[i], MatVec(whiten, row))
	}

	rng := rand.New(rand.NewSource(seed))
	rotation := NewMat(n, n)
	proj := make([]float64, t)
	for p := 0; p < n; p++ {
		w := make([]float64, n)
		for j := range w {
			w[j] = rng.NormFloat64()
		}
		normalize(w)
		for iter := 0; iter < 300; iter++ {
			var meanGPrime float64
			wNew := make([]float64, n)
			for i := 0; i < t; i++ {
				proj[i] = Dot(w, z[i])
				g := math.Tanh(proj[i])
				meanGPrime += 1 - g*g
				for j := 0; j < n; j++ {
					wNew[j] += z[i][j] * g
				}
			}
			meanGPrime /= float64(t)
			for j := range wNew {
				wNew[j] = wNew[j]/float64(t) - meanGPrime*w[j]
			}
			for q := 0; q < p; q++ {
				d := Dot(wNew, rotation[q])
				for j := range wNew {
					wNew[j] -= d * rotation[q][j]
				}
			}
			if !normalize(wNew) {
				return ICAResult{}, errors.New("icaport: ICA iteration collapsed")
			}
			converged := math.Abs(Dot(wNew, w)) > 1-1e-9
			w = wNew
			if converged {
				break
			}
		}
		copy(rotation[p], w)
	}
	u := MatMul(rotation, whiten)
	b, err := Inverse(u)
	if err != nil {
		return ICAResult{}, err
	}
	return ICAResult{U: u, B: b}, nil
}

func normalize(v []float64) bool {
	norm := math.Sqrt(Dot(v, v))
	if norm < 1e-14 || math.IsNaN(norm) {
		return false
	}
	for i := range v {
		v[i] /= norm
	}
	return true
}

func Components(x Mat, u Mat) Mat {
	out := NewMat(len(x), len(u))
	for i, row := range x {
		copy(out[i], MatVec(u, row))
	}
	return out
}
