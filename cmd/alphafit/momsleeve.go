package main

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
)

type sleeveRun struct {
	gross, cost, short, turn []float64
	net17, net25, net275     []float64
	wlong, bench, univ       []float64
	thin                     int
}

var equalWeight bool

var minAim = 40

func isRevKind(kind string) bool {
	return kind == "gap" || kind == "blend" || strings.HasPrefix(kind, "cand:")
}

func revSignal(t *tfmCtx, d int, rows []*row, kind string) []float64 {
	out := filled(len(rows))
	if strings.HasPrefix(kind, "cand:") {
		for i, x := range rows {
			out[i] = t.file(d, x.secid, kind[5:])
		}
		return out
	}
	names := []string{"B05"}
	if kind == "blend" {
		names = []string{"B05", "B04", "B12"}
	}
	cols := make([][]float64, len(names))
	var who []int
	for i, x := range rows {
		ok := true
		vals := make([]float64, len(names))
		for k, nm := range names {
			vals[k] = t.file(d, x.secid, nm)
			if math.IsNaN(vals[k]) {
				ok = false
			}
		}
		if !ok {
			continue
		}
		who = append(who, i)
		for k := range names {
			cols[k] = append(cols[k], vals[k])
		}
	}
	if len(who) < 30 {
		return out
	}
	acc := make([]float64, len(who))
	for k := range names {
		rk := rankScale(cols[k])
		for j := range who {
			acc[j] += rk[j] / float64(len(names))
		}
	}
	for j, i := range who {
		out[i] = -acc[j]
	}
	return out
}

func isDiagKind(kind string) bool {
	return isRevKind(kind) || isLBKind(kind)
}

func isLBKind(kind string) bool {
	return kind == "lbmom" || kind == "lbmom3"
}

func lbSignal(t *tfmCtx, d int, rows []*row, kind string) []float64 {
	out := filled(len(rows))
	var who []int
	for i, x := range rows {
		if !math.IsNaN(t.file(d, x.secid, "M256")) && !math.IsNaN(t.file(d, x.secid, "C01")) {
			who = append(who, i)
		}
	}
	if len(who) < 30 {
		return out
	}
	sort.SliceStable(who, func(a, b int) bool {
		return t.file(d, rows[who[a]].secid, "C01") < t.file(d, rows[who[b]].secid, "C01")
	})
	k := len(who) / 2
	if kind == "lbmom3" {
		k = len(who) / 3
	}
	for _, i := range who[:k] {
		out[i] = t.file(d, rows[i].secid, "M256")
	}
	return out
}

