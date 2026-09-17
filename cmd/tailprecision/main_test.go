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
	complete := moex.Candle{
		Open: decimal.NewFromInt(100), Close: decimal.NewFromInt(101),
		Begin: time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 9, 16, 23, 59, 55, 0, time.UTC),
	}
	partial := moex.Candle{
		Open: decimal.NewFromInt(101), Close: decimal.NewFromInt(102),
		Begin: time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 9, 17, 15, 19, 4, 0, time.UTC),
	}

	candles := dropIncompleteTrailing([]moex.Candle{complete, complete, partial})
	if len(candles) != 2 {
		t.Fatalf("len = %d, want 2 (trailing partial dropped)", len(candles))
	}

	candles = dropIncompleteTrailing([]moex.Candle{complete, complete})
	if len(candles) != 2 {
		t.Fatalf("len = %d, want 2 (completed candles kept)", len(candles))
	}

	if got := dropIncompleteTrailing(nil); len(got) != 0 {
		t.Fatalf("nil input len = %d, want 0", len(got))
	}
}
