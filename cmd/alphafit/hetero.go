package main

import (
	"fmt"
	"math"
	"math/rand"
	"path/filepath"
	"sort"
)

type heteroTest struct {
	id   string
	kind string
	sig  string
	sigB string
	mod  string
	reg  string
	hh   int
}

type heteroStudy struct {
	name  string
	from  string
	till  string
	split string
	tests []heteroTest
}

func heteroStudies() map[string]heteroStudy {
	mom := heteroStudy{name: "momentum", from: "2021-04-08", till: "2024-12-17"}
	for _, m := range []struct{ id, col string }{{"63a liquidity (D02)", "D02"}, {"63b volatility (C04)", "C04"}, {"63c beta (C01)", "C01"}} {
		mom.tests = append(mom.tests, heteroTest{id: m.id, kind: "tercile", sig: "M256", mod: m.col, hh: 10})
	}
	mom.tests = append(mom.tests,
		heteroTest{id: "64a IMOEX below its 200-day mean", kind: "regime", sig: "M256", reg: "imoex200", hh: 10},
		heteroTest{id: "64b RVI above its 60-day median", kind: "regime", sig: "M256", reg: "rvi60", hh: 10},
		heteroTest{id: "68 beta-neutral minus raw", kind: "orth", sig: "M256", mod: "C01", hh: 10})
	news := heteroStudy{name: "attention", from: "2022-01-03", till: "2024-12-17"}
	news.tests = append(news.tests,
		heteroTest{id: "21 size: positive shocks, top minus bottom tercile (next-day abnormal return)", kind: "posbucket", sig: "zc", hh: 1},
		heteroTest{id: "22 source: high-trust minus low-trust", kind: "paired", sig: "zc_hi", sigB: "zc_lo", hh: 1},
		heteroTest{id: "23 liquidity (D02)", kind: "tercile", sig: "zc", mod: "D02", hh: 1},
		heteroTest{id: "24a IMOEX below its 200-day mean", kind: "regime", sig: "zc", reg: "imoex200", hh: 1},
		heteroTest{id: "24b RVI above its 60-day median", kind: "regime", sig: "zc", reg: "rvi60", hh: 1},
		heteroTest{id: "26 company minus aggregated", kind: "paired", sig: "zc_co", sigB: "zc_ag", hh: 1},
		heteroTest{id: "30a dividend headlines minus overall", kind: "paired", sig: "zc_div", sigB: "zc", hh: 1},
		heteroTest{id: "30b sanctions and restrictions minus overall", kind: "paired", sig: "zc_san", sigB: "zc", hh: 1},
		heteroTest{id: "30c results and profit minus overall", kind: "paired", sig: "zc_res", sigB: "zc", hh: 1})
	rep := heteroStudy{name: "low-beta momentum replication", from: "2025-01-03", till: "2026-10-16", split: "2026-01-05"}
	rep.tests = append(rep.tests, heteroTest{id: "R1 beta tercile (C01), lowest minus highest", kind: "tercile", sig: "M256", mod: "C01", hh: 10})
	return map[string]heteroStudy{"momentum": mom, "attention": news, "lbrep": rep}
}

type heteroData struct {
	sigCV  map[string][]float64
	sigCol map[string]int
	modCV  map[string][]float64
	modCol map[string]int
}

func heteroLoad(sigPath, modPath string) *heteroData {
	hd := &heteroData{sigCol: map[string]int{}, modCol: map[string]int{}}
	names, cv := readCandidates(sigPath)
	hd.sigCV = cv
	for i, n := range names {
		hd.sigCol[n] = i
	}
	if modPath == sigPath {
		hd.modCV, hd.modCol = cv, hd.sigCol
		return hd
	}
	mnames, mcv := readCandidates(modPath)
	hd.modCV = mcv
	for i, n := range mnames {
		hd.modCol[n] = i
	}
	return hd
}

func (h *heteroData) sig(date, secid, col string) float64 {
	v := h.sigCV[date+"|"+secid]
	i, ok := h.sigCol[col]
	if v == nil || !ok {
		return nan
	}
	return v[i]
}

func (h *heteroData) mod(date, secid, col string) float64 {
	v := h.modCV[date+"|"+secid]
	i, ok := h.modCol[col]
	if v == nil || !ok {
		return nan
	}
	return v[i]
}

func outcomeOf(x *row, hh int) float64 {
	if x.divDrop {
		return nan
	}
	switch hh {
	case 1:
		return x.f1 - x.u1
	case 5:
		return x.f5 - x.u5
	}
	return x.f10 - x.u10
}

