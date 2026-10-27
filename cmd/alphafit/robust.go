package main

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var delistDay map[string]int
var delistHits int

var robustBlocks = [][2]string{{"2025-01-03", "2025-03-31"}, {"2025-04-01", "2025-06-30"}, {"2025-07-01", "2025-09-30"}, {"2025-10-01", "2025-12-31"}, {"2026-01-01", "2026-03-31"}, {"2026-04-01", "2026-06-30"}, {"2026-07-01", "2026-10-16"}}

var robustAlias = map[string]string{"YDEX": "YNDX", "T": "TCSG"}

func robustRun(pp *pipe, fz frozen, keep bool) *runner {
	t0 := sort.SearchStrings(pp.ds.dates, "2020-04-01")
	r := newRunner(pp, fz, []string{"blend"})
	if keep {
		r.keepAll = map[int][]float64{}
	}
	for _, b := range robustBlocks {
		lo, hi := pp.dateRange(b[0], b[1])
		r.block(lo, hi, t0, nil, true)
	}
	return r
}

func windowSeries(x []float64, lo, hi int) []float64 {
	return x[lo : hi+1]
}

func lastTradeDates(dir, from string) (map[string]string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "tradestats", "*.csv.gz"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	out := map[string]string{}
	for _, f := range files {
		day := strings.TrimSuffix(filepath.Base(f), ".csv.gz")
		if day < from {
			continue
		}
		fh, err := os.Open(f)
		if err != nil {
			return nil, err
		}
		zr, err := gzip.NewReader(fh)
		if err != nil {
			fh.Close()
			return nil, err
		}
		sc := bufio.NewScanner(zr)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		sc.Scan()
		for sc.Scan() {
			p := strings.SplitN(sc.Text(), ",", 4)
			if len(p) < 4 {
				continue
			}
			s := p[2]
			if a, ok := robustAlias[s]; ok {
				s = a
			}
			out[s] = day
		}
		zr.Close()
		fh.Close()
	}
	return out, nil
}

func buildDelist(ds *dataset, dir string, cutoff string) map[string]int {
	last, err := lastTradeDates(dir, "2025-01-03")
	if err != nil {
		log.Fatal(err)
	}
	inMatrix := map[string]bool{}
	for _, x := range ds.rows {
		inMatrix[x.secid] = true
	}
	res := map[string]int{}
	for s, day := range last {
		if !inMatrix[s] || day >= cutoff {
			continue
		}
		i := sort.SearchStrings(ds.dates, day)
		if i >= len(ds.dates) {
			continue
		}
		res[s] = i
	}
	return res
}

func snapshotFeat(ds *dataset) [][]float32 {
	out := make([][]float32, len(ds.rows))
	for i, x := range ds.rows {
		out[i] = append([]float32(nil), x.feat...)
	}
	return out
}

func restoreFeat(ds *dataset, snap [][]float32) {
	for i, x := range ds.rows {
		copy(x.feat, snap[i])
	}
}

func perturbFeat(ds *dataset, sd float64, seed int64) {
	nf := len(ds.names)
	rng := rand.New(rand.NewSource(seed))
	for _, x := range ds.rows {
		for k := 0; k < nf; k++ {
			if !math.IsNaN(x.raw[k]) {
				x.feat[k] += float32(rng.NormFloat64() * sd)
			}
		}
		for c, v := range []float64{x.rb1, x.rb5, x.rb20} {
			if !math.IsNaN(v) {
				x.feat[nf+c] += float32(rng.NormFloat64() * sd)
			}
		}
	}
}

func normQuantile(p float64) float64 {
	return math.Sqrt2 * math.Erfinv(2*p-1)
}

func expectedMaxZ(n float64) float64 {
	const gamma = 0.5772156649015329
	return (1-gamma)*normQuantile(1-1/n) + gamma*normQuantile(1-1/(n*math.E))
}

func sharpeAnn(x []float64) float64 {
	c := clean(x)
	m := mean(c)
	v := 0.0
	for _, y := range c {
		v += (y - m) * (y - m)
	}
	return m / math.Sqrt(v/float64(len(c)-1)) * math.Sqrt(252)
}

