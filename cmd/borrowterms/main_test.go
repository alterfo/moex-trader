package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppendAndReadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "terms.jsonl")
	first := []termRecord{
		{Date: "2026-10-29", Ticker: "SBER", ShortEnabled: true, Dshort: "0.2", DshortMin: "0.1428"},
		{Date: "2026-10-29", Ticker: "GAZP", ShortEnabled: true, Dshort: "0.2767", DshortMin: "0.1818"},
	}
	if err := appendRecords(path, first); err != nil {
		t.Fatalf("appendRecords: %v", err)
	}
	second := []termRecord{{Date: "2026-10-30", Ticker: "SBER", ShortEnabled: false, Dshort: "0", DshortMin: "0"}}
	if err := appendRecords(path, second); err != nil {
		t.Fatalf("appendRecords second: %v", err)
	}
	got, err := readRecords(path)
	if err != nil {
		t.Fatalf("readRecords: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("records = %d, want 3", len(got))
	}
	if got[0].Ticker != "SBER" || got[0].Dshort != "0.2" || !got[0].ShortEnabled {
		t.Fatalf("first record = %+v", got[0])
	}
	if got[2].ShortEnabled {
		t.Fatalf("third record = %+v", got[2])
	}
}

func TestReadRecordsOnMissingFile(t *testing.T) {
	got, err := readRecords(filepath.Join(t.TempDir(), "absent.jsonl"))
	if err != nil {
		t.Fatalf("missing file must not fail: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("records = %d", len(got))
	}
}

func TestHadAllTickersToday(t *testing.T) {
	existing := []termRecord{
		{Date: "2026-10-29", Ticker: "SBER"},
		{Date: "2026-10-29", Ticker: "GAZP"},
		{Date: "2026-10-28", Ticker: "LKOH"},
	}
	if !hadAllTickersToday(existing, "2026-10-29", []string{"sber", "GAZP"}) {
		t.Fatal("both tickers are recorded for the date")
	}
	if hadAllTickersToday(existing, "2026-10-29", []string{"SBER", "LKOH"}) {
		t.Fatal("LKOH is only recorded for the previous day")
	}
	if err := os.WriteFile(filepath.Join(t.TempDir(), "x"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestQuotationString(t *testing.T) {
	if got := quotationString(nil); got != "0" {
		t.Fatalf("nil quotation = %q", got)
	}
}
