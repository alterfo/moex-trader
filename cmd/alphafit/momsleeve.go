package main

import (
	"fmt"
	"math"
	"math/rand"
)

type sleeveRun struct {
	gross, cost, short, turn []float64
	net17, net25, net275     []float64
}

func sleeveSignal(t *tfmCtx, d int, rows []*row, kind string) []float64 {
	out := filled(len(rows))
	for i, x := range rows {
		s := t.ser[x.secid]
		p, ok := t.pos[x.secid][d]
		if !ok || p < 255 {
			continue
		}
		if kind == "mom121" {
			out[i] = sumRange(s.ab, p-255, p-20)
		} else {
			out[i] = sumRange(s.ab, p-255, p)
		}
	}
	return out
}

func runSleeve(t *tfmCtx, kind string, seed int64, rate float64, long bool, a, b int) *sleeveRun {
	ds := t.ds
	nd := b - a + 1
	r := &sleeveRun{}
	for _, p := range []*[]float64{&r.gross, &r.cost, &r.short, &r.turn, &r.net17, &r.net25, &r.net275} {
		*p = filled(nd)
	}
	prevLong := longOnly
	longOnly = long
	defer func() { longOnly = prevLong }()
	bk := newBook(rate)
	for d := a; d <= b; d++ {
		rows := ds.byDate[d]
		sig := sleeveSignal(t, d, rows, kind)
		if seed != 0 {
			var who []int
			for i := range sig {
				if !math.IsNaN(sig[i]) {
					who = append(who, i)
				}
			}
			rng := rand.New(rand.NewSource(seed*1000003 + int64(d)))
			perm := rng.Perm(len(who))
			saved := make([]float64, len(who))
			for j, i := range who {
				saved[j] = sig[i]
			}
			for j, i := range who {
				sig[i] = saved[perm[j]]
			}
		}
		aim := aimWeights(rows, sig)
		pl := bk.step(rows, aim)
		if long {
			bench := 0.0
			for _, x := range rows {
				if !math.IsNaN(x.u1) {
					bench = x.u1
					break
				}
			}
			pl.gross -= pl.wlong * bench
		}
		i := d - a
		r.gross[i], r.cost[i], r.short[i], r.turn[i] = pl.gross, pl.cost, pl.short, pl.turn
		r.net17[i] = pl.gross - pl.cost - pl.short*borrow17
		r.net25[i] = pl.gross - pl.cost - pl.short*borrow25
		r.net275[i] = r.net25[i] - 0.3125*(r.net17[i]-r.net25[i])
	}
	return r
}

func statLine(label string, x []float64, split int) string {
	v := clean(x)
	if len(v) < 50 {
		return fmt.Sprintf("%-44s not testable", label)
	}
	m, tv := neweyWest(v, 10)
	lo, hi := bootCI(v, 5000, 20261026)
	h1, h2 := halves(x, split)
	return fmt.Sprintf("%-44s mean %+.5f%% NW t %+.2f boot95 [%+.5f%%, %+.5f%%] halves %+.5f%% / %+.5f%% n %d", label, m*100, tv, lo*100, hi*100, h1*100, h2*100, len(v))
}

func firstSignalDate(t *tfmCtx, kind string, from int) int {
	for d := from; d < len(t.ds.dates); d++ {
		rows := t.ds.byDate[d]
		sig := sleeveSignal(t, d, rows, kind)
		n := 0
		for _, v := range sig {
			if !math.IsNaN(v) {
				n++
			}
		}
		if n >= 40 {
			return d
		}
	}
	return from
}

