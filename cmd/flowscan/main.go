package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

type frozen struct {
	MEff      float64          `json:"m_eff"`
	M         int              `json:"m"`
	Threshold float64          `json:"threshold"`
	Drop      map[string][]int `json:"drop"`
}

type run struct {
	p    *panel
	win  window
	thr  float64
	fz   frozen
	w    io.Writer
	mode string
}

func listDays(dir, from, till string) []string {
	files, _ := filepath.Glob(filepath.Join(dir, "tradestats", "*.csv.gz"))
	var days []string
	for _, f := range files {
		d := strings.TrimSuffix(filepath.Base(f), ".csv.gz")
		if d >= from && d <= till {
			days = append(days, d)
		}
	}
	sort.Strings(days)
	return days
}

func loadAll(dir string, days []string, workers int, lagFrom string, lag int) []dayResult {
	results := make([]dayResult, len(days))
	var wg sync.WaitGroup
	sem := make(chan struct{}, workers)
	var done int64
	for i, d := range days {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, d string) {
			defer wg.Done()
			defer func() { <-sem }()
			o := loadOpts{dir: dir}
			if lag > 0 && d >= lagFrom {
				o.lagDays = lag
			}
			r, err := loadDay(o, d)
			if err != nil {
				log.Fatalf("flowscan: %v", err)
			}
			results[i] = r
			if n := atomic.AddInt64(&done, 1); n%200 == 0 {
				log.Printf("flowscan: loaded %d/%d days", n, len(days))
			}
		}(i, d)
	}
	wg.Wait()
	var kept []dayResult
	for _, r := range results {
		if len(r.recs) > 0 {
			kept = append(kept, r)
		}
	}
	return kept
}

func (r *run) buildX(sp *rowSpec) [][]float32 {
	p := r.p
	X := make([][]float32, len(p.tickers))
	for i := range X {
		X[i] = make([]float32, len(p.dates))
		for d := range X[i] {
			X[i][d] = float32(nan)
		}
	}
	for d := r.win.lo - 2; d <= r.win.hi && d < len(p.dates); d++ {
		if d < 61 {
			continue
		}
		adm := p.admittedAt(d)
		vals := sp.fn(p, d, adm)
		for k, i := range adm {
			if !math.IsNaN(vals[k]) && !math.IsInf(vals[k], 0) {
				X[i][d] = float32(vals[k])
			}
		}
	}
	return X
}

func ewmaX(X [][]float32, halfLife float64) [][]float32 {
	lam := math.Pow(0.5, 1/halfLife)
	out := make([][]float32, len(X))
	for i := range X {
		out[i] = make([]float32, len(X[i]))
		st := nan
		for d := range X[i] {
			x := float64(X[i][d])
			out[i][d] = float32(nan)
			if math.IsNaN(x) {
				continue
			}
			if math.IsNaN(st) {
				st = x
			} else {
				st = lam*st + (1-lam)*x
			}
			out[i][d] = float32(st)
		}
	}
	return out
}

func pct(v float64) string {
	if math.IsNaN(v) {
		return "n/a"
	}
	return fmt.Sprintf("%+.4f%%", v*100)
}

func (r *run) dropFor(id string) []int {
	return r.fz.Drop[id]
}

func (r *run) verdictT1(sp *rowSpec, e evalOut) bool {
	return e.t >= r.thr && e.net[0] > 0 && e.net[1] > 0 && !e.yearFlip
}

func (r *run) line(sp *rowSpec, e evalOut) string {
	v := ""
	if sp.tier == 1 {
		v = passWord(r.verdictT1(sp, e))
	}
	yrs := make([]string, 0, len(e.yearT))
	for y := range e.yearT {
		yrs = append(yrs, y)
	}
	sort.Strings(yrs)
	ys := ""
	for _, y := range yrs {
		ys += fmt.Sprintf(" %s:%+.1f", y, e.yearT[y])
	}
	return fmt.Sprintf("%s T%d sign %+.0f h%d n %d coef %+.5f t %+.2f halves %+.2f/%+.2f gross %s net %s / %s turn %.2f years%s %s", sp.id, sp.tier, sp.sign, holdOf(sp.kind), e.n, e.coef, e.t, e.halfT[0], e.halfT[1], pct(e.grossAll), pct(e.net[0]), pct(e.net[1]), e.turnAll, ys, v)
}

