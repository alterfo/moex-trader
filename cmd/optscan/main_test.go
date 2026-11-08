package main

import (
	"math"
	"testing"
)

func TestParseOptionID(t *testing.T) {
	cases := []struct {
		id     string
		prefix string
		strike float64
		typ    string
		ok     bool
	}{
		{"BR100BK6", "BR", 100, "C", true},
		{"BR100BW6", "BR", 100, "P", true},
		{"BR100BK6B", "BR", 100, "C", true},
		{"BR90BV3", "BR", 90, "P", true},
		{"CR10.25BC7", "CR", 10.25, "C", true},
		{"GD3000BL6", "GD", 3000, "C", true},
		{"BRZ6", "BR", 0, "", false},
		{"SRZ6", "SR", 0, "", false},
		{"BR100", "BR", 0, "", false},
	}
	for _, c := range cases {
		strike, typ, ok := parseOptionID(c.id, c.prefix)
		if ok != c.ok || (ok && (strike != c.strike || typ != c.typ)) {
			t.Fatalf("%s: got (%v,%q,%v), want (%v,%q,%v)", c.id, strike, typ, ok, c.strike, c.typ, c.ok)
		}
	}
}

func TestBlack76RoundTrip(t *testing.T) {
	f, k, tau := 100.0, 95.0, 0.1
	for _, call := range []bool{true, false} {
		for _, sigma := range []float64{0.15, 0.4, 0.9} {
			price := black76(f, k, sigma, tau, call)
			iv, ok := impliedVol(price, f, k, tau, call)
			if !ok {
				t.Fatalf("no iv for sigma %v call %v", sigma, call)
			}
			if math.Abs(iv-sigma) > 1e-4 {
				t.Fatalf("iv %v, want %v", iv, sigma)
			}
		}
	}
}

func TestDeltaSigns(t *testing.T) {
	if d := delta(100, 100, 0.3, 0.1, true); math.Abs(d-0.55) > 0.08 {
		t.Fatalf("atm call delta %v", d)
	}
	if d := delta(100, 100, 0.3, 0.1, false); math.Abs(d+0.45) > 0.08 {
		t.Fatalf("atm put delta %v", d)
	}
}

func TestNeweyWest(t *testing.T) {
	xs := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	m, tv := neweyWest(xs, 2)
	if math.Abs(m-5.5) > 1e-9 {
		t.Fatalf("mean %v", m)
	}
	if tv <= 0 {
		t.Fatalf("t %v", tv)
	}
}
