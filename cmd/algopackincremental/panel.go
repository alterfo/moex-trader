package main

import (
	"encoding/csv"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strconv"
)

func buildPanel(blockPath, outPath string, fromDate string, shuffleSeed int64) error {
	blk, err := loadBlock(blockPath, false)
	if err != nil {
		return err
	}
	hdr, rows, err := readCSV(blockPath)
	if err != nil {
		return err
	}
	ix := colIndex(hdr)
	inUni := map[blockKey]bool{}
	for _, rec := range rows {
		if rec[ix["adm100"]] == "1" {
			inUni[blockKey{rec[ix["date"]], rec[ix["secid"]]}] = true
		}
	}
	sub := map[blockKey]*blockRow{}
	for k, v := range blk {
		if inUni[k] {
			sub[k] = v
		}
	}
	allAbn, allEnd := abnormalReturns(blk, sub, 10)
	lab, ld := map[blockKey]string{}, map[blockKey]string{}
	abn := map[blockKey]float64{}
	for k := range sub {
		a, ok := allAbn[k]
		if !ok {
			continue
		}
		abn[k] = a
		ld[k] = allEnd[k]
		switch {
		case a > 0.005:
			lab[k] = "1"
		case a < -0.005:
			lab[k] = "0"
		}
	}
	byDate := map[string][]blockKey{}
	for k, br := range sub {
		if !br.ok || (fromDate != "" && k.date < fromDate) {
			continue
		}
		if _, ok := abn[k]; !ok {
			continue
		}
		byDate[k.date] = append(byDate[k.date], k)
	}
	dates := make([]string, 0, len(byDate))
	for d := range byDate {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if err := w.Write(append([]string{"ticker", "date", "label", "label_date", "split", "abn_ret"}, apNames...)); err != nil {
		return err
	}
	var rng *rand.Rand
	if shuffleSeed != 0 {
		rng = rand.New(rand.NewSource(shuffleSeed))
	}
	n := 0
	for _, d := range dates {
		g := byDate[d]
		if len(g) < 30 {
			continue
		}
		sort.Slice(g, func(a, b int) bool { return g[a].secid < g[b].secid })
		var mu [10]float64
		for _, k := range g {
			for c := range mu {
				mu[c] += sub[k].vals[c]
			}
		}
		for c := range mu {
			mu[c] /= float64(len(g))
		}
		vals := make([][10]float64, len(g))
		for i, k := range g {
			for c := range mu {
				vals[i][c] = sub[k].vals[c] - mu[c]
			}
		}
		if rng != nil {
			perm := rng.Perm(len(g))
			orig := append([][10]float64(nil), vals...)
			for i := range vals {
				vals[i] = orig[perm[i]]
			}
		}
		for i, k := range g {
			split := "train"
			if lab[k] == "" {
				split = "eval"
			}
			rec := []string{k.secid, k.date, lab[k], ld[k], split, strconv.FormatFloat(abn[k], 'g', 10, 64)}
			for c := range mu {
				rec = append(rec, strconv.FormatFloat(vals[i][c], 'g', 10, 64))
			}
			if err := w.Write(rec); err != nil {
				return err
			}
			n++
		}
	}
	w.Flush()
	fmt.Fprintf(os.Stderr, "panel: wrote %d rows over %d dates to %s\n", n, len(dates), outPath)
	return w.Error()
}

func abnormalReturns(blk map[blockKey]*blockRow, sub map[blockKey]*blockRow, horizon int) (map[blockKey]float64, map[blockKey]string) {
	var cal []string
	seen := map[string]bool{}
	for k := range blk {
		if !seen[k.date] {
			seen[k.date] = true
			cal = append(cal, k.date)
		}
	}
	sort.Strings(cal)
	calIdx := map[string]int{}
	for i, d := range cal {
		calIdx[d] = i
	}
	fwd := map[blockKey]float64{}
	end := map[blockKey]string{}
	byDate := map[string][]blockKey{}
	for k := range sub {
		if !sub[k].ok {
			continue
		}
		i := calIdx[k.date]
		if i+horizon >= len(cal) {
			continue
		}
		ed := cal[i+horizon]
		a, b := blk[k], blk[blockKey{ed, k.secid}]
		if a == nil || b == nil || !(a.close > 0) || !(b.close > 0) {
			continue
		}
		x := b.close/a.close - 1
		if x <= -0.45 || x >= 0.80 {
			continue
		}
		fwd[k] = x
		end[k] = ed
		byDate[k.date] = append(byDate[k.date], k)
	}
	out := map[blockKey]float64{}
	for _, ks := range byDate {
		if len(ks) < 30 {
			continue
		}
		m := 0.0
		for _, k := range ks {
			m += fwd[k]
		}
		m /= float64(len(ks))
		for _, k := range ks {
			out[k] = fwd[k] - m
		}
	}
	return out, end
}
