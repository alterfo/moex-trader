package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	fundIndFrom  = "2021-04-08"
	fundIndSplit = "2023-07-01"
	fundRtFrom   = "2025-01-03"
	fundRtTill   = "2026-10-16"
	fundRtSplit  = "2026-01-05"
	fundLabelGap = 11
	bookSwitch   = 0.0010
)

func fundFamily(name string) string {
	return name[:1]
}

func fundConfirmable(name string) bool {
	switch fundFamily(name) {
	case "F", "J", "K", "M":
		return false
	}
	return name != "E06"
}

func plugInLag(x []float64) int {
	n := len(x)
	if n < 30 {
		return 1
	}
	m := mean(x)
	var num, den float64
	for i := 1; i < n; i++ {
		num += (x[i] - m) * (x[i-1] - m)
	}
	for _, v := range x {
		den += (v - m) * (v - m)
	}
	if den <= 0 {
		return 1
	}
	rho := math.Max(-0.95, math.Min(0.95, num/den))
	alpha := 4 * rho * rho / math.Pow(1-rho*rho, 2)
	l := int(math.Ceil(1.1447 * math.Pow(alpha*float64(n), 1.0/3)))
	if l < 1 {
		l = 1
	}
	if l > n/4 {
		l = n / 4
	}
	return l
}

func blockBootInterval(x []float64, block, draws int, seed int64) (float64, float64) {
	n := len(x)
	if n < 2*block {
		return nan, nan
	}
	rng := rand.New(rand.NewSource(seed))
	ms := make([]float64, 0, draws)
	for b := 0; b < draws; b++ {
		s, c := 0.0, 0
		for q := 0; q < (n+block-1)/block; q++ {
			st := rng.Intn(n - block + 1)
			for k := 0; k < block; k++ {
				s += x[st+k]
				c++
			}
		}
		ms = append(ms, s/float64(c))
	}
	sort.Float64s(ms)
	return percentile(ms, 0.005), percentile(ms, 0.995)
}

