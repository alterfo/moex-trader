package main

import (
	"bufio"
	"compress/gzip"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const nSlots = 17

var nan = math.NaN()

type dayProfile struct {
	sum [nSlots]float64
	n   [nSlots]int
}

func slotOf(t string) int {
	if len(t) < 5 {
		return -1
	}
	h, _ := strconv.Atoi(t[:2])
	m, _ := strconv.Atoi(t[3:5])
	minutes := h*60 + m
	if minutes < 10*60+5 || minutes > 18*60+30 {
		return -1
	}
	s := (minutes - 10*60 - 5) / 30
	if s < 0 || s >= nSlots {
		return -1
	}
	return s
}

func loadDay(path string, date string, out map[string]*dayProfile) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	r := csv.NewReader(bufio.NewReaderSize(gz, 1<<20))
	hdr, err := r.Read()
	if err != nil {
		return err
	}
	ix := map[string]int{}
	for i, h := range hdr {
		ix[h] = i
	}
	cutoff := ""
	if t, err := time.Parse("2006-01-02", date); err == nil {
		cutoff = t.AddDate(0, 0, 30).Format("2006-01-02")
	}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if st, ok := ix["SYSTIME"]; ok && len(rec[st]) >= 10 && rec[st][:10] > cutoff {
			continue
		}
		s := slotOf(rec[ix["tradetime"]])
		if s < 0 {
			continue
		}
		v, err := strconv.ParseFloat(rec[ix["spread_bbo"]], 64)
		if err != nil || v <= 0 {
			continue
		}
		sec := rec[ix["secid"]]
		p := out[sec]
		if p == nil {
			p = &dayProfile{}
			out[sec] = p
		}
		p.sum[s] += v
		p.n[s]++
	}
	return nil
}

type tick struct {
	dates []string
	prof  []*[nSlots]float64
}

func profile(p *dayProfile) *[nSlots]float64 {
	var out [nSlots]float64
	valid := 0
	for s := 0; s < nSlots; s++ {
		if p.n[s] >= 3 {
			out[s] = p.sum[s] / float64(p.n[s])
			valid++
		} else {
			out[s] = nan
		}
	}
	if valid < 14 {
		return nil
	}
	return &out
}

func median(a []float64) float64 {
	c := append([]float64(nil), a...)
	sort.Float64s(c)
	n := len(c)
	if n%2 == 1 {
		return c[n/2]
	}
	return (c[n/2-1] + c[n/2]) / 2
}

func mean(a []float64) float64 {
	var s float64
	for _, x := range a {
		s += x
	}
	return s / float64(len(a))
}

func rankOf(v []float64, idx []int) []float64 {
	o := append([]int(nil), idx...)
	sort.Slice(o, func(a, b int) bool { return v[o[a]] < v[o[b]] })
	r := make([]float64, len(v))
	for k, i := range o {
		r[i] = float64(k)
	}
	return r
}

func spearman(a, b []float64) float64 {
	var idx []int
	for i := range a {
		if !math.IsNaN(a[i]) && !math.IsNaN(b[i]) {
			idx = append(idx, i)
		}
	}
	if len(idx) < 10 {
		return nan
	}
	ra, rb := rankOf(a, idx), rankOf(b, idx)
	var xs, ys []float64
	for _, i := range idx {
		xs, ys = append(xs, ra[i]), append(ys, rb[i])
	}
	mx, my := mean(xs), mean(ys)
	var sxy, sxx, syy float64
	for i := range xs {
		sxy += (xs[i] - mx) * (ys[i] - my)
		sxx += (xs[i] - mx) * (xs[i] - mx)
		syy += (ys[i] - my) * (ys[i] - my)
	}
	if sxx == 0 || syy == 0 {
		return nan
	}
	return sxy / math.Sqrt(sxx*syy)
}