func regimeFlags(ds *dataset, macroDir, name string) []bool {
	out := make([]bool, len(ds.dates))
	switch name {
	case "imoex200":
		d, v := readSeriesCSV(filepath.Join(macroDir, "imoex.csv"))
		for t := range ds.dates {
			j := lastBefore(d, ds.dates[t])
			if j >= 200 {
				m := 0.0
				for _, x := range v[j-199 : j+1] {
					m += x
				}
				out[t] = v[j] < m/200
			}
		}
	case "rvi60":
		d, v := readSeriesCSV(filepath.Join(macroDir, "rvi.csv"))
		for t := range ds.dates {
			j := lastBefore(d, ds.dates[t])
			if j >= 60 {
				w := append([]float64{}, v[j-59:j+1]...)
				sort.Float64s(w)
				out[t] = v[j] > w[30]
			}
		}
	}
	return out
}

func blockBootLevel(x []float64, block, draws int, seed int64, alpha float64) (float64, float64) {
	n := len(x)
	if n < 2*block {
		return nan, nan
	}
	rng := rand.New(rand.NewSource(seed))
	ms := make([]float64, 0, draws)
	for b := 0; b < draws; b++ {
		s, c := 0.0, 0
		for q := 0; q < (n+block-1)/block; q++ {
			st := rng.Intn(n - block + 1)
			for k := 0; k < block; k++ {
				s += x[st+k]
				c++
			}
		}
		ms = append(ms, s/float64(c))
	}
	sort.Float64s(ms)
	return percentile(ms, alpha/2), percentile(ms, 1-alpha/2)
}

func regimeBootLevel(ic []float64, on []bool, block, draws int, seed int64, alpha float64) (float64, float64, float64) {
	n := len(ic)
	diffOf := func(idx []int) float64 {
		var a, b []float64
		for _, i := range idx {
			if on[i] {
				a = append(a, ic[i])
			} else {
				b = append(b, ic[i])
			}
		}
		if len(a) < 20 || len(b) < 20 {
			return nan
		}
		return mean(a) - mean(b)
	}
	all := make([]int, n)
	for i := range all {
		all[i] = i
	}
	point := diffOf(all)
	if n < 2*block {
		return point, nan, nan
	}
	rng := rand.New(rand.NewSource(seed))
	var ds []float64
	for b := 0; b < draws; b++ {
		var idx []int
		for q := 0; q < (n+block-1)/block; q++ {
			st := rng.Intn(n - block + 1)
			for k := 0; k < block; k++ {
				idx = append(idx, st+k)
			}
		}
		if v := diffOf(idx); !math.IsNaN(v) {
			ds = append(ds, v)
		}
	}
	sort.Float64s(ds)
	if len(ds) < 100 {
		return point, nan, nan
	}
	return point, percentile(ds, alpha/2), percentile(ds, 1-alpha/2)
}

func orthResidual(sig, beta []float64) []float64 {
	idx := []int{}
	for i := range sig {
		if !math.IsNaN(sig[i]) && !math.IsNaN(beta[i]) {
			idx = append(idx, i)
		}
	}
	out := filled(len(sig))
	if len(idx) < 30 {
		return out
	}
	s, b := make([]float64, len(idx)), make([]float64, len(idx))
	for k, i := range idx {
		s[k], b[k] = sig[i], beta[i]
	}
	rs, rb := rankScale(s), rankScale(b)
	mb := mean(rb)
	var cov, vr float64
	for k := range rs {
		cov += (rs[k] - mean(rs)) * (rb[k] - mb)
		vr += (rb[k] - mb) * (rb[k] - mb)
	}
	slope := 0.0
	if vr > 0 {
		slope = cov / vr
	}
	for k, i := range idx {
		out[i] = rs[k] - slope*(rb[k]-mb)
	}
	return out
}

func terciles(mod []float64, valid []int) (low, high []int) {
	idx := append([]int{}, valid...)
	sort.SliceStable(idx, func(a, b int) bool { return mod[idx[a]] < mod[idx[b]] })
	k := len(idx) / 3
	return idx[:k], idx[len(idx)-k:]
}

func icOn(sig, y []float64, idx []int) float64 {
	if len(idx) < 15 {
		return nan
	}
	s, o := make([]float64, len(idx)), make([]float64, len(idx))
	for k, i := range idx {
		s[k], o[k] = sig[i], y[i]
	}
	return pearson(rankScale(s), rankScale(o))
}

