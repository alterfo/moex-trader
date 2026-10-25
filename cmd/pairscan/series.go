package main

import "math"

type series struct {
	spec          pairSpec
	dates         []string
	z, mz, fd     []float64
	y1, y5, y10   []float64
	n             int
	spOrd, spPref []float64
}

func mean(a []float64) float64 {
	var s float64
	for _, x := range a {
		s += x
	}
	return s / float64(len(a))
}

func sd(a []float64) float64 {
	if len(a) < 2 {
		return nan
	}
	m := mean(a)
	var s float64
	for _, x := range a {
		s += (x - m) * (x - m)
	}
	return math.Sqrt(s / float64(len(a)-1))
}

func sideCost(sp float64) float64 {
	if math.IsNaN(sp) || sp <= 0 {
		return 0.0015
	}
	return 0.0005 + 0.5*sp/1e4
}

func build(p *panel, sp pairSpec) *series {
	s := &series{spec: sp}
	var S []float64
	cum := 0.0
	var diffs []float64
	var fdRaw []float64
	for _, d := range p.dates {
		a, okA := p.rows[d][sp.ord]
		b, okB := p.rows[d][sp.pref]
		if !okA || !okB {
			continue
		}
		step := 0.0
		if !math.IsNaN(a.rb1) && !math.IsNaN(b.rb1) && !a.div && !b.div {
			step = a.rb1 - b.rb1
		}
		cum += step
		S = append(S, cum)
		diffs = append(diffs, step)
		s.dates = append(s.dates, d)
		y := func(x, w float64) float64 {
			if math.IsNaN(x) || math.IsNaN(w) {
				return nan
			}
			return x - w
		}
		s.y1 = append(s.y1, y(a.f1, b.f1))
		s.y5 = append(s.y5, y(a.f5, b.f5))
		s.y10 = append(s.y10, y(a.f10, b.f10))
		fdRaw = append(fdRaw, y(a.a2, b.a2))
		s.spOrd = append(s.spOrd, a.sp)
		s.spPref = append(s.spPref, b.sp)
	}
	n := len(S)
	s.n = n
	s.z, s.mz, s.fd = make([]float64, n), make([]float64, n), make([]float64, n)
	for i := 0; i < n; i++ {
		s.z[i], s.mz[i], s.fd[i] = nan, nan, nan
		if i >= 60 {
			w := S[i-60 : i]
			if sdw := sd(w); sdw > 0 {
				s.z[i] = (S[i] - mean(w)) / sdw
			}
			dw := diffs[i-60 : i]
			if sdd := sd(dw); sdd > 0 && i >= 20 {
				s.mz[i] = (S[i] - S[i-20]) / (sdd * math.Sqrt(20))
			}
			var f []float64
			for _, v := range fdRaw[i-60 : i] {
				if !math.IsNaN(v) {
					f = append(f, v)
				}
			}
			if len(f) >= 40 && !math.IsNaN(fdRaw[i]) {
				if sdf := sd(f); sdf > 0 {
					s.fd[i] = fdRaw[i] / sdf
				}
			}
		}
	}
	return s
}
