package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"runtime"
	"sort"
	"strings"
)

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
				theta := (m[q][q] - m[p][p]) / (2 * m[p][q])
				t := 1 / (math.Abs(theta) + math.Sqrt(theta*theta+1))
				if theta < 0 {
					t = -t
				}
				c := 1 / math.Sqrt(t*t+1)
				sn := t * c
				for k := 0; k < n; k++ {
					kp, kq := m[k][p], m[k][q]
					m[k][p], m[k][q] = c*kp-sn*kq, sn*kp+c*kq
				}
				for k := 0; k < n; k++ {
					pk, qk := m[p][k], m[q][k]
					m[p][k], m[q][k] = c*pk-sn*qk, sn*pk+c*qk
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

func newCtx(foPath, fxPath string) *ctx {
	fo, err := loadStore(foPath, false)
	if err != nil {
		log.Fatalf("futscan: %v", err)
	}
	fx, err := loadStore(fxPath, true)
	if err != nil {
		log.Fatalf("futscan: %v", err)
	}
	c := &ctx{fo: fo, fx: fx}
	c.foF, c.fxF = buildFeat(fo), buildFeat(fx)
	c.lo, c.hi, c.spl = dateWin(fo)
	c.fxLo, c.fxHi, c.fxSpl = dateWin(fx)
	return c
}

func (c *ctx) prep() {
	names := append([]string{}, featNames...)
	names = append(names, "aBI", "BIxDG", "BIxTZ", "TIxSR")
	k := len(names)
	sum := make([][]float64, k)
	cnt := make([][]int, k)
	for i := range sum {
		sum[i] = make([]float64, k)
		cnt[i] = make([]int, k)
	}
	s := c.fo
	for d := c.lo; d <= c.hi; d++ {
		L := s.liquid(d, 30)
		var gs []int
		for gi, g := range s.groups {
			if L[g] {
				gs = append(gs, gi)
			}
		}
		cols := make([][]float64, k)
		for i, n := range names {
			v := make([]float64, len(gs))
			for j, gi := range gs {
				switch n {
				case "aBI":
					v[j] = math.Abs(c.foF["BI"][gi][d])
				case "BIxDG", "BIxTZ", "TIxSR":
					v[j] = nan
				default:
					v[j] = c.foF[n][gi][d]
				}
			}
			cols[i] = v
		}
		inter := map[string][2]string{"BIxDG": {"BI", "DG"}, "BIxTZ": {"BI", "TZ"}, "TIxSR": {"TI", "SR"}}
		for i, n := range names {
			pair, ok := inter[n]
			if !ok {
				continue
			}
			ia, ib := -1, -1
			for q, m := range names {
				if m == pair[0] {
					ia = q
				}
				if m == pair[1] {
					ib = q
				}
			}
			var ra, rb []float64
			var pos []int
			for q := range gs {
				if !isNaN(cols[ia][q]) && !isNaN(cols[ib][q]) {
					ra, rb, pos = append(ra, cols[ia][q]), append(rb, cols[ib][q]), append(pos, q)
				}
			}
			out := make([]float64, len(gs))
			for q := range out {
				out[q] = nan
			}
			if len(pos) >= 15 {
				sa, sb := rankScale(ra), rankScale(rb)
				for t, q := range pos {
					out[q] = sa[t] * (sb[t] + 0.5)
				}
			}
			cols[i] = out
		}
		for i := 0; i < k; i++ {
			for j := i; j < k; j++ {
				var a, b []float64
				for q := range gs {
					if !isNaN(cols[i][q]) && !isNaN(cols[j][q]) {
						a, b = append(a, cols[i][q]), append(b, cols[j][q])
					}
				}
				if len(a) < 15 {
					continue
				}
				ra, rb := rankScale(a), rankScale(b)
				ma, mb := mean(ra), mean(rb)
				var sab, saa, sbb float64
				for q := range ra {
					sab += (ra[q] - ma) * (rb[q] - mb)
					saa += (ra[q] - ma) * (ra[q] - ma)
					sbb += (rb[q] - mb) * (rb[q] - mb)
				}
				if saa > 0 && sbb > 0 {
					sum[i][j] += sab / math.Sqrt(saa*sbb)
					cnt[i][j]++
				}
			}
		}
	}
	R := make([][]float64, k)
	for i := range R {
		R[i] = make([]float64, k)
		for j := range R[i] {
			x, y := i, j
			if x > y {
				x, y = y, x
			}
			if cnt[x][y] > 0 {
				R[i][j] = sum[x][y] / float64(cnt[x][y])
			}
			if i == j {
				R[i][j] = 1
			}
		}
	}
	ev := jacobiEigen(R)
	mu := 0.0
	for _, e := range ev {
		mu += e
	}
	mu /= float64(k)
	v := 0.0
	for _, e := range ev {
		v += (e - mu) * (e - mu)
	}
	v /= float64(k - 1)
	mEff := 1 + float64(k-1)*(1-v/float64(k))
	thr := math.Max(3.0, normQuantile(0.05/mEff))
	fmt.Printf("prep: window %s..%s, %d feature definitions %v\n", s.dates[c.lo], s.dates[c.hi], k, names)
	for i := range R {
		row := make([]string, k)
		for j := range R[i] {
			row[j] = fmt.Sprintf("%+.2f", R[i][j])
		}
		fmt.Printf("  %-6s %s\n", names[i], strings.Join(row, " "))
	}
	fmt.Printf("prep: M %d, Nyholt M_eff %.3f, threshold t >= %.3f\n", k, mEff, thr)
}

func (c *ctx) scan(thr float64, only string) {
	all := specs()
	type out struct {
		sp rowSpec
		r  res
	}
	var outs []out
	for _, sp := range all {
		if only != "" && !strings.HasPrefix(sp.id, only) {
			continue
		}
		r := c.run(sp)
		outs = append(outs, out{sp, r})
		fmt.Println(r.String(sp))
	}
	idx := make([]int, len(outs))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool {
		pa, pb := outs[idx[a]].r.p, outs[idx[b]].r.p
		if math.IsNaN(pa) {
			return false
		}
		if math.IsNaN(pb) {
			return true
		}
		return pa < pb
	})
	m := len(outs)
	cut := -1
	for k, i := range idx {
		if !math.IsNaN(outs[i].r.p) && outs[i].r.p <= 0.05*float64(k+1)/float64(m) {
			cut = k
		}
	}
	fmt.Printf("\n== Book 1: threshold |t| >= %.3f, registered sign (two-sided rows: either), half coefficients with the registered sign and the pooled sign, BH q=0.05 over %d rows\n", thr, m)
	pass, ecoPass := 0, 0
	for k, i := range idx {
		o := outs[i]
		two := o.sp.kind == "vol" || o.sp.kind == "fxvol" || o.sp.kind == "disp"
		tt := o.r.t * o.sp.sign
		if two {
			tt = math.Abs(o.r.t)
		}
		same := o.r.hc[0]*o.r.hc[1] > 0 && o.r.hc[0]*o.r.coef > 0
		if !two {
			same = same && o.r.coef*o.sp.sign > 0
		}
		ok := tt >= thr && same && k <= cut
		eco := ok && o.r.hasNet && o.r.net[0] > 0 && o.r.net[1] > 0 && o.r.netStress[0] > 0 && o.r.netStress[1] > 0
		if ok {
			pass++
		}
		if eco {
			ecoPass++
		}
		extra := ""
		if o.sp.kind == "vol" {
			extra = fmt.Sprintf(" dR2 %+.4f (material at 0.01: %v)", o.r.dr2, o.r.dr2 >= 0.01)
		}
		fmt.Printf("%s t %+.2f p %.5f BH %v same-sign %v%s -> %s%s\n", o.sp.id, o.r.t, o.r.p, k <= cut, same, extra, map[bool]string{true: "STAT-PASS", false: "fail"}[ok], map[bool]string{true: " +ECON(abnormal net>0 both halves at 0.03% and 0.10% per side)", false: ""}[eco])
	}
	fmt.Printf("Book 1: %d of %d statistically pass, %d also pass the net gate\n", pass, m, ecoPass)
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("futscan: subcommand panel, step0, prep, scan, rows")
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	dir := fs.String("dir", "", "panel: AlgoPack market directory")
	market := fs.String("market", "fo", "panel: fo or fx")
	out := fs.String("out", "", "panel: output CSV")
	foPath := fs.String("fo", "work/panel-fo.csv", "fo panel CSV")
	fxPath := fs.String("fx", "work/panel-fx.csv", "fx panel CSV")
	late := fs.Bool("allow-late", false, "use regenerated (late SYSTIME) rows, exploration only")
	loFlag := fs.String("from", "", "scan window first date")
	hiFlag := fs.String("till", "", "scan window last date")
	thr := fs.Float64("thr", 3.0, "frozen threshold")
	only := fs.String("only", "", "row id prefix")
	blockPath := fs.String("block", "work/block.csv", "flowscan export CSV with equity closes")
	imoexPath := fs.String("imoex", "data/macro/imoex.csv", "IMOEX closes")
	hi2Path := fs.String("hi2", "work/hi2-fo.csv", "fo hi2 panel CSV")
	floor := fs.Int("floor", 80, "minimum eligible dates for a row to be scored (80 is the registered floor; lower only for a descriptive early read)")
	divPath := fs.String("div", "data/dividends.jsonl", "dividend calendar")
	if err := fs.Parse(os.Args[2:]); err != nil {
		log.Fatal(err)
	}
	allowLate = *late
	minDates = *floor
	if *loFlag != "" {
		winLo = *loFlag
	}
	if *hiFlag != "" {
		winHi = *hiFlag
	}
	switch os.Args[1] {
	case "panel":
		if err := buildPanel(*dir, *market, "2020-01-01", "2026-12-31", *out, runtime.NumCPU()); err != nil {
			log.Fatal(err)
		}
	case "step0":
		step0(newCtx(*foPath, *fxPath))
	case "prep":
		newCtx(*foPath, *fxPath).prep()
	case "scan":
		newCtx(*foPath, *fxPath).scan(*thr, *only)
	case "scan2":
		c := newCtx(*foPath, *fxPath)
		e, err := loadEquity(*blockPath, *imoexPath, *divPath)
		if err != nil {
			log.Fatal(err)
		}
		c.scan2(e)
	case "dbg":
		newCtx(*foPath, *fxPath).dbg(fs.Arg(0), fs.Arg(1), fs.Arg(2))
	case "hi2panel":
		if err := buildHi2Panel(*dir, *out); err != nil {
			log.Fatal(err)
		}
	case "prep3", "scan3":
		c := newCtx(*foPath, *fxPath)
		e, err := loadEquity(*blockPath, *imoexPath, *divPath)
		if err != nil {
			log.Fatal(err)
		}
		eb, err := loadEqBlock(*blockPath, *divPath, e)
		if err != nil {
			log.Fatal(err)
		}
		hi, err := loadHi2(*hi2Path)
		if err != nil {
			log.Fatal(err)
		}
		x := newCtx3(c, e, eb, hi)
		if os.Args[1] == "prep3" {
			x.prep()
			x.expcheck("MIX")
			x.expcheck("Si")
			x.expcheck("RTS")
			n := 0
			for _, v := range eb.band2 {
				_ = v
				n++
			}
			fmt.Printf("thin universe: %d distinct names ever in rank 101-300 (excluding top 100 and the production 18)\n", n)
		} else {
			x.scan(*thr)
		}
	case "rows3":
		for _, sp := range append(append(specs3(), specs4()...), specs5()...) {
			st := "test"
			if sp.kind == "deferred" {
				st = "deferred"
			}
			if sp.book == "2" {
				st = "lead"
			}
			fmt.Printf("| %s | %s | %s | %s | %+.0f | h%d | %s |\n", sp.id, st, sp.kind, sp.feat+" "+sp.group, sp.sign, sp.hor, sp.desc)
		}
	case "rows":
		for _, sp := range append(specs(), specs2()...) {
			st := "test"
			if sp.book == "1" && (sp.kind == "fm" || sp.kind == "vol") && (sp.feat == "NOF" || sp.feat == "CI" || sp.feat == "OT") {
				st = "deferred"
			}
			if sp.book == "2" {
				st = "lead"
			}
			fmt.Printf("| %s | %s | %s | %s | %+.0f | h%d | %s |\n", sp.id, st, sp.kind, sp.feat+sp.group, sp.sign, sp.hor, sp.desc)
		}
	default:
		log.Fatalf("futscan: unknown subcommand %q", os.Args[1])
	}
}

