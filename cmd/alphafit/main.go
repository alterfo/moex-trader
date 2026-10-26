package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"sort"
	"strings"
)

type frozen struct {
	LambdaRidge float64 `json:"lambda_ridge"`
	LambdaEN    float64 `json:"lambda_en"`
	Alpha       float64 `json:"alpha"`
	Rate        float64 `json:"trade_rate"`
}

type pipe struct {
	ds     *dataset
	nf, p  int
	a2     int
	gr     []*gram
	grPerm map[int64][]*gram
}

func newPipe(ds *dataset) *pipe {
	pp := &pipe{ds: ds, nf: len(ds.names)}
	pp.p = pp.nf + 3
	for i, n := range ds.names {
		if n == "A2" {
			pp.a2 = i
		}
	}
	pp.gr = make([]*gram, len(ds.dates))
	pp.grPerm = map[int64][]*gram{}
	for d := range ds.dates {
		g := newGram(pp.p)
		for _, x := range ds.byDate[d] {
			if !math.IsNaN(x.y10) {
				g.addRow(x.feat, x.y10)
			}
		}
		pp.gr[d] = g
	}
	return pp
}

func permFor(seed int64, d, n int) []int {
	rng := rand.New(rand.NewSource(seed*1000003 + int64(d)))
	return rng.Perm(n)
}

func (pp *pipe) permGram(seed int64) []*gram {
	if g, ok := pp.grPerm[seed]; ok {
		return g
	}
	out := make([]*gram, len(pp.ds.dates))
	for d := range pp.ds.dates {
		rows := pp.ds.byDate[d]
		perm := permFor(seed, d, len(rows))
		g := newGram(pp.p)
		for i, x := range rows {
			if !math.IsNaN(x.y10) {
				g.addRow(rows[perm[i]].feat, x.y10)
			}
		}
		out[d] = g
	}
	pp.grPerm[seed] = out
	return out
}

func sumGram(gs []*gram, lo, hi int) *gram {
	m := newGram(gs[0].p)
	for d := lo; d <= hi; d++ {
		m.merge(gs[d])
	}
	return m
}

type models struct {
	ridge, en, a2 []float64
}

func (pp *pipe) fit(gs []*gram, lo, hi int, fz frozen) models {
	m := sumGram(gs, lo, hi)
	all := make([]int, pp.p)
	for i := range all {
		all[i] = i
	}
	return models{
		ridge: solveRidge(m, fz.LambdaRidge, all),
		en:    solveEN(m, fz.LambdaEN, fz.Alpha),
		a2:    solveRidge(m, 0.001, []int{pp.a2, pp.nf, pp.nf + 1, pp.nf + 2}),
	}
}

type outSeries struct {
	ic       map[string][]float64
	netD     map[string][]float64
	grossD   map[string][]float64
	netGP    map[string][]float64
	netD25   map[string][]float64
	turnD    map[string][]float64
	grossGP  map[string][]float64
	turnGP   map[string][]float64
	netGP25  map[string][]float64
	glongGP  map[string][]float64
	gshortGP map[string][]float64
	icT      map[string][][]float64
}

func newOut(nd int, names []string) *outSeries {
	o := &outSeries{ic: map[string][]float64{}, netD: map[string][]float64{}, grossD: map[string][]float64{}, netGP: map[string][]float64{}, netD25: map[string][]float64{}, turnD: map[string][]float64{}, grossGP: map[string][]float64{}, turnGP: map[string][]float64{}, netGP25: map[string][]float64{}, glongGP: map[string][]float64{}, gshortGP: map[string][]float64{}, icT: map[string][][]float64{}}
	for _, n := range names {
		for _, m := range []map[string][]float64{o.ic, o.netD, o.grossD, o.netGP, o.netD25, o.turnD, o.grossGP, o.turnGP, o.netGP25, o.glongGP, o.gshortGP} {
			v := make([]float64, nd)
			for i := range v {
				v[i] = nan
			}
			m[n] = v
		}
		o.icT[n] = [][]float64{make([]float64, nd), make([]float64, nd), make([]float64, nd)}
		for k := 0; k < 3; k++ {
			for i := range o.icT[n][k] {
				o.icT[n][k][i] = nan
			}
		}
	}
	return o
}

