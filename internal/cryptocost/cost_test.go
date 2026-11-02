package cryptocost

import (
	"math"
	"testing"
)

func closeTo(t *testing.T, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Fatalf("got %.6f want %.6f", got, want)
	}
}

func TestPerpTakerRoundTrip(t *testing.T) {
	c := Config{Venue: Binance(), Instrument: Perp, Exec: Taker, HalfSpreadBps: 1, ImpactBps: 1}
	closeTo(t, c.FeeBpsPerSide(), 5, 1e-9)
	closeTo(t, c.AllInBpsPerSide(), 7, 1e-9)
	closeTo(t, c.RoundTripBps(), 14, 1e-9)
}

func TestSpotTakerIsDearer(t *testing.T) {
	perp := Config{Venue: Binance(), Instrument: Perp, Exec: Taker}
	spot := Config{Venue: Binance(), Instrument: Spot, Exec: Taker}
	closeTo(t, spot.FeeBpsPerSide(), 10, 1e-9)
	closeTo(t, perp.FeeBpsPerSide(), 5, 1e-9)
}

func TestMakerCrossProbability(t *testing.T) {
	c := Config{Venue: Binance(), Instrument: Perp, Exec: Maker, HalfSpreadBps: 1, ImpactBps: 1, MakerFillProb: 0.7}
	closeTo(t, c.FeeBpsPerSide(), 2, 1e-9)
	closeTo(t, c.CrossCostBpsPerSide(), 0.3*5+0.5+1, 1e-9)
}

func TestMakerFullFillPaysFeeAndAdverseSelection(t *testing.T) {
	c := Config{Venue: Binance(), Instrument: Perp, Exec: Maker, HalfSpreadBps: 4, ImpactBps: 0, MakerFillProb: 1}
	closeTo(t, c.AllInBpsPerSide(), 2+2, 1e-9)
	c.MakerFillProb = 0
	closeTo(t, c.AllInBpsPerSide(), 2+5+2, 1e-9)
}

func TestFundingHoldingCost(t *testing.T) {
	closeTo(t, FundingBps(0.0001, 8, 240), 30, 1e-9)
	closeTo(t, FundingBps(0.0001, 8, 24), 3, 1e-9)
	closeTo(t, FundingBps(-0.0002, 8, 96), -24, 1e-9)
	closeTo(t, FundingBps(0.0001, 0, 24), 0, 1e-9)
}

func TestFundingAnnualisation(t *testing.T) {
	closeTo(t, FundingAPR(0.0001, 8), 0.1095, 1e-9)
	closeTo(t, FundingAPR(0.0001, 4), 0.2190, 1e-9)
	closeTo(t, FundingAPR(0.0001, 1), 0.8760, 1e-9)
}

func TestBreakEvenAndRequiredGross(t *testing.T) {
	closeTo(t, BreakEvenPerSideBps(5, 0.5), 10, 1e-9)
	closeTo(t, BreakEvenPerSideBps(5, 0), 0, 1e-9)
	closeTo(t, RequiredGrossPerSideBps(0.5, 7), 7, 1e-9)
}

func TestCostPerRebalance(t *testing.T) {
	c := Config{Venue: Binance(), Instrument: Perp, Exec: Taker, HalfSpreadBps: 1, ImpactBps: 1}
	closeTo(t, c.CostPerRebalanceBps(1), 14, 1e-9)
	closeTo(t, c.CostPerRebalanceBps(0.1), 1.4, 1e-9)
}

func TestCarry(t *testing.T) {
	c := Config{Venue: Binance(), Instrument: Perp, BorrowAPR: 0.11, ExchangeHaircutAPR: 0.05}
	closeTo(t, c.CarryBpsPerDay(), (0.16*1e4)/365, 1e-9)
}

func TestVenueNames(t *testing.T) {
	for _, name := range []string{"binance", "bybit", "okx"} {
		if _, err := VenueByName(name); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, err := VenueByName("ftx"); err == nil {
		t.Fatal("unknown venue accepted")
	}
	if Bybit().Perp.Taker <= Binance().Perp.Maker {
		t.Fatal("fee sanity")
	}
}
