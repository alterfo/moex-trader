package main

import (
	"fmt"
	"math"
	"strings"
)

const svgW = 820.0

func labelOf(c cellJSON) string {
	name := c.Signal
	switch c.Signal {
	case "mom":
		if c.Lookback > 100 {
			name = "12m mom"
		} else {
			name = "1m mom"
		}
	case "momfast":
		name = "1m mom"
	case "rev":
		name = "1d reversal"
	case "taker":
		name = "taker flow"
	case "funding":
		name = "funding carry"
	}
	return fmt.Sprintf("%s %.2f", name, c.Rate)
}

func coverageChart(cov []coverageJSON, windowStart string) string {
	h := 250.0
	c := newCanvas(svgW, h)
	left, right, top, bottom := 52.0, 16.0, 18.0, 42.0
	plotW := svgW - left - right
	plotH := h - top - bottom
	maxN := 0
	for _, p := range cov {
		if p.Symbols > maxN {
			maxN = p.Symbols
		}
	}
	if maxN == 0 {
		return c.finish()
	}
	maxN = int(math.Ceil(float64(maxN)/100) * 100)
	for i := 0; i <= 4; i++ {
		y := top + plotH*float64(i)/4
		val := float64(maxN) * (1 - float64(i)/4)
		c.line(left, y, left+plotW, y, colRule, 1, "")
		c.mono(left-8, y+4, fmt.Sprintf("%.0f", val), colMuted, 10, "end")
	}
	step := plotW / float64(len(cov)-1)
	pts := make([]string, 0, len(cov))
	for i, p := range cov {
		x := left + step*float64(i)
		y := top + plotH*(1-float64(p.Symbols)/float64(maxN))
		pts = append(pts, fmt.Sprintf("%.1f,%.1f", x, y))
	}
	area := "M " + pts[0] + " " + strings.Join(pts[1:], " ") +
		fmt.Sprintf(" L %.1f,%.1f L %.1f,%.1f Z", left+plotW, top+plotH, left, top+plotH)
	c.path(area, "none", colSurface, 0)
	c.path("M "+strings.Join(pts, " L "), colAccent2, "none", 1.8)
	if windowStart != "" {
		for i, p := range cov {
			if p.Month == windowStart[:7] {
				x := left + step*float64(i)
				c.line(x, top, x, top+plotH, colAccent, 1.5, "5 4")
				c.text(x+6, top+12, "evaluation window starts", colAccent, 10, "start", "600")
				break
			}
		}
	}
	for i, p := range cov {
		if i%6 != 0 && i != len(cov)-1 {
			continue
		}
		x := left + step*float64(i)
		c.mono(x, h-20, p.Month, colMuted, 9, "middle")
	}
	c.text(left, h-4, "contracts with a bar at month end", colMuted, 10, "start", "400")
	last := cov[len(cov)-1]
	c.mono(left+plotW, top+12, fmt.Sprintf("%d contracts", last.Symbols), colInk, 11, "end")
	return c.finish()
}

func costChart(costs []tierJSON) string {
	h := 260.0
	c := newCanvas(svgW, h)
	left, right, top, bottom := 52.0, 16.0, 20.0, 52.0
	plotW := svgW - left - right
	plotH := h - top - bottom
	maxV := 0.0
	for _, t := range costs {
		if t.RoundTrip > maxV {
			maxV = t.RoundTrip
		}
	}
	maxV = math.Ceil(maxV/10) * 10
	if maxV < 30 {
		maxV = 30
	}
	for i := 0; i <= 5; i++ {
		y := top + plotH*float64(i)/5
		c.line(left, y, left+plotW, y, colRule, 1, "")
		c.mono(left-8, y+4, fmt.Sprintf("%.0f", maxV*(1-float64(i)/5)), colMuted, 10, "end")
	}
	group := plotW / float64(len(costs))
	barW := math.Min(18, group/5)
	for i, t := range costs {
		cx := left + group*float64(i) + group/2
		vals := []struct {
			v     float64
			label string
			col   string
		}{
			{t.Fee, "fee", colMuted},
			{t.Cross, "spread+impact", colAccent2},
			{t.AllIn, "all-in per side", colAccent},
		}
		for j, v := range vals {
			bh := plotH * v.v / maxV
			x := cx - 1.5*barW - 4 + float64(j)*(barW+4)
			c.rect(x, top+plotH-bh, barW, bh, v.col)
			c.mono(x+barW/2, top+plotH-bh-4, fmt.Sprintf("%.1f", v.v), colInk, 9, "middle")
		}
		c.text(cx, h-26, t.Name, colInk, 10.5, "middle", "600")
	}
	y15 := top + plotH*(1-15.0/maxV)
	if maxV > 15 {
		c.line(left, y15, left+plotW, y15, colNeg, 1.2, "4 4")
		c.text(left+plotW-4, y15-6, "MOEX programme: 15 bps a side", colNeg, 10, "end", "500")
	}
	c.text(left, h-6, "bps  ·  fee, spread+impact and all-in cost per side, retail tier, 2026-11-02", colMuted, 10, "start", "400")
	return c.finish()
}