func (r *run) runAll(only map[string]bool) {
	w := r.w
	all := specs()
	res := map[string]evalOut{}
	xs := map[string][][]float32{}
	var specsRun []rowSpec
	for k := range all {
		sp := all[k]
		if len(only) > 0 && !only[sp.id] {
			continue
		}
		if sp.special {
			continue
		}
		X := r.buildX(&sp)
		to := tradeOpts{}
		if sp.id == "H010" {
			to.minN = 25
		}
		e := r.p.evaluate(X, sp.sign, sp.kind, r.win, r.dropFor(sp.id), to)
		res[sp.id] = e
		if sp.tier == 1 {
			xs[sp.id] = X
		}
		specsRun = append(specsRun, sp)
		fmt.Fprintln(w, r.line(&sp, e))
	}
	type pv struct {
		id string
		p  float64
		ok bool
	}
	var t2 []pv
	for _, sp := range specsRun {
		if sp.tier != 2 {
			continue
		}
		e := res[sp.id]
		t2 = append(t2, pv{sp.id, normP(e.t), !math.IsNaN(e.t)})
	}
	sort.Slice(t2, func(a, b int) bool { return t2[a].p < t2[b].p })
	m := len(t2)
	cut := -1
	for k, q := range t2 {
		if q.p <= 0.10*float64(k+1)/float64(m) {
			cut = k
		}
	}
	fmt.Fprintf(w, "\n== Tier 2 leads (BH q=0.10 over %d rows, registered sign required)\n", m)
	leads := 0
	for k, q := range t2 {
		if k <= cut && q.ok && res[q.id].t > 0 {
			fmt.Fprintf(w, "LEAD %s t %+.2f p %.4f\n", q.id, res[q.id].t, q.p)
			leads++
		}
	}
	fmt.Fprintf(w, "leads: %d\n", leads)
	fmt.Fprintln(w, "\n== Tier 1 summary")
	pass := 0
	for _, sp := range specsRun {
		if sp.tier == 1 && r.verdictT1(&sp, res[sp.id]) {
			fmt.Fprintf(w, "T1 PASS %s\n", sp.id)
			pass++
		}
	}
	fmt.Fprintf(w, "Tier 1 passed: %d (threshold t >= %.3f)\n", pass, r.thr)
	if _, ok := res["H001"]; ok || len(only) == 0 {
		r.specials(res, xs, all)
	}
}