func icOf(rows []*row, pred []float64) float64 {
	var a, b []float64
	for i, x := range rows {
		if !math.IsNaN(pred[i]) && !math.IsNaN(x.y10) {
			a = append(a, pred[i])
			b = append(b, x.y10)
		}
	}
	if len(a) < 30 {
		return nan
	}
	return pearson(rankScale(a), b)
}

type spec struct {
	lo, hi string
}

func (pp *pipe) dateRange(lo, hi string) (int, int) {
	a := sort.SearchStrings(pp.ds.dates, lo)
	b := sort.SearchStrings(pp.ds.dates, hi)
	if b >= len(pp.ds.dates) || pp.ds.dates[b] > hi {
		b--
	}
	return a, b
}

const borrow17 = 0.17 / 252
const borrow25 = 0.25 / 252

type runner struct {
	pp        *pipe
	fz        frozen
	rate      float64
	names     []string
	out       *outSeries
	books     map[string]*book
	booksGP   map[string]*book
	books25   map[string]*book
	booksGP25 map[string]*book
	keep      map[int][]float64
}

func newRunner(pp *pipe, fz frozen, names []string) *runner {
	r := &runner{pp: pp, fz: fz, names: names, out: newOut(len(pp.ds.dates), names), books: map[string]*book{}, booksGP: map[string]*book{}, books25: map[string]*book{}, booksGP25: map[string]*book{}}
	for _, n := range names {
		r.books[n] = newBook(1)
		r.books25[n] = newBook(1)
		r.booksGP[n] = newBook(fz.Rate)
		r.booksGP25[n] = newBook(fz.Rate)
	}
	return r
}

func (r *runner) day(d int, preds map[string][]float64) {
	rows := r.pp.ds.byDate[d]
	bench := 0.0
	if longOnly {
		for _, x := range rows {
			if !math.IsNaN(x.u1) {
				bench = x.u1
				break
			}
		}
	}
	if neutralMode == "sector" {
		for k, v := range preds {
			preds[k] = sectorNeutral(rows, v)
		}
	}
	for _, n := range r.names {
		pr := preds[n]
		r.out.ic[n][d] = icOf(rows, pr)
		aim := aimWeights(rows, pr)
		pl := r.books[n].step(rows, aim)
		if longOnly {
			pl.gross -= pl.wlong * bench
		}
		r.out.grossD[n][d] = pl.gross
		r.out.netD[n][d] = pl.gross - pl.cost - pl.short*borrow17
		r.out.turnD[n][d] = pl.turn
		pl25 := r.books25[n].step(rows, aim)
		if longOnly {
			pl25.gross -= pl25.wlong * bench
		}
		r.out.netD25[n][d] = pl25.gross - pl25.cost - pl25.short*borrow25
		plg := r.booksGP[n].step(rows, aim)
		if longOnly {
			plg.gross -= plg.wlong * bench
		}
		r.out.netGP[n][d] = plg.gross - plg.cost - plg.short*borrow17
		r.out.glongGP[n][d] = plg.glong
		r.out.gshortGP[n][d] = plg.gshort
		for k := 0; k < 3; k++ {
			r.out.icT[n][k][d] = icOfRankThird(rows, pr, k)
		}
		r.out.grossGP[n][d] = plg.gross
		r.out.turnGP[n][d] = plg.turn
		plg25 := r.booksGP25[n].step(rows, aim)
		if longOnly {
			plg25.gross -= plg25.wlong * bench
		}
		r.out.netGP25[n][d] = plg25.gross - plg25.cost - plg25.short*borrow25
	}
}

