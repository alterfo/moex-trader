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
	"sort"
	"strings"
	"time"
)

type response struct {
	Futoi struct {
		Columns []string        `json:"columns"`
		Data    [][]interface{} `json:"data"`
	} `json:"futoi"`
}

type point struct {
	time  string
	pos   float64
	long  float64
	short float64
}

func main() {
	series := flag.String("series", "MX,YD,SR,LK,GZ,GK,RN,TN,MG,VB", "futoi tickers")
	fromStr := flag.String("from", "2023-12-01", "first day")
	tillStr := flag.String("till", "", "last day (default today)")
	out := flag.String("out", "futoi.csv", "output CSV")
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
	f, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{"series", "date", "group", "pos", "pos_long", "pos_short"}); err != nil {
		log.Fatal(err)
	}
	client := &http.Client{Timeout: 60 * time.Second}
	for _, s := range strings.Split(*series, ",") {
		s = strings.TrimSpace(s)
		last := map[string]point{}
		for start := from; !start.After(till); start = start.AddDate(0, 0, 1) {
			if wd := start.Weekday(); wd == time.Saturday || wd == time.Sunday {
				continue
			}
			rows, err := fetch(client, s, start, start)
			if err != nil {
				log.Fatalf("%s %s: %v", s, start.Format("2006-01-02"), err)
			}
			for _, r := range rows {
				key := r.date + "|" + r.group
				if cur, ok := last[key]; !ok || r.time > cur.time {
					last[key] = point{time: r.time, pos: r.pos, long: r.long, short: r.short}
				}
			}
		}
		keys := make([]string, 0, len(last))
		for k := range last {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			parts := strings.SplitN(k, "|", 2)
			p := last[k]
			if err := w.Write([]string{s, parts[0], parts[1], fmt.Sprintf("%g", p.pos), fmt.Sprintf("%g", p.long), fmt.Sprintf("%g", p.short)}); err != nil {
				log.Fatal(err)
			}
		}
		fmt.Fprintf(os.Stderr, "%s: %d day-group points\n", s, len(keys))
	}
}

type row struct {
	date, time, group string
	pos, long, short  float64
}

func fetch(client *http.Client, series string, from, till time.Time) ([]row, error) {
	url := fmt.Sprintf("https://iss.moex.com/iss/analyticalproducts/futoi/securities/%s.json?from=%s&till=%s&iss.meta=off", strings.ToLower(series), from.Format("2006-01-02"), till.Format("2006-01-02"))
	var body []byte
	for attempt := 0; attempt < 3; attempt++ {
		resp, err := client.Get(url)
		if err == nil && resp.StatusCode == 200 {
			body, err = io.ReadAll(resp.Body)
			resp.Body.Close()
			if err == nil {
				break
			}
		} else if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
	}
	if body == nil {
		return nil, fmt.Errorf("no response for %s", url)
	}
	var r response
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	idx := map[string]int{}
	for i, c := range r.Futoi.Columns {
		idx[c] = i
	}
	num := func(v interface{}) float64 {
		x, _ := v.(float64)
		return x
	}
	str := func(v interface{}) string {
		x, _ := v.(string)
		return x
	}
	out := make([]row, 0, len(r.Futoi.Data))
	for _, d := range r.Futoi.Data {
		out = append(out, row{date: str(d[idx["tradedate"]]), time: str(d[idx["tradetime"]]), group: str(d[idx["clgroup"]]), pos: num(d[idx["pos"]]), long: num(d[idx["pos_long"]]), short: num(d[idx["pos_short"]])})
	}
	return out, nil
}
