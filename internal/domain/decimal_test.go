package domain

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestMedianDecimal(t *testing.T) {
	tests := []struct {
		name   string
		values []decimal.Decimal
		want   string
	}{
		{name: "empty", values: nil, want: "0"},
		{name: "odd", values: decs("3", "1", "2"), want: "2"},
		{name: "even", values: decs("3", "1", "2", "4"), want: "2.5"},
		{name: "single", values: decs("7"), want: "7"},
		{name: "with negatives", values: decs("-2", "1", "5"), want: "1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MedianDecimal(tt.values)
			if !got.Equal(decimal.RequireFromString(tt.want)) {
				t.Fatalf("median = %s, want %s", got.String(), tt.want)
			}
		})
	}
}

func TestMedianDecimalDoesNotMutateInput(t *testing.T) {
	values := decs("3", "1", "2")
	MedianDecimal(values)
	if !values[0].Equal(decimal.RequireFromString("3")) || !values[1].Equal(decimal.RequireFromString("1")) {
		t.Fatalf("input mutated: %v", values)
	}
}

func decs(values ...string) []decimal.Decimal {
	out := make([]decimal.Decimal, len(values))
	for i, v := range values {
		out[i] = decimal.RequireFromString(v)
	}
	return out
}