func (r *runner) block(lo, hi int, trainFrom int, seeds []int64, withBook bool) {
	pp := r.pp
	trainHi := lo - (horizon + 1)
	if trainHi <= trainFrom {
		return
	}
	mReal := pp.fit(pp.gr, trainFrom, trainHi, r.fz)
	mPerm := map[int64]models{}
	for _, sd := range seeds {
		mPerm[sd] = pp.fit(pp.permGram(sd), trainFrom, trainHi, r.fz)
	}
	for d := lo; d <= hi; d++ {
		rows := pp.ds.byDate[d]
		n := len(rows)
		preds := map[string][]float64{}
		get := func(name string) []float64 {
			if preds[name] == nil {
				preds[name] = make([]float64, n)
			}
			return preds[name]
		}
		for i, x := range rows {
			pr, pe, pa := predict(mReal.ridge, x.feat), predict(mReal.en, x.feat), predict(mReal.a2, x.feat)
			get("ridge")[i], get("en")[i], get("blend")[i], get("a2")[i] = pr, pe, 0.5*(pr+pe), pa
		}
		for _, sd := range seeds {
			perm := permFor(sd+7, d, n)
			nm := fmt.Sprintf("plc%d", sd)
			for i := range rows {
				xs := rows[perm[i]].feat
				m := mPerm[sd]
				get(nm)[i] = 0.5 * (predict(m.ridge, xs) + predict(m.en, xs))
			}
		}
		if withBook {
			r.day(d, preds)
		} else {
			for _, nm := range r.names {
				r.out.ic[nm][d] = icOf(rows, preds[nm])
			}
			if r.keep != nil {
				r.keep[d] = preds["blend"]
			}
		}
	}
}

