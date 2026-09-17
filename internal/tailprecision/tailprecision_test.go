package tailprecision

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
	"time"
)

func buy(ticker string, day int, ret float64) Decision {
	return Decision{
		Ticker:           ticker,
		Date:             time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, day),
		Direction:        DirectionBuy,
		Probability:      0.75,
		ForwardReturnPct: ret,
	}
}

func sell(ticker string, day int, ret float64) Decision {
	return Decision{
		Ticker:           ticker,
		Date:             time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, day),
		Direction:        DirectionSell,
		Probability:      0.25,
		ForwardReturnPct: ret,
	}
}

func TestComputeBaseRate(t *testing.T) {
	rate := ComputeBaseRate([]float64{0.6, -0.6, 0.1, 0.9, -0.2}, 0.5)
	if rate.Samples != 5 {
		t.Fatalf("Samples = %d, want 5", rate.Samples)
	}
	if math.Abs(rate.Up-0.4) > 1e-9 {
		t.Errorf("Up = %.4f, want 0.4", rate.Up)
	}
	if math.Abs(rate.Down-0.2) > 1e-9 {
		t.Errorf("Down = %.4f, want 0.2", rate.Down)
	}
}

func TestComputeBaseRateEmpty(t *testing.T) {
	rate := ComputeBaseRate(nil, 0.5)
	if rate.Samples != 0 || rate.Up != 0 || rate.Down != 0 {
		t.Fatalf("empty base rate = %+v, want zeros", rate)
	}
}

func TestOutcomeInDirection(t *testing.T) {
	cases := []struct {
		name string
		d    Decision
		want bool
	}{
		{"buy up", buy("T", 0, 1.0), true},
		{"buy deadband", buy("T", 0, 0.5), false},
		{"buy down", buy("T", 0, -1.0), false},
		{"sell down", sell("T", 0, -1.0), true},
		{"sell deadband", sell("T", 0, -0.5), false},
		{"sell up", sell("T", 0, 1.0), false},
		{"hold never wins", Decision{Direction: DirectionHold, ForwardReturnPct: 10}, false},
	}
	for _, tc := range cases {
		if got := OutcomeInDirection(tc.d, 0.5); got != tc.want {
			t.Errorf("%s: OutcomeInDirection = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestPooledPrecision(t *testing.T) {
	decisions := []Decision{
		buy("T", 0, 1.0),
		buy("T", 1, -1.0),
		sell("T", 2, -1.0),
		sell("T", 3, 1.0),
	}
	precision, wins := PooledPrecision(decisions, 0.5)
	if wins != 2 {
		t.Fatalf("wins = %d, want 2", wins)
	}
	if math.Abs(precision-0.5) > 1e-9 {
		t.Errorf("precision = %.4f, want 0.5", precision)
	}
}

func TestPooledBaseRate(t *testing.T) {
	decisions := []Decision{
		buy("A", 0, 0),
		buy("B", 0, 0),
		sell("C", 0, 0),
	}
	rate := PooledBaseRate(decisions, BaseRate{Up: 0.6, Down: 0.2, Samples: 100})
	want := (2*0.6 + 1*0.2) / 3
	if math.Abs(rate-want) > 1e-9 {
		t.Errorf("PooledBaseRate = %.6f, want %.6f", rate, want)
	}
}

func TestEpisodesGroupsByTickerAndDirection(t *testing.T) {
	decisions := []Decision{
		buy("B", 1, 1),
		buy("A", 0, 1),
		buy("A", 1, 1),
		sell("A", 2, -1),
		buy("A", 3, 1),
		sell("B", 0, -1),
	}
	episodes := Episodes(decisions)
	if len(episodes) != 5 {
		t.Fatalf("len(episodes) = %d, want 5", len(episodes))
	}
	if len(episodes[0]) != 2 || episodes[0][0].Ticker != "A" || episodes[0][0].Direction != DirectionBuy {
		t.Errorf("episode 0 wrong: %+v", episodes[0])
	}
	if len(episodes[1]) != 1 || episodes[1][0].Direction != DirectionSell {
		t.Errorf("episode 1 wrong: %+v", episodes[1])
	}
	if len(episodes[2]) != 1 || episodes[2][0].Direction != DirectionBuy {
		t.Errorf("episode 2 wrong: %+v", episodes[2])
	}
	if episodes[3][0].Ticker != "B" || episodes[3][0].Direction != DirectionSell {
		t.Errorf("episode 3 wrong: %+v", episodes[3])
	}
	if episodes[4][0].Ticker != "B" || episodes[4][0].Direction != DirectionBuy {
		t.Errorf("episode 4 wrong: %+v", episodes[4])
	}
}

func nullDecisions() []Decision {
	var out []Decision
	for e := 0; e < 20; e++ {
		ret := 1.0
		if e%2 == 1 {
			ret = -1.0
		}
		for d := 0; d < 4; d++ {
			out = append(out, buy(fmt.Sprintf("T%d", e), d, ret))
		}
	}
	return out
}

func skillDecisions() []Decision {
	var out []Decision
	for e := 0; e < 30; e++ {
		for d := 0; d < 4; d++ {
			out = append(out, buy(fmt.Sprintf("W%d", e), d, 1.0))
		}
	}
	for e := 0; e < 10; e++ {
		for d := 0; d < 4; d++ {
			out = append(out, buy(fmt.Sprintf("L%d", e), d, -1.0))
		}
	}
	return out
}

func TestBootstrapPValueNullIsNotSignificant(t *testing.T) {
	decisions := nullDecisions()
	precision, _ := PooledPrecision(decisions, 0.5)
	if math.Abs(precision-0.5) > 1e-9 {
		t.Fatalf("null fixture precision = %.4f, want 0.5", precision)
	}
	res := BootstrapPValue(decisions, 0.5, 0.5, rand.New(rand.NewSource(7)), 2000)
	if res.PValue < 0.2 || res.PValue > 0.8 {
		t.Errorf("null p-value = %.4f, want ~0.5 (0.2..0.8)", res.PValue)
	}
}

func TestBootstrapPValueSkillIsSignificant(t *testing.T) {
	decisions := skillDecisions()
	precision, _ := PooledPrecision(decisions, 0.5)
	if math.Abs(precision-0.75) > 1e-9 {
		t.Fatalf("skill fixture precision = %.4f, want 0.75", precision)
	}
	res := BootstrapPValue(decisions, 0.5, 0.5, rand.New(rand.NewSource(7)), 5000)
	if res.PValue >= 0.05 {
		t.Errorf("skill p-value = %.4f, want < 0.05", res.PValue)
	}
}

func TestBootstrapPValueDeterministic(t *testing.T) {
	decisions := skillDecisions()
	a := BootstrapPValue(decisions, 0.5, 0.5, rand.New(rand.NewSource(42)), 1000)
	b := BootstrapPValue(decisions, 0.5, 0.5, rand.New(rand.NewSource(42)), 1000)
	if a.PValue != b.PValue || a.BootstrapMean != b.BootstrapMean || a.BootstrapSD != b.BootstrapSD {
		t.Errorf("bootstrap not deterministic for same seed: %+v vs %+v", a, b)
	}
}

func TestBootstrapPValueEmptyDecisions(t *testing.T) {
	res := BootstrapPValue(nil, 0.5, 0.5, rand.New(rand.NewSource(1)), 100)
	if res.PValue != 1 {
		t.Errorf("empty decisions p-value = %v, want 1", res.PValue)
	}
}