func fundingChart(funding []fundingJSON) string {
	h := 250.0
	c := newCanvas(svgW, h)
	left, right, top, bottom := 52.0, 90.0, 18.0, 40.0
	plotW := svgW - left - right
	plotH := h - top - bottom
	xs := []float64{1, 5, 10, 40}
	maxV := 0.0
	for _, f := range funding {
		for _, v := range []float64{f.Day1, f.Day5, f.Day10, f.Day40} {
			if v > maxV {
				maxV = v
			}
		}
	}
	maxV = math.Ceil(maxV/50) * 50
	xOf := func(d float64) float64 {
		switch d {
		case 1:
			return left
		case 5:
			return left + plotW*0.22
		case 10:
			return left + plotW*0.44
		default:
			return left + plotW
		}
	}
	for i := 0; i <= 5; i++ {
		y := top + plotH*float64(i)/5
		c.line(left, y, left+plotW, y, colRule, 1, "")
		c.mono(left-8, y+4, fmt.Sprintf("%.0f", maxV*(1-float64(i)/5)), colMuted, 10, "end")
	}
	cols := []string{colAccent2, colAccent, colNeg}
	for i, f := range funding {
		vals := []float64{f.Day1, f.Day5, f.Day10, f.Day40}
		var d strings.Builder
		for j, v := range vals {
			y := top + plotH*(1-v/maxV)
			cmd := "M"
			if j > 0 {
				cmd = "L"
			}
			fmt.Fprintf(&d, "%s %.1f,%.1f ", cmd, xOf(xs[j]), y)
			c.circle(xOf(xs[j]), y, 3, cols[i], colPaper)
		}
		c.path(strings.TrimSpace(d.String()), cols[i], "none", 1.8)
		y := top + plotH*(1-vals[len(vals)-1]/maxV)
		c.mono(left+plotW+8, y+4, fmt.Sprintf("%.0f%% APR -> %.0f", f.APR*100, vals[len(vals)-1]), cols[i], 10, "start")
	}
	for _, d := range xs {
		c.mono(xOf(d), h-20, fmt.Sprintf("%.0f d", d), colMuted, 10, "middle")
	}
	c.text(left, h-4, "funding paid by a long, bps of notional, against the holding period", colMuted, 10, "start", "400")
	return c.finish()
}