func lbBench(rows []*row, sig []float64) float64 {
	s, n := 0.0, 0
	for i, x := range rows {
		if !math.IsNaN(sig[i]) && !math.IsNaN(x.f1) && !x.divDrop {
			s += x.f1
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return s / float64(n)
}

func minAimFor(kind string) int {
	switch kind {
	case "lbmom":
		return 24
	case "lbmom3":
		return 18
	}
	return 40
}

func sleeveSignal(t *tfmCtx, d int, rows []*row, kind string) []float64 {
	if isRevKind(kind) {
		return revSignal(t, d, rows, kind)
	}
	if isLBKind(kind) {
		return lbSignal(t, d, rows, kind)
	}
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
	return runSleeveWith(t, kind, rate, long, a, b, func(d int, rows []*row) []float64 {
		sig := sleeveSignal(t, d, rows, kind)
		if seed != 0 {
			permuteSignal(sig, seed, d)
		}
		return sig
	})
}

func runSleeveWith(t *tfmCtx, kind string, rate float64, long bool, a, b int, signal func(d int, rows []*row) []float64) *sleeveRun {
	ds := t.ds
	nd := b - a + 1
	r := &sleeveRun{}
	for _, p := range []*[]float64{&r.gross, &r.cost, &r.short, &r.turn, &r.net17, &r.net25, &r.net275, &r.wlong, &r.bench, &r.univ} {
		*p = filled(nd)
	}
	prevLong, prevMin := longOnly, minAim
	longOnly, minAim = long, minAimFor(kind)
	defer func() { longOnly, minAim = prevLong, prevMin }()
	bk := newBook(rate)
	for d := a; d <= b; d++ {
		rows := ds.byDate[d]
		sig := signal(d, rows)
		aim := aimWeights(rows, sig)
		pl := bk.step(rows, aim)
		thin := len(aim) == 0
		if thin {
			r.thin++
		}
		if long {
			bench := 0.0
			for _, x := range rows {
				if !math.IsNaN(x.u1) {
					bench = x.u1
					break
				}
			}
			r.univ[d-a] = bench
			if isLBKind(kind) {
				bench = lbBench(rows, sleeveSignal(t, d, rows, kind))
			}
			pl.gross -= pl.wlong * bench
			r.wlong[d-a], r.bench[d-a] = pl.wlong, bench
		}
		i := d - a
		r.gross[i], r.cost[i], r.short[i], r.turn[i] = pl.gross, pl.cost, pl.short, pl.turn
		if thin && (isRevKind(kind) || isLBKind(kind)) {
			continue
		}
		r.net17[i] = pl.gross - pl.cost - pl.short*borrow17
		r.net25[i] = pl.gross - pl.cost - pl.short*borrow25
		r.net275[i] = r.net25[i] - 0.3125*(r.net17[i]-r.net25[i])
	}
	return r
}

func nanAdd(a, b float64) float64 {
	if math.IsNaN(a) {
		return b
	}
	return a + b
}

func lbRateOK(rate float64) bool {
	return rate == 0.10 || rate == 0.05 || rate == 0.03
}

func revRateOK(rate float64) bool {
	return rate == 1.0 || rate == 0.5 || rate == 0.3
}

func permuteSignal(sig []float64, seed int64, d int) {
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

func breakEven(r *sleeveRun) float64 {
	g, t := mean(clean(r.gross)), mean(clean(r.turn))
	if t <= 0 {
		return nan
	}
	return g / t
}

func rateLine(label string, rate float64, r *sleeveRun) string {
	return fmt.Sprintf("%s rate %.2f: gross %+.5f%% a day, cost %+.5f%%, turnover %.3f, break-even cost per side %.2f bp (charged on average %.2f bp), thin days %d", label, rate, mean(clean(r.gross))*100, mean(clean(r.cost))*100, mean(clean(r.turn)), breakEven(r)*1e4, mean(clean(r.cost))/math.Max(1e-12, mean(clean(r.turn)))*1e4, r.thin)
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
	if isLBKind(kind) {
		requirePlaybookFile(candFile)
	}
	path := ""
	if isRevKind(kind) || isLBKind(kind) {
		path = candFile
	}
	t := buildTfmCtx(pp, path)
	_, eb := pp.dateRange("2022-04-01", "2024-12-31")
	ea := firstSignalDate(t, kind, 0)
	if isRevKind(kind) {
		ea = firstSignalDate(t, kind, sortSearch(ds.dates, "2021-04-08"))
	}
	if isLBKind(kind) {
		ea = sortSearch(ds.dates, "2021-04-08")
	}
	ra, rb := pp.dateRange("2025-01-03", "2026-10-16")
	rates := []float64{0.10, 0.05, 0.03}
	if isRevKind(kind) {
		rates = []float64{1.0, 0.5, 0.3}
	}
	if stage == "profile" {
		revProfile(t, kind, ea, eb, ra, rb)
		return
	}
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
			if isDiagKind(kind) {
				fmt.Println(rateLine("momsleeve S1", r, l))
				fmt.Println(rateLine("momsleeve S2", r, sl))
				fmt.Println(statLine(fmt.Sprintf("S1 exploration net at rate %.2f", r), l.net17, esplit))
				fmt.Println(statLine(fmt.Sprintf("S2 exploration net25 at rate %.2f", r), sl.net25, esplit))
			}
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
		if isDiagKind(kind) {
			realL, realS := runSleeve(t, kind, 0, chL, true, ea, eb), runSleeve(t, kind, 0, chS, false, ea, eb)
			avgL, avgS := filled(len(realL.net17)), filled(len(realS.net25))
			for k := 0; k < 5; k++ {
				pl := runSleeve(t, kind, int64(20261101+k), chL, true, ea, eb)
				ps := runSleeve(t, kind, int64(20261101+k), chS, false, ea, eb)
				fmt.Println(statLine(fmt.Sprintf("S1 exploration placebo %d net", k+1), pl.net17, esplit))
				fmt.Println(statLine(fmt.Sprintf("S2 exploration placebo %d net25", k+1), ps.net25, esplit))
				for i := range avgL {
					if !math.IsNaN(pl.net17[i]) {
						avgL[i] = nanAdd(avgL[i], pl.net17[i]/5)
					}
					if !math.IsNaN(ps.net25[i]) {
						avgS[i] = nanAdd(avgS[i], ps.net25[i]/5)
					}
				}
			}
			dl, ds2 := filled(len(avgL)), filled(len(avgS))
			for i := range dl {
				if !math.IsNaN(realL.net17[i]) && !math.IsNaN(avgL[i]) {
					dl[i] = realL.net17[i] - avgL[i]
				}
				if !math.IsNaN(realS.net25[i]) && !math.IsNaN(avgS[i]) {
					ds2[i] = realS.net25[i] - avgS[i]
				}
			}
			fmt.Println(statLine("S1 exploration placebo-corrected net (real minus mean of 5 random rankings)", dl, esplit))
			fmt.Println(statLine("S2 exploration placebo-corrected net25 (real minus mean of 5 random rankings)", ds2, esplit))
			bar := 2.64
			if isLBKind(kind) {
				bar = 2.0
			}
			lo1, md1, hi1 := availNames(t, kind, ea, eb)
			fmt.Printf("momsleeve %s: fixed bar for the primary book is t %.2f; thin days at the chosen rates S1 %d, S2 %d; names available per date min %d median %d max %d\n", kind, bar, realL.thin, realS.thin, lo1, md1, hi1)
		}
		return
	}
	bestL, bestS := rateL, rateS
	if isLBKind(kind) && (!lbRateOK(bestL) || !lbRateOK(bestS)) {
		fatal(fmt.Errorf("momsleeve verdict for %s needs -ratel and -rates from {0.10, 0.05, 0.03}, got %v and %v", kind, bestL, bestS))
	}
	if isRevKind(kind) {
		if !revRateOK(bestL) || !revRateOK(bestS) {
			fatal(fmt.Errorf("momsleeve verdict for %s needs -ratel and -rates from {1.0, 0.5, 0.3}, got %v and %v", kind, bestL, bestS))
		}
		if basePath == "" {
			fatal(fmt.Errorf("momsleeve verdict needs -base artifacts/algopack-wave3b/w3b-base.csv"))
		}
	}
	split := sortSearch(ds.dates, "2026-01-05") - ra
	fmt.Println(statLine("S1 exploration long-only net (no carry)", runSleeve(t, kind, 0, bestL, true, ea, eb).net17, esplit))
	fmt.Println(statLine("S2 exploration sleeve net 25% carry", runSleeve(t, kind, 0, bestS, false, ea, eb).net25, esplit))
	fmt.Println("momsleeve realtime:")
	rl := runSleeve(t, kind, 0, bestL, true, ra, rb)
	fmt.Printf("momsleeve S1 turnover %.3f gross %+.5f%%\n", mean(clean(rl.turn)), mean(clean(rl.gross))*100)
	if isDiagKind(kind) {
		fmt.Println(rateLine("momsleeve realtime S1", bestL, rl))
		lo1, md1, hi1 := availNames(t, kind, ra, rb)
		fmt.Printf("momsleeve realtime names available per date min %d median %d max %d; thin days %d (nets on thin days are missing and the gate t uses non-thin days only)\n", lo1, md1, hi1, rl.thin)
	}
	fmt.Println(statLine("S1 realtime long-only net (no carry)", rl.net17, split))
	absS1 := filled(len(rl.net17))
	uni := filled(len(rl.net17))
	for i := range absS1 {
		if !math.IsNaN(rl.net17[i]) {
			absS1[i] = rl.net17[i] + rl.wlong[i]*rl.bench[i]
			uni[i] = rl.bench[i]
		}
	}
	fmt.Printf("momsleeve S1 absolute net (book return after costs, no carry): %+.5f%% a day, %.1f%% a year; benchmark mean %+.5f%% a day, %.1f%% a year (the low-beta group mean for lbmom kinds, the equal-weight universe otherwise); mean deployed long weight %.3f\n", mean(clean(absS1))*100, mean(clean(absS1))*25200, mean(clean(uni))*100, mean(clean(uni))*25200, mean(clean(rl.wlong)))
	if isLBKind(kind) {
		prem := filled(len(rl.bench))
		for i := range prem {
			if !math.IsNaN(rl.bench[i]) && !math.IsNaN(rl.univ[i]) {
				prem[i] = rl.bench[i] - rl.univ[i]
			}
		}
		fmt.Printf("momsleeve universe equal-weight mean %+.5f%% a day\n", mean(clean(rl.univ))*100)
		fmt.Println(statLine("descriptive: low-beta group mean minus universe mean (the low-beta premium)", prem, split))
	}
	plcL := make([]*sleeveRun, 5)
	for k := range plcL {
		plcL[k] = runSleeve(t, kind, int64(20261027+k), bestL, true, ra, rb)
		fmt.Println(statLine(fmt.Sprintf("S1 placebo %d long-only net", k+1), plcL[k].net17, split))
	}
	if isLBKind(kind) {
		avg := filled(len(rl.net17))
		for _, p := range plcL {
			for i, v := range p.net17 {
				if !math.IsNaN(v) {
					avg[i] = nanAdd(avg[i], v/5)
				}
			}
		}
		corr := filled(len(avg))
		for i := range corr {
			if !math.IsNaN(rl.net17[i]) && !math.IsNaN(avg[i]) {
				corr[i] = rl.net17[i] - avg[i]
			}
		}
		fmt.Println(statLine("S1 realtime placebo-corrected net (real minus mean of 5 random rankings inside the group)", corr, split))
	}
	rs := runSleeve(t, kind, 0, bestS, false, ra, rb)
	fmt.Printf("momsleeve S2 sleeve turnover %.3f gross %+.5f%% short %.3f\n", mean(clean(rs.turn)), mean(clean(rs.gross))*100, mean(clean(rs.short)))
	if isDiagKind(kind) {
		fmt.Println(rateLine("momsleeve realtime S2", bestS, rs))
	}
	fmt.Println(statLine("S2 realtime sleeve net 17% carry", rs.net17, split))
	fmt.Println(statLine("S2 realtime sleeve net 25% carry", rs.net25, split))
	fmt.Println(statLine("S2 realtime sleeve net 27.5% carry", rs.net275, split))
	if isLBKind(kind) {
		return
	}
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
