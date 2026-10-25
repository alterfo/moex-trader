package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
)

type row struct {
	id, desc string
	series   []int
	sig      func(*series) []float64
	hor      int
	win      string
	inter    bool
}

var split = "2025-01-03"

func collect(sr []*series, r row, win string, shift int, di map[string]int) []point {
	var pts []point
	for _, k := range r.series {
		s := sr[k]
		sig := r.sig(s)
		var fd []float64
		if r.inter {
			fd = s.fd
		}
		y := s.y5
		switch r.hor {
		case 1:
			y = s.y1
		case 10:
			y = s.y10
		}
		n := s.n
		for i := 0; i < n; i++ {
			d := s.dates[i]
			if win == "rt" && d < split {
				continue
			}
			if win == "pre" && d >= split {
				continue
			}
			j := i
			if shift != 0 {
				j = (i + shift) % n
			}
			x := sig[j]
			if math.IsNaN(x) || math.IsNaN(y[i]) {
				continue
			}
			var xs []float64
			if r.inter {
				f := fd[j]
				if math.IsNaN(f) {
					continue
				}
				xs = []float64{x, f, x * f}
			} else {
				xs = []float64{x}
			}
			pts = append(pts, point{k: k, t: di[d], x: xs, y: y[i]})
		}
	}
	return pts
}

func main() {
	matrix := flag.String("matrix", "work/matrix.csv", "flowscan matrix")
	flag.Parse()
	want := map[string]bool{}
	for _, p := range append(append([]pairSpec{}, realPairs...), nearPairs...) {
		want[p.ord], want[p.pref] = true, true
	}
	p, err := loadPanel(*matrix, want)
	if err != nil {
		log.Fatal(err)
	}
	var sr []*series
	for _, sp := range append(append([]pairSpec{}, realPairs...), nearPairs...) {
		sr = append(sr, build(p, sp))
	}
	nReal := len(realPairs)
	var allReal []int
	for i := 0; i < nReal; i++ {
		allReal = append(allReal, i)
	}
	zsig := func(s *series) []float64 { return s.z }
	msig := func(s *series) []float64 { return s.mz }
	fsig := func(s *series) []float64 { return s.fd }
	var rows []row
	for i := 0; i < nReal; i++ {
		rows = append(rows, row{id: fmt.Sprintf("PR%02d", i+1), desc: sr[i].spec.id + " reversion of the ordinary/preferred spread z, h5", series: []int{i}, sig: zsig, hor: 5, win: "all"})
	}
	for k, h := range []int{1, 5, 10} {
		rows = append(rows, row{id: fmt.Sprintf("PR%02d", nReal+k+1), desc: fmt.Sprintf("pooled reversion z, h%d", h), series: allReal, sig: zsig, hor: h, win: "all"})
	}
	for k, h := range []int{5, 10} {
		rows = append(rows, row{id: fmt.Sprintf("PM%02d", k+1), desc: fmt.Sprintf("pooled spread momentum (20d change), h%d", h), series: allReal, sig: msig, hor: h, win: "all"})
	}
	for i := 0; i < nReal; i++ {
		rows = append(rows, row{id: fmt.Sprintf("PF%02d", i+1), desc: sr[i].spec.id + " A2 difference, h1 (realtime)", series: []int{i}, sig: fsig, hor: 1, win: "rt"})
	}
	rows = append(rows, row{id: "PF07", desc: "pooled A2 difference, h1 (realtime)", series: allReal, sig: fsig, hor: 1, win: "rt"})
	rows = append(rows, row{id: "PF08", desc: "pooled A2 difference, h5 (realtime)", series: allReal, sig: fsig, hor: 5, win: "rt"})
	rows = append(rows, row{id: "PX01", desc: "pooled z x A2 difference interaction, h5 (realtime)", series: allReal, sig: zsig, hor: 5, win: "rt", inter: true})
	var near []int
	for i := nReal; i < len(sr); i++ {
		near = append(near, i)
	}
	nearRow := row{id: "PN01", desc: "near-pair (same sector, different companies) pooled reversion z, h5 (descriptive)", series: near, sig: zsig, hor: 5, win: "all"}

	report(sr, rows, nearRow)
}

func report(sr []*series, rows []row, nearRow row) {
	dateSet := map[string]bool{}
	for _, s := range sr {
		for _, d := range s.dates {
			dateSet[d] = true
		}
	}
	var dates []string
	for d := range dateSet {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	di := map[string]int{}
	for t, d := range dates {
		di[d] = t
	}
	nT := len(dates)
	fmt.Printf("%-5s %-62s %6s %9s %7s %8s %9s %9s\n", "id", "row", "n", "coef", "t", "perm_p", "t_pre", "t_rt")
	var results []result
	run := func(r row) {
		pts := collect(sr, r, r.win, 0, di)
		f := regress(pts, nT, r.hor+5)
		if f.coef == nil {
			fmt.Printf("%-5s %-62s not testable\n", r.id, r.desc)
			return
		}
		idx := len(f.coef) - 1
		ge, cnt := 0, 0
		for off := 120; off < 1400; off += 7 {
			var shifted []point
			for _, k := range r.series {
				if off >= sr[k].n-120 {
					continue
				}
				one := r
				one.series = []int{k}
				shifted = append(shifted, collect(sr, one, r.win, off, di)...)
			}
			fs := regress(shifted, nT, r.hor+5)
			if fs.coef == nil {
				continue
			}
			cnt++
			if math.Abs(fs.t[idx]) >= math.Abs(f.t[idx]) {
				ge++
			}
		}
		var tPre, tRt = nan, nan
		if r.win == "all" {
			a := regress(collect(sr, r, "pre", 0, di), nT, r.hor+5)
			b := regress(collect(sr, r, "rt", 0, di), nT, r.hor+5)
			if a.coef != nil {
				tPre = a.t[idx]
			}
			if b.coef != nil {
				tRt = b.t[idx]
			}
		}
		fmt.Printf("%-5s %-62s %6d %+9.4f %+7.2f %8.4f %+9.2f %+9.2f\n", r.id, r.desc, f.n, f.coef[idx], f.t[idx], float64(1+ge)/float64(1+cnt), tPre, tRt)
		sigma := map[int]float64{}
		for _, k := range r.series {
			sg := r.sig(sr[k])
			for i, d := range sr[k].dates {
				if r.win == "rt" && d < split {
					continue
				}
				if !math.IsNaN(sg[i]) {
					sigma[k*100000+di[d]] = sg[i]
				}
			}
		}
		sign := -1.0
		if strings.HasPrefix(r.id, "PM") || strings.HasPrefix(r.id, "PF") {
			sign = 1
		}
		results = append(results, result{id: r.id, coef: f.coef[idx], t: f.t[idx], p: float64(1+ge) / float64(1+cnt), tPre: tPre, tRt: tRt, ok: true, signal: sigma, sign: sign, price: r.win == "all", twoSide: r.inter})
	}
	for _, r := range rows {
		run(r)
	}
	run(nearRow)
	verdict(results)
	economics(sr, "all")
	economics(sr, "rt")
}