func (r *run) specials(res map[string]evalOut, xs map[string][][]float32, all []rowSpec) {
	w := r.w
	p := r.p
	fmt.Fprintln(w, "\n== Special and conditional rows")
	base, ok := res["H001"]
	X1 := xs["H001"]
	if ok {
		r.h069(w, base)
		r.h080(w, base)
	}
	r.h082(w)
	r.h084(w)
	h001ok := ok && r.verdictT1(&rowSpec{id: "H001", tier: 1}, base)
	if ok && (h001ok || r.mode == "explore") {
		fmt.Fprintf(w, "H085-H088 run (H001 passed or explore mode: %v)\n", h001ok)
		for _, k := range []okind{kH1, kH3, kH5, kH10} {
			e := p.evaluate(X1, 1, k, r.win, r.dropFor("H001"), tradeOpts{})
			_, t := neweyWest(e.netSeries, lagsFor(holdOf(k)))
			fmt.Fprintf(w, "H085 hold %d: net per day %s halves %s / %s, NW t %+.2f (bar 2.50)\n", holdOf(k), pct(e.netAll), pct(e.net[0]), pct(e.net[1]), t)
		}
		on := p.evaluate(X1, 1, kOvernight, r.win, r.dropFor("H001"), tradeOpts{hold: 1})
		id := p.evaluate(X1, 1, kOpenH1, r.win, r.dropFor("H001"), tradeOpts{hold: 1})
		fmt.Fprintf(w, "H086 first-day gross, overnight %s / %s, intraday %s / %s (halves)\n", pct(on.gross[0]), pct(on.gross[1]), pct(id.gross[0]), pct(id.gross[1]))
		oe := p.evaluate(X1, 1, kOpenH5, r.win, r.dropFor("H001"), tradeOpts{})
		fmt.Fprintf(w, "H087 entry at next open, hold 5: net per day %s halves %s / %s; entry at close: %s halves %s / %s\n", pct(oe.netAll), pct(oe.net[0]), pct(oe.net[1]), pct(base.netAll), pct(base.net[0]), pct(base.net[1]))
		med := map[int]float64{}
		for d := r.win.lo; d <= r.win.hi && d < len(p.dates); d++ {
			var v []float64
			for _, i := range p.admittedAt(d) {
				if x := p.v(i, d, bSpLv); !math.IsNaN(x) {
					v = append(v, x)
				}
			}
			sort.Float64s(v)
			if len(v) > 0 {
				med[d] = v[len(v)/2]
			}
		}
		for _, low := range []bool{true, false} {
			lowCopy := low
			e := p.evaluate(X1, 1, kH5, r.win, r.dropFor("H001"), tradeOpts{keep: func(ti, d int) bool {
				s := p.v(ti, d, bSpLv)
				if math.IsNaN(s) {
					return false
				}
				return (s <= med[d]) == lowCopy
			}})
			name := "wider-spread half"
			if low {
				name = "narrower-spread half"
			}
			fmt.Fprintf(w, "H088 %s: net per day %s halves %s / %s, gross %s\n", name, pct(e.netAll), pct(e.net[0]), pct(e.net[1]), pct(e.grossAll))
		}
	} else {
		fmt.Fprintln(w, "H085-H088 not run: H001 did not pass")
	}
	a2 := rowSpec{id: "A2", tier: 2, sign: 1, kind: kH1, fn: col(bA2)}
	XA := r.buildX(&a2)
	ea := p.evaluate(XA, 1, kH1, r.win, nil, tradeOpts{})
	r.h092(w, ea.betas, ea.betaDays)
	r.h093(w, ea.betas, ea.betaDays)
	r.h094(w)
	fmt.Fprintln(w, "\n== H089 decay, H090 EWMA, H091 turnover filter for Tier 1 passes")
	for _, sp := range all {
		if sp.tier != 1 || sp.special {
			continue
		}
		e, ok := res[sp.id]
		if !ok || !r.verdictT1(&sp, e) {
			continue
		}
		X := xs[sp.id]
		for _, k := range []okind{kH1, kH3, kH5, kH10} {
			ee := p.evaluate(X, sp.sign, k, r.win, r.dropFor(sp.id), tradeOpts{})
			_, t := neweyWest(ee.netSeries, lagsFor(holdOf(k)))
			fmt.Fprintf(w, "H089 %s hold %d: net per day %s, NW t %+.2f\n", sp.id, holdOf(k), pct(ee.netAll), t)
		}
		ew := p.evaluate(ewmaX(X, 3), sp.sign, sp.kind, r.win, r.dropFor(sp.id), tradeOpts{})
		fmt.Fprintf(w, "H090 %s EWMA: t %+.2f net %s / %s turn %.2f\n", sp.id, ew.t, pct(ew.net[0]), pct(ew.net[1]), ew.turnAll)
		hy := p.evaluate(X, sp.sign, sp.kind, r.win, r.dropFor(sp.id), tradeOpts{hyst: 0.2})
		fmt.Fprintf(w, "H091 %s filter 0.2: net %s / %s turn %.2f\n", sp.id, pct(hy.net[0]), pct(hy.net[1]), hy.turnAll)
	}
}

