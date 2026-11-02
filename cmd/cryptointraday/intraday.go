package main

import (
	"fmt"
	"log"
	"math"
	"sort"
)

func runSessions(o options, w sessionWindow, contracts []string, barDir, out string) error {
	type key struct{ sym, date string }
	recs := map[key]*sessionRec{}
	for _, sym := range contracts {
		path := barFilePath(barDir, sym)
		var cur key
		var acc sessionRec
		flush := func() {
			if acc.bars > 0 {
				r := acc
				recs[cur] = &r
			}
			acc = sessionRec{}
		}
		err := streamBars(path, func(b bar) bool {
			m := minuteOfDay(b.t)
			if m < w.startMin || m > w.endMin {
				return true
			}
			d := dateOf(b.t)
			if acc.date != d {
				flush()
				acc = sessionRec{symbol: sym, date: d}
				cur = key{sym, d}
			}
			if m < w.startMin+w.lenMin {
				if acc.bars == 0 {
					acc.open = b.open
					acc.high = b.high
					acc.low = b.low
				} else {
					acc.high = math.Max(acc.high, b.high)
					acc.low = math.Min(acc.low, b.low)
				}
				acc.close = b.close
				acc.qvol += b.quoteVol
				acc.bars++
				return true
			}
			acc.close = b.close
			return true
		})
		if err != nil {
			continue
		}
		flush()
	}
	rows := make([]*sessionRec, 0, len(recs))
	for _, r := range recs {
		if r.bars == w.lenMin/5 {
			rows = append(rows, r)
		}
	}
	sort.Slice(rows, func(a, b int) bool {
		if rows[a].date != rows[b].date {
			return rows[a].date < rows[b].date
		}
		return rows[a].symbol < rows[b].symbol
	})
	if err := writeSessions(out, rows); err != nil {
		return err
	}
	fmt.Printf("sessions %s: %d records, %d contracts, %s .. %s -> %s\n",
		w.name, len(rows), len(contracts), rows[0].date, rows[len(rows)-1].date, out)
	return nil
}

