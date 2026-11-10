package main

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
)

type rec struct {
	date string
	net  float64
}

func load(path, col string) []rec {
	f, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	rd := csv.NewReader(f)
	rows, err := rd.ReadAll()
	if err != nil {
		panic(err)
	}
	ix := map[string]int{}
	for i, h := range rows[0] {
		ix[h] = i
	}
	var out []rec
	for _, r := range rows[1:] {
		v, err := strconv.ParseFloat(r[ix[col]], 64)
		if err != nil {
			continue
		}
		out = append(out, rec{date: r[ix["date"]], net: v * 100})
	}
	return out
}

func sd(x []float64) float64 {
	m := 0.0
	for _, v := range x {
		m += v
	}
	m /= float64(len(x))
	s := 0.0
	for _, v := range x {
		s += (v - m) * (v - m)
	}
	return math.Sqrt(s / float64(len(x)-1))
}

func moments(x []float64) (float64, float64) {
	m := 0.0
	for _, v := range x {
		m += v
	}
	m /= float64(len(x))
	var s2, s3, s4 float64
	for _, v := range x {
		d := v - m
		s2 += d * d
		s3 += d * d * d
		s4 += d * d * d * d
	}
	n := float64(len(x))
	s2 /= n
	s3 /= n
	s4 /= n
	return s3 / math.Pow(s2, 1.5), s4/(s2*s2) - 3
}

func pctile(x []float64, p float64) float64 {
	c := append([]float64{}, x...)
	sort.Float64s(c)
	i := int(p * float64(len(c)-1))
	return c[i]
}

func maxDD(rs []rec) {
	eq, peak, mdd := 1.0, 1.0, 0.0
	peakDate, troughDate := rs[0].date, rs[0].date
	curPeak := rs[0].date
	for _, r := range rs {
		eq *= 1 + r.net/100
		if eq > peak {
			peak = eq
			curPeak = r.date
		}
		dd := eq/peak - 1
		if dd < mdd {
			mdd = dd
			peakDate, troughDate = curPeak, r.date
		}
	}
	fmt.Printf("max drawdown %.2f%%  peak %s  trough %s  final equity %.3f\n", 100*mdd, peakDate, troughDate, eq)
}

func monthly(rs []rec) map[string]float64 {
	m := map[string]float64{}
	var keys []string
	for _, r := range rs {
		k := r.date[:7]
		if _, ok := m[k]; !ok {
			keys = append(keys, k)
		}
		m[k] += r.net
	}
	sort.Strings(keys)
	neg, worst, best := 0, 0.0, 0.0
	for _, k := range keys {
		if m[k] < 0 {
			neg++
		}
		if m[k] < worst {
			worst = m[k]
		}
		if m[k] > best {
			best = m[k]
		}
	}
	fmt.Printf("months %d, negative %d (%.0f%%), worst %.2f%% (%s), best %.2f%%\n", len(keys), neg, 100*float64(neg)/float64(len(keys)), worst, "", best)
	return m
}

func cfVaR(x []float64, p float64) float64 {
	m := 0.0
	for _, v := range x {
		m += v
	}
	m /= float64(len(x))
	s := sd(x)
	sk, ku := moments(x)
	z := math.Sqrt2 * math.Erfinv(2*p-1)
	zcf := z + (z*z-1)*sk/6 + (z*z*z-3*z)*ku/24 - (2*z*z*z-5*z)*sk*sk/36
	return m + zcf*s
}

func main() {
	path := os.Args[1]
	col := "net_gp_blend"
	if len(os.Args) > 2 {
		col = os.Args[2]
	}
	rs := load(path, col)
	var x []float64
	for _, r := range rs {
		x = append(x, r.net)
	}
	fmt.Printf("%s column %s: %d days, mean %.5f %%/day, sd %.5f\n", path, col, len(rs), mean(x), sd(x))
	maxDD(rs)
	monthly(rs)
	sk, ku := moments(x)
	fmt.Printf("skew %.3f  excess kurtosis %.3f\n", sk, ku)
	q01 := pctile(x, 0.01)
	var tail []float64
	for _, v := range x {
		if v <= q01 {
			tail = append(tail, v)
		}
	}
	fmt.Printf("historical 99%% VaR %.3f %%/day; CVaR %.3f; normal approx %.3f; Cornish-Fisher %.3f\n",
		q01, mean(tail), mean(x)+2.326*sd(x), cfVaR(x, 0.01))
	for _, c := range []float64{25, 30, 36.9} {
		shift := -(c - 17) * 0.00381
		rs2 := make([]rec, len(rs))
		for i, r := range rs {
			rs2[i] = rec{date: r.date, net: r.net + shift}
		}
		fmt.Printf("carry %.1f%% (shift %.4f): ", c, shift)
		maxDD(rs2)
	}
}

func mean(x []float64) float64 {
	s := 0.0
	for _, v := range x {
		s += v
	}
	return s / float64(len(x))
}
