package main

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
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

const (
	bucketURL  = "https://s3-ap-northeast-1.amazonaws.com/data.binance.vision"
	cdnURL     = "https://data.binance.vision"
	venueName  = "binance-um"
	klinesPfx  = "data/futures/um/monthly/klines/"
	fundingPfx = "data/futures/um/monthly/fundingRate/"
)

type options struct {
	mode        string
	dataDir     string
	interval    string
	concurrency int
	retries     int
	limit       int
	from        string
	to          string
	symbols     string
	timeout     time.Duration
}

func main() {
	var o options
	flag.StringVar(&o.mode, "mode", "inventory", "symbols | klines | funding | inventory")
	flag.StringVar(&o.dataDir, "data", "data/crypto", "root data directory")
	flag.StringVar(&o.interval, "interval", "1d", "kline interval")
	flag.IntVar(&o.concurrency, "concurrency", 8, "parallel downloads")
	flag.IntVar(&o.retries, "retries", 4, "attempts per object")
	flag.IntVar(&o.limit, "limit", 0, "max symbols (0 = all)")
	flag.StringVar(&o.from, "from", "", "first month YYYY-MM (default from symbols.csv)")
	flag.StringVar(&o.to, "to", "", "last month YYYY-MM (default from symbols.csv)")
	flag.StringVar(&o.symbols, "symbols", "", "symbols file (default <data>/<venue>/symbols.csv)")
	flag.DurationVar(&o.timeout, "timeout", 60*time.Second, "per-request timeout")
	flag.Parse()

	if err := run(o); err != nil {
		log.Fatalf("%s: %v", o.mode, err)
	}
}

func run(o options) error {
	venueDir := filepath.Join(o.dataDir, venueName)
	if err := os.MkdirAll(venueDir, 0o755); err != nil {
		return err
	}
	switch o.mode {
	case "symbols":
		return fetchSymbols(o, venueDir)
	case "klines":
		return fetchKlines(o, venueDir)
	case "funding":
		return fetchFunding(o, venueDir)
	case "inventory":
		return inventory(o, venueDir)
	case "classify":
		return classify(o, venueDir)
	default:
		return fmt.Errorf("unknown mode %q", o.mode)
	}
}

type symbolRow struct {
	Symbol     string
	FirstMonth string
	LastMonth  string
	Months     int
}

func symbolsPath(o options, venueDir string) string {
	if o.symbols != "" {
		return o.symbols
	}
	return filepath.Join(venueDir, "symbols.csv")
}

func fetchSymbols(o options, venueDir string) error {
	client := newClient(o.timeout)
	prefixes, err := listCommonPrefixes(client, klinesPfx, o.retries)
	if err != nil {
		return err
	}
	symbols := make([]string, 0, len(prefixes))
	for _, p := range prefixes {
		s := strings.TrimSuffix(strings.TrimPrefix(p, klinesPfx), "/")
		if s != "" {
			symbols = append(symbols, s)
		}
	}
	sort.Strings(symbols)
	if o.limit > 0 && len(symbols) > o.limit {
		symbols = symbols[:o.limit]
	}
	rows := make([]symbolRow, len(symbols))
	sem := make(chan struct{}, o.concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	for i, sym := range symbols {
		wg.Add(1)
		go func(i int, sym string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			months, err := listMonthKeys(client, klinesPfx+sym+"/"+o.interval+"/", o.retries)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("%s: %w", sym, err)
				}
				return
			}
			if len(months) == 0 {
				return
			}
			sort.Strings(months)
			rows[i] = symbolRow{Symbol: sym, FirstMonth: months[0], LastMonth: months[len(months)-1], Months: len(months)}
		}(i, sym)
	}
	wg.Wait()
	if firstErr != nil {
		return firstErr
	}
	path := filepath.Join(venueDir, "symbols.csv")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if err := w.Write([]string{"symbol", "first_month", "last_month", "months"}); err != nil {
		return err
	}
	n := 0
	for _, r := range rows {
		if r.Symbol == "" || r.Months == 0 {
			continue
		}
		if err := w.Write([]string{r.Symbol, r.FirstMonth, r.LastMonth, strconv.Itoa(r.Months)}); err != nil {
			return err
		}
		n++
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return err
	}
	log.Printf("symbols: %d listed, %d with %s klines -> %s", len(symbols), n, o.interval, path)
	return nil
}

