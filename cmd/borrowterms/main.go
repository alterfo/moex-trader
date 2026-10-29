package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/shopspring/decimal"
	pb "github.com/tinkoff/invest-api-go-sdk/proto"

	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/tinkoff"
)

type termRecord struct {
	Date         string `json:"date"`
	Ticker       string `json:"ticker"`
	ShortEnabled bool   `json:"short_enabled"`
	Kshort       string `json:"kshort"`
	Dshort       string `json:"dshort"`
	DshortMin    string `json:"dshort_min"`
	RecordedAt   string `json:"recorded_at"`
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var configPath, tickersStr, outPath string
	var force bool
	flag.StringVar(&configPath, "config", "config.sandbox.yaml", "path to config YAML")
	flag.StringVar(&tickersStr, "tickers", "", "comma-separated tickers (overrides config)")
	flag.StringVar(&outPath, "out", "data/borrow_terms.jsonl", "append-only JSONL file with the daily short terms")
	flag.BoolVar(&force, "force", false, "record even when the file already holds today's rows")
	flag.Parse()

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.Tinkoff.Token) == "" {
		return fmt.Errorf("MOEX_TRADER_TINKOFF_TOKEN is not set (env or .env)")
	}
	tickers := cfg.Tickers
	if tickersStr != "" {
		tickers = splitComma(tickersStr)
	}
	if len(tickers) == 0 {
		return fmt.Errorf("borrowterms: no tickers configured")
	}
	sort.Strings(tickers)

	existing, err := readRecords(outPath)
	if err != nil {
		return err
	}
	today := time.Now().Format("2006-01-02")
	if !force && len(existing) > 0 && hadAllTickersToday(existing, today, tickers) {
		log.Printf("borrowterms: %s already holds today's terms for every ticker; nothing to do", outPath)
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	client, err := tinkoff.New(ctx, tinkoff.Config{
		Endpoint: cfg.Tinkoff.Endpoint,
		Token:    cfg.Tinkoff.Token,
		AppName:  "moex-trader-borrowterms",
	})
	if err != nil {
		return fmt.Errorf("dial tinkoff: %w", err)
	}
	defer client.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	records := make([]termRecord, 0, len(tickers))
	for _, ticker := range tickers {
		uid, err := client.ResolveInstrumentUID(ctx, ticker)
		if err != nil {
			log.Printf("%-12s resolve failed: %v", ticker, err)
			continue
		}
		share, err := client.ShareBy(ctx, uid)
		if err != nil {
			log.Printf("%-12s share lookup failed: %v", ticker, err)
			continue
		}
		records = append(records, termRecord{
			Date:         today,
			Ticker:       strings.ToUpper(ticker),
			ShortEnabled: share.GetShortEnabledFlag(),
			Kshort:       quotationString(share.GetKshort()),
			Dshort:       quotationString(share.GetDshort()),
			DshortMin:    quotationString(share.GetDshortMin()),
			RecordedAt:   now,
		})
	}
	if len(records) == 0 {
		return fmt.Errorf("borrowterms: no ticker resolved")
	}
	if err := appendRecords(outPath, records); err != nil {
		return err
	}
	log.Printf("borrowterms: appended %d term rows for %s to %s", len(records), today, outPath)
	return nil
}

func quotationString(q *pb.Quotation) string {
	if q == nil {
		return "0"
	}
	return decimal.NewFromInt(int64(q.GetUnits())).
		Add(decimal.New(int64(q.GetNano()), -9)).String()
}

func readRecords(path string) ([]termRecord, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []termRecord
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec termRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, fmt.Errorf("borrowterms: %s: %w", path, err)
		}
		out = append(out, rec)
	}
	return out, nil
}

func hadAllTickersToday(existing []termRecord, date string, tickers []string) bool {
	seen := map[string]bool{}
	for _, rec := range existing {
		if rec.Date == date {
			seen[rec.Ticker] = true
		}
	}
	for _, ticker := range tickers {
		if !seen[strings.ToUpper(ticker)] {
			return false
		}
	}
	return true
}

func appendRecords(path string, records []termRecord) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("borrowterms: open %s: %w", path, err)
	}
	defer file.Close()
	writer := json.NewEncoder(file)
	for _, rec := range records {
		if err := writer.Encode(rec); err != nil {
			return fmt.Errorf("borrowterms: write %s: %w", path, err)
		}
	}
	return nil
}

func splitComma(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
