package verifier

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/storage"
)

type fakeEvents struct {
	events []domain.AuditEvent
}

func (f fakeEvents) ListAuditEvents(_ context.Context, _ time.Time) ([]domain.AuditEvent, error) {
	return f.events, nil
}

type fakeAllEvents struct {
	events []domain.AuditEvent
}

func (f fakeAllEvents) ListAuditEvents(_ context.Context, _ time.Time) ([]domain.AuditEvent, error) {
	return f.events, nil
}

func (f fakeAllEvents) ListAllAuditEvents(_ context.Context) ([]domain.AuditEvent, error) {
	return f.events, nil
}

func dec(s string) decimal.Decimal {
	return decimal.RequireFromString(s)
}

func signalEvent(id, ticker string, action domain.Action, confidence, reasoning string, at time.Time) domain.AuditEvent {
	payload, _ := json.Marshal(domain.TradeSignal{
		Ticker:      ticker,
		Action:      action,
		Confidence:  dec(confidence),
		TargetLots:  1,
		Reasoning:   reasoning,
		GeneratedAt: at,
	})
	return domain.AuditEvent{
		ID:        id,
		Ticker:    ticker,
		Stage:     stageSignal,
		Payload:   string(payload),
		CreatedAt: at,
	}
}

func fillEvent(id, ticker string, action domain.Action, lots int, price string, at time.Time) domain.AuditEvent {
	payload, _ := json.Marshal(fillRecord{
		ID:         id,
		Ticker:     ticker,
		Action:     action,
		Lots:       lots,
		Price:      dec(price),
		ExecutedAt: at,
	})
	return domain.AuditEvent{
		ID:        id,
		Ticker:    ticker,
		Stage:     stageExecutor,
		Payload:   string(payload),
		CreatedAt: at,
	}
}

func fillEventWithCommission(id, ticker string, action domain.Action, lots int, price, commission string, at time.Time) domain.AuditEvent {
	payload, _ := json.Marshal(fillRecord{
		ID:         id,
		Ticker:     ticker,
		Action:     action,
		Lots:       lots,
		Price:      dec(price),
		Commission: dec(commission),
		ExecutedAt: at,
	})
	return domain.AuditEvent{
		ID:        id,
		Ticker:    ticker,
		Stage:     stageExecutor,
		Payload:   string(payload),
		CreatedAt: at,
	}
}

func TestRunProducesLosingLongTradeReport(t *testing.T) {
	base := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	now := base.Add(10 * time.Second)

	events := []domain.AuditEvent{
		signalEvent("sig-sber", "SBER", domain.ActionBuy, "0.8", "positive momentum", base),
		fillEvent("fill-sber-buy", "SBER", domain.ActionBuy, 1, "100", base.Add(time.Second)),
		fillEvent("fill-sber-sell", "SBER", domain.ActionSell, 1, "95", base.Add(2*time.Second)),
	}

	report, err := New(fakeEvents{events: events}, func() time.Time { return now }).Run(context.Background(), base)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if report.ClosedTrades != 1 {
		t.Fatalf("ClosedTrades = %d, want 1", report.ClosedTrades)
	}
	if report.LosingTrades != 1 {
		t.Fatalf("LosingTrades = %d, want 1", report.LosingTrades)
	}
	if got := report.TotalRealizedPnL.String(); got != "-5" {
		t.Fatalf("TotalRealizedPnL = %q, want -5", got)
	}
	if len(report.Trades) != 1 {
		t.Fatalf("len(Trades) = %d, want 1", len(report.Trades))
	}

	trade := report.Trades[0]
	if trade.Ticker != "SBER" || trade.Direction != domain.ActionBuy || trade.Lots != 1 {
		t.Fatalf("trade = %+v, want SBER long 1 lot", trade)
	}
	if trade.EntryPrice.String() != "100" || trade.ExitPrice.String() != "95" || trade.RealizedPnL.String() != "-5" {
		t.Fatalf("prices/pnl = %s/%s/%s, want 100/95/-5", trade.EntryPrice, trade.ExitPrice, trade.RealizedPnL)
	}
	if trade.Signal == nil || trade.Signal.Action != domain.ActionBuy || trade.Signal.Confidence.String() != "0.8" {
		t.Fatalf("Signal = %+v, want BUY 0.8", trade.Signal)
	}
	if !strings.Contains(trade.Adequacy, "No — the BUY signal was followed by a losing long trade") {
		t.Fatalf("Adequacy = %q", trade.Adequacy)
	}

	markdown := report.Markdown()
	for _, want := range []string{
		"# Verifier Report",
		"- Closed trades: 1",
		"- Losing trades: 1",
		"## Losing trade: SBER",
		"confidence 0.8",
		"Was the LLM signal adequate? No — the BUY signal was followed by a losing long trade",
	} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("Markdown() missing %q:\n%s", want, markdown)
		}
	}
}

