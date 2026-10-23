package main

import (
	"bufio"
	"compress/gzip"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const (
	slotLo   = 10*60 + 5
	slotMain = 18*60 + 50
	slotFeat = 18*60 + 30
	nSlots   = (slotMain-slotLo)/5 + 1
)

type fbar struct {
	has                                [3]bool
	close, open                        float64
	val, trades, valB, valS            float64
	mid, spread1                       float64
	depthB, depthS                     float64
	putValB, putValS, canValB, canValS float64
}

type secDay struct {
	secid, asset string
	bars         [nSlots]fbar
	late, rows   int
	lateD, rowsD [3]int
}

func minutesOf(tm string) int {
	if len(tm) < 5 {
		return -1
	}
	h, e1 := strconv.Atoi(tm[:2])
	m, e2 := strconv.Atoi(tm[3:5])
	if e1 != nil || e2 != nil {
		return -1
	}
	return h*60 + m
}

func pf(s string) float64 {
	if s == "" {
		return math.NaN()
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return math.NaN()
	}
	return v
}

func scanGz(path string, fn func(ix map[string]int, f []string)) error {
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
	r := csv.NewReader(bufio.NewReaderSize(zr, 1<<20))
	r.FieldsPerRecord = -1
	hdr, err := r.Read()
	if err != nil {
		if err == io.EOF {
			return nil
		}
		return err
	}
	ix := make(map[string]int, len(hdr))
	for i, h := range hdr {
		ix[h] = i
	}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		fn(ix, rec)
	}
}

func lateDays(sys, date string, lim int) bool {
	if len(sys) < 10 || len(date) < 10 {
		return false
	}
	return sys[:10] > addDays(date, lim)
}

func addDays(d string, n int) string {
	y, _ := strconv.Atoi(d[:4])
	m, _ := strconv.Atoi(d[5:7])
	dd, _ := strconv.Atoi(d[8:10])
	days := daysFromCivil(y, m, dd) + n
	yy, mm, d2 := civilFromDays(days)
	return fmt.Sprintf("%04d-%02d-%02d", yy, mm, d2)
}

func daysFromCivil(y, m, d int) int {
	if m <= 2 {
		y--
	}
	era := y / 400
	yoe := y - era*400
	mp := (m + 9) % 12
	doy := (153*mp+2)/5 + d - 1
	doe := yoe*365 + yoe/4 - yoe/100 + doy
	return era*146097 + doe - 719468
}

func civilFromDays(z int) (int, int, int) {
	z += 719468
	era := z / 146097
	doe := z - era*146097
	yoe := (doe - doe/1460 + doe/36524 - doe/146096) / 365
	y := yoe + era*400
	doy := doe - (365*yoe + yoe/4 - yoe/100)
	mp := (5*doy + 2) / 153
	d := doy - (153*mp+2)/5 + 1
	m := mp + 3
	if m > 12 {
		m -= 12
	}
	if m <= 2 {
		y++
	}
	return y, m, d
}

