package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

type bar struct {
	date      string
	open      float64
	close     float64
	quoteVol  float64
	takerBuyQ float64
	trades    float64
}

type series struct {
	symbol  string
	bars    []bar
	byDate  map[string]int
	funding map[string]float64
}

type panel struct {
	series []*series
	dates  []string
	index  map[string]int
}

func loadPanel(dataDir string, maxSymbols int) (*panel, error) {
	symbolsPath := filepath.Join(dataDir, "symbols.csv")
	syms, err := readSymbolCSV(symbolsPath)
	if err != nil {
		return nil, err
	}
	if maxSymbols > 0 && len(syms) > maxSymbols {
		syms = syms[:maxSymbols]
	}
	p := &panel{index: map[string]int{}}
	dateSet := map[string]bool{}
	for _, sym := range syms {
		s, err := loadSeries(dataDir, sym)
		if err != nil {
			return nil, err
		}
		if s == nil || len(s.bars) == 0 {
			continue
		}
		p.series = append(p.series, s)
		for _, b := range s.bars {
			dateSet[b.date] = true
		}
	}
	for d := range dateSet {
		p.dates = append(p.dates, d)
	}
	sort.Strings(p.dates)
	for i, d := range p.dates {
		p.index[d] = i
	}
	return p, nil
}

func readSymbolCSV(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	if _, err := r.Read(); err != nil {
		return nil, err
	}
	var out []string
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(rec) == 0 || rec[0] == "" {
			continue
		}
		out = append(out, rec[0])
	}
	return out, nil
}

func loadSeries(dataDir, sym string) (*series, error) {
	path := filepath.Join(dataDir, "klines1d", sym+".csv")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	if _, err := r.Read(); err != nil {
		return nil, err
	}
	s := &series{symbol: sym, byDate: map[string]int{}, funding: map[string]float64{}}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(rec) < 11 {
			continue
		}
		b := bar{
			date:      rec[0],
			open:      parseF(rec[2]),
			close:     parseF(rec[5]),
			quoteVol:  parseF(rec[7]),
			trades:    parseF(rec[8]),
			takerBuyQ: parseF(rec[10]),
		}
		if b.close <= 0 {
			continue
		}
		s.byDate[b.date] = len(s.bars)
		s.bars = append(s.bars, b)
	}
	if err := loadFunding(dataDir, s); err != nil {
		return nil, err
	}
	return s, nil
}

func loadFunding(dataDir string, s *series) error {
	path := filepath.Join(dataDir, "funding", s.symbol+".csv")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	if _, err := r.Read(); err != nil {
		return err
	}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if len(rec) < 3 {
			continue
		}
		ms, err := strconv.ParseInt(rec[0], 10, 64)
		if err != nil {
			continue
		}
		rate := parseF(rec[2])
		date := msToDate(ms)
		s.funding[date] += rate * 1e4
	}
	return nil
}

func msToDate(ms int64) string {
	t := ms / 1000
	days := t / 86400
	return civilFromDays(int(days))
}

func civilFromDays(z int) string {
	z += 719468
	era := z / 146097
	if z < 0 {
		era = (z - 146096) / 146097
	}
	doe := z - era*146097
	yoe := (doe - doe/1460 + doe/36524 - doe/146096) / 365
	y := yoe + era*400
	doy := doe - (365*yoe + yoe/4 - yoe/100)
	mp := (5*doy + 2) / 153
	d := doy - (153*mp+2)/5 + 1
	m := mp + 3
	if mp >= 10 {
		m = mp - 9
		y++
	}
	return fmt.Sprintf("%04d-%02d-%02d", y, m, d)
}

func parseF(s string) float64 {
	if s == "" {
		return math.NaN()
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return math.NaN()
	}
	return v
}

func (p *panel) dateIndex() map[string]int {
	return p.index
}
