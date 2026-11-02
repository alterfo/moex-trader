package cryptocost

import "fmt"

type Instrument int

const (
	Spot Instrument = iota
	Perp
)

type Execution int

const (
	Taker Execution = iota
	Maker
)

type Fees struct {
	Maker float64
	Taker float64
}

type Venue struct {
	Name       string
	Spot       Fees
	Perp       Fees
	BNBDiscnt  float64
	FiatRamp   float64
	SourceDate string
}

func Binance() Venue {
	return Venue{
		Name:       "binance",
		Spot:       Fees{Maker: 0.0010, Taker: 0.0010},
		Perp:       Fees{Maker: 0.0002, Taker: 0.0005},
		BNBDiscnt:  0.10,
		FiatRamp:   0.0030,
		SourceDate: "2026-11-02",
	}
}

func Bybit() Venue {
	return Venue{
		Name:       "bybit",
		Spot:       Fees{Maker: 0.0010, Taker: 0.0010},
		Perp:       Fees{Maker: 0.0002, Taker: 0.00055},
		BNBDiscnt:  0,
		FiatRamp:   0.0030,
		SourceDate: "2026-11-02",
	}
}

func OKX() Venue {
	return Venue{
		Name:       "okx",
		Spot:       Fees{Maker: 0.0008, Taker: 0.0010},
		Perp:       Fees{Maker: 0.0002, Taker: 0.0005},
		BNBDiscnt:  0,
		FiatRamp:   0.0030,
		SourceDate: "2026-11-02",
	}
}

func Venues() []Venue {
	return []Venue{Binance(), Bybit(), OKX()}
}

func VenueByName(name string) (Venue, error) {
	for _, v := range Venues() {
		if v.Name == name {
			return v, nil
		}
	}
	return Venue{}, fmt.Errorf("unknown venue %q", name)
}

type Config struct {
	Venue              Venue
	Instrument         Instrument
	Exec               Execution
	HalfSpreadBps      float64
	ImpactBps          float64
	MakerFillProb      float64
	BorrowAPR          float64
	ExchangeHaircutAPR float64
}

func (c Config) feePerSide() float64 {
	f := c.Venue.Spot
	if c.Instrument == Perp {
		f = c.Venue.Perp
	}
	if c.Exec == Maker {
		return f.Maker
	}
	return f.Taker
}

func (c Config) FeeBpsPerSide() float64 {
	return c.feePerSide() * 1e4
}

func (c Config) CrossCostBpsPerSide() float64 {
	if c.Exec == Maker {
		cross := 1 - c.MakerFillProb
		if cross < 0 {
			cross = 0
		}
		takerFee := c.Venue.Perp.Taker
		if c.Instrument == Spot {
			takerFee = c.Venue.Spot.Taker
		}
		return cross*(takerFee*1e4) + c.HalfSpreadBps*0.5 + c.ImpactBps
	}
	return c.HalfSpreadBps + c.ImpactBps
}

func (c Config) AllInBpsPerSide() float64 {
	return c.FeeBpsPerSide() + c.CrossCostBpsPerSide()
}

func (c Config) RoundTripBps() float64 {
	return 2 * c.AllInBpsPerSide()
}

func (c Config) CarryBpsPerDay() float64 {
	return (c.BorrowAPR + c.ExchangeHaircutAPR) * 1e4 / 365
}

func (c Config) CostPerRebalanceBps(turnover float64) float64 {
	return 2 * turnover * c.AllInBpsPerSide()
}

func BreakEvenPerSideBps(grossBps, turnover float64) float64 {
	if turnover == 0 {
		return 0
	}
	return grossBps / turnover
}

func RequiredGrossPerSideBps(turnover, costBpsPerSide float64) float64 {
	return 2 * turnover * costBpsPerSide
}

func FundingBps(ratePerInterval, intervalHours, holdingHours float64) float64 {
	if intervalHours <= 0 {
		return 0
	}
	return ratePerInterval * 1e4 * (holdingHours / intervalHours)
}

func FundingAPR(ratePerInterval, intervalHours float64) float64 {
	if intervalHours <= 0 {
		return 0
	}
	return ratePerInterval * (24 / intervalHours) * 365
}

func FundingBpsPerDay(ratePerInterval, intervalHours float64) float64 {
	return FundingBps(ratePerInterval, intervalHours, 24)
}

func FiatRampBpsPerCycle(v Venue) float64 {
	return v.FiatRamp * 1e4
}
