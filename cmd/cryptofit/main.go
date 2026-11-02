package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/olegsidorkin/moex-trader/internal/cryptocost"
)

type options struct {
	mode      string
	dataDir   string
	signal    string
	lookback  int
	top       int
	quantile  float64
	rate      float64
	horizon   int
	venue     string
	exec      string
	placebo   int
	noFunding bool
	from      string
	to        string
	maxSyms   int
	out       string
}

func main() {
	var o options
	flag.StringVar(&o.mode, "mode", "costs", "costs | panel | book")
	flag.StringVar(&o.dataDir, "data", "data/crypto/binance-um", "panel directory")
	flag.StringVar(&o.signal, "signal", "mom", "mom | rev | taker | funding")
	flag.IntVar(&o.lookback, "lookback", 365, "signal lookback in daily bars")
	flag.IntVar(&o.top, "top", 100, "universe size by trailing quote volume")
	flag.Float64Var(&o.quantile, "quantile", 0.2, "long and short quantile of the ranked signal")
	flag.Float64Var(&o.rate, "rate", 0.5, "partial-adjustment trade rate per day")
	flag.IntVar(&o.horizon, "horizon", 5, "forward horizon in daily bars")
	flag.StringVar(&o.venue, "venue", "binance", "binance | bybit | okx")
	flag.StringVar(&o.exec, "exec", "taker", "taker | maker")
	flag.IntVar(&o.placebo, "placebo", 0, "within-date permutations (0 = none)")
	flag.BoolVar(&o.noFunding, "no-funding", false, "switch funding off (diagnostic only)")
	flag.StringVar(&o.from, "from", "", "first date YYYY-MM-DD")
	flag.StringVar(&o.to, "to", "", "last date YYYY-MM-DD")
	flag.IntVar(&o.maxSyms, "max-symbols", 0, "limit symbols loaded (0 = all)")
	flag.StringVar(&o.out, "out", "", "output path for the freeze manifest")
	flag.Parse()

	if err := run(o); err != nil {
		log.Fatalf("%s: %v", o.mode, err)
	}
}

func run(o options) error {
	switch o.mode {
	case "costs":
		printCostTable()
		return nil
	case "panel":
		return printPanel(o)
	case "s1":
		printS1()
		return nil
	case "battery":
		return printBattery(o)
	case "freeze":
		return freezePanel(o)
	case "book":
		return runBooks(o)
	default:
		return fmt.Errorf("unknown mode %q", o.mode)
	}
}

func printCostTable() {
	fmt.Println("cost per side and per round trip, retail tier, crypto perp (bps)")
	fmt.Println("venue  exec   tier                 fee  cross  all-in  round-trip")
	for _, v := range cryptocost.Venues() {
		for _, exec := range []cryptocost.Execution{cryptocost.Taker, cryptocost.Maker} {
			name := "taker"
			if exec == cryptocost.Maker {
				name = "maker"
			}
			for _, t := range tiers {
				c := costConfig(v, exec, t.minQuoteVol)
				fmt.Printf("%-6s %-6s %-20s %5.2f %6.2f %7.2f %10.2f\n",
					v.Name, name, tierLabel(t.minQuoteVol),
					c.FeeBpsPerSide(), c.CrossCostBpsPerSide(), c.AllInBpsPerSide(), c.RoundTripBps())
			}
		}
	}
	fmt.Println()
	fmt.Println("required gross per side (bps) to break even, by turnover per side per day")
	fmt.Println("tier                 turn=1.0  turn=0.5  turn=0.2  turn=0.05")
	for _, t := range tiers {
		c := costConfig(cryptocost.Binance(), cryptocost.Taker, t.minQuoteVol)
		cost := c.AllInBpsPerSide()
		fmt.Printf("%-20s %9.2f %9.2f %9.2f %9.2f\n", tierLabel(t.minQuoteVol),
			cryptocost.RequiredGrossPerSideBps(1.0, cost),
			cryptocost.RequiredGrossPerSideBps(0.5, cost),
			cryptocost.RequiredGrossPerSideBps(0.2, cost),
			cryptocost.RequiredGrossPerSideBps(0.05, cost))
	}
	fmt.Println()
	fmt.Println("funding cost of a held long (bps of notional) at an APR of 9%, 11% and 22%")
	for _, apr := range []float64{0.09, 0.11, 0.22} {
		rate8h := apr / (3 * 365)
		fmt.Printf("APR %4.0f%%: 1d %5.2f  5d %5.2f  10d %5.2f  40d %7.2f  365d %7.1f\n",
			apr*100,
			cryptocost.FundingBps(rate8h, 8, 24),
			cryptocost.FundingBps(rate8h, 8, 5*24),
			cryptocost.FundingBps(rate8h, 8, 10*24),
			cryptocost.FundingBps(rate8h, 8, 40*24),
			cryptocost.FundingBps(rate8h, 8, 365*24))
	}
}

