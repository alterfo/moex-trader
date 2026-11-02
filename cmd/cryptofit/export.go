package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/cryptocost"
)

type cellJSON struct {
	Signal   string  `json:"signal"`
	Lookback int     `json:"lookback"`
	Rate     float64 `json:"rate"`
	Days     int     `json:"days"`
	IC       float64 `json:"ic"`
	Gross    float64 `json:"gross"`
	Cost     float64 `json:"cost"`
	Fund     float64 `json:"fund"`
	Net      float64 `json:"net"`
	T        float64 `json:"t"`
	Sharpe   float64 `json:"sharpe"`
	DSR      float64 `json:"dsr"`
	H1       float64 `json:"h1"`
	H2       float64 `json:"h2"`
	Turn     float64 `json:"turn"`
	AvgCost  float64 `json:"avg_cost_per_side"`
}

type tierJSON struct {
	Name      string  `json:"name"`
	Fee       float64 `json:"fee"`
	Cross     float64 `json:"cross"`
	AllIn     float64 `json:"all_in"`
	RoundTrip float64 `json:"round_trip"`
}

type fundingJSON struct {
	APR   float64 `json:"apr"`
	Day1  float64 `json:"day1"`
	Day5  float64 `json:"day5"`
	Day10 float64 `json:"day10"`
	Day40 float64 `json:"day40"`
}

type coverageJSON struct {
	Month   string `json:"month"`
	Symbols int    `json:"symbols"`
}

type variantJSON struct {
	Label  string  `json:"label"`
	Net    float64 `json:"net"`
	T      float64 `json:"t"`
	Sharpe float64 `json:"sharpe"`
}

type panelJSON struct {
	Symbols int    `json:"symbols"`
	Dates   int    `json:"dates"`
	First   string `json:"first"`
	Last    string `json:"last"`
}

type reportJSON struct {
	Generated string         `json:"generated"`
	Window    string         `json:"window"`
	Top       int            `json:"top"`
	Venue     string         `json:"venue"`
	Exec      string         `json:"exec"`
	Panel     panelJSON      `json:"panel"`
	Coverage  []coverageJSON `json:"coverage"`
	Costs     []tierJSON     `json:"costs"`
	Funding   []fundingJSON  `json:"funding"`
	Close     []cellJSON     `json:"close_entry"`
	Open      []cellJSON     `json:"open_entry"`
	Lead      struct {
		Signal    string        `json:"signal"`
		Lookback  int           `json:"lookback"`
		Rate      float64       `json:"rate"`
		Base      cellJSON      `json:"base"`
		Variants  []variantJSON `json:"variants"`
		Placebo   []float64     `json:"placebo"`
		BreakEven float64       `json:"break_even_per_side"`
		Charged   float64       `json:"charged_per_side"`
	} `json:"lead"`
	Bars struct {
		Bonferroni10 float64 `json:"bonferroni_10"`
		Gate         float64 `json:"gate_t"`
	} `json:"bars"`
}

func cellOf(c batteryCell, r result) cellJSON {
	dsr, _ := deflatedSharpe(netSeries(r), 10)
	return cellJSON{
		Signal: c.signal, Lookback: c.lookback, Rate: c.rate, Days: len(r.points),
		IC: r.meanIC, Gross: r.meanGen, Cost: r.meanCost, Fund: r.meanFund,
		Net: r.meanNet, T: r.tNet, Sharpe: r.sharpe, DSR: dsr, H1: r.half1, H2: r.half2,
		Turn: r.turn, AvgCost: r.avgCost,
	}
}

