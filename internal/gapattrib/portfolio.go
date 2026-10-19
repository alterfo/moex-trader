package gapattrib

import (
	"math"
	"sort"
	"time"
)

const (
	Cost            = 0.0015
	BorrowPerDay    = 0.00005
	Step            = 21
	firstPosition   = 300
	liveBuyR        = 0.8
	liveSellR       = 0.2
	LiveNotionalRUB = 15000.0
	DepositRUB      = 1_000_000.0
)

type Inputs struct {
	Panel  *Panel
	Adj    *Adjusted
	Scores *Scores
	Live   map[string]bool
	Rate   *KeyRate
}

type Period struct {
	Date     time.Time
	End      time.Time
	Names    int
	Longs    int
	Shorts   int
	Gross    float64
	CostAmt  float64
	LongPnL  float64
	ShortPnL float64
	LongCap  float64
	ShortCap float64
	Net      float64
	Capital  float64
	Carry    float64
	CalDays  int
}

func (p Period) NetPerCapital() float64 {
	if p.Capital == 0 {
		return 0
	}
	return p.Net / p.Capital
}

func (p Period) ExcessPerCapital() float64 {
	if p.Capital == 0 {
		return 0
	}
	return (p.Net - p.Carry) / p.Capital
}

type Rung string

const (
	RungResearch     Rung = "R0 research quintiles, eligible universe"
	RungLiveNames    Rung = "R1 quintiles restricted to the live tickers"
	RungLiveLegMean  Rung = "R1b live thresholds r>=0.8 / r<=0.2, equal weight per leg"
	RungLiveRule     Rung = "R2 live rule r>=0.8 / r<=0.2 on live tickers, unit weights"
	RungLiveNotional Rung = "R3 live rule at 15000 RUB per name"
)

type state struct {
	prevLong  map[int]bool
	prevShort map[int]bool
}

func positions(in *Inputs) []int {
	var out []int
	for p := firstPosition; p < len(in.Panel.Dates)-Step-2; p += Step {
		out = append(out, p)
	}
	return out
}

func scoreRow(in *Inputs, day string) ([]float64, bool) {
	i := sort.SearchStrings(in.Scores.Dates, day)
	if i >= len(in.Scores.Dates) || in.Scores.Dates[i] != day {
		return nil, false
	}
	row := make([]float64, len(in.Panel.Tickers))
	for j := range row {
		row[j] = math.NaN()
	}
	index := in.Panel.TickerIndex()
	for k, ticker := range in.Scores.Tickers {
		if j, ok := index[ticker]; ok {
			row[j] = in.Scores.R[i][k]
		}
	}
	return row, true
}

func forward(in *Inputs, p int) []float64 {
	cols := len(in.Panel.Tickers)
	out := make([]float64, cols)
	for j := 0; j < cols; j++ {
		entry := in.Adj.Open[p+1][j]
		if math.IsNaN(entry) {
			entry = in.Adj.Close[p][j]
		}
		v := in.Adj.Fill[p+Step][j]/entry - 1
		if math.IsNaN(v) || math.IsInf(v, 0) {
			v = 0
		}
		out[j] = v
	}
	return out
}

func mean(v []float64, idx []int) float64 {
	if len(idx) == 0 {
		return 0
	}
	s := 0.0
	for _, j := range idx {
		s += v[j]
	}
	return s / float64(len(idx))
}

func symDiff(a, b map[int]bool) int {
	n := 0
	for k := range a {
		if !b[k] {
			n++
		}
	}
	for k := range b {
		if !a[k] {
			n++
		}
	}
	return n
}

func toSet(idx []int) map[int]bool {
	m := make(map[int]bool, len(idx))
	for _, j := range idx {
		m[j] = true
	}
	return m
}

func quintiles(names []int, r []float64) (top, bot []int) {
	ranked := append([]int(nil), names...)
	sort.SliceStable(ranked, func(a, b int) bool { return r[ranked[a]] < r[ranked[b]] })
	k := len(ranked) / 5
	if k < 1 {
		k = 1
	}
	return ranked[len(ranked)-k:], ranked[:k]
}

