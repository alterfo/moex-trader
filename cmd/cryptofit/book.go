package main

import (
	"math"
	"sort"

	"github.com/olegsidorkin/moex-trader/internal/cryptocost"
)

type tier struct {
	minQuoteVol float64
	halfSpread  float64
	impact      float64
}

var tiers = []tier{
	{minQuoteVol: 1e9, halfSpread: 0.5, impact: 1.0},
	{minQuoteVol: 1e8, halfSpread: 1.5, impact: 2.0},
	{minQuoteVol: 1e7, halfSpread: 4.0, impact: 4.0},
	{minQuoteVol: 0, halfSpread: 10.0, impact: 10.0},
}

func tierFor(medianQuoteVol float64) tier {
	for _, t := range tiers {
		if medianQuoteVol >= t.minQuoteVol {
			return t
		}
	}
	return tiers[len(tiers)-1]
}

func costConfig(venue cryptocost.Venue, exec cryptocost.Execution, medianQuoteVol float64) cryptocost.Config {
	t := tierFor(medianQuoteVol)
	fill := 0.0
	if exec == cryptocost.Maker {
		fill = 0.55
	}
	return cryptocost.Config{
		Venue:         venue,
		Instrument:    cryptocost.Perp,
		Exec:          exec,
		HalfSpreadBps: t.halfSpread,
		ImpactBps:     t.impact,
		MakerFillProb: fill,
	}
}

type point struct {
	date    string
	gross   float64
	cost    float64
	funding float64
	net     float64
	turn    float64
	names   int
}

type result struct {
	label     string
	points    []point
	ic        []float64
	meanIC    float64
	meanGen   float64
	meanCost  float64
	meanFund  float64
	meanNet   float64
	tNet      float64
	sharpe    float64
	half1     float64
	half2     float64
	turn      float64
	bePerSide float64
	fundPaid  float64
	fundRecv  float64
}

type bookOptions struct {
	lookback  int
	top       int
	quantile  float64
	rate      float64
	horizon   int
	venue     cryptocost.Venue
	exec      cryptocost.Execution
	noFunding bool
	shuffle   bool
	seed      int64
	signal    string
	entry     string
	from      string
	to        string
}

func (p *panel) medianQuoteVol(s *series, idx, window int) float64 {
	start := idx - window + 1
	if start < 0 {
		start = 0
	}
	vals := make([]float64, 0, idx-start+1)
	for i := start; i <= idx && i < len(s.bars); i++ {
		v := s.bars[i].quoteVol
		if v > 0 && !math.IsNaN(v) {
			vals = append(vals, v)
		}
	}
	if len(vals) == 0 {
		return 0
	}
	sort.Float64s(vals)
	return vals[len(vals)/2]
}

func (p *panel) signalFor(s *series, idx int, name string, o bookOptions) float64 {
	if idx < o.lookback || idx >= len(s.bars) {
		return math.NaN()
	}
	cur := s.bars[idx]
	switch name {
	case "mom":
		prev := s.bars[idx-o.lookback]
		if prev.close <= 0 {
			return math.NaN()
		}
		return cur.close/prev.close - 1
	case "rev":
		prev := s.bars[idx-1]
		if prev.close <= 0 {
			return math.NaN()
		}
		return -(cur.close/prev.close - 1)
	case "taker":
		if cur.quoteVol <= 0 {
			return math.NaN()
		}
		return cur.takerBuyQ / cur.quoteVol
	case "funding":
		if idx < 7 {
			return math.NaN()
		}
		sum := 0.0
		n := 0
		for i := idx - 6; i <= idx; i++ {
			if v, ok := s.funding[s.bars[i].date]; ok {
				sum += v
				n++
			}
		}
		if n == 0 {
			return math.NaN()
		}
		return -sum
	}
	return math.NaN()
}

func rankNormalise(vals []float64) []float64 {
	n := len(vals)
	out := make([]float64, n)
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return vals[idx[a]] < vals[idx[b]] })
	for rank, i := range idx {
		if n == 1 {
			out[i] = 0
			continue
		}
		out[i] = float64(rank)/float64(n-1) - 0.5
	}
	return out
}

