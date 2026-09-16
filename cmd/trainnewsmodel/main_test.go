package main

import (
	"os"
	"strings"
	"testing"
)

func TestParseOptionsRequiresNewsAndSplit(t *testing.T) {
	if _, err := parseOptions(nil); err == nil {
		t.Fatal("parseOptions() error = nil, want error for missing -news/-split")
	}
	if _, err := parseOptions([]string{"-news", "raw.jsonl"}); err == nil {
		t.Fatal("parseOptions() error = nil, want error for missing -split")
	}
}

func TestParseOptionsDefaults(t *testing.T) {
	opts, err := parseOptions([]string{"-news", "raw.jsonl", "-split", "2026-01-01"})
	if err != nil {
		t.Fatalf("parseOptions() error = %v", err)
	}
	if opts.configPath != defaultConfigPath {
		t.Errorf("configPath = %q, want %q", opts.configPath, defaultConfigPath)
	}
	if opts.horizonDays != 3 || opts.vocabSize != 3000 || opts.minDocFreq != 3 {
		t.Errorf("unexpected defaults: %+v", opts)
	}
	if opts.outPath != "news_classifier.json" {
		t.Errorf("outPath = %q, want news_classifier.json", opts.outPath)
	}
}

func TestParseOptionsRejectsExtraArgs(t *testing.T) {
	_, err := parseOptions([]string{"-news", "raw.jsonl", "-split", "2026-01-01", "extra"})
	if err == nil || !strings.Contains(err.Error(), "unexpected arguments") {
		t.Fatalf("parseOptions() error = %v, want unexpected-arguments error", err)
	}
}

func TestLoadNewsArticlesFiltersInvalidRows(t *testing.T) {
	path := writeTempFile(t, strings.Join([]string{
		`{"ticker":"SBER","article_id":"a1","title":"рост прибыли","published_ts":1700000000}`,
		`{"ticker":"SBER","article_id":"a2","title":"no title","published_ts":1700000100}`,
		`{"ticker":"","article_id":"a3","title":"без тикера","published_ts":1700000200}`,
		`{"ticker":"GAZP","article_id":"a4","title":"","published_ts":1700000300}`,
		`{"ticker":"GAZP","article_id":"a5","title":"убыток санкции","published_ts":0}`,
		`not json`,
		`{"ticker":"gazp","article_id":"a6","title":"  падение выручки  ","published_ts":1700000400}`,
	}, "\n"))

	articles, err := loadNewsArticles(path)
	if err != nil {
		t.Fatalf("loadNewsArticles() error = %v", err)
	}
	if len(articles) != 2 {
		t.Fatalf("loadNewsArticles() returned %d articles, want 2: %+v", len(articles), articles)
	}
	if articles[0].Ticker != "SBER" || articles[0].Title != "рост прибыли" {
		t.Errorf("articles[0] = %+v, unexpected", articles[0])
	}
	if articles[1].Ticker != "GAZP" || articles[1].Title != "падение выручки" {
		t.Errorf("articles[1] = %+v, unexpected", articles[1])
	}
}

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	path := t.TempDir() + "/news.jsonl"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	return path
}
