package dividends

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type Record struct {
	Ticker       string    `json:"ticker"`
	Figi         string    `json:"figi"`
	DeclaredDate time.Time `json:"declared_date"`
	LastBuyDate  time.Time `json:"last_buy_date"`
	PaymentDate  time.Time `json:"payment_date"`
	DividendNet  string    `json:"dividend_net"`
	Regularity   string    `json:"regularity"`
	DividendType string    `json:"dividend_type"`
}

func (r Record) Net() (decimal.Decimal, error) {
	return decimal.NewFromString(strings.TrimSpace(r.DividendNet))
}

func Load(path string) ([]Record, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("dividends: open %s: %w", path, err)
	}
	defer f.Close()

	var records []Record
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec Record
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, fmt.Errorf("dividends: %s:%d: invalid record: %w", path, lineNo, err)
		}
		if strings.TrimSpace(rec.Ticker) == "" {
			return nil, fmt.Errorf("dividends: %s:%d: ticker must not be empty", path, lineNo)
		}
		if _, err := rec.Net(); err != nil {
			return nil, fmt.Errorf("dividends: %s:%d: %s: invalid dividend_net: %w", path, lineNo, rec.Ticker, err)
		}
		records = append(records, rec)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("dividends: read %s: %w", path, err)
	}
	return records, nil
}
