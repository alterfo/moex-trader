package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
	"time"
)

type splitEvent struct {
	e         event
	net       float64
	month     string
	gap       float64
	newsAfter float64
	haveNews  bool
}

func netOf(e event) float64 {
	cost := 2 * (0.0005 + 0.5*e.spread/1e4)
	if math.IsNaN(e.spread) {
		cost = 0.003
	}
	return e.run10 - cost
}

func newsDays(path string) map[string][]string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	loc := time.FixedZone("MSK", 3*3600)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	out := map[string][]string{}
	for sc.Scan() {
		var r struct {
			Ticker string `json:"ticker"`
			TS     int64  `json:"published_ts"`
		}
		if json.Unmarshal(sc.Bytes(), &r) != nil || r.Ticker == "" || r.TS == 0 {
			continue
		}
		out[r.Ticker] = append(out[r.Ticker], time.Unix(r.TS, 0).In(loc).Format("2006-01-02"))
	}
	for t := range out {
		sort.Strings(out[t])
	}
	return out
}

func diffGroups(es []splitEvent, grp []int) float64 {
	var a, b []float64
	for i, g := range grp {
		switch g {
		case 1:
			a = append(a, es[i].net)
		case -1:
			b = append(b, es[i].net)
		}
	}
	if len(a) < 5 || len(b) < 5 {
		return nan
	}
	return mean(a) - mean(b)
}

func clusterBoot(es []splitEvent, grp []int, draws int, seed int64, alpha float64) (float64, float64) {
	by := map[string][]int{}
	var months []string
	for i, e := range es {
		if _, ok := by[e.month]; !ok {
			months = append(months, e.month)
		}
		by[e.month] = append(by[e.month], i)
	}
	rng := rand.New(rand.NewSource(seed))
	var ds []float64
	for b := 0; b < draws; b++ {
		var sub []splitEvent
		var sg []int
		for range months {
			for _, i := range by[months[rng.Intn(len(months))]] {
				sub, sg = append(sub, es[i]), append(sg, grp[i])
			}
		}
		if v := diffGroups(sub, sg); !math.IsNaN(v) {
			ds = append(ds, v)
		}
	}
	if len(ds) < 100 {
		return nan, nan
	}
	sort.Float64s(ds)
	return ds[int(alpha/2*float64(len(ds)-1))], ds[int((1-alpha/2)*float64(len(ds)-1))]
}

func splitReport(id string, es []splitEvent, grp []int, alpha float64, seed int64) string {
	point := diffGroups(es, grp)
	if math.IsNaN(point) {
		return fmt.Sprintf("%-62s not testable", id)
	}
	lo, hi := clusterBoot(es, grp, 4000, seed, alpha)
	var e1, e2 []splitEvent
	var g1, g2 []int
	for i, e := range es {
		if e.e.last < "2023-07-01" {
			e1, g1 = append(e1, e), append(g1, grp[i])
		} else {
			e2, g2 = append(e2, e), append(g2, grp[i])
		}
	}
	d1, d2 := diffGroups(e1, g1), diffGroups(e2, g2)
	na, nb := 0, 0
	for _, g := range grp {
		if g == 1 {
			na++
		} else if g == -1 {
			nb++
		}
	}
	hits := 0
	for p := 0; p < 5; p++ {
		pg := append([]int{}, grp...)
		rand.New(rand.NewSource(seed+int64(p)+1)).Shuffle(len(pg), func(i, j int) { pg[i], pg[j] = pg[j], pg[i] })
		plo, phi := clusterBoot(es, pg, 1500, seed+int64(p)+10, alpha)
		if plo > 0 || phi < 0 {
			hits++
		}
	}
	word := "no hit"
	if (lo > 0 || hi < 0) && d1*point > 0 && d2*point > 0 && hits == 0 {
		word = "HIT (lead)"
	}
	return fmt.Sprintf("%-62s diff %+.2f%% [%+.2f%%, %+.2f%%] halves %+.2f%% / %+.2f%% groups %d vs %d placebo hits %d of 5 %s", id, point*100, lo*100, hi*100, d1*100, d2*100, na, nb, hits, word)
}

func eventSplits(events []event, dates []string, byDate map[string]map[string]quote, newsPath string) {
	nd := newsDays(newsPath)
	var es []splitEvent
	for _, e := range events {
		if e.yield < 0.03 || math.IsNaN(e.run10) || e.last >= "2025-01-01" {
			continue
		}
		s := splitEvent{e: e, net: netOf(e), month: e.last[:7], gap: nan}
		p0 := byDate[dates[e.li-11]][e.ticker].close
		c0, c2 := byDate[dates[e.li]][e.ticker].close, byDate[dates[e.li+2]][e.ticker].close
		if p0 > 0 && c0 > 0 && c2 > 0 && e.yield > 0 {
			s.gap = (c0 - c2) / (e.yield * p0)
		}
		if e.ann >= 0 && e.ann <= e.li-11 {
			s.haveNews = true
			n := 0
			for _, d := range nd[e.ticker] {
				if d > dates[e.ann] && d <= dates[e.li-11] {
					n++
				}
			}
			s.newsAfter = float64(n)
		}
		es = append(es, s)
	}
	alpha := 0.05 / 3
	fmt.Printf("divrunup splits: %d events with yield of at least 3%% and last-buy day before 2025, net of spread costs, month-cluster bootstrap, Bonferroni-adjusted level %.4f, halves split 2023-07-01\n", len(es), alpha)
	med := func(v []float64) float64 {
		s := append([]float64{}, v...)
		sort.Float64s(s)
		return s[len(s)/2]
	}
	var sp []float64
	for _, s := range es {
		if !math.IsNaN(s.e.spread) {
			sp = append(sp, s.e.spread)
		}
	}
	g34 := make([]int, len(es))
	sorted := append([]float64{}, sp...)
	sort.Float64s(sorted)
	lo3, hi3 := sorted[len(sorted)/3], sorted[2*len(sorted)/3]
	for i, s := range es {
		switch {
		case math.IsNaN(s.e.spread):
		case s.e.spread >= hi3:
			g34[i] = 1
		case s.e.spread <= lo3:
			g34[i] = -1
		}
	}
	fmt.Println(splitReport("34 liquidity: wide entry spread minus tight", es, g34, alpha, 20261501))
	var gaps []float64
	for _, s := range es {
		if !math.IsNaN(s.gap) {
			gaps = append(gaps, s.gap)
		}
	}
	g38 := make([]int, len(es))
	mg := med(gaps)
	for i, s := range es {
		if math.IsNaN(s.gap) {
			continue
		}
		if s.gap >= mg {
			g38[i] = 1
		} else {
			g38[i] = -1
		}
	}
	fmt.Printf("38 realised ex-date gap over the dividend: median %.2f\n", mg)
	fmt.Println(splitReport("38 gap size: high half minus low half (post-event, not tradable)", es, g38, alpha, 20261502))
	var cnts []float64
	for _, s := range es {
		if s.haveNews {
			cnts = append(cnts, s.newsAfter)
		}
	}
	g39 := make([]int, len(es))
	if len(cnts) >= 10 {
		mc := med(cnts)
		fmt.Printf("39 headlines on the name between the announcement and the entry: median %.0f, verified events %d\n", mc, len(cnts))
		for i, s := range es {
			if !s.haveNews {
				continue
			}
			if s.newsAfter > mc {
				g39[i] = 1
			} else {
				g39[i] = -1
			}
		}
	}
	fmt.Println(splitReport("39 news after the announcement: above-median count minus rest", es, g39, alpha, 20261503))
}
