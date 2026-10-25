package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"strings"
)

var candFile string

func readCandidates(path string) (names []string, vals map[string][]float64) {
	f, err := os.Open(path)
	if err != nil {
		fatal(err)
	}
	defer f.Close()
	r := csv.NewReader(bufio.NewReaderSize(f, 1<<22))
	hdr, err := r.Read()
	if err != nil {
		fatal(err)
	}
	names = hdr[2:]
	vals = map[string][]float64{}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			fatal(err)
		}
		v := make([]float64, len(names))
		for i := range names {
			v[i] = parseF(rec[2+i])
		}
		vals[rec[0]+"|"+rec[1]] = v
	}
	return names, vals
}

func libExt(pp *pipe, frzPath string) {
	ds := pp.ds
	names, cv := readCandidates(candFile)
	raw, err := readFile(frzPath)
	if err != nil {
		fatal(err)
	}
	var fz frozen
	unmarshal(raw, &fz)
	t0 := sortSearch(ds.dates, "2020-04-01")
	r := newRunner(pp, fz, []string{"blend"})
	r.keep = map[int][]float64{}
	blocks := [][2]string{{"2025-01-03", "2025-03-31"}, {"2025-04-01", "2025-06-30"}, {"2025-07-01", "2025-09-30"}, {"2025-10-01", "2025-12-31"}, {"2026-01-01", "2026-03-31"}, {"2026-04-01", "2026-06-30"}, {"2026-07-01", "2026-10-16"}}
	for _, b := range blocks {
		lo, hi := pp.dateRange(b[0], b[1])
		r.block(lo, hi, t0, nil, false)
	}
	lo, _ := pp.dateRange("2025-01-03", "2026-10-16")
	_, hi := pp.dateRange("2025-01-03", "2026-10-16")
	split := sortSearch(ds.dates, "2026-01-05") - lo
	total := len(names) + 5
	label := append([]string{}, names...)
	for k := 0; k < 5; k++ {
		label = append(label, fmt.Sprintf("placebo%d (%s permuted)", k+1, names[0]))
	}
	ic := make([][]float64, total)
	bs := make([][]float64, total)
	for c := range ic {
		ic[c], bs[c] = make([]float64, hi-lo+1), make([]float64, hi-lo+1)
		for i := range ic[c] {
			ic[c][i], bs[c][i] = nan, nan
		}
	}
	for d := lo; d <= hi; d++ {
		rows := ds.byDate[d]
		pr := r.keep[d]
		if pr == nil {
			continue
		}
		cols := make([][]float64, total)
		for c := range cols {
			cols[c] = make([]float64, len(rows))
			for i := range cols[c] {
				cols[c][i] = nan
			}
		}
		for i, x := range rows {
			if v, ok := cv[ds.dates[d]+"|"+x.secid]; ok {
				for c := range names {
					cols[c][i] = v[c]
				}
			}
		}
		var who []int
		for i := range rows {
			if !math.IsNaN(cols[0][i]) {
				who = append(who, i)
			}
		}
		for k := 0; k < 5; k++ {
			rng := rand.New(rand.NewSource(int64(20261030+k)*1000003 + int64(d)))
			perm := rng.Perm(len(who))
			for j, i := range who {
				cols[len(names)+k][i] = cols[0][who[perm[j]]]
			}
		}
		for c := 0; c < total; c++ {
			var y, p, s []float64
			for i, x := range rows {
				if math.IsNaN(cols[c][i]) || math.IsNaN(x.y10) || math.IsNaN(pr[i]) {
					continue
				}
				y, p, s = append(y, x.y10), append(p, pr[i]), append(s, cols[c][i])
			}
			if len(y) < 30 {
				continue
			}
			p = rankScale(p)
			s = rankScale(s)
			ic[c][d-lo] = pearson(s, y)
			my, mp, ms := mean(y), mean(p), mean(s)
			var spp, sss, sps, spy, ssy float64
			for i := range y {
				a, b, z := p[i]-mp, s[i]-ms, y[i]-my
				spp += a * a
				sss += b * b
				sps += a * b
				spy += a * z
				ssy += b * z
			}
			if den := spp*sss - sps*sps; den > 1e-12 {
				bs[c][d-lo] = (spp*ssy - sps*spy) / den
			}
		}
	}
	fmt.Printf("libext: %d candidate columns from %s, %d placebo permutations of the first column\n", len(names), candFile, 5)
	fmt.Printf("%-34s %9s %7s %10s %7s %9s %9s %s\n", "candidate", "IC", "t", "inc.slope", "t", "half1", "half2", "flags")
	hits, ctl := 0, 0
	for c := 0; c < total; c++ {
		icv, bsv := seqStat(ic[c], 0, hi-lo), seqStat(bs[c], 0, hi-lo)
		if len(bsv) < 100 {
			fmt.Printf("%-34s not testable (%d dates)\n", label[c], len(bsv))
			continue
		}
		mi, ti := neweyWest(icv, 10)
		mb, tb := neweyWest(bsv, 10)
		h1, h2 := halves(bs[c], split)
		flags := ""
		if math.Abs(tb) >= 3.0 && h1*mb > 0 && h2*mb > 0 && mi*mb > 0 {
			flags = "HIT"
			if c < len(names) {
				hits++
			} else {
				ctl++
			}
		}
		fmt.Printf("%-34s %+9.5f %+7.2f %+10.5f %+7.2f %+9.5f %+9.5f %s\n", strings.TrimSpace(label[c]), mi, ti, mb, tb, h1, h2, flags)
	}
	fmt.Printf("libext: hits %d of %d candidates, placebo hits %d of 5 (rule: |t| of the incremental slope at least 3.0, same sign in both halves and for the IC)\n", hits, len(names), ctl)
}
