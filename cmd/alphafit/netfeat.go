package main

import (
	"bufio"
	"encoding/json"
	"math"
	"os"
	"sort"
	"strings"
)

func (ds *dataset) colIndex(name string) int {
	for i, n := range ds.names {
		if n == name {
			return i
		}
	}
	return -1
}

func (ds *dataset) addNetwork() {
	k := ds.colIndex("A2")
	ds.names = append(ds.names, "A2PR")
	for _, x := range ds.rows {
		x.raw = append(x.raw, nan)
	}
	const win = 20
	for d := win; d < len(ds.dates); d++ {
		names := map[string]int{}
		var list []string
		series := map[string][]float64{}
		for t := d - win + 1; t <= d; t++ {
			for _, x := range ds.byDate[t] {
				if _, ok := series[x.secid]; !ok {
					series[x.secid] = make([]float64, win)
					for q := range series[x.secid] {
						series[x.secid][q] = nan
					}
				}
				series[x.secid][t-(d-win+1)] = x.raw[k]
			}
		}
		for s, v := range series {
			c := 0
			for _, a := range v {
				if !math.IsNaN(a) {
					c++
				}
			}
			if c >= 15 {
				list = append(list, s)
			}
		}
		if len(list) < 40 {
			continue
		}
		sort.Strings(list)
		for i, s := range list {
			names[s] = i
		}
		n := len(list)
		w := make([][]float64, n)
		for i := range w {
			w[i] = make([]float64, n)
		}
		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				c := corrPairs(series[list[i]], series[list[j]])
				if !math.IsNaN(c) {
					w[i][j], w[j][i] = math.Abs(c), math.Abs(c)
				}
			}
		}
		pr := make([]float64, n)
		for i := range pr {
			pr[i] = 1 / float64(n)
		}
		col := make([]float64, n)
		for i := 0; i < n; i++ {
			for j := 0; j < n; j++ {
				col[i] += w[i][j]
			}
		}
		for it := 0; it < 40; it++ {
			nx := make([]float64, n)
			for j := 0; j < n; j++ {
				if col[j] == 0 {
					continue
				}
				share := pr[j] / col[j]
				for i := 0; i < n; i++ {
					nx[i] += w[i][j] * share
				}
			}
			for i := range nx {
				nx[i] = 0.15/float64(n) + 0.85*nx[i]
			}
			pr = nx
		}
		for _, x := range ds.byDate[d] {
			if i, ok := names[x.secid]; ok {
				x.raw[len(x.raw)-1] = pr[i] * float64(n)
			}
		}
	}
}

func corrPairs(a, b []float64) float64 {
	var xs, ys []float64
	for i := range a {
		if !math.IsNaN(a[i]) && !math.IsNaN(b[i]) {
			xs = append(xs, a[i])
			ys = append(ys, b[i])
		}
	}
	if len(xs) < 12 {
		return nan
	}
	return pearson(xs, ys)
}

func (ds *dataset) addDivFlow(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	alias := map[string]string{"YNDX": "YDEX", "TCSG": "T"}
	type ev struct{ decl, last string }
	events := map[string][]ev{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var r struct {
			Ticker string `json:"ticker"`
			Decl   string `json:"declared_date"`
			Last   string `json:"last_buy_date"`
		}
		if json.Unmarshal([]byte(sc.Text()), &r) != nil || len(r.Last) < 10 {
			continue
		}
		t := strings.TrimSpace(r.Ticker)
		if a, ok := alias[t]; ok {
			t = a
		}
		d := ""
		if len(r.Decl) >= 10 {
			d = r.Decl[:10]
		}
		events[t] = append(events[t], ev{d, r.Last[:10]})
	}
	k := ds.colIndex("UnexpA2")
	ds.names = append(ds.names, "DivFlow")
	byKey := map[string][]float64{}
	for _, x := range ds.rows {
		x.raw = append(x.raw, nan)
		if _, ok := byKey[x.secid]; !ok {
			byKey[x.secid] = make([]float64, len(ds.dates))
			for i := range byKey[x.secid] {
				byKey[x.secid][i] = nan
			}
		}
		byKey[x.secid][x.di] = x.raw[k]
	}
	for _, x := range ds.rows {
		for _, e := range events[x.secid] {
			if e.decl != "" && e.decl > x.date {
				continue
			}
			li := sort.SearchStrings(ds.dates, e.last)
			if li >= len(ds.dates) {
				continue
			}
			ahead := li - x.di
			if ahead < 1 || ahead > 5 {
				continue
			}
			var s float64
			c := 0
			for t := x.di - 2; t <= x.di; t++ {
				if t >= 0 {
					if v := byKey[x.secid][t]; !math.IsNaN(v) {
						s += v
						c++
					}
				}
			}
			if c >= 2 {
				x.raw[len(x.raw)-1] = s / float64(c)
			}
		}
	}
	return nil
}
