package main

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

func cell(kind, borrow string, p float64) cellReport {
	return cellReport{Window: "W2", Placebo: kind, Borrow: borrow, Real: 1000, P: p}
}

func TestVerdictSkill(t *testing.T) {
	got := verdict([]cellReport{
		cell("shift", "0.00005", 0.01), cell("permute", "0.00005", 0.01),
		cell("shift", "0.00047", 0.04), cell("permute", "0.00047", 0.02),
	})
	if len(got) != 1 || got[0][:5] != "skill" || got[0][:11] == "skill limit" {
		t.Fatalf("got %v", got)
	}
}

func TestVerdictLimitedByRate(t *testing.T) {
	got := verdict([]cellReport{
		cell("shift", "0.00005", 0.01), cell("permute", "0.00005", 0.02),
		cell("shift", "0.00047", 0.30), cell("permute", "0.00047", 0.02),
	})
	if len(got) != 1 || got[0][:11] != "skill limit" {
		t.Fatalf("got %v", got)
	}
}

func TestVerdictIndistinguishable(t *testing.T) {
	got := verdict([]cellReport{
		cell("shift", "0.00005", 0.40), cell("permute", "0.00005", 0.20),
		cell("shift", "0.00047", 0.60), cell("permute", "0.00047", 0.90),
	})
	if len(got) != 1 || got[0][:8] != "skill is" {
		t.Fatalf("got %v", got)
	}
}

func TestSummarize(t *testing.T) {
	c := summarize("W2", "shift", "0.00005", outcome{Realized: 10, Closed: 3}, []float64{1, 2, 3, 20}, 5, 1.5)
	if c.N != 4 || c.Mean != 6.5 || c.P != 0.4 || c.RealPercent != 0.75 {
		t.Fatalf("got %+v", c)
	}
	if c.Q50 != 2.5 {
		t.Fatalf("median %v", c.Q50)
	}
}

func TestMemSourceFiltersByDate(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	var candles []moex.Candle
	for d := 0; d < 10; d++ {
		candles = append(candles, moex.Candle{Begin: start.AddDate(0, 0, d), Close: decimal.NewFromInt(1)})
	}
	source := &memSource{candles: map[string][]moex.Candle{"A": candles}}
	got, err := source.History(context.Background(), "A", start.AddDate(0, 0, 2), start.AddDate(0, 0, 5))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 || !got[0].Begin.Equal(start.AddDate(0, 0, 2)) {
		t.Fatalf("got %d candles", len(got))
	}
}

func TestParseWindows(t *testing.T) {
	windows, err := parseWindows("2025-03-05", "2026-10-18", "2026-07-20", "2026-10-18")
	if err != nil || len(windows) != 2 || windows[0].Name != "W2" || windows[1].Name != "W1" {
		t.Fatalf("got %v %v", windows, err)
	}
	if _, err := parseWindows("bad", "2026-10-18", "2026-07-20", "2026-10-18"); err == nil {
		t.Fatal("expected error")
	}
}

func TestVerdictLosingBook(t *testing.T) {
	c := cell("shift", "0.00005", 0.40)
	c.Real = -2424
	got := verdict([]cellReport{c})
	if len(got) != 1 || got[0][:13] != "no skill to t" {
		t.Fatalf("got %v", got)
	}
}

func TestVerdictNoCriterionWindow(t *testing.T) {
	got := verdict([]cellReport{{Window: "W1", Placebo: "shift", Borrow: "0.00005", Real: 10, P: 0.3}})
	if len(got) != 1 || got[0][:7] != "no W2 c" {
		t.Fatalf("got %v", got)
	}
}

func TestSelectWindows(t *testing.T) {
	all := []window{{Name: "W2"}, {Name: "W1"}}
	if got, err := selectWindows(all, "w1"); err != nil || len(got) != 1 || got[0].Name != "W1" {
		t.Fatalf("got %v %v", got, err)
	}
	if got, err := selectWindows(all, ""); err != nil || len(got) != 2 {
		t.Fatalf("got %v %v", got, err)
	}
	if _, err := selectWindows(all, "x"); err == nil {
		t.Fatal("expected error")
	}
}
