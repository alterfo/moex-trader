package main

import (
	"math"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

func TestProbabilityFromReasoning(t *testing.T) {
	cases := []struct {
		reasoning string
		want      float64
		ok        bool
	}{
		{"ensemble:p=0.6134", 0.6134, true},
		{"ensemble:p=0.4000", 0.4, true},
		{"csvprob:p=0.1234", 0.1234, true},
		{"no probability here", 0, false},
		{"", 0, false},
	}
	for _, tc := range cases {
		got, ok := probabilityFromReasoning(tc.reasoning)
		if ok != tc.ok {
			t.Errorf("probabilityFromReasoning(%q) ok = %v, want %v", tc.reasoning, ok, tc.ok)
			continue
		}
		if tc.ok && math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("probabilityFromReasoning(%q) = %v, want %v", tc.reasoning, got, tc.want)
		}
	}
}

func TestDropIncompleteTrailing(t *testing.T) {
	now := time.Now().UTC()
	complete := moex.Candle{
		Open: decimal.NewFromInt(100), Close: decimal.NewFromInt(101),
		Begin: now.AddDate(0, 0, -1),
		End:   now.AddDate(0, 0, -1).Add(8 * time.Hour),
	}
	today := moex.Candle{
		Open: decimal.NewFromInt(101), Close: decimal.NewFromInt(102),
		Begin: now,
		End:   now.Add(8 * time.Hour),
	}

	candles := dropIncompleteTrailing([]moex.Candle{complete, complete, today})
	if len(candles) != 2 {
		t.Fatalf("len = %d, want 2 (trailing today candle dropped)", len(candles))
	}

	candles = dropIncompleteTrailing([]moex.Candle{complete, complete})
	if len(candles) != 2 {
		t.Fatalf("len = %d, want 2 (completed candles kept)", len(candles))
	}

	if got := dropIncompleteTrailing(nil); len(got) != 0 {
		t.Fatalf("nil input len = %d, want 0", len(got))
	}
}