func rankCorr(x, y []float64) float64 {
	if len(x) < 3 {
		return math.NaN()
	}
	rx := rankNormalise(x)
	ry := rankNormalise(y)
	var sx, sy, sxy, sxx, syy float64
	for i := range rx {
		sx += rx[i]
		sy += ry[i]
		sxy += rx[i] * ry[i]
		sxx += rx[i] * rx[i]
		syy += ry[i] * ry[i]
	}
	n := float64(len(rx))
	num := n*sxy - sx*sy
	den := math.Sqrt((n*sxx - sx*sx) * (n*syy - sy*sy))
	if den == 0 {
		return math.NaN()
	}
	return num / den
}

type cand struct {
	sym  string
	sig  float64
	r1   float64
	vol  float64
	fund float64
	cost cryptocost.Config
	rh   float64
}

func (p *panel) candidates(d string, di int, o bookOptions) []cand {
	cands := make([]cand, 0, 256)
	for _, s := range p.series {
		i, ok := s.byDate[d]
		if !ok {
			continue
		}
		sig := p.signalFor(s, i, o.signal, o)
		if math.IsNaN(sig) {
			continue
		}
		cur := s.bars[i].close
		if cur <= 0 {
			continue
		}
		var r1, fund float64
		if o.entry == "open" {
			if i+2 >= len(s.bars) {
				continue
			}
			entryBar := s.bars[i+1]
			exitBar := s.bars[i+2]
			if entryBar.open <= 0 || exitBar.open <= 0 {
				continue
			}
			r1 = exitBar.open/entryBar.open - 1
			fund = s.funding[entryBar.date]
		} else {
			if i+1 >= len(s.bars) {
				continue
			}
			nxt := s.bars[i+1].close
			if nxt <= 0 {
				continue
			}
			r1 = nxt/cur - 1
			fund = s.funding[s.bars[i+1].date]
		}
		vol := p.medianQuoteVol(s, i, 20)
		if vol <= 0 {
			continue
		}
		h := o.horizon
		if i+h >= len(s.bars) || s.bars[i+h].close <= 0 {
			h = 1
		}
		cands = append(cands, cand{
			sym:  s.symbol,
			sig:  sig,
			r1:   r1,
			vol:  vol,
			fund: fund,
			cost: costConfig(o.venue, o.exec, vol),
			rh:   s.bars[i+h].close/cur - 1,
		})
	}
	return cands
}