func main() {
	mode := flag.String("mode", "explore", "explore or verdict")
	mat := flag.String("matrix", "work/matrix.csv", "flowscan matrix CSV")
	frz := flag.String("frozen", "work/alphafit-frozen.json", "frozen hyperparameters")
	models := flag.String("models", "work/alphafit-models.json", "frozen model coefficients")
	outPath := flag.String("out", "work/alphafit-series.csv", "dump: daily series CSV")
	zero := flag.String("zero", "", "comma list of feature names set to the missing value after ranking")
	extPath := flag.String("ext", "", "flowscan matrix extension CSV (f20, f40, u20, u40, ovn1, idy1)")
	hz := flag.Int("horizon", 10, "target horizon in trading days: 10, 20 or 40 (20 and 40 need -ext)")
	cm := flag.String("cost", "flat", "cost model: flat (0.15% a side), bbo (0.05% plus half the BBO spread), mio (0.05% plus half the 1 mln RUB spread)")
	em := flag.String("entry", "close", "entry: close (trade at the close of the signal day) or open (change in weights earns the next day intraday return only)")
	divPath := flag.String("div", "", "dividend calendar: adds DivFlow and the A2PR network column (wave 3b)")
	stage := flag.String("stage", "explore", "momsleeve: explore (freeze the rates on the exploration sample) or verdict")
	rateL := flag.Float64("ratel", 0.05, "momsleeve verdict: frozen rate of the long-only sleeve")
	rateS := flag.Float64("rates", 0.05, "momsleeve verdict: frozen rate of the long-short sleeve")
	plcDump := flag.String("plcdump", "", "s3diff: placebo dump CSV")
	sigKind := flag.String("sig", "mom256", "momsleeve: mom256 or mom121")
	momF := flag.String("mom", "", "add Mom120 and Mom256 (sums of abnormal returns over 120 and 256 observations) as features: on, or placebo (within-date permutation)")
	with3b := flag.Bool("with3b", false, "keep the four wave-3b base columns (ProfP2, ProfP3, VolEnt, DepEnt) as features")
	aucInc := flag.Bool("aucinc", false, "keep AucA1 as a feature (valid only with next-open entry)")
	libWin := flag.String("libwin", "realtime", "lib2: realtime (the verdict, run once) or explore (debug on the regenerated window, not evidence)")
	dsrCol := flag.String("col", "net_gp_blend", "dsr: column of the dump CSV given by -base")
	neutral := flag.String("neutral", "none", "none or sector: demean the prediction within frozen sector lists before building the book")
	candPath := flag.String("candfile", "", "libext: CSV date,secid,candidate columns")
	basePath := flag.String("base", "", "compare: baseline dump CSV")
	extDump := flag.String("extdump", "", "compare: extended dump CSV")
	bookKind := flag.String("book", "longshort", "longshort or longonly (top quintile long only, abnormal against the equal-weight universe mean, no carry)")
	fromDate := flag.String("from", "2026-10-21", "forward: first feature date")
	flag.Parse()
	if *mode == "s3diff" {
		s3Diff(*basePath, *extDump, *plcDump)
		return
	}
	if *mode == "compare" {
		compareDumps(*basePath, *extDump)
		return
	}
	if *mode == "dsr" {
		deflatedSharpe(*basePath, *dsrCol, []int{1, 10, 50, 100, 300, 800})
		betaDiagnostic(*basePath, *dsrCol)
		return
	}
	libWindow = *libWin
	candFile = *candPath
	neutralMode = *neutral
	longOnly = *bookKind == "longonly"
	if *aucInc {
		delete(excludeCols, "AucA1")
	}
	if *with3b {
		for _, c := range []string{"ProfP2", "ProfP3", "VolEnt", "DepEnt"} {
			delete(excludeCols, c)
		}
	}
	ds, err := load(*mat)
	if err != nil {
		log.Fatal(err)
	}
	horizon, costMode, entryMode = *hz, *cm, *em
	prepCosts(ds)
	if *extPath != "" {
		if err := loadExt(ds, *extPath); err != nil {
			log.Fatal(err)
		}
	} else if horizon != 10 || entryMode == "open" {
		log.Fatal("horizon other than 10 and open entry need -ext")
	}
	ds.derived()
	if *momF != "" {
		momPlacebo = *momF == "placebo"
		ds.addMomentum()
	}
	if *divPath != "" {
		ds.addNetwork()
		if err := ds.addDivFlow(*divPath); err != nil {
			log.Fatal(err)
		}
	}
	ds.build()
	if *zero != "" {
		zeroFeatures(ds, strings.Split(*zero, ","))
	}
	pp := newPipe(ds)
	log.Printf("alphafit: %d rows, %d dates, %d AlgoPack features plus 3 controls", len(ds.rows), len(ds.dates), pp.nf)
	switch *mode {
	case "explore":
		explore(pp, *frz)
	case "verdict":
		verdict(pp, *frz)
	case "probe":
		probe(pp)
	case "freeze":
		freezeModels(pp, *frz, *models)
	case "lib2":
		lib2(pp, *frz)
	case "momdiag":
		momentumDiagnostic(pp, candFile, *outPath)
	case "momsleeve":
		momSleeve(pp, *basePath, *sigKind, *stage, *rateL, *rateS)
	case "tfmb":
		tfmB(pp, *frz)
	case "libext":
		libExt(pp, *frz)
	case "lib3":
		libMode = "lib3"
		lib2(pp, *frz)
	case "dump":
		dump(pp, *frz, *outPath)
	case "shadow":
		shadowScores(pp, *models, *fromDate, *outPath)
	case "forward":
		forward(pp, *models, *fromDate)
	}
}

