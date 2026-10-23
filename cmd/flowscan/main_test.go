package main

import (
	"math"
	"os"
	"testing"
)

func TestSlotMapping(t *testing.T) {
	cases := []struct {
		tm         string
		kind, slot int
	}{
		{"10:05:00", 0, 0},
		{"18:30:00", 0, 101},
		{"18:50:00", 0, 105},
		{"07:00:00", 1, 0},
		{"09:55:00", 1, 35},
		{"19:05:00", 2, 0},
		{"23:45:00", 2, 56},
		{"23:50:00", -1, -1},
		{"10:07:00", -1, -1},
	}
	for _, c := range cases {
		k, s := locate(c.tm)
		if k != c.kind || s != c.slot {
			t.Errorf("%s: got %d,%d want %d,%d", c.tm, k, s, c.kind, c.slot)
		}
	}
}

func TestBaseHandMade(t *testing.T) {
	w := &sessions{}
	for s := 0; s <= slotFeat; s++ {
		b := &w.main[s]
		b.has = [3]bool{true, true, true}
		b.f[tOpen], b.f[tHigh], b.f[tLow], b.f[tClose] = 100, 101, 99, 100
		b.f[tVal], b.f[tValB], b.f[tValS] = 1000, 700, 300
		b.f[tVol], b.f[tTrades], b.f[tTrB], b.f[tTrS] = 10, 10, 6, 4
		b.f[oImbVal] = 0.5
		b.f[oSpLv] = 20
		b.f[sPutValB], b.f[sPutValS], b.f[sCanValB], b.f[sCanValS] = 300, 100, 100, 100
	}
	r := computeBase(w)
	check := func(name string, k int, want float64) {
		if got := float64(r[k]); math.Abs(got-want) > 1e-5 {
			t.Errorf("%s: got %v want %v", name, got, want)
		}
	}
	check("A1", bA1, 0.4)
	check("A2", bA2, 0.5)
	check("A4", bA4, 0.5)
	check("A5", bA5, 0.5)
	check("spread", bSpLv, 20)
	check("close1830", bClose1830, 100)
	check("share last", bShareLast, 13.0/102.0)
	check("ticket", bTicket, 100)
}

func TestRealDay(t *testing.T) {
	if _, err := os.Stat("../../data/algopack/tradestats/2025-06-16.csv.gz"); err != nil {
		t.Skip("no data")
	}
	res, err := loadDay(loadOpts{dir: "../../data/algopack", lagDays: 30}, "2025-06-16")
	if err != nil {
		t.Fatal(err)
	}
	r := res.recs["SBER"]
	if r == nil {
		t.Fatal("no SBER")
	}
	if a2 := float64(r[bA2]); math.IsNaN(a2) || a2 < -1 || a2 > 1 {
		t.Errorf("A2 out of range: %v", a2)
	}
	if res.drop.dropped[1] != 0 {
		t.Errorf("realtime day lost obstats rows: %d", res.drop.dropped[1])
	}
}
