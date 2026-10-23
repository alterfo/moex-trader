package main

import (
	"fmt"
	"math"
	"sort"
)

func (c *ctx) dbg(market, g, feat string) {
	s, F := c.fo, c.foF
	lo, hi := c.lo, c.hi
	if market == "fx" {
		s, F, lo, hi = c.fx, c.fxF, c.fxLo, c.fxHi
	}
	gi := s.gi(g)
	fmt.Printf("dbg %s %s %s: group index %d, window %s..%s\n", market, g, feat, gi, s.dates[lo], s.dates[hi])
	if gi < 0 {
		return
	}
	var nFront, nFeat, nZ, nCtl, nY, nAll int
	for d := lo; d <= hi; d++ {
		if s.byGroup[g][d] != nil {
			nFront++
		}
		if !math.IsNaN(F[feat][gi][d]) {
			nFeat++
		}
		if !math.IsNaN(z60(F[feat][gi], d)) {
			nZ++
		}
		ct := s.ctrls(g, d)
		if ct != nil {
			nCtl++
		}
		if !math.IsNaN(s.sret(g, d, 1)) {
			nY++
		}
		if !math.IsNaN(z60(F[feat][gi], d)) && ct != nil && !math.IsNaN(s.sret(g, d, 1)) {
			nAll++
		}
	}
	fmt.Printf("  dates with front %d, feature %d, z60 %d, ctrls %d, y %d, all %d\n", nFront, nFeat, nZ, nCtl, nY, nAll)
}

func (c *ctx3) expcheck(group string) {
	s := c.c.fo
	exp := c.expWk
	fs := s.frontSec[group]
	byWeek := map[string]int{}
	total, inWeek := 0, 0
	for d := 1; d < len(s.dates); d++ {
		if fs[d] != "" && fs[d-1] != "" && fs[d] != fs[d-1] && s.dates[d] >= "2025-01-01" {
			total++
			if exp[d] > 0 {
				inWeek++
			}
			byWeek[s.dates[d]]++
		}
	}
	var days []string
	for d := range byWeek {
		days = append(days, d)
	}
	sort.Strings(days)
	fmt.Printf("expcheck %s: front changes since 2025: %d, inside third-Thursday expiry weeks: %d; dates: %v\n", group, total, inWeek, days)
	var flagged []string
	for d, v := range exp {
		if v > 0 && s.dates[d] >= "2025-01-01" {
			flagged = append(flagged, s.dates[d])
		}
	}
	fmt.Printf("  flagged expiry-week dates: %v\n", flagged)
}
