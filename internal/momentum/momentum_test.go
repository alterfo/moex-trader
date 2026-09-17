package momentum

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

func day(year, month, day int) time.Time {
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

func mom(values map[string]map[time.Time]float64, ticker string, date time.Time, value float64) {
	if values[ticker] == nil {
		values[ticker] = make(map[time.Time]float64)
	}
	values[ticker][date] = value
}

func TestBuildPlanSelectsTopK(t *testing.T) {
	dates := []time.Time{day(2026, 1, 5), day(2026, 1, 6), day(2026, 1, 7)}
	values := map[string]map[time.Time]float64{}
	for _, d := range dates {
		mom(values, "A", d, 4)
		mom(values, "B", d, 3)
		mom(values, "C", d, 2)
		mom(values, "D", d, 1)
		mom(values, "E", d, 0)
	}

	plan, err := BuildPlan(values, dates, 2, 1, VariantLongOnly)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Rebalances) != 3 {
		t.Fatalf("got %d rebalances, want 3", len(plan.Rebalances))
	}
	for _, rebalance := range plan.Rebalances {
		if rebalance.Directions["A"] != 1 || rebalance.Directions["B"] != 1 {
			t.Fatalf("top-2 should be A and B, got %v", rebalance.Directions)
		}
		if rebalance.Directions["C"] != 0 || rebalance.Directions["D"] != 0 || rebalance.Directions["E"] != 0 {
			t.Fatalf("non-top tickers should be flat, got %v", rebalance.Directions)
		}
	}
}

func TestBuildPlanLongShort(t *testing.T) {
	dates := []time.Time{day(2026, 1, 5)}
	values := map[string]map[time.Time]float64{}
	for _, d := range dates {
		mom(values, "A", d, 4)
		mom(values, "B", d, 3)
		mom(values, "C", d, 2)
		mom(values, "D", d, 1)
		mom(values, "E", d, 0)
	}

	plan, err := BuildPlan(values, dates, 2, 1, VariantLongShort)
	if err != nil {
		t.Fatal(err)
	}
	dirs := plan.Rebalances[0].Directions
	if dirs["A"] != 1 || dirs["B"] != 1 {
		t.Fatalf("top-2 should be long, got %v", dirs)
	}
	if dirs["E"] != -1 || dirs["D"] != -1 {
		t.Fatalf("bottom-2 should be short, got %v", dirs)
	}
	if dirs["C"] != 0 {
		t.Fatalf("middle ticker should be flat, got %v", dirs)
	}
}

func TestBuildPlanRebalanceCadence(t *testing.T) {
	var dates []time.Time
	for i := 0; i < 10; i++ {
		dates = append(dates, day(2026, 1, 1).AddDate(0, 0, i))
	}
	values := map[string]map[time.Time]float64{}
	for _, d := range dates {
		mom(values, "A", d, 1)
		mom(values, "B", d, 0)
	}

	plan, err := BuildPlan(values, dates, 1, 3, VariantLongOnly)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Rebalances) != 4 {
		t.Fatalf("rebalanceEvery=3 over 10 days should yield 4 rebalances (0,3,6,9), got %d", len(plan.Rebalances))
	}
	wantDates := []time.Time{dates[0], dates[3], dates[6], dates[9]}
	for i, rebalance := range plan.Rebalances {
		if !rebalance.Date.Equal(wantDates[i]) {
			t.Fatalf("rebalance %d date = %s, want %s", i, rebalance.Date, wantDates[i])
		}
	}
}

func TestBuildPlanTieBreakDeterministic(t *testing.T) {
	dates := []time.Time{day(2026, 1, 5)}
	values := map[string]map[time.Time]float64{}
	for _, d := range dates {
		mom(values, "B", d, 1)
		mom(values, "A", d, 1)
		mom(values, "C", d, 0)
	}

	plan, err := BuildPlan(values, dates, 1, 1, VariantLongOnly)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Rebalances[0].Directions["A"] != 1 {
		t.Fatalf("tie should be broken alphabetically (A before B), got %v", plan.Rebalances[0].Directions)
	}
	if plan.Rebalances[0].Directions["B"] != 0 {
		t.Fatalf("B should not be selected when A wins the tie, got %v", plan.Rebalances[0].Directions)
	}
}

func TestBuildPlanValidation(t *testing.T) {
	dates := []time.Time{day(2026, 1, 5)}
	values := map[string]map[time.Time]float64{}
	mom(values, "A", dates[0], 1)

	if _, err := BuildPlan(values, dates, 0, 1, VariantLongOnly); err == nil {
		t.Fatal("expected error for k <= 0")
	}
	if _, err := BuildPlan(values, dates, 1, 0, VariantLongOnly); err == nil {
		t.Fatal("expected error for rebalanceEvery <= 0")
	}
	if _, err := BuildPlan(values, dates, 1, 1, Variant(99)); err == nil {
		t.Fatal("expected error for unknown variant")
	}
}