func invNormTwoSided(p float64) float64 {
	lo, hi := 0.0, 12.0
	for i := 0; i < 80; i++ {
		mid := (lo + hi) / 2
		if math.Erfc(mid/math.Sqrt2) > p {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2
}

func indHiIndex(pp *pipe) int {
	return sortSearch(pp.ds.dates, fundRtFrom) - fundLabelGap
}

func fundMeff(pp *pipe) {
	ds := pp.ds
	t := buildTfmCtx(pp, candFile)
	var fam []int
	for c, n := range t.names {
		if n != "M256" {
			fam = append(fam, c)
		}
	}
	k := len(fam)
	indLo, indHi := sortSearch(ds.dates, fundIndFrom), indHiIndex(pp)
	sum := make([][]float64, k)
	cnt := make([][]float64, k)
	for i := range sum {
		sum[i], cnt[i] = make([]float64, k), make([]float64, k)
	}
	for d := indLo; d <= indHi; d++ {
		rows := ds.byDate[d]
		cols := make([][]float64, k)
		for i := range cols {
			cols[i] = filled(len(rows))
		}
		for r, x := range rows {
			if v := t.cv[ds.dates[d]+"|"+x.secid]; v != nil {
				for i, c := range fam {
					cols[i][r] = v[c]
				}
			}
		}
		rk := make([][]float64, k)
		for i := range cols {
			rk[i] = rankOrNaN(cols[i])
		}
		for i := 0; i < k; i++ {
			for j := i; j < k; j++ {
				var a, b []float64
				for r := range rows {
					if !math.IsNaN(rk[i][r]) && !math.IsNaN(rk[j][r]) {
						a, b = append(a, rk[i][r]), append(b, rk[j][r])
					}
				}
				if len(a) >= 30 {
					if c := pearson(a, b); !math.IsNaN(c) {
						sum[i][j] += c
						cnt[i][j]++
					}
				}
			}
		}
	}
	corr := make([][]float64, k)
	keep := []int{}
	for i := 0; i < k; i++ {
		if cnt[i][i] >= 200 {
			keep = append(keep, i)
		}
	}
	for i := range corr {
		corr[i] = make([]float64, k)
	}
	for _, i := range keep {
		for _, j := range keep {
			a, b := i, j
			if a > b {
				a, b = b, a
			}
			if i == j {
				corr[i][j] = 1
			} else if cnt[a][b] >= 200 {
				corr[i][j] = sum[a][b] / cnt[a][b]
			}
		}
	}
	sub := make([][]float64, len(keep))
	for x, i := range keep {
		sub[x] = make([]float64, len(keep))
		for y, j := range keep {
			sub[x][y] = corr[i][j]
		}
	}
	ev := jacobiEigen(sub)
	m := float64(len(keep))
	mn := mean(ev)
	v := 0.0
	for _, e := range ev {
		v += (e - mn) * (e - mn)
	}
	v /= m - 1
	meff := 1 + (m-1)*(1-v/m)
	z := invNormTwoSided(0.05 / meff)
	fmt.Printf("fundmeff: %d family columns with at least 200 stage-I dates of %d, Nyholt M_eff %.1f, family-wise z for 5%% %.3f, stage-R threshold max(3.0, z) = %.3f (computed from feature correlations only, no outcome used)\n", len(keep), k, meff, z, math.Max(3.0, z))
}

func rankOrNaN(v []float64) []float64 {
	idx := []int{}
	for i, x := range v {
		if !math.IsNaN(x) {
			idx = append(idx, i)
		}
	}
	sub := make([]float64, len(idx))
	for k, i := range idx {
		sub[k] = v[i]
	}
	r := rankScale(sub)
	o := filled(len(v))
	for k, i := range idx {
		o[i] = r[k]
	}
	return o
}

type fundShift struct {
	name string
	col  int
	by   int
}

type fundStat struct {
	n          int
	width      float64
	mean, t    float64
	h1, h2     float64
	byYear     map[string]float64
	tPlug, lo  float64
	hi         float64
	slopeMean  float64
	slopeT     float64
	slopeH1    float64
	slopeH2    float64
	slopeTPlug float64
	slopeLo    float64
	slopeHi    float64
	rm256      float64
}

func seriesInference(x []float64) (m, t, tPlug, lo, hi float64) {
	v := clean(x)
	m, t = neweyWest(v, 10)
	_, tPlug = neweyWest(v, plugInLag(v))
	lo, hi = blockBootInterval(v, 60, 5000, 20261101)
	return
}

func fundTest(pp *pipe, frzPath, macroDir, basePath string, thr float64) {
	if thr < 3.0 {
		fatal(fmt.Errorf("fundtest needs -thr from fundmeff (the family-wise bar), got %v", thr))
	}
	ds := pp.ds
	t := buildTfmCtx(pp, candFile)
	nc := len(t.names)
	raw, err := readFile(frzPath)
	if err != nil {
		fatal(err)
	}
	var fz frozen
	unmarshal(raw, &fz)

	indLo, indHi := sortSearch(ds.dates, fundIndFrom), indHiIndex(pp)
	indSplit := sortSearch(ds.dates, fundIndSplit) - indLo
	indIC := make([][]float64, nc)
	indW := make([]float64, nc)
	for c := range indIC {
		indIC[c] = filled(indHi - indLo + 1)
	}
	for d := indLo; d <= indHi; d++ {
		rows := ds.byDate[d]
		cvs := make([][]float64, len(rows))
		for i, x := range rows {
			cvs[i] = t.cv[ds.dates[d]+"|"+x.secid]
		}
		for c := 0; c < nc; c++ {
			var s, y []float64
			for i, x := range rows {
				if cvs[i] == nil || math.IsNaN(cvs[i][c]) || math.IsNaN(x.y10) {
					continue
				}
				s, y = append(s, cvs[i][c]), append(y, x.y10)
			}
			if len(s) >= 30 {
				indIC[c][d-indLo] = pearson(rankScale(s), y)
				indW[c] += float64(len(s))
			}
		}
	}

	t0 := sortSearch(ds.dates, "2020-04-01")
	r := newRunner(pp, fz, []string{"blend"})
	r.keep = map[int][]float64{}
	blocks := [][2]string{{"2025-01-03", "2025-03-31"}, {"2025-04-01", "2025-06-30"}, {"2025-07-01", "2025-09-30"}, {"2025-10-01", "2025-12-31"}, {"2026-01-01", "2026-03-31"}, {"2026-04-01", "2026-06-30"}, {"2026-07-01", "2026-10-16"}}
	for _, b := range blocks {
		lo, hi := pp.dateRange(b[0], b[1])
		r.block(lo, hi, t0, nil, false)
	}
	lo, hi := pp.dateRange(fundRtFrom, fundRtTill)
	split := sortSearch(ds.dates, fundRtSplit) - lo
	nd := hi - lo + 1
	shifts := []fundShift{{name: "A01", by: 260}, {name: "A01", by: 380}, {name: "A04", by: 260}, {name: "C04", by: 380}, {name: "D02", by: 520}}
	for k := range shifts {
		shifts[k].col = t.col[shifts[k].name]
	}
	total := nc + 5 + len(shifts)
	label := append([]string{}, t.names...)
	for k := 0; k < 5; k++ {
		label = append(label, fmt.Sprintf("plc%d", k+1))
	}
	for _, s := range shifts {
		label = append(label, fmt.Sprintf("shift%s/%d", s.name, s.by))
	}
	mk := func() [][]float64 {
		o := make([][]float64, total)
		for c := range o {
			o[c] = filled(nd)
		}
		return o
	}
	ic, bs, cr := mk(), mk(), mk()
	rtW := make([]float64, total)
	for d := lo; d <= hi; d++ {
		pr := r.keep[d]
		if pr == nil {
			continue
		}
		rows := ds.byDate[d]
		n := len(rows)
		y := make([]float64, n)
		m120, m256 := filled(n), filled(n)
		cols := make([][]float64, total)
		for c := range cols {
			cols[c] = filled(n)
		}
		for i, x := range rows {
			y[i] = x.y10
			s := t.ser[x.secid]
			p := t.pos[x.secid][d]
			if p >= 119 {
				m120[i] = sumRange(s.ab, p-119, p)
			}
			if p >= 255 {
				m256[i] = sumRange(s.ab, p-255, p)
			}
			if v := t.cv[ds.dates[d]+"|"+x.secid]; v != nil {
				for c := 0; c < nc; c++ {
					cols[c][i] = v[c]
				}
			}
			for k, sh := range shifts {
				if d-sh.by >= 0 {
					if v := t.cv[ds.dates[d-sh.by]+"|"+x.secid]; v != nil {
						cols[nc+5+k][i] = v[sh.col]
					}
				}
			}
		}
		var who []int
		for i := range rows {
			if !math.IsNaN(cols[0][i]) && !math.IsNaN(m256[i]) {
				who = append(who, i)
			}
		}
		for k := 0; k < 5; k++ {
			rng := rand.New(rand.NewSource(int64(20261101+k)*1000003 + int64(d)))
			perm := rng.Perm(len(who))
			for j, i := range who {
				cols[nc+k][i] = cols[0][who[perm[j]]]
			}
		}
		for c := 0; c < total; c++ {
			var ys, ss, ms []float64
			for i := range rows {
				if math.IsNaN(y[i]) || math.IsNaN(pr[i]) || math.IsNaN(cols[c][i]) || math.IsNaN(m256[i]) {
					continue
				}
				ys, ss, ms = append(ys, y[i]), append(ss, cols[c][i]), append(ms, m256[i])
			}
			if len(ys) < 30 {
				continue
			}
			ic[c][d-lo] = pearson(rankScale(ss), ys)
			cr[c][d-lo] = pearson(rankScale(ss), rankScale(ms))
			bs[c][d-lo] = slopeOf(y, [][]float64{pr, m120, m256}, cols[c])
			rtW[c] += float64(len(ys))
		}
	}

	redund, redCnt := redundancyMap(pp, t, lo, hi)
	fmt.Printf("fundtest: %d candidate columns (M256 is the benchmark, not in the family) plus 5 permutation and %d time-shift placebos\n", nc, len(shifts))
	fmt.Printf("stage I (exploration grade) %s..%s, last date whose 10-day label closes before %s; split %s; stage R %s..%s, split %s; threshold for stage R |t| %.3f (Nyholt, from fundmeff)\n", fundIndFrom, ds.dates[indHi], fundRtFrom, fundIndSplit, ds.dates[lo], ds.dates[hi], fundRtSplit, thr)
	fmt.Printf("%-6s %5s %5s | %7s %6s %6s %7s %7s %6s %6s | %5s %5s | %7s %6s %6s %7s %6s %9s %9s %6s %8s %8s %6s %s\n",
		"col", "I:n", "width", "I:IC", "t10", "tPlug", "half1", "half2", "yrs+", "yrs", "R:n", "width", "R:IC", "t10", "tPlug", "slope", "t10", "boot99lo", "boot99hi", "tPlug", "half1", "half2", "rM256", "redundant with / verdict")
	var cand, leads, plHit, indSup int
	for c := 0; c < total; c++ {
		isPl := c >= nc
		name := label[c]
		iv := seqStat(indIC[min(c, nc-1)], 0, indHi-indLo)
		iline := fmt.Sprintf("%5s %5s | %7s %6s %6s %7s %7s %6s %6s", "-", "-", "-", "-", "-", "-", "-", "-", "-")
		indOK := false
		if !isPl {
			if len(iv) >= 200 {
				m, tv, tp, _, _ := seriesInference(indIC[c])
				h1, h2 := halves(indIC[c], indSplit)
				yp, yn := yearSigns(indIC[c], indLo, ds.dates, m)
				indOK = math.Abs(m) >= 0.02 && math.Abs(tv) >= 2.0 && h1*m > 0 && h2*m > 0
				iline = fmt.Sprintf("%5d %5.0f | %+7.4f %+6.2f %+6.2f %+7.4f %+7.4f %6d %6d", len(iv), indW[c]/float64(len(iv)), m, tv, tp, h1, h2, yp, yn)
			} else {
				iline = fmt.Sprintf("%5d %5s | NOT TESTED (fewer than 200 stage-I dates)%s", len(iv), "-", strings.Repeat(" ", 3))
			}
		}
		rIC, rBS := seqStat(ic[c], 0, nd-1), seqStat(bs[c], 0, nd-1)
		rline := fmt.Sprintf("%5s %5s | NOT TESTED", "-", "-")
		hit, tested := false, false
		if len(rBS) >= 100 && !strings.HasPrefix(name, "M256") {
			tested = true
			mi, ti := neweyWest(rIC, 10)
			_, tpi := neweyWest(rIC, plugInLag(rIC))
			_ = tpi
			mb, tb, tbp, lo99, hi99 := seriesInference(bs[c])
			h1, h2 := halves(bs[c], split)
			hit = math.Abs(tb) >= thr && h1*mb > 0 && h2*mb > 0 && mi*mb > 0 && math.Abs(tbp) >= 3.0 && (lo99 > 0 || hi99 < 0)
			rline = fmt.Sprintf("%5d %5.0f | %+7.4f %+6.2f %+6.2f %+7.4f %+6.2f %+9.4f %+9.4f %+6.2f %+8.4f %+8.4f %+6.3f", len(rBS), rtW[c]/float64(len(rBS)), mi, ti, tpi, mb, tb, lo99, hi99, tbp, h1, h2, mean(seqStat(cr[c], 0, nd-1)))
		} else if len(rIC) >= 100 {
			mi, ti, tpi, lo99, hi99 := seriesInference(ic[c])
			h1, h2 := halves(ic[c], split)
			rline = fmt.Sprintf("%5d %5.0f | %+7.4f %+6.2f %+6.2f IC only: boot99 [%+.4f, %+.4f] halves %+.4f %+.4f", len(rIC), rtW[c]/float64(len(rIC)), mi, ti, tpi, lo99, hi99, h1, h2)
		}
		verdict := ""
		redundant := false
		if !isPl {
			best, bestV := "", 0.0
			for nm, s := range redund[c] {
				if v := s / float64(redCnt); math.Abs(v) > math.Abs(bestV) {
					best, bestV = nm, v
				}
			}
			verdict = fmt.Sprintf("%s %+.2f", best, bestV)
			if math.Abs(bestV) > 0.7 {
				verdict += " REDUNDANT"
				redundant = true
			}
		}
		switch {
		case isPl && hit:
			verdict = "PLACEBO HIT"
			plHit++
		case isPl:
			verdict = ""
		case !tested && name != "M256":
			verdict += " | NOT TESTED"
		case hit && redundant:
			verdict += " | LEAD (redundant with a composite input)"
			leads++
		case hit && fundConfirmable(name) && indOK:
			verdict += " | CANDIDATE"
			cand++
		case hit && !fundConfirmable(name):
			verdict += " | LEAD (excluded from candidates by pre-registration)"
			leads++
		case hit:
			verdict += " | LEAD (no stage-I support)"
			leads++
		case indOK:
			verdict += " | stage I only"
			indSup++
		}
		fmt.Printf("%-6s %s | %s %s\n", name, iline, rline, verdict)
	}
	fmt.Printf("fundtest: candidates (stage R hit under all three inference methods, stage-I support, same family rules) %d, leads %d, stage-I-only %d, placebo hits %d of %d (one placebo hit voids the run)\n", cand, leads, indSup, plHit, 5+len(shifts))
	bookRules(basePath, macroDir, "net_gp25_blend")
	bookRules(basePath, macroDir, "net_d_blend")
}

func yearSigns(x []float64, lo int, dates []string, m float64) (int, int) {
	by := map[string][]float64{}
	for i, v := range x {
		if !math.IsNaN(v) {
			y := dates[lo+i][:4]
			by[y] = append(by[y], v)
		}
	}
	same, n := 0, 0
	for _, v := range by {
		if len(v) >= 50 {
			n++
			if mean(v)*m > 0 {
				same++
			}
		}
	}
	return same, n
}

func readSeriesCSV(path string) ([]string, []float64) {
	f, err := os.Open(path)
	if err != nil {
		fatal(err)
	}
	defer f.Close()
	r := csv.NewReader(bufio.NewReader(f))
	r.Read()
	var d []string
	var v []float64
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			fatal(err)
		}
		x, err := strconv.ParseFloat(rec[1], 64)
		if err != nil {
			continue
		}
		d, v = append(d, rec[0]), append(v, x)
	}
	return d, v
}

