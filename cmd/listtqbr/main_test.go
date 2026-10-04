package main

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

func TestShareTickersFiltersAndSorts(t *testing.T) {
	in := []moex.ListedSecurity{
		{SecID: "GAZP", SecType: "1"},
		{SecID: "SBMM", SecType: "J"},
		{SecID: "SBERP", SecType: "2"},
		{SecID: "SBER", SecType: "1"},
		{SecID: "SBER", SecType: "1"},
	}
	got := shareTickers(in)
	want := []string{"GAZP", "SBER", "SBERP"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestWriteListFileRoundTrip(t *testing.T) {
	path := t.TempDir() + "/train_tickers.json"
	list := trainTickerList{
		FetchedAt: time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC),
		Source:    "https://iss.moex.com/iss/engines/stock/markets/shares/boards/TQBR/securities.json",
		Engine:    "stock",
		Market:    "shares",
		Board:     "TQBR",
		SecTypes:  []string{"1", "2"},
		Tickers:   []string{"GAZP", "SBER"},
	}
	if err := writeListFile(path, list); err != nil {
		t.Fatalf("writeListFile: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var back trainTickerList
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("parse written list: %v", err)
	}
	if len(back.Tickers) != 2 || back.Tickers[0] != "GAZP" {
		t.Fatalf("round trip tickers = %v", back.Tickers)
	}
	if back.FetchedAt.IsZero() {
		t.Fatal("fetched_at not persisted")
	}
}