func listMonthKeys(client *http.Client, prefix string, retries int) ([]string, error) {
	keys, err := listKeys(client, prefix, retries)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		base := strings.TrimSuffix(filepath.Base(k), ".zip")
		parts := strings.Split(base, "-")
		if len(parts) < 3 {
			continue
		}
		month := parts[len(parts)-2] + "-" + parts[len(parts)-1]
		if len(month) != 7 {
			continue
		}
		out = append(out, month)
	}
	return out, nil
}

func readSymbols(path string) ([]symbolRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	if _, err := r.Read(); err != nil {
		return nil, err
	}
	var rows []symbolRow
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(rec) < 4 {
			continue
		}
		n, _ := strconv.Atoi(rec[3])
		rows = append(rows, symbolRow{Symbol: rec[0], FirstMonth: rec[1], LastMonth: rec[2], Months: n})
	}
	return rows, nil
}

func monthRange(from, to string) ([]string, error) {
	start, err := time.Parse("2006-01", from)
	if err != nil {
		return nil, fmt.Errorf("from %q: %w", from, err)
	}
	end, err := time.Parse("2006-01", to)
	if err != nil {
		return nil, fmt.Errorf("to %q: %w", to, err)
	}
	var out []string
	for m := start; !m.After(end); m = m.AddDate(0, 1, 0) {
		out = append(out, m.Format("2006-01"))
	}
	return out, nil
}

func fetchKlines(o options, venueDir string) error {
	syms, err := readSymbols(symbolsPath(o, venueDir))
	if err != nil {
		return err
	}
	if o.limit > 0 && len(syms) > o.limit {
		syms = syms[:o.limit]
	}
	outDir := filepath.Join(venueDir, "klines"+o.interval)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	client := newClient(o.timeout)
	return forEachSymbol(o.concurrency, syms, func(s symbolRow) error {
		path := filepath.Join(outDir, s.Symbol+".csv")
		if fi, err := os.Stat(path); err == nil && fi.Size() > 0 {
			return nil
		}
		from, to := s.FirstMonth, s.LastMonth
		if o.from != "" {
			from = o.from
		}
		if o.to != "" {
			to = o.to
		}
		months, err := monthRange(from, to)
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		buf.WriteString("date,open_time,open,high,low,close,volume,quote_volume,trades,taker_buy_volume,taker_buy_quote_volume\n")
		rows := 0
		for _, m := range months {
			url := fmt.Sprintf("%s/%s%s/%s/%s-%s-%s.zip", cdnURL, klinesPfx, s.Symbol, o.interval, s.Symbol, o.interval, m)
			body, err := getWithRetries(client, url, o.retries)
			if err != nil {
				if isNotFound(err) {
					continue
				}
				return err
			}
			payload, err := unzipSingle(body)
			if err != nil {
				return fmt.Errorf("%s: %w", url, err)
			}
			for _, row := range parseKlines(payload) {
				if _, err := buf.WriteString(row + "\n"); err != nil {
					return err
				}
				rows++
			}
		}
		if rows == 0 {
			return nil
		}
		return os.WriteFile(path, buf.Bytes(), 0o644)
	})
}