func momSleeve(pp *pipe, basePath, kind, stage string, rateL, rateS float64) {
	ds := pp.ds
	t := buildTfmCtx(pp, "")
	_, eb := pp.dateRange("2022-04-01", "2024-12-31")
	ea := firstSignalDate(t, kind, 0)
	ra, rb := pp.dateRange("2025-01-03", "2026-10-16")
	rates := []float64{0.10, 0.05, 0.03}
	fmt.Printf("momsleeve: stage %s, signal %s, exploration %s..%s, realtime %s..%s, cost mode %s\n", stage, kind, ds.dates[ea], ds.dates[eb], ds.dates[ra], ds.dates[rb], costMode)
	esplit := sortSearch(ds.dates, "2023-07-01") - ea
	if stage == "explore" {
		nan, tot := 0, 0
		for d := ea; d <= eb; d++ {
			for _, x := range ds.byDate[d] {
				tot++
				if math.IsNaN(x.spb) || x.spb <= 0 {
					nan++
				}
			}
		}
		fmt.Printf("momsleeve SpBBO missing or non-positive on exploration rows: %.3f of %d\n", float64(nan)/float64(tot), tot)
		bl, bs := math.Inf(-1), math.Inf(-1)
		var chL, chS float64
		allPosL, allPosS := true, true
		for _, r := range rates {
			l := runSleeve(t, kind, 0, r, true, ea, eb)
			sl := runSleeve(t, kind, 0, r, false, ea, eb)
			ml, ms := mean(clean(l.net17)), mean(clean(sl.net25))
			fmt.Printf("momsleeve exploration rate %.2f: S1 long-only net %+.5f%% (gross %+.5f%%, turnover %.3f); S2 sleeve net25 %+.5f%% (gross %+.5f%%, turnover %.3f)\n", r, ml*100, mean(clean(l.gross))*100, mean(clean(l.turn)), ms*100, mean(clean(sl.gross))*100, mean(clean(sl.turn)))
			if ml <= 0 {
				allPosL = false
			}
			if ms <= 0 {
				allPosS = false
			}
			if ml >= bl {
				bl, chL = ml, r
			}
			if ms >= bs {
				bs, chS = ms, r
			}
		}
		fmt.Printf("momsleeve frozen rates (ties to the lower rate): S1 %.2f, S2 %.2f; exploration net positive at all three rates: S1 %v, S2 %v\n", chL, chS, allPosL, allPosS)
		fmt.Println(statLine("S1 exploration long-only net at the chosen rate", runSleeve(t, kind, 0, chL, true, ea, eb).net17, esplit))
		fmt.Println(statLine("S2 exploration sleeve net 25% at the chosen rate", runSleeve(t, kind, 0, chS, false, ea, eb).net25, esplit))
		return
	}
	bestL, bestS := rateL, rateS
	split := sortSearch(ds.dates, "2026-01-05") - ra
	fmt.Println(statLine("S1 exploration long-only net (no carry)", runSleeve(t, kind, 0, bestL, true, ea, eb).net17, esplit))
	fmt.Println(statLine("S2 exploration sleeve net 25% carry", runSleeve(t, kind, 0, bestS, false, ea, eb).net25, esplit))
	fmt.Println("momsleeve realtime:")
	rl := runSleeve(t, kind, 0, bestL, true, ra, rb)
	fmt.Printf("momsleeve S1 turnover %.3f gross %+.5f%%\n", mean(clean(rl.turn)), mean(clean(rl.gross))*100)
	fmt.Println(statLine("S1 realtime long-only net (no carry)", rl.net17, split))
	plcL := make([]*sleeveRun, 5)
	for k := range plcL {
		plcL[k] = runSleeve(t, kind, int64(20261027+k), bestL, true, ra, rb)
		fmt.Println(statLine(fmt.Sprintf("S1 placebo %d long-only net", k+1), plcL[k].net17, split))
	}
	rs := runSleeve(t, kind, 0, bestS, false, ra, rb)
	fmt.Printf("momsleeve S2 sleeve turnover %.3f gross %+.5f%% short %.3f\n", mean(clean(rs.turn)), mean(clean(rs.gross))*100, mean(clean(rs.short)))
	fmt.Println(statLine("S2 realtime sleeve net 17% carry", rs.net17, split))
	fmt.Println(statLine("S2 realtime sleeve net 25% carry", rs.net25, split))
	fmt.Println(statLine("S2 realtime sleeve net 27.5% carry", rs.net275, split))
	cols, dates := readDump(basePath)
	idx := map[string]int{}
	for i, dd := range dates {
		idx[dd] = i
	}
	cmp := func(col string) []float64 {
		o := filled(rb - ra + 1)
		for d := ra; d <= rb; d++ {
			if i, ok := idx[ds.dates[d]]; ok {
				o[d-ra] = cols[col][i]
			}
		}
		return o
	}
	c17, c25 := cmp("net_gp_blend"), cmp("net_gp25_blend")
	blend := func(sl *sleeveRun) (b17, b25, b275 []float64) {
		b17, b25, b275 = filled(len(c17)), filled(len(c17)), filled(len(c17))
		for i := range c17 {
			if math.IsNaN(c17[i]) || math.IsNaN(c25[i]) || math.IsNaN(sl.net25[i]) {
				continue
			}
			b17[i] = 0.5*c17[i] + 0.5*sl.net17[i]
			b25[i] = 0.5*c25[i] + 0.5*sl.net25[i]
			b275[i] = 0.5*(c25[i]-0.3125*(c17[i]-c25[i])) + 0.5*sl.net275[i]
		}
		return
	}
	diff := func(a, b []float64) []float64 {
		o := filled(len(a))
		for i := range a {
			if !math.IsNaN(a[i]) && !math.IsNaN(b[i]) {
				o[i] = a[i] - b[i]
			}
		}
		return o
	}
	fmt.Println(statLine("composite sleeve alone net 25% carry", c25, split))
	t17, t25, t275 := blend(rs)
	fmt.Println(statLine("S2 blend net 17% carry", t17, split))
	fmt.Println(statLine("S2 blend net 25% carry", t25, split))
	fmt.Println(statLine("S2 blend net 27.5% carry", t275, split))
	td := diff(t25, c25)
	fmt.Println(statLine("S2 blend minus composite alone, 25% carry", td, split))
	pd := make([][]float64, 5)
	avg := filled(len(td))
	for k := 0; k < 5; k++ {
		ps := runSleeve(t, kind, int64(20261027+k), bestS, false, ra, rb)
		_, p25, _ := blend(ps)
		pd[k] = diff(p25, c25)
		fmt.Println(statLine(fmt.Sprintf("S2 placebo %d blend minus composite, 25%%", k+1), pd[k], split))
		for i := range avg {
			if math.IsNaN(pd[k][i]) {
				continue
			}
			if math.IsNaN(avg[i]) {
				avg[i] = 0
			}
			avg[i] += pd[k][i] / 5
		}
	}
	fmt.Println(statLine("S2 placebo-corrected difference, 25% carry", diff(td, avg), split))
}

