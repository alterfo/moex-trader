package model

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/features"
)

func writeNewsHistoryFixture(t *testing.T, lines []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "news_history.jsonl")
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestLoadFinanalysNewsHistory(t *testing.T) {
	path := writeNewsHistoryFixture(t, []string{
		`{"ticker":"sber","article_id":"a1","trust_weight":1.0,"published_ts":1723622400,"sentiment":0.5}`,
		`{"ticker":"SBER","article_id":"a2","trust_weight":0.5,"published_ts":1723622500,"sentiment":-0.2}`,
		`  `,
		`{"ticker":"GAZP","article_id":"a3","trust_weight":0,"published_ts":1723622600,"sentiment":0.3}`,
	})

	records, err := LoadFinanalysNewsHistory(path)
	if err != nil {
		t.Fatalf("LoadFinanalysNewsHistory() error = %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("got %d records, want 3", len(records))
	}
	if records[0].Ticker != "SBER" {
		t.Fatalf("Ticker = %q, want normalized SBER", records[0].Ticker)
	}
	if records[2].TrustWeight != 1 {
		t.Fatalf("TrustWeight = %v, want default 1 for zero input", records[2].TrustWeight)
	}
}

func TestLoadFinanalysNewsHistoryMissingFile(t *testing.T) {
	if _, err := LoadFinanalysNewsHistory(filepath.Join(t.TempDir(), "missing.jsonl")); err == nil {
		t.Fatal("LoadFinanalysNewsHistory() error = nil, want error for missing file")
	}
}

func TestLoadFinanalysNewsHistoryMalformedLine(t *testing.T) {
	path := writeNewsHistoryFixture(t, []string{`{not json`})
	if _, err := LoadFinanalysNewsHistory(path); err == nil {
		t.Fatal("LoadFinanalysNewsHistory() error = nil, want error for malformed line")
	}
}

func TestAggregateDailySentimentWeightedAverage(t *testing.T) {
	base := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	records := []HistoricalNewsRecord{
		{Ticker: "SBER", PublishedAt: base, Sentiment: 1.0, TrustWeight: 1.0},
		{Ticker: "SBER", PublishedAt: base.Add(2 * time.Hour), Sentiment: -1.0, TrustWeight: 3.0},
		{Ticker: "SBER", PublishedAt: base.AddDate(0, 0, 1), Sentiment: 0.5, TrustWeight: 1.0},
	}
	agg := AggregateDailySentiment(records)
	day1 := agg["SBER"][dateKey(base)]
	if day1.Count != 2 {
		t.Fatalf("day1 count = %d, want 2", day1.Count)
	}
	want := (1.0*1.0 + -1.0*3.0) / 4.0
	if diff := day1.Sentiment - want; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("day1 sentiment = %v, want %v", day1.Sentiment, want)
	}
	day2 := agg["SBER"][dateKey(base.AddDate(0, 0, 1))]
	if day2.Count != 1 || day2.Sentiment != 0.5 {
		t.Fatalf("day2 = %+v, want count=1 sentiment=0.5", day2)
	}
}

func TestAggregateDailyTopicSignalsWeightedAverage(t *testing.T) {
	base := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	records := []HistoricalNewsRecord{
		{Ticker: "SBER", PublishedAt: base, Sentiment: 0.8, TrustWeight: 0.5, Title: "переговоры продвинулись"},
		{Ticker: "SBER", PublishedAt: base.Add(2 * time.Hour), Sentiment: -0.4, TrustWeight: 0.5, Title: "санкции усилены"},
		{Ticker: "SBER", PublishedAt: base.Add(3 * time.Hour), Sentiment: 0.9, TrustWeight: 1.0, Title: "отчет компании"},
		{Ticker: "SBER", PublishedAt: base.Add(4 * time.Hour), Sentiment: 0.1, TrustWeight: 0, Title: "переговоры сорвались"},
		{Ticker: "SBER", PublishedAt: base.Add(5 * time.Hour), Sentiment: 0.2, TrustWeight: 1.0, Title: ""},
		{Ticker: "SBER", PublishedAt: base.AddDate(0, 0, 1), Sentiment: 0.6, TrustWeight: 0.25, Title: "уиткофф приехал"},
		{Ticker: "SBER", PublishedAt: base.AddDate(0, 0, 1).Add(time.Hour), Sentiment: 0.2, TrustWeight: 0.75, Title: "переговоры продолжаются"},
	}

	agg := AggregateDailyTopicSignals(records)
	day1 := agg["SBER"][dateKey(base)]
	if day1.Negotiations != 0.8 {
		t.Fatalf("day1 negotiations = %v, want 0.8", day1.Negotiations)
	}
	if day1.Sanctions != -0.4 {
		t.Fatalf("day1 sanctions = %v, want -0.4", day1.Sanctions)
	}
	day2 := agg["SBER"][dateKey(base.AddDate(0, 0, 1))]
	want := (0.6*0.25 + 0.2*0.75) / 1.0
	if diff := day2.Negotiations - want; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("day2 negotiations = %v, want %v", day2.Negotiations, want)
	}
	if day2.Sanctions != 0 {
		t.Fatalf("day2 sanctions = %v, want 0", day2.Sanctions)
	}
}

