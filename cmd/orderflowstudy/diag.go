package main

import (
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
)

func (p *panel) tLags(r fmResult, lags int) float64 {
	_, t := neweyWest(r.betas, lags)
	return t
}

func (p *panel) byYear(r fmResult) map[string][]float64 {
	out := map[string][]float64{}
	for i, d := range r.days {
		y := p.dates[d][:4]
		out[y] = append(out[y], r.betas[i])
	}
	return out
}

func (p *panel) demeanedRows(rows [][]fmRow, feat int, window int) [][]fmRow {
	out := make([][]fmRow, len(rows))
	for d, rs := range rows {
		for _, r := range rs {
			sum, n := 0.0, 0
			for k := d - window; k < d; k++ {
				if k < 0 {
					continue
				}
				if rec := p.recs[r.ti][k]; rec != nil && !math.IsNaN(rec.feat[feat]) {
					sum += rec.feat[feat]
					n++
				}
			}
			if n < window/2 {
				continue
			}
			nr := r
			nr.feats = append([]float64(nil), r.feats...)
			nr.feats[feat] = r.feats[feat] - sum/float64(n)
			out[d] = append(out[d], nr)
		}
	}
	return out
}

func (p *panel) persistence(rows [][]fmRow, feat int) float64 {
	var cors []float64
	for d := 1; d < len(rows); d++ {
		prev := map[int]float64{}
		for _, r := range rows[d-1] {
			if !math.IsNaN(r.feats[feat]) {
				prev[r.ti] = r.feats[feat]
			}
		}
		var a, b []float64
		for _, r := range rows[d] {
			if v, ok := prev[r.ti]; ok && !math.IsNaN(r.feats[feat]) {
				a = append(a, v)
				b = append(b, r.feats[feat])
			}
		}
		if len(a) < minPerDay {
			continue
		}
		ra, rb := rankScale(a), rankScale(b)
		ma, mb := mean(ra), mean(rb)
		var sab, saa, sbb float64
		for i := range ra {
			sab += (ra[i] - ma) * (rb[i] - mb)
			saa += (ra[i] - ma) * (ra[i] - ma)
			sbb += (rb[i] - mb) * (rb[i] - mb)
		}
		cors = append(cors, sab/math.Sqrt(saa*sbb))
	}
	return mean(cors)
}

type cumSeries struct {
	cum []float64
	bad []int
}

func (p *panel) cumulative(i int) cumSeries {
	n := len(p.dates)
	cs := cumSeries{cum: make([]float64, n), bad: make([]int, n)}
	for d := 1; d < n; d++ {
		r := p.logRet(i, d)
		cs.cum[d], cs.bad[d] = cs.cum[d-1], cs.bad[d-1]
		if math.IsNaN(r) {
			cs.bad[d]++
		} else {
			cs.cum[d] += r
		}
	}
	return cs
}

func (p *panel) withFactorControls(rows [][]fmRow) [][]fmRow {
	cums := map[int]cumSeries{}
	out := make([][]fmRow, len(rows))
	for d, rs := range rows {
		for _, r := range rs {
			cs, ok := cums[r.ti]
			if !ok {
				cs = p.cumulative(r.ti)
				cums[r.ti] = cs
			}
			if d < 260 || cs.bad[d]-cs.bad[d-250] != 0 {
				continue
			}
			hi := cs.cum[d-250]
			for k := d - 250; k <= d; k++ {
				if cs.cum[k] > hi {
					hi = cs.cum[k]
				}
			}
			nr := r
			nr.extra = []float64{cs.cum[d] - cs.cum[d-20], cs.cum[d] - cs.cum[d-60], cs.cum[d-21] - cs.cum[d-250], cs.cum[d] - hi}
			out[d] = append(out[d], nr)
		}
	}
	return out
}

func loadKeyRate(path string) map[string]float64 {
	out := map[string]float64{}
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(data), "\n")[1:] {
		f := strings.Split(strings.TrimSpace(line), ",")
		if len(f) == 2 {
			if v, e := strconv.ParseFloat(f[1], 64); e == nil {
				out[f[0]] = v
			}
		}
	}
	return out
}

