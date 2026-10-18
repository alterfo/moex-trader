package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

type tickerNameEntry struct {
	Ticker    string `json:"ticker"`
	ShortName string `json:"short_name"`
	SecName   string `json:"sec_name"`
	LatName   string `json:"lat_name"`
}

type tickerNameList struct {
	FetchedAt time.Time         `json:"fetched_at"`
	Source    string            `json:"source"`
	Tickers   []tickerNameEntry `json:"tickers"`
}

type trainTickerList struct {
	FetchedAt time.Time `json:"fetched_at"`
	Source    string    `json:"source"`
	Engine    string    `json:"engine"`
	Market    string    `json:"market"`
	Board     string    `json:"board"`
	SecTypes  []string  `json:"sectypes"`
	Tickers   []string  `json:"tickers"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	baseURL := "https://iss.moex.com/iss"
	outPath := "data/train_tickers.json"
	fs := flag.NewFlagSet("listtqbr", flag.ContinueOnError)
	fs.StringVar(&baseURL, "base-url", baseURL, "MOEX ISS base URL")
	fs.StringVar(&outPath, "out", outPath, "output JSON path")
	namesOut := ""
	fs.StringVar(&namesOut, "names-out", namesOut, "also write ticker names (short/full/latin) to this JSON path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx := context.Background()
	client := moex.NewClient(baseURL, nil)
	securities, err := client.ListSecurities(ctx, "stock", "shares", "TQBR")
	if err != nil {
		return fmt.Errorf("list TQBR securities: %w", err)
	}
	list := trainTickerList{
		FetchedAt: time.Now().UTC(),
		Source:    baseURL + "/engines/stock/markets/shares/boards/TQBR/securities.json",
		Engine:    "stock",
		Market:    "shares",
		Board:     "TQBR",
		SecTypes:  []string{"1", "2"},
		Tickers:   shareTickers(securities),
	}
	if err := writeListFile(outPath, list); err != nil {
		return err
	}
	log.Printf("listtqbr: wrote %d tickers to %s", len(list.Tickers), outPath)
	if namesOut == "" {
		return nil
	}
	names, err := client.ListSecurityNames(ctx, "stock", "shares", "TQBR")
	if err != nil {
		return fmt.Errorf("list TQBR security names: %w", err)
	}
	entries := nameEntries(names)
	raw, err := json.MarshalIndent(tickerNameList{FetchedAt: time.Now().UTC(), Source: list.Source, Tickers: entries}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal ticker names: %w", err)
	}
	if err := os.WriteFile(namesOut, append(raw, '\n'), 0o644); err != nil {
		return fmt.Errorf("write ticker names: %w", err)
	}
	log.Printf("listtqbr: wrote %d ticker names to %s", len(entries), namesOut)
	return nil
}

func nameEntries(names []moex.SecurityName) []tickerNameEntry {
	seen := make(map[string]struct{}, len(names))
	var out []tickerNameEntry
	for _, n := range names {
		if n.SecType != "1" && n.SecType != "2" {
			continue
		}
		if _, ok := seen[n.SecID]; ok {
			continue
		}
		seen[n.SecID] = struct{}{}
		out = append(out, tickerNameEntry{Ticker: n.SecID, ShortName: n.ShortName, SecName: n.SecName, LatName: n.LatName})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ticker < out[j].Ticker })
	return out
}

func shareTickers(securities []moex.ListedSecurity) []string {
	seen := make(map[string]struct{}, len(securities))
	var out []string
	for _, sec := range securities {
		if sec.SecType != "1" && sec.SecType != "2" {
			continue
		}
		if _, ok := seen[sec.SecID]; ok {
			continue
		}
		seen[sec.SecID] = struct{}{}
		out = append(out, sec.SecID)
	}
	sort.Strings(out)
	return out
}

func writeListFile(path string, list trainTickerList) error {
	raw, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal ticker list: %w", err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		return fmt.Errorf("write ticker list: %w", err)
	}
	return nil
}
