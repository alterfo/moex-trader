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
)

const (
	nSlots     = 106
	slotMid    = 35
	slotLate   = 83
	slotFeat   = 101
	minKeepVal = 2e6
)

var nan = math.NaN()

type tsBar struct{ open, close, val, valB, valS float32 }
type obBar struct{ spread, imb, imbBBO float32 }
type osBar struct{ putB, putS, cancB, cancS float32 }

type work struct {
	ts   [nSlots]tsBar
	tsOK [nSlots]bool
	ob   [nSlots]obBar
	obOK [nSlots]bool
	os   [nSlots]osBar
	osOK [nSlots]bool
}

type bars struct {
	close  [nSlots]float32
	imb    [nSlots]float32
	putB   [nSlots]float32
	putS   [nSlots]float32
	cancB  [nSlots]float32
	cancS  [nSlots]float32
	ts     [nSlots]bool
	joined [nSlots]bool
}

type agg struct {
	n                                   int
	val, valBS, imb, bbo, spread        float64
	netFlow, putSides, cancel, putTotal float64
	priceClose                          float64
	priceSlot                           int
}

type cut struct {
	tradeImb, bookImb, spread, price float64
	priceSlot, n                     int
}

type dayRec struct {
	day      int
	secid    string
	openDay  float64
	closeDay float64
	valDay   float64
	feat     [5]float64
	spread   float64
	nJoined  int
	mid      cut
	late     cut
	end      float64
	endSlot  int
	bars     *bars
}

type dayResult struct {
	date string
	recs []*dayRec
	hhi  float64
}

