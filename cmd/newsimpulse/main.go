package main

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"time"
)

var nan = math.NaN()

type cell struct {
	f1, f5, f10, u1, u5, u10, a2, idy1 float64
	secid                              string
}

type agg struct {
	cnt, tw, sent, eveCnt, eveSent float64
	lc, ls, elc, els               float64
}

func f64(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nan
	}
	return v
}

func main() {
	matrix := flag.String("matrix", "work/matrix.csv", "flowscan matrix")
	ext := flag.String("ext", "work/matrix.csv.ext", "matrix side file with idy1")
	newsPath := flag.String("news", "data/news_history.jsonl", "news history")
	llmPath := flag.String("llm", "", "newsllm scores.csv; runs the L rows instead of the N rows")
	dumpPath := flag.String("dump", "", "write the count-shock variants by date and name to this CSV and exit")
	flag.Parse()
	dates, byDate := loadMatrix(*matrix, *ext)
	items, err := loadNews(*newsPath)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("newsimpulse: %d matrix dates, %d news items\n", len(dates), len(items))
	if *dumpPath != "" {
		if err := dumpShocks(dates, byDate, items, *dumpPath); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *llmPath != "" {
		if err := attachLLM(items, *llmPath); err != nil {
			log.Fatal(err)
		}
		signals := buildSignals(dates, byDate, items)
		runLLMRows(dates, byDate, signals)
		return
	}
	signals := buildSignals(dates, byDate, items)
	runRows(dates, byDate, signals)
}

func loadMatrix(path, extPath string) ([]string, map[string][]cell) {
	idy := map[string]float64{}
	if fe, err := os.Open(extPath); err == nil {
		r := csv.NewReader(bufio.NewReaderSize(fe, 1<<22))
		hdr, _ := r.Read()
		ix := map[string]int{}
		for i, h := range hdr {
			ix[h] = i
		}
		for {
			rec, err := r.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Fatal(err)
			}
			idy[rec[ix["date"]]+"|"+rec[ix["secid"]]] = f64(rec[ix["idy1"]])
		}
		fe.Close()
	}
	f, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	r := csv.NewReader(bufio.NewReaderSize(f, 1<<22))
	hdr, _ := r.Read()
	ix := map[string]int{}
	for i, h := range hdr {
		ix[h] = i
	}
	by := map[string][]cell{}
	seen := map[string]bool{}
	var dates []string
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		d := rec[ix["date"]]
		if !seen[d] {
			seen[d] = true
			dates = append(dates, d)
		}
		c := cell{secid: rec[ix["secid"]], f1: f64(rec[ix["f1"]]), f5: f64(rec[ix["f5"]]), f10: f64(rec[ix["f10"]]), u1: f64(rec[ix["u1"]]), u5: f64(rec[ix["u5"]]), u10: f64(rec[ix["u10"]]), a2: f64(rec[ix["A2"]]), idy1: nan}
		if v, ok := idy[d+"|"+c.secid]; ok {
			c.idy1 = v
		}
		if rec[ix["div_drop"]] == "1" {
			c.f1, c.f5, c.f10, c.idy1 = nan, nan, nan, nan
		}
		by[d] = append(by[d], c)
	}
	sort.Strings(dates)
	return dates, by
}

func cutoff(d string) time.Time {
	t, _ := time.ParseInLocation("2006-01-02 15:04", d+" 18:30", time.FixedZone("MSK", 3*3600))
	return t
}

func buildSignals(dates []string, byDate map[string][]cell, items []item) map[string]map[string]*agg {
	cuts := make([]time.Time, len(dates))
	for i, d := range dates {
		cuts[i] = cutoff(d)
	}
	out := map[string]map[string]*agg{}
	get := func(d, sec string) *agg {
		if out[d] == nil {
			out[d] = map[string]*agg{}
		}
		a := out[d][sec]
		if a == nil {
			a = &agg{}
			out[d][sec] = a
		}
		return a
	}
	for _, it := range items {
		i := sort.Search(len(cuts), func(k int) bool { return !cuts[k].Before(it.ts) })
		if i >= len(dates) {
			continue
		}
		a := get(dates[i], it.ticker)
		a.cnt++
		a.tw += it.trust
		a.sent += it.trust * it.score
		if it.hasLLM {
			a.lc++
			a.ls += it.trust * it.llm
		}
		if i > 0 && it.ts.After(cuts[i-1]) && it.ts.Before(cuts[i-1].Add(15*time.Hour+20*time.Minute)) {
			e := get(dates[i-1], it.ticker)
			e.eveCnt++
			e.eveSent += it.trust * it.score
			if it.hasLLM {
				e.elc++
				e.els += it.trust * it.llm
			}
		}
	}
	return out
}

func rankAvg(v []float64) []float64 {
	n := len(v)
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return v[idx[a]] < v[idx[b]] })
	r := make([]float64, n)
	for i := 0; i < n; {
		j := i
		for j+1 < n && v[idx[j+1]] == v[idx[i]] {
			j++
		}
		for k := i; k <= j; k++ {
			r[idx[k]] = float64(i+j) / 2
		}
		i = j + 1
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

func mean(a []float64) float64 {
	var s float64
	for _, x := range a {
		s += x
	}
	return s / float64(len(a))
}

func neweyWest(x []float64, lags int) (float64, float64) {
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
