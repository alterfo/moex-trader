package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/tinkoff"
)

type dividendRecord struct {
	Ticker       string    `json:"ticker"`
	Figi         string    `json:"figi"`
	DeclaredDate time.Time `json:"declared_date"`
	LastBuyDate  time.Time `json:"last_buy_date"`
	PaymentDate  time.Time `json:"payment_date"`
	DividendNet  string    `json:"dividend_net"`
	Regularity   string    `json:"regularity"`
	DividendType string    `json:"dividend_type"`
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var configPath, tickersStr, outPath, fromStr, tillStr string
	flag.StringVar(&configPath, "config", "config.sandbox.yaml", "path to config YAML")
	flag.StringVar(&tickersStr, "tickers", "", "comma-separated tickers (default: config tickers)")
	flag.StringVar(&outPath, "out", "data/dividends.jsonl", "output JSONL path")
	flag.StringVar(&fromStr, "from", "2021-01-01", "dividend window start YYYY-MM-DD")
	flag.StringVar(&tillStr, "till", "2027-12-31", "dividend window end YYYY-MM-DD")
	flag.Parse()

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	tickers := cfg.Tickers
	if tickersStr != "" {
		tickers = splitComma(tickersStr)
	}
	if len(tickers) == 0 {
		return fmt.Errorf("no tickers configured")
	}
	from, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		return fmt.Errorf("parse -from: %w", err)
	}
	till, err := time.Parse("2006-01-02", tillStr)
	if err != nil {
		return fmt.Errorf("parse -till: %w", err)
	}
	token := os.Getenv("MOEX_TRADER_TINKOFF_TOKEN")
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("MOEX_TRADER_TINKOFF_TOKEN is empty")
	}
	ctx := context.Background()
	client, err := tinkoff.New(ctx, tinkoff.Config{
		Endpoint: cfg.Tinkoff.Endpoint,
		Token:    token,
	})
	if err != nil {
		return fmt.Errorf("tinkoff client: %w", err)
	}
	defer client.Close()

	var out strings.Builder
	total := 0
	for _, raw := range tickers {
		ticker := strings.ToUpper(strings.TrimSpace(raw))
		if ticker == "" {
			continue
		}
		figi, err := client.FigiByTicker(ctx, ticker)
		if err != nil {
			log.Printf("calendar: %s: skip (figi resolution): %v", ticker, err)
			continue
		}
		events, err := client.Dividends(ctx, figi, from, till)
		if err != nil {
			log.Printf("calendar: %s: skip (dividends): %v", ticker, err)
			continue
		}
		for _, ev := range events {
			if ev.DividendType == "Cancelled" {
				continue
			}
			rec := dividendRecord{
				Ticker:       ticker,
				Figi:         figi,
				DeclaredDate: ev.DeclaredDate,
				LastBuyDate:  ev.LastBuyDate,
				PaymentDate:  ev.PaymentDate,
				DividendNet:  ev.DividendNet.String(),
				Regularity:   ev.Regularity,
				DividendType: ev.DividendType,
			}
			b, err := json.Marshal(rec)
			if err != nil {
				return fmt.Errorf("%s: marshal: %w", ticker, err)
			}
			out.Write(b)
			out.WriteByte('\n')
			total++
		}
		log.Printf("calendar: %s: %d dividend events", ticker, len(events))
	}
	if err := os.MkdirAll(dirOf(outPath), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	if err := os.WriteFile(outPath, []byte(out.String()), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", outPath, err)
	}
	log.Printf("calendar: wrote %d events to %s", total, outPath)
	return nil
}

func splitComma(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func dirOf(path string) string {
	i := strings.LastIndexByte(path, '/')
	if i < 0 {
		return "."
	}
	return path[:i]
}
