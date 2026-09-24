package tearsheet

import (
	"fmt"
	"html/template"
	"strings"
)

const (
	svgWidth     = 760
	svgHeight    = 180
	svgPadX      = 44.0
	svgPadTop    = 12.0
	svgPadBottom = 24.0
)

func lineChartSVG(values []float64, strokeVar, fillVar string, baselineAtTop bool) template.HTML {
	n := len(values)
	if n == 0 {
		return ""
	}
	plotW := float64(svgWidth) - 2*svgPadX
	plotH := float64(svgHeight) - svgPadTop - svgPadBottom

	minV, maxV := values[0], values[0]
	for _, v := range values {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}
	rng := maxV - minV

	xAt := func(i int) float64 {
		if n == 1 {
			return svgPadX
		}
		return svgPadX + float64(i)*plotW/float64(n-1)
	}
	yAt := func(v float64) float64 {
		if rng <= 0 {
			return svgPadTop + plotH/2
		}
		return svgPadTop + (maxV-v)/rng*plotH
	}

	var line strings.Builder
	for i, v := range values {
		fmt.Fprintf(&line, "%.2f,%.2f ", xAt(i), yAt(v))
	}

	baselineY := svgPadTop + plotH
	if baselineAtTop {
		baselineY = svgPadTop
	}
	var area strings.Builder
	fmt.Fprintf(&area, "M%.2f,%.2f ", xAt(0), baselineY)
	for i, v := range values {
		fmt.Fprintf(&area, "L%.2f,%.2f ", xAt(i), yAt(v))
	}
	fmt.Fprintf(&area, "L%.2f,%.2f Z", xAt(n-1), baselineY)

	gridTop := svgPadTop
	gridMid := svgPadTop + plotH/2
	axisBottom := svgPadTop + plotH

	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="0 0 %d %d" width="100%%" height="%d" preserveAspectRatio="none" role="img">`, svgWidth, svgHeight, svgHeight)
	fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" class="ts-svg-grid"/>`, svgPadX, gridTop, float64(svgWidth)-svgPadX, gridTop)
	fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" class="ts-svg-grid"/>`, svgPadX, gridMid, float64(svgWidth)-svgPadX, gridMid)
	fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" class="ts-svg-axis"/>`, svgPadX, axisBottom, float64(svgWidth)-svgPadX, axisBottom)
	fmt.Fprintf(&b, `<path d="%s" style="fill:var(%s);stroke:none" opacity="0.16"/>`, area.String(), fillVar)
	fmt.Fprintf(&b, `<polyline points="%s" style="fill:none;stroke:var(%s)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>`, strings.TrimSpace(line.String()), strokeVar)
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}
