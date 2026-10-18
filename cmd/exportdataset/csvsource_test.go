package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCSVSourceReturnsSortedWindow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.csv")
	data := "ticker,date,open,high,low,close,volume\nSBER,2024-01-03,101,103,100,102,1000\nSBER,2024-01-02,100,102,99,101,900\nSBER,2024-01-05,102,104,101,103,800\nGAZP,2024-01-02,10,11,9,10,50\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	src, err := newCSVSource(path)
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	till := time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC)
	got, err := src.History(context.Background(), "sber", from, till)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !got[0].Begin.Before(got[1].Begin) || got[0].Close.String() != "101" || got[1].Close.String() != "102" {
		t.Fatalf("unexpected candles %+v", got)
	}
	if none, _ := src.History(context.Background(), "NOPE", from, till); len(none) != 0 {
		t.Fatalf("unknown ticker returned %d candles", len(none))
	}
}

func TestCSVSourceRejectsMissingColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.csv")
	if err := os.WriteFile(path, []byte("ticker,date,open\nSBER,2024-01-02,1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := newCSVSource(path); err == nil {
		t.Fatal("expected an error for a missing column")
	}
}
