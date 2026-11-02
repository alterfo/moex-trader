package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type intradayCell struct {
	Cell       string  `json:"cell"`
	Session    string  `json:"session"`
	Trades     int     `json:"trades"`
	Days       int     `json:"days"`
	TradesDay  float64 `json:"trades_per_day"`
	HitRate    float64 `json:"hit_rate"`
	HoldMin    float64 `json:"hold_minutes"`
	GrossTrade float64 `json:"gross_per_trade"`
	CostTrade  float64 `json:"cost_per_trade"`
	FundTrade  float64 `json:"funding_per_trade"`
	NetTrade   float64 `json:"net_per_trade"`
	TTrade     float64 `json:"t_per_trade"`
	GrossDay   float64 `json:"gross_per_day"`
	CostDay    float64 `json:"cost_per_day"`
	FundDay    float64 `json:"funding_per_day"`
	NetDay     float64 `json:"net_per_day"`
	NetDayT    float64 `json:"net_per_day_t"`
	Sharpe     float64 `json:"sharpe"`
	H1         float64 `json:"h1"`
	H2         float64 `json:"h2"`
}

func loadIntraday(dir string) []intradayCell {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []intradayCell
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var c intradayCell
		if err := json.Unmarshal(raw, &c); err != nil {
			continue
		}
		out = append(out, c)
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Cell != out[b].Cell {
			return out[a].Cell < out[b].Cell
		}
		return out[a].Session < out[b].Session
	})
	return out
}

func tradeCells(cells []intradayCell) []intradayCell {
	var out []intradayCell
	for _, c := range cells {
		if c.Trades > 0 {
			out = append(out, c)
		}
	}
	return out
}

func momCells(cells []intradayCell) []intradayCell {
	var out []intradayCell
	for _, c := range cells {
		if c.Trades == 0 {
			out = append(out, c)
		}
	}
	return out
}

func tradeEconChart(cells []intradayCell) string {
	h := 280.0
	c := newCanvas(svgW, h)
	left, right, top, bottom := 56.0, 16.0, 22.0, 66.0
	plotW := svgW - left - right
	plotH := h - top - bottom
	lo, hi := 0.0, 0.0
	for _, t := range cells {
		lo = math.Min(lo, t.NetTrade)
		hi = math.Max(hi, math.Max(t.GrossTrade, t.CostTrade))
	}
	lo, hi, step := axisRange(lo, hi)
	yOf := func(v float64) float64 { return top + plotH*(1-(v-lo)/(hi-lo)) }
	for v := lo; v <= hi+step/2; v += step {
		c.line(left, yOf(v), left+plotW, yOf(v), colRule, 1, "")
		c.mono(left-8, yOf(v)+4, fmt.Sprintf("%.0f", v), colMuted, 10, "end")
	}
	group := plotW / float64(len(cells))
	barW := math.Min(22, group/4.5)
	for i, t := range cells {
		cx := left + group*float64(i) + group/2
		vals := []struct {
			v     float64
			col   string
			label string
		}{
			{t.GrossTrade, colAccent2, "gross"},
			{t.CostTrade, colNeg, "cost"},
			{t.NetTrade, colAccent, "net"},
		}
		for j, v := range vals {
			x := cx - 1.5*barW - 3 + float64(j)*(barW+3)
			y0 := yOf(0)
			y1 := yOf(v.v)
			topY := math.Min(y0, y1)
			hh := math.Abs(y1 - y0)
			if hh < 1 {
				hh = 1
			}
			c.rect(x, topY, barW, hh, v.col)
			c.mono(x+barW/2, topY-4, fmt.Sprintf("%.1f", v.v), colInk, 9, "middle")
		}
		c.text(cx, h-46, t.Cell, colInk, 10.5, "middle", "600")
		c.mono(cx, h-32, strings.ToUpper(t.Session), colMuted, 9.5, "middle")
	}
	c.text(left, h-4, "bps per trade: what the rule earns gross, what the round trip costs, what is left", colMuted, 10, "start", "400")
	return c.finish()
}

