package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

var fundNames = []string{
	"A01", "A02", "A03", "A04", "A05", "A06", "A07", "A08", "A09", "A10", "A11", "A12", "A13", "A14",
	"B01", "B02", "B03", "B04", "B05", "B06", "B07", "B08", "B09", "B10", "B11", "B12", "B13", "B14",
	"C01", "C02", "C03", "C04", "C05", "C06", "C07", "C08", "C09", "C10", "C11", "C12", "C13", "C14", "C15",
	"D01", "D02", "D03", "D04", "D05", "D06", "D07", "D08", "D09",
	"E01", "E02", "E03", "E04", "E05", "E06", "E07", "E08",
	"F01", "F02", "F03", "F04", "F05", "F06", "F07", "F08", "F09", "F10", "F11", "F12", "F13",
	"H01", "H02", "H03", "H04", "H05", "H06",
	"I01", "I02", "I03", "I04", "I05",
	"J01", "J02", "J03", "J04", "J05",
	"K01", "K02", "K03", "K04", "K05",
	"M256",
}

var fundMacro = []string{"brent", "gold", "cnyrub", "usdrub", "cm_hg", "cm_ali", "cm_ng", "cm_ttf", "cm_si", "cm_hrc", "cm_ura", "cm_pl", "btc"}

const (
	holeFrom = "2022-02-25"
	holeTill = "2022-03-28"
)

type fser struct {
	secid                               string
	adm                                 []bool
	r, ab, op, hi, lo, cl, val, a2, lev []float64
}

func finite(v ...float64) bool {
	for _, x := range v {
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return false
		}
	}
	return true
}

func sdOf(v []float64) float64 {
	n := float64(len(v))
	if n < 2 {
		return nan
	}
	m := mean(v)
	s := 0.0
	for _, x := range v {
		s += (x - m) * (x - m)
	}
	return math.Sqrt(s / (n - 1))
}

func skewOf(v []float64) float64 {
	n := float64(len(v))
	if n < 10 {
		return nan
	}
	m := mean(v)
	var s2, s3 float64
	for _, x := range v {
		s2 += (x - m) * (x - m)
		s3 += (x - m) * (x - m) * (x - m)
	}
	s2 /= n
	if s2 <= 0 {
		return nan
	}
	return (s3 / n) / math.Pow(s2, 1.5)
}

func kurtOf(v []float64) float64 {
	n := float64(len(v))
	if n < 10 {
		return nan
	}
	m := mean(v)
	var s2, s4 float64
	for _, x := range v {
		d := (x - m) * (x - m)
		s2 += d
		s4 += d * d
	}
	s2 /= n
	if s2 <= 0 {
		return nan
	}
	return (s4 / n) / (s2 * s2)
}

func betaOf(r, m []float64) float64 {
	mr, mm := mean(r), mean(m)
	var c, v float64
	for i := range r {
		c += (r[i] - mr) * (m[i] - mm)
		v += (m[i] - mm) * (m[i] - mm)
	}
	if v <= 0 {
		return nan
	}
	return c / v
}

func slopeIdx(y []float64) float64 {
	n := float64(len(y))
	if n < 3 {
		return nan
	}
	var sx, sy, sxy, sxx float64
	for i, v := range y {
		x := float64(i)
		sx += x
		sy += v
		sxy += x * v
		sxx += x * x
	}
	den := n*sxx - sx*sx
	if den == 0 {
		return nan
	}
	return (n*sxy - sx*sy) / den
}

func trendT(y []float64) float64 {
	n := len(y)
	if n < 10 {
		return nan
	}
	b := slopeIdx(y)
	my := mean(y)
	mx := float64(n-1) / 2
	var sxx, sse float64
	for i, v := range y {
		x := float64(i)
		sxx += (x - mx) * (x - mx)
		fit := my + b*(x-mx)
		sse += (v - fit) * (v - fit)
	}
	if sxx == 0 {
		return nan
	}
	se := math.Sqrt(sse / float64(n-2) / sxx)
	if se == 0 {
		return nan
	}
	return b / se
}

