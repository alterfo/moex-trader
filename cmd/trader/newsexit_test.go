package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/storage"
)

type fakePositions struct{ lots map[string]int }

func (f *fakePositions) CurrentLots(_ context.Context, ticker string) (int, error) {
	return f.lots[ticker], nil
}

type exitFixture struct {
	gate *newsGateSignalSource
	pos  *fakePositions
	now  *time.Time
	exit *newsExitManager
}

func newExitFixture(t *testing.T, modelAction domain.Action) *exitFixture {
	t.Helper()
	gate := newIncidentShortGate(t, modelAction, nil)
	pos := &fakePositions{lots: map[string]int{}}
	clock := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	exits, err := newNewsExitManager(newsExitConfig{
		TakeProfit: decimal.RequireFromString("0.015"),
		Trail:      decimal.RequireFromString("0.007"),
		StopLoss:   decimal.RequireFromString("0.025"),
		MaxHold:    24 * time.Hour,
		Cooldown:   48 * time.Hour,
		StatePath:  filepath.Join(t.TempDir(), "news_trades.json"),
	}, pos, func() time.Time { return clock }, nil)
	if err != nil {
		t.Fatal(err)
	}
	gate.exits = exits
	return &exitFixture{gate: gate, pos: pos, now: &clock, exit: exits}
}

func (f *exitFixture) gen(t *testing.T, incidents int, price int64) domain.TradeSignal {
	t.Helper()
	signal, err := f.gate.Generate(context.Background(), domain.FeatureContext{
		Ticker:        "YDEX",
		IncidentCount: incidents,
		LastPrice:     decimal.NewFromInt(price),
		LotSize:       decimal.NewFromInt(1),
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	return signal
}

func (f *exitFixture) advance(d time.Duration) { *f.now = f.now.Add(d) }

func openShort(t *testing.T, f *exitFixture) {
	t.Helper()
	signal := f.gen(t, 3, 300)
	if signal.Action != domain.ActionSell {
		t.Fatalf("entry signal = %+v, want SELL", signal)
	}
	f.pos.lots["YDEX"] = -signal.TargetLots
	f.advance(5 * time.Minute)
}

func TestNewsExitTakesProfitAfterRetrace(t *testing.T) {
	f := newExitFixture(t, domain.ActionHold)
	openShort(t, f)

	if s := f.gen(t, 3, 292); s.Flat {
		t.Fatalf("peak gain 2.7%% with no retrace must not exit, got %+v", s)
	}
	f.advance(5 * time.Minute)
	if s := f.gen(t, 3, 294); s.Flat {
		t.Fatalf("retrace 0.6pp is below trail 0.7pp, got exit %+v", s)
	}
	f.advance(5 * time.Minute)
	s := f.gen(t, 3, 296)
	if !s.Flat || s.Action != domain.ActionBuy || s.TargetLots != 0 {
		t.Fatalf("signal = %+v, want flat BUY exit after retrace from peak", s)
	}
	if err := s.Validate(); err != nil {
		t.Fatalf("exit signal must validate: %v", err)
	}
}

func TestNewsExitStopLoss(t *testing.T) {
	f := newExitFixture(t, domain.ActionHold)
	openShort(t, f)
	s := f.gen(t, 3, 308)
	if !s.Flat || s.Action != domain.ActionBuy {
		t.Fatalf("signal = %+v, want stop-loss exit at +2.7%% against the short", s)
	}
}

func TestNewsExitTimeExit(t *testing.T) {
	f := newExitFixture(t, domain.ActionHold)
	openShort(t, f)
	f.advance(24 * time.Hour)
	s := f.gen(t, 3, 300)
	if !s.Flat {
		t.Fatalf("signal = %+v, want time exit after max hold", s)
	}
}

func TestNewsExitNoReshortDuringCooldownAndKeepsReemittingUntilFlat(t *testing.T) {
	f := newExitFixture(t, domain.ActionHold)
	openShort(t, f)
	if s := f.gen(t, 3, 308); !s.Flat {
		t.Fatalf("want exit, got %+v", s)
	}
	f.advance(5 * time.Minute)
	if s := f.gen(t, 3, 308); !s.Flat {
		t.Fatalf("exit must repeat while position still open, got %+v", s)
	}
	f.pos.lots["YDEX"] = 0
	f.advance(5 * time.Minute)
	if s := f.gen(t, 3, 300); s.Action == domain.ActionSell {
		t.Fatalf("incident must not re-short during cooldown, got %+v", s)
	}
	f.advance(48 * time.Hour)
	if s := f.gen(t, 3, 300); s.Action != domain.ActionSell || s.Flat {
		t.Fatalf("after cooldown a fresh incident may short again, got %+v", s)
	}
}

func TestNewsExitIgnoresPreExistingShort(t *testing.T) {
	f := newExitFixture(t, domain.ActionHold)
	f.pos.lots["YDEX"] = -10
	f.gen(t, 3, 300)
	if f.exit.tracked("YDEX") {
		t.Fatal("a short that existed before the incident is not news-owned")
	}
	f.advance(48 * time.Hour)
	if s := f.gen(t, 3, 330); s.Flat {
		t.Fatalf("untracked position must never be force-closed, got %+v", s)
	}
}

func TestNewsExitDropsUnfilledEntry(t *testing.T) {
	f := newExitFixture(t, domain.ActionHold)
	f.gen(t, 3, 300)
	f.advance(20 * time.Minute)
	f.gen(t, 0, 300)
	if f.exit.tracked("YDEX") {
		t.Fatal("entry that never filled must be dropped after the grace period")
	}
}

func TestNewsExitStatePersists(t *testing.T) {
	f := newExitFixture(t, domain.ActionHold)
	openShort(t, f)
	reloaded, err := newNewsExitManager(f.exit.cfg, f.pos, func() time.Time { return *f.now }, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded.tracked("YDEX") {
		t.Fatal("tracked trade must survive restart")
	}
}

func TestNewsExitRecordsAuditEventOnce(t *testing.T) {
	store, err := storage.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	f := newExitFixture(t, domain.ActionHold)
	f.exit.store = store
	openShort(t, f)
	f.gen(t, 3, 308)
	f.advance(5 * time.Minute)
	f.gen(t, 3, 308)
	events, err := store.ListAllAuditEvents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range events {
		if e.Stage == "news_exit" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("news_exit audit events = %d, want exactly 1 per trade", n)
	}
}

func TestNewsExitStatePersistsInStore(t *testing.T) {
	store, err := storage.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	clock := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	pos := &fakePositions{lots: map[string]int{}}
	cfg := newsExitConfig{TakeProfit: decimal.RequireFromString("0.015"), Trail: decimal.RequireFromString("0.007"), StopLoss: decimal.RequireFromString("0.025"), MaxHold: time.Hour}
	first, err := newNewsExitManager(cfg, pos, func() time.Time { return clock }, store)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.noteShortEntry(context.Background(), "YDEX", decimal.NewFromInt(300)); err != nil {
		t.Fatal(err)
	}
	second, err := newNewsExitManager(cfg, pos, func() time.Time { return clock }, store)
	if err != nil {
		t.Fatal(err)
	}
	if !second.tracked("YDEX") {
		t.Fatal("state stored in the DB must be visible to a manager on another node")
	}
}
