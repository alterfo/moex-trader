package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
)

var apNames = []string{"ap_a2", "ap_a2_m5", "ap_a1", "ap_a4", "ap_a5", "ap_lev_asym", "ap_spread_ratio", "ap_depth_growth", "ap_ticket_z", "ap_otr_z"}

type blockRow struct {
	vals  [10]float64
	ok    bool
	close float64
}

type blockKey struct{ date, secid string }

func readCSV(path string) ([]string, [][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	r := csv.NewReader(bufio.NewReaderSize(f, 1<<20))
	r.FieldsPerRecord = -1
	hdr, err := r.Read()
	if err != nil {
		return nil, nil, err
	}
	var rows [][]string
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		rows = append(rows, rec)
	}
	return hdr, rows, nil
}

func colIndex(hdr []string) map[string]int {
	m := make(map[string]int, len(hdr))
	for i, h := range hdr {
		m[h] = i
	}
	return m
}

func loadBlock(path string, onlyT18 bool) (map[blockKey]*blockRow, error) {
	hdr, rows, err := readCSV(path)
	if err != nil {
		return nil, err
	}
	ix := colIndex(hdr)
	out := make(map[blockKey]*blockRow, len(rows))
	for _, rec := range rows {
		if onlyT18 && rec[ix["t18"]] != "1" {
			continue
		}
		br := &blockRow{ok: true}
		for k, n := range apNames {
			s := rec[ix[n]]
			if s == "" {
				br.ok = false
				continue
			}
			v, err := strconv.ParseFloat(s, 64)
			if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
				br.ok = false
				continue
			}
			br.vals[k] = v
		}
		if c, err := strconv.ParseFloat(rec[ix["close"]], 64); err == nil {
			br.close = c
		}
		out[blockKey{rec[ix["date"]], rec[ix["secid"]]}] = br
	}
	return out, nil
}

type mergeOpts struct {
	shuffleSeed int64
	fromDate    string
	demean      bool
	abnormal    bool
}

func abnormalLabels(blk map[blockKey]*blockRow, horizon int, band float64, strict bool) (map[blockKey]string, map[blockKey]string) {
	calIdx := map[string]int{}
	if strict {
		var cal []string
		seen := map[string]bool{}
		for k := range blk {
			if !seen[k.date] {
				seen[k.date] = true
				cal = append(cal, k.date)
			}
		}
		sort.Strings(cal)
		for i, d := range cal {
			calIdx[d] = i
		}
	}
	byT := map[string][]string{}
	for k := range blk {
		byT[k.secid] = append(byT[k.secid], k.date)
	}
	fwd := map[blockKey]float64{}
	endDate := map[blockKey]string{}
	byDate := map[string][]blockKey{}
	for t, ds := range byT {
		sort.Strings(ds)
		for i := 0; i+horizon < len(ds); i++ {
			a, b := blk[blockKey{ds[i], t}], blk[blockKey{ds[i+horizon], t}]
			if a == nil || b == nil || !(a.close > 0) || !(b.close > 0) {
				continue
			}
			x := b.close/a.close - 1
			if x <= -0.45 || x >= 0.80 {
				continue
			}
			if strict && calIdx[ds[i+horizon]]-calIdx[ds[i]] != horizon {
				continue
			}
			k := blockKey{ds[i], t}
			fwd[k] = x
			endDate[k] = ds[i+horizon]
			byDate[ds[i]] = append(byDate[ds[i]], k)
		}
	}
	lab := map[blockKey]string{}
	ld := map[blockKey]string{}
	for _, ks := range byDate {
		if len(ks) < 8 {
			continue
		}
		m := 0.0
		for _, k := range ks {
			m += fwd[k]
		}
		m /= float64(len(ks))
		for _, k := range ks {
			a := fwd[k] - m
			switch {
			case a > band:
				lab[k] = "1"
			case a < -band:
				lab[k] = "0"
			default:
				continue
			}
			ld[k] = endDate[k]
		}
	}
	return lab, ld
}

func merge(basePath, blockPath, outPath string, o mergeOpts) error {
	shuffleSeed, fromDate := o.shuffleSeed, o.fromDate
	blk, err := loadBlock(blockPath, true)
	if err != nil {
		return err
	}
	hdr, rows, err := readCSV(basePath)
	if err != nil {
		return err
	}
	ix := colIndex(hdr)
	var abLab, abEnd map[blockKey]string
	if o.abnormal {
		abLab, abEnd = abnormalLabels(blk, 10, 0.005, false)
	}
	type kept struct {
		rec  []string
		vals [10]float64
	}
	byDate := map[string][]*kept{}
	var order []*kept
	per := map[string][2]int{}
	for _, rec := range rows {
		t, d := rec[ix["ticker"]], rec[ix["date"]]
		if fromDate != "" && d < fromDate {
			continue
		}
		c := per[t]
		c[0]++
		br := blk[blockKey{d, t}]
		if br == nil || !br.ok {
			per[t] = c
			continue
		}
		if o.abnormal {
			key := blockKey{d, t}
			l, ok := abLab[key]
			if !ok {
				per[t] = c
				continue
			}
			rec = append([]string{}, rec...)
			rec[ix["label"]] = l
			rec[ix["label_date"]] = abEnd[key]
			rec[ix["split"]] = "train"
		}
		c[1]++
		per[t] = c
		k := &kept{rec: rec, vals: br.vals}
		byDate[d] = append(byDate[d], k)
		order = append(order, k)
	}
	if o.demean {
		for _, g := range byDate {
			if len(g) < 5 {
				for _, k := range g {
					k.rec = nil
				}
				continue
			}
			var mu [10]float64
			for _, k := range g {
				for c := range mu {
					mu[c] += k.vals[c]
				}
			}
			for c := range mu {
				mu[c] /= float64(len(g))
			}
			for _, k := range g {
				for c := range mu {
					k.vals[c] -= mu[c]
				}
			}
		}
		var live []*kept
		for _, k := range order {
			if k.rec != nil {
				live = append(live, k)
			}
		}
		order = live
	}
	if shuffleSeed != 0 {
		rng := rand.New(rand.NewSource(shuffleSeed))
		dates := make([]string, 0, len(byDate))
		for d := range byDate {
			dates = append(dates, d)
		}
		sort.Strings(dates)
		for _, d := range dates {
			g := byDate[d]
			perm := rng.Perm(len(g))
			orig := make([][10]float64, len(g))
			for i := range g {
				orig[i] = g[i].vals
			}
			for i := range g {
				g[i].vals = orig[perm[i]]
			}
		}
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	nh := append(append([]string{}, hdr...), apNames...)
	if err := w.Write(nh); err != nil {
		return err
	}
	for _, k := range order {
		rec := append([]string{}, k.rec...)
		for _, v := range k.vals {
			rec = append(rec, strconv.FormatFloat(v, 'g', 10, 64))
		}
		if err := w.Write(rec); err != nil {
			return err
		}
	}
	w.Flush()
	tick := make([]string, 0, len(per))
	for t := range per {
		tick = append(tick, t)
	}
	sort.Strings(tick)
	for _, t := range tick {
		fmt.Fprintf(os.Stderr, "merge %s: %d base rows, %d with a full block\n", t, per[t][0], per[t][1])
	}
	fmt.Fprintf(os.Stderr, "merge: wrote %d of %d rows to %s\n", len(order), len(rows), outPath)
	return w.Error()
}

func trimDate(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return strings.TrimSpace(s)
}
