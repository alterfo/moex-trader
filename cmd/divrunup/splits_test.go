package main

import (
	"math"
	"testing"
)

func TestNetOfChargesSpreadAndFallsBack(t *testing.T) {
	e := event{run10: 0.05, spread: 20}
	want := 0.05 - 2*(0.0005+0.5*20/1e4)
	if math.Abs(netOf(e)-want) > 1e-12 {
		t.Fatalf("net %v want %v", netOf(e), want)
	}
	e.spread = math.NaN()
	if math.Abs(netOf(e)-(0.05-0.003)) > 1e-12 {
		t.Fatal("missing spread must cost 0.3%")
	}
}

func TestDiffGroupsNeedsFiveEventsPerGroup(t *testing.T) {
	es := make([]splitEvent, 12)
	grp := make([]int, 12)
	for i := range es {
		es[i].net = float64(i)
		if i < 6 {
			grp[i] = -1
		} else {
			grp[i] = 1
		}
	}
	if d := diffGroups(es, grp); d != 6 {
		t.Fatalf("difference %v", d)
	}
	if !math.IsNaN(diffGroups(es[:8], grp[:8])) {
		t.Fatal("a group below five events must give NaN")
	}
}
