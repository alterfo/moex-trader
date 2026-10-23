package main

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
)

type r2Date struct {
	d    int
	ctrl [][]float64
	feat [][]float64
	y    []float64
}

func (r *run) collectR2(tg fcTarget, feats []fcFeature) []r2Date {
	p := r.p
	var out []r2Date
	for d := r.win.lo - 60; d <= r.win.hi && d+tg.hor < len(p.dates); d++ {
		if d < 61 || p.excluded[d] {
			continue
		}
		adm := p.admittedAt(d)
		fv := make([][]float64, len(feats))
		for f := range feats {
			fv[f] = feats[f].fn(p, d, adm)
		}
		var cRows, fRows [][]float64
		var ys []float64
		for k, i := range adm {
			bad := false
			for s := d; s <= d+tg.hor; s++ {
				if p.excluded[s] || p.divDrop[i][s] {
					bad = true
				}
			}
			if bad {
				continue
			}
			y := tg.value(p, i, d)
			if math.IsNaN(y) {
				continue
			}
			c := make([]float64, 0, len(tg.ctrls))
			for _, cf := range tg.ctrls {
				v := cf(p, i, d)
				if math.IsNaN(v) || math.IsInf(v, 0) {
					c = nil
					break
				}
				c = append(c, v)
			}
			if c == nil {
				continue
			}
			f := make([]float64, len(feats))
			for j := range feats {
				v := fv[j][k]
				if math.IsNaN(v) || math.IsInf(v, 0) {
					f = nil
					break
				}
				f[j] = v
			}
			if f == nil {
				continue
			}
			cRows = append(cRows, c)
			fRows = append(fRows, f)
			ys = append(ys, y)
		}
		if len(ys) < 40 {
			continue
		}
		rd := r2Date{d: d, y: rankScale(ys)}
		rd.ctrl = rankCols(cRows)
		rd.feat = rankCols(fRows)
		out = append(out, rd)
	}
	return out
}

func rankCols(rows [][]float64) [][]float64 {
	nc := len(rows[0])
	out := make([][]float64, len(rows))
	for i := range out {
		out[i] = make([]float64, nc)
	}
	for c := 0; c < nc; c++ {
		v := make([]float64, len(rows))
		for i := range rows {
			v[i] = rows[i][c]
		}
		rk := rankScale(v)
		for i := range rows {
			out[i][c] = rk[i]
		}
	}
	return out
}

func ridgeFit(X [][]float64, y []float64, lambda float64) []float64 {
	k := len(X[0])
	aug := make([][]float64, 0, len(X)+k)
	ya := make([]float64, 0, len(X)+k)
	aug = append(aug, X...)
	ya = append(ya, y...)
	s := math.Sqrt(lambda)
	for j := 1; j < k; j++ {
		row := make([]float64, k)
		row[j] = s
		aug = append(aug, row)
		ya = append(ya, 0)
	}
	b, ok := ols(aug, ya)
	if !ok {
		return nil
	}
	return b
}

func design(rd r2Date, withFeat bool) [][]float64 {
	X := make([][]float64, len(rd.y))
	for i := range X {
		row := []float64{1}
		row = append(row, rd.ctrl[i]...)
		if withFeat {
			row = append(row, rd.feat[i]...)
		}
		X[i] = row
	}
	return X
}

func (r *run) addedValue(tg fcTarget, feats []fcFeature, trainFromD int) {
	w := r.w
	data := r.collectR2(tg, feats)
	edges := foldEdgesFlow()
	type acc struct{ sseW, sseO, sst []float64 }
	var perDate []struct{ sseW, sseO, sst float64 }
	var dateIdx []int
	var foldOf []int
	for k, e := range edges {
		lo := r.p.dateIndex(e[0])
		hi := r.p.dateIndex(e[1])
		if hi >= len(r.p.dates) || r.p.dates[hi] > e[1] {
			hi--
		}
		var trX0, trX1 [][]float64
		var trY []float64
		for _, rd := range data {
			if rd.d < trainFromD || rd.d+tg.hor >= lo-tg.hor {
				continue
			}
			trX0 = append(trX0, design(rd, false)...)
			trX1 = append(trX1, design(rd, true)...)
			trY = append(trY, rd.y...)
		}
		if len(trY) < 500 {
			continue
		}
		lam := 0.002 * float64(len(trY))
		b0, b1 := ridgeFit(trX0, trY, lam), ridgeFit(trX1, trY, lam)
		if b0 == nil || b1 == nil {
			continue
		}
		for _, rd := range data {
			if rd.d < lo || rd.d > hi {
				continue
			}
			x0, x1 := design(rd, false), design(rd, true)
			var sw, so, st float64
			for i := range rd.y {
				p0, p1 := 0.0, 0.0
				for j, v := range x0[i] {
					p0 += b0[j] * v
				}
				for j, v := range x1[i] {
					p1 += b1[j] * v
				}
				sw += (rd.y[i] - p1) * (rd.y[i] - p1)
				so += (rd.y[i] - p0) * (rd.y[i] - p0)
				st += rd.y[i] * rd.y[i]
			}
			perDate = append(perDate, struct{ sseW, sseO, sst float64 }{sw, so, st})
			dateIdx = append(dateIdx, rd.d)
			foldOf = append(foldOf, k)
		}
	}
	if len(perDate) == 0 {
		fmt.Fprintf(w, "added value %s: no folds\n", tg.name)
		return
	}
	calc := func(wt []float64, sel func(int) bool) (float64, float64, float64) {
		var sw, so, st float64
		for i, p := range perDate {
			if wt[i] == 0 || !sel(i) {
				continue
			}
			sw += wt[i] * p.sseW
			so += wt[i] * p.sseO
			st += wt[i] * p.sst
		}
		return 1 - sw/st, 1 - so/st, st
	}
	one := make([]float64, len(perDate))
	for i := range one {
		one[i] = 1
	}
	all := func(int) bool { return true }
	rw, ro, _ := calc(one, all)
	h0 := func(i int) bool { return dateIdx[i] < r.win.split }
	h1 := func(i int) bool { return dateIdx[i] >= r.win.split }
	w0, o0, _ := calc(one, h0)
	w1, o1, _ := calc(one, h1)
	rng := rand.New(rand.NewSource(20261022))
	n := len(perDate)
	var diffs []float64
	for b := 0; b < 2000; b++ {
		wt := make([]float64, n)
		need := (n + 9) / 10
		for q := 0; q < need; q++ {
			s := rng.Intn(n - 9)
			for k := 0; k < 10; k++ {
				wt[s+k]++
			}
		}
		x, y, _ := calc(wt, all)
		diffs = append(diffs, x-y)
	}
	sort.Float64s(diffs)
	fmt.Fprintf(w, "added value %s: out-of-fold R2 controls only %.4f, controls+block %.4f, gain %+.4f (95%% [%+.4f, %+.4f]); halves gain %+.4f / %+.4f; %d dates\n", tg.name, ro, rw, rw-ro, diffs[50], diffs[1949], w0-o0, w1-o1, n)
}

func foldEdgesFlow() [][2]string {
	return [][2]string{{"2025-01-03", "2025-03-31"}, {"2025-04-01", "2025-06-30"}, {"2025-07-01", "2025-09-30"}, {"2025-10-01", "2025-12-31"}, {"2026-01-01", "2026-03-31"}, {"2026-04-01", "2026-06-30"}, {"2026-07-01", "2026-10-16"}}
}
