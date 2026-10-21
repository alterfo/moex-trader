package main

import "testing"

func TestVerdictWorks(t *testing.T) {
	got := verdict([]rateReport{
		{Borrow: "0.00005", Full: 1, WalkForward: 5, P: 0.01},
		{Borrow: "0.00047", Full: 1, WalkForward: 5, P: 0.04},
	})
	if got[0][:13] != "ticker filter" {
		t.Fatalf("got %v", got)
	}
}

func TestVerdictLimitedByRate(t *testing.T) {
	got := verdict([]rateReport{
		{Borrow: "0.00005", Full: 1, WalkForward: 5, P: 0.01},
		{Borrow: "0.00047", Full: 1, WalkForward: 5, P: 0.30},
	})
	if got[0][:7] != "limited" {
		t.Fatalf("got %v", got)
	}
}

func TestVerdictRequiresBeatingFull(t *testing.T) {
	got := verdict([]rateReport{
		{Borrow: "0.00005", Full: 9, WalkForward: 5, P: 0.01},
		{Borrow: "0.00047", Full: 9, WalkForward: 5, P: 0.01},
	})
	if got[0][:8] != "dropping" {
		t.Fatalf("got %v", got)
	}
}
