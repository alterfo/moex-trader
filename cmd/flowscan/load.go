package main

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	nMain    = 106
	nMorn    = 36
	nEve     = 57
	slotFeat = 101
	slotNear = 95
)

var nan = math.NaN()

const (
	tOpen = iota
	tHigh
	tLow
	tClose
	tStd
	tVol
	tVal
	tTrades
	tVwap
	tChg
	tTrB
	tTrS
	tValB
	tValS
	tVolB
	tVolS
	tVwB
	tVwS
	oSpBBO
	oSpLv
	oSp1m
	oLvB
	oLvS
	oVolB
	oVolS
	oValB
	oValS
	oImbVolBBO
	oImbValBBO
	oImbVol
	oImbVal
	oVwB1
	oVwS1
	sPutOrB
	sPutOrS
	sPutValB
	sPutValS
	sPutVolB
	sPutVolS
	sPutVwB
	sPutVwS
	sCanOrB
	sCanOrS
	sCanValB
	sCanValS
	sCanVolB
	sCanVolS
	sCanVwB
	sCanVwS
	nF
)

var tsCols = []string{"pr_open", "pr_high", "pr_low", "pr_close", "pr_std", "vol", "val", "trades", "pr_vwap", "pr_change", "trades_b", "trades_s", "val_b", "val_s", "vol_b", "vol_s", "pr_vwap_b", "pr_vwap_s"}
var obCols = []string{"spread_bbo", "spread_lv10", "spread_1mio", "levels_b", "levels_s", "vol_b", "vol_s", "val_b", "val_s", "imbalance_vol_bbo", "imbalance_val_bbo", "imbalance_vol", "imbalance_val", "vwap_b_1mio", "vwap_s_1mio"}
var osCols = []string{"put_orders_b", "put_orders_s", "put_val_b", "put_val_s", "put_vol_b", "put_vol_s", "put_vwap_b", "put_vwap_s", "cancel_orders_b", "cancel_orders_s", "cancel_val_b", "cancel_val_s", "cancel_vol_b", "cancel_vol_s", "cancel_vwap_b", "cancel_vwap_s"}

const (
	hasTS = iota
	hasOB
	hasOS
)

type bar struct {
	has [3]bool
	f   [nF]float32
}

type sessions struct {
	main [nMain]bar
	morn [nMorn]bar
	eve  [nEve]bar
}

type hiRow [11]float32

var hiNames = []string{"hhi_agressive", "hhi_agressive_buy", "hhi_agressive_sell", "hhi_buy", "hhi_sell", "hhi_netflow_buy", "hhi_netflow_sell", "hhi_passive", "hhi_passive_buy", "hhi_passive_sell", "hhi_volume"}

type dropStat struct {
	rows, dropped [3]int
}

type dayResult struct {
	date string
	recs map[string]*[nb]float32
	drop dropStat
	hi   map[string]*hiRow
}

func minutes(tm string) int {
	if len(tm) < 5 {
		return -1
	}
	h, e1 := strconv.Atoi(tm[0:2])
	m, e2 := strconv.Atoi(tm[3:5])
	if e1 != nil || e2 != nil {
		return -1
	}
	return h*60 + m
}

func locate(tm string) (int, int) {
	min := minutes(tm)
	switch {
	case min >= 605 && min <= 1130 && (min-605)%5 == 0:
		return 0, (min - 605) / 5
	case min >= 420 && min <= 595 && (min-420)%5 == 0:
		return 1, (min - 420) / 5
	case min >= 1145 && min <= 1425 && (min-1145)%5 == 0:
		return 2, (min - 1145) / 5
	}
	return -1, -1
}

func (s *sessions) at(kind, slot int) *bar {
	switch kind {
	case 0:
		return &s.main[slot]
	case 1:
		return &s.morn[slot]
	}
	return &s.eve[slot]
}

func splitFields(line string, dst []string) []string {
	dst = dst[:0]
	start := 0
	for i := 0; i < len(line); i++ {
		if line[i] == ',' {
			dst = append(dst, line[start:i])
			start = i + 1
		}
	}
	return append(dst, line[start:])
}

