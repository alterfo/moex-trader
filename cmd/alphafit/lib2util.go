package main

import (
	"encoding/json"
	"log"
	"math"
	"os"
	"sort"
)

func readFile(p string) ([]byte, error) { return os.ReadFile(p) }

func fatal(err error) { log.Fatal(err) }

func unmarshal(b []byte, v interface{}) { json.Unmarshal(b, v) }

func sortSearch(a []string, s string) int { return sort.SearchStrings(a, s) }

func jacobiEigen(a [][]float64) []float64 {
	n := len(a)
	m := make([][]float64, n)
	for i := range m {
		m[i] = append([]float64(nil), a[i]...)
	}
	for sweep := 0; sweep < 100; sweep++ {
		off := 0.0
		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				off += m[i][j] * m[i][j]
			}
		}
		if off < 1e-18 {
			break
		}
		for p := 0; p < n; p++ {
			for q := p + 1; q < n; q++ {
				if math.Abs(m[p][q]) < 1e-15 {
					continue
				}
				th := (m[q][q] - m[p][p]) / (2 * m[p][q])
				t := math.Copysign(1, th) / (math.Abs(th) + math.Sqrt(th*th+1))
				c := 1 / math.Sqrt(t*t+1)
				s := t * c
				for k := 0; k < n; k++ {
					kp, kq := m[k][p], m[k][q]
					m[k][p] = c*kp - s*kq
					m[k][q] = s*kp + c*kq
				}
				for k := 0; k < n; k++ {
					pk, qk := m[p][k], m[q][k]
					m[p][k] = c*pk - s*qk
					m[q][k] = s*pk + c*qk
				}
			}
		}
	}
	e := make([]float64, n)
	for i := range e {
		e[i] = m[i][i]
	}
	return e
}