func TestRunOnlyReportsLosingTrades(t *testing.T) {
	base := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	now := base.Add(20 * time.Second)

	events := []domain.AuditEvent{
		signalEvent("sig-sber", "SBER", domain.ActionBuy, "0.7", "buy", base),
		fillEvent("sber-buy", "SBER", domain.ActionBuy, 1, "100", base.Add(time.Second)),
		fillEvent("sber-sell", "SBER", domain.ActionSell, 1, "110", base.Add(2*time.Second)),
		signalEvent("sig-ydex", "YDEX", domain.ActionSell, "0.7", "sell", base.Add(3*time.Second)),
		fillEvent("ydex-sell", "YDEX", domain.ActionSell, 1, "200", base.Add(4*time.Second)),
		fillEvent("ydex-buy", "YDEX", domain.ActionBuy, 1, "210", base.Add(5*time.Second)),
	}

	report, err := New(fakeEvents{events: events}, func() time.Time { return now }).Run(context.Background(), base)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if report.ClosedTrades != 2 {
		t.Fatalf("ClosedTrades = %d, want 2", report.ClosedTrades)
	}
	if report.LosingTrades != 1 {
		t.Fatalf("LosingTrades = %d, want 1", report.LosingTrades)
	}
	if got := report.TotalRealizedPnL.String(); got != "0" {
		t.Fatalf("TotalRealizedPnL = %q, want 0", got)
	}
	if len(report.Trades) != 1 || report.Trades[0].Ticker != "YDEX" {
		t.Fatalf("Trades = %+v, want only YDEX", report.Trades)
	}
	if report.Trades[0].Direction != domain.ActionSell {
		t.Fatalf("Trades[0].Direction = %s, want SELL", report.Trades[0].Direction)
	}
	if !strings.Contains(report.Trades[0].Adequacy, "No — the SELL signal was followed by a losing short trade") {
		t.Fatalf("Adequacy = %q", report.Trades[0].Adequacy)
	}
	if markdown := report.Markdown(); strings.Contains(markdown, "Losing trade: SBER") {
		t.Fatalf("Markdown() should not include the winning SBER trade:\n%s", markdown)
	}
}

func TestRunGrossPositiveNetNegativeAfterCommission(t *testing.T) {
	base := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	now := base.Add(5 * time.Second)

	events := []domain.AuditEvent{
		signalEvent("sig-sber", "SBER", domain.ActionBuy, "0.8", "positive momentum", base),
		fillEventWithCommission("fill-sber-buy", "SBER", domain.ActionBuy, 1, "100", "1", base.Add(time.Second)),
		fillEventWithCommission("fill-sber-sell", "SBER", domain.ActionSell, 1, "101", "1", base.Add(2*time.Second)),
	}

	report, err := New(fakeEvents{events: events}, func() time.Time { return now }).Run(context.Background(), base)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if report.ClosedTrades != 1 {
		t.Fatalf("ClosedTrades = %d, want 1", report.ClosedTrades)
	}
	if report.LosingTrades != 1 {
		t.Fatalf("LosingTrades = %d, want 1", report.LosingTrades)
	}
	if got := report.TotalGrossPnL.String(); got != "1" {
		t.Fatalf("TotalGrossPnL = %q, want 1", got)
	}
	if got := report.TotalCommission.String(); got != "2" {
		t.Fatalf("TotalCommission = %q, want 2", got)
	}
	if got := report.TotalRealizedPnL.String(); got != "-1" {
		t.Fatalf("TotalRealizedPnL = %q, want -1", got)
	}
	if len(report.Trades) != 1 {
		t.Fatalf("len(Trades) = %d, want 1", len(report.Trades))
	}

	trade := report.Trades[0]
	if trade.GrossPnL.String() != "1" || trade.Commission.String() != "2" || trade.RealizedPnL.String() != "-1" {
		t.Fatalf("trade P&L = gross %s / commission %s / net %s, want 1/2/-1", trade.GrossPnL, trade.Commission, trade.RealizedPnL)
	}

	markdown := report.Markdown()
	for _, want := range []string{
		"- Total realized P&L (gross): 1",
		"- Total commissions: 2",
		"- Total realized P&L (net): -1",
		"- Gross P&L: 1",
		"- Commission: 2",
		"- Net P&L: -1",
	} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("Markdown() missing %q:\n%s", want, markdown)
		}
	}
}

