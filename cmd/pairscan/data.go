package main

import (
	"bufio"
	"encoding/csv"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
)

var nan = math.NaN()

type obs struct {
	rb1, f1, f5, f10, a2, sp float64
	div                      bool
}

type pairSpec struct {
	id, ord, pref string
	control       bool
}

var realPairs = []pairSpec{
	{"SBER", "SBER", "SBERP", false}, {"TATN", "TATN", "TATNP", false}, {"SNGS", "SNGS", "SNGSP", false},
	{"MTLR", "MTLR", "MTLRP", false}, {"BANE", "BANE", "BANEP", false}, {"RTKM", "RTKM", "RTKMP", false},
}

var nearPairs = []pairSpec{
	{"SBER-VTBR", "SBER", "VTBR", true}, {"LKOH-ROSN", "LKOH", "ROSN", true}, {"GAZP-NVTK", "GAZP", "NVTK", true},
	{"NLMK-MAGN", "NLMK", "MAGN", true}, {"MTSS-RTKM", "MTSS", "RTKM", true},
}

type panel struct {
	dates []string
	rows  map[string]map[string]obs
}

func f64(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nan
	}
	return v
}

func loadPanel(path string, want map[string]bool) (*panel, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(bufio.NewReaderSize(f, 1<<22))
	hdr, err := r.Read()
	if err != nil {
		return nil, err
	}
	ix := map[string]int{}
	for i, h := range hdr {
		ix[h] = i
	}
	p := &panel{rows: map[string]map[string]obs{}}
	seen := map[string]bool{}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		s := rec[ix["secid"]]
		if !want[s] {
			continue
		}
		d := rec[ix["date"]]
		if !seen[d] {
			seen[d] = true
			p.dates = append(p.dates, d)
		}
		if p.rows[d] == nil {
			p.rows[d] = map[string]obs{}
		}
		p.rows[d][s] = obs{rb1: f64(rec[ix["rb1"]]), f1: f64(rec[ix["f1"]]), f5: f64(rec[ix["f5"]]), f10: f64(rec[ix["f10"]]), a2: f64(rec[ix["A2"]]), sp: f64(rec[ix["SpBBO"]]), div: rec[ix["div_drop"]] == "1"}
	}
	sort.Strings(p.dates)
	return p, nil
}
