package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/features"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
	"github.com/olegsidorkin/moex-trader/internal/model"
)

type fakeSource struct {
	candles map[string][]moex.Candle
}

func (f *fakeSource) History(_ context.Context, ticker string, _, _ time.Time) ([]moex.Candle, error) {
	return f.candles[ticker], nil
}

func makeCandles(start time.Time, count int, step int) []moex.Candle {
	candles := make([]moex.Candle, 0, count)
	price := 100
	for i := 0; i < count; i++ {
		open := decimal.NewFromInt(int64(price))
		price += step
		closePx := decimal.NewFromInt(int64(price))
		day := start.AddDate(0, 0, i)
		candles = append(candles, moex.Candle{
			Open:   open,
			High:   closePx.Add(decimal.NewFromInt(10)),
			Low:    open,
			Close:  closePx,
			Volume: decimal.NewFromInt(1000),
			Begin:  day,
			End:    day,
		})
	}
	return candles
}

func makeIntradayCandles(start time.Time, count, stepMinutes int) []moex.Candle {
	candles := make([]moex.Candle, 0, count)
	price := 100
	for i := 0; i < count; i++ {
		open := decimal.NewFromInt(int64(price))
		price += 1
		closePx := decimal.NewFromInt(int64(price))
		begin := start.Add(time.Duration(i*stepMinutes) * time.Minute)
		candles = append(candles, moex.Candle{
			Open:   open,
			High:   closePx.Add(decimal.NewFromInt(1)),
			Low:    open,
			Close:  closePx,
			Volume: decimal.NewFromInt(1000),
			Begin:  begin,
			End:    begin,
		})
	}
	return candles
}

// Regression: intraday datasets have many decision bars per calendar date, so
// a date-only label key collapsed them and every bar of a day inherited one
// label. Two bars on the same date must keep their distinct labels.
func TestWriteTickerKeysLabelsPerBarWithinADay(t *testing.T) {
	start := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	candles := makeIntradayCandles(start, 14, 10)
	source := &fakeSource{candles: map[string][]moex.Candle{"TEST": candles}}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	header := []string{"ticker", "date", "label", "label_date", "split", "return_pct", "realized_volatility",
		"news_sentiment", "news_count", "order_book_imbalance", "mom_5d", "mom_21d", "mom_63d",
		"reversal_1d", "rsi_14", "dist_ma20_pct", "dist_ma50_pct", "realized_vol_21d_annualized_pct", "volume_zscore_20d",
		"macd_hist_pct", "stoch_k_14", "williams_r_14", "alligator_spread_pct",
		"event_dividend", "event_buyback", "event_sanctions", "event_ipo", "event_report", "event_delisting", "event_mna", "event_default"}

	const warmup = 10
	from := start
	till := candles[len(candles)-1].Begin
	labels := map[string]labeledRow{
		"TEST|" + barKey(candles[warmup].Begin):   {label: 1, date: candles[warmup].Begin},
		"TEST|" + barKey(candles[warmup+1].Begin): {label: 0, date: candles[warmup+1].Begin},
	}

	err := writeTicker(context.Background(), writer, source, features.NewBuilder(time.Now),
		"TEST", start, till, from, from, defaultHorizonDays, warmup, labels, nil, nil, header)
	if err != nil {
		t.Fatalf("writeTicker: %v", err)
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		t.Fatalf("writer: %v", err)
	}

	rows, err := csv.NewReader(strings.NewReader(buf.String())).ReadAll()
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if len(rows) < 2 {
		t.Fatalf("expected at least 2 labeled rows, got %d rows", len(rows))
	}
	if rows[0][2] != "1" || rows[1][2] != "0" {
		t.Fatalf("same-date bars got labels %q and %q, want 1 and 0 - labels collapsed to a date key", rows[0][2], rows[1][2])
	}
}

func TestWriteTickerRowMatchesHeader(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	source := &fakeSource{candles: map[string][]moex.Candle{
		"TEST": makeCandles(start, minLabelCandles+5, 1),
	}}
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	from := start.AddDate(0, 0, minLabelCandles)
	till := start.AddDate(0, 0, minLabelCandles+4)
	split := start.AddDate(0, 0, minLabelCandles+2)
	header := []string{"ticker", "date", "label", "label_date", "split", "return_pct", "realized_volatility",
		"news_sentiment", "news_count", "order_book_imbalance", "mom_5d", "mom_21d", "mom_63d",
		"reversal_1d", "rsi_14", "dist_ma20_pct", "dist_ma50_pct", "realized_vol_21d_annualized_pct", "volume_zscore_20d",
		"macd_hist_pct", "stoch_k_14", "williams_r_14", "alligator_spread_pct",
		"event_dividend", "event_buyback", "event_sanctions", "event_ipo", "event_report", "event_delisting", "event_mna", "event_default"}

	news := model.AggregateDailySentiment([]model.HistoricalNewsRecord{
		{Ticker: "TEST", PublishedAt: from, Sentiment: 0.75, TrustWeight: 1},
	})
	events := model.AggregateDailyEvents([]model.HistoricalNewsRecord{
		{Ticker: "TEST", PublishedAt: from, TrustWeight: 1, Title: "Дивиденды и выкуп акций утверждены"},
	})
	labels := map[string]labeledRow{}

	err := writeTicker(context.Background(), writer, source, features.NewBuilder(time.Now),
		"TEST", start, till, from, split, defaultHorizonDays, minLabelCandles, labels, news, events, header)
	if err != nil {
		t.Fatalf("writeTicker: %v", err)
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		t.Fatalf("writer: %v", err)
	}

	rows, err := csv.NewReader(strings.NewReader(buf.String())).ReadAll()
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("no rows written")
	}
	for i, row := range rows {
		if len(row) != len(header) {
			t.Fatalf("row %d has %d fields, header has %d", i, len(row), len(header))
		}
	}
	rowsWide := rows[1:]
	gotNews := false
	for _, row := range rowsWide {
		if row[7] != "0" || row[8] != "0" {
			gotNews = true
		}
	}
	if !gotNews {
		t.Log("note: no news row found in this window; columns still sized correctly")
	}
}
