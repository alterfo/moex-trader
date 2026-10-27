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
)

type trade struct {
	date, month string
	z, sp, net  float64
	gross, cost float64
}

func tradesOf(sr []*series, hold int, costMult float64, flat bool) []trade {
	var out []trade
	for _, s := range sr {
		if s.spec.control {
			continue
		}
		y := s.y5
		switch hold {
		case 1:
			y = s.y1
		case 10:
			y = s.y10
		}
		for i := 0; i < s.n; {
			d := s.dates[i]
			if d >= split || math.IsNaN(s.z[i]) || math.IsNaN(y[i]) || math.Abs(s.z[i]) < entryZ {
				i++
				continue
			}
			sign := -1.0
			if s.z[i] < 0 {
				sign = 1
			}
			g := sign * y[i]
			cost := 2 * (sideCost(s.spOrd[i]) + sideCost(s.spPref[i])) * costMult
			if flat {
				cost = 2 * (0.0015 + 0.0015)
			}
			bor := borrowPerCalendarDay * float64(hold) * float64(calendarDaysPerHold) / float64(holdDays)
			sp := (s.spOrd[i] + s.spPref[i]) / 2
			out = append(out, trade{date: d, month: d[:7], z: math.Abs(s.z[i]), sp: sp, net: g - cost - bor, gross: g, cost: cost})
			i += hold
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].date < out[b].date })
	return out
}

func readSeriesCSV(path string) ([]string, []float64) {
	f, err := os.Open(path)
	if err != nil {
		panic(err)
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
			panic(err)
		}
		x, err := strconv.ParseFloat(rec[1], 64)
		if err != nil {
			continue
		}
		d, v = append(d, rec[0]), append(v, x)
	}
	return d, v
}

func regimeOn(macroDir, name, date string) bool {
	switch name {
	case "imoex200":
		d, v := readSeriesCSV(filepath.Join(macroDir, "imoex.csv"))
		j := sort.SearchStrings(d, date) - 1
		if j < 200 {
			return false
		}
		m := 0.0
		for _, x := range v[j-199 : j+1] {
			m += x
		}
		return v[j] < m/200
	}
	d, v := readSeriesCSV(filepath.Join(macroDir, "rvi.csv"))
	j := sort.SearchStrings(d, date) - 1
	if j < 60 {
		return false
	}
	w := append([]float64{}, v[j-59:j+1]...)
	sort.Float64s(w)
	return v[j] > w[30]
}

func groupDiff(tr []trade, grp []int) float64 {
	var a, b []float64
	for i, g := range grp {
		switch g {
		case 1:
			a = append(a, tr[i].net)
		case -1:
			b = append(b, tr[i].net)
		}
	}
	if len(a) < 5 || len(b) < 5 {
		return nan
	}
	return mean(a) - mean(b)
}

func monthBoot(tr []trade, grp []int, draws int, seed int64, alpha float64) (float64, float64) {
	byMonth := map[string][]int{}
	var months []string
	for i, t := range tr {
		if _, ok := byMonth[t.month]; !ok {
			months = append(months, t.month)
		}
		byMonth[t.month] = append(byMonth[t.month], i)
	}
	rng := rand.New(rand.NewSource(seed))
	var ds []float64
	for b := 0; b < draws; b++ {
		var sub []trade
		var sg []int
		for range months {
			m := months[rng.Intn(len(months))]
			for _, i := range byMonth[m] {
				sub = append(sub, tr[i])
				sg = append(sg, grp[i])
			}
		}
		if v := groupDiff(sub, sg); !math.IsNaN(v) {
			ds = append(ds, v)
		}
	}
	if len(ds) < 100 {
		return nan, nan
	}
	return percentile(ds, alpha/2), percentile(ds, 1-alpha/2)
}

