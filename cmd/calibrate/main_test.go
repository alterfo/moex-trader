package main

import (
	"testing"
	"time"
)

func TestParseOptionsDefaults(t *testing.T) {
	opts, err := parseOptions(nil)
	if err != nil {
		t.Fatalf("parseOptions() error = %v", err)
	}
	if opts.configPath != defaultConfigPath {
		t.Fatalf("config = %q, want %q", opts.configPath, defaultConfigPath)
	}
	if opts.horizonsStr != defaultHorizonsStr {
		t.Fatalf("horizonsStr = %q, want %q", opts.horizonsStr, defaultHorizonsStr)
	}
	if opts.stride != defaultStride {
		t.Fatalf("stride = %d, want %d", opts.stride, defaultStride)
	}
}

func TestParseOptionsOverrides(t *testing.T) {
	opts, err := parseOptions([]string{
		"-config", "other.yaml",
		"-tickers", "SBER,OZON",
		"-from", "2024-01-01",
		"-till", "2025-01-01",
		"-horizons", "2,4",
		"-stride", "5",
		"-out", "/tmp/report.md",
	})
	if err != nil {
		t.Fatalf("parseOptions() error = %v", err)
	}
	if opts.tickersFlag != "SBER,OZON" || opts.horizonsStr != "2,4" || opts.stride != 5 || opts.outPath != "/tmp/report.md" {
		t.Fatalf("unexpected options: %+v", opts)
	}
}

func TestParseOptionsRejectsPositionalArgs(t *testing.T) {
	if _, err := parseOptions([]string{"unexpected"}); err == nil {
		t.Fatal("parseOptions() error = nil, want unexpected argument error")
	}
}

func TestParseHorizons(t *testing.T) {
	horizons, err := parseHorizons("1, 3,5")
	if err != nil {
		t.Fatalf("parseHorizons() error = %v", err)
	}
	want := []int{1, 3, 5}
	if len(horizons) != len(want) {
		t.Fatalf("horizons = %v, want %v", horizons, want)
	}
	for i := range want {
		if horizons[i] != want[i] {
			t.Fatalf("horizons = %v, want %v", horizons, want)
		}
	}
}

func TestParseHorizonsErrors(t *testing.T) {
	if _, err := parseHorizons(""); err == nil {
		t.Fatal("parseHorizons(\"\") error = nil, want empty-horizons error")
	}
	if _, err := parseHorizons("1,abc"); err == nil {
		t.Fatal("parseHorizons(\"1,abc\") error = nil, want parse error")
	}
}

func TestResolveWindowDefaults(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	from, till, err := resolveWindow(options{}, now)
	if err != nil {
		t.Fatalf("resolveWindow() error = %v", err)
	}
	if !from.Equal(now.AddDate(-2, 0, 0)) {
		t.Fatalf("from = %v, want %v", from, now.AddDate(-2, 0, 0))
	}
	if !till.Equal(now) {
		t.Fatalf("till = %v, want %v", till, now)
	}
}

func TestResolveWindowRejectsInvertedRange(t *testing.T) {
	_, _, err := resolveWindow(options{fromStr: "2025-01-01", tillStr: "2024-01-01"}, time.Now())
	if err == nil {
		t.Fatal("resolveWindow() error = nil, want from-after-till error")
	}
}
