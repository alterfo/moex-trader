package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func buildHi2Panel(dir, out string) error {
	files, _ := filepath.Glob(filepath.Join(dir, "hi2", "*.csv.gz"))
	sort.Strings(files)
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()
	fmt.Fprintln(w, "date,secid,asset,metric,value,late")
	n := 0
	for _, path := range files {
		date := strings.TrimSuffix(filepath.Base(path), ".csv.gz")
		err := scanGz(path, func(ix map[string]int, rec []string) {
			if rec[ix["tradetime"]] != "18:50:00" {
				return
			}
			asset := rec[ix["secid"]]
			if i, ok := ix["asset_code"]; ok {
				asset = rec[i]
			}
			late := 0
			if i, ok := ix["SYSTIME"]; ok && lateDays(rec[i], date, 30) {
				late = 1
			}
			fmt.Fprintf(w, "%s,%s,%s,%s,%s,%d\n", date, rec[ix["secid"]], asset, rec[ix["metric"]], rec[ix["value"]], late)
			n++
		})
		if err != nil {
			return err
		}
	}
	fmt.Fprintf(os.Stderr, "futscan: hi2 panel %d rows to %s\n", n, out)
	return nil
}

type hi2Map map[string]float64

func loadHi2(path string) (hi2Map, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(bufio.NewReaderSize(f, 1<<20))
	hdr, err := r.Read()
	if err != nil {
		return nil, err
	}
	ix := map[string]int{}
	for i, h := range hdr {
		ix[h] = i
	}
	out := hi2Map{}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if !allowLate && rec[ix["late"]] == "1" {
			continue
		}
		out[rec[ix["secid"]]+"|"+rec[ix["date"]]+"|"+rec[ix["metric"]]] = f64(rec[ix["value"]])
	}
	return out, nil
}

func (h hi2Map) at(s *store, g string, d int, metric string) float64 {
	sec := s.frontSec[g][d]
	if sec == "" {
		return nan
	}
	if v, ok := h[sec+"|"+s.dates[d]+"|"+metric]; ok {
		return v
	}
	return nan
}

func (h hi2Map) series(s *store, g string, expr string) []float64 {
	out := make([]float64, len(s.dates))
	for d := range out {
		out[d] = nan
		switch expr {
		case "agr_diff":
			a, b := h.at(s, g, d, "hhi_agressive_buy"), h.at(s, g, d, "hhi_agressive_sell")
			if !isNaN(a) && !isNaN(b) {
				out[d] = a - b
			}
		case "net_diff":
			a, b := h.at(s, g, d, "hhi_netflow_buy"), h.at(s, g, d, "hhi_netflow_sell")
			if !isNaN(a) && !isNaN(b) {
				out[d] = a - b
			}
		default:
			out[d] = h.at(s, g, d, expr)
		}
	}
	return out
}
