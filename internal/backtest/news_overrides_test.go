package backtest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/features"
)

func TestLoadNewsOverridesTopicSignals(t *testing.T) {
	path := filepath.Join(t.TempDir(), "news_history.jsonl")
	lines := []string{
		`{"ticker":"sber","article_id":"a1","trust_weight":0.5,"published_ts":1786919666,"sentiment":0.8,"title":"прогресс в переговорах"}`,
		`{"ticker":"SBER","article_id":"a2","trust_weight":1.5,"published_ts":1786919667,"sentiment":0.4,"title":"уиткофф обсудил мирный план"}`,
		`{"ticker":"SBER","article_id":"a3","trust_weight":1.0,"published_ts":1786919668,"sentiment":0.3,"title":"санкции смягчены"}`,
		`{"ticker":"SBER","article_id":"a4","trust_weight":1.0,"published_ts":1787006066,"sentiment":1.0,"title":"квартальный отчет"}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	overrides, err := LoadNewsOverrides(path)
	if err != nil {
		t.Fatalf("LoadNewsOverrides() error = %v", err)
	}
	news, events, topics := overrides.News, overrides.Events, overrides.Topics

	day1 := "2026-08-16"
	day2 := "2026-08-17"
	if got := news["SBER"][day1].Count; got != 3 {
		t.Fatalf("day1 news count = %d, want 3", got)
	}
	wantSentiment := (0.8*0.5 + 0.4*1.5 + 0.3*1.0) / (0.5 + 1.5 + 1.0)
	if got := news["SBER"][day1].Sentiment; got > wantSentiment+1e-9 || got < wantSentiment-1e-9 {
		t.Fatalf("day1 news sentiment = %v, want %v", got, wantSentiment)
	}
	if got := events["SBER"][day1].Sanctions; got != 1 {
		t.Fatalf("day1 event sanctions = %d, want 1", got)
	}

	topic1 := topics["SBER"][day1]
	if diff := topic1.Negotiations - 0.5; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("day1 negotiations = %v, want 0.5", topic1.Negotiations)
	}
	if diff := topic1.Sanctions - 0.3; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("day1 sanctions = %v, want 0.3", topic1.Sanctions)
	}
	if _, ok := topics["SBER"][day2]; ok {
		t.Fatal("expected no topic aggregate for the unrelated-headline day")
	}
}

func TestLoadNewsOverridesTopicSignalsNoMatches(t *testing.T) {
	path := filepath.Join(t.TempDir(), "news_history.jsonl")
	lines := []string{
		`{"ticker":"sber","article_id":"a1","trust_weight":1.0,"published_ts":1786919666,"sentiment":0.9,"title":"квартальный отчет"}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	overrides, err := LoadNewsOverrides(path)
	if err != nil {
		t.Fatalf("LoadNewsOverrides() error = %v", err)
	}
	topics := overrides.Topics
	if len(topics) != 0 {
		t.Fatalf("expected empty topic map, got %+v", topics)
	}
}

type topicCaptureSignal struct {
	features []domain.FeatureContext
}

func (s *topicCaptureSignal) Generate(_ context.Context, feature domain.FeatureContext) (domain.TradeSignal, error) {
	s.features = append(s.features, feature)
	return domain.TradeSignal{Ticker: feature.Ticker, Action: domain.ActionHold, GeneratedAt: feature.GeneratedAt}, nil
}

func TestProcessTickerDayAppliesTopicSignalOverrides(t *testing.T) {
	candles := benchCandles(120)
	from := candles[105].Begin
	day := from.Format("2006-01-02")
	signal := &topicCaptureSignal{}

	engine, err := NewEngine(Config{
		Tickers:        []string{"TEST"},
		From:           from,
		Till:           candles[106].Begin,
		Deposit:        decimal.NewFromInt(100000),
		MaxLots:        1,
		CommissionRate: decimal.Zero,
		SignalSource:   signal,
		Source:         fakeSource{candles: candles},
		TopicSignalOverrides: map[string]map[string]features.TopicSignalAggregate{
			"TEST": {day: {Negotiations: 0.75, Sanctions: -0.6}},
		},
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	if _, err := engine.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, feature := range signal.features {
		if feature.GeneratedAt.Format("2006-01-02") != day {
			continue
		}
		if !feature.NegotiationsSignal.Equal(decimal.NewFromFloat(0.75)) {
			t.Fatalf("NegotiationsSignal = %s, want 0.75", feature.NegotiationsSignal)
		}
		if !feature.SanctionsSignal.Equal(decimal.NewFromFloat(-0.6)) {
			t.Fatalf("SanctionsSignal = %s, want -0.6", feature.SanctionsSignal)
		}
		return
	}
	t.Fatalf("no feature captured for override day %s", day)
}
