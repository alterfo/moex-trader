package main

import (
	"path/filepath"
	"testing"
)

func TestParseMapsColumnsByName(t *testing.T) {
	cols := []string{"TRADENO", "TRADETIME", "SECID", "PRICE", "QUANTITY", "PERIOD", "VALUE", "BOARDID", "SYSTIME", "BUYSELL", "DECIMALS", "TRADINGSESSION", "TRADEDATE"}
	data := [][]interface{}{{float64(7), "10:00:01", "SBER", 287.5, float64(10), "N", 2875.0, "TQBR", "x", "B", float64(2), "1", "2026-10-19"}}
	rows := parse(cols, data)
	if len(rows) != 1 || rows[0].no != 7 || rows[0].buysell != "B" || rows[0].period != "N" || rows[0].price != 287.5 || rows[0].date != "2026-10-19" {
		t.Fatalf("unexpected parse: %+v", rows)
	}
}

func TestWriteKeepsLargerFileAndSplitsByDate(t *testing.T) {
	dir := t.TempDir()
	a := []trade{{no: 1, date: "2026-10-19", tm: "10:00:00", price: 1, qty: 1, value: 1}, {no: 2, date: "2026-10-19", tm: "10:00:01", price: 1, qty: 1, value: 1}, {no: 3, date: "2026-10-20", tm: "10:00:00", price: 1, qty: 1, value: 1}}
	if err := write(dir, "SBER", a); err != nil {
		t.Fatal(err)
	}
	p19 := filepath.Join(dir, "2026-10-19", "SBER.csv.gz")
	if lineCount(p19) != 2 || lineCount(filepath.Join(dir, "2026-10-20", "SBER.csv.gz")) != 1 {
		t.Fatalf("counts %d", lineCount(p19))
	}
	if err := write(dir, "SBER", a[:1]); err != nil {
		t.Fatal(err)
	}
	if lineCount(p19) != 2 {
		t.Fatalf("smaller rerun overwrote the fuller file: %d", lineCount(p19))
	}
}
