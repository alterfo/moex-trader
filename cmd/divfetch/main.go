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
	"strings"
	"time"
)

type response struct {
	Dividends struct {
		Columns []string        `json:"columns"`
		Data    [][]interface{} `json:"data"`
	} `json:"dividends"`
}

func main() {
	tickers := flag.String("tickers", "", "comma-separated tickers")
	out := flag.String("out", "dividends_iss.csv", "output CSV")
	flag.Parse()
	f, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{"ticker", "registry_close_date", "value", "currency"}); err != nil {
		log.Fatal(err)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	total := 0
	for _, t := range strings.Split(*tickers, ",") {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		var body []byte
		for attempt := 0; attempt < 3; attempt++ {
			resp, err := client.Get(fmt.Sprintf("https://iss.moex.com/iss/securities/%s/dividends.json?iss.meta=off", t))
			if err == nil && resp.StatusCode == 200 {
				body, err = io.ReadAll(resp.Body)
				resp.Body.Close()
				if err == nil {
					break
				}
			} else if resp != nil {
				resp.Body.Close()
			}
			time.Sleep(time.Duration(attempt+1) * time.Second)
		}
		if body == nil {
			log.Printf("%s: no response", t)
			continue
		}
		var r response
		if err := json.Unmarshal(body, &r); err != nil {
			log.Printf("%s: %v", t, err)
			continue
		}
		idx := map[string]int{}
		for i, c := range r.Dividends.Columns {
			idx[c] = i
		}
		for _, d := range r.Dividends.Data {
			date, _ := d[idx["registryclosedate"]].(string)
			value, _ := d[idx["value"]].(float64)
			cur, _ := d[idx["currencyid"]].(string)
			if date == "" || value <= 0 {
				continue
			}
			if err := w.Write([]string{t, date, fmt.Sprintf("%g", value), cur}); err != nil {
				log.Fatal(err)
			}
			total++
		}
		time.Sleep(150 * time.Millisecond)
	}
	fmt.Fprintf(os.Stderr, "%d dividend records\n", total)
}