func lastBefore(dates []string, d string) int {
	return sort.SearchStrings(dates, d) - 1
}

func bookNetFlags(net []float64) (volFlag, ddFlag []bool) {
	n := len(net)
	volFlag, ddFlag = make([]bool, n), make([]bool, n)
	var sig []float64
	for t := 61; t < n; t++ {
		w := net[t-60 : t]
		sig = append(sig, sdOf(w))
		med := append([]float64{}, sig...)
		sort.Float64s(med)
		volFlag[t] = len(med) >= 20 && sig[len(sig)-1] > med[len(med)/2]
		cum := 0.0
		for _, x := range w {
			cum += x
		}
		ddFlag[t] = cum < 0
	}
	return
}

func maxDrawdown(r []float64) float64 {
	cum, peak, dd := 0.0, 0.0, 0.0
	for _, x := range r {
		cum += x
		peak = math.Max(peak, cum)
		dd = math.Min(dd, cum-peak)
	}
	return dd
}

func sharpe(r []float64) float64 {
	sd := sdOf(r)
	if sd == 0 {
		return nan
	}
	return mean(r) / sd * math.Sqrt(252)
}

func circBlockPerm(flags []bool, block int, seed int64) []bool {
	n := len(flags)
	nb := (n + block - 1) / block
	order := rand.New(rand.NewSource(seed)).Perm(nb)
	out := make([]bool, 0, n)
	for _, b := range order {
		lo, hi := b*block, (b+1)*block
		if hi > n {
			hi = n
		}
		out = append(out, flags[lo:hi]...)
	}
	return out[:n]
}

