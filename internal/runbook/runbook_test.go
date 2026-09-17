package runbook

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestDecideAllowsEquityAtThreshold(t *testing.T) {
	decision := Decide(decimal.NewFromInt(300), decimal.NewFromInt(200))
	if !decision.Allowed {
		t.Fatalf("Decision.Allowed = false at exactly 1.5x, want true")
	}
	if !decision.RequiredEquity.Equal(decimal.NewFromInt(300)) {
		t.Fatalf("RequiredEquity = %s, want 300", decision.RequiredEquity)
	}
}

func TestDecideRefusesEquityBelowThreshold(t *testing.T) {
	decision := Decide(decimal.NewFromInt(299), decimal.NewFromInt(200))
	if decision.Allowed {
		t.Fatalf("Decision.Allowed = true below 1.5x, want false")
	}
	if !decision.RequiredEquity.Equal(decimal.NewFromInt(300)) {
		t.Fatalf("RequiredEquity = %s, want 300", decision.RequiredEquity)
	}
}

func TestDecideAllowsNoOpenPositions(t *testing.T) {
	decision := Decide(decimal.NewFromInt(100), decimal.Zero)
	if !decision.Allowed {
		t.Fatalf("Decision.Allowed = false with zero max notional, want true")
	}
	if !decision.RequiredEquity.IsZero() {
		t.Fatalf("RequiredEquity = %s, want zero", decision.RequiredEquity)
	}
}

func TestDecideTreatsNegativeNotionalAsZero(t *testing.T) {
	decision := Decide(decimal.NewFromInt(100), decimal.NewFromInt(-200))
	if !decision.Allowed {
		t.Fatalf("Decision.Allowed = false with negative max notional, want true")
	}
}