func nanSlice(n int) []float64 {
	o := make([]float64, n)
	for i := range o {
		o[i] = nan
	}
	return o
}

func readOHLC(path string) ([]string, map[string]*fser, []float64) {
	f, err := os.Open(path)
	if err != nil {
		fatal(err)
	}
	defer f.Close()
	rd := csv.NewReader(bufio.NewReaderSize(f, 1<<22))
	hdr, err := rd.Read()
	if err != nil {
		fatal(err)
	}
	ix := map[string]int{}
	for i, h := range hdr {
		ix[h] = i
	}
	type raw struct {
		date, secid                       string
		rb1, op, hi, lo, cl, val, a2, lev float64
		div                               bool
	}
	var rows []raw
	dset := map[string]bool{}
	for {
		rec, err := rd.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			fatal(err)
		}
		x := raw{date: rec[ix["date"]], secid: rec[ix["secid"]], rb1: f64(rec[ix["rb1"]]), op: f64(rec[ix["OpenDay"]]), hi: f64(rec[ix["High"]]),
			lo: f64(rec[ix["Low"]]), cl: f64(rec[ix["CloseDay"]]), val: f64(rec[ix["ValDay"]]), a2: f64(rec[ix["A2"]]), lev: f64(rec[ix["LevAsym"]]), div: rec[ix["div_drop"]] == "1"}
		rows = append(rows, x)
		dset[x.date] = true
	}
	t0, _ := time.Parse("2006-01-02", holeFrom)
	t1, _ := time.Parse("2006-01-02", holeTill)
	for t := t0; !t.After(t1); t = t.AddDate(0, 0, 1) {
		if wd := t.Weekday(); wd != time.Saturday && wd != time.Sunday {
			dset[t.Format("2006-01-02")] = true
		}
	}
	var dates []string
	for d := range dset {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	idx := map[string]int{}
	for i, d := range dates {
		idx[d] = i
	}
	nd := len(dates)
	mkt := nanSlice(nd)
	sums := make([]float64, nd)
	cnts := make([]int, nd)
	for _, x := range rows {
		if finite(x.rb1) {
			sums[idx[x.date]] += x.rb1
			cnts[idx[x.date]]++
		}
	}
	for d := 0; d < nd; d++ {
		if cnts[d] >= 30 {
			mkt[d] = sums[d] / float64(cnts[d])
		}
	}
	out := map[string]*fser{}
	for _, x := range rows {
		s := out[x.secid]
		if s == nil {
			s = &fser{secid: x.secid, adm: make([]bool, nd), r: nanSlice(nd), ab: nanSlice(nd), op: nanSlice(nd), hi: nanSlice(nd), lo: nanSlice(nd),
				cl: nanSlice(nd), val: nanSlice(nd), a2: nanSlice(nd), lev: nanSlice(nd)}
			out[x.secid] = s
		}
		d := idx[x.date]
		s.adm[d] = true
		s.op[d], s.hi[d], s.lo[d], s.cl[d], s.val[d], s.a2[d], s.lev[d] = x.op, x.hi, x.lo, x.cl, x.val, x.a2, x.lev
		switch {
		case finite(x.rb1, mkt[d]):
			s.r[d], s.ab[d] = x.rb1, x.rb1-mkt[d]
		case x.div:
			s.r[d], s.ab[d] = 0, 0
		}
	}
	return dates, out, mkt
}

