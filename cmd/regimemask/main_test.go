package main

import "testing"

func TestRiskOffDates(t *testing.T) {
	dates := []string{"2026-01-05", "2026-01-06", "2026-01-07", "2026-01-08", "2026-01-09", "2026-01-12"}
	closes := map[string]float64{"2026-01-05": 100, "2026-01-06": 97, "2026-01-07": 98, "2026-01-08": 99, "2026-01-09": 99, "2026-01-12": 94}
	r1 := riskOffDates(closes, dates, "r1", -0.02, -0.05)
	if !r1["2026-01-06"] || r1["2026-01-07"] || !r1["2026-01-12"] {
		t.Fatalf("r1 marks: %v", r1)
	}
	r2 := riskOffDates(closes, dates, "r2", -0.02, -0.05)
	if !r2["2026-01-12"] || len(r2) != 1 {
		t.Fatalf("r2 marks: %v", r2)
	}
	if len(riskOffDates(closes, dates, "none", -0.02, -0.05)) != 0 {
		t.Fatal("none must mark nothing")
	}
}