func fetchFunding(o options, venueDir string) error {
	syms, err := readSymbols(symbolsPath(o, venueDir))
	if err != nil {
		return err
	}
	if o.limit > 0 && len(syms) > o.limit {
		syms = syms[:o.limit]
	}
	outDir := filepath.Join(venueDir, "funding")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	client := newClient(o.timeout)
	return forEachSymbol(o.concurrency, syms, func(s symbolRow) error {
		path := filepath.Join(outDir, s.Symbol+".csv")
		if fi, err := os.Stat(path); err == nil && fi.Size() > 0 {
			return nil
		}
		from, to := s.FirstMonth, s.LastMonth
		if o.from != "" {
			from = o.from
		}
		if o.to != "" {
			to = o.to
		}
		months, err := monthRange(from, to)
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		buf.WriteString("calc_time,interval_hours,rate\n")
		rows := 0
		for _, m := range months {
			url := fmt.Sprintf("%s/%s%s/%s-fundingRate-%s.zip", cdnURL, fundingPfx, s.Symbol, s.Symbol, m)
			body, err := getWithRetries(client, url, o.retries)
			if err != nil {
				if isNotFound(err) {
					continue
				}
				return err
			}
			payload, err := unzipSingle(body)
			if err != nil {
				return fmt.Errorf("%s: %w", url, err)
			}
			for _, row := range parseFunding(payload) {
				if _, err := buf.WriteString(row + "\n"); err != nil {
					return err
				}
				rows++
			}
		}
		if rows == 0 {
			return nil
		}
		return os.WriteFile(path, buf.Bytes(), 0o644)
	})
}

func forEachSymbol(concurrency int, syms []symbolRow, fn func(symbolRow) error) error {
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	done := 0
	for _, s := range syms {
		wg.Add(1)
		go func(s symbolRow) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			err := fn(s)
			mu.Lock()
			defer mu.Unlock()
			done++
			if err != nil && firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", s.Symbol, err)
			}
			if done%100 == 0 {
				log.Printf("progress: %d/%d", done, len(syms))
			}
		}(s)
	}
	wg.Wait()
	return firstErr
}

func parseKlines(payload []byte) []string {
	lines := strings.Split(string(payload), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "open_time") {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 11 {
			continue
		}
		ms, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			continue
		}
		date := time.UnixMilli(ms).UTC().Format("2006-01-02")
		fields := make([]string, 0, 11)
		fields = append(fields, date, parts[0])
		fields = append(fields, parts[1], parts[2], parts[3], parts[4])
		fields = append(fields, parts[5], parts[7], parts[8])
		fields = append(fields, parts[9], parts[10])
		out = append(out, strings.Join(fields, ","))
	}
	return out
}

func parseFunding(payload []byte) []string {
	lines := strings.Split(string(payload), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "calc_time") {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 3 {
			continue
		}
		out = append(out, strings.Join(parts[:3], ","))
	}
	return out
}

type s3List struct {
	IsTruncated bool   `xml:"IsTruncated"`
	NextMarker  string `xml:"NextMarker"`
	Contents    []struct {
		Key string `xml:"Key"`
	} `xml:"Contents"`
	CommonPrefixes []struct {
		Prefix string `xml:"Prefix"`
	} `xml:"CommonPrefixes"`
}

func listCommonPrefixes(client *http.Client, prefix string, retries int) ([]string, error) {
	var out []string
	marker := ""
	for {
		url := fmt.Sprintf("%s?delimiter=/&prefix=%s", bucketURL, prefix)
		if marker != "" {
			url += "&marker=" + marker
		}
		body, err := getWithRetries(client, url, retries)
		if err != nil {
			return nil, err
		}
		var res s3List
		if err := xml.Unmarshal(body, &res); err != nil {
			return nil, fmt.Errorf("parse listing: %w", err)
		}
		for _, p := range res.CommonPrefixes {
			out = append(out, p.Prefix)
		}
		if !res.IsTruncated || res.NextMarker == "" || res.NextMarker == marker {
			break
		}
		marker = res.NextMarker
	}
	return out, nil
}

func listKeys(client *http.Client, prefix string, retries int) ([]string, error) {
	var out []string
	marker := ""
	for {
		url := fmt.Sprintf("%s?prefix=%s", bucketURL, prefix)
		if marker != "" {
			url += "&marker=" + marker
		}
		body, err := getWithRetries(client, url, retries)
		if err != nil {
			return nil, err
		}
		var res s3List
		if err := xml.Unmarshal(body, &res); err != nil {
			return nil, fmt.Errorf("parse listing: %w", err)
		}
		for _, c := range res.Contents {
			out = append(out, c.Key)
		}
		if !res.IsTruncated || res.NextMarker == "" || res.NextMarker == marker {
			break
		}
		marker = res.NextMarker
	}
	return out, nil
}

type statusError struct {
	code int
	url  string
}

func (e statusError) Error() string {
	return fmt.Sprintf("http %d: %s", e.code, e.url)
}