func cellsBarChart(cells []cellJSON, title string) string {
	h := 280.0
	c := newCanvas(svgW, h)
	left, right, top, bottom := 52.0, 16.0, 18.0, 74.0
	plotW := svgW - left - right
	plotH := h - top - bottom
	minV, maxV := 0.0, 0.0
	for _, cl := range cells {
		minV = math.Min(minV, cl.Net)
		maxV = math.Max(maxV, cl.Net)
	}
	pad := math.Max(5, (maxV-minV)*0.12)
	minV = math.Floor((minV-pad)/5) * 5
	maxV = math.Ceil((maxV+pad)/5) * 5
	yOf := func(v float64) float64 {
		return top + plotH*(1-(v-minV)/(maxV-minV))
	}
	c.line(left, yOf(0), left+plotW, yOf(0), colInk, 1, "")
	steps := int(math.Round((maxV - minV) / 10))
	if steps < 4 {
		steps = 4
	}
	for i := 0; i <= steps; i++ {
		v := minV + (maxV-minV)*float64(i)/float64(steps)
		y := yOf(v)
		c.line(left, y, left+plotW, y, colRule, 1, "")
		c.mono(left-8, y+4, fmt.Sprintf("%.0f", v), colMuted, 10, "end")
	}
	group := plotW / float64(len(cells))
	barW := math.Min(38, group*0.62)
	for i, cl := range cells {
		cx := left + group*float64(i) + group/2
		y0 := yOf(0)
		y1 := yOf(cl.Net)
		fill := colAccent
		if cl.Net < 0 {
			fill = colNeg
		}
		topY := math.Min(y0, y1)
		hh := math.Abs(y1 - y0)
		c.rect(cx-barW/2, topY, barW, hh, fill)
		labelY := topY - 5
		if cl.Net < 0 {
			labelY = topY + hh + 12
		}
		c.mono(cx, labelY, fmt.Sprintf("%.1f", cl.Net), colInk, 10, "middle")
		parts := strings.Split(labelOf(cl), " ")
		c.text(cx, h-52, strings.Join(parts[:len(parts)-1], " "), colInk, 9.5, "middle", "600")
		c.mono(cx, h-38, parts[len(parts)-1], colMuted, 9.5, "middle")
	}
	c.text(left, h-4, title, colMuted, 10, "start", "400")
	return c.finish()
}

func tChart(cells []cellJSON, bars struct {
	Bonferroni10 float64 `json:"bonferroni_10"`
	Gate         float64 `json:"gate_t"`
}) string {
	h := 236.0
	c := newCanvas(svgW, h)
	left, right, top, bottom := 52.0, 16.0, 26.0, 62.0
	plotW := svgW - left - right
	plotH := h - top - bottom
	lo, hi := 0.0, 0.0
	for _, cl := range cells {
		lo = math.Min(lo, cl.T)
		hi = math.Max(hi, cl.T)
	}
	lo = math.Min(lo, -bars.Gate) - 0.4
	hi = math.Max(hi, bars.Gate) + 0.4
	yOf := func(v float64) float64 { return top + plotH*(1-(v-lo)/(hi-lo)) }
	for i := 0; i <= 4; i++ {
		v := lo + (hi-lo)*float64(i)/4
		c.line(left, yOf(v), left+plotW, yOf(v), colRule, 1, "")
		c.mono(left-8, yOf(v)+4, fmt.Sprintf("%.1f", v), colMuted, 10, "end")
	}
	c.line(left, yOf(0), left+plotW, yOf(0), colInk, 1, "")
	c.line(left, yOf(bars.Bonferroni10), left+plotW, yOf(bars.Bonferroni10), colAccent, 1.2, "5 4")
	c.text(left+6, yOf(bars.Bonferroni10)+13, fmt.Sprintf("Bonferroni over 10 cells: %.2f", bars.Bonferroni10), colAccent, 10, "start", "500")
	c.line(left, yOf(bars.Gate), left+plotW, yOf(bars.Gate), colNeg, 1.2, "")
	c.text(left+6, yOf(bars.Gate)-6, fmt.Sprintf("registered gate: %.1f", bars.Gate), colNeg, 10, "start", "500")
	c.line(left, yOf(-bars.Bonferroni10), left+plotW, yOf(-bars.Bonferroni10), colAccent, 1.2, "5 4")
	c.line(left, yOf(-bars.Gate), left+plotW, yOf(-bars.Gate), colNeg, 1.2, "")
	group := plotW / float64(len(cells))
	for i, cl := range cells {
		cx := left + group*float64(i) + group/2
		y := yOf(cl.T)
		mark := colInk
		radius := 4.0
		if math.Abs(cl.T) >= bars.Bonferroni10 {
			mark = colAccent
			radius = 5.5
		}
		if math.Abs(cl.T) >= bars.Gate {
			mark = colPos
			radius = 6
		}
		c.circle(cx, y, radius, mark, colPaper)
		c.mono(cx, y-9, fmt.Sprintf("%.2f", cl.T), colInk, 9, "middle")
	}
	c.text(left, h-4, "Newey-West t of the daily net, same ten cells; the farther from zero, the stronger", colMuted, 10, "start", "400")
	return c.finish()
}

