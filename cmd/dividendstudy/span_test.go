package main

import (
	"testing"
	"time"
)

func spanFixture() ([]time.Time, map[time.Time]float64) {
	start := time.Date(2022, 2, 14, 0, 0, 0, 0, time.UTC)
	var days []time.Time
	closes := map[time.Time]float64{}
	for i := 0; i < 30; i++ {
		d := start.AddDate(0, 0, i)
		days = append(days, d)
		closes[d] = 100
	}
	return days, closes
}

func TestSpanRejectedByHaltWindow(t *testing.T) {
	days, closes := spanFixture()
	from := time.Date(2022, 2, 25, 0, 0, 0, 0, time.UTC)
	till := time.Date(2022, 3, 28, 0, 0, 0, 0, time.UTC)
	if !spanRejected(days, closes, 15, from, till, 0) {
		t.Fatal("span touching the halt must be rejected")
	}
	if spanRejected(days, closes, 15, time.Time{}, time.Time{}, 0) {
		t.Fatal("no filters must keep the event")
	}
}

func TestSpanRejectedByCorporateAction(t *testing.T) {
	days, closes := spanFixture()
	closes[days[12]] = 20
	if !spanRejected(days, closes, 15, time.Time{}, time.Time{}, 0.45) {
		t.Fatal("an 80% drop in the span must be rejected")
	}
	if spanRejected(days, closes, 25, time.Time{}, time.Time{}, 0.45) {
		t.Fatal("a move outside the span must not matter")
	}
	if spanRejected(days, closes, 15, time.Time{}, time.Time{}, 0) {
		t.Fatal("max-move 0 disables the guard")
	}
}

func TestEventReturnsKeepsEventsAlignedWithReturns(t *testing.T) {
	days, closes := spanFixture()
	opens := map[time.Time]float64{}
	for _, d := range days {
		opens[d] = 100
	}
	events := []event{
		{ticker: "AAA", entryIdx: 1, exitIdx: 2},
		{ticker: "AAA", entryIdx: 20, exitIdx: 21},
	}
	dayIdx := map[string][]time.Time{"AAA": days}
	closeBy := map[string]map[time.Time]float64{"AAA": closes}
	openBy := map[string]map[time.Time]float64{"AAA": opens}
	rets, imoex, _, kept := eventReturns(events, -5, dayIdx, closeBy, openBy, map[time.Time]float64{}, 0)
	if len(rets) != 1 || len(imoex) != 1 || len(kept) != 1 {
		t.Fatalf("rets %d imoex %d kept %d", len(rets), len(imoex), len(kept))
	}
	if kept[0].entryIdx != 20 {
		t.Fatalf("kept the wrong event: %+v", kept[0])
	}
}