func timingSeries(flags []bool, net []float64) []float64 {
	duty := 0.0
	for _, f := range flags {
		if f {
			duty++
		}
	}
	duty /= float64(len(flags))
	cm := 1 - 0.5*duty
	o := make([]float64, len(flags))
	for i, f := range flags {
		e := 1.0
		if f {
			e = 0.5
		}
		o[i] = (e - cm) * net[i]
	}
	return o
}

func bookRules(basePath, macroDir, col string) {
	f, err := os.Open(basePath)
	if err != nil {
		fatal(err)
	}
	r := csv.NewReader(bufio.NewReaderSize(f, 1<<20))
	hdr, _ := r.Read()
	ix := map[string]int{}
	for i, h := range hdr {
		ix[h] = i
	}
	var days []string
	var net []float64
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			fatal(err)
		}
		days = append(days, rec[ix["date"]])
		net = append(net, f64(rec[ix[col]]))
	}
	f.Close()
	n := len(days)
	imD, imV := readSeriesCSV(filepath.Join(macroDir, "imoex.csv"))
	rvD, rvV := readSeriesCSV(filepath.Join(macroDir, "rvi.csv"))
	krD, krV := readSeriesCSV(filepath.Join(macroDir, "keyrate.csv"))
	names := []string{"G01 book volatility above its past median", "G02 IMOEX below its 200-day mean", "G03 IMOEX 12-month return negative", "G04 RVI above its 60-day median", "G05 key rate up over 20 days", "G06 book 60-day cumulative net negative"}
	flags := make([][]bool, len(names))
	for g := range flags {
		flags[g] = make([]bool, n)
	}
	flags[0], flags[5] = bookNetFlags(net)
	for t := 0; t < n; t++ {
		j := lastBefore(imD, days[t])
		if j >= 252 {
			m := 0.0
			for _, x := range imV[j-199 : j+1] {
				m += x
			}
			flags[1][t] = imV[j] < m/200
			flags[2][t] = imV[j]/imV[j-252] < 1
		}
		if k := lastBefore(rvD, days[t]); k >= 60 {
			w := append([]float64{}, rvV[k-59:k+1]...)
			sort.Float64s(w)
			flags[3][t] = rvV[k] > w[30]
		}
		if k := lastBefore(krD, days[t]); k >= 20 {
			flags[4][t] = krV[k] > krV[k-20]
		}
	}
	start := 61
	sub := net[start:]
	dd := days[start:]
	split := sortSearch(dd, fundRtSplit)
	blocks := [][2]string{{"2025-01-01", "2025-06-30"}, {"2025-07-01", "2025-12-31"}, {"2026-01-01", "2026-06-30"}, {"2026-07-01", "2026-12-31"}}
	fmt.Printf("\nbook rules on %s, %d days from %s; flags use information through the previous day only (G01/G06 read net[t-60:t], tested); exposure 0.5 when on; primary comparator: the same-duty constant-exposure book; cost per switch assumed %.2f%% of capital\n", col, len(sub), dd[0], 100*bookSwitch)
	fmt.Printf("base: mean %+.5f, Sharpe %.2f, max drawdown %+.4f\n", mean(sub), sharpe(sub), maxDrawdown(sub))
	for g, nm := range names {
		fl := flags[g][start:]
		duty := 0.0
		sw := 0
		for i, x := range fl {
			if x {
				duty++
			}
			if i > 0 && x != fl[i-1] {
				sw++
			}
		}
		duty /= float64(len(fl))
		ts := timingSeries(fl, sub)
		m, tv := neweyWest(ts, 10)
		h1, h2 := halves(padNaN(ts, 0), split)
		scaled := make([]float64, len(sub))
		for i, x := range sub {
			e := 1.0
			if fl[i] {
				e = 0.5
			}
			scaled[i] = e * x
		}
		order := make([]int, len(sub))
		for i := range order {
			order[i] = i
		}
		sort.Slice(order, func(a, b int) bool { return math.Abs(sub[order[a]]) > math.Abs(sub[order[b]]) })
		drop := map[int]bool{}
		for _, i := range order[:5] {
			drop[i] = true
		}
		var kept []float64
		for i, x := range ts {
			if !drop[i] {
				kept = append(kept, x)
			}
		}
		var mon []string
		for mi, cur := 0, ""; mi < len(dd); mi++ {
			if dd[mi][:7] != cur {
				cur = dd[mi][:7]
				var seg []float64
				for mj := mi; mj < len(dd) && dd[mj][:7] == cur; mj++ {
					seg = append(seg, ts[mj])
				}
				mon = append(mon, fmt.Sprintf("%s:%+.4f", cur[2:], mean(seg)))
			}
		}
		var blk []string
		for _, b := range blocks {
			var seg []float64
			for i, d := range dd {
				if d >= b[0] && d <= b[1] {
					seg = append(seg, ts[i])
				}
			}
			if len(seg) > 0 {
				blk = append(blk, fmt.Sprintf("%+.4f", mean(seg)))
			}
		}
		var pl [2][]float64
		for bi, bl := range []int{20, 60} {
			for k := 0; k < 200; k++ {
				pf := circBlockPerm(fl, bl, int64(20261101+k)*977+int64(g)*13+int64(bl))
				pl[bi] = append(pl[bi], mean(timingSeries(pf, sub)))
			}
			sort.Float64s(pl[bi])
		}
		p95 := math.Max(percentile(pl[0], 0.95), percentile(pl[1], 0.95))
		years := float64(len(sub)) / 252
		cost := float64(sw) / float64(len(sub)) * bookSwitch
		verdict := ""
		if tv >= 2.6 && h1 > 0 && h2 > 0 && m > p95 {
			verdict = "PASS (upper bound)"
		}
		fmt.Printf("%-42s duty %.2f  switches/yr %.1f  timing mean %+.5f NW t %+.2f halves %+.5f %+.5f  top5-dropped %+.5f  blocks %s  months %s  placebo mean %+.5f p5/p50/p95 %+.5f %+.5f %+.5f (len20) %+.5f %+.5f %+.5f (len60)  cost-adjusted %+.5f  Sharpe %.2f DD %+.4f %s\n",
			nm, duty, float64(sw)/years, m, tv, h1, h2, mean(kept), strings.Join(blk, " "), strings.Join(mon, " "), mean(append(append([]float64{}, pl[0]...), pl[1]...)), percentile(pl[0], 0.05), percentile(pl[0], 0.5), percentile(pl[0], 0.95), percentile(pl[1], 0.05), percentile(pl[1], 0.5), percentile(pl[1], 0.95), m-cost, sharpe(scaled), maxDrawdown(scaled), verdict)
	}
}

