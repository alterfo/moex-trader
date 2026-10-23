package main

import (
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var base = "https://apim.moex.com/iss/datashop/algopack/eq"

type block struct {
	Columns []string        `json:"columns"`
	Data    [][]interface{} `json:"data"`
}

type reply struct {
	Data   block `json:"data"`
	Cursor block `json:"data.cursor"`
	Dates  block `json:"data.dates"`
}

type page struct {
	columns  []string
	rows     [][]interface{}
	total    int
	pageSize int
}

func getJSON(client *http.Client, token, url string) (reply, error) {
	var last error
	for attempt := 0; attempt < 6; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*attempt) * time.Second)
		}
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := client.Do(req)
		if err != nil {
			last = err
			continue
		}
		data, rerr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return reply{}, fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
		}
		if resp.StatusCode != http.StatusOK || rerr != nil {
			last = fmt.Errorf("http %d", resp.StatusCode)
			continue
		}
		var out reply
		if err := json.Unmarshal(data, &out); err != nil {
			last = err
			continue
		}
		return out, nil
	}
	return reply{}, last
}

func fetchPage(client *http.Client, token, dataset, day string, start int) (page, error) {
	url := fmt.Sprintf("%s/%s.json?date=%s&iss.meta=off&start=%d", base, dataset, day, start)
	out, err := getJSON(client, token, url)
	if err != nil {
		return page{}, err
	}
	p := page{columns: out.Data.Columns, rows: out.Data.Data}
	if len(out.Cursor.Data) > 0 && len(out.Cursor.Data[0]) >= 3 {
		total, _ := out.Cursor.Data[0][1].(float64)
		size, _ := out.Cursor.Data[0][2].(float64)
		p.total, p.pageSize = int(total), int(size)
	}
	return p, nil
}

func fetchDay(client *http.Client, token, dataset, day string) ([]string, [][]interface{}, error) {
	var columns []string
	var rows [][]interface{}
	start := 0
	for {
		p, err := fetchPage(client, token, dataset, day, start)
		if err != nil {
			return nil, nil, err
		}
		if columns == nil {
			columns = p.columns
		}
		rows = append(rows, p.rows...)
		if len(p.rows) == 0 || p.pageSize == 0 {
			break
		}
		start += len(p.rows)
		if start >= p.total {
			break
		}
	}
	return columns, rows, nil
}

func cell(v interface{}) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		if strings.ContainsAny(x, ",\"\n") {
			return strconv.Quote(x)
		}
		return x
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	}
	return fmt.Sprint(v)
}

func writeDay(path string, columns []string, rows [][]interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	zw := gzip.NewWriter(f)
	fmt.Fprintln(zw, strings.Join(columns, ","))
	parts := make([]string, len(columns))
	for _, r := range rows {
		for i := range columns {
			if i < len(r) {
				parts[i] = cell(r[i])
			} else {
				parts[i] = ""
			}
		}
		fmt.Fprintln(zw, strings.Join(parts, ","))
	}
	if err := zw.Close(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func weekdays(from, till time.Time) []string {
	var days []string
	for d := from; !d.After(till); d = d.AddDate(0, 0, 1) {
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			continue
		}
		days = append(days, d.Format("2006-01-02"))
	}
	return days
}

func main() {
	market := flag.String("market", "eq", "datashop market: eq, fo or fx")
	datasets := flag.String("datasets", "", "datashop datasets, default depends on the market")
	from := flag.String("from", "2020-01-03", "first day")
	till := flag.String("till", "", "last day, default today")
	out := flag.String("out", "data/algopack", "output directory")
	tokenEnv := flag.String("token-env", "MOEX_TRADER_ALGOPACK_TOKEN", "token env var")
	workers := flag.Int("workers", 6, "parallel days")
	dates := flag.String("dates", "", "explicit comma-separated days, overrides from and till")
	flag.Parse()
	base = "https://apim.moex.com/iss/datashop/algopack/" + *market
	if *datasets == "" {
		*datasets = "tradestats,obstats,orderstats,hi2"
		if *market != "eq" {
			*datasets = "tradestats,obstats,orderstats"
		}
	}
	token := strings.TrimSpace(os.Getenv(*tokenEnv))
	if token == "" {
		log.Fatal("algopackfetch: token is empty")
	}
	start, err := time.Parse("2006-01-02", *from)
	if err != nil {
		log.Fatal(err)
	}
	end := time.Now()
	if *till != "" {
		if end, err = time.Parse("2006-01-02", *till); err != nil {
			log.Fatal(err)
		}
	}
	client := &http.Client{Timeout: 90 * time.Second}
	type job struct{ dataset, day string }
	var queue []job
	for _, ds := range strings.Split(*datasets, ",") {
		ds = strings.TrimSpace(ds)
		days := weekdays(start, end)
		if *dates != "" {
			days = strings.Split(*dates, ",")
		}
		for _, d := range days {
			if _, err := os.Stat(filepath.Join(*out, ds, d+".csv.gz")); err == nil {
				continue
			}
			queue = append(queue, job{ds, d})
		}
	}
	log.Printf("algopackfetch: %d day files to fetch", len(queue))
	jobs := make(chan job)
	var wg sync.WaitGroup
	var done, failed, nonEmpty, rowTotal int64
	for w := 0; w < *workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				cols, rows, err := fetchDay(client, token, j.dataset, j.day)
				if err == nil && len(cols) > 0 {
					err = writeDay(filepath.Join(*out, j.dataset, j.day+".csv.gz"), cols, rows)
				}
				if err != nil {
					atomic.AddInt64(&failed, 1)
					log.Printf("algopackfetch: %s %s: %v", j.dataset, j.day, err)
				} else if len(rows) > 0 {
					atomic.AddInt64(&nonEmpty, 1)
					atomic.AddInt64(&rowTotal, int64(len(rows)))
				}
				if n := atomic.AddInt64(&done, 1); n%100 == 0 {
					log.Printf("algopackfetch: %d/%d done, %d failed, %d rows", n, len(queue), atomic.LoadInt64(&failed), atomic.LoadInt64(&rowTotal))
				}
			}
		}()
	}
	for _, j := range queue {
		jobs <- j
	}
	close(jobs)
	wg.Wait()
	log.Printf("algopackfetch: %d files, %d with rows, %d rows, %d failed", done, nonEmpty, rowTotal, failed)
	if failed > 0 {
		os.Exit(1)
	}
}
