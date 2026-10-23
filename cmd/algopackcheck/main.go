package main

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type dayStat struct {
	rows, zeroVal int
	tickers       map[string]int
	lastClose     map[string]float64
	lastTime      map[string]string
}

func openGz(path string) (*bufio.Scanner, func(), error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	zr, err := gzip.NewReader(f)
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	sc := bufio.NewScanner(zr)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	return sc, func() { zr.Close(); f.Close() }, nil
}

func inMain(tm string) bool {
	return tm >= "10:05:00" && tm <= "18:50:00"
}

func scanTradestats(path string) (dayStat, error) {
	st := dayStat{tickers: map[string]int{}, lastClose: map[string]float64{}, lastTime: map[string]string{}}
	sc, closeFn, err := openGz(path)
	if err != nil {
		return st, err
	}
	defer closeFn()
	sc.Scan()
	header := strings.Split(sc.Text(), ",")
	idx := map[string]int{}
	for i, h := range header {
		idx[h] = i
	}
	for sc.Scan() {
		f := strings.Split(sc.Text(), ",")
		if len(f) < len(header) {
			continue
		}
		st.rows++
		tm, sec := f[idx["tradetime"]], f[idx["secid"]]
		val, _ := strconv.ParseFloat(f[idx["val"]], 64)
		if val == 0 {
			st.zeroVal++
		}
		if !inMain(tm) {
			continue
		}
		st.tickers[sec]++
		if tm >= st.lastTime[sec] {
			if c, err := strconv.ParseFloat(f[idx["pr_close"]], 64); err == nil && c > 0 {
				st.lastTime[sec] = tm
				st.lastClose[sec] = c
			}
		}
	}
	return st, sc.Err()
}

func countRows(path string) (rows int, tickers map[string]bool, metrics map[string]int, err error) {
	sc, closeFn, err := openGz(path)
	if err != nil {
		return 0, nil, nil, err
	}
	defer closeFn()
	tickers, metrics = map[string]bool{}, map[string]int{}
	sc.Scan()
	header := strings.Split(sc.Text(), ",")
	mi := -1
	for i, h := range header {
		if h == "metric" {
			mi = i
		}
	}
	for sc.Scan() {
		f := strings.Split(sc.Text(), ",")
		if len(f) < 3 {
			continue
		}
		rows++
		tickers[f[2]] = true
		if mi >= 0 && mi < len(f) {
			metrics[f[mi]]++
		}
	}
	return rows, tickers, metrics, sc.Err()
}

func issGet(url string, v interface{}) error {
	var last error
	for attempt := 0; attempt < 5; attempt++ {
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
		return json.Unmarshal(data, v)
	}
	return last
}

func issCalendar(from, till string) ([]string, error) {
	var days []string
	for start := 0; ; {
		var out struct {
			History struct {
				Data [][]interface{} `json:"data"`
			} `json:"history"`
		}
		url := fmt.Sprintf("https://iss.moex.com/iss/history/engines/stock/markets/index/boards/SNDX/securities/IMOEX.json?from=%s&till=%s&iss.meta=off&history.columns=TRADEDATE&start=%d", from, till, start)
		if err := issGet(url, &out); err != nil {
			return nil, err
		}
		if len(out.History.Data) == 0 {
			return days, nil
		}
		for _, r := range out.History.Data {
			d, _ := r[0].(string)
			days = append(days, d)
		}
		start += len(out.History.Data)
	}
}

func issCloses(secid, from, till string) (map[string]float64, error) {
	closes := map[string]float64{}
	for start := 0; ; {
		var out struct {
			Candles struct {
				Data [][]interface{} `json:"data"`
			} `json:"candles"`
		}
		url := fmt.Sprintf("https://iss.moex.com/iss/engines/stock/markets/shares/boards/TQBR/securities/%s/candles.json?from=%s&till=%s&interval=24&iss.meta=off&start=%d", secid, from, till, start)
		if err := issGet(url, &out); err != nil {
			return nil, err
		}
		if len(out.Candles.Data) == 0 {
			return closes, nil
		}
		for _, r := range out.Candles.Data {
			c, _ := r[1].(float64)
			b, _ := r[6].(string)
			if len(b) >= 10 {
				closes[b[:10]] = c
			}
		}
		start += len(out.Candles.Data)
	}
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	sort.Float64s(v)
	return v[len(v)/2]
}

