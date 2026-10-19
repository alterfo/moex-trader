package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func page(client *http.Client, url string) ([][]interface{}, error) {
	var last error
	for attempt := 0; attempt < 5; attempt++ {
		resp, err := client.Get(url)
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
		var out map[string]struct {
			Data [][]interface{} `json:"data"`
		}
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, err
		}
		for _, block := range out {
			return block.Data, nil
		}
		return nil, nil
	}
	return nil, last
}

func num(v interface{}) string {
	f, ok := v.(float64)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%g", f)
}

func candles(client *http.Client, secid string, interval int, from, till string) ([]string, error) {
	var rows []string
	start := 0
	for {
		url := fmt.Sprintf("https://iss.moex.com/iss/engines/stock/markets/shares/boards/TQBR/securities/%s/candles.json?interval=%d&from=%s&till=%s&start=%d&iss.meta=off", secid, interval, from, till, start)
		data, err := page(client, url)
		if err != nil {
			return nil, err
		}
		if len(data) == 0 {
			return rows, nil
		}
		for _, r := range data {
			if len(r) < 8 {
				continue
			}
			b, _ := r[6].(string)
			rows = append(rows, fmt.Sprintf("%s,%s,%s,%s,%s,%s,%s", b, num(r[0]), num(r[1]), num(r[2]), num(r[3]), num(r[4]), num(r[5])))
		}
		start += len(data)
	}
}

func indexHistory(client *http.Client, secid, from, till string) ([]string, error) {
	var rows []string
	start := 0
	for {
		url := fmt.Sprintf("https://iss.moex.com/iss/history/engines/stock/markets/index/securities/%s.json?from=%s&till=%s&start=%d&iss.meta=off&iss.only=history", secid, from, till, start)
		data, err := page(client, url)
		if err != nil {
			return nil, err
		}
		if len(data) == 0 {
			return rows, nil
		}
		for _, r := range data {
			if len(r) < 9 {
				continue
			}
			d, _ := r[2].(string)
			rows = append(rows, fmt.Sprintf("%s,%s,%s,%s,%s", d, num(r[6]), num(r[5]), num(r[7]), num(r[8])))
		}
		start += len(data)
	}
}

func main() {
	tickers := flag.String("tickers", "SBER,SBERP,LKOH,GAZP,GMKN,ROSN,NVTK,TATN,TATNP,SNGS,SNGSP,MTSS,MGNT,PLZL,CHMF,MAGN,NLMK,VTBR,AFLT,ALRS,MOEX,PIKK,IRAO,HYDR,FEES,PHOR,RUAL,AFKS,YNDX,YDEX", "share SECIDs")
	index := flag.String("index", "", "index SECID for daily history (e.g. IMOEX)")
	interval := flag.Int("interval", 10, "candle interval, minutes")
	from := flag.String("from", "2011-12-15", "first day")
	till := flag.String("till", "2026-10-16", "last day")
	out := flag.String("out", "intraday", "output directory")
	workers := flag.Int("workers", 6, "parallel tickers")
	flag.Parse()
	client := &http.Client{Timeout: 60 * time.Second}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	if *index != "" {
		rows, err := indexHistory(client, *index, *from, *till)
		if err != nil {
			log.Fatal(err)
		}
		f, _ := os.Create(filepath.Join(*out, strings.ToLower(*index)+"_daily.csv"))
		fmt.Fprintln(f, "date,open,close,high,low")
		for _, r := range rows {
			fmt.Fprintln(f, r)
		}
		f.Close()
		log.Printf("intradayfetch: %s %d rows", *index, len(rows))
		return
	}
	jobs := make(chan string)
	var wg sync.WaitGroup
	for w := 0; w < *workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range jobs {
				rows := []string{}
				ok := true
				for y := mustYear(*from); y <= mustYear(*till); y++ {
					a, b := fmt.Sprintf("%d-01-01", y), fmt.Sprintf("%d-12-31", y)
					if a < *from {
						a = *from
					}
					if b > *till {
						b = *till
					}
					r, err := candles(client, t, *interval, a, b)
					if err != nil {
						log.Printf("intradayfetch: %s %d failed: %v", t, y, err)
						ok = false
						break
					}
					rows = append(rows, r...)
				}
				if !ok {
					continue
				}
				f, _ := os.Create(filepath.Join(*out, t+".csv"))
				fmt.Fprintln(f, "begin,open,close,high,low,value,volume")
				for _, r := range rows {
					fmt.Fprintln(f, r)
				}
				f.Close()
				log.Printf("intradayfetch: %s %d rows", t, len(rows))
			}
		}()
	}
	for _, t := range strings.Split(*tickers, ",") {
		jobs <- strings.TrimSpace(t)
	}
	close(jobs)
	wg.Wait()
}

func mustYear(d string) int {
	t, err := time.Parse("2006-01-02", d)
	if err != nil {
		log.Fatal(err)
	}
	return t.Year()
}