func padNaN(v []float64, lead int) []float64 {
	o := make([]float64, 0, lead+len(v))
	for i := 0; i < lead; i++ {
		o = append(o, nan)
	}
	return append(o, v...)
}

func redundancyMap(pp *pipe, t *tfmCtx, lo, hi int) ([]map[string]float64, int) {
	ds := pp.ds
	nc := len(t.names)
	out := make([]map[string]float64, nc)
	for c := range out {
		out[c] = map[string]float64{}
	}
	cnt := 0
	for d := lo; d <= hi; d += 3 {
		rows := ds.byDate[d]
		cols := make([][]float64, nc)
		for c := range cols {
			cols[c] = filled(len(rows))
		}
		for i, x := range rows {
			if v := t.cv[ds.dates[d]+"|"+x.secid]; v != nil {
				for c := 0; c < nc; c++ {
					cols[c][i] = v[c]
				}
			}
		}
		cnt++
		for c := 0; c < nc; c++ {
			for k, nm := range ds.names {
				var a, b []float64
				for i, x := range rows {
					if !math.IsNaN(cols[c][i]) && !math.IsNaN(x.raw[k]) {
						a, b = append(a, cols[c][i]), append(b, x.raw[k])
					}
				}
				if len(a) >= 30 {
					if v := pearson(rankScale(a), rankScale(b)); !math.IsNaN(v) {
						out[c][nm] += v
					}
				}
			}
		}
	}
	return out, cnt
}