func Run(in *Inputs, rung Rung, from, till time.Time) []Period {
	var out []Period
	st := state{}
	for _, p := range positions(in) {
		date := in.Panel.Dates[p]
		if date.Before(from) || date.After(till) {
			continue
		}
		r, ok := scoreRow(in, date.Format("2006-01-02"))
		if !ok {
			continue
		}
		var names, liveNames []int
		for j := range in.Panel.Tickers {
			if math.IsNaN(r[j]) || !in.Panel.Eligible(p, j) {
				continue
			}
			names = append(names, j)
			if in.Live[in.Panel.Tickers[j]] {
				liveNames = append(liveNames, j)
			}
		}
		if len(names) < minNamesPerDay {
			continue
		}
		fwd := forward(in, p)
		end := in.Panel.Dates[p+Step]
		days := int(end.Sub(date).Hours() / 24)
		carryRate := in.Rate.Carry(date, end)
		period := Period{Date: date, End: end, Names: len(names), CalDays: days}
		switch rung {
		case RungResearch, RungLiveNames:
			pool := names
			if rung == RungLiveNames {
				pool = liveNames
			}
			if len(pool) == 0 {
				continue
			}
			top, bot := quintiles(pool, r)
			tnLong, tnShort := 1.0, 1.0
			if st.prevLong != nil {
				tnLong = float64(symDiff(toSet(top), st.prevLong)) / float64(len(top))
				tnShort = float64(symDiff(toSet(bot), st.prevShort)) / float64(len(bot))
			}
			st.prevLong, st.prevShort = toSet(top), toSet(bot)
			gross := mean(fwd, top) - mean(fwd, bot)
			cost := Cost*(tnLong+tnShort) + BorrowPerDay*float64(days)
			period.Longs, period.Shorts = len(top), len(bot)
			period.CostAmt = cost
			period.Gross, period.Net = gross, gross-cost
			period.Capital = 2
			period.Carry = carryRate * 2
		case RungLiveLegMean:
			var longs, shorts []int
			for _, j := range liveNames {
				switch {
				case r[j] >= liveBuyR:
					longs = append(longs, j)
				case r[j] <= liveSellR:
					shorts = append(shorts, j)
				}
			}
			tnLong, tnShort := 1.0, 1.0
			if st.prevLong != nil {
				if len(longs) > 0 {
					tnLong = float64(symDiff(toSet(longs), st.prevLong)) / float64(len(longs))
				}
				if len(shorts) > 0 {
					tnShort = float64(symDiff(toSet(shorts), st.prevShort)) / float64(len(shorts))
				}
			}
			st.prevLong, st.prevShort = toSet(longs), toSet(shorts)
			legs := 0.0
			cost := 0.0
			gross := 0.0
			if len(longs) > 0 {
				legs++
				gross += mean(fwd, longs)
				cost += Cost * tnLong
			}
			if len(shorts) > 0 {
				legs++
				gross -= mean(fwd, shorts)
				cost += Cost*tnShort + BorrowPerDay*float64(days)
			}
			period.Longs, period.Shorts = len(longs), len(shorts)
			period.LongPnL, period.ShortPnL = mean(fwd, longs), -mean(fwd, shorts)
			if len(longs) > 0 {
				period.LongCap = 1
			}
			if len(shorts) > 0 {
				period.ShortCap = 1
			}
			period.CostAmt = cost
			period.Gross, period.Net = gross, gross-cost
			period.Capital = legs
			period.Carry = carryRate * legs
		case RungLiveRule, RungLiveNotional:
			var longs, shorts []int
			for _, j := range liveNames {
				switch {
				case r[j] >= liveBuyR:
					longs = append(longs, j)
				case r[j] <= liveSellR:
					shorts = append(shorts, j)
				}
			}
			unit := 1.0
			if rung == RungLiveNotional {
				unit = LiveNotionalRUB
			}
			ls, ss := toSet(longs), toSet(shorts)
			traded := len(longs) + len(shorts)
			if st.prevLong != nil {
				traded = symDiff(ls, st.prevLong) + symDiff(ss, st.prevShort)
			}
			st.prevLong, st.prevShort = ls, ss
			var sumL, sumS float64
			for _, j := range longs {
				sumL += fwd[j]
			}
			for _, j := range shorts {
				sumS += fwd[j]
			}
			gross := unit * (sumL - sumS)
			cost := unit*Cost*float64(traded) + unit*BorrowPerDay*float64(days)*float64(len(shorts))
			period.Longs, period.Shorts = len(longs), len(shorts)
			period.LongPnL, period.ShortPnL = unit*sumL, -unit*sumS
			period.LongCap, period.ShortCap = unit*float64(len(longs)), unit*float64(len(shorts))
			period.CostAmt = cost
			period.Gross, period.Net = gross, gross-cost
			period.Capital = unit * float64(len(longs)+len(shorts))
			period.Carry = carryRate * period.Capital
		}
		out = append(out, period)
	}
	return out
}

