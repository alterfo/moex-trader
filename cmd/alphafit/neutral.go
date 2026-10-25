package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"sort"
)

var neutralMode = "none"

var sectorLists = map[string][]string{
	"fin":    {"SBER", "SBERP", "VTBR", "T", "CBOM", "BSPB", "BSPBP", "SVCB", "MOEX", "SPBE", "MBNK", "RENI", "DOMRF", "ZAYM", "LEAS", "SFIN"},
	"oilgas": {"LKOH", "ROSN", "TATN", "TATNP", "SNGS", "SNGSP", "SIBN", "BANE", "BANEP", "RNFT", "TRNFP", "NVTK", "GAZP"},
	"metals": {"GMKN", "NLMK", "MAGN", "CHMF", "RUAL", "PLZL", "ALRS", "MTLR", "MTLRP", "UGLD", "TRMK", "SELG", "RASP", "AMEZ", "ENPG", "VSMO"},
	"power":  {"IRAO", "HYDR", "MSNG", "UPRO", "FEES", "OGKB", "TGKA", "TGKN", "MRKP", "MRKC", "MRKV", "MRKU", "MRKZ", "LSNGP", "ELFV", "MSRS"},
}

var sectorOf = func() map[string]string {
	m := map[string]string{}
	for s, l := range sectorLists {
		for _, t := range l {
			m[t] = s
		}
	}
	return m
}()

func sectorNeutral(rows []*row, pred []float64) []float64 {
	sum := map[string]float64{}
	cnt := map[string]int{}
	for i, x := range rows {
		if math.IsNaN(pred[i]) {
			continue
		}
		s := sectorOf[x.secid]
		sum[s] += pred[i]
		cnt[s]++
	}
	out := make([]float64, len(pred))
	for i, x := range rows {
		out[i] = pred[i]
		s := sectorOf[x.secid]
		if !math.IsNaN(pred[i]) && cnt[s] >= 3 {
			out[i] = pred[i] - sum[s]/float64(cnt[s])
		}
	}
	return out
}

func normCDF(x float64) float64 { return 0.5 * math.Erfc(-x/math.Sqrt2) }

func normInv(p float64) float64 {
	lo, hi := -10.0, 10.0
	for i := 0; i < 200; i++ {
		mid := (lo + hi) / 2
		if normCDF(mid) < p {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2
}

func moments(x []float64) (m, sd, skew, kurt float64) {
	n := float64(len(x))
	for _, v := range x {
		m += v
	}
	m /= n
	var s2, s3, s4 float64
	for _, v := range x {
		d := v - m
		s2 += d * d
		s3 += d * d * d
		s4 += d * d * d * d
	}
	sd = math.Sqrt(s2 / n)
	skew = (s3 / n) / (sd * sd * sd)
	kurt = (s4 / n) / (sd * sd * sd * sd)
	return
}

func readDump(p string) (map[string][]float64, []string) {
	f, err := os.Open(p)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	r := csv.NewReader(bufio.NewReader(f))
	hdr, _ := r.Read()
	cols := map[string][]float64{}
	var dates []string
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		dates = append(dates, rec[0])
		for i := 1; i < len(hdr); i++ {
			cols[hdr[i]] = append(cols[hdr[i]], parseF(rec[i]))
		}
	}
	return cols, dates
}

func deflatedSharpe(path string, col string, trials []int) {
	cols, dates := readDump(path)
	x := clean(cols[col])
	T := float64(len(x))
	m, sd, skew, kurt := moments(x)
	sr := m / sd
	fmt.Printf("dsr: %s column %s, T %d days, mean %+.5f%% per day, daily Sharpe %+.4f, annualised %+.2f, skew %+.2f, kurtosis %.2f\n", path, col, len(x), m*100, sr, sr*math.Sqrt(252), skew, kurt)
	denom := math.Sqrt(1 - skew*sr + (kurt-1)/4*sr*sr)
	psr := normCDF(sr * math.Sqrt(T-1) / denom)
	fmt.Printf("dsr: probabilistic Sharpe ratio against zero %.3f (one trial)\n", psr)
	gamma := 0.5772156649
	v := 1 / T
	for _, n := range trials {
		nn := float64(n)
		sr0 := math.Sqrt(v) * ((1-gamma)*normInv(1-1/nn) + gamma*normInv(1-1/(nn*math.E)))
		dsr := normCDF((sr - sr0) * math.Sqrt(T-1) / denom)
		fmt.Printf("dsr: trials %4d: expected maximum daily Sharpe under zero skill %.4f (annualised %.2f), deflated Sharpe probability %.3f\n", n, sr0, sr0*math.Sqrt(252), dsr)
	}
	_ = dates
	_ = sort.Float64s
}

func betaDiagnostic(path string, col string) {
	cols, _ := readDump(path)
	y, u := cols[col], cols["u1"]
	var xs, ys []float64
	for i := range y {
		if !math.IsNaN(y[i]) && !math.IsNaN(u[i]) {
			xs = append(xs, u[i])
			ys = append(ys, y[i])
		}
	}
	mx, my := mean(xs), mean(ys)
	var sxy, sxx float64
	for i := range xs {
		sxy += (xs[i] - mx) * (ys[i] - my)
		sxx += (xs[i] - mx) * (xs[i] - mx)
	}
	beta := sxy / sxx
	res := make([]float64, len(xs))
	for i := range xs {
		res[i] = ys[i] - beta*xs[i]
	}
	m, t := neweyWest(res, 10)
	m0, t0 := neweyWest(ys, 10)
	lo, hi := bootCI(res, 5000, 20261022)
	fmt.Printf("beta: %s on the universe mean 1-day return: beta %+.3f; mean net %+.5f%% (NW t %+.2f), beta-hedged residual mean %+.5f%% (NW t %+.2f, boot95 [%+.5f%%, %+.5f%%]), n %d\n", col, beta, m0*100, t0, m*100, t, lo*100, hi*100, len(xs))
}