func slotOf(tm string) int {
	if len(tm) < 5 {
		return -1
	}
	h, e1 := strconv.Atoi(tm[0:2])
	m, e2 := strconv.Atoi(tm[3:5])
	if e1 != nil || e2 != nil {
		return -1
	}
	min := h*60 + m
	if min < 605 || min > 1130 || (min-605)%5 != 0 {
		return -1
	}
	return (min - 605) / 5
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

func aggregate(w *work, maxSlot int) agg {
	var a agg
	a.priceSlot = -1
	for s := 0; s <= maxSlot && s < nSlots; s++ {
		if w.tsOK[s] && w.ts[s].close > 0 {
			a.priceClose = float64(w.ts[s].close)
			a.priceSlot = s
		}
		if !(w.tsOK[s] && w.obOK[s] && w.osOK[s]) {
			continue
		}
		t, o, r := w.ts[s], w.ob[s], w.os[s]
		v := float64(t.val)
		a.n++
		a.val += v
		a.valBS += float64(t.valB) - float64(t.valS)
		a.imb += float64(o.imb) * v
		a.bbo += float64(o.imbBBO) * v
		a.spread += float64(o.spread)
		a.netFlow += (float64(r.putB) - float64(r.cancB)) - (float64(r.putS) - float64(r.cancS))
		a.putSides += float64(r.putB) + float64(r.putS)
		a.cancel += float64(r.cancB) + float64(r.cancS)
		a.putTotal += float64(r.putB) + float64(r.putS)
	}
	return a
}

func ratio(num, den float64) float64 {
	if den <= 0 {
		return nan
	}
	return num / den
}

func cutOf(a agg, minBars int) cut {
	c := cut{tradeImb: nan, bookImb: nan, spread: nan, price: a.priceClose, priceSlot: a.priceSlot, n: a.n}
	if a.n < minBars || a.val <= 0 {
		return c
	}
	c.tradeImb = a.valBS / a.val
	c.bookImb = a.imb / a.val
	c.spread = a.spread / float64(a.n)
	return c
}

func buildRec(day int, secid string, w *work) *dayRec {
	r := &dayRec{day: day, secid: secid, openDay: nan, closeDay: nan}
	first := true
	for s := 0; s < nSlots; s++ {
		if !w.tsOK[s] {
			continue
		}
		t := w.ts[s]
		r.valDay += float64(t.val)
		if first && t.open > 0 {
			r.openDay = float64(t.open)
			first = false
		}
		if t.close > 0 {
			r.closeDay = float64(t.close)
		}
	}
	full := aggregate(w, slotFeat)
	r.nJoined = full.n
	r.end = full.priceClose
	r.endSlot = full.priceSlot
	for i := range r.feat {
		r.feat[i] = nan
	}
	r.spread = nan
	if full.n >= 20 && full.val > 0 {
		r.feat[0] = full.valBS / full.val
		r.feat[1] = full.imb / full.val
		r.feat[2] = full.bbo / full.val
		r.feat[3] = ratio(full.netFlow, full.putSides)
		r.feat[4] = ratio(full.cancel, full.putTotal)
		r.spread = full.spread / float64(full.n)
	}
	r.mid = cutOf(aggregate(w, slotMid), 10)
	r.late = cutOf(aggregate(w, slotLate), 20)
	if r.valDay >= minKeepVal {
		b := &bars{}
		for s := 0; s < nSlots; s++ {
			b.close[s] = float32(nan)
			if w.tsOK[s] {
				b.ts[s] = true
				b.close[s] = w.ts[s].close
			}
			if w.tsOK[s] && w.obOK[s] && w.osOK[s] {
				b.joined[s] = true
				b.imb[s] = w.ob[s].imb
				b.putB[s] = w.os[s].putB
				b.putS[s] = w.os[s].putS
				b.cancB[s] = w.os[s].cancB
				b.cancS[s] = w.os[s].cancS
			}
		}
		r.bars = b
	}
	return r
}

func loadDay(dir, date string, dayIdx int) (dayResult, error) {
	res := dayResult{date: date, hhi: nan}
	works := map[string]*work{}
	get := func(sec string) *work {
		w := works[sec]
		if w == nil {
			w = &work{}
			works[sec] = w
		}
		return w
	}
	err := scanGz(filepath.Join(dir, "tradestats", date+".csv.gz"), func(ix map[string]int, f []string) {
		s := slotOf(f[ix["tradetime"]])
		if s < 0 {
			return
		}
		w := get(f[ix["secid"]])
		w.ts[s] = tsBar{pf(f[ix["pr_open"]]), pf(f[ix["pr_close"]]), pf(f[ix["val"]]), pf(f[ix["val_b"]]), pf(f[ix["val_s"]])}
		w.tsOK[s] = true
	})
	if err != nil {
		return res, fmt.Errorf("tradestats %s: %w", date, err)
	}
	if len(works) == 0 {
		return res, nil
	}
	err = scanGz(filepath.Join(dir, "obstats", date+".csv.gz"), func(ix map[string]int, f []string) {
		s := slotOf(f[ix["tradetime"]])
		if s < 0 {
			return
		}
		w := works[f[ix["secid"]]]
		if w == nil {
			return
		}
		w.ob[s] = obBar{pf(f[ix["spread_lv10"]]), pf(f[ix["imbalance_val"]]), pf(f[ix["imbalance_val_bbo"]])}
		w.obOK[s] = true
	})
	if err != nil {
		return res, fmt.Errorf("obstats %s: %w", date, err)
	}
	err = scanGz(filepath.Join(dir, "orderstats", date+".csv.gz"), func(ix map[string]int, f []string) {
		s := slotOf(f[ix["tradetime"]])
		if s < 0 {
			return
		}
		w := works[f[ix["secid"]]]
		if w == nil {
			return
		}
		w.os[s] = osBar{pf(f[ix["put_val_b"]]), pf(f[ix["put_val_s"]]), pf(f[ix["cancel_val_b"]]), pf(f[ix["cancel_val_s"]])}
		w.osOK[s] = true
	})
	if err != nil {
		return res, fmt.Errorf("orderstats %s: %w", date, err)
	}
	err = scanGz(filepath.Join(dir, "hi2", date+".csv.gz"), func(ix map[string]int, f []string) {
		if f[ix["secid"]] == "MARKET" && f[ix["metric"]] == "hhi_agressive" && f[ix["tradetime"]] == "18:40:00" {
			if v, e := strconv.ParseFloat(f[ix["value"]], 64); e == nil {
				res.hhi = v
			}
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
		res.recs = append(res.recs, buildRec(dayIdx, s, works[s]))
	}
	return res, nil
}