func tierLabel(minQuoteVol float64) string {
	switch {
	case minQuoteVol >= 1e9:
		return "major >=1B"
	case minQuoteVol >= 1e8:
		return "large >=100M"
	case minQuoteVol >= 1e7:
		return "mid >=10M"
	default:
		return "small <10M"
	}
}

func printPanel(o options) error {
	p, err := loadPanel(o.dataDir, o.maxSyms)
	if err != nil {
		return err
	}
	if len(p.dates) == 0 {
		return fmt.Errorf("empty panel")
	}
	fmt.Printf("symbols loaded: %d\n", len(p.series))
	fmt.Printf("dates: %s .. %s (%d)\n", p.dates[0], p.dates[len(p.dates)-1], len(p.dates))
	withFunding := 0
	for _, s := range p.series {
		if len(s.funding) > 0 {
			withFunding++
		}
	}
	fmt.Printf("symbols with funding history: %d\n", withFunding)
	sample := []int{0, len(p.dates) / 4, len(p.dates) / 2, 3 * len(p.dates) / 4, len(p.dates) - 1}
	for _, di := range sample {
		d := p.dates[di]
		n := 0
		for _, s := range p.series {
			if _, ok := s.byDate[d]; ok {
				n++
			}
		}
		fmt.Printf("  %s: %d symbols with a bar\n", d, n)
	}
	delisted := 0
	last := p.dates[len(p.dates)-1]
	for _, s := range p.series {
		if s.bars[len(s.bars)-1].date < last {
			delisted++
		}
	}
	fmt.Printf("symbols whose data ends before %s: %d\n", last, delisted)
	return nil
}

func runBooks(o options) error {
	venue, err := cryptocost.VenueByName(o.venue)
	if err != nil {
		return err
	}
	exec := cryptocost.Taker
	if o.exec == "maker" {
		exec = cryptocost.Maker
	}
	p, err := loadPanel(o.dataDir, o.maxSyms)
	if err != nil {
		return err
	}
	if len(p.dates) == 0 {
		return fmt.Errorf("empty panel")
	}
	opts := bookOptions{
		lookback:  o.lookback,
		top:       o.top,
		quantile:  o.quantile,
		rate:      o.rate,
		horizon:   o.horizon,
		venue:     venue,
		exec:      exec,
		noFunding: o.noFunding,
		signal:    o.signal,
		from:      o.from,
		to:        o.to,
	}
	fmt.Printf("panel %d symbols, %s .. %s\n", len(p.series), p.dates[0], p.dates[len(p.dates)-1])
	fmt.Printf("signal %s lookback %d, top %d, quantile %.2f, rate %.2f, horizon %d, %s %s, funding %v\n\n",
		o.signal, o.lookback, o.top, o.quantile, o.rate, o.horizon, venue.Name, o.exec, !o.noFunding)
	printHeader()
	res := p.runBook(opts, "real")
	printResult(res)
	for i := 0; i < o.placebo; i++ {
		popts := opts
		popts.shuffle = true
		popts.seed = int64(i + 1)
		pr := p.runBook(popts, fmt.Sprintf("placebo-%d", i+1))
		printResult(pr)
	}
	return nil
}

func printHeader() {
	fmt.Printf("%-12s %5s %6s %6s %8s %7s %7s %8s %7s %7s %7s %7s %7s\n",
		"label", "days", "names", "IC", "gross", "cost", "fund", "net", "tNW", "sharpe", "h1", "h2", "BE/side")
}

func printResult(r result) {
	if len(r.points) == 0 {
		fmt.Printf("%-12s no book\n", r.label)
		return
	}
	names := 0.0
	for _, pt := range r.points {
		names += float64(pt.names)
	}
	names /= float64(len(r.points))
	fmt.Printf("%-12s %5d %6.0f %6.3f %8.2f %7.2f %7.2f %8.2f %7.2f %7.2f %7.2f %7.2f %7.2f\n",
		r.label, len(r.points), names, r.meanIC,
		r.meanGen, r.meanCost, r.meanFund, r.meanNet,
		r.tNet, r.sharpe, r.half1, r.half2, r.bePerSide)
}

func init() {
	log.SetFlags(0)
	log.SetOutput(os.Stderr)
}
