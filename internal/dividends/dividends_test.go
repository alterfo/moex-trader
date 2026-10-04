package dividends

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadParsesRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dividends.jsonl")
	content := strings.Join([]string{
		`{"ticker":"SBER","figi":"F1","last_buy_date":"2026-07-17T00:00:00Z","dividend_net":"37.64"}`,
		`{"ticker":"LKOH","figi":"F2","last_buy_date":"2026-04-30T00:00:00Z","dividend_net":"278"}`,
		``,
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	records, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("Load() returned %d records, want 2", len(records))
	}
	if records[0].Ticker != "SBER" {
		t.Fatalf("records[0].Ticker = %q, want SBER", records[0].Ticker)
	}
	net, err := records[0].Net()
	if err != nil {
		t.Fatalf("Net() error = %v", err)
	}
	if net.String() != "37.64" {
		t.Fatalf("records[0] net = %s, want 37.64", net.String())
	}
}

func TestLoadMissingFileErrors(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing.jsonl")); err == nil {
		t.Fatal("Load() error = nil, want missing-file error")
	}
}

func TestLoadInvalidRecordErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dividends.jsonl")
	if err := os.WriteFile(path, []byte(`{"ticker":"SBER"`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load() error = nil, want invalid-JSON error")
	}
}

func TestLoadRejectsEmptyTicker(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dividends.jsonl")
	if err := os.WriteFile(path, []byte(`{"ticker":"","dividend_net":"1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load() error = nil, want empty-ticker error")
	}
}

func TestLoadRejectsInvalidNet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dividends.jsonl")
	if err := os.WriteFile(path, []byte(`{"ticker":"SBER","dividend_net":"not-a-number"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load() error = nil, want invalid-dividend-net error")
	}
}
