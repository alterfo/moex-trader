package main

import "math"

var featNames = []string{"BI", "TI", "NOF", "CI", "DG", "SR", "TZ", "OT"}

var featDeps = map[string]string{"BI": "to", "TI": "t", "NOF": "st", "CI": "s", "DG": "o", "SR": "o", "TZ": "t", "OT": "ts"}

var featSign = map[string]float64{"BI": 1, "TI": 1, "NOF": 1, "CI": 1, "DG": 1, "SR": 1, "TZ": 1, "OT": -1}

type featMat map[string][][]float64

func buildFeat(s *store) featMat {
	nd := len(s.dates)
	out := featMat{}
	for _, n := range featNames {
		out[n] = make([][]float64, len(s.groups))
	}
	for gi, g := range s.groups {
		for _, n := range featNames {
			v := make([]float64, nd)
			for i := range v {
				v[i] = nan
			}
			out[n][gi] = v
		}
		rows := s.byGroup[g]
		for d := 0; d < nd; d++ {
			x := rows[d]
			if x == nil || s.excl[d] || s.expired(x.secid, d) {
				continue
			}
			ok := func(deps string) bool { return eligible(x, deps) }
			put := func(n string, v float64) {
				if !math.IsNaN(v) && !math.IsInf(v, 0) && ok(featDeps[n]) {
					out[n][gi][d] = v
				}
			}
			put("BI", x.bi)
			put("TI", x.ti)
			put("NOF", x.nof)
			put("CI", x.ci)
			var dep, sp, tk, ot []float64
			for k := d - 20; k < d; k++ {
				if k < 0 {
					continue
				}
				y := rows[k]
				if y == nil || y.secid != x.secid {
					continue
				}
				if y.depth > 0 && eligible(y, featDeps["DG"]) {
					dep = append(dep, y.depth)
				}
				if y.relSpread > 0 && eligible(y, featDeps["SR"]) {
					sp = append(sp, y.relSpread)
				}
				if y.ticket > 0 && eligible(y, featDeps["TZ"]) {
					tk = append(tk, math.Log(y.ticket))
				}
				if !math.IsNaN(y.otr) && eligible(y, featDeps["OT"]) {
					ot = append(ot, y.otr)
				}
			}
			if len(dep) >= 10 && x.depth > 0 {
				put("DG", math.Log(x.depth/mean(dep)))
			}
			if len(sp) >= 10 && x.relSpread > 0 {
				put("SR", math.Log(x.relSpread/mean(sp)))
			}
			if len(tk) >= 10 && x.ticket > 0 {
				if sd := stdOf(tk); sd > 0 {
					put("TZ", (math.Log(x.ticket)-mean(tk))/sd)
				}
			}
			if len(ot) >= 10 && !math.IsNaN(x.otr) {
				if sd := stdOf(ot); sd > 0 {
					put("OT", (x.otr-mean(ot))/sd)
				}
			}
		}
	}
	return out
}

func (s *store) gi(g string) int {
	for i, x := range s.groups {
		if x == g {
			return i
		}
	}
	return -1
}

func (s *store) turnZ(g string, d int) float64 {
	rows := s.byGroup[g]
	x := rows[d]
	if x == nil || !(x.valDay > 0) {
		return nan
	}
	var v []float64
	for k := d - 20; k < d; k++ {
		if k >= 0 && rows[k] != nil && rows[k].valDay > 0 {
			v = append(v, rows[k].valDay)
		}
	}
	if len(v) < 10 {
		return nan
	}
	return math.Log(x.valDay / mean(v))
}

func (s *store) liquid(d, topN int) map[string]bool {
	type c struct {
		g string
		m float64
	}
	var cs []c
	for _, g := range s.groups {
		var v []float64
		for k := d - 60; k < d; k++ {
			if k >= 0 && s.byGroup[g][k] != nil {
				v = append(v, s.byGroup[g][k].valDay)
			}
		}
		if len(v) < 40 {
			continue
		}
		cs = append(cs, c{g, medianOf(v)})
	}
	for i := 0; i < len(cs); i++ {
		for j := i + 1; j < len(cs); j++ {
			if cs[j].m > cs[i].m {
				cs[i], cs[j] = cs[j], cs[i]
			}
		}
	}
	out := map[string]bool{}
	for i := 0; i < len(cs) && i < topN; i++ {
		out[cs[i].g] = true
	}
	return out
}

func medianOf(v []float64) float64 {
	w := append([]float64(nil), v...)
	for i := 1; i < len(w); i++ {
		for j := i; j > 0 && w[j] < w[j-1]; j-- {
			w[j], w[j-1] = w[j-1], w[j]
		}
	}
	return w[len(w)/2]
}

var allowLate bool

var minDates = 80

func eligible(x *rec, deps string) bool {
	if allowLate {
		return true
	}
	for _, c := range deps {
		switch c {
		case 't':
			if x.lateTS > 0.2 {
				return false
			}
		case 'o':
			if x.lateOB > 0.2 {
				return false
			}
		case 's':
			if x.lateOS > 0.2 {
				return false
			}
		}
	}
	return true
}
