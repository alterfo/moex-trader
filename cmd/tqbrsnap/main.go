package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

type response struct {
	History struct {
		Columns []string        `json:"columns"`
		Data    [][]interface{} `json:"data"`
	} `json:"history"`
}

func get(client *http.Client, url string) ([]byte, error) {
	var last error
	for attempt := 0; attempt < 4; attempt++ {
		resp, err := client.Get(url)
		if err == nil {
			body, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr == nil && resp.StatusCode == 200 {
				return body, nil
			}
			last = fmt.Errorf("http %d %v", resp.StatusCode, readErr)
		} else {
			last = err
		}
		time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
	}
	return nil, last
}

func day(client *http.Client, d time.Time) ([][]string, error) {
	var rows [][]string
	for start := 0; ; {
		url := fmt.Sprintf("https://iss.moex.com/iss/history/engines/stock/markets/shares/boards/TQBR/securities.json?date=%s&start=%d&iss.meta=off&iss.only=history&history.columns=SECID,OPEN,HIGH,LOW,CLOSE,VOLUME,VALUE,NUMTRADES", d.Format("2006-01-02"), start)
		body, err := get(client, url)
		if err != nil {
			return nil, err
		}
		var r response
		if err := json.Unmarshal(body, &r); err != nil {
			return nil, err
		}
		if len(r.History.Data) == 0 {
			break
		}
		for _, row := range r.History.Data {
			out := []string{d.Format("2006-01-02")}
			for _, v := range row {
				if v == nil {
					out = append(out, "")
				} else {
					out = append(out, fmt.Sprintf("%v", v))
				}
			}
			rows = append(rows, out)
		}
		start += len(r.History.Data)
	}
	return rows, nil
}

func main() {
	fromStr := flag.String("from", "2014-01-01", "first day")
	tillStr := flag.String("till", "", "last day (default today)")
	out := flag.String("out", "tqbr_snap.csv", "output CSV")
	workers := flag.Int("workers", 6, "parallel workers")
	flag.Parse()
	from, err := time.Parse("2006-01-02", *fromStr)
	if err != nil {
		log.Fatal(err)
	}
	till := time.Now()
	if *tillStr != "" {
		if till, err = time.Parse("2006-01-02", *tillStr); err != nil {
			log.Fatal(err)
		}
	}
	var days []time.Time
	for d := from; !d.After(till); d = d.AddDate(0, 0, 1) {
		if wd := d.Weekday(); wd != time.Saturday && wd != time.Sunday {
			days = append(days, d)
		}
	}
	f, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	_ = w.Write([]string{"date", "ticker", "open", "high", "low", "close", "volume", "value", "numtrades"})
	var mu sync.Mutex
	var wg sync.WaitGroup
	jobs := make(chan time.Time)
	client := &http.Client{Timeout: 60 * time.Second}
	done := 0
	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for d := range jobs {
				rows, err := day(client, d)
				if err != nil {
					log.Printf("%s: %v", d.Format("2006-01-02"), err)
					continue
				}
				mu.Lock()
				for _, r := range rows {
					_ = w.Write(r)
				}
				done++
				if done%200 == 0 {
					fmt.Fprintf(os.Stderr, "%d/%d days\n", done, len(days))
				}
				mu.Unlock()
			}
		}()
	}
	for _, d := range days {
		jobs <- d
	}
	close(jobs)
	wg.Wait()
	fmt.Fprintf(os.Stderr, "done %d days\n", done)
}
