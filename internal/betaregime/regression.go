package betaregime

import (
	"fmt"
	"math"
)

// RegressionResult holds OLS coefficients with cluster-robust standard errors
// (CR1 small-sample correction). StdErr, TStat and PValue are aligned with
// Coef: index 0 is the intercept, index 1.. are the predictor columns.
type RegressionResult struct {
	Observations int
	Parameters   int
	Clusters     int
	Coef         []float64
	StdErr       []float64
	TStat        []float64
	PValue       []float64
	R2           float64
	DegreesFree  int
}

// FitClusterOLS estimates y = beta0 + x*beta with one-way cluster-robust
// standard errors. x is [][]float64 with one row per observation (no intercept
// column); clusters maps each observation to a cluster id. The CR1 correction
// is (G/(G-1))*((N-1)/(N-K)); p-values use a Student t with G-1 degrees of
// freedom, the conventional cluster-robust choice when G is small.
func FitClusterOLS(y []float64, x [][]float64, clusters []int) (RegressionResult, error) {
	n := len(y)
	if n == 0 {
		return RegressionResult{}, fmt.Errorf("regression: no observations")
	}
	if len(x) != n || len(clusters) != n {
		return RegressionResult{}, fmt.Errorf("regression: y, x and clusters must have equal length")
	}
	predictors := 0
	if n > 0 {
		predictors = len(x[0])
	}
	for i, row := range x {
		if len(row) != predictors {
			return RegressionResult{}, fmt.Errorf("regression: ragged predictor matrix at row %d", i)
		}
	}

	k := predictors + 1
	if n <= k {
		return RegressionResult{}, fmt.Errorf("regression: need more observations (%d) than parameters (%d)", n, k)
	}

	xm := make([][]float64, n)
	for i := range xm {
		xm[i] = make([]float64, k)
		xm[i][0] = 1
		copy(xm[i][1:], x[i])
	}

	xtx := matMulTranspose(xm, xm)
	xty := matVecMulTranspose(xm, y)
	xtxInv, err := invertMatrix(xtx)
	if err != nil {
		return RegressionResult{}, fmt.Errorf("regression: design matrix singular: %w", err)
	}

	coef := matVecMul(xtxInv, xty)

	resid := make([]float64, n)
	yBar := 0.0
	for _, v := range y {
		yBar += v
	}
	yBar /= float64(n)
	ssTot := 0.0
	ssRes := 0.0
	for i := 0; i < n; i++ {
		fitted := 0.0
		for j := 0; j < k; j++ {
			fitted += xm[i][j] * coef[j]
		}
		resid[i] = y[i] - fitted
		ssRes += resid[i] * resid[i]
		ssTot += (y[i] - yBar) * (y[i] - yBar)
	}
	r2 := 0.0
	if ssTot > 0 {
		r2 = 1 - ssRes/ssTot
	}

	clusterIDs := uniqueInts(clusters)
	g := len(clusterIDs)
	if g < 2 {
		return RegressionResult{}, fmt.Errorf("regression: need at least 2 clusters for cluster-robust standard errors, got %d", g)
	}

	meat := make([][]float64, k)
	for i := range meat {
		meat[i] = make([]float64, k)
	}
	for _, cid := range clusterIDs {
		score := make([]float64, k)
		for i := 0; i < n; i++ {
			if clusters[i] != cid {
				continue
			}
			for j := 0; j < k; j++ {
				score[j] += xm[i][j] * resid[i]
			}
		}
		for a := 0; a < k; a++ {
			for b := 0; b < k; b++ {
				meat[a][b] += score[a] * score[b]
			}
		}
	}

	correction := (float64(g) / float64(g-1)) * (float64(n-1) / float64(n-k))
	vcov := matMul(matMul(xtxInv, meat), xtxInv)
	for i := range vcov {
		for j := range vcov[i] {
			vcov[i][j] *= correction
		}
	}

	res := RegressionResult{
		Observations: n,
		Parameters:   k,
		Clusters:     g,
		Coef:         coef,
		StdErr:       make([]float64, k),
		TStat:        make([]float64, k),
		PValue:       make([]float64, k),
		R2:           r2,
		DegreesFree:  g - 1,
	}
	for j := 0; j < k; j++ {
		v := vcov[j][j]
		if v < 0 {
			v = 0
		}
		se := math.Sqrt(v)
		res.StdErr[j] = se
		if se > 0 {
			res.TStat[j] = coef[j] / se
		}
		res.PValue[j] = 2 * (1 - StudentTCDF(math.Abs(res.TStat[j]), float64(g-1)))
	}
	return res, nil
}

func uniqueInts(values []int) []int {
	seen := make(map[int]struct{}, len(values))
	out := make([]int, 0, len(values))
	for _, v := range values {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func matMulTranspose(a, b [][]float64) [][]float64 {
	rows := len(a[0])
	cols := len(b[0])
	out := make([][]float64, rows)
	for i := range out {
		out[i] = make([]float64, cols)
	}
	for i := 0; i < rows; i++ {
		for j := 0; j < cols; j++ {
			sum := 0.0
			for r := 0; r < len(a); r++ {
				sum += a[r][i] * b[r][j]
			}
			out[i][j] = sum
		}
	}
	return out
}

func matVecMulTranspose(a [][]float64, v []float64) []float64 {
	cols := len(a[0])
	out := make([]float64, cols)
	for j := 0; j < cols; j++ {
		sum := 0.0
		for r := 0; r < len(a); r++ {
			sum += a[r][j] * v[r]
		}
		out[j] = sum
	}
	return out
}

func matVecMul(a [][]float64, v []float64) []float64 {
	out := make([]float64, len(a))
	for i := range a {
		sum := 0.0
		for j := range v {
			sum += a[i][j] * v[j]
		}
		out[i] = sum
	}
	return out
}

func matMul(a, b [][]float64) [][]float64 {
	rows := len(a)
	cols := len(b[0])
	out := make([][]float64, rows)
	for i := range out {
		out[i] = make([]float64, cols)
	}
	for i := 0; i < rows; i++ {
		for j := 0; j < cols; j++ {
			sum := 0.0
			for r := range b {
				sum += a[i][r] * b[r][j]
			}
			out[i][j] = sum
		}
	}
	return out
}

func invertMatrix(m [][]float64) ([][]float64, error) {
	n := len(m)
	a := make([][]float64, n)
	inv := make([][]float64, n)
	for i := range m {
		a[i] = append([]float64(nil), m[i]...)
		inv[i] = make([]float64, n)
		inv[i][i] = 1
	}
	for col := 0; col < n; col++ {
		pivot := col
		for row := col + 1; row < n; row++ {
			if math.Abs(a[row][col]) > math.Abs(a[pivot][col]) {
				pivot = row
			}
		}
		if math.Abs(a[pivot][col]) < 1e-12 {
			return nil, fmt.Errorf("singular matrix at column %d", col)
		}
		a[col], a[pivot] = a[pivot], a[col]
		inv[col], inv[pivot] = inv[pivot], inv[col]
		div := a[col][col]
		for j := 0; j < n; j++ {
			a[col][j] /= div
			inv[col][j] /= div
		}
		for row := 0; row < n; row++ {
			if row == col {
				continue
			}
			factor := a[row][col]
			if factor == 0 {
				continue
			}
			for j := 0; j < n; j++ {
				a[row][j] -= factor * a[col][j]
				inv[row][j] -= factor * inv[col][j]
			}
		}
	}
	return inv, nil
}