func s3Diff(basePath, extPath, plcPath string) {
	bc, dates := readDump(basePath)
	ec, ed := readDump(extPath)
	pc, pd := readDump(plcPath)
	if len(dates) != len(ed) || len(dates) != len(pd) {
		fatal(fmt.Errorf("dumps differ in length: %d %d %d", len(dates), len(ed), len(pd)))
	}
	split := 0
	for i, d := range dates {
		if d < "2026-01-05" {
			split = i + 1
		}
	}
	for _, col := range []string{"net_gp_blend", "net_gp25_blend"} {
		b, e, p := bc[col], ec[col], pc[col]
		de, dp, cor := filled(len(b)), filled(len(b)), filled(len(b))
		for i := range b {
			if math.IsNaN(b[i]) || math.IsNaN(e[i]) || math.IsNaN(p[i]) {
				continue
			}
			de[i], dp[i] = e[i]-b[i], p[i]-b[i]
			cor[i] = de[i] - dp[i]
		}
		fmt.Println(statLine("S3 "+col+" baseline", b, split))
		fmt.Println(statLine("S3 "+col+" extended (momentum)", e, split))
		fmt.Println(statLine("S3 "+col+" placebo", p, split))
		fmt.Println(statLine("S3 "+col+" extended minus baseline", de, split))
		fmt.Println(statLine("S3 "+col+" placebo minus baseline", dp, split))
		fmt.Println(statLine("S3 "+col+" placebo-corrected difference", cor, split))
	}
}
