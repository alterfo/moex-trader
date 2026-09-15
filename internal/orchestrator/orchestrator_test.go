package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/executor"
	"github.com/olegsidorkin/moex-trader/internal/features"
	"github.com/olegsidorkin/moex-trader/internal/risk"
	"github.com/olegsidorkin/moex-trader/internal/storage"
)

func openTestStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "trader.db"))
	if err != nil {
		t.Fatalf("storage.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("store.Close() error = %v", err)
		}
	})
	return store
}

type fakeIngestor struct {
	inputs map[string]features.Input
	err    error
	calls  atomic.Int32
}

func (f *fakeIngestor) Ingest(ctx context.Context, ticker string) (features.Input, error) {
	f.calls.Add(1)
	if f.err != nil {
		return features.Input{}, f.err
	}
	input, ok := f.inputs[ticker]
	if !ok {
		return features.Input{}, fmt.Errorf("no fixture input for %s", ticker)
	}
	return input, nil
}

type cyclePreparingIngestor struct {
	*fakeIngestor
	prepared atomic.Int32
	tickers  []string
}

func (f *cyclePreparingIngestor) PrepareCycle(_ context.Context, tickers []string) {
	f.prepared.Add(1)
	f.tickers = append([]string(nil), tickers...)
}

type fakeSource struct {
	signal domain.TradeSignal
	now    func() time.Time
}

func (f *fakeSource) Generate(ctx context.Context, feature domain.FeatureContext) (domain.TradeSignal, error) {
	signal := f.signal
	signal.Ticker = feature.Ticker
	signal.GeneratedAt = f.now()
	return signal, nil
}

type fixedTickerSource struct {
	signal domain.TradeSignal
	now    func() time.Time
}

func (f *fixedTickerSource) Generate(_ context.Context, _ domain.FeatureContext) (domain.TradeSignal, error) {
	signal := f.signal
	signal.GeneratedAt = f.now()
	return signal, nil
}

type executorCall struct {
	signal domain.TradeSignal
	price  decimal.Decimal
}

type fakeExecutor struct {
	store *storage.Store
	now   func() time.Time

	mu    sync.Mutex
	calls []executorCall
}

type failingAuditWriter struct{}

func (failingAuditWriter) InsertAuditEvent(_ context.Context, _ domain.AuditEvent) error {
	return fmt.Errorf("disk full")
}

func (f *fakeExecutor) Execute(ctx context.Context, signal domain.TradeSignal, price decimal.Decimal) (executor.Fill, error) {
	f.mu.Lock()
	f.calls = append(f.calls, executorCall{signal: signal, price: price})
	f.mu.Unlock()

	fill := executor.Fill{
		ID:         uuid.NewString(),
		Ticker:     signal.Ticker,
		Action:     signal.Action,
		Lots:       signal.TargetLots,
		Price:      price,
		ExecutedAt: f.now(),
	}
	payload, err := json.Marshal(fill)
	if err != nil {
		return executor.Fill{}, err
	}
	event := domain.AuditEvent{
		ID:        fill.ID,
		Ticker:    fill.Ticker,
		Stage:     StageExecutor,
		Payload:   string(payload),
		CreatedAt: f.now(),
	}
	if err := f.store.InsertAuditEvent(ctx, event); err != nil {
		return executor.Fill{}, err
	}
	return fill, nil
}

func (f *fakeExecutor) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func fixtureInput(ticker string) features.Input {
	return features.Input{
		Ticker: ticker,
		Price: features.PriceSnapshot{
			LastPrice: decimal.NewFromFloat(110),
			PrevClose: decimal.NewFromFloat(100),
			Bid:       decimal.NewFromFloat(109.9),
			Ask:       decimal.NewFromFloat(110.1),
			AsOf:      time.Date(2024, 1, 11, 12, 0, 0, 0, time.UTC),
		},
	}
}

