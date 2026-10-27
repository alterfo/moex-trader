package main

import (
	"fmt"
	"math"
)

func revProfile(t *tfmCtx, kind string, ea, eb, ra, rb int) {
	ds := t.ds
	hs := []struct {
		h    int
		name string
		get  func(x *row) float64
	}{
		{1, "1 day", func(x *row) float64 { return x.f1 - x.u1 }},
		{5, "5 days", func(x *row) float64 { return x.f5 - x.u5 }},
		{10, "10 days", func(x *row) float64 { return x.f10 - x.u10 }},
	}
	spans := []struct {
		name   string
		a, b   int
		splitD string
	}{
		{"exploration", ea, eb, "2023-07-01"},
		{"realtime (spent for this signal)", ra, rb, "2026-01-05"},
	}
	fmt.Printf("revprofile: signal %s, daily rank IC against the abnormal return\n", kind)
	for _, sp := range spans {
		split := sortSearch(ds.dates, sp.splitD) - sp.a
		for _, h := range hs {
			ic := filled(sp.b - sp.a + 1)
			for d := sp.a; d <= sp.b; d++ {
				rows := ds.byDate[d]
				sig := sleeveSignal(t, d, rows, kind)
				var s, y []float64
				for i, x := range rows {
					o := h.get(x)
					if math.IsNaN(sig[i]) || math.IsNaN(o) || x.divDrop {
						continue
					}
					s, y = append(s, sig[i]), append(y, o)
				}
				if len(s) >= 30 {
					ic[d-sp.a] = pearson(rankScale(s), rankScale(y))
				}
			}
			v := clean(ic)
			if len(v) < 50 {
				fmt.Printf("revprofile %-34s %-8s not testable\n", sp.name, h.name)
				continue
			}
			m, tv := neweyWest(v, h.h+5)
			h1, h2 := halves(ic, split)
			fmt.Printf("revprofile %-34s %-8s mean IC %+.4f NW t %+.2f halves %+.4f / %+.4f dates %d\n", sp.name, h.name, m, tv, h1, h2, len(v))
		}
	}
}
