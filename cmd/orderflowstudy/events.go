package main

import (
	"fmt"
	"io"
	"math"
	"sort"
)

const (
	horizonSlots = 12
	maxGapSlots  = 3
	lastEvent    = 89
	bootReps     = 2000
	bootSeed     = 20261022
	econHurdle   = 0.003
)

type event struct {
	day  int
	dir  float64
	out  float64
	c1   float64
	c2   float64
	real bool
}

func forwardFill(b *bars) [nSlots]float64 {
	var ff [nSlots]float64
	last := -1000
	cur := nan
	for s := 0; s < nSlots; s++ {
		if b.ts[s] && b.close[s] > 0 {
			cur = float64(b.close[s])
			last = s
		}
		if s-last > horizonSlots || math.IsNaN(cur) {
			ff[s] = nan
		} else {
			ff[s] = cur
		}
	}
	return ff
}

func (p *panel) hourReturns(adm []int, d int) [nSlots]float64 {
	var out [nSlots]float64
	var sums [nSlots]float64
	var cnt [nSlots]int
	for _, i := range adm {
		rec := p.recs[i][d]
		if rec == nil || rec.bars == nil {
			continue
		}
		ff := forwardFill(rec.bars)
		for s := 0; s+horizonSlots < nSlots; s++ {
			if math.IsNaN(ff[s]) || math.IsNaN(ff[s+horizonSlots]) {
				continue
			}
			sums[s] += math.Log(ff[s+horizonSlots] / ff[s])
			cnt[s]++
		}
	}
	for s := range out {
		out[s] = nan
		if cnt[s] >= minPerDay {
			out[s] = sums[s] / float64(cnt[s])
		}
	}
	return out
}

func (p *panel) priorCloses(i, d int) ([]float64, bool) {
	if d < 21 {
		return nil, false
	}
	cl := make([]float64, 0, 20)
	for k := d - 20; k < d; k++ {
		if p.excluded[k] {
			return nil, false
		}
		c := float64(p.close[i][k])
		if c <= 0 {
			return nil, false
		}
		if k > d-20 {
			if r := c/cl[len(cl)-1] - 1; !p.simpleOK(r) {
				return nil, false
			}
		}
		cl = append(cl, c)
	}
	return cl, true
}

func roundStep(price float64) float64 {
	return math.Pow(10, math.Floor(math.Log10(price))-1)
}

func (p *panel) eventsFor(i, d int, rec *dayRec, hour [nSlots]float64) []event {
	b := rec.bars
	closes, ok := p.priorCloses(i, d)
	if !ok {
		return nil
	}
	sorted := append([]float64(nil), closes...)
	sort.Float64s(sorted)
	hi, lo := sorted[len(sorted)-1], sorted[0]
	hi4, lo4 := sorted[len(sorted)-4], sorted[3]
	ff := forwardFill(b)
	done := map[string]bool{}
	var out []event
	prevSlot := -1
	prevClose := 0.0
	for s := 0; s < nSlots; s++ {
		if !b.ts[s] || !(b.close[s] > 0) {
			continue
		}
		cur := float64(b.close[s])
		if prevSlot >= 0 && s-prevSlot <= maxGapSlots && s <= lastEvent && s >= 1 {
			type cross struct {
				key  string
				dir  float64
				real bool
			}
			var cs []cross
			if prevClose <= hi && hi < cur {
				cs = append(cs, cross{"ext", 1, true})
			}
			if prevClose >= lo && lo > cur {
				cs = append(cs, cross{"ext", -1, true})
			}
			if prevClose <= hi4 && hi4 < cur {
				cs = append(cs, cross{"ext", 1, false})
			}
			if prevClose >= lo4 && lo4 > cur {
				cs = append(cs, cross{"ext", -1, false})
			}
			step := roundStep(prevClose)
			if math.Floor(cur/step) > math.Floor(prevClose/step) {
				cs = append(cs, cross{"rnd", 1, true})
			} else if math.Floor(cur/step) < math.Floor(prevClose/step) {
				cs = append(cs, cross{"rnd", -1, true})
			}
			if math.Floor(cur/step-0.5) > math.Floor(prevClose/step-0.5) {
				cs = append(cs, cross{"rnd", 1, false})
			} else if math.Floor(cur/step-0.5) < math.Floor(prevClose/step-0.5) {
				cs = append(cs, cross{"rnd", -1, false})
			}
			for _, c := range cs {
				k := fmt.Sprintf("%s|%v|%v", c.key, c.dir, c.real)
				if done[k] {
					continue
				}
				done[k] = true
				end := ff[s+horizonSlots]
				if math.IsNaN(end) || math.IsNaN(hour[s]) {
					continue
				}
				e := event{day: d, dir: c.dir, real: c.real, c1: nan, c2: nan}
				e.out = c.dir * (math.Log(end/cur) - hour[s])
				var n int
				var imb, cancel, put float64
				for q := s - 3; q < s; q++ {
					if q < 0 || !b.joined[q] {
						continue
					}
					n++
					imb += float64(b.imb[q])
					if c.dir > 0 {
						cancel += float64(b.cancS[q])
						put += float64(b.putS[q])
					} else {
						cancel += float64(b.cancB[q])
						put += float64(b.putB[q])
					}
				}
				if n >= 2 {
					e.c1 = c.dir * imb / float64(n)
					if put > 0 {
						e.c2 = cancel / put
					}
				}
				out = append(out, e)
			}
		}
		prevSlot, prevClose = s, cur
	}
	return out
}

