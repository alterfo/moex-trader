package main

import (
	"bufio"
	"encoding/csv"
	"io"
	"math"
	"os"
	"strconv"
)

var (
	horizon   = 10
	costMode  = "flat"
	entryMode = "close"
)

func prepCosts(ds *dataset) {
	spb, sp1 := -1, -1
	for k, n := range ds.names {
		if n == "SpBBO" {
			spb = k
		}
		if n == "Sp1m" {
			sp1 = k
		}
	}
	for _, x := range ds.rows {
		x.spb, x.sp1, x.idy1 = nan, nan, nan
		if spb >= 0 {
			x.spb = x.raw[spb]
		}
		if sp1 >= 0 {
			x.sp1 = x.raw[sp1]
		}
	}
}

func (x *row) sideCost() float64 {
	var s float64
	switch costMode {
	case "bbo":
		s = x.spb
	case "mio":
		s = x.sp1
	default:
		return 0.0015
	}
	if math.IsNaN(s) || s <= 0 {
		return 0.0015
	}
	return 0.0005 + 0.5*s/1e4
}

func loadExt(ds *dataset, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := csv.NewReader(bufio.NewReaderSize(f, 1<<22))
	hdr, err := r.Read()
	if err != nil {
		return err
	}
	ix := map[string]int{}
	for i, h := range hdr {
		ix[h] = i
	}
	type key struct{ d, s string }
	m := map[key][]float64{}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		v := make([]float64, 6)
		for k, c := range []string{"f20", "f40", "u20", "u40", "ovn1", "idy1"} {
			v[k] = f64(rec[ix[c]])
		}
		m[key{rec[ix["date"]], rec[ix["secid"]]}] = v
	}
	for _, x := range ds.rows {
		v, ok := m[key{x.date, x.secid}]
		if !ok {
			continue
		}
		x.idy1 = v[5]
		switch horizon {
		case 20:
			x.f10, x.u10 = v[0], v[2]
		case 40:
			x.f10, x.u10 = v[1], v[3]
		}
	}
	return nil
}

func parseF(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nan
	}
	return v
}

func bootBlock() int {
	if horizon > 10 {
		return horizon
	}
	return 10
}
