package icaport

type VAR1 struct {
	Phi Mat
}

func FitVAR1(y Mat) (VAR1, Mat, error) {
	t := len(y)
	n := len(y[0])
	x := y[:t-1]
	next := y[1:]
	a := NewMat(n, n)
	b := NewMat(n, n)
	for r := range x {
		for i := 0; i < n; i++ {
			for j := 0; j < n; j++ {
				a[i][j] += x[r][i] * x[r][j]
				b[i][j] += next[r][i] * x[r][j]
			}
		}
	}
	for i := 0; i < n; i++ {
		a[i][i] += 1e-8 * float64(t)
	}
	inv, err := Inverse(a)
	if err != nil {
		return VAR1{}, nil, err
	}
	phi := MatMul(b, inv)
	resid := NewMat(t-1, n)
	for r := range x {
		fit := MatVec(phi, x[r])
		for i := 0; i < n; i++ {
			resid[r][i] = next[r][i] - fit[i]
		}
	}
	return VAR1{Phi: phi}, resid, nil
}

func (v VAR1) Forecast(last []float64) []float64 {
	return MatVec(v.Phi, last)
}