func isNotFound(err error) bool {
	se, ok := err.(statusError)
	return ok && (se.code == 404 || se.code == 403)
}

func newClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}

func getWithRetries(client *http.Client, url string, retries int) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < retries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
		}
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "moex-trader-cryptofetch/1.0")
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode == http.StatusOK {
			return body, nil
		}
		lastErr = statusError{code: resp.StatusCode, url: url}
		if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden {
			return nil, lastErr
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("giving up: %s", url)
	}
	return nil, lastErr
}

func unzipSingle(body []byte) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, err
	}
	if len(zr.File) == 0 {
		return nil, fmt.Errorf("empty archive")
	}
	rc, err := zr.File[0].Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

type inventoryRow struct {
	Symbol     string
	FirstMonth string
	LastMonth  string
	KlineDays  int
	FirstDate  string
	LastDate   string
	Funding    int
}

func inventory(o options, venueDir string) error {
	syms, err := readSymbols(symbolsPath(o, venueDir))
	if err != nil {
		return err
	}
	rows := make([]inventoryRow, 0, len(syms))
	for _, s := range syms {
		r := inventoryRow{Symbol: s.Symbol, FirstMonth: s.FirstMonth, LastMonth: s.LastMonth}
		if k, err := readKlineBounds(filepath.Join(venueDir, "klines"+o.interval, s.Symbol+".csv")); err == nil {
			r.KlineDays, r.FirstDate, r.LastDate = k.days, k.first, k.last
		}
		if n, err := countLines(filepath.Join(venueDir, "funding", s.Symbol+".csv")); err == nil {
			r.Funding = n
		}
		rows = append(rows, r)
	}
	active, delisted, withKlines, withFunding, days := 0, 0, 0, 0, 0
	firstDate, lastDate := "", ""
	for _, r := range rows {
		if r.KlineDays > 0 {
			withKlines++
			days += r.KlineDays
			if r.LastDate > lastDate {
				lastDate = r.LastDate
			}
			if firstDate == "" || r.FirstDate < firstDate {
				firstDate = r.FirstDate
			}
		}
		if r.Funding > 0 {
			withFunding++
		}
	}
	today := time.Now().UTC().Format("2006-01-02")
	for _, r := range rows {
		if r.LastDate != "" && daysBetween(r.LastDate, today) > 7 {
			delisted++
		} else if r.KlineDays > 0 {
			active++
		}
	}
	log.Printf("symbols: %d (%d active, %d no data in 7d)", len(rows), active, delisted)
	log.Printf("klines: %d symbols, %d rows, %s..%s", withKlines, days, firstDate, lastDate)
	log.Printf("funding: %d symbols", withFunding)
	path := filepath.Join(venueDir, "inventory.csv")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	w.Write([]string{"symbol", "first_month", "last_month", "kline_days", "first_date", "last_date", "funding_rows"})
	for _, r := range rows {
		w.Write([]string{r.Symbol, r.FirstMonth, r.LastMonth, strconv.Itoa(r.KlineDays), r.FirstDate, r.LastDate, strconv.Itoa(r.Funding)})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return err
	}
	log.Printf("inventory -> %s", path)
	return nil
}

type bounds struct {
	days        int
	first, last string
}

func readKlineBounds(path string) (bounds, error) {
	f, err := os.Open(path)
	if err != nil {
		return bounds{}, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	if _, err := r.Read(); err != nil {
		return bounds{}, err
	}
	var b bounds
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return bounds{}, err
		}
		if len(rec) == 0 {
			continue
		}
		b.days++
		if b.first == "" {
			b.first = rec[0]
		}
		b.last = rec[0]
	}
	return b, nil
}

func countLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	if _, err := r.Read(); err != nil {
		return 0, err
	}
	n := 0
	for {
		_, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}
		n++
	}
	return n, nil
}

func daysBetween(a, b string) int {
	ta, err := time.Parse("2006-01-02", a)
	if err != nil {
		return 0
	}
	tb, err := time.Parse("2006-01-02", b)
	if err != nil {
		return 0
	}
	return int(tb.Sub(ta).Hours() / 24)
}