func jacobiEigen(a [][]float64) []float64 {
	n := len(a)
	m := make([][]float64, n)
	for i := range m {
		m[i] = append([]float64(nil), a[i]...)
	}
	for sweep := 0; sweep < 100; sweep++ {
		off := 0.0
		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				off += m[i][j] * m[i][j]
			}
		}
		if off < 1e-12 {
			break
		}
		for p := 0; p < n; p++ {
			for q := p + 1; q < n; q++ {
				if math.Abs(m[p][q]) < 1e-15 {
					continue
				}
				th := (m[q][q] - m[p][p]) / (2 * m[p][q])
				t := 1 / (math.Abs(th) + math.Sqrt(th*th+1))
				if th < 0 {
					t = -t
				}
				c := 1 / math.Sqrt(t*t+1)
				s := t * c
				for k := 0; k < n; k++ {
					kp, kq := m[k][p], m[k][q]
					m[k][p], m[k][q] = c*kp-s*kq, s*kp+c*kq
				}
				for k := 0; k < n; k++ {
					pk, qk := m[p][k], m[q][k]
					m[p][k], m[q][k] = c*pk-s*qk, s*pk+c*qk
				}
			}
		}
	}
	ev := make([]float64, n)
	for i := range ev {
		ev[i] = m[i][i]
	}
	return ev
}

func (r *run) prep(out string) {
	p := r.p
	w := r.w
	all := specs()
	var feats []rowSpec
	for _, sp := range all {
		if !sp.special {
			feats = append(feats, sp)
		}
	}
	t1 := map[string]bool{}
	var t1Specs []rowSpec
	for _, sp := range feats {
		if sp.tier == 1 {
			t1[sp.id] = true
			t1Specs = append(t1Specs, sp)
		}
	}
	nf := len(feats)
	sumFF := make([][]float64, nf)
	cntFF := make([][]int, nf)
	for i := range sumFF {
		sumFF[i] = make([]float64, nf)
		cntFF[i] = make([]int, nf)
	}
	sumFC := make([][nCtrl]float64, nf)
	cntFC := make([][nCtrl]int, nf)
	for d := r.win.lo; d <= r.win.hi && d < len(p.dates); d++ {
		if d < 61 {
			continue
		}
		adm := p.admittedAt(d)
		cv := p.ctrlAt(d, 0)
		if !cv.built || len(cv.ok) != len(adm) {
			continue
		}
		ranks := make([][]float64, nf)
		for k := range feats {
			ranks[k] = rankOf(feats[k].fn(p, d, adm))
		}
		ctrlRanks := make([][]float64, nCtrl)
		for c := 0; c < nCtrl; c++ {
			v := make([]float64, len(adm))
			for j := range adm {
				v[j] = nan
				if cv.ok[j] {
					v[j] = cv.c[j][c]
				}
			}
			ctrlRanks[c] = rankOf(v)
		}
		for a := 0; a < nf; a++ {
			for c := 0; c < nCtrl; c++ {
				if x := corrPair(ranks[a], ctrlRanks[c]); !math.IsNaN(x) {
					sumFC[a][c] += x
					cntFC[a][c]++
				}
			}
			if !t1[feats[a].id] {
				continue
			}
			for b := a; b < nf; b++ {
				if !t1[feats[b].id] {
					continue
				}
				if x := corrPair(ranks[a], ranks[b]); !math.IsNaN(x) {
					sumFF[a][b] += x
					cntFF[a][b]++
				}
			}
		}
	}
	idx := []int{}
	for k, sp := range feats {
		if t1[sp.id] {
			idx = append(idx, k)
		}
	}
	m := len(idx)
	R := make([][]float64, m)
	for a := range R {
		R[a] = make([]float64, m)
		for b := range R[a] {
			x, y := idx[a], idx[b]
			if x > y {
				x, y = y, x
			}
			if cntFF[x][y] > 0 {
				R[a][b] = sumFF[x][y] / float64(cntFF[x][y])
			}
			if a == b {
				R[a][b] = 1
			}
		}
	}
	ev := jacobiEigen(R)
	mean := 0.0
	for _, e := range ev {
		mean += e
	}
	mean /= float64(m)
	v := 0.0
	for _, e := range ev {
		v += (e - mean) * (e - mean)
	}
	v /= float64(m - 1)
	mEff := 1 + float64(m-1)*(1-v/float64(m))
	thr := math.Max(3.0, normQuantile(0.05/mEff))
	fmt.Fprintf(w, "prep: M %d Tier 1 feature rows, Nyholt M_eff %.3f, threshold t >= %.3f\n", m, mEff, thr)
	fz := frozen{MEff: mEff, M: m, Threshold: thr, Drop: map[string][]int{}}
	for a, sp := range feats {
		var drops []int
		for c := 0; c < nCtrl; c++ {
			if cntFC[a][c] > 0 && math.Abs(sumFC[a][c]/float64(cntFC[a][c])) > 0.7 {
				drops = append(drops, c)
			}
		}
		if len(drops) > 0 {
			fz.Drop[sp.id] = drops
			fmt.Fprintf(w, "drop controls for %s: %v\n", sp.id, drops)
		}
	}
	if out != "" {
		b, _ := json.MarshalIndent(fz, "", "  ")
		if err := os.WriteFile(out, b, 0o644); err != nil {
			log.Fatalf("flowscan: %v", err)
		}
		fmt.Fprintf(w, "frozen file written: %s\n", out)
	}
}

