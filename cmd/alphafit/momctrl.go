package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

func rankScaleMasked(v []float64) []float64 {
	idx := make([]int, 0, len(v))
	for i, x := range v {
		if !math.IsNaN(x) {
			idx = append(idx, i)
		}
	}
	out := filled(len(v))
	if len(idx) < 3 {
		return out
	}
	sub := make([]float64, len(idx))
	for k, i := range idx {
		sub[k] = v[i]
	}
	scaled := rankScale(sub)
	for k, i := range idx {
		out[i] = scaled[k]
	}
	return out
}

func rankResidual(y, ctrl []float64) []float64 {
	out := filled(len(y))
	var ys, xs []float64
	for i := range y {
		if math.IsNaN(y[i]) || math.IsNaN(ctrl[i]) {
			continue
		}
		ys = append(ys, y[i])
		xs = append(xs, ctrl[i])
	}
	if len(ys) < 10 {
		return out
	}
	coef := olsCoef([][]float64{xs}, ys)
	if len(coef) != 1 {
		return out
	}
	my, mx := mean(ys), mean(xs)
	slope := coef[0]
	for i := range out {
		if math.IsNaN(y[i]) || math.IsNaN(ctrl[i]) {
			continue
		}
		out[i] = (y[i] - my) - slope*(ctrl[i]-mx)
	}
	return out
}

type momArms struct {
	comp, mom, resid [][]float64
}

func momControl(pp *pipe, frzPath, outPath string) {
	ds := pp.ds
	raw, err := os.ReadFile(frzPath)
	if err != nil {
		fatal(err)
	}
	var fz frozen
	unmarshal(raw, &fz)
	t0 := sortSearch(ds.dates, "2020-04-01")
	blocks := [][2]string{{"2025-01-03", "2025-03-31"}, {"2025-04-01", "2025-06-30"}, {"2025-07-01", "2025-09-30"}, {"2025-10-01", "2025-12-31"}, {"2026-01-01", "2026-03-31"}, {"2026-04-01", "2026-06-30"}, {"2026-07-01", "2026-10-16"}}
	r := newRunner(pp, fz, []string{"blend"})
	r.keep = map[int][]float64{}
	for _, b := range blocks {
		lo, hi := pp.dateRange(b[0], b[1])
		r.block(lo, hi, t0, nil, false)
	}

	t := buildTfmCtx(pp, "")
	arms := &momArms{
		comp:  make([][]float64, len(ds.dates)),
		mom:   make([][]float64, len(ds.dates)),
		resid: make([][]float64, len(ds.dates)),
	}
	for d := range ds.dates {
		comp := r.keep[d]
		if comp == nil {
			continue
		}
		rows := ds.byDate[d]
		mom := filled(len(rows))
		for i, x := range rows {
			p, ok := t.pos[x.secid][d]
			if !ok || p < 255 {
				continue
			}
			mom[i] = sumRange(t.ser[x.secid].ab, p-255, p)
		}
		arms.comp[d] = comp
		arms.mom[d] = mom
		arms.resid[d] = rankResidual(rankScaleMasked(comp), rankScaleMasked(mom))
	}

	lo, hi := pp.dateRange("2025-01-03", "2026-10-16")
	split := sortSearch(ds.dates, "2026-01-05") - lo
	fmt.Printf("momctrl: window %s..%s (%d dates), frozen trade rate %.2f, carry 17%% and 25%%\n", ds.dates[lo], ds.dates[hi], hi-lo+1, fz.Rate)

	var lines []string
	emit := func(format string, args ...any) {
		line := fmt.Sprintf(format, args...)
		lines = append(lines, line)
		fmt.Println(line)
	}

	for _, rate := range []float64{fz.Rate, 0.50} {
		comp := runSleeveWith(t, "comp", rate, false, lo, hi, func(d int, rows []*row) []float64 { return arms.comp[d] })
		mom := runSleeveWith(t, "mom256", rate, false, lo, hi, func(d int, rows []*row) []float64 { return arms.mom[d] })
		res := runSleeveWith(t, "resid", rate, false, lo, hi, func(d int, rows []*row) []float64 { return arms.resid[d] })
		emit("momctrl rate %.2f: %s", rate, rateLine("composite", rate, comp))
		emit("momctrl rate %.2f: %s", rate, rateLine("mom256", rate, mom))
		emit("momctrl rate %.2f: %s", rate, rateLine("residual", rate, res))
		emit("momctrl rate %.2f: %s", rate, statLine("composite sleeve net 17% carry", comp.net17, split))
		emit("momctrl rate %.2f: %s", rate, statLine("mom256 sleeve net 17% carry", mom.net17, split))
		emit("momctrl rate %.2f: %s", rate, statLine("residual sleeve net 17% carry", res.net17, split))
		emit("momctrl rate %.2f: %s", rate, statLine("composite sleeve net 25% carry", comp.net25, split))
		emit("momctrl rate %.2f: %s", rate, statLine("mom256 sleeve net 25% carry", mom.net25, split))
		emit("momctrl rate %.2f: %s", rate, statLine("residual sleeve net 25% carry", res.net25, split))
		emit("momctrl rate %.2f: %s", rate, statLine("composite minus mom256, net 25% carry", seriesDiff(comp.net25, mom.net25), split))
		emit("momctrl rate %.2f: %s", rate, statLine("residual minus mom256, net 25% carry", seriesDiff(res.net25, mom.net25), split))
		emit("momctrl rate %.2f: %s", rate, statLine("composite minus residual, net 25% carry", seriesDiff(comp.net25, res.net25), split))
		emit("momctrl rate %.2f: %s", rate, statLine("composite minus mom256, net 17% carry", seriesDiff(comp.net17, mom.net17), split))
		emit("momctrl rate %.2f: %s", rate, statLine("residual minus mom256, net 17% carry", seriesDiff(res.net17, mom.net17), split))
		emit("momctrl rate %.2f: mean short notional weight composite %.3f, mom256 %.3f, residual %.3f",
			rate, mean(clean(comp.short))*100, mean(clean(mom.short))*100, mean(clean(res.short))*100)
	}

	for _, arm := range []struct {
		name string
		sig  [][]float64
	}{{"composite", arms.comp}, {"mom256", arms.mom}, {"residual", arms.resid}} {
		ics := make([]float64, hi-lo+1)
		for i := range ics {
			ics[i] = nan
		}
		for d := lo; d <= hi; d++ {
			if arm.sig[d] == nil {
				continue
			}
			ics[d-lo] = icOf(ds.byDate[d], arm.sig[d])
		}
		emit("momctrl IC: %s mean %+.5f, %s", arm.name, mean(clean(ics)), statLine(arm.name+" daily IC", ics, split))
	}

	if outPath != "" {
		if dir := filepath.Dir(outPath); dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				fatal(err)
			}
		}
		if err := os.WriteFile(outPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			fatal(err)
		}
	}
}

func seriesDiff(a, b []float64) []float64 {
	out := filled(len(a))
	for i := range a {
		if !math.IsNaN(a[i]) && !math.IsNaN(b[i]) {
			out[i] = a[i] - b[i]
		}
	}
	return out
}
