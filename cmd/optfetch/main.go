package main

import (
	"compress/gzip"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const defaultISS = "https://iss.moex.com/iss"

type table struct {
	Columns []string            `json:"columns"`
	Data    [][]json.RawMessage `json:"data"`
}

type responder struct {
	base   string
	client *http.Client
	delay  time.Duration
}

func newResponder(base string) *responder {
	return &responder{base: strings.TrimRight(base, "/"), client: &http.Client{Timeout: 90 * time.Second}, delay: 120 * time.Millisecond}
}

func (r *responder) get(path string, params url.Values) (map[string]table, error) {
	u := r.base + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	var last error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
		resp, err := r.client.Get(u)
		if err != nil {
			last = err
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			last = err
			continue
		}
		if resp.StatusCode == http.StatusNotFound {
			return map[string]table{}, nil
		}
		if resp.StatusCode != http.StatusOK {
			last = fmt.Errorf("GET %s: status %d", u, resp.StatusCode)
			continue
		}
		var out map[string]table
		if err := json.Unmarshal(body, &out); err != nil {
			last = fmt.Errorf("GET %s: %w", u, err)
			continue
		}
		time.Sleep(r.delay)
		return out, nil
	}
	return nil, last
}

func (t table) index(col string) int {
	for i, c := range t.Columns {
		if c == col {
			return i
		}
	}
	return -1
}

func cell(row []json.RawMessage, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	s := string(row[i])
	if s == "null" {
		return ""
	}
	var str string
	if err := json.Unmarshal(row[i], &str); err == nil {
		return str
	}
	return strings.Trim(s, `"`)
}

type pageFn func(start int) (table, error)

func gather(fn pageFn) (table, error) {
	var out table
	start := 0
	for page := 0; page < 500; page++ {
		t, err := fn(start)
		if err != nil {
			return out, err
		}
		if page == 0 {
			out.Columns = t.Columns
		}
		out.Data = append(out.Data, t.Data...)
		if len(t.Data) < 100 {
			return out, nil
		}
		start += 100
	}
	return out, errors.New("pagination did not terminate")
}

func parseDay(s string) (time.Time, error) {
	return time.Parse("2006-01-02", s)
}

func dayRange(from, to time.Time) []time.Time {
	var days []time.Time
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		days = append(days, d)
	}
	return days
}

func writeGzCSV(path string, header []string, rows [][]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := gzip.NewWriter(f)
	w := csv.NewWriter(zw)
	if err := w.Write(header); err != nil {
		return err
	}
	for _, r := range rows {
		if err := w.Write(r); err != nil {
			return err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return err
	}
	return zw.Close()
}

func readGzCSV(path string) ([][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	return csv.NewReader(zr).ReadAll()
}

func chainMode(r *responder, dir string) error {
	t, err := r.get("/engines/futures/markets/options/securities.json", url.Values{"iss.meta": {"off"}, "iss.only": {"securities"}})
	if err != nil {
		return err
	}
	sec, ok := t["securities"]
	if !ok {
		return errors.New("no securities table")
	}
	idx := map[string]int{}
	for _, c := range []string{"SECID", "SHORTNAME", "ASSETCODE", "LASTTRADEDATE", "OPTIONTYPE", "STRIKE", "UNDERLYINGASSET"} {
		idx[c] = sec.index(c)
	}
	var lines []string
	for _, row := range sec.Data {
		rec := map[string]string{}
		for c, i := range idx {
			rec[strings.ToLower(c)] = cell(row, i)
		}
		b, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		lines = append(lines, string(b))
	}
	path := filepath.Join(dir, "chain", time.Now().Format("2006-01-02")+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "chain: %d series -> %s\n", len(lines), path)
	return nil
}

var historyColumns = []string{"TRADEDATE", "SECID", "OPEN", "LOW", "HIGH", "CLOSE", "WAPRICE", "SETTLEPRICE", "THEOR_PRICE", "VOLUME", "VALUE", "NUMTRADES", "OPENPOSITION"}

func dayMode(r *responder, dir string, from, to time.Time, workers int, force bool) error {
	days := dayRange(from, to)
	work := make(chan time.Time)
	var wg sync.WaitGroup
	var mu sync.Mutex
	fetched, skipped, failed := 0, 0, 0
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for d := range work {
				q := d.Format("2006-01-02")
				path := filepath.Join(dir, "history", q+".csv.gz")
				if !force {
					if _, err := os.Stat(path); err == nil {
						mu.Lock()
						skipped++
						mu.Unlock()
						continue
					}
				}
				t, err := gather(func(start int) (table, error) {
					params := url.Values{
						"iss.meta":        {"off"},
						"date":            {q},
						"history.columns": {strings.Join(historyColumns, ",")},
						"start":           {fmt.Sprint(start)},
					}
					resp, err := r.get("/history/engines/futures/markets/options/securities.json", params)
					if err != nil {
						return table{}, err
					}
					return resp["history"], nil
				})
				if err != nil {
					mu.Lock()
					failed++
					mu.Unlock()
					log.Printf("day %s: %v", q, err)
					continue
				}
				rows := make([][]string, 0, len(t.Data))
				for _, row := range t.Data {
					out := make([]string, len(historyColumns))
					for i, c := range historyColumns {
						out[i] = cell(row, t.index(c))
					}
					rows = append(rows, out)
				}
				if err := writeGzCSV(path, historyColumns, rows); err != nil {
					mu.Lock()
					failed++
					mu.Unlock()
					log.Printf("day %s: %v", q, err)
					continue
				}
				mu.Lock()
				fetched++
				n := fetched
				mu.Unlock()
				if n%20 == 0 {
					fmt.Fprintf(os.Stderr, "days: %d fetched, %d skipped, %d failed (last %s)\n", n, skipped, failed, q)
				}
			}
		}()
	}
	for _, d := range days {
		work <- d
	}
	close(work)
	wg.Wait()
	fmt.Fprintf(os.Stderr, "days: %d fetched, %d skipped, %d failed, %d dates total\n", fetched, skipped, failed, len(days))
	return nil
}

