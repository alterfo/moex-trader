package main

import (
	"fmt"
	"io"
	"math"
	"sort"
)

func ksDistance(a, b []float64) float64 {
	sort.Float64s(a)
	sort.Float64s(b)
	i, j := 0, 0
	var d float64
	for i < len(a) && j < len(b) {
		x := math.Min(a[i], b[j])
		for i < len(a) && a[i] <= x {
			i++
		}
		for j < len(b) && b[j] <= x {
			j++
		}
		d = math.Max(d, math.Abs(float64(i)/float64(len(a))-float64(j)/float64(len(b))))
	}
	return d
}

func runKS(w io.Writer, blockPath string, loA, hiA, loB, hiB string, limit float64) error {
	hdr, rows, err := readCSV(blockPath)
	if err != nil {
		return err
	}
	ix := colIndex(hdr)
	trig := false
	fmt.Fprintf(w, "ks: %s..%s against %s..%s, T18 tickers pooled, trigger above %.2f\n", loA, hiA, loB, hiB, limit)
	for _, n := range apNames {
		var a, b []float64
		for _, rec := range rows {
			if rec[ix["t18"]] != "1" || rec[ix[n]] == "" {
				continue
			}
			d := rec[ix["date"]]
			var v float64
			if _, err := fmt.Sscan(rec[ix[n]], &v); err != nil {
				continue
			}
			if d >= loA && d <= hiA {
				a = append(a, v)
			} else if d >= loB && d <= hiB {
				b = append(b, v)
			}
		}
		k := ksDistance(a, b)
		fmt.Fprintf(w, "ks %-16s n %d / %d distance %.4f\n", n, len(a), len(b), k)
		if k > limit {
			trig = true
		}
	}
	fmt.Fprintf(w, "ks trigger (realtime-only primary arm): %v\n", trig)
	return nil
}