func TestRunWithoutSignalReportsUnknown(t *testing.T) {
	base := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	now := base.Add(5 * time.Second)

	events := []domain.AuditEvent{
		fillEvent("buy", "SBER", domain.ActionBuy, 1, "100", base.Add(time.Second)),
		fillEvent("sell", "SBER", domain.ActionSell, 1, "90", base.Add(2*time.Second)),
	}

	report, err := New(fakeEvents{events: events}, func() time.Time { return now }).Run(context.Background(), base)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(report.Trades) != 1 {
		t.Fatalf("len(Trades) = %d, want 1", len(report.Trades))
	}
	if report.Trades[0].Signal != nil {
		t.Fatalf("Signal = %+v, want nil", report.Trades[0].Signal)
	}
	if !strings.Contains(report.Trades[0].Adequacy, "Unknown — no LLM signal was recorded before this long trade") {
		t.Fatalf("Adequacy = %q", report.Trades[0].Adequacy)
	}
}

func TestRunReportsSignalDirectionMismatch(t *testing.T) {
	base := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	now := base.Add(5 * time.Second)

	events := []domain.AuditEvent{
		signalEvent("sig", "SBER", domain.ActionBuy, "0.6", "expects upside", base),
		fillEvent("sell", "SBER", domain.ActionSell, 1, "200", base.Add(time.Second)),
		fillEvent("buy", "SBER", domain.ActionBuy, 1, "210", base.Add(2*time.Second)),
	}

	report, err := New(fakeEvents{events: events}, func() time.Time { return now }).Run(context.Background(), base)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(report.Trades) != 1 {
		t.Fatalf("len(Trades) = %d, want 1", len(report.Trades))
	}
	if !strings.Contains(report.Trades[0].Adequacy, "No — the last signal was BUY, but the losing trade was short") {
		t.Fatalf("Adequacy = %q", report.Trades[0].Adequacy)
	}
}

func TestRunFiltersToWindow(t *testing.T) {
	base := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	since := base.Add(5 * time.Second)
	now := base.Add(20 * time.Second)

	events := []domain.AuditEvent{
		fillEvent("old-buy", "SBER", domain.ActionBuy, 1, "100", base.Add(time.Second)),
		fillEvent("old-sell", "SBER", domain.ActionSell, 1, "90", base.Add(2*time.Second)),
		fillEvent("new-buy", "YDEX", domain.ActionBuy, 1, "100", base.Add(6*time.Second)),
		fillEvent("new-sell", "YDEX", domain.ActionSell, 1, "95", base.Add(7*time.Second)),
	}

	report, err := New(fakeEvents{events: events}, func() time.Time { return now }).Run(context.Background(), since)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if report.ClosedTrades != 1 {
		t.Fatalf("ClosedTrades = %d, want 1", report.ClosedTrades)
	}
	if got := report.TotalRealizedPnL.String(); got != "-5" {
		t.Fatalf("TotalRealizedPnL = %q, want -5", got)
	}
	if len(report.Trades) != 1 || report.Trades[0].Ticker != "YDEX" {
		t.Fatalf("Trades = %+v, want only YDEX in window", report.Trades)
	}
}

func TestRunReconstructsPositionsOpenedBeforeWindow(t *testing.T) {
	base := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	since := base.Add(10 * time.Second)
	now := base.Add(20 * time.Second)

	events := []domain.AuditEvent{
		fillEvent("old-buy", "SBER", domain.ActionBuy, 1, "100", base.Add(5*time.Second)),
		fillEvent("close-sell", "SBER", domain.ActionSell, 1, "105", base.Add(15*time.Second)),
	}

	report, err := New(fakeAllEvents{events: events}, func() time.Time { return now }).Run(context.Background(), since)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if report.ClosedTrades != 1 {
		t.Fatalf("ClosedTrades = %d, want 1", report.ClosedTrades)
	}
	if got := report.TotalRealizedPnL.String(); got != "5" {
		t.Fatalf("TotalRealizedPnL = %q, want 5", got)
	}
	if report.LosingTrades != 0 || len(report.Trades) != 0 {
		t.Fatalf("report = %+v, want no losing trades", report)
	}
}

