package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

func listDays(dir, till string) []string {
	files, _ := filepath.Glob(filepath.Join(dir, "tradestats", "*.csv.gz"))
	var days []string
	for _, f := range files {
		d := strings.TrimSuffix(filepath.Base(f), ".csv.gz")
		if d <= till {
			days = append(days, d)
		}
	}
	sort.Strings(days)
	return days
}

func loadAll(dir string, days []string, workers int) []dayResult {
	results := make([]dayResult, len(days))
	var wg sync.WaitGroup
	sem := make(chan struct{}, workers)
	var done int64
	for i, d := range days {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, d string) {
			defer wg.Done()
			defer func() { <-sem }()
			r, err := loadDay(dir, d, i)
			if err != nil {
				log.Fatalf("orderflowstudy: %v", err)
			}
			results[i] = r
			if n := atomic.AddInt64(&done, 1); n%200 == 0 {
				log.Printf("orderflowstudy: loaded %d/%d days", n, len(days))
			}
		}(i, d)
	}
	wg.Wait()
	var kept []dayResult
	for _, r := range results {
		if len(r.recs) > 0 {
			kept = append(kept, r)
		}
	}
	return kept
}

func main() {
	dir := flag.String("dir", "data/algopack", "AlgoPack directory")
	till := flag.String("till", "2026-10-21", "last day included")
	workers := flag.Int("workers", runtime.NumCPU(), "parallel day loaders")
	hedge := flag.Bool("hedge", false, "follow-up F1: hedged form of the A2 pass")
	forward := flag.String("forward-from", "", "follow-up F2: evaluate the frozen A2 rule from this feature date")
	diag := flag.Bool("diag", false, "post-hoc diagnostics of block A, outside the registered criteria")
	flag.Parse()

	days := listDays(*dir, *till)
	if len(days) == 0 {
		log.Fatal("orderflowstudy: no days")
	}
	log.Printf("orderflowstudy: %d day files", len(days))
	results := loadAll(*dir, days, *workers)
	p := newPanel(results)
	hhi := map[string]float64{}
	for _, r := range results {
		if !math.IsNaN(r.hhi) {
			hhi[r.date] = r.hhi
		}
	}
	log.Printf("orderflowstudy: panel %d days, %d tickers", len(p.dates), len(p.tickers))

	w := os.Stdout
	if *diag {
		runDiagnostics(w, p)
		return
	}
	if *forward != "" {
		p.reportForward(w, p.baseRowsA(100), *forward)
		return
	}
	if *hedge {
		cal, closes, err := fetchIMOEX("2019-12-01", *till)
		if err != nil {
			log.Fatalf("orderflowstudy: IMOEX: %v", err)
		}
		p.reportHedge(w, p.baseRowsA(100), closes, cal, loadKeyRate("data/macro/keyrate.csv"))
		return
	}
	var verdicts []verdict
	names := []string{"A1 trade imbalance", "A2 book imbalance (10 levels)", "A3 top-of-book imbalance", "A4 net order flow", "A5 cancellation intensity"}

	fmt.Fprintln(w, "== A. Daily order-flow features, next-day abnormal return (universe top 100)")
	rowsA := p.baseRowsA(100)
	for k, n := range names {
		verdicts = append(verdicts, p.reportFM(w, n, rowsA, k, 0))
	}
	fmt.Fprintln(w, "\n== A descriptive: entry at next open (outcome next open to next close), universe top 100")
	for k, n := range names {
		res := runFM(rowsA, k, true)
		s := p.summarize(res)
		fmt.Fprintf(w, "%s: coef %+.5f, NW t %+.2f (halves %+.2f / %+.2f)\n", n, s.mean, s.t, s.t0, s.t1)
	}
	for _, n := range []int{50, 200} {
		fmt.Fprintf(w, "\n== A descriptive: universe top %d\n", n)
		rows := p.baseRowsA(n)
		for k, nm := range names {
			res := runFM(rows, k, false)
			s := p.summarize(res)
			fmt.Fprintf(w, "%s: coef %+.5f, NW t %+.2f (halves %+.2f / %+.2f)\n", nm, s.mean, s.t, s.t0, s.t1)
		}
	}

	fmt.Fprintln(w, "\n== B. Intraday flow, checkpoint to 18:30 abnormal return (universe top 100)")
	rowsMid := p.baseRowsB(100, false)
	rowsLate := p.baseRowsB(100, true)
	verdicts = append(verdicts, p.reportFM(w, "B1 trade imbalance to 13:00", rowsMid, 0, 0.003))
	verdicts = append(verdicts, p.reportFM(w, "B2 book imbalance to 13:00", rowsMid, 1, 0.003))
	verdicts = append(verdicts, p.reportFM(w, "B3 trade imbalance to 17:00", rowsLate, 0, 0.003))
	verdicts = append(verdicts, p.reportFM(w, "B4 book imbalance to 17:00", rowsLate, 1, 0.003))

	fmt.Fprintln(w, "\n== C. Order flow at price levels (universe top 100, 5-minute bars, horizon 1 hour)")
	evs := p.collectEvents(100)
	var nr, np int
	for _, e := range evs {
		if e.real {
			nr++
		} else {
			np++
		}
	}
	fmt.Fprintf(w, "events: real %d, placebo %d\n", nr, np)
	verdicts = append(verdicts, p.reportC(w, "C1 book imbalance in the move direction", evs, func(e event) float64 { return e.c1 }))
	verdicts = append(verdicts, p.reportC(w, "C2 cancellation intensity on the level side", evs, func(e event) float64 { return e.c2 }))

	fmt.Fprintln(w, "\n== D. hi2 concentration and IMOEX")
	cal, closes, err := fetchIMOEX("2022-01-01", *till)
	if err != nil {
		log.Fatalf("orderflowstudy: IMOEX: %v", err)
	}
	verdicts = append(verdicts, reportD(w, runD(cal, closes, hhi)))

	fmt.Fprintln(w, "\n== Summary (family of 12, |t| >= 3.0)")
	pass := 0
	for _, v := range verdicts {
		fmt.Fprintf(w, "%-45s %s\n", v.name, map[bool]string{true: "PASS", false: "fail"}[v.pass])
		if v.pass {
			pass++
		}
	}
	fmt.Fprintf(w, "passed %d of %d\n", pass, len(verdicts))
}
