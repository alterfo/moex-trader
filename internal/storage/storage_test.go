package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/shopspring/decimal"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "trader.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})
	return store
}

func TestOpenConfiguresWALBusyTimeoutAndSchema(t *testing.T) {
	store := openTestStore(t)

	var journalMode string
	if err := store.db.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", journalMode)
	}

	var busyTimeout int
	if err := store.db.QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		t.Fatalf("query busy_timeout: %v", err)
	}
	if busyTimeout != 5000 {
		t.Fatalf("busy_timeout = %d, want 5000", busyTimeout)
	}

	rows, err := store.db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name`)
	if err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table name: %v", err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate sqlite_master: %v", err)
	}

	got := strings.Join(tables, ",")
	for _, want := range []string{"audit_events", "schema_migrations", "trade_signals"} {
		if !strings.Contains(got, want) {
			t.Fatalf("schema missing %q, got tables %v", want, tables)
		}
	}
}

func TestAuditEventInsertAndListRoundTrip(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Add(-time.Minute)

	events := []domain.AuditEvent{
		{ID: "event-1", Ticker: "SBER", Stage: "ingest", Payload: `{"price":"270.5"}`, CreatedAt: now.Add(-time.Minute)},
		{ID: "event-2", Ticker: "YDEX", Stage: "risk_gate", Payload: `{"approved":true}`, CreatedAt: now},
		{ID: "event-3", Ticker: "OZON", Stage: "executor", Payload: `{"filled":true}`, CreatedAt: now.Add(time.Minute)},
	}
	for _, event := range events {
		if err := store.InsertAuditEvent(ctx, event); err != nil {
			t.Fatalf("InsertAuditEvent(%s) error = %v", event.ID, err)
		}
	}

	since := now
	got, err := store.ListAuditEvents(ctx, since)
	if err != nil {
		t.Fatalf("ListAuditEvents() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListAuditEvents(since) returned %d events, want 2", len(got))
	}
	wantIDs := []string{"event-2", "event-3"}
	for i, wantID := range wantIDs {
		if got[i].ID != wantID {
			t.Fatalf("event[%d].ID = %q, want %q", i, got[i].ID, wantID)
		}
		if got[i].CreatedAt.UnixNano() != events[i+1].CreatedAt.UnixNano() {
			t.Fatalf("event[%d].CreatedAt = %d, want %d", i, got[i].CreatedAt.UnixNano(), events[i+1].CreatedAt.UnixNano())
		}
	}

	all, err := store.ListAuditEvents(ctx, time.Time{})
	if err != nil {
		t.Fatalf("ListAuditEvents(all) error = %v", err)
	}
	if len(all) != len(events) {
		t.Fatalf("ListAuditEvents(all) returned %d events, want %d", len(all), len(events))
	}
	for i, event := range events {
		if all[i].ID != event.ID || all[i].Payload != event.Payload {
			t.Fatalf("all[%d] = %+v, want %+v", i, all[i], event)
		}
	}
}

func TestStoreCurrentLots(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	events := []domain.AuditEvent{
		{ID: "buy", Ticker: "SBER", Stage: "executor", Payload: `{"action":"BUY","lots":1}`, CreatedAt: now},
		{ID: "sell-two", Ticker: "SBER", Stage: "executor", Payload: `{"action":"SELL","lots":2}`, CreatedAt: now.Add(time.Second)},
		{ID: "other-ticker", Ticker: "YDEX", Stage: "executor", Payload: `{"action":"BUY","lots":5}`, CreatedAt: now},
		{ID: "malformed", Ticker: "SBER", Stage: "executor", Payload: `{not-json`, CreatedAt: now},
	}
	for _, event := range events {
		if err := store.InsertAuditEvent(ctx, event); err != nil {
			t.Fatalf("InsertAuditEvent(%s) error = %v", event.ID, err)
		}
	}

	got, err := store.CurrentLots(ctx, "SBER")
	if err != nil {
		t.Fatalf("CurrentLots() error = %v", err)
	}
	if got != -1 {
		t.Fatalf("CurrentLots(SBER) = %d, want -1", got)
	}
}

func TestInsertAuditEventDuplicateIDFails(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	event := domain.AuditEvent{
		ID:        "duplicate-id",
		Ticker:    "SBER",
		Stage:     "ingest",
		Payload:   `{}`,
		CreatedAt: time.Now(),
	}

	if err := store.InsertAuditEvent(ctx, event); err != nil {
		t.Fatalf("first InsertAuditEvent() error = %v", err)
	}
	err := store.InsertAuditEvent(ctx, event)
	if err == nil {
		t.Fatal("second InsertAuditEvent() error = nil, want constraint violation")
	}
	lower := strings.ToLower(err.Error())
	if !strings.Contains(lower, "constraint") && !strings.Contains(lower, "unique") {
		t.Fatalf("second InsertAuditEvent() error = %v, want constraint violation", err)
	}
}

func TestInsertTradeSignalRoundTripAndValidation(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	signal := domain.TradeSignal{
		Ticker:      "SBER",
		Action:      domain.ActionBuy,
		Confidence:  decimal.NewFromFloat(0.55),
		TargetLots:  1,
		Reasoning:   "positive momentum",
		GeneratedAt: time.Now().Add(-time.Second),
	}
	if err := store.InsertTradeSignal(ctx, signal); err != nil {
		t.Fatalf("InsertTradeSignal() error = %v", err)
	}

	var ticker, action, confidence string
	var targetLots int
	err := store.db.QueryRow(`SELECT ticker, action, confidence, target_lots FROM trade_signals WHERE ticker = ?`, "SBER").Scan(&ticker, &action, &confidence, &targetLots)
	if err != nil {
		t.Fatalf("query trade_signal: %v", err)
	}
	if ticker != "SBER" || action != "BUY" || confidence != "0.55" || targetLots != 1 {
		t.Fatalf("stored signal = (%q, %q, %q, %d)", ticker, action, confidence, targetLots)
	}

	invalid := signal
	invalid.Action = domain.Action("HODL")
	if err := store.InsertTradeSignal(ctx, invalid); err == nil {
		t.Fatal("InsertTradeSignal(invalid action) error = nil, want validation error")
	}

	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM trade_signals`).Scan(&count); err != nil {
		t.Fatalf("count trade_signals: %v", err)
	}
	if count != 1 {
		t.Fatalf("trade_signals count = %d, want 1", count)
	}
}