func TestAggregateDailyTopicSignalsNoMatches(t *testing.T) {
	base := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	records := []HistoricalNewsRecord{
		{Ticker: "SBER", PublishedAt: base, Sentiment: 0.5, TrustWeight: 1.0, Title: "отчет компании"},
		{Ticker: "SBER", PublishedAt: base.Add(time.Hour), Sentiment: 0.6, TrustWeight: 0, Title: "переговоры сорвались"},
	}
	agg := AggregateDailyTopicSignals(records)
	if len(agg) != 0 {
		t.Fatalf("AggregateDailyTopicSignals() = %+v, want empty map for no matching headlines", agg)
	}
}

func TestApplyNewsOverride(t *testing.T) {
	day := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	samples := []LabeledSample{
		{Feature: domain.FeatureContext{Ticker: "SBER", GeneratedAt: day, NewsSentiment: decimal.Zero, NewsCount: 0}},
		{Feature: domain.FeatureContext{Ticker: "SBER", GeneratedAt: day.AddDate(0, 0, 10), NewsSentiment: decimal.Zero}},
	}
	news := map[string]map[string]NewsAggregate{
		"SBER": {dateKey(day): {Sentiment: 0.42, Count: 5}},
	}

	applied := ApplyNewsOverride(samples, news)
	if applied != 1 {
		t.Fatalf("applied = %d, want 1", applied)
	}
	if !samples[0].Feature.NewsSentiment.Equal(decimal.NewFromFloat(0.42)) || samples[0].Feature.NewsCount != 5 {
		t.Fatalf("sample 0 not overridden: %+v", samples[0].Feature)
	}
	if !samples[1].Feature.NewsSentiment.IsZero() || samples[1].Feature.NewsCount != 0 {
		t.Fatalf("sample 1 should remain unchanged (no news match): %+v", samples[1].Feature)
	}
}

func TestApplyNewsOverrideToCalibration(t *testing.T) {
	day := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	names := []string{"return_pct", "news_sentiment", "news_count"}
	samples := []CalibrationSample{
		{Ticker: "SBER", Date: day, Vector: []float64{1, 0, 0}, Names: names},
		{Ticker: "SBER", Date: day.AddDate(0, 0, 10), Vector: []float64{1, 0, 0}, Names: names},
	}
	news := map[string]map[string]NewsAggregate{
		"SBER": {dateKey(day): {Sentiment: 0.7, Count: 3}},
	}

	applied := ApplyNewsOverrideToCalibration(samples, news)
	if applied != 1 {
		t.Fatalf("applied = %d, want 1", applied)
	}
	if samples[0].Vector[1] != 0.7 || samples[0].Vector[2] != 3 {
		t.Fatalf("sample 0 vector not overridden: %v", samples[0].Vector)
	}
	if samples[1].Vector[1] != 0 || samples[1].Vector[2] != 0 {
		t.Fatalf("sample 1 should remain unchanged: %v", samples[1].Vector)
	}
}

func TestApplyNewsOverrideToCalibrationNoFeatureColumns(t *testing.T) {
	samples := []CalibrationSample{
		{Ticker: "SBER", Date: time.Now(), Vector: []float64{1}, Names: []string{"return_pct"}},
	}
	if applied := ApplyNewsOverrideToCalibration(samples, map[string]map[string]NewsAggregate{"SBER": {}}); applied != 0 {
		t.Fatalf("applied = %d, want 0 when news columns are absent", applied)
	}
}

func TestApplyTopicSignalOverrides(t *testing.T) {
	day := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	samples := []LabeledSample{
		{Feature: domain.FeatureContext{Ticker: "SBER", GeneratedAt: day}},
		{Feature: domain.FeatureContext{Ticker: "SBER", GeneratedAt: day.AddDate(0, 0, 10)}},
	}
	topics := map[string]map[string]features.TopicSignalAggregate{
		"SBER": {dateKey(day): {Negotiations: 0.6, Sanctions: -0.4}},
	}

	applied := ApplyTopicSignalOverrides(samples, topics)
	if applied != 1 {
		t.Fatalf("applied = %d, want 1", applied)
	}
	if !samples[0].Feature.NegotiationsSignal.Equal(decimal.NewFromFloat(0.6)) || !samples[0].Feature.SanctionsSignal.Equal(decimal.NewFromFloat(-0.4)) {
		t.Fatalf("sample 0 topic signals not overridden: %+v", samples[0].Feature)
	}
	if !samples[1].Feature.NegotiationsSignal.IsZero() || !samples[1].Feature.SanctionsSignal.IsZero() {
		t.Fatalf("sample 1 should remain unchanged: %+v", samples[1].Feature)
	}
}

func TestApplyTopicSignalOverridesToCalibration(t *testing.T) {
	day := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	names := []string{"return_pct", "negotiations_signal", "sanctions_signal"}
	samples := []CalibrationSample{
		{Ticker: "SBER", Date: day, Vector: []float64{1, 0, 0}, Names: names},
		{Ticker: "SBER", Date: day.AddDate(0, 0, 10), Vector: []float64{1, 0, 0}, Names: names},
	}
	topics := map[string]map[string]features.TopicSignalAggregate{
		"SBER": {dateKey(day): {Negotiations: 0.7, Sanctions: -0.3}},
	}

	applied := ApplyTopicSignalOverridesToCalibration(samples, topics)
	if applied != 1 {
		t.Fatalf("applied = %d, want 1", applied)
	}
	if samples[0].Vector[1] != 0.7 || samples[0].Vector[2] != -0.3 {
		t.Fatalf("sample 0 vector not overridden: %v", samples[0].Vector)
	}
	if samples[1].Vector[1] != 0 || samples[1].Vector[2] != 0 {
		t.Fatalf("sample 1 should remain unchanged: %v", samples[1].Vector)
	}
}
