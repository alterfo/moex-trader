package main

import (
	"fmt"
	"math"
	"strings"
)

const (
	colInk     = "#1c1a17"
	colMuted   = "#6d655c"
	colRule    = "#d9d2c7"
	colPaper   = "#f7f4ee"
	colSurface = "#efeae1"
	colAccent  = "#9c5b1e"
	colAccent2 = "#3f5f66"
	colNeg     = "#8f3a32"
	colPos     = "#3f6b4a"
)

type canvas struct {
	W, H float64
	body strings.Builder
}

func newCanvas(w, h float64) *canvas {
	c := &canvas{W: w, H: h}
	fmt.Fprintf(&c.body, `<svg viewBox="0 0 %.0f %.0f" role="img" xmlns="http://www.w3.org/2000/svg" preserveAspectRatio="xMidYMid meet">`, w, h)
	return c
}

func (c *canvas) finish() string {
	c.body.WriteString(`</svg>`)
	return c.body.String()
}

func (c *canvas) rect(x, y, w, h float64, fill string) {
	fmt.Fprintf(&c.body, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s"/>`, x, y, w, h, fill)
}

func (c *canvas) line(x1, y1, x2, y2 float64, stroke string, width float64, dash string) {
	d := ""
	if dash != "" {
		d = fmt.Sprintf(` stroke-dasharray="%s"`, dash)
	}
	fmt.Fprintf(&c.body, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="%.1f"%s/>`,
		x1, y1, x2, y2, stroke, width, d)
}

func (c *canvas) circle(x, y, r float64, fill, stroke string) {
	fmt.Fprintf(&c.body, `<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s" stroke="%s" stroke-width="1"/>`, x, y, r, fill, stroke)
}

func (c *canvas) text(x, y float64, s, fill string, size float64, anchor, weight string) {
	family := "ui-sans-serif, -apple-system, Segoe UI, Roboto, sans-serif"
	fmt.Fprintf(&c.body, `<text x="%.1f" y="%.1f" fill="%s" font-size="%.1f" font-family="%s" text-anchor="%s" font-weight="%s">%s</text>`,
		x, y, fill, size, family, anchor, weight, escape(s))
}

func (c *canvas) mono(x, y float64, s, fill string, size float64, anchor string) {
	family := `"SF Mono", ui-monospace, Menlo, monospace`
	fmt.Fprintf(&c.body, `<text x="%.1f" y="%.1f" fill="%s" font-size="%.1f" font-family="%s" text-anchor="%s">%s</text>`,
		x, y, fill, size, family, anchor, escape(s))
}

func (c *canvas) path(d, stroke, fill string, width float64) {
	fmt.Fprintf(&c.body, `<path d="%s" fill="%s" stroke="%s" stroke-width="%.1f" stroke-linejoin="round" stroke-linecap="round"/>`,
		d, fill, stroke, width)
}

func escape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

func fmt1(v float64) string {
	if math.IsNaN(v) {
		return "n/a"
	}
	return fmt.Sprintf("%.1f", v)
}

func fmt2(v float64) string {
	if math.IsNaN(v) {
		return "n/a"
	}
	return fmt.Sprintf("%.2f", v)
}

func fmt3(v float64) string {
	if math.IsNaN(v) {
		return "n/a"
	}
	return fmt.Sprintf("%.3f", v)
}

func niceStep(span float64, target int) float64 {
	if span <= 0 || target <= 0 {
		return 1
	}
	raw := span / float64(target)
	mag := math.Pow(10, math.Floor(math.Log10(raw)))
	for _, m := range []float64{1, 2, 2.5, 5, 10} {
		if raw <= m*mag {
			return m * mag
		}
	}
	return 10 * mag
}

func axisRange(lo, hi float64) (float64, float64, float64) {
	step := niceStep(hi-lo, 5)
	lo = math.Floor(lo/step) * step
	hi = math.Ceil(hi/step) * step
	if hi == lo {
		hi = lo + step
	}
	return lo, hi, step
}
