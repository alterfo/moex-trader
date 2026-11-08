package main

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

func parseDay(s string) (time.Time, error) {
	return time.Parse("2006-01-02", s)
}

func deltaStrike(opts []*option, entry string, forward, t float64, call bool, target float64) (*option, float64) {
	var best *option
	bestDiff := math.Inf(1)
	var bestIV float64
	for _, o := range opts {
		if (o.typ == "C") != call {
			continue
		}
		price, ok := o.settle[entry]
		if !ok {
			continue
		}
		iv, ok := impliedVol(price, forward, o.strike, t, call)
		if !ok {
			continue
		}
		d := delta(forward, o.strike, iv, t, call)
		if math.Abs(d-target) < bestDiff {
			bestDiff = math.Abs(d - target)
			best = o
			bestIV = iv
		}
	}
	return best, bestIV
}

type book struct {
	family   string
	strategy string
	dte      int
	n        int
	pnl      []float64
	capital  []float64
	days     []float64
	expiry   []string
	volume   []float64
	worst    float64
	best     float64
}

func (b *book) add(pnl, capital float64, days int, expiry string, volume float64) {
	b.pnl = append(b.pnl, pnl)
	b.capital = append(b.capital, capital)
	b.days = append(b.days, float64(days))
	b.expiry = append(b.expiry, expiry)
	b.volume = append(b.volume, volume)
	if len(b.pnl) == 1 || pnl < b.worst {
		b.worst = pnl
	}
	if len(b.pnl) == 1 || pnl > b.best {
		b.best = pnl
	}
}

func (b *book) summary() []string {
	if len(b.pnl) == 0 {
		return nil
	}
	m, t := neweyWest(b.pnl, 5)
	cap := mean(b.capital)
	days := mean(b.days)
	ann := 0.0
	if cap > 0 && days > 0 {
		ann = m / cap * 365 / days * 100
	}
	first, second := []float64{}, []float64{}
	for i, e := range b.expiry {
		if e < splitDate {
			first = append(first, b.pnl[i])
		} else {
			second = append(second, b.pnl[i])
		}
	}
	return []string{
		b.family, b.strategy, strconv.Itoa(b.dte), strconv.Itoa(len(b.pnl)),
		fmt.Sprintf("%.1f", m), fmt.Sprintf("%.1f", median(b.pnl)), fmt.Sprintf("%.1f", stdev(b.pnl)),
		fmt.Sprintf("%.2f", t), fmt.Sprintf("%.1f", b.worst), fmt.Sprintf("%.1f", cvar(b.pnl, 0.05)),
		fmt.Sprintf("%.0f", cap), fmt.Sprintf("%.1f", ann),
		strconv.Itoa(len(first)), fmt.Sprintf("%.1f", mean(first)),
		strconv.Itoa(len(second)), fmt.Sprintf("%.1f", mean(second)),
		fmt.Sprintf("%.0f", median(b.volume)),
	}
}

func hedgePnL(f *future, opts []*option, atmCall, atmPut *option, entry, expiry string, mult float64, futCost float64) (float64, float64) {
	ei, xi := f.at[entry], f.at[expiry]
	if ei < 0 || xi <= ei {
		return 0, 0
	}
	var pnl, cost float64
	lastH := 0.0
	first := true
	for i := ei; i < xi; i++ {
		d := f.dates[i]
		next := f.dates[i+1]
		fd := f.settle[d]
		civ := 0.0
		piv := 0.0
		t := float64(daysBetween(d, expiry)) / 365
		if p, ok := atmCall.settle[d]; ok {
			if iv, ok := impliedVol(p, fd, atmCall.strike, t, true); ok {
				civ = iv
			}
		}
		if p, ok := atmPut.settle[d]; ok {
			if iv, ok := impliedVol(p, fd, atmPut.strike, t, false); ok {
				piv = iv
			}
		}
		h := delta(fd, atmCall.strike, civ, t, true) + delta(fd, atmPut.strike, piv, t, false)
		if civ == 0 || piv == 0 {
			h = lastH
		}
		pnl += h * (f.settle[next] - fd) * mult
		if first || h != lastH {
			cost += math.Abs(h) * fd * mult * futCostFrac(futCost)
			first = false
		}
		lastH = h
	}
	return pnl, cost
}

