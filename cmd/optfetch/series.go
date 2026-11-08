package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var monthLetters = []string{"F", "G", "H", "J", "K", "M", "N", "Q", "U", "V", "X", "Z"}

var optionMonthLetters = []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L"}

var optionPutMonthLetters = []string{"M", "N", "O", "P", "Q", "R", "S", "T", "U", "V", "W", "X"}

type chainEntry struct {
	SecID           string `json:"secid"`
	AssetCode       string `json:"assetcode"`
	LastTradeDate   string `json:"lasttradedate"`
	OptionType      string `json:"optiontype"`
	Strike          string `json:"strike"`
	UnderlyingAsset string `json:"underlyingasset"`
}

func latestChain(dir string) ([]chainEntry, error) {
	entries, err := filepath.Glob(filepath.Join(dir, "chain", "*.jsonl"))
	if err != nil || len(entries) == 0 {
		return nil, fmt.Errorf("no chain snapshot in %s", filepath.Join(dir, "chain"))
	}
	sort.Strings(entries)
	b, err := os.ReadFile(entries[len(entries)-1])
	if err != nil {
		return nil, err
	}
	var out []chainEntry
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if line == "" {
			continue
		}
		var e chainEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

func strikeValue(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

func strikeString(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func prefixStrikes(chain []chainEntry, prefix string) []float64 {
	seen := map[float64]bool{}
	var out []float64
	for _, e := range chain {
		if !strings.HasPrefix(e.SecID, prefix) {
			continue
		}
		v := strikeValue(e.Strike)
		if v == 0 || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Float64s(out)
	return out
}

func readPath(path string) (map[string]float64, error) {
	rows, err := readGzCSV(path)
	if err != nil {
		return nil, err
	}
	out := map[string]float64{}
	for _, row := range rows[1:] {
		if len(row) < 3 {
			continue
		}
		v, err := strconv.ParseFloat(row[2], 64)
		if err != nil || v == 0 {
			continue
		}
		out[row[0]] = v
	}
	return out, nil
}

func sortedDates(m map[string]float64) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (r *responder) series(sec, market string, from, to time.Time, columns []string) ([][]string, error) {
	t, err := gather(func(start int) (table, error) {
		params := url.Values{
			"iss.meta":        {"off"},
			"from":            {from.Format("2006-01-02")},
			"till":            {to.Format("2006-01-02")},
			"history.columns": {strings.Join(columns, ",")},
			"start":           {fmt.Sprint(start)},
		}
		resp, err := r.get("/history/engines/futures/markets/"+market+"/securities/"+url.PathEscape(sec)+".json", params)
		if err != nil {
			return table{}, err
		}
		return resp["history"], nil
	})
	if err != nil {
		return nil, err
	}
	rows := make([][]string, 0, len(t.Data))
	for _, row := range t.Data {
		out := make([]string, len(columns))
		for i, c := range columns {
			out[i] = cell(row, t.index(c))
		}
		rows = append(rows, out)
	}
	return rows, nil
}

func futureIDs(prefix string, from, to time.Time) []string {
	var out []string
	for y := from.Year(); y <= to.Year(); y++ {
		for m := 1; m <= 12; m++ {
			out = append(out, fmt.Sprintf("%s%s%d", prefix, monthLetters[m-1], y%10))
		}
	}
	return out
}

func seriesMode(r *responder, dir, prefix string, from, to time.Time, workers int, window float64, grid int) error {
	chain, err := latestChain(dir)
	if err != nil {
		return err
	}
	strikes := prefixStrikes(chain, prefix)
	if len(strikes) == 0 {
		return fmt.Errorf("no chain entries for prefix %q", prefix)
	}
	fmt.Fprintf(os.Stderr, "series %s: %d strikes in chain %v..%v\n", prefix, len(strikes), strikes[0], strikes[len(strikes)-1])

	futDir := filepath.Join(dir, "futures")
	if err := os.MkdirAll(futDir, 0o755); err != nil {
		return err
	}
	futFrom := from.AddDate(0, -4, 0)
	futTill := to.AddDate(0, 6, 0)
	type fut struct {
		id    string
		lst   string
		path  string
		dates []string
	}
	var futures []fut
	for _, id := range futureIDs(prefix, futFrom, futTill) {
		path := filepath.Join(futDir, id+".csv.gz")
		if _, err := os.Stat(path); err != nil {
			rows, err := r.series(id, "forts", futFrom, futTill, []string{"TRADEDATE", "CLOSE", "SETTLEPRICE", "VOLUME"})
			if err != nil {
				log.Printf("future %s: %v", id, err)
				continue
			}
			if len(rows) == 0 {
				continue
			}
			if err := writeGzCSV(path, []string{"TRADEDATE", "CLOSE", "SETTLEPRICE", "VOLUME"}, rows); err != nil {
				log.Printf("future %s: %v", id, err)
				continue
			}
		}
		closes, err := readPath(path)
		if err != nil || len(closes) == 0 {
			continue
		}
		ds := sortedDates(closes)
		futures = append(futures, fut{id: id, lst: ds[len(ds)-1], path: path, dates: ds})
	}
	fmt.Fprintf(os.Stderr, "series %s: %d futures with history\n", prefix, len(futures))
	if len(futures) == 0 {
		return fmt.Errorf("no futures for prefix %q", prefix)
	}

	type job struct {
		id   string
		from time.Time
		to   time.Time
	}
	jobs := make(chan job)
	var wg sync.WaitGroup
	var mu sync.Mutex
	done, failed, skipped := 0, 0, 0
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				path := filepath.Join(dir, "series", j.id+".csv.gz")
				if _, err := os.Stat(path); err == nil {
					mu.Lock()
					skipped++
					mu.Unlock()
					continue
				}
				rows, err := r.series(j.id, "options", j.from, j.to, []string{"TRADEDATE", "WAPRICE", "SETTLEPRICE", "VOLUME"})
				if err != nil {
					mu.Lock()
					failed++
					mu.Unlock()
					log.Printf("series %s: %v", j.id, err)
					continue
				}
				if len(rows) == 0 {
					mu.Lock()
					skipped++
					mu.Unlock()
					continue
				}
				if err := writeGzCSV(path, []string{"TRADEDATE", "WAPRICE", "SETTLEPRICE", "VOLUME"}, rows); err != nil {
					mu.Lock()
					failed++
					mu.Unlock()
					log.Printf("series %s: %v", j.id, err)
					continue
				}
				mu.Lock()
				done++
				n := done
				mu.Unlock()
				if n%200 == 0 {
					fmt.Fprintf(os.Stderr, "series %s: %d fetched, %d empty, %d failed\n", prefix, n, skipped, failed)
				}
			}
		}()
	}

	started := time.Now()
	queued := 0
	for _, f := range futures {
		lst, err := parseDay(f.lst)
		if err != nil || lst.After(to.AddDate(0, 1, 0)) {
			continue
		}
		closes, err := readPath(f.path)
		if err != nil {
			continue
		}
		center := 0.0
		idx := len(f.dates) - 1
		target := lst.AddDate(0, 0, -25).Format("2006-01-02")
		for i := len(f.dates) - 1; i >= 0; i-- {
			if f.dates[i] <= target {
				idx = i
				break
			}
		}
		center = closes[f.dates[idx]]
		if center <= 0 {
			continue
		}
		step := strikeStep(strikes)
		if step <= 0 {
			step = center * 0.01
		}
		n := int(window * center / step)
		if n < 1 {
			n = 1
		}
		var candidates []float64
		for k := -n; k <= n; k++ {
			candidates = append(candidates, roundTo(center+float64(k)*step, step))
		}
		if grid > 0 && len(candidates) > grid {
			stride := (len(candidates) + grid - 1) / grid
			var thin []float64
			for i := 0; i < len(candidates); i += stride {
				thin = append(thin, candidates[i])
			}
			if len(thin) > 0 && thin[len(thin)-1] != candidates[len(candidates)-1] {
				thin = append(thin, candidates[len(candidates)-1])
			}
			candidates = thin
		}
		if len(candidates) == 0 {
			continue
		}
		months := []time.Month{lst.Month(), lst.AddDate(0, 0, -1).Month()}
		expiryFrom := lst.AddDate(0, -3, 0)
		for _, m := range months {
			letter := optionMonthLetters[int(m)-1]
			year := lst.Year()
			if int(m) > int(lst.Month()) {
				year--
			}
			variants := probeVariants(r, prefix, candidates, letter, year, expiryFrom, lst)
			if len(variants) == 0 {
				variants = []string{""}
			}
			for _, s := range candidates {
				for _, typ := range []string{"B", "W"} {
					letterFor := letter
					if typ == "W" {
						letterFor = optionPutMonthLetters[int(m)-1]
					}
					for _, suf := range variants {
						id := fmt.Sprintf("%s%sB%s%d%s", prefix, strikeString(s), letterFor, year%10, suf)
						jobs <- job{id: id, from: expiryFrom, to: lst.AddDate(0, 1, 0)}
						queued++
					}
				}
			}
		}
	}
	close(jobs)
	wg.Wait()
	fmt.Fprintf(os.Stderr, "series %s: %d fetched, %d empty, %d failed, %d queued in %s\n", prefix, done, skipped, failed, queued, time.Since(started).Round(time.Second))
	if done == 0 {
		return fmt.Errorf("no series written for prefix %q", prefix)
	}
	return nil
}

func probeVariants(r *responder, prefix string, candidates []float64, letter string, year int, expiryFrom, lst time.Time) []string {
	probes := []float64{candidates[len(candidates)/2]}
	if len(candidates) > 6 {
		probes = append(probes, candidates[len(candidates)/4], candidates[len(candidates)*3/4])
	}
	if len(candidates) > 12 {
		probes = append(probes, candidates[1], candidates[len(candidates)-2])
	}
	for _, strike := range probes {
		var found []string
		for _, suf := range []string{"", "B", "C", "D", "E"} {
			probe := fmt.Sprintf("%s%sB%s%d%s", prefix, strikeString(strike), letter, year%10, suf)
			rows, err := r.series(probe, "options", expiryFrom, lst.AddDate(0, 1, 0), []string{"TRADEDATE", "WAPRICE", "SETTLEPRICE", "VOLUME"})
			if err != nil {
				continue
			}
			if len(rows) >= 3 {
				found = append(found, suf)
			}
		}
		if len(found) > 0 {
			return found
		}
	}
	return nil
}

func strikeStep(strikes []float64) float64 {
	best := math.Inf(1)
	for i := 1; i < len(strikes); i++ {
		if d := strikes[i] - strikes[i-1]; d > 0 && d < best {
			best = d
		}
	}
	if math.IsInf(best, 1) {
		return 0
	}
	return best
}

func roundTo(v float64, step float64) float64 {
	if step <= 0 {
		return v
	}
	return math.Round(v/step) * step
}