func loadSecDays(dir, date string) (map[string]*secDay, error) {
	out := map[string]*secDay{}
	slotOf := func(tm string) int {
		m := minutesOf(tm)
		if m < slotLo || m > slotMain || (m-slotLo)%5 != 0 {
			return -1
		}
		return (m - slotLo) / 5
	}
	get := func(ix map[string]int, f []string, create bool) (*secDay, int) {
		s := slotOf(f[ix["tradetime"]])
		if s < 0 {
			return nil, -1
		}
		sec := f[ix["secid"]]
		sd := out[sec]
		if sd == nil {
			if !create {
				return nil, -1
			}
			sd = &secDay{secid: sec}
			if i, ok := ix["asset_code"]; ok {
				sd.asset = f[i]
			} else {
				sd.asset = sec
			}
			out[sec] = sd
		}
		return sd, s
	}
	g := func(ix map[string]int, f []string, name string) float64 {
		i, ok := ix[name]
		if !ok || i >= len(f) {
			return math.NaN()
		}
		return pf(f[i])
	}
	late := func(ix map[string]int, f []string, sd *secDay, k int) {
		sd.rows++
		sd.rowsD[k]++
		if i, ok := ix["SYSTIME"]; ok && i < len(f) && lateDays(f[i], date, 30) {
			sd.late++
			sd.lateD[k]++
		}
	}
	err := scanGz(filepath.Join(dir, "tradestats", date+".csv.gz"), func(ix map[string]int, f []string) {
		sd, s := get(ix, f, true)
		if sd == nil {
			return
		}
		b := &sd.bars[s]
		b.has[0] = true
		b.open, b.close = g(ix, f, "pr_open"), g(ix, f, "pr_close")
		b.val, b.trades, b.valB, b.valS = g(ix, f, "val"), g(ix, f, "trades"), g(ix, f, "val_b"), g(ix, f, "val_s")
		late(ix, f, sd, 0)
	})
	if err != nil {
		return nil, err
	}
	err = scanGz(filepath.Join(dir, "obstats", date+".csv.gz"), func(ix map[string]int, f []string) {
		sd, s := get(ix, f, false)
		if sd == nil {
			return
		}
		b := &sd.bars[s]
		b.has[1] = true
		b.mid, b.spread1 = g(ix, f, "mid_price"), g(ix, f, "spread_l1")
		b.depthB, b.depthS = g(ix, f, "vol_b_l10"), g(ix, f, "vol_s_l10")
		late(ix, f, sd, 1)
	})
	if err != nil {
		return nil, err
	}
	err = scanGz(filepath.Join(dir, "orderstats", date+".csv.gz"), func(ix map[string]int, f []string) {
		sd, s := get(ix, f, false)
		if sd == nil {
			return
		}
		b := &sd.bars[s]
		b.has[2] = true
		b.putValB, b.putValS = g(ix, f, "put_val_b"), g(ix, f, "put_val_s")
		b.canValB, b.canValS = g(ix, f, "cancel_val_b"), g(ix, f, "cancel_val_s")
		late(ix, f, sd, 2)
	})
	return out, err
}

type dayRow struct {
	date, secid, asset                string
	valDay, closeMain, openMain       float64
	bi, ti, nof, ci, depth, relSpread float64
	ticket, otr, rv                   float64
	nJoined, nTrade                   int
	lateShare                         float64
	lateTS, lateOB, lateOS            float64
}

func nz(x float64) float64 {
	if math.IsNaN(x) {
		return 0
	}
	return x
}

