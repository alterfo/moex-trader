package filltracking

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

func TestSlippageBps(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		actual   string
		action   domain.Action
		want     string
	}{
		{name: "buy better", expected: "271.31", actual: "270.50", action: domain.ActionBuy, want: "29.855147248535"},
		{name: "buy worse", expected: "270.50", actual: "271.31", action: domain.ActionBuy, want: "-29.944547134935"},
		{name: "sell better", expected: "269.19", actual: "270.50", action: domain.ActionSell, want: "48.664512054683"},
		{name: "sell worse", expected: "270.50", actual: "269.19", action: domain.ActionSell, want: "-48.428835489834"},
		{name: "hold has no bound", expected: "270.50", actual: "270.00", action: domain.ActionHold, want: "0"},
		{name: "zero expected", expected: "0", actual: "270.00", action: domain.ActionBuy, want: "0"},
		{name: "zero actual", expected: "270.00", actual: "0", action: domain.ActionBuy, want: "0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SlippageBps(decimal.RequireFromString(tt.expected), decimal.RequireFromString(tt.actual), tt.action)
			want := decimal.RequireFromString(tt.want)
			if got.Sub(want).Abs().GreaterThan(decimal.RequireFromString("0.000000000000001")) {
				t.Fatalf("SlippageBps() = %s, want %s", got, want)
			}
		})
	}
}

func TestFromAuditEventsClassifiesFillsAndStatuses(t *testing.T) {
	events := []domain.AuditEvent{
		{ID: "1", Ticker: "sber", Stage: StageExecutor, Payload: `{"id":"1","ticker":"SBER","action":"BUY","lots":1,"price":"270.50","expected_price":"271.31"}`, CreatedAt: time.Now()},
		{ID: "2", Ticker: "sber", Stage: StageExecutor, Payload: `{"status":"rejected","ticker":"SBER","action":"BUY","target_lots":2,"price":"271.00","expected_price":"271.81"}`, CreatedAt: time.Now()},
		{ID: "3", Ticker: "gazp", Stage: StageExecutor, Payload: `{"id":"3","ticker":"GAZP","action":"SELL","lots":10,"price":"168.20","expected_price":"167.70"}`, CreatedAt: time.Now()},
		{ID: "4", Ticker: "sber", Stage: "ingest", Payload: `{"action":"BUY","lots":1,"price":"270.50"}`, CreatedAt: time.Now()},
		{ID: "5", Ticker: "sber", Stage: StageExecutor, Payload: `{"action":"HOLD","lots":0,"price":"270.50"}`, CreatedAt: time.Now()},
	}

	digest := FromAuditEvents(events, Options{MaxSlippagePct: decimal.RequireFromString("0.003")})

	if digest.FillCount() != 2 {
		t.Fatalf("FillCount() = %d, want 2", digest.FillCount())
	}
	if digest.Submitted != 3 {
		t.Fatalf("Submitted = %d, want 3", digest.Submitted)
	}
	if digest.Rejected != 1 {
		t.Fatalf("Rejected = %d, want 1", digest.Rejected)
	}
	if got := digest.Count(StatusFilled); got != 2 {
		t.Fatalf("Count(filled) = %d, want 2", got)
	}
	if got := digest.Count(StatusRejected); got != 1 {
		t.Fatalf("Count(rejected) = %d, want 1", got)
	}

	wantRate := decimal.NewFromInt(1).Div(decimal.NewFromInt(3))
	if got := digest.RejectionRate(); got.Sub(wantRate).Abs().GreaterThan(decimal.RequireFromString("0.000000000000001")) {
		t.Fatalf("RejectionRate() = %s, want %s", got, wantRate)
	}
	if !digest.MaxSlippagePct.Equal(decimal.RequireFromString("0.003")) {
		t.Fatalf("MaxSlippagePct = %s, want 0.003", digest.MaxSlippagePct)
	}
}

func TestFromAuditEventsSlippageSummary(t *testing.T) {
	events := []domain.AuditEvent{
		{ID: "1", Ticker: "SBER", Stage: StageExecutor, Payload: `{"action":"BUY","lots":1,"price":"270.50","expected_price":"271.31"}`, CreatedAt: time.Now()},
		{ID: "2", Ticker: "SBER", Stage: StageExecutor, Payload: `{"action":"BUY","lots":1,"price":"272.00","expected_price":"271.31"}`, CreatedAt: time.Now()},
		{ID: "3", Ticker: "SBER", Stage: StageExecutor, Payload: `{"action":"SELL","lots":1,"price":"168.20","expected_price":"167.70"}`, CreatedAt: time.Now()},
	}

	digest := FromAuditEvents(events, Options{})
	if digest.FillCount() != 3 {
		t.Fatalf("FillCount() = %d, want 3", digest.FillCount())
	}

	wantMean := decimal.RequireFromString("11.4127102905923333")
	if got := digest.MeanSlippageBps(); got.Sub(wantMean).Abs().GreaterThan(decimal.RequireFromString("0.00000000001")) {
		t.Fatalf("MeanSlippageBps() = %s, want %s", got, wantMean)
	}

	if got := digest.MaxAdverseSlippageBps(); got.Sign() >= 0 {
		t.Fatalf("MaxAdverseSlippageBps() = %s, want a negative value", got)
	}
	if got := digest.MaxFavorableSlippageBps(); got.Sign() <= 0 {
		t.Fatalf("MaxFavorableSlippageBps() = %s, want a positive value", got)
	}

	byTicker := digest.MeanSlippageBpsByTicker()
	if len(byTicker) != 1 {
		t.Fatalf("MeanSlippageBpsByTicker() returned %d tickers, want 1", len(byTicker))
	}
	if got := byTicker["SBER"]; got.Sub(wantMean).Abs().GreaterThan(decimal.RequireFromString("0.00000000001")) {
		t.Fatalf("MeanSlippageBpsByTicker()[SBER] = %s, want %s", got, wantMean)
	}
}

func TestFromAuditEventsEmpty(t *testing.T) {
	digest := FromAuditEvents(nil, Options{})
	if digest.FillCount() != 0 || digest.Submitted != 0 || digest.Rejected != 0 {
		t.Fatalf("empty digest = %+v, want zero counts", digest)
	}
	if !digest.RejectionRate().IsZero() {
		t.Fatalf("RejectionRate() = %s, want 0", digest.RejectionRate())
	}
	if !digest.MeanSlippageBps().IsZero() {
		t.Fatalf("MeanSlippageBps() = %s, want 0", digest.MeanSlippageBps())
	}
}
