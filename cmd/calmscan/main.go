package main

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"os"
	"sort"
	"strconv"
)

var nan = math.NaN()

const (
	split    = "2025-01-03"
	mid      = "2023-01-01"
	lastDate = "2026-10-16"
	first    = "2021-04-01"
	minNames = 50
	minCalm  = 10
	lags     = 10
	frac     = 0.2
)

type obs struct {
	di                   int
	rb1, rb5, f10, u10   float64
	div                  bool
	sd10, sd120, box20   float64
	mom60, mom20, negRb5 float64
}

type cell struct {
	calm, pred                string
	ic, icOther, diff, spread []float64
	dates                     []string
}

func f64(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || s == "" {
		return nan
	}
	return v
}

func mean(v []float64) float64 {
	if len(v) == 0 {
		return nan
	}
	s := 0.0
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}

func neweyWest(x []float64, l int) (float64, float64) {
	n := len(x)
	if n < l+3 {
		return mean(x), nan
	}
	m := mean(x)
	v := 0.0
	for _, a := range x {
		v += (a - m) * (a - m)
	}
	v /= float64(n)
	for k := 1; k <= l; k++ {
		g := 0.0
		for i := k; i < n; i++ {
			g += (x[i] - m) * (x[i-k] - m)
		}
		g /= float64(n)
		v += 2 * (1 - float64(k)/float64(l+1)) * g
	}
	if v <= 0 {
		return m, nan
	}
	return m, m / math.Sqrt(v/float64(n))
}

func ranks(v []float64) []float64 {
	idx := make([]int, len(v))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return v[idx[a]] < v[idx[b]] })
	r := make([]float64, len(v))
	for p, i := range idx {
		r[i] = float64(p)
	}
	return r
}

func pearson(a, b []float64) float64 {
	ma, mb := mean(a), mean(b)
	var sab, saa, sbb float64
	for i := range a {
		sab += (a[i] - ma) * (b[i] - mb)
		saa += (a[i] - ma) * (a[i] - ma)
		sbb += (b[i] - mb) * (b[i] - mb)
	}
	if saa == 0 || sbb == 0 {
		return nan
	}
	return sab / math.Sqrt(saa*sbb)
}

func rankIC(x, y []float64) float64 {
	if len(x) < minCalm {
		return nan
	}
	return pearson(ranks(x), ranks(y))
}

func std(v []float64) float64 {
	m := mean(v)
	s := 0.0
	for _, x := range v {
		s += (x - m) * (x - m)
	}
	return math.Sqrt(s / float64(len(v)-1))
}

func load(path string) ([]string, map[string][]*obs, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	r := csv.NewReader(bufio.NewReaderSize(f, 1<<22))
	hdr, err := r.Read()
	if err != nil {
		return nil, nil, err
	}
	ix := map[string]int{}
	for i, h := range hdr {
		ix[h] = i
	}
	type raw struct {
		date, sec string
		o         *obs
	}
	var all []raw
	dset := map[string]bool{}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		o := &obs{rb1: f64(rec[ix["rb1"]]), rb5: f64(rec[ix["rb5"]]), f10: f64(rec[ix["f10"]]), u10: f64(rec[ix["u10"]]), div: rec[ix["div_drop"]] == "1"}
		all = append(all, raw{rec[ix["date"]], rec[ix["secid"]], o})
		dset[rec[ix["date"]]] = true
	}
	var dates []string
	for d := range dset {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	di := map[string]int{}
	for i, d := range dates {
		di[d] = i
	}
	by := map[string][]*obs{}
	for _, a := range all {
		a.o.di = di[a.date]
		by[a.sec] = append(by[a.sec], a.o)
	}
	for _, s := range by {
		sort.Slice(s, func(i, j int) bool { return s[i].di < s[j].di })
	}
	return dates, by, nil
}

func features(by map[string][]*obs) {
	for _, s := range by {
		start := 0
		for k, o := range s {
			if k > 0 && o.di != s[k-1].di+1 {
				start = k
			}
			o.sd10, o.sd120, o.box20, o.mom60, o.mom20, o.negRb5 = nan, nan, nan, nan, nan, nan
			if k-start+1 < 120 || math.IsNaN(o.rb1) {
				continue
			}
			var w10, w120 []float64
			bad := false
			for j := k - 119; j <= k; j++ {
				if math.IsNaN(s[j].rb1) {
					bad = true
					break
				}
				w120 = append(w120, s[j].rb1)
			}
			if bad {
				continue
			}
			w10 = w120[110:]
			o.sd10, o.sd120 = std(w10), std(w120)
			cum, lo, hi := 0.0, math.Inf(1), math.Inf(-1)
			for _, v := range w120[100:] {
				cum += v
				lo, hi = math.Min(lo, cum), math.Max(hi, cum)
			}
			o.box20 = hi - lo
			o.mom20 = sum(w120[100:])
			o.mom60 = sum(w120[60:])
			if !math.IsNaN(o.rb5) {
				o.negRb5 = -o.rb5
			}
		}
	}
}