func (p *panel) context(w io.Writer, rows [][]fmRow) {
	kr := loadKeyRate("data/macro/keyrate.csv")
	byYear := map[string][]float64{}
	var all []float64
	var krs []float64
	for d := 7; d+1 < len(p.dates); d++ {
		if len(rows[d]) < minPerDay {
			continue
		}
		adm := p.admittedAt(d, 100)
		sum, n := 0.0, 0
		for _, i := range adm {
			if r := p.logRet(i, d+1); !math.IsNaN(r) {
				sum += r
				n++
			}
		}
		if n < minPerDay {
			continue
		}
		u := sum / float64(n)
		byYear[p.dates[d][:4]] = append(byYear[p.dates[d][:4]], u)
		all = append(all, u)
		if v, ok := kr[p.dates[d]]; ok {
			krs = append(krs, v)
		}
	}
	var parts []string
	for y := 2020; y <= 2026; y++ {
		if v := byYear[fmt.Sprintf("%d", y)]; len(v) > 0 {
			parts = append(parts, fmt.Sprintf("%d %+.1f%%", y, mean(v)*252*100))
		}
	}
	fmt.Fprintf(w, "context: equal-weighted admitted universe, mean daily log return times 252 by year: %v; all years %+.1f%%; mean key rate on dates where known (%d days) %.1f%%\n", parts, mean(all)*252*100, len(krs), mean(krs))
}

func runDiagnostics(w io.Writer, p *panel) {
	names := []string{"A1 trade imbalance", "A2 book imbalance (10 levels)", "A3 top-of-book imbalance", "A4 net order flow", "A5 cancellation intensity"}
	rows := p.baseRowsA(100)
	p.context(w, rows)
	fmt.Fprintln(w, "== Post-hoc diagnostics of block A (outside the registered criteria)")
	for k, n := range names {
		res := runFM(rows, k, false)
		s := p.summarize(res)
		fmt.Fprintf(w, "\n%s: coef %+.5f, NW t at 5 lags %+.2f, at 20 lags %+.2f, at 60 lags %+.2f, mean day-to-day rank autocorrelation of the feature %.3f\n", n, s.mean, s.t, p.tLags(res, 20), p.tLags(res, 60), p.persistence(rows, k))
		var ys []string
		by := p.byYear(res)
		for y := 2020; y <= 2026; y++ {
			key := fmt.Sprintf("%d", y)
			if v := by[key]; len(v) > 0 {
				ys = append(ys, fmt.Sprintf("%s %+.5f", key, mean(v)))
			}
		}
		fmt.Fprintf(w, "  by year: %v\n", ys)
		dm := p.demeanedRows(rows, k, 60)
		rd := runFM(dm, k, false)
		sd := p.summarize(rd)
		fmt.Fprintf(w, "  feature minus the ticker's own previous 60-day mean: coef %+.5f, NW t at 5 lags %+.2f, at 60 lags %+.2f (halves %+.2f / %+.2f)\n", sd.mean, sd.t, p.tLags(rd, 60), sd.t0, sd.t1)
		if k == 1 {
			fr := p.withFactorControls(rows)
			rf := runFM(fr, k, false)
			sf := p.summarize(rf)
			fmt.Fprintf(w, "  with extra controls (20-day and 60-day return, 12-1 momentum, distance to 52-week high): coef %+.5f, NW t at 5 lags %+.2f, at 60 lags %+.2f (halves %+.2f / %+.2f)\n", sf.mean, sf.t, p.tLags(rf, 60), sf.t0, sf.t1)
			tf := p.tradeForm(fr, k, 1, 0.2)
			fmt.Fprintf(w, "  same rows, top quintile minus universe: gross %s, net 0.15%% %s (halves %s / %s)\n", pct(tf.grossAll), pct(tf.netAll), pct(tf.net[0]), pct(tf.net[1]))
			for h := 2; h <= 5; h++ {
				rh := runFM(p.baseRowsAh(100, h), k, false)
				sh := p.summarize(rh)
				fmt.Fprintf(w, "  outcome of day t+%d only: coef %+.5f, NW t %+.2f\n", h, sh.mean, sh.t)
			}
			for _, n := range []int{20, 30, 50} {
				rn := runFM(p.baseRowsA(n), k, false)
				sn := p.summarize(rn)
				fmt.Fprintf(w, "  universe top %d: coef %+.5f, NW t %+.2f\n", n, sn.mean, sn.t)
			}
			ts := p.tradeForm(dm, k, 1, 0.2)
			fmt.Fprintf(w, "  demeaned top quintile minus universe per day: gross %s, net 0.15%% %s (halves %s / %s)\n", pct(ts.grossAll), pct(ts.netAll), pct(ts.net[0]), pct(ts.net[1]))
		}
	}
}