func momChart(cells []intradayCell) string {
	h := 250.0
	c := newCanvas(svgW, h)
	left, right, top, bottom := 56.0, 16.0, 22.0, 62.0
	plotW := svgW - left - right
	plotH := h - top - bottom
	lo, hi := 0.0, 0.0
	for _, t := range cells {
		mirror := -t.GrossDay - t.CostDay + t.FundDay
		lo = math.Min(lo, math.Min(t.NetDay, mirror))
		hi = math.Max(hi, math.Max(t.NetDay, mirror))
	}
	lo, hi, step := axisRange(lo, hi)
	yOf := func(v float64) float64 { return top + plotH*(1-(v-lo)/(hi-lo)) }
	for v := lo; v <= hi+step/2; v += step {
		c.line(left, yOf(v), left+plotW, yOf(v), colRule, 1, "")
		c.mono(left-8, yOf(v)+4, fmt.Sprintf("%.0f", v), colMuted, 10, "end")
	}
	c.line(left, yOf(0), left+plotW, yOf(0), colInk, 1, "")
	group := plotW / float64(len(cells))
	barW := math.Min(46, group/3)
	cols := []string{colNeg, colAccent}
	for i, t := range cells {
		cx := left + group*float64(i) + group/2
		mirror := -t.GrossDay - t.CostDay + t.FundDay
		vals := []float64{t.NetDay, mirror}
		for j, v := range vals {
			x := cx - barW - 2 + float64(j)*(barW+4)
			y0, y1 := yOf(0), yOf(v)
			topY := math.Min(y0, y1)
			hh := math.Abs(y1 - y0)
			if hh < 1 {
				hh = 1
			}
			c.rect(x, topY, barW, hh, cols[j])
			lbl := fmt.Sprintf("%.1f", v)
			ly := topY - 4
			if v < 0 {
				ly = topY + hh + 12
			}
			c.mono(x+barW/2, ly, lbl, colInk, 9, "middle")
		}
		c.text(cx, h-40, t.Cell+" "+strings.ToUpper(t.Session), colInk, 10, "middle", "600")
	}
	c.text(left, h-22, "bps a day net: the momentum book against its mirror (fade the opening range)", colMuted, 10, "start", "400")
	c.mono(left, h-6, fmt.Sprintf("legend: %s = momentum, %s = mirror", "dark red", "ochre"), colMuted, 10, "start")
	return c.finish()
}

func intradaySection(d *doc, cells []intradayCell) {
	if len(cells) == 0 {
		return
	}
	tc := tradeCells(cells)
	mc := momCells(cells)
	d.w(`<section id="intraday">
<h2><span class="num">05</span>Intraday: breakout, momentum and levels on 5-minute bars</h2>
<p>Three rules the owner asked about, each run in two session definitions — the UTC day (range 00:00-00:30) and the US cash open (13:30-14:00) — on the same 100 contracts, 2023-01-01 to 2026-10-31. Costs are the same frozen model; funding is charged only at settlements that fall inside the hold.</p>
<p>The economic fact that decides this section before any signal does: an intraday book turns its notional over once or twice a day, and at the retail tier that costs 26-52 bps a day, against 1.7-5.6 bps a day for the low-turnover daily books above.</p>
`)
	if len(tc) > 0 {
		d.w(`<figure>`)
		d.w(tradeEconChart(tc))
		d.w(`<figcaption>Per trade. The breakout rule earns a real gross — about 16 bps over a four-hour hold — and the taker round trip takes 14 of it. The level rule earns 0.1-0.8 bps against 27 bps of cost: at this specification levels are not a signal at all.</figcaption></figure>
`)
	}
	if len(mc) > 0 {
		d.w(`<figure>`)
		d.w(momChart(mc))
		d.w(`<figcaption>Per day. The first half hour of the UTC day <em>reverses</em> over the rest of it: the momentum book loses 34.7 bps gross and the mirrored book earns the same — and still loses 18.0 bps a day net, because the mirrored book pays 52.7 bps of turnover to collect it.</figcaption></figure>
`)
	}
	d.w(`<table><caption>Six intraday cells, run once on the frozen 5-minute panel</caption>
<thead><tr><th>cell</th><th>session</th><th class="num">units</th><th class="num">gross</th><th class="num">cost</th><th class="num">net</th><th class="num">t</th><th class="num">halves</th></tr></thead>
<tbody>`)
	for _, c := range cells {
		units, gross, cost, net, tv := "", "", "", "", ""
		if c.Trades > 0 {
			units = fmt.Sprintf("%.1f trades/day", c.TradesDay)
			gross = fmt1(c.GrossTrade) + " /t"
			cost = fmt1(c.CostTrade) + " /t"
			net = fmt1(c.NetTrade) + " /t"
			tv = fmt2(c.TTrade)
		} else {
			units = "65 names/day"
			gross = fmt1(c.GrossDay) + " /d"
			cost = fmt1(c.CostDay) + " /d"
			net = fmt1(c.NetDay) + " /d"
			tv = fmt2(c.NetDayT)
		}
		d.f(`<tr><td class="mono">%s</td><td class="mono">%s</td><td class="num">%s</td><td class="num">%s</td><td class="num">%s</td><td class="num">%s</td><td class="num">%s</td><td class="num">%s / %s</td></tr>`,
			c.Cell, strings.ToUpper(c.Session), units, gross, cost, net, tv, fmt1(c.H1), fmt1(c.H2))
	}
	d.w(`</tbody></table>
<div class="callout"><p class="big">0 candidates, no leads.</p><p>The breakout has a real gross effect that the round trip eats; the momentum book has a large, statistically loud effect with the wrong sign for the strategy as published, and the mirror cannot pay the turnover; the level rule has no gross effect to eat. None of the three is worth a forward record.</p></div>
</section>
`)
}
