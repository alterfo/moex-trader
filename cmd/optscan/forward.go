package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

type snapshot struct {
	Date    string  `json:"date"`
	Family  string  `json:"family"`
	Expiry  string  `json:"expiry"`
	Future  string  `json:"future"`
	DTE     int     `json:"dte"`
	Forward float64 `json:"forward"`
	Strike  float64 `json:"strike"`
	CallWap float64 `json:"call_wap"`
	PutWap  float64 `json:"put_wap"`
	CallVol float64 `json:"call_vol"`
	PutVol  float64 `json:"put_vol"`
}

func runForward(p params, date string, dteMin, dteMax int) error {
	path := filepath.Join(p.dir, "forward-snapshots.jsonl")
	seen := map[string]bool{}
	if b, err := os.ReadFile(path); err == nil {
		for _, line := range splitLines(string(b)) {
			var s snapshot
			if err := json.Unmarshal([]byte(line), &s); err != nil {
				continue
			}
			seen[s.Date+"|"+s.Expiry] = true
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	specs := loadSpecs(p.dir)
	written := 0
	for _, family := range p.families {
		futures := loadFutures(p.dir, family, specs, p.usdrub)
		byExpiry := loadOptions(p.dir, family)
		for expiry, opts := range byExpiry {
			fut := pickFuture(futures, expiry)
			if fut == nil {
				continue
			}
			entryIdx, ok := fut.at[date]
			if !ok {
				continue
			}
			if expiry <= date {
				continue
			}
			dte := 0
			for i := entryIdx; i < len(fut.dates) && fut.dates[i] <= expiry; i++ {
				dte++
			}
			dte--
			if dte < dteMin || dte > dteMax {
				continue
			}
			fwd := fut.settle[date]
			call, put := atmPair(opts, date, fwd)
			if call == nil || put == nil {
				continue
			}
			if seen[date+"|"+expiry] {
				continue
			}
			s := snapshot{Date: date, Family: family, Expiry: expiry, Future: fut.id, DTE: dte,
				Forward: fwd, Strike: call.strike,
				CallWap: call.wap[date], PutWap: put.wap[date],
				CallVol: call.vol[date], PutVol: put.vol[date]}
			b, err := json.Marshal(s)
			if err != nil {
				return err
			}
			if _, err := f.WriteString(string(b) + "\n"); err != nil {
				return err
			}
			seen[date+"|"+expiry] = true
			written++
		}
	}
	fmt.Fprintf(os.Stderr, "forward: %d snapshots for %s\n", written, date)
	return nil
}

func runForwardSettle(p params, targetDTE int, costs params) error {
	path := filepath.Join(p.dir, "forward-snapshots.jsonl")
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	specs := loadSpecs(p.dir)
	futures := map[string][]*future{}
	var rows [][]string
	rows = append(rows, []string{"date", "expiry", "future", "dte", "strike", "forward", "premium", "payoff", "pnl_rub"})
	var pnls []float64
	for _, line := range splitLines(string(b)) {
		var s snapshot
		if err := json.Unmarshal([]byte(line), &s); err != nil {
			continue
		}
		if s.DTE != targetDTE {
			continue
		}
		if s.CallWap <= 0 || s.PutWap <= 0 || s.CallVol < p.minVol || s.PutVol < p.minVol {
			continue
		}
		if _, ok := futures[s.Family]; !ok {
			futures[s.Family] = loadFutures(p.dir, s.Family, specs, p.usdrub)
		}
		f := pickFuture(futures[s.Family], s.Expiry)
		if f == nil {
			continue
		}
		fe, ok := f.settle[s.Expiry]
		if !ok {
			continue
		}
		premium := s.CallWap + s.PutWap
		payoff := math.Abs(fe - s.Strike)
		cost := costsRUB(costs.feeRUB, costs.spreadFrac, premium, f.mult, 2)
		pnl := (premium-payoff)*f.mult - cost
		pnls = append(pnls, pnl)
		rows = append(rows, []string{s.Date, s.Expiry, s.Future, strconv.Itoa(s.DTE), fmt.Sprintf("%.2f", s.Strike),
			fmt.Sprintf("%.2f", s.Forward), fmt.Sprintf("%.2f", premium), fmt.Sprintf("%.2f", payoff), fmt.Sprintf("%.0f", pnl)})
	}
	if err := writeCSV(filepath.Join("artifacts", "options", "forward-results.csv"), rows); err != nil {
		return err
	}
	if len(pnls) == 0 {
		fmt.Printf("forward: no settled trades at DTE %d yet\n", targetDTE)
		return nil
	}
	sort.Float64s(pnls)
	m, t := neweyWest(pnls, 5)
	fmt.Printf("forward DTE %d: n %d mean %.1f t %.2f worst %.1f\n", targetDTE, len(pnls), m, t, pnls[0])
	return nil
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			if i > start {
				out = append(out, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