func explore(pp *pipe, frzPath string) {
	t0 := sort.SearchStrings(pp.ds.dates, "2020-04-01")
	blocks := [][2]string{{"2022-07-01", "2022-12-30"}, {"2023-01-02", "2023-06-30"}, {"2023-07-03", "2023-12-29"}, {"2024-01-01", "2024-06-28"}, {"2024-07-01", "2024-12-30"}}
	type res struct {
		l, a float64
		ic   float64
	}
	var ridgeRes, enRes []res
	for _, l := range []float64{0.003, 0.01, 0.03, 0.1, 0.3, 1} {
		fz := frozen{LambdaRidge: l, LambdaEN: 0.001, Alpha: 0.5, Rate: 1}
		r := newRunner(pp, fz, []string{"ridge"})
		for _, b := range blocks {
			lo, hi := pp.dateRange(b[0], b[1])
			r.block(lo, hi, t0, nil, false)
		}
		m := mean(clean(r.out.ic["ridge"]))
		ridgeRes = append(ridgeRes, res{l, 0, m})
		fmt.Printf("explore ridge lambda %.3f mean IC %.5f\n", l, m)
	}
	for _, l := range []float64{0.0002, 0.0005, 0.001, 0.002, 0.004} {
		fz := frozen{LambdaRidge: 0.1, LambdaEN: l, Alpha: 0.5, Rate: 1}
		r := newRunner(pp, fz, []string{"en"})
		for _, b := range blocks {
			lo, hi := pp.dateRange(b[0], b[1])
			r.block(lo, hi, t0, nil, false)
		}
		m := mean(clean(r.out.ic["en"]))
		enRes = append(enRes, res{l, 0.5, m})
		fmt.Printf("explore elastic-net lambda %.4f alpha 0.5 mean IC %.5f\n", l, m)
	}
	sort.Slice(ridgeRes, func(a, b int) bool { return ridgeRes[a].ic > ridgeRes[b].ic })
	sort.Slice(enRes, func(a, b int) bool { return enRes[a].ic > enRes[b].ic })
	fz := frozen{LambdaRidge: ridgeRes[0].l, LambdaEN: enRes[0].l, Alpha: 0.5, Rate: 1}
	fmt.Printf("explore chosen ridge lambda %.3f, elastic-net lambda %.4f\n", fz.LambdaRidge, fz.LambdaEN)
	var best float64 = -1e9
	for _, a := range []float64{1, 0.5, 0.33, 0.2, 0.1, 0.05} {
		f2 := fz
		f2.Rate = a
		r := newRunner(pp, f2, []string{"blend", "a2"})
		for _, b := range blocks {
			lo, hi := pp.dateRange(b[0], b[1])
			r.block(lo, hi, t0, nil, true)
		}
		nb := mean(clean(r.out.netGP["blend"]))
		na := mean(clean(r.out.netGP["a2"]))
		fmt.Printf("explore trade rate %.2f: blend net/day %.5f%% (gross %.5f%%), a2 net/day %.5f%%, IC blend %.5f a2 %.5f\n", a, nb*100, mean(clean(r.out.grossD["blend"]))*100, na*100, mean(clean(r.out.ic["blend"])), mean(clean(r.out.ic["a2"])))
		if nb > best {
			best = nb
			fz.Rate = a
		}
	}
	fmt.Printf("explore chosen trade rate %.2f\n", fz.Rate)
	b, _ := json.MarshalIndent(fz, "", "  ")
	os.WriteFile(frzPath, b, 0o644)
	fmt.Println("frozen written", frzPath)
}

func report(name string, x []float64, split int, draws int) {
	c := clean(x)
	m, t := neweyWest(c, 10)
	lo, hi := bootCI(x, draws, 20261022)
	h0, h1 := halves(x, split)
	fmt.Printf("%-28s mean %+.5f NW10 t %+.2f boot95 [%+.5f, %+.5f] halves %+.5f / %+.5f n %d\n", name, m, t, lo, hi, h0, h1, len(c))
}