func descMode(r *responder, dir string, from, to time.Time, workers int) error {
	secids := map[string]struct{}{}
	entries, err := os.ReadDir(filepath.Join(dir, "history"))
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".csv.gz") {
			continue
		}
		q := strings.TrimSuffix(e.Name(), ".csv.gz")
		d, err := parseDay(q)
		if err != nil || d.Before(from) || d.After(to) {
			continue
		}
		rows, err := readGzCSV(filepath.Join(dir, "history", e.Name()))
		if err != nil {
			return err
		}
		if len(rows) < 2 {
			continue
		}
		col := -1
		for i, c := range rows[0] {
			if c == "SECID" {
				col = i
			}
		}
		if col < 0 {
			continue
		}
		for _, row := range rows[1:] {
			secids[row[col]] = struct{}{}
		}
	}
	var todo []string
	for s := range secids {
		if _, err := os.Stat(filepath.Join(dir, "desc", s+".json")); err != nil {
			todo = append(todo, s)
		}
	}
	sort.Strings(todo)
	fmt.Fprintf(os.Stderr, "desc: %d series in history, %d missing\n", len(secids), len(todo))
	work := make(chan string)
	var wg sync.WaitGroup
	var mu sync.Mutex
	done, failed := 0, 0
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for s := range work {
				t, err := r.get("/securities/"+url.PathEscape(s)+".json", url.Values{"iss.meta": {"off"}, "iss.only": {"description"}})
				if err != nil {
					mu.Lock()
					failed++
					mu.Unlock()
					log.Printf("desc %s: %v", s, err)
					continue
				}
				d, ok := t["description"]
				if !ok {
					continue
				}
				rec := map[string]string{}
				for _, row := range d.Data {
					rec[cell(row, 0)] = cell(row, 1)
				}
				b, err := json.Marshal(rec)
				if err != nil {
					log.Printf("desc %s: %v", s, err)
					continue
				}
				if err := os.MkdirAll(filepath.Join(dir, "desc"), 0o755); err != nil {
					log.Printf("desc %s: %v", s, err)
					continue
				}
				if err := os.WriteFile(filepath.Join(dir, "desc", s+".json"), b, 0o644); err != nil {
					log.Printf("desc %s: %v", s, err)
					continue
				}
				mu.Lock()
				done++
				n := done
				mu.Unlock()
				if n%200 == 0 {
					fmt.Fprintf(os.Stderr, "desc: %d done, %d failed (last %s)\n", n, failed, s)
				}
			}
		}()
	}
	for _, s := range todo {
		work <- s
	}
	close(work)
	wg.Wait()
	fmt.Fprintf(os.Stderr, "desc: %d fetched, %d failed\n", done, failed)
	return nil
}

