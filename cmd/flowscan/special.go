package main

import (
	"fmt"
	"io"
	"math"
	"sort"
	"time"
)

func normP(t float64) float64 {
	if math.IsNaN(t) {
		return 1
	}
	return math.Erfc(math.Abs(t) / math.Sqrt2)
}

func normQuantile(alpha float64) float64 {
	lo, hi := 0.0, 10.0
	for k := 0; k < 100; k++ {
		mid := (lo + hi) / 2
		if math.Erfc(mid/math.Sqrt2) > alpha {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2
}

func regNW(x, y []float64, lags int) (float64, float64) {
	n := len(x)
	if n < lags+10 {
		return nan, nan
	}
	mx, my := mean(x), mean(y)
	sxx, sxy := 0.0, 0.0
	for k := range x {
		sxx += (x[k] - mx) * (x[k] - mx)
		sxy += (x[k] - mx) * (y[k] - my)
	}
	if sxx <= 0 {
		return nan, nan
	}
	b := sxy / sxx
	a := my - b*mx
	u := make([]float64, n)
	for k := range x {
		u[k] = (x[k] - mx) * (y[k] - a - b*x[k])
	}
	s := 0.0
	for _, v := range u {
		s += v * v
	}
	for l := 1; l <= lags; l++ {
		g := 0.0
		for k := l; k < n; k++ {
			g += u[k] * u[k-l]
		}
		s += 2 * (1 - float64(l)/float64(lags+1)) * g
	}
	if s <= 0 {
		return b, nan
	}
	return b, b / (math.Sqrt(s) / sxx)
}

func ir(v []float64) float64 {
	sd := stdOf(v)
	if !(sd > 0) {
		return nan
	}
	return mean(v) / sd * math.Sqrt(252)
}

func (r *run) h069(w io.Writer, base evalOut) {
	var eq, wt []float64
	prevW := map[int]map[int]float64{}
	for _, dp := range base.ports {
		var inv []float64
		tot := 0.0
		for _, ti := range dp.members {
			v := prevVals(r.p, ti, dp.d, bRV, 20, true)
			x := nan
			if len(v) >= 12 && mean(v) > 0 {
				x = 1 / mean(v)
			}
			inv = append(inv, x)
			if !math.IsNaN(x) {
				tot += x
			}
		}
		if tot <= 0 {
			continue
		}
		cur := map[int]float64{}
		g := 0.0
		for k, ti := range dp.members {
			wk := 1 / float64(len(dp.members))
			if !math.IsNaN(inv[k]) {
				wk = inv[k] / tot
			}
			cur[ti] = wk
			g += wk * dp.ys[k]
		}
		turn := 1.0
		if prev := prevW[dp.d-5]; prev != nil {
			turn = 0
			for t, x := range cur {
				turn += math.Abs(x - prev[t])
			}
			for t, x := range prev {
				if _, ok := cur[t]; !ok {
					turn += x
				}
			}
		}
		prevW[dp.d] = cur
		wt = append(wt, (g-costLive*turn)/5)
		eq = append(eq, dp.net)
	}
	diff := make([]float64, len(wt))
	for k := range wt {
		diff[k] = wt[k] - eq[k]
	}
	_, t := neweyWest(diff, 5)
	d := ir(wt) - ir(eq)
	pass := d >= 0.1 && t >= 2.0
	fmt.Fprintf(w, "H069 sizer on H001: IR equal %.2f, IR inverse-vol %.2f, gain %+.2f, NW t of net difference %+.2f, n %d -> %s\n", ir(eq), ir(wt), d, t, len(wt), passWord(pass))
}

func passWord(b bool) string {
	if b {
		return "PASS"
	}
	return "fail"
}

func (r *run) h080(w io.Writer, base evalOut) {
	p := r.p
	var hi, lo []float64
	for k, d := range base.netSeriesDay {
		adm := p.admittedAt(d)
		std := func(s int) float64 {
			var v []float64
			for _, i := range p.admittedAt(s) {
				if x := p.v(i, s, bA2); !math.IsNaN(x) {
					v = append(v, x)
				}
			}
			return stdOf(v)
		}
		_ = adm
		cur := std(d)
		var prev []float64
		for s := d - 60; s < d; s++ {
			if s > 61 {
				if x := std(s); !math.IsNaN(x) {
					prev = append(prev, x)
				}
			}
		}
		if len(prev) < 30 || math.IsNaN(cur) {
			continue
		}
		if cur > mean(prev) {
			hi = append(hi, base.netSeries[k])
		} else {
			lo = append(lo, base.netSeries[k])
		}
	}
	if len(hi) < 20 || len(lo) < 20 {
		fmt.Fprintf(w, "H080 H001 payoff by A2 dispersion regime: too few days (%d high, %d low)\n", len(hi), len(lo))
		return
	}
	se := math.Sqrt(math.Pow(stdOf(hi), 2)/float64(len(hi)) + math.Pow(stdOf(lo), 2)/float64(len(lo)))
	fmt.Fprintf(w, "H080 H001 net per day, high A2 dispersion %s (n %d) vs low %s (n %d), naive t of difference %+.2f\n", pct(mean(hi)), len(hi), pct(mean(lo)), len(lo), (mean(hi)-mean(lo))/se)
}

func (r *run) h082(w io.Writer) {
	p := r.p
	var x, y []float64
	for d := r.win.lo; d <= r.win.hi && d+1 < len(p.dates); d++ {
		adm := p.admittedAt(d)
		pos, n := 0, 0
		for _, i := range adm {
			if v := p.v(i, d, bA2); !math.IsNaN(v) {
				n++
				if v > 0 {
					pos++
				}
			}
		}
		u := p.uniRet[d+1]
		if n < minPerDay || math.IsNaN(u) {
			continue
		}
		x = append(x, float64(pos)/float64(n))
		y = append(y, u)
	}
	b, t := regNW(x, y, 5)
	fmt.Fprintf(w, "H082 breadth of positive A2 -> next-day equal-weighted universe return: slope %+.5f, NW t %+.2f, n %d\n", b, t, len(x))
}

func (r *run) h084(w io.Writer) {
	p := r.p
	pairs := [][2]string{{"SBER", "SBERP"}, {"TATN", "TATNP"}, {"SNGS", "SNGSP"}, {"BANE", "BANEP"}}
	var x, y []float64
	for d := r.win.lo; d <= r.win.hi && d+1 < len(p.dates); d++ {
		var gs, rs []float64
		for _, pr := range pairs {
			a, ok1 := p.tIdx[pr[0]]
			b, ok2 := p.tIdx[pr[1]]
			if !ok1 || !ok2 {
				continue
			}
			ga := p.v(a, d, bA2) - p.v(b, d, bA2)
			ra, rb := p.ret(a, d, d+1), p.ret(b, d, d+1)
			if math.IsNaN(ga) || math.IsNaN(ra) || math.IsNaN(rb) {
				continue
			}
			gs = append(gs, ga)
			rs = append(rs, ra-rb)
		}
		if len(gs) >= 2 {
			x = append(x, mean(gs))
			y = append(y, mean(rs))
		}
	}
	b, t := regNW(x, y, 5)
	fmt.Fprintf(w, "H084 ordinary-minus-preferred A2 gap -> next-day relative return: slope %+.5f, NW t %+.2f, n %d\n", b, t, len(x))
}

func weekdayOf(date string) time.Weekday {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return 0
	}
	return t.Weekday()
}

func (r *run) h092(w io.Writer, betas []float64, days []int) {
	p := r.p
	byWd := map[time.Weekday][]float64{}
	for k, d := range days {
		wd := weekdayOf(p.dates[d])
		byWd[wd] = append(byWd[wd], betas[k])
	}
	var wds []int
	for wd := range byWd {
		wds = append(wds, int(wd))
	}
	sort.Ints(wds)
	fmt.Fprint(w, "H092 A2 coefficient by weekday (mean, NW t of difference from the other days):")
	for _, wi := range wds {
		wd := time.Weekday(wi)
		var rest []float64
		for k, d := range days {
			if weekdayOf(p.dates[d]) != wd {
				rest = append(rest, betas[k])
			}
		}
		cur := byWd[wd]
		se := math.Sqrt(math.Pow(stdOf(cur), 2)/float64(len(cur)) + math.Pow(stdOf(rest), 2)/float64(len(rest)))
		fmt.Fprintf(w, " %s %+.5f (%+.2f)", wd.String()[:3], mean(cur), (mean(cur)-mean(rest))/se)
	}
	fmt.Fprintln(w)
}

func (r *run) h093(w io.Writer, betas []float64, days []int) {
	p := r.p
	var last, rest []float64
	for k, d := range days {
		if d+1 < len(p.dates) && p.dates[d+1][5:7] != p.dates[d][5:7] {
			last = append(last, betas[k])
		} else {
			rest = append(rest, betas[k])
		}
	}
	if len(last) < 5 {
		fmt.Fprintf(w, "H093 A2 coefficient on the last trading day of the month: too few days (%d)\n", len(last))
		return
	}
	se := math.Sqrt(math.Pow(stdOf(last), 2)/float64(len(last)) + math.Pow(stdOf(rest), 2)/float64(len(rest)))
	fmt.Fprintf(w, "H093 A2 coefficient, last trading day of month %+.5f (n %d) vs rest %+.5f, naive t of difference %+.2f\n", mean(last), len(last), mean(rest), (mean(last)-mean(rest))/se)
}

func (r *run) h094(w io.Writer) {
	p := r.p
	type acc struct {
		rows, dropped [3]int
		a2            []float64
	}
	by := map[string]*acc{}
	for d := range p.dates {
		yr := p.dates[d][:4]
		a := by[yr]
		if a == nil {
			a = &acc{}
			by[yr] = a
		}
		for k := 0; k < 3; k++ {
			a.rows[k] += p.drops[d].rows[k]
			a.dropped[k] += p.drops[d].dropped[k]
		}
		for _, i := range p.admittedAt(d) {
			if v := p.v(i, d, bA2); !math.IsNaN(v) {
				a.a2 = append(a.a2, v)
			}
		}
	}
	var yrs []string
	for y := range by {
		yrs = append(yrs, y)
	}
	sort.Strings(yrs)
	fmt.Fprintln(w, "H094 provenance by year (rows read / rows dropped by the SYSTIME rule: tradestats, obstats, orderstats; A2 mean and std of admitted ticker-days)")
	for _, y := range yrs {
		a := by[y]
		fmt.Fprintf(w, "  %s: %d/%d, %d/%d, %d/%d; A2 mean %+.4f std %.4f, n %d\n", y, a.rows[0], a.dropped[0], a.rows[1], a.dropped[1], a.rows[2], a.dropped[2], mean(a.a2), stdOf(a.a2), len(a.a2))
	}
}
