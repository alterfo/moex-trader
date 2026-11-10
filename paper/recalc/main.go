package main

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"strconv"
	"time"
)

type row struct {
	date  string
	ic    float64
	icA2  float64
	net   float64
	net25 float64
	gl    float64
	gs    float64
	u1    float64
}

func atof(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return math.NaN()
	}
	return v
}

func phi(x float64) float64   { return 0.5 * (1 + math.Erf(x/math.Sqrt2)) }
func phinv(p float64) float64 { return math.Sqrt2 * math.Erfinv(2*p-1) }

func mean(x []float64) float64 {
	var s float64
	for _, v := range x {
		s += v
	}
	return s / float64(len(x))
}

func sd(x []float64) float64 {
	m := mean(x)
	var s float64
	for _, v := range x {
		s += (v - m) * (v - m)
	}
	return math.Sqrt(s / float64(len(x)-1))
}

func skewKurt(x []float64) (float64, float64) {
	m := mean(x)
	n := float64(len(x))
	var s2, s3, s4 float64
	for _, v := range x {
		d := v - m
		s2 += d * d
		s3 += d * d * d
		s4 += d * d * d * d
	}
	s2 /= n
	s3 /= n
	s4 /= n
	return s3 / math.Pow(s2, 1.5), s4/(s2*s2) - 3
}

func acf(x []float64, l int) float64 {
	m := mean(x)
	var num, den float64
	for i := range x {
		den += (x[i] - m) * (x[i] - m)
	}
	for i := 0; i+l < len(x); i++ {
		num += (x[i] - m) * (x[i+l] - m)
	}
	return num / den
}

func nwVar(x []float64, L int) float64 {
	m := mean(x)
	n := float64(len(x))
	var g0 float64
	for _, v := range x {
		g0 += (v - m) * (v - m)
	}
	g0 /= n
	v := g0
	for l := 1; l <= L; l++ {
		var g float64
		for i := 0; i+l < len(x); i++ {
			g += (x[i] - m) * (x[i+l] - m)
		}
		g /= n
		v += 2 * (1 - float64(l)/float64(L+1)) * g
	}
	return v
}

func ratio(pr, side float64, n int) float64 {
	if side == 0 {
		return 0
	}
	return pr / side / float64(n)
}

