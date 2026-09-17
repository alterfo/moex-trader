package spread

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

func TestHalfSpreadPct(t *testing.T) {
	tests := []struct {
		name   string
		bid    string
		ask    string
		want   string
		wantOK bool
	}{
		{name: "simple", bid: "100", ask: "102", want: "0.00990099009900990099", wantOK: true},
		{name: "one tick", bid: "270.1", ask: "270.3", want: "0.00037009622501850481", wantOK: true},
		{name: "crossed book", bid: "102", ask: "100", wantOK: false},
		{name: "zero bid", bid: "0", ask: "102", wantOK: false},
		{name: "zero ask", bid: "100", ask: "0", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := HalfSpreadPct(decimal.RequireFromString(tt.bid), decimal.RequireFromString(tt.ask))
			if ok != tt.wantOK {
				t.Fatalf("HalfSpreadPct() ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			want := decimal.RequireFromString(tt.want)
			if got.Sub(want).Abs().GreaterThan(decimal.RequireFromString("0.000000000000001")) {
				t.Fatalf("HalfSpreadPct() = %s, want %s", got, want)
			}
		})
	}
}

func TestFromAuditEventsUsesMedianPerTicker(t *testing.T) {
	events := []domain.AuditEvent{
		{ID: "1", Ticker: "sber", Stage: StageIngest, Payload: `{"bid":"100","ask":"104"}`, CreatedAt: time.Now()},
		{ID: "2", Ticker: "SBER", Stage: StageIngest, Payload: `{"bid":"100","ask":"102"}`, CreatedAt: time.Now()},
		{ID: "3", Ticker: "SBER", Stage: StageIngest, Payload: `{"bid":"100","ask":"106"}`, CreatedAt: time.Now()},
		{ID: "4", Ticker: "OZON", Stage: StageIngest, Payload: `{"bid":"200","ask":"202"}`, CreatedAt: time.Now()},
		{ID: "5", Ticker: "VTBR", Stage: "executor", Payload: `{"bid":"100","ask":"102"}`, CreatedAt: time.Now()},
		{ID: "6", Ticker: "BROKEN", Stage: StageIngest, Payload: `{"bid":"0","ask":"0"}`, CreatedAt: time.Now()},
	}

	table, err := FromAuditEvents(events, Options{})
	if err != nil {
		t.Fatalf("FromAuditEvents() error = %v", err)
	}
	if len(table) != 2 {
		t.Fatalf("FromAuditEvents() returned %d tickers, want 2", len(table))
	}

	got, ok := table.Lookup("sber")
	if !ok {
		t.Fatal("expected SBER in spread table")
	}
	want := decimal.RequireFromString("0.01960784313725490196")
	if got.Sub(want).Abs().GreaterThan(decimal.RequireFromString("0.000000000000001")) {
		t.Fatalf("SBER median half-spread = %s, want %s", got, want)
	}

	got, ok = table.Lookup("OZON")
	if !ok {
		t.Fatal("expected OZON in spread table")
	}
	want = decimal.RequireFromString("0.00497512437810945274")
	if got.Sub(want).Abs().GreaterThan(decimal.RequireFromString("0.000000000000001")) {
		t.Fatalf("OZON median half-spread = %s, want %s", got, want)
	}
}

func TestFromAuditEventsMinObservations(t *testing.T) {
	events := []domain.AuditEvent{
		{ID: "1", Ticker: "SBER", Stage: StageIngest, Payload: `{"bid":"100","ask":"102"}`, CreatedAt: time.Now()},
		{ID: "2", Ticker: "OZON", Stage: StageIngest, Payload: `{"bid":"200","ask":"202"}`, CreatedAt: time.Now()},
		{ID: "3", Ticker: "OZON", Stage: StageIngest, Payload: `{"bid":"200","ask":"204"}`, CreatedAt: time.Now()},
	}

	table, err := FromAuditEvents(events, Options{MinObservations: 2})
	if err != nil {
		t.Fatalf("FromAuditEvents() error = %v", err)
	}
	if _, ok := table.Lookup("SBER"); ok {
		t.Fatal("SBER has one observation, should be excluded with MinObservations=2")
	}
	if _, ok := table.Lookup("OZON"); !ok {
		t.Fatal("OZON has two observations, should be present")
	}
}

func TestTableResolveFallsBack(t *testing.T) {
	table := Table{"SBER": decimal.RequireFromString("0.0005")}
	fallback := decimal.RequireFromString("0.001")

	if got := table.Resolve("sber", fallback); !got.Equal(decimal.RequireFromString("0.0005")) {
		t.Fatalf("Resolve(SBER) = %s, want per-ticker value", got)
	}
	if got := table.Resolve("OZON", fallback); !got.Equal(fallback) {
		t.Fatalf("Resolve(OZON) = %s, want fallback", got)
	}
}
