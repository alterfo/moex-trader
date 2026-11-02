package main

import (
	"math"
	"testing"

	"github.com/olegsidorkin/moex-trader/internal/cryptocost"
)

func buildPanel(symbols, days int, mu func(i int) float64) *panel {
	p := &panel{index: map[string]int{}}
	dates := make([]string, days)
	for d := 0; d < days; d++ {
		dates[d] = civilFromDays(19000 + d)
	}
	p.dates = dates
	for i, dt := range dates {
		p.index[dt] = i
	}
	for i := 0; i < symbols; i++ {
		s := &series{symbol: "S" + string(rune('A'+i%26)) + string(rune('a'+i/26)), byDate: map[string]int{}, funding: map[string]float64{}}
		price := 100.0
		for d, dt := range dates {
			price *= 1 + mu(i)
			s.bars = append(s.bars, bar{date: dt, open: price, close: price, quoteVol: 2e9})
			s.byDate[dt] = d
			s.funding[dt] = 1.0
		}
		p.series = append(p.series, s)
	}
	return p
}

func TestMomentumBookPicksTheDrift(t *testing.T) {
	p := buildPanel(40, 40, func(i int) float64 { return (float64(i) - 20) * 1e-4 })
	o := bookOptions{
		lookback: 5, top: 40, quantile: 0.2, rate: 1, horizon: 1,
		venue: cryptocost.Binance(), exec: cryptocost.Taker, signal: "mom", noFunding: true,
	}
	r := p.runBook(o, "test")
	if len(r.points) == 0 {
		t.Fatal("no points")
	}
	if r.meanGen <= 0 {
		t.Fatalf("gross should be positive, got %.4f", r.meanGen)
	}
	if r.meanIC <= 0 {
		t.Fatalf("IC should be positive, got %.4f", r.meanIC)
	}
	if r.meanCost <= 0 {
		t.Fatalf("cost should be charged, got %.4f", r.meanCost)
	}
}

func TestFundingIsNeutralForADollarNeutralBook(t *testing.T) {
	p := buildPanel(40, 40, func(i int) float64 { return (float64(i) - 20) * 1e-4 })
	base := bookOptions{lookback: 5, top: 40, quantile: 0.2, rate: 1, horizon: 1,
		venue: cryptocost.Binance(), exec: cryptocost.Taker, signal: "mom"}
	withFunding := p.runBook(base, "with")
	base.noFunding = true
	without := p.runBook(base, "without")
	if math.Abs(withFunding.meanFund) > 0.05 {
		t.Fatalf("funding should net out on a balanced book: %.4f", withFunding.meanFund)
	}
	if math.Abs(withFunding.meanNet-without.meanNet) > 0.05 {
		t.Fatalf("net moved by funding: %.4f vs %.4f", withFunding.meanNet, without.meanNet)
	}
}

func TestFundingSignFollowsTheTilt(t *testing.T) {
	p := buildPanel(40, 40, func(i int) float64 { return (float64(i) - 20) * 1e-4 })
	for i, s := range p.series {
		for d := range s.funding {
			if i >= 20 {
				s.funding[d] = 5
			} else {
				s.funding[d] = 0
			}
		}
	}
	o := bookOptions{lookback: 5, top: 40, quantile: 0.2, rate: 1, horizon: 1,
		venue: cryptocost.Binance(), exec: cryptocost.Taker, signal: "mom"}
	r := p.runBook(o, "tilt")
	if r.meanFund >= 0 {
		t.Fatalf("long legs with the higher funding rate must cost: %.4f", r.meanFund)
	}
}

func TestPlaceboIsWorseThanReal(t *testing.T) {
	p := buildPanel(40, 60, func(i int) float64 { return (float64(i) - 20) * 1e-4 })
	o := bookOptions{lookback: 5, top: 40, quantile: 0.2, rate: 1, horizon: 1,
		venue: cryptocost.Binance(), exec: cryptocost.Taker, signal: "mom", noFunding: true}
	real := p.runBook(o, "real")
	o.shuffle = true
	o.seed = 7
	placebo := p.runBook(o, "placebo")
	if placebo.meanGen >= real.meanGen {
		t.Fatalf("permutation kept the signal: %.4f vs %.4f", placebo.meanGen, real.meanGen)
	}
}

func TestRankNormalise(t *testing.T) {
	r := rankNormalise([]float64{3, 1, 2, 4})
	want := []float64{1.0 / 6, -0.5, -1.0 / 6, 0.5}
	for i := range r {
		if math.Abs(r[i]-want[i]) > 1e-9 {
			t.Fatalf("rank %d: got %.6f want %.6f", i, r[i], want[i])
		}
	}
}

func TestRankCorrPerfect(t *testing.T) {
	x := []float64{1, 2, 3, 4, 5}
	y := []float64{10, 20, 30, 40, 50}
	if c := rankCorr(x, y); math.Abs(c-1) > 1e-9 {
		t.Fatalf("got %.6f", c)
	}
	if c := rankCorr(x, []float64{5, 4, 3, 2, 1}); math.Abs(c+1) > 1e-9 {
		t.Fatalf("got %.6f", c)
	}
}

func TestTierAssignment(t *testing.T) {
	if got := tierFor(5e9); got.halfSpread != 0.5 {
		t.Fatalf("major: %+v", got)
	}
	if got := tierFor(5e6); got.halfSpread != 10 {
		t.Fatalf("small: %+v", got)
	}
	cfg := costConfig(cryptocost.Binance(), cryptocost.Taker, 2e9)
	if math.Abs(cfg.AllInBpsPerSide()-6.5) > 1e-9 {
		t.Fatalf("major taker all-in %.4f", cfg.AllInBpsPerSide())
	}
}

func TestNWT(t *testing.T) {
	v := make([]float64, 200)
	for i := range v {
		v[i] = 1 + 0.1*math.Sin(float64(i))
	}
	if tval := nwT(v, 5); tval < 10 {
		t.Fatalf("strong mean should be significant, got %.3f", tval)
	}
}