func verdict(pp *pipe, frzPath string) {
	raw, err := os.ReadFile(frzPath)
	if err != nil {
		log.Fatal(err)
	}
	var fz frozen
	json.Unmarshal(raw, &fz)
	t0 := sort.SearchStrings(pp.ds.dates, "2020-04-01")
	blocks := [][2]string{{"2025-01-03", "2025-03-31"}, {"2025-04-01", "2025-06-30"}, {"2025-07-01", "2025-09-30"}, {"2025-10-01", "2025-12-31"}, {"2026-01-01", "2026-03-31"}, {"2026-04-01", "2026-06-30"}, {"2026-07-01", "2026-10-16"}}
	seeds := []int64{20261023, 20261024, 20261025, 20261026, 20261027}
	names := []string{"ridge", "en", "blend", "a2"}
	for _, sd := range seeds {
		names = append(names, fmt.Sprintf("plc%d", sd))
	}
	r := newRunner(pp, fz, names)
	for _, b := range blocks {
		lo, hi := pp.dateRange(b[0], b[1])
		r.block(lo, hi, t0, seeds, true)
	}
	rtFrom := sort.SearchStrings(pp.ds.dates, "2025-01-03")
	rt := newRunner(pp, fz, []string{"ridge", "en", "blend", "a2"})
	for _, b := range blocks[2:] {
		lo, hi := pp.dateRange(b[0], b[1])
		rt.block(lo, hi, rtFrom, nil, true)
	}
	split := sort.SearchStrings(pp.ds.dates, "2026-01-05")
	const draws = 5000
	fmt.Println("== descriptive arm: training on realtime rows only (blocks 3-7, 2025-07-01 on)")
	for _, n := range []string{"blend", "a2"} {
		report("IC rt-only "+n, rt.out.ic[n], split, draws)
		report("net/day rt-only "+n, rt.out.netD[n], split, draws)
	}
	fmt.Printf("verdict: frozen ridge %.3f en %.4f rate %.2f\n", fz.LambdaRidge, fz.LambdaEN, fz.Rate)
	fmt.Println("== daily IC against the 10-day abnormal rank")
	for _, n := range []string{"ridge", "en", "blend", "a2"} {
		report("IC "+n, r.out.ic[n], split, draws)
	}
	report("IC blend minus a2", diff(r.out.ic["blend"], r.out.ic["a2"]), split, draws)
	pos := 0
	for _, sd := range seeds {
		nm := fmt.Sprintf("plc%d", sd)
		report("IC "+nm, r.out.ic[nm], split, draws)
		report("IC blend minus "+nm, diff(r.out.ic["blend"], r.out.ic[nm]), split, draws)
		if mean(clean(diff(r.out.ic["blend"], r.out.ic[nm]))) > 0 {
			pos++
		}
	}
	fmt.Printf("placebo seeds with blend IC above the placebo: %d of 5\n", len(seeds)-(len(seeds)-pos))
	fmt.Println("== dollar-neutral top-minus-bottom quintile book, inverse-vol legs, 0.15% per side, borrow 17% a year (daily rebalanced)")
	for _, n := range []string{"blend", "ridge", "en", "a2"} {
		report("net/day "+n, r.out.netD[n], split, draws)
		fmt.Printf("  %s gross %+.5f%% turnover/day %.3f net@25%%borrow %+.5f%% annualised net %.1f%%\n", n, mean(clean(r.out.grossD[n]))*100, mean(clean(r.out.turnD[n])), mean(clean(r.out.netD25[n]))*100, mean(clean(r.out.netD[n]))*252*100)
	}
	for _, sd := range seeds {
		nm := fmt.Sprintf("plc%d", sd)
		report("net/day "+nm, r.out.netD[nm], split, draws)
	}
	fmt.Println("== Garleanu-Pedersen style partial adjustment (rate fixed on exploration)")
	for _, n := range []string{"blend", "a2"} {
		report("net/day GP "+n, r.out.netGP[n], split, draws)
		report("GP minus daily "+n, diff(r.out.netGP[n], r.out.netD[n]), split, draws)
		fmt.Printf("  GP %s gross %+.5f%% turnover/day %.3f net@17%% %+.5f%% net@25%% %+.5f%% annualised net@17%% %.1f%% @25%% %.1f%%\n", n, mean(clean(r.out.grossGP[n]))*100, mean(clean(r.out.turnGP[n])), mean(clean(r.out.netGP[n]))*100, mean(clean(r.out.netGP25[n]))*100, mean(clean(r.out.netGP[n]))*252*100, mean(clean(r.out.netGP25[n]))*252*100)
		report("net/day GP@25% "+n, r.out.netGP25[n], split, draws)
	}
}

