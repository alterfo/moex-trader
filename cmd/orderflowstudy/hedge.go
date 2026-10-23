package main

import (
	"fmt"
	"io"
	"math"
)

const (
	hedgeFlatCost = 0.0001
	annualDays    = 252
)

func imoexReturn(cal []string, closes map[string]float64, from, to string) float64 {
	a, ok1 := closes[from]
	b, ok2 := closes[to]
	if !ok1 || !ok2 || a <= 0 {
		return nan
	}
	return b/a - 1
}

func (p *panel) a2Daily(rows [][]fmRow) []dailyPoint {
	return p.tradeForm(rows, 1, 1, 0.2).daily
}

func betaOf(y, x []float64) float64 {
	mx, my := mean(x), mean(y)
	var sxy, sxx float64
	for i := range x {
		sxy += (x[i] - mx) * (y[i] - my)
		sxx += (x[i] - mx) * (x[i] - mx)
	}
	if sxx == 0 {
		return nan
	}
	return sxy / sxx
}

func (p *panel) reportHedge(w io.Writer, rows [][]fmRow, closes map[string]float64, cal []string, kr map[string]float64) {
	pts := p.a2Daily(rows)
	var hedged, hedged05, longOnly, top, idx []float64
	var h0, h1 []float64
	var krs []float64
	for _, pt := range pts {
		d := pt.day
		if d+1 >= len(p.dates) {
			continue
		}
		r := imoexReturn(cal, closes, p.dates[d], p.dates[d+1])
		if math.IsNaN(r) {
			continue
		}
		net := pt.rawTop - r - costLive*pt.turnover - hedgeFlatCost
		hedged = append(hedged, net)
		hedged05 = append(hedged05, pt.rawTop-r-costLow*pt.turnover-hedgeFlatCost)
		longOnly = append(longOnly, pt.rawTop-costLive*pt.turnover)
		top = append(top, pt.rawTop)
		idx = append(idx, r)
		if p.half(d) == 0 {
			h0 = append(h0, net)
		} else {
			h1 = append(h1, net)
		}
		if v, ok := kr[p.dates[d]]; ok {
			krs = append(krs, v)
		}
	}
	m, t := neweyWest(hedged, nwLags)
	m0, t0 := neweyWest(h0, nwLags)
	m1, t1 := neweyWest(h1, nwLags)
	ann := m * annualDays * 100
	fmt.Fprintf(w, "F1 hedged A2 (top quintile minus IMOEX, beta 1): days %d, mean net %s per day, NW t %+.2f, halves %s (t %+.2f, n %d) / %s (t %+.2f, n %d)\n", len(hedged), pct(m), t, pct(m0), t0, len(h0), pct(m1), t1, len(h1))
	sd := 0.0
	for _, v := range hedged {
		sd += (v - m) * (v - m)
	}
	sd = math.Sqrt(sd / float64(len(hedged)-1))
	fmt.Fprintf(w, "  annualised mean net %+.1f%% (arithmetic, times 252), daily sd %s, ratio mean/sd times sqrt(252) %.2f; mean key rate on the days where known %.1f%% (%d days)\n", ann, pct(sd), m/sd*math.Sqrt(annualDays), mean(krs), len(krs))
	mh, _ := neweyWest(hedged05, nwLags)
	ml, tl := neweyWest(longOnly, nwLags)
	fmt.Fprintf(w, "  descriptive: hedged at 0.05%% per side %s per day; long-only net %s per day (NW t %+.2f, annualised %+.1f%%); beta of the top quintile to IMOEX %.2f\n", pct(mh), pct(ml), tl, ml*annualDays*100, betaOf(top, idx))
	pass := !math.IsNaN(t) && t >= tThresh && m0 > 0 && m1 > 0
	econ := ann > mean(krs)
	fmt.Fprintf(w, "  criterion (t >= 3.0 and positive in both halves): %s; economic reading (annualised net above the key rate): %v\n", map[bool]string{true: "PASS", false: "FAIL"}[pass], econ)
}

func (p *panel) reportForward(w io.Writer, rows [][]fmRow, from string) {
	var nets, grosses []float64
	var days []int
	for _, pt := range p.a2Daily(rows) {
		if p.dates[pt.day] >= from {
			nets = append(nets, pt.net)
			grosses = append(grosses, pt.gross)
			days = append(days, pt.day)
		}
	}
	fmt.Fprintf(w, "== Forward evaluation of the frozen A2 rule from %s\n", from)
	if len(nets) == 0 {
		fmt.Fprintln(w, "no forward dates with an outcome yet")
		return
	}
	m, t := neweyWest(nets, nwLags)
	fmt.Fprintf(w, "forward dates with an outcome %d (%s .. %s), mean gross %s, mean net %s (0.15%% per side), NW t %+.2f\n", len(nets), p.dates[days[0]], p.dates[days[len(days)-1]], pct(mean(grosses)), pct(m), t)
	sub := make([][]fmRow, len(rows))
	for d := range rows {
		if p.dates[d] >= from {
			sub[d] = rows[d]
		}
	}
	res := runFM(sub, 1, false)
	var betas []float64
	betas = append(betas, res.betas...)
	cm, ct := neweyWest(betas, nwLags)
	fmt.Fprintf(w, "Fama-MacBeth A2 coefficient over the same dates: %+.5f, NW t %+.2f (dates %d)\n", cm, ct, len(betas))
	fmt.Fprintf(w, "criterion (net mean > 0, NW t >= 2.0, coefficient > 0, at least 60 dates): %v (dates %d)\n", len(nets) >= 60 && m > 0 && t >= 2.0 && cm > 0, len(nets))
}