func nw(x []float64, lags int) (float64, float64) {
	n := len(x)
	m := mean(x)
	if n < lags+3 {
		return m, nan
	}
	g0 := 0.0
	for _, v := range x {
		g0 += (v - m) * (v - m)
	}
	g0 /= float64(n)
	variance := g0
	for l := 1; l <= lags; l++ {
		g := 0.0
		for i := l; i < n; i++ {
			g += (x[i] - m) * (x[i-l] - m)
		}
		g /= float64(n)
		variance += 2 * (1 - float64(l)/float64(lags+1)) * g
	}
	if variance <= 0 {
		return m, nan
	}
	return m, m / math.Sqrt(variance/float64(n))
}

func main() {
	dir := flag.String("dir", "data/algopack/obstats", "obstats directory")
	from := flag.String("from", "2025-01-03", "first day")
	till := flag.String("till", "2026-10-23", "last day")
	flag.Parse()
	files, _ := filepath.Glob(filepath.Join(*dir, "*.csv.gz"))
	sort.Strings(files)
	var dates []string
	ticks := map[string]*tick{}
	for _, f := range files {
		d := strings.TrimSuffix(filepath.Base(f), ".csv.gz")
		if d < *from || d > *till {
			continue
		}
		m := map[string]*dayProfile{}
		if err := loadDay(f, d, m); err != nil {
			log.Fatal(err)
		}
		any := false
		for sec, p := range m {
			pr := profile(p)
			if pr == nil {
				continue
			}
			t := ticks[sec]
			if t == nil {
				t = &tick{}
				ticks[sec] = t
			}
			t.dates = append(t.dates, d)
			t.prof = append(t.prof, pr)
			any = true
		}
		if any {
			dates = append(dates, d)
		}
	}
	analyse(dates, ticks)
}

