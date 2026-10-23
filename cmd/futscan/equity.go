package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"io"
	"math"
	"os"
	"sort"
	"strings"
)

type eqData struct {
	dates   []string
	dIdx    map[string]int
	closes  map[string][]float64
	adm     map[string][]bool
	divDrop map[string][]bool
	imoex   []float64
}

func loadEquity(blockPath, imoexPath, divPath string) (*eqData, error) {
	f, err := os.Open(blockPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(bufio.NewReaderSize(f, 1<<20))
	hdr, err := r.Read()
	if err != nil {
		return nil, err
	}
	ix := map[string]int{}
	for i, h := range hdr {
		ix[h] = i
	}
	type row struct {
		date, sec string
		cl        float64
		adm       bool
	}
	var rows []row
	dset := map[string]bool{}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		rows = append(rows, row{rec[ix["date"]], rec[ix["secid"]], f64(rec[ix["close"]]), rec[ix["adm100"]] == "1"})
		dset[rec[ix["date"]]] = true
	}
	e := &eqData{dIdx: map[string]int{}, closes: map[string][]float64{}, adm: map[string][]bool{}, divDrop: map[string][]bool{}}
	for d := range dset {
		e.dates = append(e.dates, d)
	}
	sort.Strings(e.dates)
	for i, d := range e.dates {
		e.dIdx[d] = i
	}
	nd := len(e.dates)
	for _, x := range rows {
		c := e.closes[x.sec]
		if c == nil {
			c = make([]float64, nd)
			for i := range c {
				c[i] = nan
			}
			e.closes[x.sec] = c
			e.adm[x.sec] = make([]bool, nd)
			e.divDrop[x.sec] = make([]bool, nd)
		}
		if x.cl > 0 {
			c[e.dIdx[x.date]] = x.cl
		}
		if x.adm {
			e.adm[x.sec][e.dIdx[x.date]] = true
		}
	}
	df, err := os.Open(divPath)
	if err == nil {
		defer df.Close()
		sc := bufio.NewScanner(df)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			var rec struct {
				Ticker  string `json:"ticker"`
				LastBuy string `json:"last_buy_date"`
			}
			if json.Unmarshal(sc.Bytes(), &rec) != nil || len(rec.LastBuy) < 10 {
				continue
			}
			fl := e.divDrop[rec.Ticker]
			if fl == nil {
				continue
			}
			lb := rec.LastBuy[:10]
			ex := sort.SearchStrings(e.dates, lb)
			if ex < nd && e.dates[ex] == lb {
				ex++
			}
			for k := ex - 1; k <= ex+1; k++ {
				if k >= 0 && k < nd {
					fl[k] = true
				}
			}
		}
	}
	e.imoex = make([]float64, nd)
	for i := range e.imoex {
		e.imoex[i] = nan
	}
	mf, err := os.Open(imoexPath)
	if err == nil {
		defer mf.Close()
		mr := csv.NewReader(mf)
		mr.Read()
		for {
			rec, err := mr.Read()
			if err != nil {
				break
			}
			if i, ok := e.dIdx[strings.TrimSpace(rec[0])]; ok {
				e.imoex[i] = f64(rec[1])
			}
		}
	}
	return e, nil
}

func (e *eqData) ret(t string, d, h int) float64 {
	c := e.closes[t]
	if c == nil || d < 0 || d+h >= len(e.dates) {
		return nan
	}
	for k := d; k <= d+h; k++ {
		if e.divDrop[t][k] || (e.dates[k] >= "2022-02-25" && e.dates[k] <= "2022-03-28") {
			return nan
		}
	}
	a, b := c[d], c[d+h]
	if !(a > 0) || !(b > 0) {
		return nan
	}
	x := b/a - 1
	if x <= -0.45 || x >= 0.80 {
		return nan
	}
	return x
}

func (e *eqData) ewMean(d, h int) float64 {
	s, n := 0.0, 0
	for t := range e.closes {
		if !e.adm[t][d] {
			continue
		}
		if r := e.ret(t, d, h); !isNaN(r) {
			s += r
			n++
		}
	}
	if n < 30 {
		return nan
	}
	return s / float64(n)
}

func (e *eqData) abn(t string, d, h int) float64 {
	r := e.ret(t, d, h)
	m := e.ewMean(d, h)
	if isNaN(r) || isNaN(m) {
		return nan
	}
	return r - m
}

func (e *eqData) sigma60(t string, d int) float64 {
	var v []float64
	for k := d - 59; k <= d; k++ {
		if r := e.ret(t, k-1, 1); !isNaN(r) {
			v = append(v, r)
		}
	}
	if len(v) < 40 {
		return nan
	}
	return stdOf(v)
}

