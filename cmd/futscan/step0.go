package main

import (
	"fmt"
	"math"
	"sort"
)

func ksDist(a, b []float64) float64 {
	sort.Float64s(a)
	sort.Float64s(b)
	i, j := 0, 0
	var d float64
	for i < len(a) && j < len(b) {
		x := math.Min(a[i], b[j])
		for i < len(a) && a[i] <= x {
			i++
		}
		for j < len(b) && b[j] <= x {
			j++
		}
		d = math.Max(d, math.Abs(float64(i)/float64(len(a))-float64(j)/float64(len(b))))
	}
	return d
}

func quant(v []float64, q float64) float64 {
	w := append([]float64(nil), v...)
	sort.Float64s(w)
	return w[int(q*float64(len(w)-1))]
}

func step0(c *ctx) {
	for _, m := range []struct {
		name string
		s    *store
		F    featMat
	}{{"fo", c.fo, c.foF}, {"fx", c.fx, c.fxF}} {
		s := m.s
		fmt.Printf("== %s: %d dates %s..%s, %d groups\n", m.name, len(s.dates), s.dates[0], s.dates[len(s.dates)-1], len(s.groups))
		years := map[string]int{}
		lateY := map[string][2]int{}
		for _, g := range s.groups {
			for d, x := range s.byGroup[g] {
				if x != nil {
					y := s.dates[d][:4]
					years[y]++
					l := lateY[y]
					l[0]++
					if x.lateShare > 0 {
						l[1]++
					}
					lateY[y] = l
				}
			}
		}
		var ys []string
		for y := range years {
			ys = append(ys, y)
		}
		sort.Strings(ys)
		for _, y := range ys {
			fmt.Printf("  %s: group-days %d, with late SYSTIME share>0: %d\n", y, years[y], lateY[y][1])
		}
		var absr []float64
		var rolls, fronts int
		for _, g := range s.groups {
			for d := 1; d < len(s.dates); d++ {
				if r := s.ret1Back(g, d); !isNaN(r) {
					absr = append(absr, math.Abs(r))
				}
				if s.byGroup[g][d] != nil && s.byGroup[g][d-1] != nil {
					fronts++
					if s.byGroup[g][d].secid != s.byGroup[g][d-1].secid {
						rolls++
					}
				}
			}
		}
		fmt.Printf("  one-day |return| quantiles: p50 %.4f p90 %.4f p99 %.4f p99.9 %.4f max %.4f; front changes %d of %d group-days\n", quant(absr, .5), quant(absr, .9), quant(absr, .99), quant(absr, .999), quant(absr, 1), rolls, fronts)
		for _, y := range []string{"2023", "2024", "2025", "2026"} {
			n := 0
			seen := map[string]bool{}
			for _, g := range s.groups {
				cnt := 0
				for d, x := range s.byGroup[g] {
					if x != nil && s.dates[d][:4] == y {
						cnt++
					}
				}
				if cnt >= 120 {
					n++
					seen[g] = true
				}
			}
			fmt.Printf("  groups with >=120 usable days in %s: %d\n", y, n)
		}
		fmt.Printf("  features KS 2024-07-01..2024-12-31 vs 2025-01-03..2025-06-30 (pooled over groups):\n")
		for _, n := range featNames {
			var a, b []float64
			for gi := range s.groups {
				for d, v := range m.F[n][gi] {
					if isNaN(v) {
						continue
					}
					ds := s.dates[d]
					if ds >= "2024-07-01" && ds <= "2024-12-31" {
						a = append(a, v)
					} else if ds >= "2025-01-03" && ds <= "2025-06-30" {
						b = append(b, v)
					}
				}
			}
			if len(a) > 30 && len(b) > 30 {
				fmt.Printf("    %-4s n %d / %d distance %.4f\n", n, len(a), len(b), ksDist(a, b))
			}
		}
		var cs, ct []float64
		for _, g := range s.groups {
			var prev *rec
			for d := 0; d < len(s.dates); d++ {
				x := s.byGroup[g][d]
				if x != nil && prev != nil && x.secid == prev.secid && x.relSpread > 0 && prev.relSpread > 0 && s.dates[d] >= "2025-01-03" {
					cs, ct = append(cs, math.Log(prev.relSpread)), append(ct, math.Log(x.relSpread))
				}
				prev = x
			}
		}
		if len(cs) > 100 {
			ra, rb := rankScale(cs), rankScale(ct)
			ma, mb := mean(ra), mean(rb)
			var sab, saa, sbb float64
			for i := range ra {
				sab += (ra[i] - ma) * (rb[i] - mb)
				saa += (ra[i] - ma) * (ra[i] - ma)
				sbb += (rb[i] - mb) * (rb[i] - mb)
			}
			fmt.Printf("  relative spread persistence (rank correlation d vs d+1, realtime, same contract): %.3f\n", sab/math.Sqrt(saa*sbb))
		}
	}
	fmt.Printf("== testable dates per feature in %s..%s (fo, >=20 liquid groups with the feature, and the h1 outcome eligible):\n", c.fo.dates[c.lo], c.fo.dates[c.hi])
	for _, n := range featNames {
		cnt, first, last := 0, "", ""
		for d := c.lo; d <= c.hi; d++ {
			L := c.fo.liquid(d, 30)
			k := 0
			for gi, g := range c.fo.groups {
				if L[g] && !isNaN(c.foF[n][gi][d]) && !isNaN(c.fo.sret(g, d, 1)) {
					k++
				}
			}
			if k >= 20 {
				cnt++
				if first == "" {
					first = c.fo.dates[d]
				}
				last = c.fo.dates[d]
			}
		}
		fmt.Printf("  %-4s dates %d (%s..%s)\n", n, cnt, first, last)
	}
	L := c.fo.liquid(c.hi, 30)
	var gs []string
	for g := range L {
		gs = append(gs, g)
	}
	sort.Strings(gs)
	fmt.Printf("== liquid fo universe at %s (top 30): %v\n", c.fo.dates[c.hi], gs)
	fmt.Printf("== fx groups: %v\n", c.fx.groups)
}