func futMode(r *responder, dir string, from, to time.Time, workers int) error {
	underlyings := map[string]struct{}{}
	entries, err := os.ReadDir(filepath.Join(dir, "desc"))
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, "desc", e.Name()))
		if err != nil {
			return err
		}
		var rec map[string]string
		if err := json.Unmarshal(b, &rec); err != nil {
			continue
		}
		if u := rec["UNDERLYINGASSET"]; u != "" {
			underlyings[u] = struct{}{}
		}
	}
	fmt.Fprintf(os.Stderr, "fut: %d underlying futures referenced\n", len(underlyings))
	var list []string
	for u := range underlyings {
		list = append(list, u)
	}
	sort.Strings(list)
	work := make(chan string)
	var wg sync.WaitGroup
	var mu sync.Mutex
	done, failed := 0, 0
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for s := range work {
				path := filepath.Join(dir, "underlying", s+".csv.gz")
				if _, err := os.Stat(path); err == nil {
					mu.Lock()
					done++
					mu.Unlock()
					continue
				}
				t, err := gather(func(start int) (table, error) {
					params := url.Values{
						"iss.meta":        {"off"},
						"from":            {from.Format("2006-01-02")},
						"till":            {to.Format("2006-01-02")},
						"history.columns": {"TRADEDATE,CLOSE,SETTLEPRICE,VOLUME,OPENPOSITION"},
						"start":           {fmt.Sprint(start)},
					}
					resp, err := r.get("/history/engines/futures/markets/forts/securities/"+url.PathEscape(s)+".json", params)
					if err != nil {
						return table{}, err
					}
					return resp["history"], nil
				})
				if err != nil {
					mu.Lock()
					failed++
					mu.Unlock()
					log.Printf("fut %s: %v", s, err)
					continue
				}
				rows := make([][]string, 0, len(t.Data))
				for _, row := range t.Data {
					out := make([]string, 4)
					out[0] = cell(row, t.index("TRADEDATE"))
					out[1] = cell(row, t.index("CLOSE"))
					out[2] = cell(row, t.index("SETTLEPRICE"))
					out[3] = cell(row, t.index("VOLUME"))
					rows = append(rows, out)
				}
				if err := writeGzCSV(path, []string{"TRADEDATE", "CLOSE", "SETTLEPRICE", "VOLUME"}, rows); err != nil {
					mu.Lock()
					failed++
					mu.Unlock()
					log.Printf("fut %s: %v", s, err)
					continue
				}
				mu.Lock()
				done++
				mu.Unlock()
			}
		}()
	}
	for _, s := range list {
		work <- s
	}
	close(work)
	wg.Wait()
	fmt.Fprintf(os.Stderr, "fut: %d done, %d failed\n", done, failed)
	return nil
}

func main() {
	mode := flag.String("mode", "chain", "chain|day|desc|fut|series|meta|cbr")
	dir := flag.String("dir", "data/options", "output directory")
	fromStr := flag.String("from", "2023-10-01", "first date")
	tillStr := flag.String("till", time.Now().Format("2006-01-02"), "last date")
	iss := flag.String("iss", defaultISS, "ISS base URL")
	workers := flag.Int("workers", 3, "parallel requests")
	force := flag.Bool("force", false, "refetch existing day files")
	prefix := flag.String("prefix", "", "option secid prefix for series mode, e.g. BR or GD")
	window := flag.Float64("window", 0.15, "strike window around the future price for series mode")
	grid := flag.Int("grid", 25, "maximum strikes per future for series mode")
	flag.Parse()
	from, err := parseDay(*fromStr)
	if err != nil {
		log.Fatal(err)
	}
	till, err := parseDay(*tillStr)
	if err != nil {
		log.Fatal(err)
	}
	r := newResponder(*iss)
	switch *mode {
	case "chain":
		err = chainMode(r, *dir)
	case "day":
		err = dayMode(r, *dir, from, till, *workers, *force)
	case "desc":
		err = descMode(r, *dir, from, till, *workers)
	case "fut":
		err = futMode(r, *dir, from, till, *workers)
	case "cbr":
		err = cbrMode(*dir, from, till)
	case "meta":
		err = metaMode(r, *dir, *workers)
	case "series":
		if *prefix == "" {
			log.Fatal("-prefix is required for series mode")
		}
		for _, p := range strings.Split(*prefix, ",") {
			if err := seriesMode(r, *dir, strings.TrimSpace(p), from, till, *workers, *window, *grid); err != nil {
				log.Fatal(err)
			}
		}
	default:
		log.Fatalf("unknown mode %q", *mode)
	}
	if err != nil {
		log.Fatal(err)
	}
}