func readMacroDeltas(dir string, dates []string) map[string][]float64 {
	out := map[string][]float64{}
	for _, name := range fundMacro {
		f, err := os.Open(filepath.Join(dir, name+".csv"))
		if err != nil {
			fatal(err)
		}
		rd := csv.NewReader(f)
		rd.Read()
		type kv struct {
			d string
			v float64
		}
		var pts []kv
		for {
			rec, err := rd.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				fatal(err)
			}
			v, err := strconv.ParseFloat(rec[1], 64)
			if err != nil || v <= 0 {
				continue
			}
			pts = append(pts, kv{rec[0], v})
		}
		f.Close()
		sort.Slice(pts, func(a, b int) bool { return pts[a].d < pts[b].d })
		dl := nanSlice(len(dates))
		j := -1
		for t := 2; t < len(dates); t++ {
			for j+1 < len(pts) && pts[j+1].d <= dates[t-1] {
				j++
			}
			if j >= 1 && pts[j].d > dates[t-2] && pts[j].v != pts[j-1].v {
				dl[t] = math.Log(pts[j].v / pts[j-1].v)
			}
		}
		out[name] = dl
	}
	return out
}

func window(a []float64, p, n int) []float64 {
	if p < n-1 || p >= len(a) {
		return nil
	}
	return a[p-n+1 : p+1]
}

func fw(a []float64, p, n int) []float64 {
	w := window(a, p, n)
	if w == nil {
		return nil
	}
	for _, x := range w {
		if !finite(x) {
			return nil
		}
	}
	return w
}

func rsum(a []float64, lo, hi int) float64 {
	if lo < 0 || hi >= len(a) || lo > hi {
		return nan
	}
	s := 0.0
	for _, x := range a[lo : hi+1] {
		if !finite(x) {
			return nan
		}
		s += x
	}
	return s
}

func wsum(a []float64, p, n int) float64 {
	return rsum(a, p-n+1, p)
}

func finiteOnly(a []float64) []float64 {
	var o []float64
	for _, x := range a {
		if finite(x) {
			o = append(o, x)
		}
	}
	return o
}

func amihud(r, val []float64, p, n int) float64 {
	w, vw := fw(r, p, n), fw(val, p, n)
	if w == nil || vw == nil {
		return nan
	}
	s := 0.0
	for i := range w {
		if vw[i] <= 0 {
			return nan
		}
		s += math.Abs(w[i]) / vw[i]
	}
	return s / float64(n) * 1e9
}

type frec struct {
	secid string
	di    int
	v     map[string]float64
}

type fcal struct {
	wday     []int
	mday     []int
	tom      []bool
	monthKey []string
}

