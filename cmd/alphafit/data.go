package main

import (
	"bufio"
	"encoding/csv"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
)

var nan = math.NaN()

var excludeCols = map[string]bool{
	"OpenDay": true, "CloseDay": true, "Open10": true, "Close10": true, "Close1830": true, "High": true, "Low": true,
	"ValDay": true, "AucA1": true, "EvA1": true, "EvA2": true, "EvSpRatio": true, "EvRet": true, "MornA1": true, "MornShare": true,
}

type row struct {
	date, secid              string
	di                       int
	divDrop                  bool
	rb1, rb5, rb20           float64
	f1, f5, f10, u1, u5, u10 float64
	raw                      []float64
	feat                     []float32
	y10, y5                  float64
	sig                      float64
	rank                     float64
}

type dataset struct {
	names  []string
	dates  []string
	rows   []*row
	byDate [][]*row
}

func f64(s string) float64 {
	if s == "" {
		return nan
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nan
	}
	return v
}

func load(path string) (*dataset, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(bufio.NewReaderSize(f, 1<<22))
	hdr, err := r.Read()
	if err != nil {
		return nil, err
	}
	ix := map[string]int{}
	for i, h := range hdr {
		ix[h] = i
	}
	var cols []int
	ds := &dataset{}
	for i := 14; i < len(hdr); i++ {
		if excludeCols[hdr[i]] {
			continue
		}
		cols = append(cols, i)
		ds.names = append(ds.names, hdr[i])
	}
	dset := map[string]bool{}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		x := &row{date: rec[ix["date"]], secid: rec[ix["secid"]], divDrop: rec[ix["div_drop"]] == "1",
			rb1: f64(rec[ix["rb1"]]), rb5: f64(rec[ix["rb5"]]), rb20: f64(rec[ix["rb20"]]),
			f1: f64(rec[ix["f1"]]), f5: f64(rec[ix["f5"]]), f10: f64(rec[ix["f10"]]),
			u1: f64(rec[ix["u1"]]), u5: f64(rec[ix["u5"]]), u10: f64(rec[ix["u10"]]), rank: f64(rec[ix["rank"]])}
		x.raw = make([]float64, len(cols))
		for k, c := range cols {
			x.raw[k] = f64(rec[c])
		}
		ds.rows = append(ds.rows, x)
		dset[x.date] = true
	}
	for d := range dset {
		ds.dates = append(ds.dates, d)
	}
	sort.Strings(ds.dates)
	idx := map[string]int{}
	for i, d := range ds.dates {
		idx[d] = i
	}
	ds.byDate = make([][]*row, len(ds.dates))
	for _, x := range ds.rows {
		x.di = idx[x.date]
		ds.byDate[x.di] = append(ds.byDate[x.di], x)
	}
	for d := range ds.byDate {
		sort.Slice(ds.byDate[d], func(a, b int) bool { return ds.byDate[d][a].secid < ds.byDate[d][b].secid })
	}
	return ds, nil
}

func rankScale(v []float64) []float64 {
	n := len(v)
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return v[idx[a]] < v[idx[b]] })
	out := make([]float64, n)
	for i := 0; i < n; {
		j := i
		for j+1 < n && v[idx[j+1]] == v[idx[i]] {
			j++
		}
		r := float64(i+j)/2 + 0.5
		for k := i; k <= j; k++ {
			out[idx[k]] = r/float64(n) - 0.5
		}
		i = j + 1
	}
	return out
}

func (ds *dataset) derived() {
	for _, name := range []string{"A1", "A2"} {
		k := -1
		for i, n := range ds.names {
			if n == name {
				k = i
			}
		}
		ds.names = append(ds.names, "Unexp"+name)
		hist := map[string][]float64{}
		for d := range ds.byDate {
			for _, x := range ds.byDate[d] {
				v := x.raw[k]
				u := nan
				h := hist[x.secid]
				if !math.IsNaN(v) && len(h) >= 10 {
					lo := len(h) - 20
					if lo < 0 {
						lo = 0
					}
					w := h[lo:]
					m, s := 0.0, 0.0
					for _, a := range w {
						m += a
					}
					m /= float64(len(w))
					for _, a := range w {
						s += (a - m) * (a - m)
					}
					s = math.Sqrt(s / float64(len(w)))
					if s > 0 {
						u = (v - m) / s
					}
				}
				x.raw = append(x.raw, u)
				if !math.IsNaN(v) {
					hist[x.secid] = append(h, v)
				}
			}
		}
	}
	prevRet := map[string][]float64{}
	for d := range ds.byDate {
		for _, x := range ds.byDate[d] {
			h := prevRet[x.secid]
			x.sig = nan
			if len(h) >= 15 {
				lo := len(h) - 20
				if lo < 0 {
					lo = 0
				}
				w := h[lo:]
				m, s := 0.0, 0.0
				for _, a := range w {
					m += a
				}
				m /= float64(len(w))
				for _, a := range w {
					s += (a - m) * (a - m)
				}
				x.sig = math.Sqrt(s / float64(len(w)))
			}
			if !math.IsNaN(x.rb1) {
				prevRet[x.secid] = append(h, x.rb1)
			}
		}
	}
}

func (ds *dataset) build() {
	nf := len(ds.names)
	for d := range ds.byDate {
		rows := ds.byDate[d]
		n := len(rows)
		for _, x := range rows {
			x.feat = make([]float32, nf+3)
		}
		for k := 0; k < nf; k++ {
			var vals []float64
			var who []int
			for i, x := range rows {
				if !math.IsNaN(x.raw[k]) {
					vals = append(vals, x.raw[k])
					who = append(who, i)
				}
			}
			if len(vals) < 20 {
				continue
			}
			rk := rankScale(vals)
			for j, i := range who {
				rows[i].feat[k] = float32(rk[j])
			}
		}
		for c, get := range []func(*row) float64{func(x *row) float64 { return x.rb1 }, func(x *row) float64 { return x.rb5 }, func(x *row) float64 { return x.rb20 }} {
			var vals []float64
			var who []int
			for i, x := range rows {
				if v := get(x); !math.IsNaN(v) {
					vals = append(vals, v)
					who = append(who, i)
				}
			}
			if len(vals) < 20 {
				continue
			}
			rk := rankScale(vals)
			for j, i := range who {
				rows[i].feat[nf+c] = float32(rk[j])
			}
		}
		for _, h := range []int{10, 5} {
			var vals []float64
			var who []int
			for i, x := range rows {
				var f, u float64
				if h == 10 {
					f, u = x.f10, x.u10
				} else {
					f, u = x.f5, x.u5
				}
				if !math.IsNaN(f) && !math.IsNaN(u) && !x.divDrop {
					vals = append(vals, f-u)
					who = append(who, i)
				}
			}
			for _, x := range rows {
				if h == 10 {
					x.y10 = nan
				} else {
					x.y5 = nan
				}
			}
			if len(vals) < 30 {
				continue
			}
			rk := rankScale(vals)
			for j, i := range who {
				if h == 10 {
					rows[i].y10 = rk[j]
				} else {
					rows[i].y5 = rk[j]
				}
			}
		}
		_ = n
	}
}