func heteroRun(pp *pipe, sigPath, modPath, macroDir, study string, k int) {
	st, ok := heteroStudies()[study]
	if !ok {
		fatal(fmt.Errorf("unknown hetero study %s", study))
	}
	ds := pp.ds
	hd := heteroLoad(sigPath, modPath)
	lo, hi := sortSearch(ds.dates, st.from), sortSearch(ds.dates, st.till)
	if hi >= len(ds.dates) || ds.dates[hi] != st.till {
		hi = sortSearch(ds.dates, st.till) - 1
	}
	splitDate := st.split
	if splitDate == "" {
		splitDate = "2023-07-01"
	}
	split := sortSearch(ds.dates, splitDate) - lo
	alpha := 0.05 / float64(k)
	fmt.Printf("hetero: study %s, %s..%s, %d tests, Bonferroni-adjusted level %.4f, halves split %s\n", st.name, ds.dates[lo], ds.dates[hi], len(st.tests), alpha, splitDate)
	for ti, tc := range st.tests {
		a, b := filled(hi-lo+1), filled(hi-lo+1)
		var flags []bool
		if tc.kind == "regime" {
			flags = regimeFlags(ds, macroDir, tc.reg)
		}
		placeboRuns := 5
		pa, pb := make([][]float64, placeboRuns), make([][]float64, placeboRuns)
		for p := range pa {
			pa[p], pb[p] = filled(hi-lo+1), filled(hi-lo+1)
		}
		for d := lo; d <= hi; d++ {
			rows := ds.byDate[d]
			n := len(rows)
			sig, y, mod, sigB := filled(n), filled(n), filled(n), filled(n)
			for i, x := range rows {
				sig[i] = hd.sig(ds.dates[d], x.secid, tc.sig)
				y[i] = outcomeOf(x, tc.hh)
				if tc.mod != "" {
					mod[i] = hd.mod(ds.dates[d], x.secid, tc.mod)
				}
				if tc.sigB != "" {
					sigB[i] = hd.sig(ds.dates[d], x.secid, tc.sigB)
				}
			}
			var valid []int
			for i := range rows {
				ok := !math.IsNaN(sig[i]) && !math.IsNaN(y[i])
				if tc.kind == "tercile" || tc.kind == "orth" {
					ok = ok && !math.IsNaN(mod[i])
				}
				if tc.kind == "paired" {
					ok = ok && !math.IsNaN(sigB[i])
				}
				if ok {
					valid = append(valid, i)
				}
			}
			if len(valid) < 45 {
				continue
			}
			switch tc.kind {
			case "tercile":
				l, h := terciles(mod, valid)
				a[d-lo], b[d-lo] = icOn(sig, y, h), icOn(sig, y, l)
				for p := 0; p < placeboRuns; p++ {
					pm := append([]float64{}, mod...)
					permuteSignal(pm, int64(20261201+p), d)
					l, h := terciles(pm, valid)
					pa[p][d-lo], pb[p][d-lo] = icOn(sig, y, h), icOn(sig, y, l)
				}
			case "regime":
				a[d-lo] = icOn(sig, y, valid)
			case "orth":
				res := orthResidual(sig, mod)
				a[d-lo], b[d-lo] = icOn(sig, y, valid), icOn(res, y, valid)
				for p := 0; p < placeboRuns; p++ {
					pbeta := append([]float64{}, mod...)
					permuteSignal(pbeta, int64(20261201+p), d)
					pres := orthResidual(sig, pbeta)
					pa[p][d-lo], pb[p][d-lo] = icOn(sig, y, valid), icOn(pres, y, valid)
				}
			case "paired":
				a[d-lo], b[d-lo] = icOn(sig, y, valid), icOn(sigB, y, valid)
			case "posbucket":
				var pos []int
				for _, i := range valid {
					if sig[i] > 0 {
						pos = append(pos, i)
					}
				}
				if len(pos) < 30 {
					continue
				}
				l, h := terciles(sig, pos)
				a[d-lo], b[d-lo] = meanOn(y, h), meanOn(y, l)
				for p := 0; p < placeboRuns; p++ {
					ps := append([]float64{}, sig...)
					permuteSignal(ps, int64(20261201+p), d)
					var ppos []int
					for _, i := range valid {
						if ps[i] > 0 {
							ppos = append(ppos, i)
						}
					}
					if len(ppos) >= 30 {
						l, h := terciles(ps, ppos)
						pa[p][d-lo], pb[p][d-lo] = meanOn(y, h), meanOn(y, l)
					}
				}
			}
		}
		fmt.Println(heteroReport(ti, tc, a, b, pa, pb, flags, lo, split, alpha))
	}
}

func meanOn(y []float64, idx []int) float64 {
	if len(idx) == 0 {
		return nan
	}
	s := 0.0
	for _, i := range idx {
		s += y[i]
	}
	return s / float64(len(idx))
}

func pairDiff(a, b []float64) []float64 {
	o := filled(len(a))
	for i := range a {
		if !math.IsNaN(a[i]) && !math.IsNaN(b[i]) {
			o[i] = a[i] - b[i]
		}
	}
	return o
}

