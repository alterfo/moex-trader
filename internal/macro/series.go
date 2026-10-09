package macro

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

type Point struct {
	Date  time.Time
	Value float64
}

type Series []Point

func Day(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func (s Series) Sorted() Series {
	out := append(Series(nil), s...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	deduped := out[:0]
	for _, p := range out {
		if n := len(deduped); n > 0 && deduped[n-1].Date.Equal(p.Date) {
			deduped[n-1] = p
			continue
		}
		deduped = append(deduped, p)
	}
	return deduped
}

func (s Series) before(day time.Time) Series {
	cut := Day(day)
	n := sort.Search(len(s), func(i int) bool { return !s[i].Date.Before(cut) })
	return s[:n]
}

func (s Series) Last() (Point, bool) {
	if len(s) == 0 {
		return Point{}, false
	}
	return s[len(s)-1], true
}

func (s Series) returnPct(window int) (float64, bool) {
	if len(s) <= window {
		return 0, false
	}
	prev := s[len(s)-1-window].Value
	if prev <= 0 {
		return 0, false
	}
	return (s[len(s)-1].Value/prev - 1) * 100, true
}

func (s Series) distMAPct(window int) (float64, bool) {
	if len(s) < window {
		return 0, false
	}
	sum := 0.0
	for _, p := range s[len(s)-window:] {
		sum += p.Value
	}
	ma := sum / float64(window)
	if ma <= 0 {
		return 0, false
	}
	return (s[len(s)-1].Value/ma - 1) * 100, true
}

func (s Series) valueOnOrBefore(day time.Time) (float64, bool) {
	cut := Day(day)
	n := sort.Search(len(s), func(i int) bool { return s[i].Date.After(cut) })
	if n == 0 {
		return 0, false
	}
	return s[n-1].Value, true
}

func SaveCSV(path string, s Series) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := csv.NewWriter(f)
	if err := w.Write([]string{"date", "value"}); err != nil {
		f.Close()
		return err
	}
	for _, p := range s.Sorted() {
		if err := w.Write([]string{p.Date.Format("2006-01-02"), strconv.FormatFloat(p.Value, 'f', -1, 64)}); err != nil {
			f.Close()
			return err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func LoadCSV(path string) (Series, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("macro: read %s: %w", path, err)
	}
	var out Series
	for i, row := range rows {
		if i == 0 || len(row) < 2 {
			continue
		}
		date, err := time.Parse("2006-01-02", row[0])
		if err != nil {
			return nil, fmt.Errorf("macro: %s line %d: %w", path, i+1, err)
		}
		value, err := strconv.ParseFloat(row[1], 64)
		if err != nil {
			return nil, fmt.Errorf("macro: %s line %d: %w", path, i+1, err)
		}
		out = append(out, Point{Date: date, Value: value})
	}
	return out.Sorted(), nil
}
