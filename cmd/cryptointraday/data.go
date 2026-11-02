package main

import (
	"encoding/csv"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

type bar struct {
	t        int64
	open     float64
	high     float64
	low      float64
	close    float64
	quoteVol float64
}

type sessionRec struct {
	symbol  string
	date    string
	open    float64
	high    float64
	low     float64
	close   float64
	qvol    float64
	bars    int
	entries int
	trades  int
}

type sessionWindow struct {
	name     string
	startMin int
	lenMin   int
	endMin   int
}

func windowByName(name string) (sessionWindow, bool) {
	switch name {
	case "s0":
		return sessionWindow{name: "s0", startMin: 0, lenMin: 30, endMin: 23*60 + 55}, true
	case "s1":
		return sessionWindow{name: "s1", startMin: 13*60 + 30, lenMin: 30, endMin: 20 * 60}, true
	}
	return sessionWindow{}, false
}

func minuteOfDay(ms int64) int {
	t := time.UnixMilli(ms).UTC()
	return t.Hour()*60 + t.Minute()
}

func dateOf(ms int64) string {
	return time.UnixMilli(ms).UTC().Format("2006-01-02")
}

func streamBars(path string, fn func(bar) bool) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	r.ReuseRecord = true
	if _, err := r.Read(); err != nil {
		return err
	}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if len(rec) < 11 {
			continue
		}
		ms, err := strconv.ParseInt(rec[1], 10, 64)
		if err != nil {
			continue
		}
		b := bar{
			t:        ms,
			open:     parseNum(rec[2]),
			high:     parseNum(rec[3]),
			low:      parseNum(rec[4]),
			close:    parseNum(rec[5]),
			quoteVol: parseNum(rec[7]),
		}
		if b.close <= 0 || b.open <= 0 {
			continue
		}
		if !fn(b) {
			return nil
		}
	}
}

func parseNum(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

func listContracts(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if filepath.Ext(name) != ".csv" {
			continue
		}
		out = append(out, name[:len(name)-4])
	}
	sort.Strings(out)
	return out, nil
}

type dailyVol map[string]map[string]float64

func loadDailyVolume(dir string, symbols []string) (dailyVol, error) {
	out := dailyVol{}
	for _, sym := range symbols {
		path := filepath.Join(dir, sym+".csv")
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		r := csv.NewReader(f)
		r.FieldsPerRecord = -1
		if _, err := r.Read(); err != nil {
			f.Close()
			continue
		}
		byDate := map[string]float64{}
		for {
			rec, err := r.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				f.Close()
				return nil, err
			}
			if len(rec) < 11 {
				continue
			}
			byDate[rec[0]] = parseNum(rec[7])
		}
		f.Close()
		out[sym] = byDate
	}
	return out, nil
}

func loadDailyFunding(dir string, symbols []string) map[string]map[string]float64 {
	out := map[string]map[string]float64{}
	for _, sym := range symbols {
		path := filepath.Join(dir, sym+".csv")
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		r := csv.NewReader(f)
		r.FieldsPerRecord = -1
		if _, err := r.Read(); err != nil {
			f.Close()
			continue
		}
		byDate := map[string]float64{}
		for {
			rec, err := r.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				f.Close()
				break
			}
			if len(rec) < 3 {
				continue
			}
			ms, err := strconv.ParseInt(rec[0], 10, 64)
			if err != nil {
				continue
			}
			byDate[dateOf(ms)] += parseNum(rec[2]) * 1e4
		}
		f.Close()
		out[sym] = byDate
	}
	return out
}