func (c *ctx) scan2(e *eqData) {
	type out struct {
		sp rowSpec
		r  res
	}
	var outs []out
	for _, sp := range specs2() {
		r := c.runEq(e, sp)
		outs = append(outs, out{sp, r})
		fmt.Println(r.String(sp))
	}
	idx := make([]int, len(outs))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool {
		pa, pb := outs[idx[a]].r.p, outs[idx[b]].r.p
		if math.IsNaN(pa) {
			return false
		}
		if math.IsNaN(pb) {
			return true
		}
		return pa < pb
	})
	m := len(outs)
	cut := -1
	for k, i := range idx {
		if !math.IsNaN(outs[i].r.p) && outs[i].r.p <= 0.10*float64(k+1)/float64(m) {
			cut = k
		}
	}
	fmt.Printf("\n== Book 2 (spent outcomes, leads only): BH q=0.10 over %d rows, registered sign required\n", m)
	leads := 0
	for k, i := range idx {
		o := outs[i]
		ok := k <= cut && o.r.t*o.sp.sign > 0
		if ok {
			leads++
			fmt.Printf("LEAD %s t %+.2f p %.4f\n", o.sp.id, o.r.t, o.r.p)
		}
	}
	fmt.Printf("Book 2 leads: %d of %d\n", leads, m)
}
