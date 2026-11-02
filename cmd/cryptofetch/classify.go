package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

type contractInfo struct {
	Symbol         string `json:"symbol"`
	QuoteAsset     string `json:"quoteAsset"`
	Status         string `json:"status"`
	UnderlyingType string `json:"underlyingType"`
	OnboardDate    int64  `json:"onboardDate"`
}

type exchangeInfo struct {
	Symbols []contractInfo `json:"symbols"`
}

type ticker24 struct {
	Symbol      string `json:"symbol"`
	QuoteVolume string `json:"quoteVolume"`
}

func classify(o options, venueDir string) error {
	client := &http.Client{Timeout: 60 * time.Second}
	infoBody, err := getWithRetries(client, "https://fapi.binance.com/fapi/v1/exchangeInfo", o.retries)
	if err != nil {
		return err
	}
	var info exchangeInfo
	if err := json.Unmarshal(infoBody, &info); err != nil {
		return err
	}
	tickBody, err := getWithRetries(client, "https://fapi.binance.com/fapi/v1/ticker/24hr", o.retries)
	if err != nil {
		return err
	}
	var tickers []ticker24
	if err := json.Unmarshal(tickBody, &tickers); err != nil {
		return err
	}
	vol := make(map[string]float64, len(tickers))
	for _, t := range tickers {
		v, err := strconv.ParseFloat(t.QuoteVolume, 64)
		if err != nil {
			continue
		}
		vol[t.Symbol] = v
	}
	months, err := readSymbolRows(symbolsPath(o, venueDir))
	if err != nil {
		return err
	}
	byName := make(map[string]symbolRow, len(months))
	for _, r := range months {
		byName[r.Symbol] = r
	}
	contractsPath := filepath.Join(venueDir, "contracts.csv")
	cf, err := os.Create(contractsPath)
	if err != nil {
		return err
	}
	cw := csv.NewWriter(cf)
	cw.Write([]string{"symbol", "quote_asset", "status", "underlying_type", "onboard_date"})
	excludePath := filepath.Join(venueDir, "exclude.csv")
	ef, err := os.Create(excludePath)
	if err != nil {
		cf.Close()
		return err
	}
	ew := csv.NewWriter(ef)
	ew.Write([]string{"symbol", "underlying_type"})
	type prio struct {
		symbol string
		vol    float64
	}
	var prios []prio
	excluded := 0
	for _, c := range info.Symbols {
		cw.Write([]string{c.Symbol, c.QuoteAsset, c.Status, c.UnderlyingType, strconv.FormatInt(c.OnboardDate, 10)})
		if c.UnderlyingType != "COIN" {
			ew.Write([]string{c.Symbol, c.UnderlyingType})
			excluded++
			continue
		}
		if c.QuoteAsset != "USDT" || c.Status != "TRADING" {
			continue
		}
		if _, ok := byName[c.Symbol]; !ok {
			continue
		}
		prios = append(prios, prio{symbol: c.Symbol, vol: vol[c.Symbol]})
	}
	cw.Flush()
	ew.Flush()
	cf.Close()
	ef.Close()
	sort.Slice(prios, func(a, b int) bool { return prios[a].vol > prios[b].vol })
	prioPath := filepath.Join(venueDir, "priority.csv")
	pf, err := os.Create(prioPath)
	if err != nil {
		return err
	}
	defer pf.Close()
	pw := csv.NewWriter(pf)
	pw.Write([]string{"symbol", "first_month", "last_month", "months"})
	for _, p := range prios {
		r := byName[p.symbol]
		pw.Write([]string{r.Symbol, r.FirstMonth, r.LastMonth, strconv.Itoa(r.Months)})
	}
	pw.Flush()
	if err := pw.Error(); err != nil {
		return err
	}
	fmt.Printf("contracts %d, excluded non-COIN %d, priority crypto USDT %d\n",
		len(info.Symbols), excluded, len(prios))
	return nil
}

func readSymbolRows(path string) ([]symbolRow, error) {
	rows, err := readSymbols(path)
	if err != nil {
		return nil, err
	}
	return rows, nil
}