func splitLine(id string, tr []trade, grp []int, alpha float64, seed int64) string {
	point := groupDiff(tr, grp)
	if math.IsNaN(point) {
		return fmt.Sprintf("%-60s not testable", id)
	}
	lo, hi := monthBoot(tr, grp, 4000, seed, alpha)
	var h1t, h2t []trade
	var h1g, h2g []int
	for i, t := range tr {
		if t.date < "2022-07-01" {
			h1t, h1g = append(h1t, t), append(h1g, grp[i])
		} else {
			h2t, h2g = append(h2t, t), append(h2g, grp[i])
		}
	}
	d1, d2 := groupDiff(h1t, h1g), groupDiff(h2t, h2g)
	var a, b []float64
	for i, g := range grp {
		if g == 1 {
			a = append(a, tr[i].net)
		} else if g == -1 {
			b = append(b, tr[i].net)
		}
	}
	hits := 0
	for p := 0; p < 5; p++ {
		pg := append([]int{}, grp...)
		rng := rand.New(rand.NewSource(seed + int64(p) + 1))
		rng.Shuffle(len(pg), func(i, j int) { pg[i], pg[j] = pg[j], pg[i] })
		plo, phi := monthBoot(tr, pg, 1500, seed+int64(p)+10, alpha)
		if plo > 0 || phi < 0 {
			hits++
		}
	}
	hit := (lo > 0 || hi < 0) && d1*point > 0 && d2*point > 0 && hits == 0
	word := "no hit"
	if hit {
		word = "HIT (lead)"
	}
	return fmt.Sprintf("%-60s diff %+.3f%% [%+.3f%%, %+.3f%%] halves %+.3f%% / %+.3f%% groups %+.3f%% (%d) vs %+.3f%% (%d) placebo hits %d of 5 %s", id, point*100, lo*100, hi*100, d1*100, d2*100, mean(a)*100, len(a), mean(b)*100, len(b), hits, word)
}

func levelLine(id string, tr []trade) string {
	var net, gross, cost []float64
	for _, t := range tr {
		net, gross, cost = append(net, t.net), append(gross, t.gross), append(cost, t.cost)
	}
	if len(net) < 10 {
		return fmt.Sprintf("%-60s not testable", id)
	}
	lo, hi := bootMean(net, 5000, 20261027)
	return fmt.Sprintf("%-60s trades %d gross %+.3f%% cost %+.3f%% net %+.3f%% [%+.3f%%, %+.3f%%] break-even cost multiplier %.2f", id, len(net), mean(gross)*100, mean(cost)*100, mean(net)*100, lo*100, hi*100, mean(gross)/math.Max(1e-9, mean(cost)))
}

func pairSplits(sr []*series, macroDir string) {
	base := tradesOf(sr, holdDays, 1, false)
	alpha := 0.05 / 4
	fmt.Printf("pairscan splits: %d baseline trades (|z| >= %.1f, hold %d, 2020-2024, broker carry), Bonferroni-adjusted level %.4f, month-cluster bootstrap, halves split 2022-07-01\n", len(base), entryZ, holdDays, alpha)
	sps := make([]float64, len(base))
	for i, t := range base {
		sps[i] = t.sp
	}
	sorted := append([]float64{}, sps...)
	sort.Float64s(sorted)
	lo3, hi3 := sorted[len(sorted)/3], sorted[2*len(sorted)/3]
	g73 := make([]int, len(base))
	for i, t := range base {
		switch {
		case t.sp >= hi3:
			g73[i] = 1
		case t.sp <= lo3:
			g73[i] = -1
		}
	}
	fmt.Println(splitLine("73 pair liquidity: wide spread minus tight spread", base, g73, alpha, 20261401))
	for k, name := range []string{"imoex200", "rvi60"} {
		g := make([]int, len(base))
		for i, t := range base {
			if regimeOn(macroDir, name, t.date) {
				g[i] = 1
			} else {
				g[i] = -1
			}
		}
		fmt.Println(splitLine("74 regime on minus off: "+name, base, g, alpha, int64(20261402+k)))
	}
	g75 := make([]int, len(base))
	for i, t := range base {
		switch {
		case t.z >= 2.5:
			g75[i] = 1
		case t.z < 2.0:
			g75[i] = -1
		}
	}
	fmt.Println(splitLine("75 extremeness: |z| >= 2.5 minus |z| < 2.0", base, g75, alpha, 20261404))
	for _, h := range []int{1, 5, 10} {
		fmt.Println(levelLine(fmt.Sprintf("76 holding period %d day(s), bbo costs", h), tradesOf(sr, h, 1, false)))
	}
	fmt.Println(levelLine("77a bbo costs x 1.0 (as run)", base))
	fmt.Println(levelLine("77b bbo costs x 1.5 (close spread wider than the day mean)", tradesOf(sr, holdDays, 1.5, false)))
	fmt.Println(levelLine("77c flat 0.15% a side on both legs", tradesOf(sr, holdDays, 1, true)))
}
