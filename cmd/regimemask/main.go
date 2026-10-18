package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

type decision struct {
	Ticker      string  `json:"ticker"`
	Date        string  `json:"date"`
	Probability float64 `json:"probability"`
}

type window struct {
	Decisions []decision `json:"decisions"`
}

func riskOffDates(closes map[string]float64, dates []string, mask string, r1, r5 float64) map[string]bool {
	off := make(map[string]bool)
	for i, d := range dates {
		switch mask {
		case "r1":
			if i >= 1 && closes[d]/closes[dates[i-1]]-1 <= r1 {
				off[d] = true
			}
		case "r2":
			if i >= 5 && closes[d]/closes[dates[i-5]]-1 <= r5 {
				off[d] = true
			}
		}
	}
	return off
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("regimemask", flag.ContinueOnError)
	configPath := fs.String("config", "config.sandbox.yaml", "config path")
	wfDir := fs.String("wf-dir", "", "walk-forward directory with <window>/window.json")
	mask := fs.String("mask", "none", "none | r1 | r2")
	r1 := fs.Float64("r1", -0.02, "R1: IMOEX one-day return at or below this marks risk-off")
	r5 := fs.Float64("r2", -0.05, "R2: IMOEX five-day return at or below this marks risk-off")
	buy := fs.Float64("buy", 0.60, "probability at or above which a decision is a BUY")
	outPath := fs.String("out", "p_ens.csv", "output CSV (ticker,date,p_ens)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *wfDir == "" {
		return fmt.Errorf("-wf-dir is required")
	}
	files, err := filepath.Glob(filepath.Join(*wfDir, "*", "window.json"))
	if err != nil || len(files) == 0 {
		return fmt.Errorf("no windows under %s", *wfDir)
	}
	var all []decision
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		var w window
		if err := json.Unmarshal(data, &w); err != nil {
			return fmt.Errorf("%s: %w", f, err)
		}
		for _, d := range w.Decisions {
			d.Date = strings.SplitN(d.Date, "T", 2)[0]
			all = append(all, d)
		}
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	src := backtest.NewISSSource(cfg.MOEXISSBaseURL, moex.NewClient(cfg.MOEXISSBaseURL, nil))
	candles, err := src.History(context.Background(), "IMOEX", time.Date(2024, 11, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC))
	if err != nil {
		return err
	}
	closes := make(map[string]float64)
	var dates []string
	for _, c := range candles {
		d := c.Begin.Format("2006-01-02")
		v, _ := c.Close.Float64()
		closes[d] = v
		dates = append(dates, d)
	}
	sort.Strings(dates)
	off := riskOffDates(closes, dates, *mask, *r1, *r5)

	out, err := os.Create(*outPath)
	if err != nil {
		return err
	}
	defer out.Close()
	w := csv.NewWriter(out)
	w.Write([]string{"ticker", "date", "p_ens"})
	suppressed := 0
	offInWindows := make(map[string]bool)
	for _, d := range all {
		if off[d.Date] {
			offInWindows[d.Date] = true
			if d.Probability >= *buy {
				suppressed++
				continue
			}
		}
		w.Write([]string{d.Ticker, d.Date, fmt.Sprintf("%.10f", d.Probability)})
	}
	w.Flush()
	log.Printf("mask=%s decisions=%d risk-off dates in windows=%d suppressed BUY=%d", *mask, len(all), len(offInWindows), suppressed)
	return w.Error()
}