func (p *panel) collectEvents(topN int) []event {
	var all []event
	for d := 21; d < len(p.dates); d++ {
		if p.excluded[d] {
			continue
		}
		adm := p.admittedAt(d, topN)
		if len(adm) == 0 {
			continue
		}
		hour := p.hourReturns(adm, d)
		for _, i := range adm {
			rec := p.recs[i][d]
			if rec == nil || rec.bars == nil {
				continue
			}
			all = append(all, p.eventsFor(i, d, rec, hour)...)
		}
	}
	return all
}

type tercileOut struct {
	diff, lo, hi       float64
	topMean            float64
	nTop, nBot         int
	datesTop, datesBot int
	diff0, diff1       float64
}

func (p *panel) tercileTest(evs []event, pick func(event) float64, real bool) tercileOut {
	var vals []float64
	var pool []event
	for _, e := range evs {
		if e.real != real {
			continue
		}
		f := pick(e)
		if math.IsNaN(f) {
			continue
		}
		vals = append(vals, f)
		pool = append(pool, e)
	}
	var t tercileOut
	if len(vals) < 30 {
		t.diff, t.lo, t.hi, t.topMean = nan, nan, nan, nan
		return t
	}
	c1, c2 := tercileCuts(vals)
	var top, bot []sample
	var top0, bot0, top1, bot1 []float64
	topDays, botDays := map[int]bool{}, map[int]bool{}
	for _, e := range pool {
		f := pick(e)
		s := sample{e.day, e.out}
		switch {
		case f >= c2:
			top = append(top, s)
			topDays[e.day] = true
			if p.half(e.day) == 0 {
				top0 = append(top0, e.out)
			} else {
				top1 = append(top1, e.out)
			}
		case f <= c1:
			bot = append(bot, s)
			botDays[e.day] = true
			if p.half(e.day) == 0 {
				bot0 = append(bot0, e.out)
			} else {
				bot1 = append(bot1, e.out)
			}
		}
	}
	t.diff, t.lo, t.hi = bootstrapDiff(top, bot, bootReps, bootSeed, 0.002, 0.998)
	tv := make([]float64, len(top))
	for i, s := range top {
		tv[i] = s.val
	}
	t.topMean = mean(tv)
	t.nTop, t.nBot, t.datesTop, t.datesBot = len(top), len(bot), len(topDays), len(botDays)
	t.diff0 = mean(top0) - mean(bot0)
	t.diff1 = mean(top1) - mean(bot1)
	return t
}

func (p *panel) reportC(w io.Writer, name string, evs []event, pick func(event) float64) verdict {
	r := p.tercileTest(evs, pick, true)
	pl := p.tercileTest(evs, pick, false)
	fmt.Fprintf(w, "%s: top tercile mean %s, top-minus-bottom %s, CI99.6 [%s, %s], halves %s / %s, events top/bottom %d/%d, dates %d/%d\n", name, pct(r.topMean), pct(r.diff), pct(r.lo), pct(r.hi), pct(r.diff0), pct(r.diff1), r.nTop, r.nBot, r.datesTop, r.datesBot)
	fmt.Fprintf(w, "  placebo control: top-minus-bottom %s, CI99.6 [%s, %s]\n", pct(pl.diff), pct(pl.lo), pct(pl.hi))
	v := verdict{name: name}
	if math.IsNaN(r.diff) {
		fmt.Fprintf(w, "  -> FAIL (too few events)\n")
		return v
	}
	excl := r.lo > 0 || r.hi < 0
	same := r.diff0*r.diff1 > 0 && r.diff0*r.diff > 0
	counts := r.nTop >= 200 && r.nBot >= 200 && r.datesTop >= 40 && r.datesBot >= 40
	placeboClean := math.IsNaN(pl.diff) || (pl.lo <= 0 && pl.hi >= 0)
	econ := r.topMean > econHurdle
	v.pass = excl && same && counts && placeboClean && econ
	fmt.Fprintf(w, "  interval excludes zero %v, same sign in halves %v, counts %v, placebo clean %v, top tercile above 30 bp %v -> %s\n", excl, same, counts, placeboClean, econ, map[bool]string{true: "PASS", false: "FAIL"}[v.pass])
	return v
}
