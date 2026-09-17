package borrowcost

import (
	"testing"

	"github.com/shopspring/decimal"
	pb "github.com/tinkoff/invest-api-go-sdk/proto"
)

func TestStressRatePerDay(t *testing.T) {
	got := StressRatePerDay()
	want := decimal.RequireFromString("0.00005")
	if !got.Equal(want) {
		t.Fatalf("StressRatePerDay = %s, want %s", got, want)
	}
}

func TestDailyStressCost(t *testing.T) {
	notional := decimal.NewFromInt(15000)
	got := DailyStressCost(notional)
	want := decimal.RequireFromString("0.75")
	if !got.Equal(want) {
		t.Fatalf("DailyStressCost(15000) = %s, want %s", got, want)
	}
}

func TestDailyStressCostIgnoresNonPositiveNotional(t *testing.T) {
	if got := DailyStressCost(decimal.Zero); !got.IsZero() {
		t.Fatalf("DailyStressCost(0) = %s, want 0", got)
	}
	if got := DailyStressCost(decimal.NewFromInt(-5)); !got.IsZero() {
		t.Fatalf("DailyStressCost(-5) = %s, want 0", got)
	}
}

func TestStressCostForDays(t *testing.T) {
	got := StressCostForDays(decimal.NewFromInt(15000), 10)
	want := decimal.RequireFromString("7.5")
	if !got.Equal(want) {
		t.Fatalf("StressCostForDays(15000, 10) = %s, want %s", got, want)
	}
}

func TestStressCostForDaysIgnoresInvalidInputs(t *testing.T) {
	if got := StressCostForDays(decimal.NewFromInt(15000), 0); !got.IsZero() {
		t.Fatalf("StressCostForDays(15000, 0) = %s, want 0", got)
	}
	if got := StressCostForDays(decimal.Zero, 10); !got.IsZero() {
		t.Fatalf("StressCostForDays(0, 10) = %s, want 0", got)
	}
	if got := StressCostForDays(decimal.NewFromInt(15000), -2); !got.IsZero() {
		t.Fatalf("StressCostForDays(15000, -2) = %s, want 0", got)
	}
}

func TestResolveDailyRateUsesMeasuredRateWhenPositive(t *testing.T) {
	res := ResolveDailyRate(decimal.RequireFromString("7.5"), decimal.RequireFromString("150000"))
	if res.UsingStress || !res.Measurable {
		t.Fatalf("expected a measured rate, got %+v", res)
	}
	want := decimal.RequireFromString("0.00005")
	if !res.MeasuredRatePerDay.Equal(want) {
		t.Fatalf("MeasuredRatePerDay = %s, want %s", res.MeasuredRatePerDay, want)
	}
}

func TestResolveDailyRateFallsBackToStressWhenNotMeasurable(t *testing.T) {
	res := ResolveDailyRate(decimal.Zero, decimal.RequireFromString("150000"))
	if !res.UsingStress || res.Measurable {
		t.Fatalf("expected stress fallback when no fees observed, got %+v", res)
	}
	if !res.MeasuredRatePerDay.Equal(StressRatePerDay()) {
		t.Fatalf("MeasuredRatePerDay = %s, want stress %s", res.MeasuredRatePerDay, StressRatePerDay())
	}

	res = ResolveDailyRate(decimal.RequireFromString("1"), decimal.Zero)
	if !res.UsingStress || res.Measurable {
		t.Fatalf("expected stress fallback when no short exposure, got %+v", res)
	}
}

func TestSumMarginFees(t *testing.T) {
	operations := []*pb.Operation{
		{OperationType: pb.OperationType_OPERATION_TYPE_MARGIN_FEE, Payment: &pb.MoneyValue{Currency: "rub", Units: 1, Nano: 250000000}},
		{OperationType: pb.OperationType_OPERATION_TYPE_BROKER_FEE, Payment: &pb.MoneyValue{Currency: "rub", Units: 5, Nano: 0}},
		{OperationType: pb.OperationType_OPERATION_TYPE_MARGIN_FEE, Payment: &pb.MoneyValue{Currency: "rub", Units: 2, Nano: 750000000}},
		nil,
		{OperationType: pb.OperationType_OPERATION_TYPE_MARGIN_FEE, Payment: nil},
	}

	fees, count := SumMarginFees(operations)
	if count != 2 {
		t.Fatalf("SumMarginFees() count = %d, want 2", count)
	}
	want := decimal.RequireFromString("4")
	if !fees.Equal(want) {
		t.Fatalf("SumMarginFees() fees = %s, want %s", fees, want)
	}
}

func TestSumMarginFeesEmpty(t *testing.T) {
	fees, count := SumMarginFees(nil)
	if count != 0 || !fees.IsZero() {
		t.Fatalf("SumMarginFees(nil) = (%s, %d), want (0, 0)", fees, count)
	}
}