func analyse(dates []string, ticks map[string]*tick) {
	type st struct {
		sec  string
		med  float64
		days int
	}
	var all []st
	for sec, t := range ticks {
		if len(t.dates) < 300 {
			continue
		}
		var m []float64
		for _, p := range t.prof {
			var v []float64
			for _, x := range p {
				if !math.IsNaN(x) {
					v = append(v, x)
				}
			}
			m = append(m, mean(v))
		}
		all = append(all, st{sec, median(m), len(t.dates)})
	}
	sort.Slice(all, func(a, b int) bool { return all[a].med < all[b].med })
	terc := map[string]int{}
	for i, s := range all {
		terc[s.sec] = i * 3 / len(all)
	}
	fmt.Printf("exectiming: %d days (%s..%s), %d tickers with at least 300 days; median session-mean BBO spread by tercile: ", len(dates), dates[0], dates[len(dates)-1], len(all))
	for k := 0; k < 3; k++ {
		var v []float64
		for _, s := range all {
			if terc[s.sec] == k {
				v = append(v, s.med)
			}
		}
		fmt.Printf("T%d %.1f bp (%d names)  ", k+1, median(v), len(v))
	}
	fmt.Println()
	dayIdx := map[string]int{}
	for i, d := range dates {
		dayIdx[d] = i
	}
	nd := len(dates)
	groups := []string{"all", "T1 (tightest)", "T2", "T3 (widest)"}
	pers := make([][][]float64, 4)
	savBest := make([][][]float64, 4)
	savOpen := make([][][]float64, 4)
	savClose := make([][][]float64, 4)
	ctlBest := make([][][]float64, 4)
	personalMinusControl := make([][][]float64, 4)
	for g := range pers {
		pers[g] = make([][]float64, nd)
		savBest[g] = make([][]float64, nd)
		savOpen[g] = make([][]float64, nd)
		savClose[g] = make([][]float64, nd)
		ctlBest[g] = make([][]float64, nd)
		personalMinusControl[g] = make([][]float64, nd)
	}
	bestSlotCount := make([]int, nSlots)
	var secs []string
	for _, s := range all {
		secs = append(secs, s.sec)
	}
	trail := func(t *tick, i int) *[nSlots]float64 {
		var out [nSlots]float64
		for s := 0; s < nSlots; s++ {
			var v []float64
			for k := i - 60; k < i; k++ {
				if k >= 0 && !math.IsNaN(t.prof[k][s]) {
					v = append(v, t.prof[k][s])
				}
			}
			if len(v) < 30 {
				return nil
			}
			out[s] = median(v)
		}
		return &out
	}
	trails := map[string][]*[nSlots]float64{}
	for _, sec := range secs {
		t := ticks[sec]
		tr := make([]*[nSlots]float64, len(t.dates))
		for i := range t.dates {
			tr[i] = trail(t, i)
		}
		trails[sec] = tr
	}
	argmin := func(p *[nSlots]float64) int {
		b := 0
		for s := 1; s < nSlots; s++ {
			if p[s] < p[b] {
				b = s
			}
		}
		return b
	}
	for ti, sec := range secs {
		t := ticks[sec]
		g := terc[sec] + 1
		oth := ticks[secs[(ti+1)%len(secs)]]
		othTr := trails[secs[(ti+1)%len(secs)]]
		othIdx := map[string]int{}
		for i, d := range oth.dates {
			othIdx[d] = i
		}
		for i, d := range t.dates {
			tr := trails[sec][i]
			if tr == nil {
				continue
			}
			p := t.prof[i]
			var v []float64
			for _, x := range p {
				if !math.IsNaN(x) {
					v = append(v, x)
				}
			}
			sess := mean(v)
			day := dayIdx[d]
			b := argmin(tr)
			bestSlotCount[b]++
			if math.IsNaN(p[b]) || math.IsNaN(p[0]) || math.IsNaN(p[nSlots-1]) {
				continue
			}
			rho := spearman(tr[:], p[:])
			push := func(arr [][][]float64, v float64) {
				if math.IsNaN(v) {
					return
				}
				arr[0][day] = append(arr[0][day], v)
				arr[g][day] = append(arr[g][day], v)
			}
			push(pers, rho)
			push(savBest, (sess-p[b])/2)
			push(savOpen, (p[0]-p[b])/2)
			push(savClose, (p[nSlots-1]-p[b])/2)
			if j, ok := othIdx[d]; ok && othTr[j] != nil {
				cb := argmin(othTr[j])
				if !math.IsNaN(p[cb]) {
					push(ctlBest, (sess-p[cb])/2)
					push(personalMinusControl, (p[cb]-p[b])/2)
				}
			}
		}
	}
	report := func(name string, arr [][][]float64, unit string) {
		fmt.Printf("\n%s\n", name)
		for g := 0; g < 4; g++ {
			var daily []float64
			for _, v := range arr[g] {
				if len(v) >= 5 {
					daily = append(daily, mean(v))
				}
			}
			if len(daily) < 50 {
				fmt.Printf("  %-14s not enough days\n", groups[g])
				continue
			}
			m, t := nw(daily, 5)
			h1 := mean(daily[:len(daily)/2])
			h2 := mean(daily[len(daily)/2:])
			fmt.Printf("  %-14s mean %+.3f %s, NW t %+.1f, halves %+.3f / %+.3f, days %d\n", groups[g], m, unit, t, h1, h2, len(daily))
		}
	}
	report("E1 persistence: Spearman correlation of the trailing 60-day median intraday spread profile with the day's profile", pers, "")
	report("E2 half-spread saved per side (bp) by trading in the predicted-best 30-minute window against the session average", savBest, "bp")
	report("E3 half-spread saved per side (bp) by the predicted-best window against the naive opening window 10:05-10:35", savOpen, "bp")
	report("E4 half-spread saved per side (bp) by the predicted-best window against the naive closing window 18:05-18:30", savClose, "bp")
	report("C1 control: the best window predicted from another ticker's trailing profile, against the session average", ctlBest, "bp")
	report("P personal window minus the control window: half-spread saved per side (bp), E2 minus C1, the product number (personal beats universal)", personalMinusControl, "bp")
	fmt.Printf("\npredicted-best window distribution (30-minute windows from 10:05): ")
	tot := 0
	for _, c := range bestSlotCount {
		tot += c
	}
	for s, c := range bestSlotCount {
		h := 10*60 + 5 + 30*s
		fmt.Printf("%02d:%02d %.0f%%  ", h/60, h%60, 100*float64(c)/float64(tot))
	}
	fmt.Println()
}