func probe(pp *pipe) {
	ds := pp.ds
	lo, hi := pp.dateRange("2022-07-01", "2024-12-30")
	fmt.Println("== single-feature mean daily IC on the exploration window (feature rank vs 10-day abnormal rank)")
	type fi struct {
		name string
		ic   float64
	}
	var res []fi
	for k := 0; k < pp.nf+3; k++ {
		var ics []float64
		for d := lo; d <= hi; d++ {
			rows := ds.byDate[d]
			a := make([]float64, 0, len(rows))
			b := make([]float64, 0, len(rows))
			for _, x := range rows {
				if !math.IsNaN(x.y10) {
					a = append(a, float64(x.feat[k]))
					b = append(b, x.y10)
				}
			}
			if len(a) >= 30 {
				if v := pearson(a, b); !math.IsNaN(v) {
					ics = append(ics, v)
				}
			}
		}
		name := "ctrl"
		if k < pp.nf {
			name = ds.names[k]
		} else {
			name = fmt.Sprintf("ctrl%d", k-pp.nf)
		}
		res = append(res, fi{name, mean(ics)})
	}
	sort.Slice(res, func(a, b int) bool { return math.Abs(res[a].ic) > math.Abs(res[b].ic) })
	for i := 0; i < 20 && i < len(res); i++ {
		fmt.Printf("  %-14s IC %+.4f\n", res[i].name, res[i].ic)
	}
	fmt.Println("== same walk-forward with every feature lagged by one and by five dates (stale features), exploration blocks")
	for _, lag := range []int{1, 5} {
		saved := make([][]float32, 0, len(ds.rows))
		for _, x := range ds.rows {
			saved = append(saved, x.feat)
		}
		prev := map[string][]float32{}
		_ = prev
		hist := map[string]map[int][]float32{}
		for d := range ds.byDate {
			for _, x := range ds.byDate[d] {
				if hist[x.secid] == nil {
					hist[x.secid] = map[int][]float32{}
				}
				hist[x.secid][d] = x.feat
			}
		}
		for d := range ds.byDate {
			for _, x := range ds.byDate[d] {
				if old, ok := hist[x.secid][d-lag]; ok {
					x.feat = old
				} else {
					x.feat = make([]float32, pp.p)
				}
			}
		}
		pp2 := newPipe(ds)
		t0 := sort.SearchStrings(ds.dates, "2020-04-01")
		fz := frozen{LambdaRidge: 0.003, LambdaEN: 0.001, Alpha: 0.5, Rate: 1}
		r := newRunner(pp2, fz, []string{"blend", "a2"})
		for _, b := range [][2]string{{"2022-07-01", "2022-12-30"}, {"2023-01-02", "2023-06-30"}, {"2023-07-03", "2023-12-29"}, {"2024-01-01", "2024-06-28"}, {"2024-07-01", "2024-12-30"}} {
			l, h := pp2.dateRange(b[0], b[1])
			r.block(l, h, t0, nil, false)
		}
		fmt.Printf("  lag %d: IC blend %.5f a2 %.5f\n", lag, mean(clean(r.out.ic["blend"])), mean(clean(r.out.ic["a2"])))
		for i, x := range ds.rows {
			x.feat = saved[i]
		}
	}
}