func variantsChart(variants []variantJSON, base variantJSON) string {
	rowH := 26.0
	h := 40 + rowH*float64(len(variants)+1)
	c := newCanvas(svgW, h)
	left, right, top := 230.0, 70.0, 24.0
	plotW := svgW - left - right
	lo, hi := 0.0, 0.0
	for _, v := range variants {
		lo = math.Min(lo, v.Net)
		hi = math.Max(hi, v.Net)
	}
	lo = math.Min(lo, base.Net)
	hi = math.Max(hi, base.Net)
	lo -= 2
	hi += 2
	xOf := func(v float64) float64 { return left + plotW*(v-lo)/(hi-lo) }
	c.line(xOf(0), top-8, xOf(0), top+rowH*float64(len(variants)+1)-8, colInk, 1, "")
	rows := append([]variantJSON{base}, variants...)
	for i, v := range rows {
		y := top + rowH*float64(i)
		label := v.Label
		c.text(left-10, y+4, label, colInk, 10.5, "end", "500")
		c.line(xOf(0), y, xOf(v.Net), y, colRule, 2, "")
		fill := colAccent
		if i == 0 {
			fill = colInk
		}
		c.circle(xOf(v.Net), y, 4.5, fill, colPaper)
		c.mono(xOf(v.Net)+10, y+4, fmt.Sprintf("%.1f  (t %.2f)", v.Net, v.T), colMuted, 9.5, "start")
	}
	c.text(left, h-4, "bps a day, net of costs and funding; the base row is the registered cell", colMuted, 10, "start", "400")
	return c.finish()
}

func placeboChart(placebo []float64, real float64) string {
	h := 230.0
	c := newCanvas(svgW, h)
	left, right, top, bottom := 52.0, 16.0, 18.0, 58.0
	plotW := svgW - left - right
	plotH := h - top - bottom
	lo, hi := 0.0, 0.0
	for _, v := range placebo {
		lo = math.Min(lo, v)
		hi = math.Max(hi, v)
	}
	lo = math.Min(lo, real)
	hi = math.Max(hi, real)
	pad := math.Max(2, (hi-lo)*0.15)
	lo = math.Floor((lo-pad)/5) * 5
	hi = math.Ceil((hi+pad)/5) * 5
	yOf := func(v float64) float64 { return top + plotH*(1-(v-lo)/(hi-lo)) }
	c.line(left, yOf(0), left+plotW, yOf(0), colInk, 1, "")
	steps := int(math.Round((hi - lo) / 5))
	if steps < 3 {
		steps = 3
	}
	for i := 0; i <= steps; i++ {
		v := lo + (hi-lo)*float64(i)/float64(steps)
		c.line(left, yOf(v), left+plotW, yOf(v), colRule, 1, "")
		c.mono(left-8, yOf(v)+4, fmt.Sprintf("%.0f", v), colMuted, 10, "end")
	}
	group := plotW / float64(len(placebo)+1)
	barW := math.Min(44, group*0.6)
	draw := func(i int, v float64, fill string, name string) {
		cx := left + group*float64(i) + group/2
		y0, y1 := yOf(0), yOf(v)
		topY := math.Min(y0, y1)
		hh := math.Abs(y1 - y0)
		if hh < 1 {
			hh = 1
		}
		c.rect(cx-barW/2, topY, barW, hh, fill)
		labelY := topY - 5
		if v < 0 {
			labelY = topY + hh + 12
		}
		c.mono(cx, labelY, fmt.Sprintf("%.1f", v), colInk, 10, "middle")
		c.text(cx, h-38, name, colMuted, 9.5, "middle", "500")
	}
	draw(0, real, colInk, "real")
	for i, v := range placebo {
		draw(i+1, v, colMuted, fmt.Sprintf("perm %d", i+1))
	}
	c.text(left, h-4, "bps a day net; five within-date permutations of the same signal through the same book", colMuted, 10, "start", "400")
	return c.finish()
}