func fundSeries(s *fser, mkt []float64, cal fcal, mdelta map[string][]float64) []*frec {
	nd := len(s.r)
	lp, ac := nanSlice(nd), nanSlice(nd)
	hiA, loA, rng, clv, lval, flat := nanSlice(nd), nanSlice(nd), nanSlice(nd), nanSlice(nd), nanSlice(nd), nanSlice(nd)
	ovn, idy := nanSlice(nd), nanSlice(nd)
	run := 0.0
	for d := 0; d < nd; d++ {
		if !s.adm[d] {
			continue
		}
		if finite(s.r[d]) {
			run += math.Log1p(s.r[d])
			lp[d], ac[d] = run, math.Exp(run)
		}
		if finite(s.hi[d], s.lo[d], s.cl[d], ac[d]) && s.cl[d] > 0 && s.hi[d] >= s.lo[d] && s.lo[d] > 0 {
			hiA[d] = ac[d] * s.hi[d] / s.cl[d]
			loA[d] = ac[d] * s.lo[d] / s.cl[d]
			rng[d] = (s.hi[d] - s.lo[d]) / s.cl[d]
			if s.hi[d] > s.lo[d] {
				clv[d] = (2*s.cl[d] - s.hi[d] - s.lo[d]) / (s.hi[d] - s.lo[d])
			}
		}
		if finite(s.val[d]) && s.val[d] > 0 {
			lval[d] = math.Log(s.val[d])
		}
		if d > 0 && s.adm[d-1] && finite(s.cl[d], s.cl[d-1]) {
			flat[d] = 0
			if s.cl[d] == s.cl[d-1] {
				flat[d] = 1
			}
		}
		if finite(s.r[d]) && s.r[d] != 0 {
			if d > 0 && s.adm[d-1] && finite(s.op[d], s.cl[d-1]) && s.cl[d-1] > 0 {
				ovn[d] = s.op[d]/s.cl[d-1] - 1
			}
			if finite(s.op[d], s.cl[d]) && s.op[d] > 0 {
				idy[d] = s.cl[d]/s.op[d] - 1
			}
		}
	}
	r, ab, val, cl := s.r, s.ab, s.val, s.cl
	var out []*frec
	for p := 0; p < nd; p++ {
		if !s.adm[p] {
			continue
		}
		v := map[string]float64{}
		for _, nm := range fundNames {
			v[nm] = nan
		}
		v["M256"] = wsum(ab, p, 256)
		v["mom120"] = wsum(ab, p, 120)
		if w := fw(ab, p, 120); w != nil {
			if sd := sdOf(w); sd > 0 {
				v["A09"] = v["mom120"] / sd
			}
			v["A14"] = trendT(window(lp, p, 120))
			if !finite(v["A14"]) {
				v["A14"] = nan
			}
		}
		for _, k := range []struct {
			n  int
			id string
		}{{20, "A01"}, {60, "A02"}, {120, "A03"}, {250, "A04"}} {
			hw, lw := fw(hiA, p, k.n), fw(loA, p, k.n)
			if hw == nil || lw == nil {
				continue
			}
			mx, mn := math.Inf(-1), math.Inf(1)
			for i := range hw {
				mx, mn = math.Max(mx, hw[i]), math.Min(mn, lw[i])
			}
			if mx > mn {
				v[k.id] = (ac[p] - mn) / (mx - mn)
			}
		}
		if w := fw(lp, p, 250); w != nil {
			mx, mn, at := math.Inf(-1), math.Inf(1), 0
			dd, peak := 0.0, math.Inf(-1)
			for i, x := range w {
				if x > mx {
					mx, at = x, i
				}
				mn = math.Min(mn, x)
				peak = math.Max(peak, x)
				dd = math.Min(dd, x-peak)
			}
			v["A05"] = math.Exp(lp[p]-mx) - 1
			v["A06"] = math.Exp(lp[p]-mn) - 1
			v["A13"] = -float64(len(w)-1-at) / 250
			v["C13"] = dd
		}
		if aw := fw(ab, p, 250); aw != nil {
			if sd := sdOf(aw); sd > 0 {
				v["A10"] = wsum(ab, p, 250) / sd
			}
			ma := mean(aw)
			var num, den float64
			for i := 1; i < len(aw); i++ {
				num += (aw[i] - ma) * (aw[i-1] - ma)
				den += (aw[i-1] - ma) * (aw[i-1] - ma)
			}
			if den > 0 {
				v["B13"] = num / den * ab[p]
			}
		}
		if rw, mw := fw(r, p, 250), fw(mkt, p, 250); rw != nil && mw != nil {
			v["C04"] = sdOf(rw)
			if beta := betaOf(rw, mw); finite(beta) {
				v["C01"] = beta
				res := make([]float64, 250)
				var down, downm []float64
				for i := range rw {
					res[i] = rw[i] - beta*mw[i]
					if mw[i] < 0 {
						down, downm = append(down, rw[i]), append(downm, mw[i])
					}
				}
				if len(down) >= 30 {
					v["C02"] = betaOf(down, downm)
				}
				v["C14"] = mean(res)
				v["C05"] = sdOf(res[190:])
				v["C15"] = skewOf(res[130:])
				v["B01"] = rsum(res, 245, 249)
				v["B02"] = res[249]
			}
		}
		if w := fw(lp, p, 100); w != nil {
			v["A07"] = (sumExp(lp, p, 20)/20)/(sumExp(lp, p, 100)/100) - 1
		}
		if fw(lp, p, 200) != nil {
			v["A08"] = (sumExp(lp, p, 50)/50)/(sumExp(lp, p, 200)/200) - 1
		}
		if w := fw(ab, p, 60); w != nil {
			pos, cnt := 0, 0
			sd := sdOf(w)
			for _, x := range w {
				if x > 0 {
					pos++
				}
				if sd > 0 && math.Abs(x) > 3*sd {
					cnt++
				}
			}
			v["A11"] = float64(pos) / 60
			v["I03"] = float64(cnt)
		}
		if w := fw(r, p, 60); w != nil {
			v["C03"] = sdOf(w)
			if sd := sdOf(w); sd > 0 {
				v["I04"] = mean(w) / sd
			}
		}
		if w := fw(ab, p, 130); w != nil {
			pos := 0
			for b := 0; b < 26; b++ {
				if rsum(w, b*5, b*5+4) > 0 {
					pos++
				}
			}
			v["A12"] = float64(pos) / 26
		}
		mv20 := nan
		if w := fw(val, p, 20); w != nil {
			mv20 = mean(w)
		}
		if finite(mv20) && mv20 > 0 && finite(val[p], ab[p]) {
			v["B04"] = ab[p] * val[p] / mv20
		}
		v["B05"] = ovn[p]
		v["B06"] = idy[p]
		if w := fw(ovn, p, 20); w != nil {
			v["B07"] = mean(w)
		}
		if w := fw(idy, p, 20); w != nil {
			v["B08"] = mean(w)
		}
		if w := fw(clv, p, 5); w != nil {
			v["B09"] = mean(w)
		}
		v["B10"] = wsum(ab, p, 2)
		v["B11"] = wsum(ab, p, 10)
		if finite(rng[p], ab[p]) {
			v["B12"] = rng[p] * math.Copysign(1, ab[p])
		}
		v["B14"] = clv[p]
		v["ab1"], v["ab5"], v["ab20"] = ab[p], wsum(ab, p, 5), wsum(ab, p, 20)
		v["a2"], v["lev"] = s.a2[p], s.lev[p]
		if w := fw(r, p, 20); w != nil {
			mx, mn := math.Inf(-1), math.Inf(1)
			for _, x := range w {
				mx, mn = math.Max(mx, x), math.Min(mn, x)
			}
			v["C06"], v["C07"] = mx, mn
		}
		if w := fw(r, p, 120); w != nil {
			v["C08"], v["C09"] = skewOf(w), kurtOf(w)
			ds := 0.0
			for _, x := range w {
				if x < 0 {
					ds += x * x
				}
			}
			v["C12"] = math.Sqrt(ds / 120)
		}
		if w := fw(r, p, 100); w != nil {
			var bs []float64
			for b := 0; b < 10; b++ {
				bs = append(bs, sdOf(w[b*10:b*10+10]))
			}
			v["C10"] = sdOf(bs)
		}
		if hw, lw := fw(s.hi, p, 60), fw(s.lo, p, 60); hw != nil && lw != nil {
			sq := 0.0
			ok := true
			for i := range hw {
				if lw[i] <= 0 || hw[i] < lw[i] {
					ok = false
					break
				}
				l := math.Log(hw[i] / lw[i])
				sq += l * l
			}
			if ok {
				v["C11"] = math.Sqrt(sq / 60 / (4 * math.Ln2))
			}
		}
		v["D01"] = amihud(r, val, p, 20)
		v["D05"] = amihud(r, val, p, 120)
		if vw := fw(val, p, 60); vw != nil {
			if mv60 := mean(vw); mv60 > 0 {
				v["D02"] = math.Log(mv60)
				v["D09"] = sdOf(vw) / mv60
				if rw := fw(r, p, 20); rw != nil {
					t := 0.0
					for i := 0; i < 20; i++ {
						if vw[40+i] <= 0 {
							t = nan
							break
						}
						t += math.Copysign(1, rw[i]) * math.Log(vw[40+i]/mv60)
					}
					v["D06"] = t
				}
			}
		}
		if w := fw(val, p, 120); w != nil && finite(mv20) && mean(w) > 0 {
			v["D03"] = mv20 / mean(w)
		}
		if w := fw(lval, p, 60); w != nil {
			v["D04"] = slopeIdx(w)
		}
		if finite(cl[p]) && cl[p] > 0 {
			v["D07"] = math.Log(cl[p])
		}
		if w := fw(flat, p, 60); w != nil {
			v["D08"] = mean(w)
		}
		v["E01"] = rsum(ab, p-249, p-240)
		v["E02"] = rsum(ab, p-499, p-490)
		if finite(v["E01"], v["E02"]) {
			v["E03"] = (v["E01"] + v["E02"]) / 2
		}
		v["E07"] = rsum(ab, p-260, p-240)
		v["E08"] = rsum(ab, p-521, p-501)
		if nx := p + 1; nx < nd && p >= 259 {
			var wd, md, tomIn, tomOut []float64
			for i := p - 259; i <= p; i++ {
				if !finite(ab[i]) {
					continue
				}
				if cal.wday[i] == cal.wday[nx] {
					wd = append(wd, ab[i])
				}
				if cal.mday[i] == cal.mday[nx] {
					md = append(md, ab[i])
				}
				if i >= p-249 {
					if cal.tom[i] {
						tomIn = append(tomIn, ab[i])
					} else {
						tomOut = append(tomOut, ab[i])
					}
				}
			}
			if len(wd) >= 40 {
				v["E04"] = mean(wd)
			}
			if len(md) >= 40 {
				v["E05"] = mean(md)
			}
			if p+10 < nd && cal.monthKey[p+10] != cal.monthKey[p] && len(tomIn) >= 10 && len(tomOut) >= 100 {
				v["E06"] = mean(tomIn) - mean(tomOut)
			}
		}
		if p >= 260 {
			for j, nm := range fundMacro {
				dx := mdelta[nm]
				var xs, ys []float64
				for t := p - 250; t < p; t++ {
					if finite(dx[t], ab[t]) {
						xs, ys = append(xs, dx[t]), append(ys, ab[t])
					}
				}
				if len(xs) < 150 {
					continue
				}
				sd := sdOf(xs)
				if sd <= 0 {
					continue
				}
				beta := betaOf(ys, xs)
				sum, cnt, oldest := 0.0, 0, p
				for t := p; t >= p-30 && cnt < 5; t-- {
					if finite(dx[t]) {
						sum += dx[t]
						cnt++
						oldest = t
					}
				}
				if cnt == 5 && finite(beta) && oldest >= p-30 {
					v[fmt.Sprintf("F%02d", j+1)] = beta * sum / (sd * math.Sqrt(5))
				}
			}
		}
		if finite(rng[p]) {
			if w := fw(rng, p-1, 20); w != nil && mean(w) > 0 {
				v["I02"] = rng[p] / mean(w)
			}
		}
		if w, vw := fw(cl, p, 20), fw(val, p, 20); w != nil && vw != nil {
			sv, sc := 0.0, 0.0
			for i := range w {
				sv += vw[i]
				sc += w[i] * vw[i]
			}
			if sv > 0 && sc > 0 {
				v["I05"] = cl[p]/(sc/sv) - 1
			}
		}
		out = append(out, &frec{secid: s.secid, di: p, v: v})
	}
	return out
}

