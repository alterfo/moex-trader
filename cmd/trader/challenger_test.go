package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

type scriptedSource struct {
	signal domain.TradeSignal
	err    error
	panics bool
	calls  int
}

func (s *scriptedSource) Generate(_ context.Context, feature domain.FeatureContext) (domain.TradeSignal, error) {
	s.calls++
	if s.panics {
		panic("boom")
	}
	out := s.signal
	out.Ticker = feature.Ticker
	return out, s.err
}

func readChallengerRecords(t *testing.T, path string) []challengerRecord {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []challengerRecord
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var rec challengerRecord
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			t.Fatal(err)
		}
		out = append(out, rec)
	}
	return out
}

func newTestChallenger(t *testing.T, champion, challenger *scriptedSource) (*challengerSignalSource, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nested", "decisions.jsonl")
	src, err := newChallengerSignalSource(champion, challenger, path, func() time.Time { return time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC) }, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = src.Close() })
	return src, path
}

func TestChallengerNeverChangesChampionSignal(t *testing.T) {
	champion := &scriptedSource{signal: domain.TradeSignal{Action: domain.ActionBuy, TargetLots: 3, Probability: decimal.RequireFromString("0.71")}}
	challenger := &scriptedSource{signal: domain.TradeSignal{Action: domain.ActionSell, TargetLots: 9, Probability: decimal.RequireFromString("0.12")}}
	src, path := newTestChallenger(t, champion, challenger)

	got, err := src.Generate(context.Background(), domain.FeatureContext{Ticker: "SBER", LastPrice: decimal.RequireFromString("301.5")})
	if err != nil {
		t.Fatal(err)
	}
	if got.Action != domain.ActionBuy || got.TargetLots != 3 {
		t.Fatalf("champion signal altered: %+v", got)
	}
	recs := readChallengerRecords(t, path)
	if len(recs) != 1 || recs[0].Ticker != "SBER" || recs[0].Price != "301.5" {
		t.Fatalf("records = %+v", recs)
	}
	if recs[0].Champion.Action != "BUY" || recs[0].Challenger.Action != "SELL" || recs[0].Challenger.Probability != "0.1200" {
		t.Fatalf("record = %+v", recs[0])
	}
}

func TestChallengerFailureAndPanicAreIsolated(t *testing.T) {
	champion := &scriptedSource{signal: domain.TradeSignal{Action: domain.ActionHold}}
	for name, challenger := range map[string]*scriptedSource{
		"error": {err: errors.New("bad model")},
		"panic": {panics: true},
	} {
		t.Run(name, func(t *testing.T) {
			src, path := newTestChallenger(t, champion, challenger)
			got, err := src.Generate(context.Background(), domain.FeatureContext{Ticker: "GAZP"})
			if err != nil || got.Action != domain.ActionHold {
				t.Fatalf("got %+v err %v", got, err)
			}
			recs := readChallengerRecords(t, path)
			if len(recs) != 1 || recs[0].Challenger.Error == "" {
				t.Fatalf("records = %+v", recs)
			}
		})
	}
}

func TestChallengerSkipsLoggingWhenChampionFails(t *testing.T) {
	champion := &scriptedSource{err: errors.New("no features")}
	challenger := &scriptedSource{}
	src, path := newTestChallenger(t, champion, challenger)
	if _, err := src.Generate(context.Background(), domain.FeatureContext{Ticker: "LKOH"}); err == nil {
		t.Fatal("champion error must propagate")
	}
	if challenger.calls != 0 || len(readChallengerRecords(t, path)) != 0 {
		t.Fatal("challenger must not run when the champion fails")
	}
}
