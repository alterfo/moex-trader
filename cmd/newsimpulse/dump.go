package main

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
)

var shockGroups = []string{"zc", "zc_hi", "zc_lo", "zc_co", "zc_ag", "zc_div", "zc_san", "zc_res", "zcneg"}

var divStems = []string{"дивиденд"}
var sanStems = []string{"санкц", "ограничен", "эмбарго", "запрет"}
var resStems = []string{"прибыл", "выручк", "убыт", "отчётност", "отчетност"}

func hasAny(title string, stems []string) bool {
	for _, s := range stems {
		if strings.Contains(title, s) {
			return true
		}
	}
	return false
}

func groupsOf(it item, medTrust float64, nTick map[string]int) []int {
	g := []int{0}
	if it.trust >= medTrust {
		g = append(g, 1)
	} else {
		g = append(g, 2)
	}
	if nTick[it.id] >= 3 {
		g = append(g, 4)
	} else {
		g = append(g, 3)
	}
	if hasAny(it.title, divStems) {
		g = append(g, 5)
	}
	if hasAny(it.title, sanStems) {
		g = append(g, 6)
	}
	if hasAny(it.title, resStems) {
		g = append(g, 7)
	}
	return g
}

func medianTrust(items []item) float64 {
	v := make([]float64, len(items))
	for i, it := range items {
		v[i] = it.trust
	}
	sort.Float64s(v)
	return v[len(v)/2]
}

func dumpShocks(dates []string, byDate map[string][]cell, items []item, path string) error {
	nd := len(dates)
	nTick := map[string]int{}
	for _, it := range items {
		nTick[it.id]++
	}
	med := medianTrust(items)
	cnt := make([]map[string][]float64, 8)
	for g := range cnt {
		cnt[g] = map[string][]float64{}
	}
	cutT := make([]int64, nd)
	for i, d := range dates {
		cutT[i] = cutoff(d).Unix()
	}
	for _, it := range items {
		i := sort.Search(nd, func(k int) bool { return cutT[k] >= it.ts.Unix() })
		if i >= nd {
			continue
		}
		for _, g := range groupsOf(it, med, nTick) {
			a := cnt[g][it.ticker]
			if a == nil {
				a = make([]float64, nd)
				cnt[g][it.ticker] = a
			}
			a[i]++
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriterSize(f, 1<<22)
	defer w.Flush()
	fmt.Fprintf(w, "date,secid,%s\n", strings.Join(shockGroups, ","))
	zeros := make([]float64, nd)
	for i, d := range dates {
		for _, c := range byDate[d] {
			vals := make([]float64, len(shockGroups))
			for g := 0; g < 8; g++ {
				a := cnt[g][c.secid]
				if a == nil {
					a = zeros
				}
				vals[g] = trailingZ(a, i)
			}
			vals[8] = -vals[0]
			fmt.Fprintf(w, "%s,%s", d, c.secid)
			for _, v := range vals {
				if math.IsNaN(v) {
					fmt.Fprint(w, ",")
				} else {
					fmt.Fprintf(w, ",%g", v)
				}
			}
			fmt.Fprintln(w)
		}
	}
	return nil
}