func heteroReport(ti int, tc heteroTest, a, b []float64, pa, pb [][]float64, flags []bool, lo, split int, alpha float64) string {
	seed := int64(20261201 + ti)
	var diff []float64
	var point, ciLo, ciHi, h1, h2 float64
	desc := ""
	if tc.kind == "regime" {
		var ic []float64
		var on []bool
		var idx []int
		for i := range a {
			if !math.IsNaN(a[i]) {
				ic, on, idx = append(ic, a[i]), append(on, flags[lo+i]), append(idx, i)
			}
		}
		if len(ic) < 200 {
			return fmt.Sprintf("%-84s not testable (%d dates)", tc.id, len(ic))
		}
		point, ciLo, ciHi = regimeBootLevel(ic, on, 20, 5000, seed, alpha)
		var first, second []float64
		var f1, f2 []bool
		for k, i := range idx {
			if i < split {
				first, f1 = append(first, ic[k]), append(f1, on[k])
			} else {
				second, f2 = append(second, ic[k]), append(f2, on[k])
			}
		}
		h1, h2 = onOffDiff(first, f1), onOffDiff(second, f2)
		var onV, offV []float64
		for k := range ic {
			if on[k] {
				onV = append(onV, ic[k])
			} else {
				offV = append(offV, ic[k])
			}
		}
		desc = fmt.Sprintf("on %+.4f (%d) off %+.4f (%d)", mean(onV), len(onV), mean(offV), len(offV))
		placeboHits := 0
		for p := 0; p < 5; p++ {
			pf := circBlockPerm(flags[lo:lo+len(a)], 20, int64(20261301+p)+seed)
			var pi []bool
			for _, i := range idx {
				pi = append(pi, pf[i])
			}
			_, plo, phi := regimeBootLevel(ic, pi, 20, 2000, seed+int64(p), alpha)
			if plo > 0 || phi < 0 {
				placeboHits++
			}
		}
		hit := (ciLo > 0 || ciHi < 0) && h1*point > 0 && h2*point > 0 && placeboHits == 0
		return fmt.Sprintf("%-84s diff %+.4f [%+.4f, %+.4f] halves %+.4f / %+.4f  %s  placebo hits %d of 5  %s", tc.id, point, ciLo, ciHi, h1, h2, desc, placeboHits, hitWord(hit))
	}
	diff = pairDiff(a, b)
	if tc.kind == "orth" {
		diff = pairDiff(b, a)
	}
	v := clean(diff)
	if len(v) < 200 {
		return fmt.Sprintf("%-84s not testable (%d dates)", tc.id, len(v))
	}
	point = mean(v)
	ciLo, ciHi = blockBootLevel(v, 20, 5000, seed, alpha)
	h1, h2 = halves(diff, split)
	ma, mb := mean(clean(a)), mean(clean(b))
	desc = fmt.Sprintf("A %+.4f B %+.4f", ma, mb)
	if tc.kind == "paired" {
		ma, mb = mean(clean(a)), mean(clean(b))
		desc = fmt.Sprintf("first %+.4f second %+.4f", ma, mb)
	}
	placeboHits := 0
	for p := 0; p < 5; p++ {
		var pd []float64
		if tc.kind == "paired" {
			pd = signFlipBlocks(diff, 20, int64(20261301+p)+seed)
		} else {
			pd = pairDiff(pa[p], pb[p])
			if tc.kind == "orth" {
				pd = pairDiff(pb[p], pa[p])
			}
		}
		pv := clean(pd)
		if len(pv) < 200 {
			continue
		}
		plo, phi := blockBootLevel(pv, 20, 2000, seed+int64(p), alpha)
		if plo > 0 || phi < 0 {
			placeboHits++
		}
	}
	hit := (ciLo > 0 || ciHi < 0) && h1*point > 0 && h2*point > 0 && placeboHits == 0
	return fmt.Sprintf("%-84s diff %+.4f [%+.4f, %+.4f] halves %+.4f / %+.4f  %s  dates %d  placebo hits %d of 5  %s", tc.id, point, ciLo, ciHi, h1, h2, desc, len(v), placeboHits, hitWord(hit))
}

func hitWord(h bool) string {
	if h {
		return "HIT (lead)"
	}
	return "no hit"
}

func onOffDiff(ic []float64, on []bool) float64 {
	var a, b []float64
	for i := range ic {
		if on[i] {
			a = append(a, ic[i])
		} else {
			b = append(b, ic[i])
		}
	}
	if len(a) < 10 || len(b) < 10 {
		return nan
	}
	return mean(a) - mean(b)
}

func signFlipBlocks(x []float64, block int, seed int64) []float64 {
	rng := rand.New(rand.NewSource(seed))
	out := make([]float64, len(x))
	sign := 1.0
	for i := range x {
		if i%block == 0 {
			sign = 1
			if rng.Intn(2) == 0 {
				sign = -1
			}
		}
		out[i] = sign * x[i]
	}
	return out
}