func main() {
	dir := flag.String("dir", "data/algopack", "AlgoPack directory")
	mode := flag.String("mode", "verdict", "prep, explore or verdict")
	from := flag.String("from", "", "first loaded day")
	till := flag.String("till", "", "last loaded day")
	evalLo := flag.String("eval-from", "", "first feature date")
	evalHi := flag.String("eval-to", "", "last feature date")
	split := flag.String("split", "", "first date of the second half")
	lagFrom := flag.String("lag-from", "2025-01-01", "SYSTIME rule applies to days from this date")
	lag := flag.Int("lag", 0, "SYSTIME eligibility lag in days, 0 disables")
	div := flag.String("div", "data/dividends.jsonl", "dividend calendar")
	frozenPath := flag.String("frozen", "docs/plans/20261023-algopack-100-hypotheses.frozen.json", "frozen M_eff, threshold and control lists")
	out := flag.String("out", "", "prep: write the frozen file here")
	onlyFlag := flag.String("only", "", "comma-separated row ids")
	universe := flag.Int("universe", 100, "admitted universe size")
	tm := flag.Float64("tm", 3.0, "mkt: frozen threshold T_M")
	econKind := flag.String("econ-kind", "h5", "econ mode: h5 or h10")
	workers := flag.Int("workers", runtime.NumCPU(), "parallel day loaders")
	flag.Parse()

	switch *mode {
	case "prep", "explore":
		if *from == "" {
			*from, *till, *evalLo, *evalHi, *split = "2020-01-03", "2024-12-31", "2020-04-01", "2024-12-31", "2022-07-01"
		}
	case "mktprep":
		if *from == "" {
			*from, *till, *evalLo, *evalHi, *split = "2023-08-01", "2024-12-31", "2023-12-01", "2024-12-31", "2024-07-01"
		}
	case "export", "matrix":
		if *from == "" {
			*from, *till, *evalLo, *evalHi, *split = "2020-01-03", "2026-10-21", "2020-04-01", "2026-10-16", "2025-01-01"
			*lag = 30
		}
	case "volfc", "spreadfc", "econ", "mkt":
		if *from == "" {
			start := "2024-09-01"
			if *mode == "mkt" {
				start = "2024-06-01"
			}
			*from, *till, *evalLo, *evalHi, *split = start, "2026-10-21", "2025-01-03", "2026-10-16", "2026-01-05"
			*lag = 30
		}
	case "verdict":
		if *from == "" {
			*from, *till, *evalLo, *evalHi, *split = "2024-09-01", "2026-10-21", "2025-01-01", "2026-10-16", "2026-01-01"
			*lag = 30
		}
	}
	admN = *universe
	if *mode == "export" {
		admN = 300
	}
	switch *mode {
	case "export", "matrix", "volfc", "spreadfc", "econ", "mkt", "mktprep":
		secAlias = map[string]string{"YNDX": "YDEX", "TCSG": "T"}
	}
	days := listDays(*dir, *from, *till)
	if len(days) == 0 {
		log.Fatal("flowscan: no days")
	}
	log.Printf("flowscan: %s mode, %d day files", *mode, len(days))
	results := loadAll(*dir, days, *workers, *lagFrom, *lag)
	p := newPanel(results)
	log.Printf("flowscan: panel %d days, %d tickers, %d dividend events", len(p.dates), len(p.tickers), p.loadDividends(*div))
	p.universeReturns()
	win := window{lo: p.dateIndex(*evalLo), hi: p.dateIndex(*evalHi), split: p.dateIndex(*split)}
	if win.hi >= len(p.dates) || p.dates[win.hi] > *evalHi {
		win.hi--
	}
	r := &run{p: p, win: win, w: os.Stdout, mode: *mode}
	if *mode == "prep" {
		r.prep(*out)
		return
	}
	if *mode == "mktprep" {
		r.mktPrep()
		return
	}
	if *mode == "mkt" {
		r.mktRun(*tm)
		return
	}
	if *mode == "econ" {
		kind := kH5
		if *econKind == "h10" {
			kind = kH10
		}
		a2 := rowSpec{id: "A2", tier: 2, sign: 1, kind: kind, fn: col(bA2)}
		X := r.buildX(&a2)
		e := p.evaluate(X, 1, kind, win, nil, tradeOpts{})
		fmt.Printf("econ universe %d %s\n", admN, r.line(&a2, e))
		_, nt := neweyWest(e.netSeries, lagsFor(holdOf(kind)))
		fmt.Printf("econ net per day %s halves %s / %s, net NW t %+.2f\n", pct(e.netAll), pct(e.net[0]), pct(e.net[1]), nt)
		return
	}
	if *mode == "volfc" || *mode == "spreadfc" {
		if *mode == "volfc" {
			r.forecastFamily("B", volTargets(), 0.05)
			for _, tg := range volTargets() {
				r.addedValue(tg, fcFeatures(), p.dateIndex("2024-10-01"))
			}
		} else {
			r.forecastFamily("C", spreadTargets(), 0.05)
			for _, tg := range spreadTargets() {
				r.addedValue(tg, fcFeatures(), p.dateIndex("2024-10-01"))
			}
		}
		return
	}
	if *mode == "matrix" {
		r.exportMatrix(*out)
		return
	}
	if *mode == "export" {
		r.exportBlock(*out, *lagFrom)
		return
	}
	b, err := os.ReadFile(*frozenPath)
	if err != nil {
		log.Fatalf("flowscan: frozen file: %v", err)
	}
	if err := json.Unmarshal(b, &r.fz); err != nil {
		log.Fatalf("flowscan: frozen file: %v", err)
	}
	r.thr = r.fz.Threshold
	only := map[string]bool{}
	for _, id := range strings.Split(*onlyFlag, ",") {
		if id != "" {
			only[id] = true
		}
	}
	fmt.Fprintf(r.w, "flowscan %s: feature dates %s..%s (%d), halves split %s, threshold %.3f, M_eff %.2f\n", *mode, p.dates[win.lo], p.dates[win.hi], win.hi-win.lo+1, p.dates[win.split], r.thr, r.fz.MEff)
	r.runAll(only)
}
