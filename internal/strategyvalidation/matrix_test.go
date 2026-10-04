package strategyvalidation

import (
	"math"
	"strings"
	"testing"
	"time"
)

func dt(year int, month int, day int) time.Time {
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

func TestJoinDailyReturnsFillsGaps(t *testing.T) {
	a := []DailyReturn{
		{Date: dt(2025, 4, 1), Value: 10},
		{Date: dt(2025, 4, 2), Value: 20},
		{Date: dt(2025, 4, 3), Value: 30},
	}
	b := []DailyReturn{
		{Date: dt(2025, 4, 1), Value: -5},
		{Date: dt(2025, 4, 3), Value: -7},
	}
	dates, matrix, err := JoinDailyReturns([][]DailyReturn{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if len(dates) != 3 {
		t.Fatalf("len(dates) = %d, want 3", len(dates))
	}
	want := [][]float64{
		{10, -5},
		{20, 0},
		{30, -7},
	}
	for row := range want {
		for col := range want[row] {
			if matrix[row][col] != want[row][col] {
				t.Fatalf("matrix[%d][%d] = %v, want %v", row, col, matrix[row][col], want[row][col])
			}
		}
	}
}

func TestJoinDailyReturnsMismatchedCalendarsError(t *testing.T) {
	a := []DailyReturn{
		{Date: dt(2025, 4, 1), Value: 1},
		{Date: dt(2025, 4, 2), Value: 2},
	}
	b := []DailyReturn{
		{Date: dt(2025, 5, 1), Value: 3},
		{Date: dt(2025, 5, 2), Value: 4},
	}
	if _, _, err := JoinDailyReturns([][]DailyReturn{a, b}); err == nil || !strings.Contains(err.Error(), "mismatched calendars") {
		t.Fatalf("JoinDailyReturns error = %v, want mismatched calendars", err)
	}
}

func TestJoinDailyReturnsRejectsInsufficientOrEmptyVariants(t *testing.T) {
	if _, _, err := JoinDailyReturns(nil); err == nil {
		t.Fatal("expected error for nil variants")
	}
	one := []DailyReturn{{Date: dt(2025, 4, 1), Value: 1}}
	if _, _, err := JoinDailyReturns([][]DailyReturn{one}); err == nil {
		t.Fatal("expected error for a single variant")
	}
	if _, _, err := JoinDailyReturns([][]DailyReturn{nil, one}); err == nil {
		t.Fatal("expected error for an empty variant")
	}
}

func TestJoinDailyReturnsRejectsDuplicateDate(t *testing.T) {
	a := []DailyReturn{
		{Date: dt(2025, 4, 1), Value: 1},
		{Date: dt(2025, 4, 1), Value: 2},
	}
	b := []DailyReturn{{Date: dt(2025, 4, 1), Value: 3}}
	if _, _, err := JoinDailyReturns([][]DailyReturn{a, b}); err == nil || !strings.Contains(err.Error(), "duplicate date") {
		t.Fatalf("JoinDailyReturns error = %v, want duplicate date", err)
	}
}

func TestJoinDailyReturnsDominantMatrixHasLowPBO(t *testing.T) {
	rows := 30
	dominant := make([]DailyReturn, rows)
	noise := make([]DailyReturn, rows)
	for row := 0; row < rows; row++ {
		date := dt(2025, 1, 1).AddDate(0, 0, row)
		dominant[row] = DailyReturn{Date: date, Value: 0.02 + 0.01*math.Sin(float64(row)*0.7)}
		noise[row] = DailyReturn{Date: date, Value: 0.001 * math.Sin(float64(row)*3+1.7)}
	}
	_, matrix, err := JoinDailyReturns([][]DailyReturn{dominant, noise})
	if err != nil {
		t.Fatal(err)
	}
	result := ProbabilityOfBacktestOverfitting(matrix, 10)
	if result.Trials != 252 {
		t.Fatalf("Trials = %d, want 252", result.Trials)
	}
	if result.PBO >= 0.2 {
		t.Fatalf("dominant variant PBO = %.4f, want < 0.2", result.PBO)
	}
}