func sumExp(lp []float64, p, n int) float64 {
	s := 0.0
	for _, x := range lp[p-n+1 : p+1] {
		s += math.Exp(x)
	}
	return s
}

type fcomp struct {
	name string
	sign float64
}

func rankPart(recs []*frec, comps []fcomp) map[*frec]float64 {
	var ok []*frec
	for _, x := range recs {
		good := true
		for _, c := range comps {
			if !finite(x.v[c.name]) {
				good = false
				break
			}
		}
		if good {
			ok = append(ok, x)
		}
	}
	res := map[*frec]float64{}
	if len(ok) < 30 {
		return res
	}
	acc := make([]float64, len(ok))
	for _, c := range comps {
		col := make([]float64, len(ok))
		for i, x := range ok {
			col[i] = x.v[c.name]
		}
		rk := rankScale(col)
		for i := range ok {
			acc[i] += c.sign * rk[i] / float64(len(comps))
		}
	}
	for i, x := range ok {
		res[x] = acc[i]
	}
	return res
}

func product(recs []*frec, a, b string) map[*frec]float64 {
	var ok []*frec
	for _, x := range recs {
		if finite(x.v[a], x.v[b]) {
			ok = append(ok, x)
		}
	}
	res := map[*frec]float64{}
	if len(ok) < 30 {
		return res
	}
	ca, cb := make([]float64, len(ok)), make([]float64, len(ok))
	for i, x := range ok {
		ca[i], cb[i] = x.v[a], x.v[b]
	}
	ra, rb := rankScale(ca), rankScale(cb)
	for i, x := range ok {
		res[x] = ra[i] * rb[i]
	}
	return res
}