func TestConcurrentWALWrites(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	const writers = 10
	const writesPerWriter = 5

	errs := make(chan error, writers*writesPerWriter)
	var wg sync.WaitGroup
	for writer := 0; writer < writers; writer++ {
		wg.Add(1)
		go func(writer int) {
			defer wg.Done()
			for write := 0; write < writesPerWriter; write++ {
				event := domain.AuditEvent{
					ID:        fmt.Sprintf("writer-%d-write-%d", writer, write),
					Ticker:    "SBER",
					Stage:     "ingest",
					Payload:   `{"concurrent":true}`,
					CreatedAt: time.Now(),
				}
				errs <- store.InsertAuditEvent(ctx, event)
			}
		}(writer)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent InsertAuditEvent() error = %v", err)
		}
	}

	events, err := store.ListAuditEvents(ctx, time.Time{})
	if err != nil {
		t.Fatalf("ListAuditEvents() error = %v", err)
	}
	if len(events) != writers*writesPerWriter {
		t.Fatalf("ListAuditEvents() returned %d events, want %d", len(events), writers*writesPerWriter)
	}
}

func TestKillSwitchPersistence(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	active, err := store.IsKillSwitchActive(ctx)
	if err != nil {
		t.Fatalf("IsKillSwitchActive() error = %v", err)
	}
	if active {
		t.Fatal("IsKillSwitchActive() = true, want false before trigger")
	}

	if err := store.SetKillSwitchActive(ctx, true); err != nil {
		t.Fatalf("SetKillSwitchActive(true) error = %v", err)
	}
	active, err = store.IsKillSwitchActive(ctx)
	if err != nil {
		t.Fatalf("IsKillSwitchActive() error = %v", err)
	}
	if !active {
		t.Fatal("IsKillSwitchActive() = false, want true after trigger")
	}

	if err := store.SetKillSwitchActive(ctx, false); err != nil {
		t.Fatalf("SetKillSwitchActive(false) error = %v", err)
	}
	active, err = store.IsKillSwitchActive(ctx)
	if err != nil {
		t.Fatalf("IsKillSwitchActive() error = %v", err)
	}
	if active {
		t.Fatal("IsKillSwitchActive() = true, want false after reset")
	}
}

func TestKillSwitchPersistsAcrossStoreInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trader.db")
	ctx := context.Background()

	first, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := first.SetKillSwitchActive(ctx, true); err != nil {
		t.Fatalf("SetKillSwitchActive(true) error = %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	second, err := Open(path)
	if err != nil {
		t.Fatalf("second Open() error = %v", err)
	}
	defer func() {
		if err := second.Close(); err != nil {
			t.Fatalf("second Close() error = %v", err)
		}
	}()

	active, err := second.IsKillSwitchActive(ctx)
	if err != nil {
		t.Fatalf("IsKillSwitchActive() error = %v", err)
	}
	if !active {
		t.Fatal("IsKillSwitchActive() = false, want true across store instances")
	}
}