func futCostFrac(v float64) float64 { return v }

var splitDate = "2025-07-01"

func runBattery(p params) error {
	specs := loadSpecs(p.dir)
	books := map[string]*book{}
	order := []string{}
	get := func(family, strategy string, dte int) *book {
		key := family + "|" + strategy + "|" + strconv.Itoa(dte)
		b, ok := books[key]
		if !ok {
			b = &book{family: family, strategy: strategy, dte: dte}
			books[key] = b
			order = append(order, key)
		}
		return b
	}
	for _, family := range p.families {
		futures := loadFutures(p.dir, family, specs, p.usdrub)
		byExpiry := loadOptions(p.dir, family)
		if len(futures) == 0 || len(byExpiry) == 0 {
			fmt.Printf("%-6s no data\n", family)
			continue
		}
		expiries := make([]string, 0, len(byExpiry))
		for e := range byExpiry {
			expiries = append(expiries, e)
		}
		sort.Strings(expiries)
		for _, dte := range p.dtes {
			for _, expiry := range expiries {
				opts := byExpiry[expiry]
				f := pickFuture(futures, expiry)
				if f == nil {
					continue
				}
				entry, call, put, ok := findEntry(f, opts, expiry, dte, 3, p.minVol, p.useSettle)
				if !ok {
					continue
				}
				fs := f.settle[entry]
				fe := f.settle[expiry]
				mult := f.mult
				days := daysBetween(entry, expiry)
				t := float64(days) / 365
				if t <= 0 {
					continue
				}
				capital := p.marginFrac * fs * mult

				prem, ok := premiumAt(call, put, entry, p.useSettle)
				if !ok {
					continue
				}
				atmCost := costsRUB(p.feeRUB, p.spreadFrac, prem, mult, 2)
				payoff := math.Abs(fe - call.strike)
				get(family, "S1_atm_straddle", dte).add((prem-payoff)*mult-atmCost, capital, days, expiry, math.Min(call.vol[entry], put.vol[entry]))

				hedge, hedgeCost := hedgePnL(f, opts, call, put, entry, expiry, mult, p.futCostFrac)
				get(family, "S4_delta_hedged", dte).add((prem-payoff)*mult-atmCost+hedge-hedgeCost, capital, days, expiry, math.Min(call.vol[entry], put.vol[entry]))

				call25, _ := deltaStrike(opts, entry, fs, t, true, 0.25)
				put25, _ := deltaStrike(opts, entry, fs, t, false, -0.25)
				if call25 != nil && put25 != nil {
					c25, ok1 := premiumOf(call25, entry, p.useSettle)
					p25, ok2 := premiumOf(put25, entry, p.useSettle)
					if ok1 && ok2 {
						stranglePrem := c25 + p25
						stranglePayoff := math.Max(fe-call25.strike, 0) + math.Max(put25.strike-fe, 0)
						strangleCost := costsRUB(p.feeRUB, p.spreadFrac, stranglePrem, mult, 2)
						get(family, "S2_strangle25", dte).add((stranglePrem-stranglePayoff)*mult-strangleCost, capital, days, expiry, math.Min(call25.vol[entry], put25.vol[entry]))
					}
				}

				call30, _ := deltaStrike(opts, entry, fs, t, true, 0.30)
				if call30 != nil {
					c30, ok := premiumOf(call30, entry, p.useSettle)
					if ok {
						ccPayoff := (fe - fs) - math.Max(fe-call30.strike, 0) + c30
						ccCost := costsRUB(p.feeRUB, p.spreadFrac, c30, mult, 1) + 2*fs*mult*p.futCostFrac
						get(family, "S3_covered_call", dte).add(ccPayoff*mult-ccCost, capital, days, expiry, call30.vol[entry])
					}
				}
			}
		}
	}

	rows := [][]string{{"family", "strategy", "dte", "n", "mean_rub", "median_rub", "sd_rub", "nw_t", "worst_rub", "cvar5_rub", "capital_rub", "ann_pct", "n_first", "mean_first", "n_second", "mean_second", "median_volume"}}
	fmt.Printf("\n%-6s %-20s %3s %5s %10s %10s %10s %6s %10s %10s %10s %7s %8s %8s\n", "family", "strategy", "dte", "n", "mean", "median", "sd", "t", "worst", "cvar5", "capital", "ann%", "1st", "2nd")
	var summaries [][]string
	for _, key := range order {
		b := books[key]
		s := b.summary()
		if s == nil {
			continue
		}
		summaries = append(summaries, s)
		rows = append(rows, s)
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i][0]+summaries[i][1]+summaries[i][2] < summaries[j][0]+summaries[j][1]+summaries[j][2]
	})
	for _, s := range summaries {
		fmt.Printf("%-6s %-20s %3s %5s %10s %10s %10s %6s %10s %10s %10s %7s %8s %8s %7s\n",
			s[0], s[1], s[2], s[3], s[4], s[5], s[6], s[7], s[8], s[9], s[10], s[11], s[13], s[15], s[16])
	}
	if err := writeCSV(filepath.Join("artifacts", "options", "battery.csv"), rows); err != nil {
		return err
	}
	tradeRows := [][]string{{"family", "strategy", "dte", "expiry", "pnl_rub", "capital_rub", "days"}}
	for _, key := range order {
		b := books[key]
		for i := range b.pnl {
			tradeRows = append(tradeRows, []string{b.family, b.strategy, strconv.Itoa(b.dte), b.expiry[i],
				fmt.Sprintf("%.1f", b.pnl[i]), fmt.Sprintf("%.1f", b.capital[i]), fmt.Sprintf("%.0f", b.days[i]),
				fmt.Sprintf("%.0f", b.volume[i])})
		}
	}
	if err := writeCSV(filepath.Join("artifacts", "options", "trades.csv"), tradeRows); err != nil {
		return err
	}
	for _, strategy := range []string{"S1_atm_straddle", "S2_strangle25", "S3_covered_call", "S4_delta_hedged"} {
		var pooled []float64
		var caps, dss []float64
		for _, b := range books {
			if b.strategy != strategy {
				continue
			}
			pooled = append(pooled, b.pnl...)
			caps = append(caps, b.capital...)
			dss = append(dss, b.days...)
		}
		if len(pooled) == 0 {
			continue
		}
		m, t := neweyWest(pooled, 5)
		var months []string
		for _, b := range books {
			if b.strategy != strategy {
				continue
			}
			for _, e := range b.expiry {
				months = append(months, e[:7])
			}
		}
		lo, hi := blockBootstrap(pooled, months, 2000)
		fmt.Printf("pooled %-20s n %4d mean %8.1f t %6.2f bs95 [%8.1f, %8.1f] worst %9.1f cvar5 %9.1f\n", strategy, len(pooled), m, t, lo, hi, minOf(pooled), cvar(pooled, 0.05))
	}
	return nil
}

func minOf(xs []float64) float64 {
	m := math.Inf(1)
	for _, x := range xs {
		if x < m {
			m = x
		}
	}
	return m
}

func blockBootstrap(pnl []float64, months []string, draws int) (float64, float64) {
	blocks := map[string][]float64{}
	for i, m := range months {
		blocks[m] = append(blocks[m], pnl[i])
	}
	keys := make([]string, 0, len(blocks))
	for k := range blocks {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	rng := uint64(42)
	next := func(n int) int {
		rng = rng*6364136223846793005 + 1442695040888963407
		return int((rng >> 33) % uint64(n))
	}
	var means []float64
	for d := 0; d < draws; d++ {
		var sum float64
		var count int
		for i := 0; i < len(keys); i++ {
			b := blocks[keys[next(len(keys))]]
			for _, v := range b {
				sum += v
				count++
			}
		}
		if count > 0 {
			means = append(means, sum/float64(count))
		}
	}
	sort.Float64s(means)
	if len(means) == 0 {
		return 0, 0
	}
	return means[len(means)*25/1000], means[len(means)*975/1000]
}