func (e *eqData) sabn(t string, d, h int) float64 {
	a := e.abn(t, d, h)
	s := e.sigma60(t, d)
	if isNaN(a) || isNaN(s) || s <= 0 {
		return nan
	}
	return a / (s * math.Sqrt(float64(h)))
}

func (e *eqData) ctrls(t string, d int) []float64 {
	s := e.sigma60(t, d)
	if isNaN(s) || s <= 0 {
		return nil
	}
	r0, r1 := e.ret(t, d-1, 1), e.ret(t, d-2, 1)
	var v []float64
	for k := d - 5; k <= d-2; k++ {
		if r := e.ret(t, k-1, 1); !isNaN(r) {
			v = append(v, r)
		}
	}
	if isNaN(r0) || isNaN(r1) || len(v) < 3 {
		return nil
	}
	return []float64{clip(r0 / s), clip(r1 / s), clip(mean(v) / s)}
}

func (e *eqData) imoexRet(d, h int) float64 {
	if d < 0 || d+h >= len(e.dates) {
		return nan
	}
	a, b := e.imoex[d], e.imoex[d+h]
	if !(a > 0) || !(b > 0) {
		return nan
	}
	return b/a - 1
}

func (e *eqData) imoexSigma(d int) float64 {
	var v []float64
	for k := d - 59; k <= d; k++ {
		if r := e.imoexRet(k-1, 1); !isNaN(r) {
			v = append(v, r)
		}
	}
	if len(v) < 40 {
		return nan
	}
	return stdOf(v)
}

func (e *eqData) imoexCtrls(d int) []float64 {
	s := e.imoexSigma(d)
	if isNaN(s) || s <= 0 {
		return nil
	}
	r0, r1 := e.imoexRet(d-1, 1), e.imoexRet(d-2, 1)
	var v []float64
	for k := d - 5; k <= d-2; k++ {
		if r := e.imoexRet(k-1, 1); !isNaN(r) {
			v = append(v, r)
		}
	}
	if isNaN(r0) || isNaN(r1) || len(v) < 3 {
		return nil
	}
	return []float64{clip(r0 / s), clip(r1 / s), clip(mean(v) / s)}
}

var stockMap = map[string]string{"SBERF": "SBER", "SBRF": "SBER", "LKOH": "LKOH", "ROSN": "ROSN", "GMKN": "GMKN", "TATN": "TATN", "VTBR": "VTBR", "YDEX": "YDEX", "T": "T", "PIKK": "PIKK", "ALRS": "ALRS", "GAZPF": "GAZP", "NOTK": "NVTK", "SBPR": "SBERP", "VKCO": "VKCO"}

func specs2() []rowSpec {
	var r []rowSpec
	add := func(prefix string, s rowSpec) {
		s.book = "2"
		s.id = prefix + twoDigits(countPrefix(r, prefix)+1)
		r = append(r, s)
	}
	add("I", rowSpec{kind: "eqts", feat: "BI", group: "MIX", hor: 1, sign: 1, desc: "BI of MIX futures -> IMOEX vol-scaled return h1"})
	add("I", rowSpec{kind: "eqts", feat: "BI", group: "MIX", hor: 5, sign: 1, desc: "BI of MIX futures -> IMOEX vol-scaled return h5"})
	add("I", rowSpec{kind: "eqts", feat: "BI", group: "RTS", hor: 1, sign: 1, desc: "BI of RTS futures -> IMOEX vol-scaled return h1"})
	add("I", rowSpec{kind: "eqts", feat: "BI", group: "Si", hor: 1, sign: 1, desc: "BI of Si futures -> IMOEX vol-scaled return h1"})
	add("I", rowSpec{kind: "eqts", feat: "BI", group: "IMOEX", hor: 1, sign: 1, desc: "BI of IMOEX perpetual futures -> IMOEX vol-scaled return h1"})
	add("I", rowSpec{kind: "eqts", feat: "TI", group: "MIX", hor: 1, sign: 1, desc: "TI of MIX futures -> IMOEX vol-scaled return h1"})
	add("I", rowSpec{kind: "eqts", feat: "BI", group: "CNY", hor: 1, sign: 1, desc: "BI of CNY futures -> IMOEX vol-scaled return h1"})
	for _, f := range []string{"BI", "TI", "NOF", "CI"} {
		add("J", rowSpec{kind: "eqfm", feat: f, hor: 1, sign: featSign[f], desc: f + " of single-stock futures -> underlying stock vol-scaled abnormal return h1, FM over stock futures"})
	}
	add("J", rowSpec{kind: "eqfm", feat: "BI", hor: 5, sign: 1, desc: "BI of single-stock futures -> underlying stock vol-scaled abnormal return h5, FM over stock futures"})
	add("K", rowSpec{kind: "eqagg", feat: "BI", group: "BR", groups: []string{"ROSN", "LKOH", "TATN", "NVTK"}, hor: 1, sign: 1, desc: "BI of BR futures -> mean abnormal return of ROSN, LKOH, TATN, NVTK h1"})
	add("K", rowSpec{kind: "eqagg", feat: "BI", group: "GOLD", groups: []string{"PLZL"}, hor: 1, sign: 1, desc: "BI of GOLD futures -> PLZL abnormal return h1"})
	add("K", rowSpec{kind: "eqagg", feat: "BI", group: "NG", groups: []string{"GAZP"}, hor: 1, sign: 1, desc: "BI of NG futures -> GAZP abnormal return h1"})
	add("K", rowSpec{kind: "eqagg", feat: "BI", group: "Si", groups: []string{"GMKN", "CHMF", "PLZL", "ROSN", "LKOH"}, hor: 1, sign: 1, desc: "BI of Si futures -> mean abnormal return of exporters GMKN, CHMF, PLZL, ROSN, LKOH h1"})
	return r
}