func writeSessions(path string, rows []*sessionRec) error {
	f, err := createFile(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csvWriter(f)
	w.write([]string{"symbol", "date", "range_open", "range_high", "range_low", "range_close", "range_qvol"})
	for _, r := range rows {
		w.write([]string{r.symbol, r.date,
			fnum(r.open), fnum(r.high), fnum(r.low), fnum(r.close), fnum(r.qvol)})
	}
	return w.done()
}

func readSessions(path string) ([]*sessionRec, error) {
	rows, err := readCSV(path)
	if err != nil {
		return nil, err
	}
	out := make([]*sessionRec, 0, len(rows))
	for _, r := range rows {
		if len(r) < 7 {
			continue
		}
		out = append(out, &sessionRec{
			symbol: r[0], date: r[1],
			open: parseNum(r[2]), high: parseNum(r[3]), low: parseNum(r[4]),
			close: parseNum(r[5]), qvol: parseNum(r[6]),
		})
	}
	return out, nil
}

type dayMom struct {
	date    string
	net     float64
	names   int
	turn    float64
	cost    float64
	funding float64
	gross   float64
}

func runMom(o options, w sessionWindow, sessions []*sessionRec) error {
	byDate := map[string][]*sessionRec{}
	var dates []string
	for _, s := range sessions {
		if _, ok := byDate[s.date]; !ok {
			dates = append(dates, s.date)
		}
		byDate[s.date] = append(byDate[s.date], s)
	}
	sort.Strings(dates)
	vol := loadDailyVolumeOrDie(o.dailyDir, contractsOf(sessions))
	fund := loadDailyFunding(o.dailyDir, contractsOf(sessions))
	var series []dayMom
	for _, d := range dates {
		recs := byDate[d]
		if len(recs) < 40 {
			continue
		}
		type cand struct {
			rec  *sessionRec
			sig  float64
			vol  float64
			cost float64
			fund float64
		}
		var cands []cand
		for _, r := range recs {
			if r.open <= 0 || r.close <= 0 {
				continue
			}
			v := prevVolume(vol, r.symbol, d)
			if v <= 0 {
				continue
			}
			cands = append(cands, cand{
				rec:  r,
				sig:  r.close/r.open - 1,
				vol:  v,
				cost: allInCostBps(o, v),
				fund: fundAt(fund, r.symbol, d, w),
			})
		}
		if len(cands) < 40 {
			continue
		}
		sort.Slice(cands, func(a, b int) bool { return cands[a].vol > cands[b].vol })
		if o.top > 0 && len(cands) > o.top {
			cands = cands[:o.top]
		}
		vals := make([]float64, len(cands))
		for i := range cands {
			vals[i] = cands[i].sig
		}
		ranks := rankNormalise(vals)
		var longN, shortN int
		for _, r := range ranks {
			if r > 0.5-o.quantile/2 {
				longN++
			}
			if r < -(0.5 - o.quantile/2) {
				shortN++
			}
		}
		if longN == 0 || shortN == 0 {
			continue
		}
		gross := 0.0
		cost := 0.0
		funding := 0.0
		for i, c := range cands {
			wgt := 0.0
			if ranks[i] > 0.5-o.quantile/2 {
				wgt = 1 / float64(longN)
			} else if ranks[i] < -(0.5 - o.quantile/2) {
				wgt = -1 / float64(shortN)
			}
			if wgt == 0 {
				continue
			}
			ret := c.rec.close/c.rec.open - 1
			gross += wgt * ret * 1e4
			cost += 2 * math.Abs(wgt) * c.cost
			funding -= wgt * c.fund / 1e4
		}
		net := gross - cost + funding
		series = append(series, dayMom{date: d, net: net, names: len(cands), gross: gross, cost: cost, funding: funding, turn: float64(longN + shortN)})
	}
	if len(series) < 30 {
		return fmt.Errorf("mom: only %d days with a book", len(series))
	}
	var nets []float64
	var gs, cs, fs []float64
	for _, s := range series {
		nets = append(nets, s.net)
		gs = append(gs, s.gross)
		cs = append(cs, s.cost)
		fs = append(fs, s.funding)
	}
	fmt.Printf("cell O-MOM %s: days %d, gross %.2f, cost %.2f, funding %.2f, net %.2f bps/day, t %.2f, sharpe %.2f\n",
		w.name, len(series), mean(gs), mean(cs), mean(fs), mean(nets), nwT(nets, 5), sharpe(nets))
	fmt.Printf("  halves %.2f / %.2f, names/day %.0f\n", mean(nets[:len(nets)/2]), mean(nets[len(nets)/2:]), meanF(series, func(s dayMom) float64 { return float64(s.names) }))
	return nil
}

func meanF(series []dayMom, f func(dayMom) float64) float64 {
	if len(series) == 0 {
		return math.NaN()
	}
	s := 0.0
	for _, x := range series {
		s += f(x)
	}
	return s / float64(len(series))
}

func contractsOf(rows []*sessionRec) []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range rows {
		if !seen[r.symbol] {
			seen[r.symbol] = true
			out = append(out, r.symbol)
		}
	}
	sort.Strings(out)
	return out
}

func prevVolume(vol dailyVol, sym, date string) float64 {
	m, ok := vol[sym]
	if !ok {
		return 0
	}
	return m[date]
}

func fundAt(fund map[string]map[string]float64, sym, date string, w sessionWindow) float64 {
	m, ok := fund[sym]
	if !ok {
		return 0
	}
	share := 2.0 / 3.0
	if w.name == "s1" {
		share = 1.0 / 3.0
	}
	return m[date] * share
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

func sharpe(v []float64) float64 {
	s := sd(v)
	if math.IsNaN(s) || s == 0 {
		return math.NaN()
	}
	return mean(v) / s * math.Sqrt(365)
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
	var g0 float64
	for _, x := range d {
		g0 += x * x
	}
	g0 /= float64(n)
	var lr float64
	for l := 1; l <= lag && l < n; l++ {
		var g float64
		for i := l; i < n; i++ {
			g += d[i] * d[i-l]
		}
		g /= float64(n)
		lr += 2 * (1 - float64(l)/float64(lag+1)) * g
	}
	varr := (g0 + lr) / float64(n)
	if varr <= 0 {
		return math.NaN()
	}
	return m / math.Sqrt(varr)
}

func logFatalf(format string, args ...any) {
	log.Fatalf(format, args...)
}