func main() {
	dir := flag.String("dir", "data/algopack", "AlgoPack directory")
	sample := flag.String("sample", "SBER,LKOH,GAZP,GMKN,ROSN,NVTK,TATN,MTSS,MGNT,PLZL,CHMF,VTBR,AFLT,ALRS,MAGN,NLMK", "tickers for the ISS close comparison")
	flag.Parse()

	dates, err := filepath.Glob(filepath.Join(*dir, "tradestats", "*.csv.gz"))
	if err != nil || len(dates) == 0 {
		log.Fatal("algopackcheck: no tradestats files")
	}
	var days []string
	for _, p := range dates {
		days = append(days, strings.TrimSuffix(filepath.Base(p), ".csv.gz"))
	}
	sort.Strings(days)
	first, last := days[0], days[len(days)-1]

	cal, err := issCalendar(first, last)
	if err != nil {
		log.Fatal(err)
	}
	calSet := map[string]bool{}
	for _, d := range cal {
		calSet[d] = true
	}

	stats := map[string]dayStat{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for _, d := range days {
		wg.Add(1)
		sem <- struct{}{}
		go func(d string) {
			defer wg.Done()
			defer func() { <-sem }()
			st, err := scanTradestats(filepath.Join(*dir, "tradestats", d+".csv.gz"))
			if err != nil {
				log.Printf("algopackcheck: tradestats %s: %v", d, err)
			}
			mu.Lock()
			stats[d] = st
			mu.Unlock()
		}(d)
	}
	wg.Wait()

	fmt.Println("== Calendar (tradestats files that carry rows vs ISS IMOEX trading days)")
	var missing, extra []string
	have := map[string]bool{}
	for d, st := range stats {
		if st.rows > 0 {
			have[d] = true
			if !calSet[d] {
				extra = append(extra, d)
			}
		}
	}
	for _, d := range cal {
		if !have[d] {
			missing = append(missing, d)
		}
	}
	sort.Strings(extra)
	sort.Strings(missing)
	fmt.Printf("ISS trading days %d, days with rows %d, missing %d, extra %d\n", len(cal), len(have), len(missing), len(extra))
	fmt.Printf("missing: %v\nextra: %v\n", missing, extra)

	fmt.Println("\n== Per year (tradestats, main session 10:05..18:50 bars)")
	fmt.Println("year,days,median_tickers,median_bars_per_ticker_day,zero_val_share")
	years := map[string][]string{}
	for d, st := range stats {
		if st.rows > 0 {
			years[d[:4]] = append(years[d[:4]], d)
		}
	}
	var ys []string
	for y := range years {
		ys = append(ys, y)
	}
	sort.Strings(ys)
	for _, y := range ys {
		var nt, bars []float64
		zero, rows := 0, 0
		for _, d := range years[y] {
			st := stats[d]
			nt = append(nt, float64(len(st.tickers)))
			for _, b := range st.tickers {
				bars = append(bars, float64(b))
			}
			zero += st.zeroVal
			rows += st.rows
		}
		fmt.Printf("%s,%d,%.0f,%.0f,%.5f\n", y, len(years[y]), median(nt), median(bars), float64(zero)/float64(rows))
	}

	fmt.Println("\n== Other datasets: rows and tickers against tradestats (days with rows)")
	for _, ds := range []string{"obstats", "orderstats", "hi2"} {
		var days0, rowDiff, tickDiff int
		metrics := map[string]map[string]int{}
		for _, d := range days {
			rows, tk, mt, err := countRows(filepath.Join(*dir, ds, d+".csv.gz"))
			if err != nil {
				log.Printf("algopackcheck: %s %s: %v", ds, d, err)
				continue
			}
			if (rows > 0) != (stats[d].rows > 0) {
				days0++
			}
			if ds != "hi2" && rows > 0 {
				ratio := float64(rows) / float64(stats[d].rows)
				if ratio < 0.9 || ratio > 1.1 {
					rowDiff++
				}
				if len(tk) < len(stats[d].tickers)*9/10 {
					tickDiff++
				}
			}
			for m, n := range mt {
				if metrics[m] == nil {
					metrics[m] = map[string]int{}
				}
				metrics[m][d[:4]] += n
			}
		}
		fmt.Printf("%s: days where presence differs from tradestats %d, days with rows differing >10%% %d, days with <90%% of tradestats tickers %d\n", ds, days0, rowDiff, tickDiff)
		if ds == "hi2" {
			var names []string
			for m := range metrics {
				names = append(names, m)
			}
			sort.Strings(names)
			fmt.Println("hi2 metric,rows by year")
			for _, m := range names {
				var parts []string
				for _, y := range ys {
					parts = append(parts, fmt.Sprintf("%s:%d", y, metrics[m][y]))
				}
				fmt.Printf("%s,%s\n", m, strings.Join(parts, " "))
			}
		}
	}

	fmt.Println("\n== Last main-session bar close vs ISS TQBR daily close")
	fmt.Println("ticker,days,median_abs_dev_pct,days_above_1pct,days_above_5pct")
	for _, sec := range strings.Split(*sample, ",") {
		closes, err := issCloses(sec, first, last)
		if err != nil {
			log.Printf("algopackcheck: iss %s: %v", sec, err)
			continue
		}
		var devs []float64
		over1, over5 := 0, 0
		for _, d := range days {
			a, ok1 := stats[d].lastClose[sec]
			b, ok2 := closes[d]
			if !ok1 || !ok2 || b == 0 {
				continue
			}
			dev := (a - b) / b * 100
			if dev < 0 {
				dev = -dev
			}
			devs = append(devs, dev)
			if dev > 1 {
				over1++
			}
			if dev > 5 {
				over5++
			}
		}
		fmt.Printf("%s,%d,%.3f,%d,%d\n", sec, len(devs), median(devs), over1, over5)
	}
}
