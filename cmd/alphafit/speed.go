package main

import (
	"fmt"
	"math"
	"sort"
	"strconv"
)

func trendSlope(y, x []float64, lag int) (float64, float64) {
	n := len(y)
	if n < 60 {
		return nan, nan
	}
	mx, my := mean(x), mean(y)
	var sxx, sxy float64
	for i := range y {
		sxx += (x[i] - mx) * (x[i] - mx)
		sxy += (x[i] - mx) * (y[i] - my)
	}
	if sxx == 0 {
		return nan, nan
	}
	b := sxy / sxx
	a := my - b*mx
	s := make([]float64, n)
	for i := range y {
		s[i] = (x[i] - mx) * (y[i] - a - b*x[i])
	}
	v := 0.0
	for _, q := range s {
		v += q * q
	}
	for l := 1; l <= lag; l++ {
		w := 1 - float64(l)/float64(lag+1)
		c := 0.0
		for i := l; i < n; i++ {
			c += s[i] * s[i-l]
		}
		v += 2 * w * c
	}
	if v <= 0 {
		return b, nan
	}
	return b, b / (math.Sqrt(v) / sxx)
}

func medianOf(v []float64) float64 {
	c := clean(v)
	if len(c) == 0 {
		return nan
	}
	sort.Float64s(c)
	return c[len(c)/2]
}

func yearFrac(date string) float64 {
	y, _ := strconv.Atoi(date[:4])
	m, _ := strconv.Atoi(date[5:7])
	d, _ := strconv.Atoi(date[8:10])
	return float64(y) + (float64(m-1)+float64(d)/31)/12
}

func speedTable(pp *pipe, featPath, newsPath string) {
	ds := pp.ds
	hd := heteroLoad(featPath, newsPath)
	fidx := map[string]int{}
	for i, n := range ds.names {
		fidx[n] = i
	}
	type sigDef struct {
		name string
		get  func(date, secid string) float64
		from string
	}
	sigs := []sigDef{
		{"M256 momentum", func(d, s string) float64 { return hd.sig(d, s, "M256") }, "2021-04-08"},
		{"GAP (minus B05)", func(d, s string) float64 { return -hd.sig(d, s, "B05") }, "2021-04-08"},
		{"ZC news count shock", func(d, s string) float64 { return hd.mod(d, s, "zc") }, "2022-01-03"},
	}
	hs := []int{1, 5, 10}
	fmt.Println("speed: Table A, mean daily rank IC by calendar year and horizon (dates in brackets); trend = OLS slope of the daily IC per year with Newey-West t")
	for _, sg := range sigs {
		for _, h := range hs {
			byYear := map[string][]float64{}
			var all, ts []float64
			for d, date := range ds.dates {
				if date < sg.from || date > "2026-10-16" {
					continue
				}
				rows := ds.byDate[d]
				var sv, yv []float64
				for _, x := range rows {
					s, y := sg.get(date, x.secid), outcomeOf(x, h)
					if math.IsNaN(s) || math.IsNaN(y) {
						continue
					}
					sv, yv = append(sv, s), append(yv, y)
				}
				if len(sv) < 30 {
					continue
				}
				ic := pearson(rankScale(sv), rankScale(yv))
				if math.IsNaN(ic) {
					continue
				}
				byYear[date[:4]] = append(byYear[date[:4]], ic)
				all, ts = append(all, ic), append(ts, yearFrac(date))
			}
			line := fmt.Sprintf("%-22s h%-2d", sg.name, h)
			var ys []string
			for y := range byYear {
				ys = append(ys, y)
			}
			sort.Strings(ys)
			for _, y := range ys {
				line += fmt.Sprintf("  %s %+.4f (%d)", y, mean(byYear[y]), len(byYear[y]))
			}
			b, t := trendSlope(all, ts, h+5)
			line += fmt.Sprintf("  | trend %+.4f per year (NW t %+.2f)", b, t)
			fmt.Println(line)
		}
	}
	fmt.Println("\nspeed: Table B, market-speed proxies by calendar year (median over name-days); SpBBO and Trades are AlgoPack columns re-emitted by the exchange before 2025")
	type acc struct {
		turn, spb, trades, amih []float64
		rev                     []float64
	}
	by := map[string]*acc{}
	for d, date := range ds.dates {
		if date < "2021-04-08" || date > "2026-10-16" {
			continue
		}
		y := date[:4]
		if by[y] == nil {
			by[y] = &acc{}
		}
		a := by[y]
		rows := ds.byDate[d]
		var rv, nx []float64
		for _, x := range rows {
			if v := hd.sig(date, x.secid, "D02"); !math.IsNaN(v) {
				a.turn = append(a.turn, math.Exp(v))
			}
			if v := hd.sig(date, x.secid, "D01"); !math.IsNaN(v) {
				a.amih = append(a.amih, v)
			}
			if x.spb > 0 {
				a.spb = append(a.spb, x.spb)
			}
			if k, ok := fidx["Trades"]; ok && !math.IsNaN(x.raw[k]) {
				a.trades = append(a.trades, x.raw[k])
			}
			o := outcomeOf(x, 1)
			if !math.IsNaN(x.rb1) && !math.IsNaN(o) {
				rv, nx = append(rv, x.rb1), append(nx, o)
			}
		}
		if len(rv) >= 30 {
			a.rev = append(a.rev, pearson(rankScale(rv), rankScale(nx)))
		}
	}
	var ys []string
	for y := range by {
		ys = append(ys, y)
	}
	sort.Strings(ys)
	fmt.Printf("%-6s %16s %12s %14s %12s %18s\n", "year", "median turnover", "median spb", "median trades", "median amih", "1-day reversal IC")
	for _, y := range ys {
		a := by[y]
		fmt.Printf("%-6s %16.3g %12.2f %14.0f %12.3g %+10.4f (%d)\n", y, medianOf(a.turn), medianOf(a.spb), medianOf(a.trades), medianOf(a.amih), mean(clean(a.rev)), len(clean(a.rev)))
	}
}