func exportReport(o options) error {
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
	from := o.from
	if from == "" {
		from = "2024-01-01"
	}
	to := o.to
	if to == "" {
		to = p.dates[len(p.dates)-1]
	}
	top := o.top
	if top == 0 {
		top = 100
	}
	rep := reportJSON{
		Generated: nowUTC(),
		Window:    from + " .. " + to,
		Top:       top,
		Venue:     venue.Name,
		Exec:      "taker",
	}
	if exec == cryptocost.Maker {
		rep.Exec = "maker"
	}
	rep.Panel = panelJSON{Symbols: len(p.series), Dates: len(p.dates), First: p.dates[0], Last: p.dates[len(p.dates)-1]}
	rep.Coverage = p.coverage()
	for _, t := range tiers {
		c := costConfig(venue, cryptocost.Taker, t.minQuoteVol+1)
		rep.Costs = append(rep.Costs, tierJSON{
			Name: tierLabel(t.minQuoteVol), Fee: c.FeeBpsPerSide(), Cross: c.CrossCostBpsPerSide(),
			AllIn: c.AllInBpsPerSide(), RoundTrip: c.RoundTripBps(),
		})
	}
	for _, apr := range []float64{0.09, 0.11, 0.22} {
		rate8h := apr / (3 * 365)
		rep.Funding = append(rep.Funding, fundingJSON{
			APR:   apr,
			Day1:  cryptocost.FundingBps(rate8h, 8, 24),
			Day5:  cryptocost.FundingBps(rate8h, 8, 5*24),
			Day10: cryptocost.FundingBps(rate8h, 8, 10*24),
			Day40: cryptocost.FundingBps(rate8h, 8, 40*24),
		})
	}
	cells := batteryCells()
	for _, entry := range []string{"close", "open"} {
		var list []cellJSON
		for _, c := range cells {
			opts := bookOptions{
				lookback: c.lookback, top: top, quantile: o.quantile, rate: c.rate,
				horizon: o.horizon, venue: venue, exec: exec, noFunding: o.noFunding,
				signal: signalKind(c.signal), entry: entry, from: from, to: to,
			}
			list = append(list, cellOf(c, p.runBook(opts, c.signal)))
		}
		if entry == "close" {
			rep.Close = list
		} else {
			rep.Open = list
		}
	}
	lead := bookOptions{
		lookback: 20, top: top, quantile: o.quantile, rate: 0.10,
		horizon: o.horizon, venue: venue, exec: cryptocost.Taker, signal: "taker",
		entry: "close", from: from, to: to,
	}
	base := p.runBook(lead, "taker")
	rep.Lead.Signal = "taker-buy share"
	rep.Lead.Lookback = 20
	rep.Lead.Rate = 0.10
	rep.Lead.Base = cellOf(batteryCell{signal: "taker", lookback: 20, rate: 0.10}, base)
	rep.Lead.BreakEven = base.bePerSide
	rep.Lead.Charged = base.avgCost
	type variant struct {
		label string
		edit  func(*bookOptions)
	}
	variants := []variant{
		{"no funding (carry removed)", func(b *bookOptions) { b.noFunding = true }},
		{"maker execution", func(b *bookOptions) { b.exec = cryptocost.Maker }},
		{"Bybit fee schedule", func(b *bookOptions) { b.venue = cryptocost.Bybit() }},
		{"quantile 0.10", func(b *bookOptions) { b.quantile = 0.10 }},
		{"quantile 0.30", func(b *bookOptions) { b.quantile = 0.30 }},
		{"lookback 10", func(b *bookOptions) { b.lookback = 10 }},
		{"lookback 40", func(b *bookOptions) { b.lookback = 40 }},
		{"top 50 universe", func(b *bookOptions) { b.top = 50 }},
		{"top 200 universe", func(b *bookOptions) { b.top = 200 }},
		{"next-open entry", func(b *bookOptions) { b.entry = "open" }},
	}
	for _, v := range variants {
		vo := lead
		v.edit(&vo)
		r := p.runBook(vo, v.label)
		rep.Lead.Variants = append(rep.Lead.Variants, variantJSON{Label: v.label, Net: r.meanNet, T: r.tNet, Sharpe: r.sharpe})
	}
	for i := 0; i < 5; i++ {
		po := lead
		po.shuffle = true
		po.seed = int64(i + 1)
		r := p.runBook(po, fmt.Sprintf("placebo-%d", i+1))
		rep.Lead.Placebo = append(rep.Lead.Placebo, r.meanNet)
	}
	rep.Bars.Bonferroni10 = bonferroniT(10, 0.05)
	rep.Bars.Gate = 3.3
	out := o.out
	if out == "" {
		out = filepath.Join("artifacts", "crypto", "w1.json")
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(out, append(data, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("report data -> %s (%d bytes)\n", out, len(data))
	fmt.Printf("cells close %d, open %d, lead variants %d, placebo %d\n",
		len(rep.Close), len(rep.Open), len(rep.Lead.Variants), len(rep.Lead.Placebo))
	return nil
}

func (p *panel) coverage() []coverageJSON {
	lastByMonth := map[string]string{}
	var months []string
	for _, d := range p.dates {
		m := d[:7]
		if _, ok := lastByMonth[m]; !ok {
			months = append(months, m)
		}
		lastByMonth[m] = d
	}
	var out []coverageJSON
	for _, m := range months {
		d := lastByMonth[m]
		n := 0
		for _, s := range p.series {
			if _, ok := s.byDate[d]; ok {
				n++
			}
		}
		out = append(out, coverageJSON{Month: m, Symbols: n})
	}
	return out
}

func nowUTC() string {
	return timeNowUTC()
}

func timeNowUTC() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05Z")
}