type Summary struct {
	Periods          int
	MeanNet          float64
	MeanNetPerCap    float64
	MeanCarryPerCap  float64
	MeanExcessPerCap float64
	MeanNames        float64
	MeanLongs        float64
	MeanShorts       float64
	SumNet           float64
	MeanCapital      float64
	TStat            float64
	LongPerCap       float64
	ShortPerCap      float64
	GrossPerCap      float64
	CostPerCap       float64
	TPerCap          float64
}

func Summarize(periods []Period) Summary {
	s := Summary{Periods: len(periods)}
	if len(periods) == 0 {
		return s
	}
	var nets, perCap []float64
	var sumCap, sumCarry, sumGross, sumCost, sumLongPnL, sumLongCap, sumShortPnL, sumShortCap float64
	for _, p := range periods {
		s.SumNet += p.Net
		s.MeanNames += float64(p.Names)
		s.MeanLongs += float64(p.Longs)
		s.MeanShorts += float64(p.Shorts)
		sumCap += p.Capital
		sumCarry += p.Carry
		sumGross += p.Gross
		sumCost += p.CostAmt
		sumLongPnL += p.LongPnL
		sumLongCap += p.LongCap
		sumShortPnL += p.ShortPnL
		sumShortCap += p.ShortCap
		nets = append(nets, p.Net)
		perCap = append(perCap, p.NetPerCapital())
	}
	n := float64(len(periods))
	s.MeanNet = s.SumNet / n
	s.MeanNames /= n
	s.MeanLongs /= n
	s.MeanShorts /= n
	s.MeanCapital = sumCap / n
	if sumCap != 0 {
		s.MeanNetPerCap = s.SumNet / sumCap
		s.MeanCarryPerCap = sumCarry / sumCap
		s.MeanExcessPerCap = (s.SumNet - sumCarry) / sumCap
		s.GrossPerCap = sumGross / sumCap
		s.CostPerCap = sumCost / sumCap
	}
	if sumLongCap != 0 {
		s.LongPerCap = sumLongPnL / sumLongCap
	}
	if sumShortCap != 0 {
		s.ShortPerCap = sumShortPnL / sumShortCap
	}
	s.TStat = neweyWestT(nets, 3)
	s.TPerCap = neweyWestT(perCap, 3)
	return s
}

func neweyWestT(x []float64, lags int) float64 {
	n := len(x)
	if n < 3 {
		return 0
	}
	m := 0.0
	for _, v := range x {
		m += v
	}
	m /= float64(n)
	gamma := func(l int) float64 {
		s := 0.0
		for i := l; i < n; i++ {
			s += (x[i] - m) * (x[i-l] - m)
		}
		return s / float64(n)
	}
	variance := gamma(0)
	for l := 1; l <= lags && l < n; l++ {
		variance += 2 * (1 - float64(l)/float64(lags+1)) * gamma(l)
	}
	if variance <= 0 {
		return 0
	}
	return m / math.Sqrt(variance/float64(n))
}
