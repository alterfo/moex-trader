package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
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
	header := datasetHeader()

	const warmup = 10
	from := start
	till := candles[len(candles)-1].Begin
	labels := map[string]labeledRow{
		"TEST|" + barKey(candles[warmup].Begin):   {label: 1, date: candles[warmup].Begin},
		"TEST|" + barKey(candles[warmup+1].Begin): {label: 0, date: candles[warmup+1].Begin},
	}

	err := writeTicker(context.Background(), writer, source, features.NewBuilder(time.Now),
		"TEST", start, till, from, from, defaultHorizonDays, warmup, labels, nil, nil, nil, header)
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
	header := datasetHeader()

	news := model.AggregateDailySentiment([]model.HistoricalNewsRecord{
		{Ticker: "TEST", PublishedAt: from, Sentiment: 0.75, TrustWeight: 1},
	})
	events := model.AggregateDailyEvents([]model.HistoricalNewsRecord{
		{Ticker: "TEST", PublishedAt: from, TrustWeight: 1, Title: "Дивиденды и выкуп акций утверждены"},
	})
	labels := map[string]labeledRow{}

	err := writeTicker(context.Background(), writer, source, features.NewBuilder(time.Now),
		"TEST", start, till, from, split, defaultHorizonDays, minLabelCandles, labels, news, nil, events, header)
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

func TestWriteTickerWritesTopicSignalColumns(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	source := &fakeSource{candles: map[string][]moex.Candle{
		"TEST": makeCandles(start, minLabelCandles+2, 1),
	}}
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	from := start.AddDate(0, 0, minLabelCandles)
	till := start.AddDate(0, 0, minLabelCandles+1)
	header := datasetHeader()
	topics := model.AggregateDailyTopicSignals([]model.HistoricalNewsRecord{
		{Ticker: "TEST", PublishedAt: from, TrustWeight: 1, Title: "Мирный план по переговорам согласован", Sentiment: 0.625},
		{Ticker: "TEST", PublishedAt: from, TrustWeight: 1, Title: "Новые санкции ограничили торговлю", Sentiment: -0.375},
	})
	if err := writer.Write(header); err != nil {
		t.Fatalf("write header: %v", err)
	}

	err := writeTicker(context.Background(), writer, source, features.NewBuilder(time.Now),
		"TEST", start, till, from, from, defaultHorizonDays, minLabelCandles, map[string]labeledRow{}, nil, topics, nil, header)
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
	if len(rows) != 3 {
		t.Fatalf("got %d CSV rows, want header + 2 decision rows", len(rows))
	}
	if rows[0][31] != "negotiations_signal" || rows[0][32] != "sanctions_signal" {
		t.Fatalf("topic signal header columns = %q/%q, want negotiations_signal/sanctions_signal", rows[0][31], rows[0][32])
	}
	if got := rows[1][31]; got != "0.625" {
		t.Fatalf("negotiations_signal = %q, want 0.625", got)
	}
	if got := rows[1][32]; got != "-0.375" {
		t.Fatalf("sanctions_signal = %q, want -0.375", got)
	}
	if got := rows[2][31]; got != "0" {
		t.Fatalf("negotiations_signal for non-matching day = %q, want 0", got)
	}
	if got := rows[2][32]; got != "0" {
		t.Fatalf("sanctions_signal for non-matching day = %q, want 0", got)
	}
}

func TestDatasetHeaderMirrorsFeatureOrder(t *testing.T) {
	header := datasetHeader()
	if len(header) < 7 {
		t.Fatalf("header too short: %v", header)
	}
	wantPrefix := []string{"ticker", "date", "label", "label_date", "split"}
	for i, want := range wantPrefix {
		if header[i] != want {
			t.Fatalf("header[%d] = %q, want %q", i, header[i], want)
		}
	}
	_, names := model.ToVector(domain.FeatureContext{})
	if len(header)-5 != len(names) {
		t.Fatalf("header has %d feature columns, want %d", len(header)-5, len(names))
	}
	for i, name := range names {
		if header[5+i] != name {
			t.Fatalf("feature column %d = %q, want %q", i, header[5+i], name)
		}
	}
	if names[len(names)-2] != "negotiations_signal" || names[len(names)-1] != "sanctions_signal" {
		t.Fatalf("new signal names missing from feature order tail: %v", names[len(names)-2:])
	}
}