func scanGz(path string, fn func(idx map[string]int, f []string)) error {
	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fh.Close()
	zr, err := gzip.NewReader(fh)
	if err != nil {
		return err
	}
	defer zr.Close()
	sc := bufio.NewScanner(zr)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	if !sc.Scan() {
		return nil
	}
	header := strings.Split(sc.Text(), ",")
	idx := map[string]int{}
	for i, h := range header {
		idx[h] = i
	}
	buf := make([]string, 0, len(header)+2)
	for sc.Scan() {
		f := splitFields(sc.Text(), buf)
		if len(f) < len(header) {
			continue
		}
		fn(idx, f)
	}
	return sc.Err()
}

func pf(s string) float32 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return float32(v)
}

func cutoffOf(date string, lagDays int) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return "9999"
	}
	return t.AddDate(0, 0, lagDays).Format("2006-01-02")
}

var secAlias map[string]string

func canonSec(s string) string {
	if a, ok := secAlias[s]; ok {
		return a
	}
	return s
}

type loadOpts struct {
	dir     string
	lagDays int
}

func loadDay(o loadOpts, date string) (dayResult, error) {
	res := dayResult{date: date, recs: map[string]*[nb]float32{}, hi: map[string]*hiRow{}}
	works := map[string]*sessions{}
	cutoff := ""
	if o.lagDays > 0 {
		cutoff = cutoffOf(date, o.lagDays)
	}
	load := func(name string, cols []string, base, which int, create bool) error {
		path := filepath.Join(o.dir, name, date+".csv.gz")
		return scanGz(path, func(ix map[string]int, f []string) {
			res.drop.rows[which]++
			if cutoff != "" {
				if st, ok := ix["SYSTIME"]; ok && len(f[st]) >= 10 && f[st][:10] > cutoff {
					res.drop.dropped[which]++
					return
				}
			}
			kind, slot := locate(f[ix["tradetime"]])
			if kind < 0 {
				return
			}
			sec := canonSec(f[ix["secid"]])
			w := works[sec]
			if w == nil {
				if !create {
					return
				}
				w = &sessions{}
				works[sec] = w
			}
			b := w.at(kind, slot)
			b.has[which] = true
			for i, c := range cols {
				if j, ok := ix[c]; ok {
					b.f[base+i] = pf(f[j])
				}
			}
		})
	}
	if err := load("tradestats", tsCols, tOpen, hasTS, true); err != nil {
		return res, fmt.Errorf("tradestats %s: %w", date, err)
	}
	if len(works) == 0 {
		return res, nil
	}
	if err := load("obstats", obCols, oSpBBO, hasOB, false); err != nil {
		return res, fmt.Errorf("obstats %s: %w", date, err)
	}
	if err := load("orderstats", osCols, sPutOrB, hasOS, false); err != nil {
		return res, fmt.Errorf("orderstats %s: %w", date, err)
	}
	hiIdx := map[string]int{}
	for i, n := range hiNames {
		hiIdx[n] = i
	}
	err := scanGz(filepath.Join(o.dir, "hi2", date+".csv.gz"), func(ix map[string]int, f []string) {
		if f[ix["tradetime"]] != "18:40:00" {
			return
		}
		k, ok := hiIdx[f[ix["metric"]]]
		if !ok {
			return
		}
		if cutoff != "" {
			if st, ok2 := ix["SYSTIME"]; ok2 && len(f[st]) >= 10 && f[st][:10] > cutoff {
				return
			}
		}
		sec := canonSec(f[ix["secid"]])
		r := res.hi[sec]
		if r == nil {
			r = &hiRow{}
			for i := range r {
				r[i] = float32(nan)
			}
			res.hi[sec] = r
		}
		v, e := strconv.ParseFloat(f[ix["value"]], 64)
		if e == nil {
			r[k] = float32(v)
		}
	})
	if err != nil {
		return res, fmt.Errorf("hi2 %s: %w", date, err)
	}
	secs := make([]string, 0, len(works))
	for s := range works {
		secs = append(secs, s)
	}
	sort.Strings(secs)
	for _, s := range secs {
		if r := computeBase(works[s]); r != nil {
			res.recs[s] = r
		}
	}
	return res, nil
}