func (p *panel) runBook(o bookOptions, label string) result {
	var res result
	res.label = label
	prevW := map[string]float64{}
	for di := 0; di+1 < len(p.dates); di++ {
		d := p.dates[di]
		if o.from != "" && d < o.from {
			continue
		}
		if o.to != "" && d > o.to {
			continue
		}
		cands := p.candidates(d, di, o)
		if len(cands) < 20 {
			continue
		}
		sort.Slice(cands, func(a, b int) bool { return cands[a].vol > cands[b].vol })
		if o.top > 0 && len(cands) > o.top {
			cands = cands[:o.top]
		}
		sigs := make([]float64, len(cands))
		fwds := make([]float64, len(cands))
		for i := range cands {
			sigs[i] = cands[i].sig
			fwds[i] = cands[i].rh
		}
		if o.shuffle && o.seed != 0 {
			shuffleWithin(sigs, o.seed)
		}
		if ic := rankCorr(sigs, fwds); !math.IsNaN(ic) {
			res.ic = append(res.ic, ic)
		}
		ranks := rankNormalise(sigs)
		target := map[string]float64{}
		for i := range cands {
			if math.Abs(ranks[i]) < 0.5-o.quantile/2 {
				continue
			}
			w := 1.0
			if ranks[i] < 0 {
				w = -1.0
			}
			target[cands[i].sym] = w
		}
		var grossPos, grossNeg float64
		for _, w := range target {
			if w > 0 {
				grossPos += w
			} else {
				grossNeg -= w
			}
		}
		if grossPos == 0 || grossNeg == 0 {
			continue
		}
		for k, w := range target {
			if w > 0 {
				target[k] = w / grossPos
			} else {
				target[k] = w / grossNeg
			}
		}
		keys := make([]string, 0, len(prevW)+len(target))
		seen := map[string]bool{}
		for k := range prevW {
			keys = append(keys, k)
			seen[k] = true
		}
		for k := range target {
			if !seen[k] {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		costOf := make(map[string]cryptocost.Config, len(cands))
		r1Of := make(map[string]float64, len(cands))
		fundOf := make(map[string]float64, len(cands))
		for i := range cands {
			costOf[cands[i].sym] = cands[i].cost
			r1Of[cands[i].sym] = cands[i].r1
			fundOf[cands[i].sym] = cands[i].fund
		}
		newW := map[string]float64{}
		turnSum := 0.0
		costBps := 0.0
		for _, k := range keys {
			old := prevW[k]
			tgt := target[k]
			w := old + o.rate*(tgt-old)
			dw := math.Abs(w - old)
			turnSum += dw
			if dw > 0 {
				if c, ok := costOf[k]; ok {
					costBps += dw * c.AllInBpsPerSide()
				} else {
					costBps += dw * 30
				}
			}
			if math.Abs(w) < 1e-12 {
				continue
			}
			newW[k] = w
		}
		gross := 0.0
		funding := 0.0
		for k, w := range newW {
			gross += w * r1Of[k]
			if !o.noFunding {
				funding -= w * fundOf[k] / 1e4
			}
		}
		turnPerSide := turnSum / 2
		cost := costBps / 1e4
		net := gross - cost + funding
		res.points = append(res.points, point{
			date:    d,
			gross:   gross * 1e4,
			cost:    cost * 1e4,
			funding: funding * 1e4,
			net:     net * 1e4,
			turn:    turnPerSide * 100,
			names:   len(cands),
		})
		prevW = newW
	}
	res.summarise()
	return res
}

func (r *result) summarise() {
	n := len(r.points)
	if n == 0 {
		return
	}
	gs := make([]float64, 0, n)
	cs := make([]float64, 0, n)
	fs := make([]float64, 0, n)
	ns := make([]float64, 0, n)
	turnSum := 0.0
	for _, pt := range r.points {
		gs = append(gs, pt.gross)
		cs = append(cs, pt.cost)
		fs = append(fs, pt.funding)
		ns = append(ns, pt.net)
		turnSum += pt.turn
		if pt.funding < 0 {
			r.fundPaid += -pt.funding
		} else {
			r.fundRecv += pt.funding
		}
	}
	r.meanGen = mean(gs)
	r.meanCost = mean(cs)
	r.meanFund = mean(fs)
	r.meanNet = mean(ns)
	r.turn = turnSum / float64(n)
	r.tNet = nwT(ns, 5)
	r.sharpe = sharpe(ns)
	if n >= 2 {
		r.half1 = mean(ns[:n/2])
		r.half2 = mean(ns[n/2:])
	}
	if len(r.ic) > 0 {
		r.meanIC = mean(r.ic)
	}
	if turnSum > 0 {
		r.bePerSide = mean(gs) / (turnSum / float64(n) / 100)
	}
}

func mean(v []float64) float64 {
	if len(v) == 0 {
		return math.NaN()
	}
	s := 0.0
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}

func sd(v []float64) float64 {
	if len(v) < 2 {
		return math.NaN()
	}
	m := mean(v)
	s := 0.0
	for _, x := range v {
		s += (x - m) * (x - m)
	}
	return math.Sqrt(s / float64(len(v)-1))
}

func nwT(v []float64, lag int) float64 {
	n := len(v)
	if n < 10 {
		return math.NaN()
	}
	m := mean(v)
	d := make([]float64, n)
	for i := range v {
		d[i] = v[i] - m
	}
	var gamma0 float64
	for _, x := range d {
		gamma0 += x * x
	}
	gamma0 /= float64(n)
	var lr float64
	for l := 1; l <= lag && l < n; l++ {
		var g float64
		for i := l; i < n; i++ {
			g += d[i] * d[i-l]
		}
		g /= float64(n)
		lr += 2 * (1 - float64(l)/float64(lag+1)) * g
	}
	varr := (gamma0 + lr) / float64(n)
	if varr <= 0 {
		return math.NaN()
	}
	return m / math.Sqrt(varr)
}

func sharpe(v []float64) float64 {
	s := sd(v)
	if math.IsNaN(s) || s == 0 {
		return math.NaN()
	}
	return mean(v) / s * math.Sqrt(365)
}

func shuffleWithin(v []float64, seed int64) {
	r := newRand(seed)
	for i := len(v) - 1; i > 0; i-- {
		j := int(r.next() % int64(i+1))
		v[i], v[j] = v[j], v[i]
	}
}

type xorshift struct{ s uint64 }

func newRand(seed int64) *xorshift {
	s := uint64(seed)
	if s == 0 {
		s = 0x9E3779B97F4A7C15
	}
	return &xorshift{s: s}
}

func (r *xorshift) next() int64 {
	r.s ^= r.s << 13
	r.s ^= r.s >> 7
	r.s ^= r.s << 17
	return int64(r.s >> 1)
}