func summarize(date string, sd *secDay) (dayRow, bool) {
	r := dayRow{date: date, secid: sd.secid, asset: sd.asset}
	nan := math.NaN()
	r.bi, r.ti, r.nof, r.ci, r.depth, r.relSpread, r.ticket, r.otr, r.rv = nan, nan, nan, nan, nan, nan, nan, nan, nan
	r.closeMain, r.openMain = nan, nan
	var closes []float64
	firstOpen := true
	for s := 0; s < nSlots; s++ {
		b := &sd.bars[s]
		if b.has[0] {
			r.valDay += nz(b.val)
			if b.close > 0 {
				r.closeMain = b.close
				closes = append(closes, b.close)
			}
			if firstOpen && b.open > 0 {
				r.openMain = b.open
				firstOpen = false
			}
			r.nTrade++
		}
	}
	if sd.rows > 0 {
		r.lateShare = float64(sd.late) / float64(sd.rows)
	}
	sh := func(k int) float64 {
		if sd.rowsD[k] == 0 {
			return 1
		}
		return float64(sd.lateD[k]) / float64(sd.rowsD[k])
	}
	r.lateTS, r.lateOB, r.lateOS = sh(0), sh(1), sh(2)
	if len(closes) >= 15 {
		sq := 0.0
		for i := 1; i < len(closes); i++ {
			x := math.Log(closes[i] / closes[i-1])
			sq += x * x
		}
		r.rv = math.Sqrt(sq)
	}
	var wsum, bi, vt, tvb, tvs, tpb, tcan, tput, tticks, ttr float64
	var dep, depN, sp, spN float64
	for s := 0; s < nSlots; s++ {
		if slotLo+5*s > slotFeat {
			break
		}
		b := &sd.bars[s]
		if !(b.has[0] && b.has[1] && b.has[2]) {
			continue
		}
		r.nJoined++
		vt += nz(b.val)
		tvb += nz(b.valB)
		tvs += nz(b.valS)
		tticks += nz(b.trades)
		tput += nz(b.putValB) + nz(b.putValS)
		tcan += nz(b.canValB) + nz(b.canValS)
		tpb += (nz(b.putValB) - nz(b.canValB)) - (nz(b.putValS) - nz(b.canValS))
		den := b.depthB + b.depthS
		if den > 0 && b.val > 0 {
			bi += b.val * (b.depthB - b.depthS) / den
			wsum += b.val
		}
		if den > 0 {
			dep += den
			depN++
		}
		if b.mid > 0 && b.spread1 >= 0 && !math.IsNaN(b.spread1) {
			sp += b.spread1 / b.mid
			spN++
		}
		ttr += 0
	}
	if r.nJoined < 20 {
		return r, false
	}
	if wsum > 0 {
		r.bi = bi / wsum
	}
	if vt > 0 {
		r.ti = (tvb - tvs) / vt
		r.otr = tput / vt
	}
	if tput > 0 {
		r.nof = tpb / tput
		r.ci = tcan / tput
	}
	if depN > 0 {
		r.depth = dep / depN
	}
	if spN > 0 {
		r.relSpread = sp / spN
	}
	if tticks > 0 && vt > 0 {
		r.ticket = vt / tticks
	}
	return r, true
}

func buildPanel(dir, market, from, till, out string, workers int) error {
	files, _ := filepath.Glob(filepath.Join(dir, "tradestats", "*.csv.gz"))
	var days []string
	for _, f := range files {
		d := strings.TrimSuffix(filepath.Base(f), ".csv.gz")
		if d >= from && d <= till {
			days = append(days, d)
		}
	}
	sort.Strings(days)
	res := make([][]dayRow, len(days))
	var wg sync.WaitGroup
	sem := make(chan struct{}, workers)
	for i, d := range days {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, d string) {
			defer wg.Done()
			defer func() { <-sem }()
			m, err := loadSecDays(dir, d)
			if err != nil {
				log.Fatalf("futscan: %s %v", d, err)
			}
			var secs []string
			for s := range m {
				secs = append(secs, s)
			}
			sort.Strings(secs)
			for _, s := range secs {
				if r, ok := summarize(d, m[s]); ok {
					res[i] = append(res[i], r)
				}
			}
		}(i, d)
	}
	wg.Wait()
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()
	fmt.Fprintln(w, "date,secid,asset,val_day,close_main,open_main,bi,ti,nof,ci,depth,rel_spread,ticket,otr,rv,n_joined,n_trade,late_share,late_ts,late_ob,late_os")
	n := 0
	ff := func(x float64) string {
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return ""
		}
		return strconv.FormatFloat(x, 'g', 10, 64)
	}
	for _, rows := range res {
		for _, r := range rows {
			fmt.Fprintf(w, "%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%d,%d,%s,%s,%s,%s\n", r.date, r.secid, r.asset, ff(r.valDay), ff(r.closeMain), ff(r.openMain), ff(r.bi), ff(r.ti), ff(r.nof), ff(r.ci), ff(r.depth), ff(r.relSpread), ff(r.ticket), ff(r.otr), ff(r.rv), r.nJoined, r.nTrade, ff(r.lateShare), ff(r.lateTS), ff(r.lateOB), ff(r.lateOS))
			n++
		}
	}
	log.Printf("futscan: %s panel %d days, %d rows to %s", market, len(days), n, out)
	return nil
}
