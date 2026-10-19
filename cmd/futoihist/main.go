package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

type row struct {
	date, tm, ticker, group string
	pos, posLong, posShort  float64
}

func fetch(client *http.Client, token, series, day string) ([]row, error) {
	url := fmt.Sprintf("https://apim.moex.com/iss/analyticalproducts/futoi/securities/%s.json?from=%s&till=%s&iss.meta=off", strings.ToLower(series), day, day)
	var last error
	for attempt := 0; attempt < 4; attempt++ {
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := client.Do(req)
		if err != nil {
			last = err
			time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
			continue
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			last = fmt.Errorf("http %d", resp.StatusCode)
			time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
			continue
		}
		var out struct {
			Futoi struct {
				Data [][]interface{} `json:"data"`
			} `json:"futoi"`
		}
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, err
		}
		var rows []row
		for _, r := range out.Futoi.Data {
			if len(r) < 9 {
				continue
			}
			tm, _ := r[3].(string)
			if tm != "18:45:00" && tm != "23:50:00" {
				continue
			}
			d, _ := r[2].(string)
			tk, _ := r[4].(string)
			g, _ := r[5].(string)
			p, _ := r[6].(float64)
			pl, _ := r[7].(float64)
			ps, _ := r[8].(float64)
			rows = append(rows, row{d, tm, tk, g, p, pl, ps})
		}
		return rows, nil
	}
	return nil, last
}

func main() {
	series := flag.String("series", "MX,RI,Si,BR,GD,SR,GZ,LK,GK,RN,TN,MG,VB,YD,AF,AL,MT,NM,NV,MM,X5,HY,SN,PZ,CNYRUBF", "futoi tickers")
	from := flag.String("from", "2020-01-01", "first day")
	till := flag.String("till", "2026-10-16", "last day")
	out := flag.String("out", "futoi_hist.csv", "output CSV")
	tokenEnv := flag.String("token-env", "MOEX_TRADER_ALGOPACK_TOKEN", "token env var")
	workers := flag.Int("workers", 8, "parallel requests")
	flag.Parse()
	token := strings.TrimSpace(os.Getenv(*tokenEnv))
	if token == "" {
		log.Fatal("futoihist: token is empty")
	}
	start, _ := time.Parse("2006-01-02", *from)
	end, _ := time.Parse("2006-01-02", *till)
	type job struct{ series, day string }
	jobs := make(chan job)
	var mu sync.Mutex
	var all []row
	failed := 0
	var wg sync.WaitGroup
	client := &http.Client{Timeout: 40 * time.Second}
	for w := 0; w < *workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				rows, err := fetch(client, token, j.series, j.day)
				mu.Lock()
				if err != nil {
					failed++
				} else {
					all = append(all, rows...)
				}
				mu.Unlock()
			}
		}()
	}
	for _, s := range strings.Split(*series, ",") {
		for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
			if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
				continue
			}
			jobs <- job{strings.TrimSpace(s), d.Format("2006-01-02")}
		}
	}
	close(jobs)
	wg.Wait()
	sort.Slice(all, func(i, j int) bool {
		a, b := all[i], all[j]
		if a.ticker != b.ticker {
			return a.ticker < b.ticker
		}
		if a.date != b.date {
			return a.date < b.date
		}
		if a.tm != b.tm {
			return a.tm < b.tm
		}
		return a.group < b.group
	})
	f, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	fmt.Fprintln(f, "ticker,date,time,group,pos,pos_long,pos_short")
	for _, r := range all {
		fmt.Fprintf(f, "%s,%s,%s,%s,%.0f,%.0f,%.0f\n", r.ticker, r.date, r.tm, r.group, r.pos, r.posLong, r.posShort)
	}
	log.Printf("futoihist: %d rows, %d failed requests", len(all), failed)
}