func TestRunEmptyHistory(t *testing.T) {
	base := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	report, err := New(fakeEvents{}, func() time.Time { return base }).Run(context.Background(), base)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if report.ClosedTrades != 0 || report.LosingTrades != 0 || len(report.Trades) != 0 {
		t.Fatalf("report = %+v, want empty", report)
	}
	if got := report.TotalRealizedPnL.String(); got != "0" {
		t.Fatalf("TotalRealizedPnL = %q, want 0", got)
	}
	if !strings.Contains(report.Markdown(), "No losing trades in this window.") {
		t.Fatalf("Markdown() missing empty summary:\n%s", report.Markdown())
	}
}

func TestRunSkipsMalformedPayloads(t *testing.T) {
	base := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	now := base.Add(5 * time.Second)

	events := []domain.AuditEvent{
		{ID: "bad-signal", Ticker: "SBER", Stage: stageSignal, Payload: "{not-json", CreatedAt: base},
		{ID: "bad-fill", Ticker: "SBER", Stage: stageExecutor, Payload: "{not-json", CreatedAt: base.Add(time.Second)},
		fillEvent("buy", "SBER", domain.ActionBuy, 1, "100", base.Add(2*time.Second)),
		fillEvent("sell", "SBER", domain.ActionSell, 1, "90", base.Add(3*time.Second)),
	}

	report, err := New(fakeEvents{events: events}, func() time.Time { return now }).Run(context.Background(), base)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if report.ClosedTrades != 1 || report.LosingTrades != 1 {
		t.Fatalf("report = %+v, want one losing trade after skipping malformed events", report)
	}
}

func TestRunSkipsNonTradeSignalPayloads(t *testing.T) {
	base := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	now := base.Add(6 * time.Second)

	events := []domain.AuditEvent{
		{ID: "llm-error", Ticker: "SBER", Stage: stageSignal, Payload: `{"error":"timeout","attempts":3,"fallback":"HOLD","target_lots":0}`, CreatedAt: base},
		signalEvent("sig", "SBER", domain.ActionBuy, "0.6", "buy", base.Add(time.Second)),
		fillEvent("buy", "SBER", domain.ActionBuy, 1, "100", base.Add(2*time.Second)),
		fillEvent("sell", "SBER", domain.ActionSell, 1, "90", base.Add(3*time.Second)),
	}

	report, err := New(fakeEvents{events: events}, func() time.Time { return now }).Run(context.Background(), base)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if report.ClosedTrades != 1 || report.LosingTrades != 1 {
		t.Fatalf("report = %+v, want one losing trade", report)
	}
	if report.Trades[0].Signal == nil || report.Trades[0].Signal.Action != domain.ActionBuy {
		t.Fatalf("Signal = %+v, want the valid BUY signal, not the LLM error payload", report.Trades[0].Signal)
	}
}

func TestRunWithStorageIntegration(t *testing.T) {
	store, err := storage.Open(filepath.Join(t.TempDir(), "trader.db"))
	if err != nil {
		t.Fatalf("storage.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	ctx := context.Background()
	base := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	for _, event := range []domain.AuditEvent{
		signalEvent("sig", "SBER", domain.ActionBuy, "0.9", "strong upside", base),
		fillEvent("buy", "SBER", domain.ActionBuy, 1, "100", base.Add(time.Second)),
		fillEvent("sell", "SBER", domain.ActionSell, 1, "80", base.Add(2*time.Second)),
	} {
		if err := store.InsertAuditEvent(ctx, event); err != nil {
			t.Fatalf("InsertAuditEvent(%s) error = %v", event.ID, err)
		}
	}

	report, err := New(store, func() time.Time { return base.Add(3 * time.Second) }).Run(ctx, base)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if report.ClosedTrades != 1 || report.LosingTrades != 1 {
		t.Fatalf("report = %+v, want one losing trade", report)
	}
	if got := report.Trades[0].RealizedPnL.String(); got != "-20" {
		t.Fatalf("RealizedPnL = %q, want -20", got)
	}
	if report.Trades[0].Signal == nil || report.Trades[0].Signal.Confidence.String() != "0.9" {
		t.Fatalf("Signal = %+v, want confidence 0.9", report.Trades[0].Signal)
	}
}

func TestRunNilReaderFails(t *testing.T) {
	if _, err := New(nil, nil).Run(context.Background(), time.Time{}); err == nil {
		t.Fatal("Run() error = nil, want nil reader error")
	}
}