func fundGen(matrixPath, macroDir, outPath string) {
	dates, series, mkt := readOHLC(matrixPath)
	mdelta := readMacroDeltas(macroDir, dates)
	nd := len(dates)
	cal := fcal{wday: make([]int, nd), mday: make([]int, nd), tom: make([]bool, nd), monthKey: make([]string, nd)}
	seen := map[string]int{}
	for i, d := range dates {
		t, _ := time.Parse("2006-01-02", d)
		cal.wday[i] = int(t.Weekday())
		cal.mday[i] = (t.Day() - 1) / 7
		cal.monthKey[i] = d[:7]
		seen[cal.monthKey[i]]++
		cal.tom[i] = seen[cal.monthKey[i]] <= 3
	}
	var secids []string
	for s := range series {
		secids = append(secids, s)
	}
	sort.Strings(secids)
	peers := map[int]map[string][]string{}
	turnover, turnN := 0.0, 0
	for d := 260; d < nd; d += 20 {
		if d%20 != 0 {
			continue
		}
		lists := map[string][]string{}
		for _, a := range secids {
			if !series[a].adm[d] {
				continue
			}
			type pc struct {
				s string
				c float64
			}
			var cs []pc
			for _, b := range secids {
				if a == b || !series[b].adm[d] {
					continue
				}
				var xa, xb []float64
				for k := d - 249; k <= d; k++ {
					if finite(series[a].ab[k], series[b].ab[k]) {
						xa, xb = append(xa, series[a].ab[k]), append(xb, series[b].ab[k])
					}
				}
				if len(xa) < 150 {
					continue
				}
				if c := pearson(xa, xb); finite(c) {
					cs = append(cs, pc{b, c})
				}
			}
			sort.Slice(cs, func(i, j int) bool {
				if cs[i].c != cs[j].c {
					return cs[i].c > cs[j].c
				}
				return cs[i].s < cs[j].s
			})
			for k := 0; k < 5 && k < len(cs); k++ {
				lists[a] = append(lists[a], cs[k].s)
			}
			if prev, ok := peers[d-20]; ok && len(prev[a]) == 5 && len(lists[a]) == 5 {
				keep := 0
				for _, x := range lists[a] {
					for _, y := range prev[a] {
						if x == y {
							keep++
						}
					}
				}
				turnover += 1 - float64(keep)/5
				turnN++
			}
		}
		peers[d] = lists
	}
	if turnN > 0 {
		fmt.Fprintf(os.Stderr, "fundgen: peer sets refreshed every 20 dates, mean share of the five peers replaced at a refresh %.3f\n", turnover/float64(turnN))
	}
	peerOf := func(d int) map[string][]string { return peerLookup(peers, d) }
	sumAb := func(s string, d, n int) float64 {
		return wsum(series[s].ab, d, n)
	}
	byDate := make([][]*frec, nd)
	for _, sid := range secids {
		for _, x := range fundSeries(series[sid], mkt, cal, mdelta) {
			byDate[x.di] = append(byDate[x.di], x)
		}
	}
	f, err := os.Create(outPath)
	if err != nil {
		fatal(err)
	}
	defer f.Close()
	w := bufio.NewWriterSize(f, 1<<22)
	defer w.Flush()
	fmt.Fprintf(w, "date,secid,%s\n", strings.Join(fundNames, ","))
	sectorSeen, sectorTot := 0, 0
	for d := 0; d < nd; d++ {
		recs := byDate[d]
		if len(recs) == 0 {
			continue
		}
		pl := peerOf(d)
		sectorMembers := map[string][]*frec{}
		for _, x := range recs {
			sectorTot++
			if sc := sectorOf[x.secid]; sc != "" {
				sectorMembers[sc] = append(sectorMembers[sc], x)
				sectorSeen++
			}
		}
		for _, x := range recs {
			if ps := pl[x.secid]; len(ps) == 5 {
				var p1, p5, p20 []float64
				for _, q := range ps {
					p1 = append(p1, series[q].ab[d])
					p5 = append(p5, sumAb(q, d, 5))
					p20 = append(p20, sumAb(q, d, 20))
				}
				if f1 := finiteOnly(p1); len(f1) >= 4 {
					x.v["H01"] = mean(f1)
				}
				if f5 := finiteOnly(p5); len(f5) >= 4 {
					x.v["H02"] = mean(f5)
					x.v["H03"] = x.v["ab5"] - x.v["H02"]
				}
				if f20 := finiteOnly(p20); len(f20) >= 4 {
					x.v["H04"] = x.v["ab20"] - mean(f20)
				}
			}
			if sc := sectorOf[x.secid]; sc != "" && len(sectorMembers[sc]) >= 5 {
				var o1, o5 []float64
				for _, q := range sectorMembers[sc] {
					if q != x {
						o1, o5 = append(o1, q.v["ab1"]), append(o5, q.v["ab5"])
					}
				}
				if a, b := finiteOnly(o1), finiteOnly(o5); len(a) >= 4 && len(b) >= 4 {
					x.v["H05"] = mean(a)
					x.v["H06"] = mean(b)
					x.v["B03"] = x.v["ab5"] - mean(b)
				}
			}
		}
		assign := func(name string, m map[*frec]float64) {
			for x, val := range m {
				x.v[name] = val
			}
		}
		assign("I01", rankPart(recs, []fcomp{{"C06", 1}, {"C08", 1}, {"D07", -1}}))
		assign("J01", product(recs, "B01", "a2"))
		assign("J02", product(recs, "A05", "a2"))
		assign("J03", product(recs, "C04", "a2"))
		assign("J04", product(recs, "mom120", "a2"))
		assign("J05", product(recs, "B04", "lev"))
		assign("K01", rankPart(recs, []fcomp{{"A03", 1}, {"A05", 1}, {"A08", 1}, {"A10", 1}}))
		assign("K02", rankPart(recs, []fcomp{{"C04", -1}, {"C01", -1}, {"C06", -1}, {"C05", -1}}))
		assign("K03", rankPart(recs, []fcomp{{"B01", -1}, {"B03", -1}, {"B10", -1}}))
		assign("K04", rankPart(recs, []fcomp{{"A05", 1}, {"A10", 1}, {"C04", -1}, {"D02", 1}}))
		assign("K05", rankPart(recs, []fcomp{{"A03", 1}, {"A05", 1}, {"A08", 1}, {"A10", 1}, {"C04", -1}, {"C01", -1}, {"C06", -1}, {"C05", -1}, {"B01", -1}, {"B03", -1}, {"B10", -1}}))
		for _, x := range recs {
			fmt.Fprintf(w, "%s,%s", dates[d], x.secid)
			for _, nm := range fundNames {
				if val := x.v[nm]; finite(val) {
					fmt.Fprintf(w, ",%g", val)
				} else {
					fmt.Fprint(w, ",")
				}
			}
			fmt.Fprintln(w)
		}
	}
	fmt.Fprintf(os.Stderr, "fundgen: sector map covers %.1f%% of universe rows; sector-based columns need a bucket of at least five names on the date\n", 100*float64(sectorSeen)/float64(sectorTot))
}

func peerLookup(peers map[int]map[string][]string, d int) map[string][]string {
	for k := d - d%20; k >= 260; k -= 20 {
		if p, ok := peers[k]; ok {
			return p
		}
	}
	return nil
}