func main() {
	f, err := os.Open(os.Args[1])
	if err != nil {
		panic(err)
	}
	defer f.Close()
	rd := csv.NewReader(f)
	recs, err := rd.ReadAll()
	if err != nil {
		panic(err)
	}
	hdr := recs[0]
	idx := map[string]int{}
	for i, h := range hdr {
		idx[h] = i
	}
	var rows []row
	for _, r := range recs[1:] {
		rows = append(rows, row{
			date:  r[idx["date"]],
			ic:    atof(r[idx["ic_blend"]]),
			icA2:  atof(r[idx["ic_a2"]]),
			net:   atof(r[idx["net_gp_blend"]]),
			net25: atof(r[idx["net_gp25_blend"]]),
			gl:    atof(r[idx["glong_gp_blend"]]),
			gs:    atof(r[idx["gshort_gp_blend"]]),
			u1:    atof(r[idx["u1"]]),
		})
	}

	var nets, ics, icsA2, inc, longs, shorts, us []float64
	for _, r := range rows {
		if !math.IsNaN(r.net) {
			nets = append(nets, r.net*100)
		}
		if !math.IsNaN(r.ic) {
			ics = append(ics, r.ic)
		}
		if !math.IsNaN(r.icA2) {
			icsA2 = append(icsA2, r.icA2)
		}
		if !math.IsNaN(r.ic) && !math.IsNaN(r.icA2) {
			inc = append(inc, r.ic-r.icA2)
		}
		if !math.IsNaN(r.gl) {
			longs = append(longs, r.gl*100)
		}
		if !math.IsNaN(r.gs) {
			shorts = append(shorts, r.gs*100)
		}
		if !math.IsNaN(r.u1) {
			us = append(us, r.u1)
		}
	}

	fmt.Printf("rows %d  book-days %d  ic-days %d  both %d\n\n", len(rows), len(nets), len(ics), len(inc))

	// --- A. book stats
	sdNet := sd(nets)
	nwNet := nwVar(nets, 10)
	srD := mean(nets) / sdNet
	sk, ku := skewKurt(nets)
	fmt.Println("== A. gate book (net_gp_blend, %/day), 17% borrow ==")
	fmt.Printf("mean %.5f  sd %.5f  daily SR %.4f  annual SR %.3f  skew %.3f  exkurt %.3f\n",
		mean(nets), sdNet, srD, srD*math.Sqrt(252), sk, ku)
	fmt.Printf("NW10 se %.5f  t %.3f  (naive se %.5f t %.3f)\n",
		math.Sqrt(nwNet/float64(len(nets))), mean(nets)/math.Sqrt(nwNet/float64(len(nets))),
		sdNet/math.Sqrt(float64(len(nets))), mean(nets)/(sdNet/math.Sqrt(float64(len(nets)))))
	vr := nwNet / (sdNet * sdNet)
	fmt.Printf("variance ratio (long-run factor) %.4f  -> long-run sd %.5f  long-run SR %.3f\n",
		vr, sdNet*math.Sqrt(vr), srD/math.Sqrt(vr)*math.Sqrt(252))
	var sumAC10 float64
	for l := 1; l <= 10; l++ {
		r := acf(nets, l)
		sumAC10 += r
		if l <= 5 {
			fmt.Printf("rho %d %.4f\n", l, r)
		}
	}
	fmt.Printf("sum of autocorrelations 1-10: %.4f (unweighted) / %.4f (Bartlett) -> V' %.3f / %.3f\n\n",
		sumAC10, (vr-1)/2, 1+2*sumAC10, vr)

	// --- B. legs
	fmt.Println("== B. legs (gross, %/day) ==")
	fmt.Printf("long leg  mean %.5f  short leg mean %.5f  sum %.5f\n", mean(longs), mean(shorts), mean(longs)+mean(shorts))
	fmt.Printf("short share of gross %.1f%%\n\n", 100*mean(shorts)/(mean(longs)+mean(shorts)))

	// --- C. beta hedge
	n := float64(len(us))
	var mu, mv float64
	for i := range us {
		mu += us[i]
		mv += nets[i]
	}
	mu /= n
	mv /= n
	var cov, varv float64
	for i := range us {
		cov += (us[i] - mu) * (nets[i] - mv)
		varv += (us[i] - mu) * (us[i] - mu)
	}
	beta := cov / varv
	var hedged []float64
	for i := range us {
		hedged = append(hedged, nets[i]-beta*us[i])
	}
	nwH := nwVar(hedged, 10)
	fmt.Println("== C. beta hedge ==")
	fmt.Printf("beta %.3f  hedged mean %.5f  NW10 t %.2f  (unhedged %.5f, t %.2f)\n", beta/100, mean(hedged),
		mean(hedged)/math.Sqrt(nwH/n), mean(nets), mean(nets)/math.Sqrt(nwNet/n))
	fmt.Printf("hedge cost %.5f  x |beta|*20%%/252 = %.5f\n\n", mean(nets)-mean(hedged), math.Abs(beta)*0.20/252*100)

	// --- D. carry sweep
	fmt.Println("== D. carry sensitivity (per 1pp carry = 0.00381 %/day) ==")
	perPP := 0.96 / 252 * 100 / 100
	for _, c := range []float64{17, 20, 25, 27.5, 30, 37, 50} {
		fmt.Printf("carry %5.1f%%  net %+.5f  NW-sign unchanged\n", c, mean(nets)-(c-17)*perPP)
	}
	fmt.Printf("per 1pp: %.5f %%/day\n\n", perPP)

	// --- E. IC increments and SE
	fmt.Println("== E. daily IC ==")
	fmt.Printf("IC composite %.5f (n %d)  sd %.5f  naive se %.5f  NW10 t %.2f\n",
		mean(ics), len(ics), sd(ics), sd(ics)/math.Sqrt(float64(len(ics))),
		mean(ics)/math.Sqrt(nwVar(ics, 10)/float64(len(ics))))
	nwInc := nwVar(inc, 10)
	seInc := math.Sqrt(nwInc / float64(len(inc)))
	fmt.Printf("increment mean %.5f  sd %.5f  naive se %.5f  NW10 se %.5f  t %.2f\n\n",
		mean(inc), sd(inc), sd(inc)/math.Sqrt(float64(len(inc))), seInc, mean(inc)/seInc)

	// --- F. forward-rule power
	fmt.Println("== F. forward rule power (95% interval of the increment above zero, i.e. t>1.96) ==")
	d := mean(inc)
	sdLR := sd(inc) * math.Sqrt(nwInc/(sd(inc)*sd(inc)))
	fmt.Printf("observed increment %.5f  long-run sd %.5f  effective n at 450 dates %.1f\n",
		d, sdLR, 450*sd(inc)*sd(inc)/nwInc)
	for _, nn := range []float64{60, 120, 250, 450} {
		se := sdLR / math.Sqrt(nn)
		mde := 1.96 * se
		pw := phi(d/se - 1.96)
		need80 := math.Pow((1.96+0.8416)*sdLR/d, 2)
		need80s := math.Pow((3.45+0.8416)*sdLR/d, 2)
		fmt.Printf("n %4.0f  se %.5f  MDE(95%%) %.5f  power vs %.4f %5.1f%%  n80(iid se) %.0f  n80(t>=3.45) %.0f\n",
			nn, se, mde, d, 100*pw, need80, need80s)
	}
	fmt.Println("(iid figures for comparison: se60 =", fmt.Sprintf("%.5f", sd(inc)/math.Sqrt(60)),
		" MDE", fmt.Sprintf("%.5f", 1.96*sd(inc)/math.Sqrt(60)), ")")

	rng := newRand(7)
	var rates []float64
	nb := (len(inc) + 9) / 10
	for s := 0; s < 4000; s++ {
		var sample []float64
		for len(sample) < 60 {
			b := rng.Intn(nb)
			for j := 0; j < 10 && len(sample) < 60; j++ {
				sample = append(sample, inc[(b*10+j)%len(inc)])
			}
		}
		rates = append(rates, mean(sample))
	}
	fmt.Printf("block bootstrap (10-date blocks, 4000 sims): se of the 60-date mean %.5f (analytic %.5f), mean %.5f\n",
		sd(rates), sdLR/math.Sqrt(60), mean(rates))
	for _, thr := range []float64{1.96, 3.0, 3.45} {
		fmt.Printf("  dates for 80%% power at |t|>=%.2f: %.0f\n", thr, math.Pow((thr+0.8416)*sdLR/d, 2))
	}
	fmt.Println()

	// --- G. sign tests
	fmt.Println("== G. sign tests ==")
	type bucket struct {
		key  string
		vals []float64
	}
	monthly := map[string][]float64{}
	var order []string
	for _, r := range rows {
		if math.IsNaN(r.ic) {
			continue
		}
		t, err := time.Parse("2006-01-02", r.date)
		if err != nil {
			continue
		}
		k := t.Format("2006-01")
		if _, ok := monthly[k]; !ok {
			order = append(order, k)
		}
		monthly[k] = append(monthly[k], r.ic)
	}
	var mvals []float64
	pos := 0
	for _, k := range order {
		m := mean(monthly[k])
		mvals = append(mvals, m)
		if m > 0 {
			pos++
		}
	}
	p := binomTail(pos, len(mvals))
	fmt.Printf("months %d positive %d  binomial p %.4f\n", len(mvals), pos, p)
	ac := []float64{}
	for l := 1; l <= 3; l++ {
		ac = append(ac, acf(mvals, l))
	}
	neff := float64(len(mvals))
	var sumAC float64
	for _, a := range ac {
		sumAC += (1 - float64(1)/float64(len(mvals))) * a
	}
	neff = float64(len(mvals)) / (1 + 2*sumAC)
	fmt.Printf("monthly autocorr lags1-3 %.3f %.3f %.3f  effective months %.1f  p at eff.n %.4f\n",
		ac[0], ac[1], ac[2], neff, binomTail(int(math.Round(float64(pos)*neff/float64(len(mvals)))), int(math.Round(neff))))
	for _, ne := range []int{22, 19, 16, 14, 12, 10} {
		pp := int(math.Round(float64(pos) / float64(len(mvals)) * float64(ne)))
		fmt.Printf("  if effective months=%2d, successes %2d -> p %.3f\n", ne, pp, binomTail(pp, ne))
	}
	// non-overlapping 10-date blocks
	var blocks []float64
	for i := 0; i+10 <= len(ics); i += 10 {
		blocks = append(blocks, mean(ics[i:i+10]))
	}
	bpos := 0
	for _, b := range blocks {
		if b > 0 {
			bpos++
		}
	}
	fmt.Printf("non-overlapping 10-date blocks %d positive %d  binomial p %.4f\n", len(blocks), bpos, binomTail(bpos, len(blocks)))
	// quarters of the book
	q := map[string][]float64{}
	var qorder []string
	for _, r := range rows {
		if math.IsNaN(r.ic) {
			continue
		}
		t, _ := time.Parse("2006-01-02", r.date)
		k := fmt.Sprintf("%dQ%d", t.Year(), (int(t.Month())-1)/3+1)
		if _, ok := q[k]; !ok {
			qorder = append(qorder, k)
		}
		q[k] = append(q[k], r.ic)
	}
	qp := 0
	for _, k := range qorder {
		if mean(q[k]) > 0 {
			qp++
		}
	}
	fmt.Printf("quarters %d positive %d  binomial p %.4f\n\n", len(qorder), qp, binomTail(qp, len(qorder)))
	for _, k := range qorder {
		fmt.Printf("  %s mean IC %+.5f\n", k, mean(q[k]))
	}
	fmt.Println()

	// --- H. equivalence and detection dates
	fmt.Println("== H. sample-size formulas (two-sided alpha=5%, power 80%: (1.96+0.8416)^2) ==")
	sdLRpct := sdNet * math.Sqrt(vr)
	for _, delta := range []float64{0.0751, 0.056, 0.05} {
		for _, s := range []float64{sdNet, sdLRpct} {
			T := math.Pow((1.96+0.8416)*s/delta, 2)
			fmt.Printf("delta %.4f  sd %.5f  T %.0f days (%.2f years)\n", delta, s, T, T/252)
		}
	}
	for _, c := range []struct {
		T     float64
		delta float64
	}{{1160, 0.0751}, {2300, 0.056}} {
		implied := c.delta * math.Sqrt(c.T) / (1.96 + 0.8416)
		fmt.Printf("T %.0f at delta %.4f implies sd %.5f\n", c.T, c.delta, implied)
	}
	fmt.Printf("observed sd %.5f, long-run sd %.5f\n\n", sdNet, sdLRpct)

	// --- I. dividend bias
	fmt.Println("== I. dividend compensation bias on the short leg (%/day of book) ==")
	for _, dy := range []float64{0, 0.5, 1.0, 2.0, 3.5} {
		fmt.Printf("yield gap long-minus-short %.1f pp -> bias %.5f  (%+.1f%% of net)\n",
			dy, 0.96*dy/100/252*100, 100*(0.96*dy/252)/mean(nets))
	}
	fmt.Println("one-sided worst case (long leg pays nothing):")
	fmt.Printf("  universe yield 3.3%%: %.5f (%.0f%% of net)  yield 5%%: %.5f\n",
		0.96*3.3/252, 100*(0.96*3.3/252)/mean(nets), 0.96*5.0/252)
}

func binomTail(k, n int) float64 {
	if k <= 0 {
		return 1
	}
	p := 0.0
	for i := k; i <= n; i++ {
		c := 1.0
		for j := 1; j <= i; j++ {
			c = c * float64(n-i+j) / float64(j)
		}
		p += c * math.Pow(0.5, float64(n))
	}
	if p > 1 {
		p = 1
	}
	return p
}

type rngT struct {
	s uint64
}

func newRand(seed uint64) *rngT {
	r := &rngT{s: seed}
	for i := 0; i < 10; i++ {
		r.next()
	}
	return r
}

func (r *rngT) next() uint64 {
	r.s ^= r.s << 13
	r.s ^= r.s >> 7
	r.s ^= r.s << 17
	return r.s
}

func (r *rngT) Intn(n int) int { return int(r.next() % uint64(n)) }
