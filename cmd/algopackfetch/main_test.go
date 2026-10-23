package main

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCellFormatsValues(t *testing.T) {
	cases := map[string]interface{}{"": nil, "SBER": "SBER", "1.5": 1.5, "100": float64(100), `"a,b"`: "a,b"}
	for want, in := range cases {
		if got := cell(in); got != want {
			t.Fatalf("cell(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestWriteDayRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tradestats", "2024-10-15.csv.gz")
	cols := []string{"tradedate", "secid", "vol"}
	rows := [][]interface{}{{"2024-10-15", "SBER", float64(10)}, {"2024-10-15", "GAZP", nil}}
	if err := writeDay(path, cols, rows); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(zr)
	want := "tradedate,secid,vol\n2024-10-15,SBER,10\n2024-10-15,GAZP,\n"
	if string(data) != want {
		t.Fatalf("got %q, want %q", data, want)
	}
	if _, err := os.Stat(path + ".tmp"); err == nil {
		t.Fatal("tmp file left behind")
	}
}

func TestWeekdaysSkipsWeekends(t *testing.T) {
	from := time.Date(2024, 10, 11, 0, 0, 0, 0, time.UTC)
	till := time.Date(2024, 10, 15, 0, 0, 0, 0, time.UTC)
	got := strings.Join(weekdays(from, till), ",")
	if got != "2024-10-11,2024-10-14,2024-10-15" {
		t.Fatalf("got %s", got)
	}
}
