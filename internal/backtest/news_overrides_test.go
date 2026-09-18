package backtest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

	news, events, topics, err := LoadNewsOverrides(path)
	if err != nil {
		t.Fatalf("LoadNewsOverrides() error = %v", err)
	}

	day1 := "2026-08-16"
	day2 := "2026-08-17"
	if got := news["SBER"][day1].Count; got != 3 {
		t.Fatalf("day1 news count = %d, want 3", got)
	}
	if got := news["SBER"][day1].Sentiment; got > 0.5+1e-9 || got < 0.5-1e-9 {
		t.Fatalf("day1 news sentiment = %v, want 0.5", got)
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

	_, _, topics, err := LoadNewsOverrides(path)
	if err != nil {
		t.Fatalf("LoadNewsOverrides() error = %v", err)
	}
	if len(topics) != 0 {
		t.Fatalf("expected empty topic map, got %+v", topics)
	}
}
