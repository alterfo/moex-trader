package risk

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestTrailingVaRESKnownSeries(t *testing.T) {
	returns := make([]decimal.Decimal, 100)
	for i := range returns {
		returns[i] = decimal.NewFromInt(int64(i - 95)).Div(decimal.NewFromInt(100))
	}

	got := TrailingVaRES(returns)
	if !got.Valid {
		t.Fatal("Valid = false, want true")
	}
	want := map[string]string{
		"VaR95": "-0.01",
		"VaR99": "0.03",
		"ES95":  "-0.48",
		"ES99":  "-0.46",
	}
	for name, value := range map[string]decimal.Decimal{
		"VaR95": got.VaR95,
		"VaR99": got.VaR99,
		"ES95":  got.ES95,
		"ES99":  got.ES99,
	} {
		if !value.Equal(mustDecimal(t, want[name])) {
			t.Fatalf("%s = %s, want %s", name, value, want[name])
		}
	}
}

func TestTrailingVaRESShortHistory(t *testing.T) {
	if got := TrailingVaRES(nil); got.Valid {
		t.Fatal("nil returns should be invalid")
	}
	if got := TrailingVaRES([]decimal.Decimal{decimal.NewFromInt(1)}); got.Valid {
		t.Fatal("single return should be invalid")
	}
}

func TestTrailingVaRESUsesOnlyLast250(t *testing.T) {
	old := make([]decimal.Decimal, 260)
	for i := range old {
		old[i] = decimal.NewFromInt(1)
	}
	recent := make([]decimal.Decimal, 250)
	for i := range recent {
		recent[i] = decimal.NewFromInt(int64(i - 238)).Div(decimal.NewFromInt(100))
	}
	all := append(old, recent...)
	got := TrailingVaRES(all)
	if !got.Valid {
		t.Fatal("Valid = false, want true")
	}
	if !got.VaR95.Equal(decimal.RequireFromString("-0.01")) {
		t.Fatalf("VaR95 = %s, want -0.01", got.VaR95)
	}
}

func TestDailyReturns(t *testing.T) {
	closes := []decimal.Decimal{
		decimal.RequireFromString("100"),
		decimal.RequireFromString("110"),
		decimal.RequireFromString("99"),
	}
	got := DailyReturns(closes)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if !got[0].Equal(decimal.RequireFromString("0.1")) || !got[1].Equal(decimal.RequireFromString("-0.1")) {
		t.Fatalf("DailyReturns = %v", got)
	}
}

func TestBookVaRESEmptyBook(t *testing.T) {
	got := BookVaRES(map[string]decimal.Decimal{}, map[string][]decimal.Decimal{
		"SBER": {decimal.RequireFromString("100"), decimal.RequireFromString("101")},
	})
	if got.Valid {
		t.Fatal("empty book should be invalid")
	}
}

func TestBookVaRESAggregatesSignedNotional(t *testing.T) {
	book := map[string]decimal.Decimal{
		"SBER": decimal.NewFromInt(1000),
		"GAZP": decimal.NewFromInt(-500),
	}
	history := map[string][]decimal.Decimal{
		"SBER": {
			decimal.RequireFromString("100"),
			decimal.RequireFromString("110"),
			decimal.RequireFromString("121"),
		},
		"GAZP": {
			decimal.RequireFromString("200"),
			decimal.RequireFromString("190"),
			decimal.RequireFromString("180.5"),
		},
	}
	got := BookVaRES(book, history)
	if !got.Valid {
		t.Fatal("Valid = false, want true")
	}
	for name, value := range map[string]decimal.Decimal{
		"VaR95": got.VaR95,
		"VaR99": got.VaR99,
		"ES95":  got.ES95,
		"ES99":  got.ES99,
	} {
		if !value.Equal(decimal.RequireFromString("125")) {
			t.Fatalf("%s = %s, want 125", name, value)
		}
	}
}

func mustDecimal(t *testing.T, value string) decimal.Decimal {
	t.Helper()
	parsed, err := decimal.NewFromString(value)
	if err != nil {
		t.Fatalf("decimal.NewFromString(%q) error = %v", value, err)
	}
	return parsed
}
