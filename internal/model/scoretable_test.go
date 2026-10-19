package model

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

func writeTable(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "scores.csv")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestScoreTableSourceMapsProbabilityToAction(t *testing.T) {
	path := writeTable(t, "ticker,date,p\nSBER,2026-10-14,0.65\nGAZP,2026-10-14,0.35\nLKOH,2026-10-14,0.5\n")
	src, err := LoadScoreTable(path, 100, decimal.NewFromInt(15000))
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 14, 11, 0, 0, 0, time.UTC)
	feature := func(ticker string) domain.FeatureContext {
		return domain.FeatureContext{Ticker: ticker, GeneratedAt: at, LastPrice: decimal.NewFromInt(300), LotSize: decimal.NewFromInt(10)}
	}
	cases := map[string]domain.Action{"SBER": domain.ActionBuy, "GAZP": domain.ActionSell, "LKOH": domain.ActionHold}
	for ticker, want := range cases {
		got, err := src.Generate(context.Background(), feature(ticker))
		if err != nil {
			t.Fatal(err)
		}
		if got.Action != want {
			t.Fatalf("%s: action %v, want %v", ticker, got.Action, want)
		}
	}
	buy, _ := src.Generate(context.Background(), feature("SBER"))
	if buy.TargetLots != 5 {
		t.Fatalf("target lots %d, want 5 (15000 / (300*10))", buy.TargetLots)
	}
}

func TestScoreTableSourceCarriesForwardAndGoesStale(t *testing.T) {
	path := writeTable(t, "ticker,date,p\nSBER,2026-10-01,0.7\n")
	src, err := LoadScoreTable(path, 100, decimal.Zero)
	if err != nil {
		t.Fatal(err)
	}
	feature := func(day int) domain.FeatureContext {
		return domain.FeatureContext{Ticker: "SBER", GeneratedAt: time.Date(2026, 10, day, 12, 0, 0, 0, time.UTC)}
	}
	got, _ := src.Generate(context.Background(), feature(5))
	if got.Action != domain.ActionBuy {
		t.Fatalf("carry forward: action %v, want BUY", got.Action)
	}
	got, _ = src.Generate(context.Background(), feature(9))
	if got.Action != domain.ActionHold {
		t.Fatalf("stale row older than 7 days must HOLD, got %v", got.Action)
	}
	before := domain.FeatureContext{Ticker: "SBER", GeneratedAt: time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)}
	got, _ = src.Generate(context.Background(), before)
	if got.Action != domain.ActionHold {
		t.Fatalf("date before first row must HOLD, got %v", got.Action)
	}
}

func TestLoadScoreTableRejectsBadHeader(t *testing.T) {
	if _, err := LoadScoreTable(writeTable(t, "ticker,when,p\nSBER,2026-10-01,0.7\n"), 1, decimal.Zero); err == nil {
		t.Fatal("expected error for missing date column")
	}
}