func twoDigits(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

func (c *ctx) runEq(e *eqData, sp rowSpec) res {
	s := c.fo
	switch sp.kind {
	case "eqts", "eqagg":
		gi := s.gi(sp.group)
		if gi < 0 {
			return res{t: nan, p: nan}
		}
		var xs, ys []float64
		var cs [][]float64
		var ds []int
		for d := c.lo; d <= c.hi; d++ {
			ed, ok := e.dIdx[s.dates[d]]
			if !ok {
				continue
			}
			x := z60(c.foF[sp.feat][gi], d)
			var y float64 = nan
			var ct []float64
			if sp.kind == "eqts" {
				r := e.imoexRet(ed, sp.hor)
				sg := e.imoexSigma(ed)
				if !isNaN(r) && !isNaN(sg) && sg > 0 {
					y = r / (sg * math.Sqrt(float64(sp.hor)))
				}
				ct = e.imoexCtrls(ed)
			} else {
				var yv []float64
				var cc [][]float64
				for _, t := range sp.groups {
					if v := e.sabn(t, ed, sp.hor); !isNaN(v) {
						yv = append(yv, v)
						if k := e.ctrls(t, ed); k != nil {
							cc = append(cc, k)
						}
					}
				}
				if len(yv) >= 1 && len(cc) >= 1 {
					y = mean(yv)
					ct = make([]float64, 3)
					for j := 0; j < 3; j++ {
						for _, k := range cc {
							ct[j] += k[j]
						}
						ct[j] /= float64(len(cc))
					}
				}
			}
			xs, ys, cs, ds = append(xs, x), append(ys, y), append(cs, ct), append(ds, d)
		}
		return tsRun(xs, ys, cs, ds, c.split(sp), sp.hor)
	case "eqfm":
		var coefs []float64
		var days []int
		for d := c.lo; d <= c.hi; d++ {
			ed, ok := e.dIdx[s.dates[d]]
			if !ok {
				continue
			}
			var fv, ys []float64
			var X [][]float64
			for gi, g := range s.groups {
				t, ok := stockMap[g]
				if !ok {
					continue
				}
				f := c.foF[sp.feat][gi][d]
				y := e.sabn(t, ed, sp.hor)
				ct := e.ctrls(t, ed)
				if isNaN(f) || isNaN(y) || ct == nil {
					continue
				}
				fv, ys, X = append(fv, f), append(ys, y), append(X, ct)
			}
			if len(fv) < 8 {
				continue
			}
			rf := rankScale(fv)
			my := mean(ys)
			M := make([][]float64, len(ys))
			Y := make([]float64, len(ys))
			for k := range ys {
				Y[k] = ys[k] - my
				M[k] = append([]float64{1, rf[k]}, X[k]...)
			}
			b, ok := ols(M, Y)
			if !ok {
				continue
			}
			coefs = append(coefs, b[1])
			days = append(days, d)
		}
		r := res{n: len(coefs)}
		if len(coefs) < 80 {
			r.t, r.p = nan, nan
			return r
		}
		r.hc, r.ht, r.coef, r.t = halfStats(coefs, days, c.split(sp), lagsFor(sp.hor))
		r.p = normP2(r.t)
		return r
	}
	return res{t: nan, p: nan}
}
