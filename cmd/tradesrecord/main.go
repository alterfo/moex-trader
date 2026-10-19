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
	"sort"
	"strings"
	"sync"
	"time"
)

type trade struct {
	no                        int64
	date, tm, buysell, period string
	price, qty, value         float64
}

func fetchPage(client *http.Client, token, secid string, from int64) ([]trade, error) {
	url := fmt.Sprintf("https://apim.moex.com/iss/engines/stock/markets/shares/boards/TQBR/securities/%s/trades.json?iss.meta=off&limit=5000&tradeno=%d", secid, from)
	var last error
	for attempt := 0; attempt < 5; attempt++ {
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
			Trades struct {
				Columns []string        `json:"columns"`
				Data    [][]interface{} `json:"data"`
			} `json:"trades"`
		}
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, err
		}
		return parse(out.Trades.Columns, out.Trades.Data), nil
	}
	return nil, last
}

func parse(cols []string, data [][]interface{}) []trade {
	idx := map[string]int{}
	for i, c := range cols {
		idx[c] = i
	}
	str := func(r []interface{}, c string) string {
		s, _ := r[idx[c]].(string)
		return s
	}
	num := func(r []interface{}, c string) float64 {
		f, _ := r[idx[c]].(float64)
		return f
	}
	var rows []trade
	for _, r := range data {
		if len(r) < len(cols) {
			continue
		}
		rows = append(rows, trade{
			no:      int64(num(r, "TRADENO")),
			date:    str(r, "TRADEDATE"),
			tm:      str(r, "TRADETIME"),
			buysell: str(r, "BUYSELL"),
			period:  str(r, "PERIOD"),
			price:   num(r, "PRICE"),
			qty:     num(r, "QUANTITY"),
			value:   num(r, "VALUE"),
		})
	}
	return rows
}

func collect(client *http.Client, token, secid string) ([]trade, error) {
	seen := map[int64]bool{}
	var all []trade
	var from int64
	for {
		rows, err := fetchPage(client, token, secid, from)
		if err != nil {
			return nil, err
		}
		added := 0
		for _, t := range rows {
			if seen[t.no] {
				continue
			}
			seen[t.no] = true
			all = append(all, t)
			added++
			if t.no > from {
				from = t.no
			}
		}
		if added == 0 {
			break
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].no < all[j].no })
	return all, nil
}

func lineCount(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return 0
	}
	data, _ := io.ReadAll(zr)
	return strings.Count(string(data), "\n") - 1
}

func write(dir, secid string, rows []trade) error {
	byDate := map[string][]trade{}
	for _, t := range rows {
		byDate[t.date] = append(byDate[t.date], t)
	}
	for d, rs := range byDate {
		path := filepath.Join(dir, d, secid+".csv.gz")
		if lineCount(path) >= len(rs) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		tmp := path + ".tmp"
		f, err := os.Create(tmp)
		if err != nil {
			return err
		}
		zw := gzip.NewWriter(f)
		fmt.Fprintln(zw, "tradeno,time,price,qty,value,buysell,period")
		for _, t := range rs {
			fmt.Fprintf(zw, "%d,%s,%g,%g,%g,%s,%s\n", t.no, t.tm, t.price, t.qty, t.value, t.buysell, t.period)
		}
		zw.Close()
		f.Close()
		if err := os.Rename(tmp, path); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	tickers := flag.String("tickers", "SBER,SBERP,LKOH,GAZP,GMKN,ROSN,NVTK,TATN,TATNP,SNGS,SNGSP,MTSS,MGNT,PLZL,CHMF,MAGN,NLMK,VTBR,AFLT,ALRS,MOEX,PIKK,IRAO,HYDR,FEES,PHOR,RUAL,AFKS,YDEX,T,OZON,POSI,DATA", "share SECIDs")
	out := flag.String("out", "trades", "output directory")
	tokenEnv := flag.String("token-env", "MOEX_TRADER_ALGOPACK_TOKEN", "token env var")
	workers := flag.Int("workers", 4, "parallel tickers")
	flag.Parse()
	token := strings.TrimSpace(os.Getenv(*tokenEnv))
	if token == "" {
		log.Fatal("tradesrecord: token is empty")
	}
	client := &http.Client{Timeout: 60 * time.Second}
	jobs := make(chan string)
	var wg sync.WaitGroup
	var mu sync.Mutex
	failed, total := 0, 0
	for w := 0; w < *workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for s := range jobs {
				rows, err := collect(client, token, s)
				if err == nil {
					err = write(*out, s, rows)
				}
				mu.Lock()
				if err != nil {
					failed++
					log.Printf("tradesrecord: %s: %v", s, err)
				} else {
					total += len(rows)
				}
				mu.Unlock()
			}
		}()
	}
	for _, s := range strings.Split(*tickers, ",") {
		jobs <- strings.TrimSpace(s)
	}
	close(jobs)
	wg.Wait()
	log.Printf("tradesrecord: %d trades, %d failed tickers", total, failed)
	if failed > 0 {
		os.Exit(1)
	}
}
