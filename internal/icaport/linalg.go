package icaport

import (
	"errors"
	"math"
)

type Mat [][]float64

func NewMat(rows, cols int) Mat {
	m := make(Mat, rows)
	for i := range m {
		m[i] = make([]float64, cols)
	}
	return m
}

func Identity(n int) Mat {
	m := NewMat(n, n)
	for i := 0; i < n; i++ {
		m[i][i] = 1
	}
	return m
}

func (m Mat) Transpose() Mat {
	if len(m) == 0 {
		return Mat{}
	}
	t := NewMat(len(m[0]), len(m))
	for i := range m {
		for j := range m[i] {
			t[j][i] = m[i][j]
		}
	}
	return t
}

func MatMul(a, b Mat) Mat {
	out := NewMat(len(a), len(b[0]))
	for i := range a {
		for k := range b {
			aik := a[i][k]
			if aik == 0 {
				continue
			}
			for j := range b[k] {
				out[i][j] += aik * b[k][j]
			}
		}
	}
	return out
}

func MatVec(a Mat, v []float64) []float64 {
	out := make([]float64, len(a))
	for i := range a {
		var s float64
		for j := range v {
			s += a[i][j] * v[j]
		}
		out[i] = s
	}
	return out
}

func Dot(a, b []float64) float64 {
	var s float64
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}

var ErrSingular = errors.New("icaport: singular matrix")

func Inverse(a Mat) (Mat, error) {
	n := len(a)
	aug := NewMat(n, 2*n)
	for i := 0; i < n; i++ {
		copy(aug[i], a[i])
		aug[i][n+i] = 1
	}
	for col := 0; col < n; col++ {
		pivot := col
		for r := col + 1; r < n; r++ {
			if math.Abs(aug[r][col]) > math.Abs(aug[pivot][col]) {
				pivot = r
			}
		}
		if math.Abs(aug[pivot][col]) < 1e-14 {
			return nil, ErrSingular
		}
		aug[col], aug[pivot] = aug[pivot], aug[col]
		div := aug[col][col]
		for j := range aug[col] {
			aug[col][j] /= div
		}
		for r := 0; r < n; r++ {
			if r == col {
				continue
			}
			f := aug[r][col]
			if f == 0 {
				continue
			}
			for j := range aug[r] {
				aug[r][j] -= f * aug[col][j]
			}
		}
	}
	out := NewMat(n, n)
	for i := 0; i < n; i++ {
		copy(out[i], aug[i][n:])
	}
	return out, nil
}

func JacobiEigen(a Mat) ([]float64, Mat) {
	n := len(a)
	m := NewMat(n, n)
	for i := range a {
		copy(m[i], a[i])
	}
	v := Identity(n)
	for sweep := 0; sweep < 100; sweep++ {
		var off float64
		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				off += m[i][j] * m[i][j]
			}
		}
		if off < 1e-22 {
			break
		}
		for p := 0; p < n; p++ {
			for q := p + 1; q < n; q++ {
				if math.Abs(m[p][q]) < 1e-300 {
					continue
				}
				theta := (m[q][q] - m[p][p]) / (2 * m[p][q])
				t := 1 / (math.Abs(theta) + math.Sqrt(theta*theta+1))
				if theta < 0 {
					t = -t
				}
				c := 1 / math.Sqrt(t*t+1)
				s := t * c
				for k := 0; k < n; k++ {
					mkp, mkq := m[k][p], m[k][q]
					m[k][p] = c*mkp - s*mkq
					m[k][q] = s*mkp + c*mkq
				}
				for k := 0; k < n; k++ {
					mpk, mqk := m[p][k], m[q][k]
					m[p][k] = c*mpk - s*mqk
					m[q][k] = s*mpk + c*mqk
				}
				for k := 0; k < n; k++ {
					vkp, vkq := v[k][p], v[k][q]
					v[k][p] = c*vkp - s*vkq
					v[k][q] = s*vkp + c*vkq
				}
			}
		}
	}
	vals := make([]float64, n)
	for i := range vals {
		vals[i] = m[i][i]
	}
	return vals, v
}

func ColumnMeans(x Mat) []float64 {
	n := len(x[0])
	means := make([]float64, n)
	for _, row := range x {
		for j, v := range row {
			means[j] += v
		}
	}
	for j := range means {
		means[j] /= float64(len(x))
	}
	return means
}

func Demean(x Mat) (Mat, []float64) {
	means := ColumnMeans(x)
	out := NewMat(len(x), len(x[0]))
	for i, row := range x {
		for j, v := range row {
			out[i][j] = v - means[j]
		}
	}
	return out, means
}

func Covariance(x Mat) Mat {
	centered, _ := Demean(x)
	n := len(x[0])
	c := NewMat(n, n)
	for _, row := range centered {
		for i := 0; i < n; i++ {
			for j := i; j < n; j++ {
				c[i][j] += row[i] * row[j]
			}
		}
	}
	denom := float64(len(x) - 1)
	for i := 0; i < n; i++ {
		for j := i; j < n; j++ {
			c[i][j] /= denom
			c[j][i] = c[i][j]
		}
	}
	return c
}
