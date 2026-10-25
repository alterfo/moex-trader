package main

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
)

type outcome struct {
	get func(c cell) float64
	h   int
}

type rowDef struct {
	id      string
	sigName string
	out     outcome
	from    string
	perm    int64
}

type res struct {
	id        string
	mean, t   float64
	pre, post float64
	isControl bool
	sigName   string
}

func trailingZ(vals []float64, i int) float64 {
	if i < 20 {
		return nan
	}
	m := mean(vals[i-20 : i])
	return (vals[i] - m) / math.Sqrt(m+1)
}

func defineRows() []rowDef {
	absH := map[int]func(c cell) float64{
		1:  func(c cell) float64 { return c.f1 - c.u1 },
		5:  func(c cell) float64 { return c.f5 - c.u5 },
		10: func(c cell) float64 { return c.f10 - c.u10 },
	}
	var rows []rowDef
	add := func(id, s string, h int, from string) {
		rows = append(rows, rowDef{id: fmt.Sprintf("%s h%d", id, h), sigName: s, out: outcome{get: absH[h], h: h}, from: from})
	}
	for _, h := range []int{1, 5, 10} {
		add("N01 count shock", "zc", h, "2022-01-03")
		add("N02 trust-weighted count shock", "zt", h, "2022-01-03")
		add("N03 lexicon sentiment", "sent", h, "2022-01-03")
		add("N04 sentiment x A2", "sentA2", h, "2025-01-03")
		add("N05 count shock x A2", "zcA2", h, "2025-01-03")
	}
	idy := outcome{get: func(c cell) float64 { return c.idy1 }, h: 1}
	rows = append(rows, rowDef{id: "N06 evening sentiment -> next open-to-close", sigName: "eveSent", out: idy, from: "2022-01-03"})
	rows = append(rows, rowDef{id: "N07 evening count shock -> next open-to-close", sigName: "eveZ", out: idy, from: "2022-01-03"})
	for k := int64(0); k < 5; k++ {
		rows = append(rows, rowDef{id: fmt.Sprintf("C%02d permuted count shock h1", k+1), sigName: "zc", out: outcome{get: absH[1], h: 1}, from: "2022-01-03", perm: 20261025 + k})
	}
	return rows
}

func runRows(dates []string, byDate map[string][]cell, signals map[string]map[string]*agg) {
	di := map[string]int{}
	for i, d := range dates {
		di[d] = i
	}
	nd := len(dates)
	secSet := map[string]bool{}
	for _, d := range dates {
		for _, c := range byDate[d] {
			secSet[c.secid] = true
		}
	}
	cnt := map[string][]float64{}
	tw := map[string][]float64{}
	eve := map[string][]float64{}
	for s := range secSet {
		cnt[s], tw[s], eve[s] = make([]float64, nd), make([]float64, nd), make([]float64, nd)
	}
	for d, m := range signals {
		i, ok := di[d]
		if !ok {
			continue
		}
		for s, a := range m {
			if _, ok := cnt[s]; ok {
				cnt[s][i], tw[s][i], eve[s][i] = a.cnt, a.tw, a.eveCnt
			}
		}
	}
	rows := defineRows()
	split := "2025-01-03"
	var results []res
	for _, r := range rows {
		var daily []float64
		var labels []string
		needA2 := r.sigName == "sentA2" || r.sigName == "zcA2"
		for i, d := range dates {
			if d < r.from {
				continue
			}
			var sv, ov, av []float64
			for _, c := range byDate[d] {
				a := signals[d][c.secid]
				if a == nil {
					a = &agg{}
				}
				var s float64
				switch r.sigName {
				case "zc", "zcA2":
					s = trailingZ(cnt[c.secid], i)
				case "zt":
					s = trailingZ(tw[c.secid], i)
				case "sent", "sentA2":
					s = a.sent / (a.cnt + 1)
				case "eveSent":
					s = a.eveSent / (a.eveCnt + 1)
				case "eveZ":
					s = trailingZ(eve[c.secid], i)
				}
				o := r.out.get(c)
				if math.IsNaN(s) || math.IsNaN(o) || (needA2 && math.IsNaN(c.a2)) {
					continue
				}
				sv, ov, av = append(sv, s), append(ov, o), append(av, c.a2)
			}
			if len(sv) < 30 {
				continue
			}
			if needA2 {
				rs, ra := rankAvg(sv), rankAvg(av)
				n := float64(len(sv))
				for j := range sv {
					sv[j] = (rs[j]/n - 0.5) * (ra[j]/n - 0.5)
				}
			}
			if r.perm != 0 {
				rng := rand.New(rand.NewSource(r.perm*1000003 + int64(i)))
				rng.Shuffle(len(sv), func(a, b int) { sv[a], sv[b] = sv[b], sv[a] })
			}
			ic := pearson(rankAvg(sv), rankAvg(ov))
			if !math.IsNaN(ic) {
				daily = append(daily, ic)
				labels = append(labels, d)
			}
		}
		if len(daily) < 100 {
			fmt.Printf("%-52s not testable (%d dates)\n", r.id, len(daily))
			continue
		}
		m, t := neweyWest(daily, r.out.h+5)
		cut := split
		if r.from >= split {
			cut = "2026-01-05"
		}
		var pre, post []float64
		for i, d := range labels {
			if d < cut {
				pre = append(pre, daily[i])
			} else {
				post = append(post, daily[i])
			}
		}
		pm, qm := nan, nan
		if len(pre) > 0 {
			pm = mean(pre)
		}
		if len(post) > 0 {
			qm = mean(post)
		}
		results = append(results, res{id: r.id, mean: m, t: t, pre: pm, post: qm, isControl: r.perm != 0, sigName: r.sigName})
		fmt.Printf("%-52s dates %4d  mean IC %+.4f  NW t %+6.2f  first half %+.4f  second half %+.4f\n", r.id, len(daily), m, t, pm, qm)
	}
	const T = 3.0
	hits, ctl, real := 0, 0, 0
	var ids []string
	for _, r := range results {
		if !r.isControl {
			real++
		}
		if math.Abs(r.t) >= T && r.pre*r.post > 0 && r.pre*r.mean > 0 {
			if r.isControl {
				ctl++
			} else {
				hits++
				ids = append(ids, r.id)
			}
		}
	}
	sort.Strings(ids)
	fmt.Printf("\nthreshold |t| >= %.1f (the project floor); hits: %d of %d rows; control hits: %d of 5; %v\n", T, hits, real, ctl, ids)
}
