package main

import (
	"fmt"

	"github.com/olegsidorkin/moex-trader/internal/cryptocost"
)

type batteryCell struct {
	signal   string
	lookback int
	rate     float64
}

func batteryCells() []batteryCell {
	lookbacks := map[string]int{"mom": 365, "momfast": 30, "rev": 2, "taker": 20, "funding": 7}
	var cells []batteryCell
	for _, sig := range []string{"mom", "momfast", "rev", "taker", "funding"} {
		for _, r := range []float64{0.1, 0.33} {
			cells = append(cells, batteryCell{signal: sig, lookback: lookbacks[sig], rate: r})
		}
	}
	return cells
}

func printBattery(o options) error {
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
	cells := batteryCells()
	from := o.from
	if from == "" {
		from = "2024-01-01"
	}
	top := o.top
	if top == 0 {
		top = 100
	}
	fmt.Printf("panel %d symbols, %s .. %s, window %s .. %s, top %d, %s %s\n",
		len(p.series), p.dates[0], p.dates[len(p.dates)-1], from, o.to, top, venue.Name, o.exec)
	fmt.Printf("battery: %d cells (5 signals x 2 rates), IC label horizon %d, funding %v\n\n",
		len(cells), o.horizon, !o.noFunding)
	fmt.Printf("%-8s %-4s %-5s %6s %7s %7s %7s %7s %7s %7s %7s %6s %7s %7s\n",
		"signal", "look", "rate", "days", "IC", "gross", "cost", "fund", "net", "tNW", "sharpe", "DSR", "h1", "h2")
	for _, c := range cells {
		opts := bookOptions{
			lookback:  c.lookback,
			top:       top,
			quantile:  o.quantile,
			rate:      c.rate,
			horizon:   o.horizon,
			venue:     venue,
			exec:      exec,
			noFunding: o.noFunding,
			signal:    signalKind(c.signal),
			entry:     o.entry,
			from:      from,
			to:        o.to,
		}
		r := p.runBook(opts, c.signal)
		if len(r.points) == 0 {
			fmt.Printf("%-8s %-4d %-5.2f no book\n", c.signal, c.lookback, c.rate)
			continue
		}
		dsr, _ := deflatedSharpe(netSeries(r), len(cells))
		fmt.Printf("%-8s %-4d %-5.2f %6d %7.3f %7.2f %7.2f %7.2f %7.2f %7.2f %7.2f %6.3f %7.2f %7.2f\n",
			c.signal, c.lookback, c.rate, len(r.points),
			r.meanIC, r.meanGen, r.meanCost, r.meanFund, r.meanNet, r.tNet, r.sharpe, dsr, r.half1, r.half2)
	}
	fmt.Println()
	fmt.Printf("Bonferroni bar for %d cells at the 5%% level: |t| >= %.2f; a cell below it is not a candidate.\n",
		len(cells), bonferroniT(len(cells), 0.05))
	return nil
}

func signalKind(name string) string {
	switch name {
	case "momfast":
		return "mom"
	default:
		return name
	}
}

func netSeries(r result) []float64 {
	out := make([]float64, 0, len(r.points))
	for _, p := range r.points {
		out = append(out, p.net)
	}
	return out
}

func bonferroniT(trials int, alpha float64) float64 {
	return normalInvCDF(1 - alpha/(2*float64(trials)))
}
