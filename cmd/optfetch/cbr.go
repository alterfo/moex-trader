package main

import (
	"fmt"
	"golang.org/x/text/encoding/charmap"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

func cbrMode(dir string, from, to time.Time) error {
	client := newResponder("https://www.cbr.ru").client
	var lines []string
	lines = append(lines, "date,rate")
	for y := from.Year(); y <= to.Year(); y++ {
		a := time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC)
		b := time.Date(y, 12, 31, 0, 0, 0, 0, time.UTC)
		if a.Before(from) {
			a = from
		}
		if b.After(to) {
			b = to
		}
		u := fmt.Sprintf("https://www.cbr.ru/scripts/XML_dynamic.asp?date_req1=%s&date_req2=%s&VAL_NM_RQ=R01235", a.Format("02/01/2006"), b.Format("02/01/2006"))
		req, err := http.NewRequest(http.MethodGet, u, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0")
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return fmt.Errorf("cbr: status %d", resp.StatusCode)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}
		dec, err := charmap.Windows1251.NewDecoder().Bytes(body)
		if err != nil {
			return err
		}
		parts := strings.Split(string(dec), "<Record Date=\"")
		for _, p := range parts[1:] {
			end := strings.Index(p, "\"")
			if end <= 0 {
				continue
			}
			d, err := time.Parse("02.01.2006", p[:end])
			if err != nil {
				continue
			}
			vi := strings.Index(p, "<VunitRate>")
			if vi < 0 {
				continue
			}
			rest := p[vi+len("<VunitRate>"):]
			ve := strings.Index(rest, "<")
			if ve <= 0 {
				continue
			}
			v := strings.ReplaceAll(rest[:ve], ",", ".")
			if _, err := strconv.ParseFloat(v, 64); err != nil {
				continue
			}
			lines = append(lines, d.Format("2006-01-02")+","+v)
		}
		time.Sleep(200 * time.Millisecond)
	}
	path := filepath.Join(dir, "usdrub.csv")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	merged := map[string]string{}
	if b, err := os.ReadFile(path); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n")[1:] {
			parts := strings.SplitN(line, ",", 2)
			if len(parts) == 2 && parts[0] != "" {
				merged[parts[0]] = parts[1]
			}
		}
	}
	for _, line := range lines[1:] {
		parts := strings.SplitN(line, ",", 2)
		if len(parts) == 2 && parts[0] != "" {
			merged[parts[0]] = parts[1]
		}
	}
	dates := make([]string, 0, len(merged))
	for d := range merged {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	out := []string{"date,rate"}
	for _, d := range dates {
		out = append(out, d+","+merged[d])
	}
	if err := os.WriteFile(path, []byte(strings.Join(out, "\n")+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "cbr: %d rows -> %s\n", len(dates), path)
	return nil
}
