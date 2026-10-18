package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

type csvSource struct {
	byTicker map[string][]moex.Candle
}

func newCSVSource(path string) (*csvSource, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("candles csv: %w", err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("candles csv: %w", err)
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("candles csv %q has no data rows", path)
	}
	col := map[string]int{}
	for i, name := range rows[0] {
		col[name] = i
	}
	for _, need := range []string{"ticker", "date", "open", "high", "low", "close", "volume"} {
		if _, ok := col[need]; !ok {
			return nil, fmt.Errorf("candles csv %q: missing column %q", path, need)
		}
	}
	src := &csvSource{byTicker: map[string][]moex.Candle{}}
	for _, row := range rows[1:] {
		date, err := time.Parse("2006-01-02", row[col["date"]])
		if err != nil {
			return nil, fmt.Errorf("candles csv: date %q: %w", row[col["date"]], err)
		}
		num := func(name string) (decimal.Decimal, error) {
			return decimal.NewFromString(strings.TrimSpace(row[col[name]]))
		}
		open, err1 := num("open")
		high, err2 := num("high")
		low, err3 := num("low")
		closeP, err4 := num("close")
		volume, err5 := num("volume")
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil {
			continue
		}
		ticker := strings.ToUpper(strings.TrimSpace(row[col["ticker"]]))
		src.byTicker[ticker] = append(src.byTicker[ticker], moex.Candle{
			Open: open, High: high, Low: low, Close: closeP, Volume: volume,
			Value: closeP.Mul(volume), Begin: date, End: date.Add(24*time.Hour - time.Second),
		})
	}
	for t := range src.byTicker {
		c := src.byTicker[t]
		sort.Slice(c, func(i, j int) bool { return c[i].Begin.Before(c[j].Begin) })
	}
	return src, nil
}

func (s *csvSource) History(_ context.Context, ticker string, from, till time.Time) ([]moex.Candle, error) {
	all := s.byTicker[strings.ToUpper(ticker)]
	var out []moex.Candle
	for _, c := range all {
		if c.Begin.Before(from) || c.Begin.After(till) {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}