func dump(pp *pipe, frzPath, outPath string) {
	raw, err := os.ReadFile(frzPath)
	if err != nil {
		log.Fatal(err)
	}
	var fz frozen
	json.Unmarshal(raw, &fz)
	t0 := sort.SearchStrings(pp.ds.dates, "2020-04-01")
	blocks := [][2]string{{"2025-01-03", "2025-03-31"}, {"2025-04-01", "2025-06-30"}, {"2025-07-01", "2025-09-30"}, {"2025-10-01", "2025-12-31"}, {"2026-01-01", "2026-03-31"}, {"2026-04-01", "2026-06-30"}, {"2026-07-01", "2026-10-16"}}
	r := newRunner(pp, fz, []string{"blend", "a2"})
	for _, b := range blocks {
		lo, hi := pp.dateRange(b[0], b[1])
		r.block(lo, hi, t0, nil, true)
	}
	f, err := os.Create(outPath)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	vital := []string{"A2", "A2Late", "A2H2", "ImbVol", "LevAsym", "TicketMed", "SpLv", "UnexpA1", "NJ"}
	vIdx := make([]int, len(vital))
	for i, v := range vital {
		vIdx[i] = -1
		for k, n := range pp.ds.names {
			if n == v {
				vIdx[i] = k
			}
		}
	}
	head := "date,ic_blend,ic_a2,net_gp_blend,net_gp_a2,net_gp25_blend,net_d_blend,u10,glong_gp_blend,gshort_gp_blend,ic_t1,ic_t2,ic_t3,u1"
	for _, v := range vital {
		head += ",f_" + v
	}
	fmt.Fprintln(f, head)
	lo, _ := pp.dateRange("2025-01-03", "2026-10-16")
	_, hi := pp.dateRange("2025-01-03", "2026-10-16")
	for d := lo; d <= hi; d++ {
		u := nan
		for _, x := range pp.ds.byDate[d] {
			if !math.IsNaN(x.u10) {
				u = x.u10
				break
			}
		}
		u1d := nan
		for _, x := range pp.ds.byDate[d] {
			if !math.IsNaN(x.u1) {
				u1d = x.u1
				break
			}
		}
		fmt.Fprintf(f, "%s,%g,%g,%g,%g,%g,%g,%g,%g,%g,%g,%g,%g,%g", pp.ds.dates[d], r.out.ic["blend"][d], r.out.ic["a2"][d], r.out.netGP["blend"][d], r.out.netGP["a2"][d], r.out.netGP25["blend"][d], r.out.netD["blend"][d], u, r.out.glongGP["blend"][d], r.out.gshortGP["blend"][d], r.out.icT["blend"][0][d], r.out.icT["blend"][1][d], r.out.icT["blend"][2][d], u1d)
		rows := pp.ds.byDate[d]
		for _, k := range vIdx {
			fv := nan
			if k >= 0 {
				pr := make([]float64, len(rows))
				for i, x := range rows {
					pr[i] = float64(x.feat[k])
				}
				fv = icOf(rows, pr)
			}
			fmt.Fprintf(f, ",%g", fv)
		}
		fmt.Fprintln(f)
	}
}

func icOfRank(rows []*row, pred []float64, lo, hi float64) float64 {
	var sub []*row
	var p []float64
	for i, x := range rows {
		if x.rank > lo && x.rank <= hi {
			sub = append(sub, x)
			p = append(p, pred[i])
		}
	}
	return icOf(sub, p)
}

func zeroFeatures(ds *dataset, names []string) {
	idx := map[int]bool{}
	for k, n := range ds.names {
		for _, z := range names {
			if n == z {
				idx[k] = true
			}
		}
	}
	for _, x := range ds.rows {
		for k := range idx {
			x.feat[k] = 0
		}
	}
}

func icOfRankThird(rows []*row, pred []float64, third int) float64 {
	rmin, rmax := math.Inf(1), math.Inf(-1)
	for _, x := range rows {
		if !math.IsNaN(x.rank) {
			rmin = math.Min(rmin, x.rank)
			rmax = math.Max(rmax, x.rank)
		}
	}
	if rmax < rmin {
		return nan
	}
	size := (rmax - rmin + 1) / 3
	lo := rmin - 1 + float64(third)*size
	return icOfRank(rows, pred, lo, lo+size)
}
