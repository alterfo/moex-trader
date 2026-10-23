package main

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/macro"
)

func stateMeans(s, y []float64) (float64, int, float64, int) {
	var a, b float64
	var na, nb int
	for i := range s {
		if math.IsNaN(s[i]) || math.IsNaN(y[i]) {
			continue
		}
		if s[i] == 1 {
			a += y[i]
			na++
		} else {
			b += y[i]
			nb++
		}
	}
	return a / float64(na), na, b / float64(nb), nb
}

func stdev(a []float64) float64 {
	if len(a) < 2 {
		return math.NaN()
	}
	m := mean(a)
	var ss float64
	for _, x := range a {
		ss += (x - m) * (x - m)
	}
	return math.Sqrt(ss / float64(len(a)-1))
}

func diagnostics(dir string, dp *dump, ss *stateSet, mx *matrix, exA2Path string) error {
	f08 := ss.values["F08"]
	fmt.Println("== Status 1 diagnostics (descriptive, no verdict): everything split by the F08 state (RVI above its trailing median)")
	var names []string
	for k := range dp.cols {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		if k == "u10" {
			continue
		}
		a, na, b, nb := stateMeans(f08, dp.cols[k])
		fmt.Printf("D %-18s state=1 %+.5f (n=%d)  state=0 %+.5f (n=%d)  diff %+.5f\n", k, a, na, b, nb, a-b)
	}
	net := dp.cols["net_gp_blend"]
	var on, onN, all float64
	var allN int
	for i := range net {
		if math.IsNaN(net[i]) {
			continue
		}
		all += net[i]
		allN++
		if f08[i] == 0 {
			on += net[i]
			onN++
		}
	}
	fmt.Printf("D01 on-off (trade only when RVI low, in-sample): total net %+.5f%% per day over all %d dates, on-off total %+.5f%% over all dates (%d of them traded), always-on %+.5f%%\n", all/float64(allN)*100, allN, on/float64(allN)*100, int(onN), all/float64(allN)*100)
	long := dp.cols["glong_gp_blend"]
	short := dp.cols["gshort_gp_blend"]
	_, _, _, _ = long, short, 0, 0
	for _, nm := range []string{"glong_gp_blend", "gshort_gp_blend"} {
		a, na, b, nb := stateMeans(f08, dp.cols[nm])
		fmt.Printf("D02 %-16s gross per day: RVI high %+.5f%% (n=%d), RVI low %+.5f%% (n=%d)\n", nm, a*100, na, b*100, nb)
	}
	ex, err := loadDump(exA2Path)
	if err == nil && len(ex.dates) == len(dp.dates) {
		for _, k := range []string{"ic_blend", "net_gp_blend"} {
			a, na, b, nb := stateMeans(f08, ex.cols[k])
			fmt.Printf("D03 A2 family zeroed, %-13s RVI high %+.5f (n=%d), RVI low %+.5f (n=%d), mean all %+.5f\n", k, a, na, b, nb, mean(clean2(ex.cols[k])))
		}
	} else {
		fmt.Println("D03 skipped: ex-A2 dump missing")
	}

	rvi, err := macro.LoadCSV(dir + "/rvi.csv")
	if err != nil {
		return err
	}
	imoex, err := macro.LoadCSV(dir + "/imoex.csv")
	if err != nil {
		return err
	}
	var mdates []time.Time
	disp := make([]float64, len(mx.dates))
	ew := make([]float64, len(mx.dates))
	for i, d := range mx.dates {
		t, _ := time.Parse(dayFmt, d)
		mdates = append(mdates, t)
		var rs []float64
		for _, r := range mx.byDay[d] {
			if v, ok := mx.ret[r.secid][d]; ok && !math.IsNaN(v) {
				rs = append(rs, v)
			}
		}
		disp[i], ew[i] = math.NaN(), math.NaN()
		if len(rs) >= 30 {
			disp[i] = stdev(rs)
			ew[i] = mean(rs)
		}
	}
	disp5 := make([]float64, len(disp))
	vol10 := make([]float64, len(disp))
	for i := range disp {
		disp5[i], vol10[i] = math.NaN(), math.NaN()
		if i >= 5 {
			var a []float64
			for k := i - 4; k <= i; k++ {
				if !math.IsNaN(disp[k]) {
					a = append(a, disp[k])
				}
			}
			if len(a) >= 4 {
				disp5[i] = mean(a)
			}
		}
		if i >= 10 {
			var a []float64
			for k := i - 9; k <= i; k++ {
				if !math.IsNaN(ew[k]) {
					a = append(a, ew[k])
				}
			}
			if len(a) >= 8 {
				vol10[i] = stdev(a)
			}
		}
	}
	imoexVol := rollVol(logChanges(imoex), 10)
	volByDate := map[string]float64{}
	for i, p := range imoex {
		volByDate[p.Date.Format(dayFmt)] = imoexVol[i] * math.Sqrt(252) * 100
	}
	gap := make([]float64, len(rvi))
	for i, p := range rvi {
		gap[i] = math.NaN()
		if v, ok := volByDate[p.Date.Format(dayFmt)]; ok && !math.IsNaN(v) {
			gap[i] = p.Value - v
		}
	}
	lastMatrix := func(d time.Time) int {
		return sort.Search(len(mdates), func(k int) bool { return !mdates[k].Before(d) }) - 1
	}
	V := map[string][]float64{}
	for _, id := range []string{"V1", "V2", "V3", "V4"} {
		V[id] = make([]float64, len(dp.dates))
	}
	for i, d := range dp.dates {
		V["V1"][i] = highFlag(disp5, lastMatrix(d))
		V["V2"][i] = highFlag(vol10, lastMatrix(d))
		j := lastBefore(rvi, d)
		V["V3"][i] = math.NaN()
		if j >= 10 {
			if rvi[j].Value > rvi[j-10].Value {
				V["V3"][i] = 1
			} else {
				V["V3"][i] = 0
			}
		}
		V["V4"][i] = highFlag(gap, j)
	}
	fmt.Println("D06 alternative volatility states (differences only, no p): composite IC and partial-adjustment net")
	for _, id := range []string{"V1", "V2", "V3", "V4"} {
		s := V[id]
		agree, tot := 0, 0
		for i := range s {
			if !math.IsNaN(s[i]) && !math.IsNaN(f08[i]) {
				tot++
				if s[i] == f08[i] {
					agree++
				}
			}
		}
		dic, n1, n0 := diffMeans(s, dp.cols["ic_blend"])
		dnet, _, _ := diffMeans(s, dp.cols["net_gp_blend"])
		a2, _, _ := diffMeans(s, dp.cols["ic_a2"])
		fmt.Printf("D06 %s: n1 %d n0 %d, D(ic_blend) %+.5f, D(ic_a2) %+.5f, D(net_gp) %+.5f, agreement with F08 %.0f%%\n", id, n1, n0, dic, a2, dnet, 100*float64(agree)/float64(tot))
	}
	return nil
}

func clean2(v []float64) []float64 {
	var o []float64
	for _, x := range v {
		if !math.IsNaN(x) {
			o = append(o, x)
		}
	}
	return o
}