func sum(v []float64) float64 {
	s := 0.0
	for _, x := range v {
		s += x
	}
	return s
}

type stat func(*obs) float64

var calmDefs = []struct {
	name string
	f    stat
}{
	{"C1", func(o *obs) float64 { return o.sd10 / o.sd120 }},
	{"C2", func(o *obs) float64 { return o.box20 }},
}

var predDefs = []struct {
	name string
	f    stat
}{
	{"D1", func(o *obs) float64 { return o.mom60 }},
	{"D2", func(o *obs) float64 { return o.mom20 }},
	{"D3", func(o *obs) float64 { return o.negRb5 }},
}

func valid(o *obs) bool {
	return !o.div && !math.IsNaN(o.f10) && !math.IsNaN(o.u10) && !math.IsNaN(o.sd120) && !math.IsNaN(o.box20) &&
		!math.IsNaN(o.mom60) && !math.IsNaN(o.mom20) && !math.IsNaN(o.negRb5) && o.sd120 > 0
}

func split2(rows []*obs, cf stat, perm *rand.Rand) (calm, other []*obs) {
	n := len(rows)
	k := int(float64(n) * frac)
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	if perm != nil {
		perm.Shuffle(n, func(a, b int) { idx[a], idx[b] = idx[b], idx[a] })
	} else {
		sort.Slice(idx, func(a, b int) bool { return cf(rows[idx[a]]) < cf(rows[idx[b]]) })
	}
	for p, i := range idx {
		if p < k {
			calm = append(calm, rows[i])
		} else {
			other = append(other, rows[i])
		}
	}
	return
}

func icOf(g []*obs, pf stat) float64 {
	x := make([]float64, len(g))
	y := make([]float64, len(g))
	for i, o := range g {
		x[i], y[i] = pf(o), o.f10-o.u10
	}
	return rankIC(x, y)
}

func spreadOf(g []*obs, pf stat) float64 {
	if len(g) < minCalm {
		return nan
	}
	s := append([]*obs(nil), g...)
	sort.Slice(s, func(a, b int) bool { return pf(s[a]) < pf(s[b]) })
	t := len(s) / 3
	if t == 0 {
		return nan
	}
	var lo, hi float64
	for _, o := range s[:t] {
		lo += o.f10 - o.u10
	}
	for _, o := range s[len(s)-t:] {
		hi += o.f10 - o.u10
	}
	return (hi - lo) / float64(t)
}

func period(dates []string, v []float64, from, to string) []float64 {
	var o []float64
	for i, d := range dates {
		if d >= from && d < to && !math.IsNaN(v[i]) {
			o = append(o, v[i])
		}
	}
	return o
}

func line(name string, dates []string, v []float64) {
	a := period(dates, v, "0000", split)
	e := period(dates, v, "0000", mid)
	l := period(dates, v, mid, split)
	b := period(dates, v, split, lastDate+"~")
	ma, ta := neweyWest(a, lags)
	mb, tb := neweyWest(b, lags)
	fmt.Printf("%-34s pre-2025 %+.4f (NW t %+.2f, n %d; 2021-22 %+.4f, 2023-24 %+.4f) | 2025-26 %+.4f (NW t %+.2f, n %d)\n", name, ma, ta, len(a), mean(e), mean(l), mb, tb, len(b))
}

