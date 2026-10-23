package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"time"
)

const (
	dFrom      = "2022-06-01"
	dSplit     = "2024-07-31"
	dHold      = 5
	dCost      = 0.0003
	dMinWindow = 250
	dLags      = 2
)

func fetchIMOEX(from, till string) ([]string, map[string]float64, error) {
	var dates []string
	closes := map[string]float64{}
	for start := 0; ; {
		url := fmt.Sprintf("https://iss.moex.com/iss/history/engines/stock/markets/index/boards/SNDX/securities/IMOEX.json?from=%s&till=%s&iss.meta=off&history.columns=TRADEDATE,CLOSE&start=%d", from, till, start)
		var out struct {
			History struct {
				Data [][]interface{} `json:"data"`
			} `json:"history"`
		}
		var last error
		ok := false
		for attempt := 0; attempt < 5 && !ok; attempt++ {
			resp, err := http.Get(url)
			if err != nil {
				last = err
				time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
				continue
			}
			data, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				last = fmt.Errorf("http %d", resp.StatusCode)
				time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
				continue
			}
			if err := json.Unmarshal(data, &out); err != nil {
				last = err
				continue
			}
			ok = true
		}
		if !ok {
			return nil, nil, last
		}
		if len(out.History.Data) == 0 {
			break
		}
		for _, r := range out.History.Data {
			d, _ := r[0].(string)
			c, _ := r[1].(float64)
			if d != "" && c > 0 {
				dates = append(dates, d)
				closes[d] = c
			}
		}
		start += len(out.History.Data)
	}
	sort.Strings(dates)
	return dates, closes, nil
}

type dOut struct {
	periods, nonzero int
	meanDiff, t      float64
	hit              float64
	mean0, mean1     float64
	n0, n1           int
	pass             bool
}

func runD(cal []string, closes map[string]float64, hhi map[string]float64) dOut {
	z := map[string]float64{}
	var n int
	var m, m2 float64
	var hdates []string
	for d := range hhi {
		hdates = append(hdates, d)
	}
	sort.Strings(hdates)
	for _, d := range hdates {
		x := hhi[d]
		if math.IsNaN(x) {
			continue
		}
		n++
		delta := x - m
		m += delta / float64(n)
		m2 += delta * (x - m)
		if n >= dMinWindow {
			sd := math.Sqrt(m2 / float64(n-1))
			if sd > 0 {
				z[d] = (x - m) / sd
			}
		}
	}
	start := sort.SearchStrings(cal, dFrom)
	var diffs, d0, d1 []float64
	prevPos := 0.0
	hits, nz := 0, 0
	for k := start; k+dHold < len(cal); k += dHold {
		zv, ok := z[cal[k]]
		if !ok {
			continue
		}
		pos := 0.0
		if zv > 0 {
			pos = -1
		} else if zv < 0 {
			pos = 1
		}
		r := math.Log(closes[cal[k+dHold]] / closes[cal[k]])
		cost := dCost * math.Abs(pos-prevPos)
		prevPos = pos
		x := pos*r - cost - r
		diffs = append(diffs, x)
		if cal[k] <= dSplit {
			d0 = append(d0, x)
		} else {
			d1 = append(d1, x)
		}
		if pos != 0 {
			nz++
			if pos*r > 0 {
				hits++
			}
		}
	}
	var o dOut
	o.periods, o.nonzero = len(diffs), nz
	o.meanDiff, o.t = neweyWest(diffs, dLags)
	if nz > 0 {
		o.hit = float64(hits) / float64(nz)
	}
	o.mean0, o.mean1, o.n0, o.n1 = mean(d0), mean(d1), len(d0), len(d1)
	o.pass = !math.IsNaN(o.t) && o.t >= tThresh && o.hit >= 0.54 && o.mean0 > 0 && o.mean1 > 0
	return o
}

func reportD(w io.Writer, o dOut) verdict {
	fmt.Fprintf(w, "D hhi_agressive (MARKET, contrarian): periods %d (nonzero %d), mean strategy-minus-buy-and-hold %s per period, NW t %+.2f, hit rate %.1f%%, halves %s (n %d) / %s (n %d) -> %s\n",
		o.periods, o.nonzero, pct(o.meanDiff), o.t, o.hit*100, pct(o.mean0), o.n0, pct(o.mean1), o.n1, map[bool]string{true: "PASS", false: "FAIL"}[o.pass])
	return verdict{name: "D", pass: o.pass}
}