func staleSeries(pp *pipe, r *runner, k, lo, hi int, withBook bool, fz frozen) ([]float64, []float64) {
	ic := make([]float64, len(pp.ds.dates))
	for i := range ic {
		ic[i] = nan
	}
	var rb *runner
	if withBook {
		rb = newRunner(pp, fz, []string{"blend"})
	}
	for d := lo; d <= hi; d++ {
		rows := pp.ds.byDate[d]
		pr := make([]float64, len(rows))
		for i := range pr {
			pr[i] = nan
		}
		if d-k >= lo {
			prev := pp.ds.byDate[d-k]
			pv := r.keepAll[d-k]
			idx := map[string]float64{}
			for i, x := range prev {
				idx[x.secid] = pv[i]
			}
			for i, x := range rows {
				if v, ok := idx[x.secid]; ok {
					pr[i] = v
				}
			}
		}
		ic[d] = icOf(rows, pr)
		if rb != nil && d-k >= lo {
			rb.day(d, map[string][]float64{"blend": pr})
		}
	}
	if rb == nil {
		return ic, nil
	}
	return ic, rb.out.netGP["blend"]
}

func robust(pp *pipe, frzPath, dir string) {
	raw, err := os.ReadFile(frzPath)
	if err != nil {
		log.Fatal(err)
	}
	var fz frozen
	json.Unmarshal(raw, &fz)
	ds := pp.ds
	lo, hi := pp.dateRange("2025-01-03", "2026-10-16")
	split := sort.SearchStrings(ds.dates, "2026-01-05") - lo
	const draws = 5000
	win := func(x []float64) []float64 { return windowSeries(x, lo, hi) }

	base := robustRun(pp, fz, true)
	baseIC := win(base.out.ic["blend"])
	baseGP := win(base.out.netGP["blend"])
	baseD := win(base.out.netD["blend"])
	fmt.Printf("== baseline: frozen composite, ridge %.3f en %.4f rate %.2f, window %s..%s, %d dates\n", fz.LambdaRidge, fz.LambdaEN, fz.Rate, ds.dates[lo], ds.dates[hi], hi-lo+1)
	report("IC blend", baseIC, split, draws)
	report("net/day daily book", baseD, split, draws)
	report("net/day GP book", baseGP, split, draws)
	icBase := mean(clean(baseIC))
	fmt.Printf("Sharpe annualised GP book %+.2f\n", sharpeAnn(baseGP))

	fmt.Println("== 1. noise in the inputs (refit), sd = m x 0.289")
	snap := snapshotFeat(ds)
	for _, m := range []float64{0.25, 0.5, 1.0} {
		var ics, nets []float64
		for _, sd := range []int64{1, 2, 3} {
			perturbFeat(ds, m*0.289, 20261027+sd)
			np := newPipe(ds)
			r := robustRun(np, fz, false)
			ic, net := mean(clean(win(r.out.ic["blend"]))), mean(clean(win(r.out.netGP["blend"])))
			ics = append(ics, ic)
			nets = append(nets, net)
			restoreFeat(ds, snap)
			fmt.Printf("m %.2f seed %d: IC %+.5f (%.0f%% of clean) GP net/day %+.5f%%\n", m, sd, ic, ic/icBase*100, net*100)
		}
		fmt.Printf("m %.2f mean IC %+.5f (%.0f%% of clean), mean GP net/day %+.5f%%\n", m, mean(ics), mean(ics)/icBase*100, mean(nets)*100)
	}

	fmt.Println("== 2. stale signal, prediction of day d-k against the outcome of day d")
	for _, k := range []int{0, 1, 2, 5, 10} {
		ic, net := staleSeries(pp, base, k, lo, hi, k == 1 || k == 2, fz)
		w := win(ic)
		fmt.Printf("k %2d: IC %+.5f (%.0f%% of k=0)", k, mean(clean(w)), mean(clean(w))/icBase*100)
		if net != nil {
			fmt.Printf("  GP net/day %+.5f%%", mean(clean(win(net)))*100)
		}
		fmt.Println()
	}

	fmt.Println("== 3. total loss for names that left the board")
	delistDay = buildDelist(ds, dir, "2026-10-10")
	var names []string
	for s := range delistDay {
		names = append(names, s)
	}
	sort.Strings(names)
	fmt.Printf("%d names with the last trade before 2026-10-10: %s\n", len(names), strings.Join(names, " "))
	delistHits = 0
	tl := robustRun(pp, fz, false)
	delistDay = nil
	tlGP, tlD := win(tl.out.netGP["blend"]), win(tl.out.netD["blend"])
	fmt.Printf("long positions lost in full, counted over the four books of the run: %d\n", delistHits)
	report("net/day GP total loss", tlGP, split, draws)
	report("GP total loss minus base", diff(tlGP, baseGP), split, draws)
	report("net/day daily total loss", tlD, split, draws)
	report("daily total loss minus base", diff(tlD, baseD), split, draws)

	fmt.Println("== 4. dividends")
	inWin, flagged, flaggedNaN, allNaN := 0, 0, 0, 0
	for d := lo; d <= hi; d++ {
		for _, x := range ds.byDate[d] {
			inWin++
			nan1 := math.IsNaN(x.f1)
			if nan1 {
				allNaN++
			}
			if x.divDrop {
				flagged++
				if nan1 {
					flaggedNaN++
				}
			}
		}
	}
	fmt.Printf("ticker-days in the window %d; within one day of an ex-date %d (%.2f%%); next-day return missing %d, of which at ex-date days %d\n", inWin, flagged, float64(flagged)/float64(inWin)*100, allNaN, flaggedNaN)
	saved := make([]float64, len(ds.rows))
	cut := 0
	for i, x := range ds.rows {
		saved[i] = x.f1
		if !math.IsNaN(x.f1) && !math.IsNaN(x.u1) && math.Abs(x.f1-x.u1) > 0.05 {
			x.f1 = nan
			cut++
		}
	}
	gap := robustRun(pp, fz, false)
	for i, x := range ds.rows {
		x.f1 = saved[i]
	}
	fmt.Printf("proxy bound: next-day returns more than 5 points away from the universe mean set missing (%d ticker-days in all years, includes dividend gaps and news jumps)\n", cut)
	report("net/day GP, jumps removed", win(gap.out.netGP["blend"]), split, draws)
	report("GP jumps removed minus base", diff(win(gap.out.netGP["blend"]), baseGP), split, draws)
	report("net/day daily, jumps removed", win(gap.out.netD["blend"]), split, draws)
	fmt.Println("== 5. independent periods")
	posQ := 0
	for _, b := range robustBlocks {
		a, z := pp.dateRange(b[0], b[1])
		ic := clean(base.out.ic["blend"][a : z+1])
		m, t := neweyWest(ic, 5)
		net := mean(clean(base.out.netGP["blend"][a : z+1]))
		if m > 0 {
			posQ++
		}
		fmt.Printf("%s..%s n %3d IC %+.5f NW5 t %+.2f GP net/day %+.5f%%\n", b[0], b[1], len(ic), m, t, net*100)
	}
	fmt.Printf("quarters with positive IC: %d of %d\n", posQ, len(robustBlocks))
	months := map[string][]float64{}
	netM := map[string][]float64{}
	for d := lo; d <= hi; d++ {
		k := ds.dates[d][:7]
		if v := base.out.ic["blend"][d]; !math.IsNaN(v) {
			months[k] = append(months[k], v)
		}
		if v := base.out.netGP["blend"][d]; !math.IsNaN(v) {
			netM[k] = append(netM[k], v)
		}
	}
	var keys []string
	for k := range months {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	posM, posN := 0, 0
	for _, k := range keys {
		m, nm := mean(months[k]), mean(netM[k])
		if m > 0 {
			posM++
		}
		if nm > 0 {
			posN++
		}
		fmt.Printf("%s days %2d IC %+.5f GP net/day %+.5f%%\n", k, len(months[k]), m, nm*100)
	}
	fmt.Printf("months with positive IC: %d of %d; with positive GP net: %d of %d\n", posM, len(keys), posN, len(keys))

	fmt.Println("== 6. minimum backtest length")
	sr := sharpeAnn(baseGP)
	years := float64(hi-lo+1) / 252
	fmt.Printf("observed annualised Sharpe of the GP book %+.2f over %.2f years\n", sr, years)
	for _, n := range []float64{10, 50, 100, 300, 800} {
		z := expectedMaxZ(n)
		fmt.Printf("N %4.0f: expected maximum annualised Sharpe of skill-free strategies %.2f; years needed for the observed Sharpe to exceed it %.2f\n", n, z/math.Sqrt(years), (z/sr)*(z/sr))
	}
}