func main() {
	path := flag.String("matrix", "matrix.csv", "study matrix")
	flag.Parse()
	dates, by, err := load(*path)
	if err != nil {
		log.Fatal(err)
	}
	features(by)
	byDate := make([][]*obs, len(dates))
	for _, s := range by {
		for _, o := range s {
			if valid(o) {
				byDate[o.di] = append(byDate[o.di], o)
			}
		}
	}
	var cells []*cell
	for _, c := range calmDefs {
		for _, p := range predDefs {
			cells = append(cells, &cell{calm: c.name, pred: p.name})
		}
	}
	placebo := map[string][]float64{}
	var ddates []string
	var magCalm, magOther, magRatio, magRatioO [2][]float64
	var dts []int
	for d, rows := range byDate {
		if len(rows) >= minNames && dates[d] >= first {
			dts = append(dts, d)
		}
	}
	for _, d := range dts {
		ddates = append(ddates, dates[d])
	}
	for ci, c := range calmDefs {
		for pi, p := range predDefs {
			ce := cells[ci*len(predDefs)+pi]
			ce.dates = ddates
			pl := make([][]float64, 5)
			for _, d := range dts {
				rows := byDate[d]
				calm, other := split2(rows, c.f, nil)
				a, b := icOf(calm, p.f), icOf(other, p.f)
				ce.ic = append(ce.ic, a)
				ce.icOther = append(ce.icOther, b)
				ce.diff = append(ce.diff, a-b)
				ce.spread = append(ce.spread, spreadOf(calm, p.f))
				for s := 0; s < 5; s++ {
					rng := rand.New(rand.NewSource(int64(20261026) + int64(s)*1000003 + int64(d)*7919))
					pc, po := split2(rows, c.f, rng)
					pl[s] = append(pl[s], icOf(pc, p.f)-icOf(po, p.f))
				}
				if pi == 0 {
					var cm, om, cr, or []float64
					for _, o := range calm {
						cm = append(cm, math.Abs(o.f10-o.u10))
						cr = append(cr, math.Abs(o.f10-o.u10)/(o.sd120*math.Sqrt(10)))
					}
					for _, o := range other {
						om = append(om, math.Abs(o.f10-o.u10))
						or = append(or, math.Abs(o.f10-o.u10)/(o.sd120*math.Sqrt(10)))
					}
					magCalm[ci] = append(magCalm[ci], mean(cm))
					magOther[ci] = append(magOther[ci], mean(om))
					magRatio[ci] = append(magRatio[ci], mean(cr))
					magRatioO[ci] = append(magRatioO[ci], mean(or))
				}
			}
			for s := 0; s < 5; s++ {
				key := fmt.Sprintf("%s-%s", c.name, p.name)
				placebo[key] = append(placebo[key], mean(period(ddates, pl[s], "0000", split)))
			}
		}
	}
	fmt.Printf("calmscan: %d dates with at least %d names, calm = bottom %.0f%%\n", len(dts), minNames, frac*100)
	fmt.Println("== magnitude (descriptive): mean |10d abnormal return| and the same scaled by sd120*sqrt(10), pre-2025 dates")
	for ci := range calmDefs {
		var cm, om, cr, or []float64
		for i, d := range ddates {
			if d < split {
				cm = append(cm, magCalm[ci][i])
				om = append(om, magOther[ci][i])
				cr = append(cr, magRatio[ci][i])
				or = append(or, magRatioO[ci][i])
			}
		}
		fmt.Printf("%s calm: |y| %.4f vs other %.4f; scaled %.3f vs %.3f (1.0 would be a typical move for the name)\n", calmDefs[ci].name, mean(cm), mean(om), mean(cr), mean(or))
	}
	fmt.Println("== gate cells (IC among calm names; difference to the other 80%; placebo = random 20% subset)")
	pass := 0
	for _, ce := range cells {
		key := ce.calm + "-" + ce.pred
		fmt.Println("-- " + key)
		line("calm IC", ce.dates, ce.ic)
		line("other IC", ce.dates, ce.icOther)
		line("calm minus other", ce.dates, ce.diff)
		line("calm tercile spread (gross 10d)", ce.dates, ce.spread)
		pre := period(ce.dates, ce.ic, "0000", split)
		e := period(ce.dates, ce.ic, "0000", mid)
		l := period(ce.dates, ce.ic, mid, split)
		rep := period(ce.dates, ce.ic, split, lastDate+"~")
		df := period(ce.dates, ce.diff, "0000", split)
		_, t1 := neweyWest(pre, lags)
		m2, t2 := neweyWest(df, lags)
		_, t4 := neweyWest(rep, lags)
		pm := 0.0
		for _, v := range placebo[key] {
			pm = math.Max(pm, math.Abs(v))
		}
		g1, g2, g3, g4, g5 := t1 >= 2.6, t2 >= 2.0, mean(e) > 0 && mean(l) > 0, t4 >= 1.65, pm <= math.Abs(m2)/3
		ok := g1 && g2 && g3 && g4 && g5
		if ok {
			pass++
		}
		fmt.Printf("gate: t>=2.6 %v, diff t>=2.0 %v, both halves %v, replication t>=1.65 %v, placebo<=1/3 %v (max |placebo diff| %.4f vs real %+.4f) -> pass %v\n", g1, g2, g3, g4, g5, pm, m2, ok)
	}
	fmt.Printf("cells passed: %d of %d\n", pass, len(cells))
	fmt.Println("== calm IC by year")
	years := map[string]bool{}
	for _, d := range ddates {
		years[d[:4]] = true
	}
	var ys []string
	for y := range years {
		ys = append(ys, y)
	}
	sort.Strings(ys)
	for _, ce := range cells {
		fmt.Printf("%s-%s:", ce.calm, ce.pred)
		for _, y := range ys {
			fmt.Printf(" %s %+.3f", y, mean(period(ce.dates, ce.ic, y+"-01-01", y+"-12-32")))
		}
		fmt.Println()
	}
}