func TestSignalSourceTargetPositionPersistsBetweenRebalances(t *testing.T) {
	d0 := day(2026, 1, 5)
	d5 := day(2026, 1, 12)
	d10 := day(2026, 1, 19)
	values := map[string]map[time.Time]float64{}
	mom(values, "A", d0, 2)
	mom(values, "B", d0, 1)
	mom(values, "A", d10, 1)
	mom(values, "B", d10, 2)

	plan, err := BuildPlan(values, []time.Time{d0, d10}, 1, 1, VariantLongOnly)
	if err != nil {
		t.Fatal(err)
	}

	src := &SignalSource{Plan: plan, TargetNotional: decimal.NewFromInt(1000), MaxLots: 10}
	feature := func(ticker string, at time.Time) domain.FeatureContext {
		return domain.FeatureContext{
			Ticker:      ticker,
			GeneratedAt: at,
			LastPrice:   decimal.NewFromInt(100),
		}
	}

	// At d0 A is selected (value 2 > 1), so it is long.
	signed, ok := src.TargetPosition(feature("A", d0))
	if !ok || signed != 10 {
		t.Fatalf("A at d0 = %d/%v, want 10/true (1000 notional / 100 price)", signed, ok)
	}
	// Between rebalances the selection persists.
	signed, ok = src.TargetPosition(feature("A", d5))
	if !ok || signed != 10 {
		t.Fatalf("A at d5 should persist long, got %d/%v", signed, ok)
	}
	// At d10 B is selected, A drops out -> flat.
	signed, ok = src.TargetPosition(feature("A", d10))
	if !ok || signed != 0 {
		t.Fatalf("A at d10 should be flat, got %d/%v", signed, ok)
	}
	signed, ok = src.TargetPosition(feature("B", d10))
	if !ok || signed != 10 {
		t.Fatalf("B at d10 should be long, got %d/%v", signed, ok)
	}
}

func TestSignalSourceGenerateActions(t *testing.T) {
	d0 := day(2026, 1, 5)
	values := map[string]map[time.Time]float64{}
	mom(values, "A", d0, 2)
	mom(values, "B", d0, 1)
	mom(values, "C", d0, 0)

	plan, err := BuildPlan(values, []time.Time{d0}, 1, 1, VariantLongShort)
	if err != nil {
		t.Fatal(err)
	}
	src := &SignalSource{Plan: plan, TargetNotional: decimal.NewFromInt(1000), MaxLots: 10}

	long, err := src.Generate(context.Background(), domain.FeatureContext{Ticker: "A", GeneratedAt: d0, LastPrice: decimal.NewFromInt(100)})
	if err != nil {
		t.Fatal(err)
	}
	if long.Action != domain.ActionBuy || long.TargetLots != 10 {
		t.Fatalf("A should BUY 10 lots, got %s/%d", long.Action, long.TargetLots)
	}

	short, err := src.Generate(context.Background(), domain.FeatureContext{Ticker: "C", GeneratedAt: d0, LastPrice: decimal.NewFromInt(100)})
	if err != nil {
		t.Fatal(err)
	}
	if short.Action != domain.ActionSell || short.TargetLots != 10 {
		t.Fatalf("C should SELL 10 lots, got %s/%d", short.Action, short.TargetLots)
	}

	flat, err := src.Generate(context.Background(), domain.FeatureContext{Ticker: "B", GeneratedAt: d0, LastPrice: decimal.NewFromInt(100)})
	if err != nil {
		t.Fatal(err)
	}
	if flat.Action != domain.ActionHold || flat.TargetLots != 0 {
		t.Fatalf("B should HOLD 0 lots, got %s/%d", flat.Action, flat.TargetLots)
	}
}

func TestSignalSourceSizingRespectsLotSize(t *testing.T) {
	d0 := day(2026, 1, 5)
	values := map[string]map[time.Time]float64{}
	mom(values, "A", d0, 1)
	plan, err := BuildPlan(values, []time.Time{d0}, 1, 1, VariantLongOnly)
	if err != nil {
		t.Fatal(err)
	}
	src := &SignalSource{Plan: plan, TargetNotional: decimal.NewFromInt(15000), MaxLots: 1000}

	signed, _ := src.TargetPosition(domain.FeatureContext{
		Ticker:      "A",
		GeneratedAt: d0,
		LastPrice:   decimal.NewFromInt(150),
		LotSize:     decimal.NewFromInt(10),
	})
	if signed != 10 {
		t.Fatalf("15000 / (150*10) = 10 lots, got %d", signed)
	}
}

func TestSignalSourceMaxLotsFallback(t *testing.T) {
	d0 := day(2026, 1, 5)
	values := map[string]map[time.Time]float64{}
	mom(values, "A", d0, 1)
	plan, err := BuildPlan(values, []time.Time{d0}, 1, 1, VariantLongOnly)
	if err != nil {
		t.Fatal(err)
	}
	src := &SignalSource{Plan: plan, MaxLots: 7}

	signed, _ := src.TargetPosition(domain.FeatureContext{Ticker: "A", GeneratedAt: d0, LastPrice: decimal.Zero})
	if signed != 7 {
		t.Fatalf("without notional and a non-positive price, maxLots should be used, got %d", signed)
	}
}