func newTestOrchestrator(t *testing.T, store *storage.Store, ingestor Ingestor, source SignalSource, now func() time.Time, exec executor.Executor) *Orchestrator {
	t.Helper()
	gate, err := risk.NewLotLimitGate(1)
	if err != nil {
		t.Fatalf("NewLotLimitGate() error = %v", err)
	}
	orch, err := New(Options{
		Tickers:      []string{"SBER", "YDEX"},
		Ingestor:     ingestor,
		Source:       source,
		Gate:         gate,
		Executor:     exec,
		Audit:        store,
		PollInterval: time.Second,
		Now:          now,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return orch
}

func countEvents(events []domain.AuditEvent, ticker, stage string) int {
	count := 0
	for _, event := range events {
		if event.Ticker == ticker && event.Stage == stage {
			count++
		}
	}
	return count
}

func TestRunOnceRecordsFullPipeline(t *testing.T) {
	store := openTestStore(t)
	now := func() time.Time { return time.Date(2024, 1, 11, 12, 30, 0, 0, time.UTC) }
	ingestor := &fakeIngestor{inputs: map[string]features.Input{
		"SBER": fixtureInput("SBER"),
		"YDEX": fixtureInput("YDEX"),
	}}
	source := &fakeSource{
		signal: domain.TradeSignal{
			Action:      domain.ActionBuy,
			Confidence:  decimal.NewFromFloat(0.8),
			TargetLots:  1,
			Reasoning:   "fixture buy",
			GeneratedAt: now(),
		},
		now: now,
	}
	exec := &fakeExecutor{store: store, now: now}
	orch := newTestOrchestrator(t, store, ingestor, source, now, exec)

	orch.RunOnce(context.Background())

	if ingestor.calls.Load() != 2 {
		t.Fatalf("ingestor calls = %d, want 2", ingestor.calls.Load())
	}
	if exec.callCount() != 2 {
		t.Fatalf("executor calls = %d, want 2", exec.callCount())
	}

	events, err := store.ListAuditEvents(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("ListAuditEvents() error = %v", err)
	}
	if len(events) != 8 {
		t.Fatalf("ListAuditEvents() returned %d events, want 8", len(events))
	}
	for _, ticker := range []string{"SBER", "YDEX"} {
		for _, stage := range []string{StageIngest, StageSignal, StageRisk, StageExecutor} {
			if got := countEvents(events, ticker, stage); got != 1 {
				t.Fatalf("event count for %s/%s = %d, want 1", ticker, stage, got)
			}
		}
	}
}

func TestRunOnceCallsCyclePreparer(t *testing.T) {
	store := openTestStore(t)
	now := func() time.Time { return time.Date(2024, 1, 11, 12, 30, 0, 0, time.UTC) }
	ingestor := &cyclePreparingIngestor{fakeIngestor: &fakeIngestor{inputs: map[string]features.Input{
		"SBER": fixtureInput("SBER"),
		"YDEX": fixtureInput("YDEX"),
	}}}
	source := &fakeSource{
		signal: domain.TradeSignal{
			Action:      domain.ActionHold,
			Confidence:  decimal.NewFromFloat(0.5),
			Reasoning:   "hold",
			GeneratedAt: now(),
		},
		now: now,
	}
	exec := &fakeExecutor{store: store, now: now}
	orch := newTestOrchestrator(t, store, ingestor, source, now, exec)

	orch.RunOnce(context.Background())

	if ingestor.prepared.Load() != 1 {
		t.Fatalf("PrepareCycle calls = %d, want 1", ingestor.prepared.Load())
	}
	if len(ingestor.tickers) != 2 || ingestor.tickers[0] != "SBER" || ingestor.tickers[1] != "YDEX" {
		t.Fatalf("unexpected prepared tickers: %v", ingestor.tickers)
	}
}

func TestOrchestratorKillSwitchBlocksNextCycle(t *testing.T) {
	store := openTestStore(t)
	now := func() time.Time { return time.Date(2024, 1, 11, 12, 30, 0, 0, time.UTC) }
	ingestor := &fakeIngestor{inputs: map[string]features.Input{
		"SBER": fixtureInput("SBER"),
		"YDEX": fixtureInput("YDEX"),
	}}
	source := &fakeSource{
		signal: domain.TradeSignal{
			Action:      domain.ActionBuy,
			Confidence:  decimal.NewFromFloat(0.8),
			TargetLots:  1,
			Reasoning:   "fixture buy",
			GeneratedAt: now(),
		},
		now: now,
	}
	exec := &fakeExecutor{store: store, now: now}

	cfg := risk.DefaultConfig()
	cfg.Store = store
	gate, err := risk.NewHardenedGate(cfg)
	if err != nil {
		t.Fatalf("NewHardenedGate() error = %v", err)
	}
	account := risk.Account{
		Deposit:        decimal.RequireFromString("1000"),
		DayStartEquity: decimal.RequireFromString("969.99"),
		CurrentEquity:  decimal.RequireFromString("969.99"),
	}

	orch, err := New(Options{
		Tickers:      []string{"SBER", "YDEX"},
		Ingestor:     ingestor,
		Source:       source,
		Gate:         gate,
		Executor:     exec,
		Audit:        store,
		PollInterval: time.Second,
		Now:          now,
		Account:      account,
		KillSwitch:   store,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	orch.RunOnce(context.Background())
	orch.RunOnce(context.Background())

	if ingestor.calls.Load() != 1 {
		t.Fatalf("ingestor calls = %d, want 1 after kill switch blocked the loop", ingestor.calls.Load())
	}
	if exec.callCount() != 0 {
		t.Fatalf("executor calls = %d, want 0", exec.callCount())
	}

	active, err := store.IsKillSwitchActive(context.Background())
	if err != nil {
		t.Fatalf("IsKillSwitchActive() error = %v", err)
	}
	if !active {
		t.Fatal("expected kill switch to remain active in storage")
	}

	events, err := store.ListAuditEvents(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("ListAuditEvents() error = %v", err)
	}
	if got := countEvents(events, "SBER", StageRisk); got != 1 {
		t.Fatalf("risk events for SBER = %d, want 1", got)
	}
	if got := countEvents(events, "YDEX", StageIngest); got != 0 {
		t.Fatalf("ingest events for YDEX = %d, want 0 after kill switch", got)
	}
}

func TestRunOnceProcessesMultipleTicks(t *testing.T) {
	store := openTestStore(t)
	now := func() time.Time { return time.Date(2024, 1, 11, 12, 30, 0, 0, time.UTC) }
	ingestor := &fakeIngestor{inputs: map[string]features.Input{
		"SBER": fixtureInput("SBER"),
		"YDEX": fixtureInput("YDEX"),
	}}
	source := &fakeSource{
		signal: domain.TradeSignal{
			Action:      domain.ActionBuy,
			Confidence:  decimal.NewFromFloat(0.8),
			TargetLots:  1,
			Reasoning:   "fixture buy",
			GeneratedAt: now(),
		},
		now: now,
	}
	exec := &fakeExecutor{store: store, now: now}
	orch := newTestOrchestrator(t, store, ingestor, source, now, exec)

	orch.RunOnce(context.Background())
	orch.RunOnce(context.Background())

	if ingestor.calls.Load() != 4 {
		t.Fatalf("ingestor calls = %d, want 4", ingestor.calls.Load())
	}
	if exec.callCount() != 4 {
		t.Fatalf("executor calls = %d, want 4", exec.callCount())
	}

	events, err := store.ListAuditEvents(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("ListAuditEvents() error = %v", err)
	}
	if len(events) != 16 {
		t.Fatalf("ListAuditEvents() returned %d events, want 16", len(events))
	}
}

func TestRunShutsDownCleanly(t *testing.T) {
	store := openTestStore(t)
	now := time.Now
	ingestor := &fakeIngestor{inputs: map[string]features.Input{
		"SBER": fixtureInput("SBER"),
	}}
	source := &fakeSource{
		signal: domain.TradeSignal{
			Action:      domain.ActionHold,
			Confidence:  decimal.NewFromFloat(0.5),
			TargetLots:  0,
			Reasoning:   "fixture hold",
			GeneratedAt: now(),
		},
		now: now,
	}
	exec := &fakeExecutor{store: store, now: now}
	gate, err := risk.NewLotLimitGate(1)
	if err != nil {
		t.Fatalf("NewLotLimitGate() error = %v", err)
	}
	orch, err := New(Options{
		Tickers:      []string{"SBER"},
		Ingestor:     ingestor,
		Source:       source,
		Gate:         gate,
		Executor:     exec,
		Audit:        store,
		PollInterval: 20 * time.Millisecond,
		Now:          now,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		orch.Run(ctx)
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for ingestor.calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if ingestor.calls.Load() == 0 {
		t.Fatal("orchestrator did not run its first sweep")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}

func TestRunOnceRiskGateRejectsSignal(t *testing.T) {
	store := openTestStore(t)
	now := func() time.Time { return time.Date(2024, 1, 11, 12, 30, 0, 0, time.UTC) }
	ingestor := &fakeIngestor{inputs: map[string]features.Input{
		"SBER": fixtureInput("SBER"),
		"YDEX": fixtureInput("YDEX"),
	}}
	source := &fakeSource{
		signal: domain.TradeSignal{
			Action:      domain.ActionBuy,
			Confidence:  decimal.NewFromFloat(0.9),
			TargetLots:  5,
			Reasoning:   "too many lots",
			GeneratedAt: now(),
		},
		now: now,
	}
	exec := &fakeExecutor{store: store, now: now}
	orch := newTestOrchestrator(t, store, ingestor, source, now, exec)

	orch.RunOnce(context.Background())

	if exec.callCount() != 0 {
		t.Fatalf("executor calls = %d, want 0 for rejected signals", exec.callCount())
	}
	events, err := store.ListAuditEvents(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("ListAuditEvents() error = %v", err)
	}
	if got := countEvents(events, "SBER", StageRisk); got != 1 {
		t.Fatalf("risk events for SBER = %d, want 1", got)
	}
	if got := countEvents(events, "SBER", StageExecutor); got != 0 {
		t.Fatalf("executor events for SBER = %d, want 0", got)
	}
}

func TestProcessTickerRecordsIngestFailure(t *testing.T) {
	store := openTestStore(t)
	now := func() time.Time { return time.Date(2024, 1, 11, 12, 30, 0, 0, time.UTC) }
	ingestor := &fakeIngestor{err: fmt.Errorf("moex unavailable")}
	source := &fakeSource{now: now}
	exec := &fakeExecutor{store: store, now: now}
	orch := newTestOrchestrator(t, store, ingestor, source, now, exec)

	err := orch.processTicker(context.Background(), "SBER")
	if err == nil {
		t.Fatal("expected error for failed ingest")
	}
	if !strings.Contains(err.Error(), "moex unavailable") {
		t.Fatalf("unexpected error %v", err)
	}

	events, listErr := store.ListAuditEvents(context.Background(), time.Time{})
	if listErr != nil {
		t.Fatalf("ListAuditEvents() error = %v", listErr)
	}
	if len(events) != 1 {
		t.Fatalf("ListAuditEvents() returned %d events, want 1", len(events))
	}
	if events[0].Stage != StageIngest {
		t.Fatalf("event stage = %q, want ingest", events[0].Stage)
	}
	if !strings.Contains(events[0].Payload, "moex unavailable") {
		t.Fatalf("event payload does not contain error: %q", events[0].Payload)
	}
}

func TestProcessTickerPropagatesAuditWriteFailure(t *testing.T) {
	store := openTestStore(t)
	now := func() time.Time { return time.Date(2024, 1, 11, 12, 30, 0, 0, time.UTC) }
	ingestor := &fakeIngestor{inputs: map[string]features.Input{"SBER": fixtureInput("SBER")}}
	source := &fakeSource{
		signal: domain.TradeSignal{
			Action:      domain.ActionBuy,
			Confidence:  decimal.NewFromFloat(0.8),
			TargetLots:  1,
			Reasoning:   "fixture buy",
			GeneratedAt: now(),
		},
		now: now,
	}
	exec := &fakeExecutor{store: store, now: now}
	orch := newTestOrchestrator(t, store, ingestor, source, now, exec)
	orch.audit = failingAuditWriter{}

	err := orch.processTicker(context.Background(), "SBER")
	if err == nil {
		t.Fatal("expected audit write error, got nil")
	}
	if !strings.Contains(err.Error(), "persist ingest audit event") {
		t.Fatalf("unexpected error: %v", err)
	}
	if exec.callCount() != 0 {
		t.Fatalf("executor calls = %d, want 0 when audit persistence fails", exec.callCount())
	}
}

func TestProcessTickerRejectsMismatchedSignalTicker(t *testing.T) {
	store := openTestStore(t)
	now := func() time.Time { return time.Date(2024, 1, 11, 12, 30, 0, 0, time.UTC) }
	ingestor := &fakeIngestor{inputs: map[string]features.Input{"SBER": fixtureInput("SBER")}}
	source := &fixedTickerSource{
		signal: domain.TradeSignal{
			Ticker:     "YDEX",
			Action:     domain.ActionBuy,
			Confidence: decimal.NewFromFloat(0.8),
			TargetLots: 1,
			Reasoning:  "wrong ticker",
		},
		now: now,
	}
	exec := &fakeExecutor{store: store, now: now}
	orch := newTestOrchestrator(t, store, ingestor, source, now, exec)

	err := orch.processTicker(context.Background(), "SBER")
	if err == nil {
		t.Fatal("expected mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "does not match requested ticker") {
		t.Fatalf("unexpected error: %v", err)
	}
	if exec.callCount() != 0 {
		t.Fatalf("executor calls = %d, want 0", exec.callCount())
	}

	events, listErr := store.ListAuditEvents(context.Background(), time.Time{})
	if listErr != nil {
		t.Fatalf("ListAuditEvents() error = %v", listErr)
	}
	if got := countEvents(events, "SBER", StageRisk); got != 0 {
		t.Fatalf("risk events = %d, want 0", got)
	}
	if got := countEvents(events, "SBER", StageExecutor); got != 0 {
		t.Fatalf("executor events = %d, want 0", got)
	}
}