func fundCover(pp *pipe) {
	ds := pp.ds
	t := buildTfmCtx(pp, candFile)
	nc := len(t.names)
	indLo, indHi := sortSearch(ds.dates, fundIndFrom), indHiIndex(pp)
	lo, hi := pp.dateRange(fundRtFrom, fundRtTill)
	fmt.Printf("fundcover: stage I %s..%s, stage R %s..%s\n", ds.dates[indLo], ds.dates[indHi], ds.dates[lo], ds.dates[hi])
	fmt.Printf("%-6s %9s %10s %10s | %6s %6s | %6s %6s\n", "col", "rows", "first", "last", "I:n", "width", "R:n", "width")
	for c := 0; c < nc; c++ {
		rows, first, last := 0, "", ""
		var iN, rN int
		var iW, rW float64
		for d := range ds.dates {
			w := 0
			for _, x := range ds.byDate[d] {
				if v := t.cv[ds.dates[d]+"|"+x.secid]; v != nil && !math.IsNaN(v[c]) {
					w++
				}
			}
			if w == 0 {
				continue
			}
			rows += w
			if first == "" {
				first = ds.dates[d]
			}
			last = ds.dates[d]
			if w >= 30 && d >= indLo && d <= indHi {
				iN++
				iW += float64(w)
			}
			if w >= 30 && d >= lo && d <= hi {
				rN++
				rW += float64(w)
			}
		}
		fmt.Printf("%-6s %9d %10s %10s | %6d %6.0f | %6d %6.0f\n", t.names[c], rows, first, last, iN, iW/math.Max(1, float64(iN)), rN, rW/math.Max(1, float64(rN)))
	}
	red, cnt := redundancyMap(pp, t, lo, hi)
	fmt.Printf("\nfundcover: largest mean cross-sectional rank correlation with an existing composite input on stage-R dates (every third date, %d dates); |rho| above 0.7 pre-declares the column redundant\n", cnt)
	for c := 0; c < nc; c++ {
		type kv struct {
			n string
			v float64
		}
		var top []kv
		for nm, s := range red[c] {
			top = append(top, kv{nm, s / float64(cnt)})
		}
		sort.Slice(top, func(a, b int) bool { return math.Abs(top[a].v) > math.Abs(top[b].v) })
		var parts []string
		for i := 0; i < 3 && i < len(top); i++ {
			parts = append(parts, fmt.Sprintf("%s %+.2f", top[i].n, top[i].v))
		}
		flag := ""
		if len(top) > 0 && math.Abs(top[0].v) > 0.7 {
			flag = " REDUNDANT"
		}
		fmt.Printf("%-6s %s%s\n", t.names[c], strings.Join(parts, ", "), flag)
	}
}
