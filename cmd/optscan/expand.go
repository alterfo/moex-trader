package main

import (
	"fmt"
	"math"
	"sort"
	"strconv"
)

type signalRow struct {
	year   string
	expiry string
	signal float64
	pnl    float64
	vol    float64
}

func trailingVol(f *future, entry string, lookback int) float64 {
	idx, ok := f.at[entry]
	if !ok || idx <= lookback {
		return 0
	}
	var rets []float64
	prev := f.settle[f.dates[idx-lookback]]
	for i := idx - lookback + 1; i <= idx; i++ {
		cur := f.settle[f.dates[i]]
		if prev > 0 && cur > 0 {
			rets = append(rets, math.Log(cur/prev))
		}
		prev = cur
	}
	return stdev(rets) * math.Sqrt(252)
}

func straddlePnL(p params, f *future, call, put *option, entry, expiry string) (float64, float64, bool) {
	prem, ok := premiumAt(call, put, entry, p.useSettle)
	if !ok {
		return 0, 0, false
	}
	fe, ok := f.settle[expiry]
	if !ok {
		return 0, 0, false
	}
	cost := costsRUB(p.feeRUB, p.spreadFrac, prem, f.mult, 2)
	return (prem-math.Abs(fe-call.strike))*f.mult - cost, prem, true
}

func runFilter(p params, thresholds []float64) error {
	specs := loadSpecs(p.dir)
	fmt.Printf("\n%-6s %4s %6s %8s %10s %10s %8s\n", "family", "dte", "n", "signal", "mean_pnl", "iv-rv20", "corr")
	for _, family := range p.families {
		futures := loadFutures(p.dir, family, specs, p.usdrub)
		byExpiry := loadOptions(p.dir, family)
		if len(futures) == 0 || len(byExpiry) == 0 {
			continue
		}
		for _, dte := range p.dtes {
			var rows []signalRow
			for expiry, opts := range byExpiry {
				f := pickFuture(futures, expiry)
				if f == nil {
					continue
				}
				entry, call, put, ok := findEntry(f, opts, expiry, dte, 3, p.minVol, p.useSettle)
				if !ok {
					continue
				}
				pnl, _, ok := straddlePnL(p, f, call, put, entry, expiry)
				if !ok {
					continue
				}
				fs := f.settle[entry]
				t := float64(daysBetween(entry, expiry)) / 365
				civ, ok1 := impliedVol(call.settle[entry], fs, call.strike, t, true)
				piv, ok2 := impliedVol(put.settle[entry], fs, put.strike, t, false)
				if !ok1 || !ok2 {
					continue
				}
				rv := trailingVol(f, entry, 20)
				if rv <= 0 {
					continue
				}
				if p.debug {
					fmt.Printf("dbg %s fut %s mult %.1f entry %s expiry %s K %.1f F %.2f FE %.2f prem %.2f pnl %.1f\n", family, f.id, f.mult, entry, expiry, call.strike, fs, f.settle[expiry], mustPrem(p, call, put, entry), pnl)
				}
				rows = append(rows, signalRow{year: expiry[:4], expiry: expiry,
					signal: 0.5*(civ+piv) - rv, pnl: pnl, vol: math.Min(call.vol[entry], put.vol[entry])})
			}
			if len(rows) < 10 {
				fmt.Printf("%-6s %4d too few (%d)\n", family, dte, len(rows))
				continue
			}
			sort.Slice(rows, func(i, j int) bool { return rows[i].signal < rows[j].signal })
			var pnls, sigs []float64
			for _, r := range rows {
				pnls = append(pnls, r.pnl)
				sigs = append(sigs, r.signal)
			}
			mP, mS := mean(pnls), mean(sigs)
			var cov, vx, vy float64
			for i := range rows {
				dx, dy := sigs[i]-mS, pnls[i]-mP
				cov += dx * dy
				vx += dx * dx
				vy += dy * dy
			}
			corr := 0.0
			if vx > 0 && vy > 0 {
				corr = cov / math.Sqrt(vx*vy)
			}
			slope := 0.0
			if vx > 0 {
				slope = cov / vx
			}
			fmt.Printf("%-6s %4d %6d %8.3f %10.1f %10.3f %8.2f slope %.0f\n", family, dte, len(rows), mS, mP, mS, corr, slope)
			for _, y := range []string{"2023", "2024", "2025", "2026"} {
				var py []float64
				var ps []float64
				for _, r := range rows {
					if r.year == y {
						py = append(py, r.pnl)
						ps = append(ps, r.signal)
					}
				}
				if len(py) > 0 {
					fmt.Printf("       %s n %3d signal %6.3f pnl %8.1f\n", y, len(py), mean(ps), mean(py))
				}
			}
			for _, th := range thresholds {
				var sel []float64
				var selYears []string
				for _, r := range rows {
					if r.signal >= th {
						sel = append(sel, r.pnl)
						selYears = append(selYears, r.year)
					}
				}
				if len(sel) < 5 {
					fmt.Printf("       threshold %4.2f: n %d (too few)\n", th, len(sel))
					continue
				}
				m, t := neweyWest(sel, 5)
				first, second := []float64{}, []float64{}
				for i, y := range selYears {
					if y+"-07-01" <= splitDate {
						first = append(first, sel[i])
					} else {
						second = append(second, sel[i])
					}
				}
				fmt.Printf("       threshold %4.2f: n %3d mean %8.1f t %6.2f halves %8.1f/%8.1f\n", th, len(sel), m, t, mean(first), mean(second))
				for _, y := range []string{"2025", "2026"} {
					var py []float64
					for i, ry := range selYears {
						if ry == y {
							py = append(py, sel[i])
						}
					}
					if len(py) > 0 {
						fmt.Printf("         %s n %3d mean %8.1f\n", y, len(py), mean(py))
					}
				}
			}
		}
	}
	return nil
}

func runLadder(p params, maxStrikes int) error {
	specs := loadSpecs(p.dir)
	fmt.Printf("\n%-6s %4s %5s %10s %10s %10s %10s\n", "family", "dte", "n", "pnl_1", "pnl_multi", "vol_1", "vol_multi")
	for _, family := range p.families {
		futures := loadFutures(p.dir, family, specs, p.usdrub)
		byExpiry := loadOptions(p.dir, family)
		if len(futures) == 0 || len(byExpiry) == 0 {
			continue
		}
		for _, dte := range p.dtes {
			var one, multi, vol1, volMulti []float64
			var years []string
			for expiry, opts := range byExpiry {
				f := pickFuture(futures, expiry)
				if f == nil {
					continue
				}
				entry, call, put, ok := findEntry(f, opts, expiry, dte, 3, p.minVol, p.useSettle)
				if !ok {
					continue
				}
				pnl1, _, ok := straddlePnL(p, f, call, put, entry, expiry)
				if !ok {
					continue
				}
				fs := f.settle[entry]
				best1 := math.Min(call.vol[entry], put.vol[entry])
				var sumPnL, sumVol float64
				used := 0
				for _, o := range opts {
					if o.typ != "C" {
						continue
					}
					pk := putAt(opts, o.strike)
					if pk == nil {
						continue
					}
					if _, ok := o.wap[entry]; !ok {
						continue
					}
					if _, ok := pk.wap[entry]; !ok {
						continue
					}
					if o.vol[entry] < p.minVol || pk.vol[entry] < p.minVol {
						continue
					}
					if math.Abs(o.strike-fs)/fs > 0.06 {
						continue
					}
					pn, _, ok := straddlePnL(p, f, o, pk, entry, expiry)
					if !ok {
						continue
					}
					sumPnL += pn
					sumVol += math.Min(o.vol[entry], pk.vol[entry])
					used++
					if used >= maxStrikes {
						break
					}
				}
				if used < 2 {
					continue
				}
				one = append(one, pnl1)
				multi = append(multi, sumPnL)
				vol1 = append(vol1, best1)
				volMulti = append(volMulti, sumVol)
				years = append(years, expiry[:4])
			}
			if len(one) < 5 {
				fmt.Printf("%-6s %4d too few (%d)\n", family, dte, len(one))
				continue
			}
			m1, _ := neweyWest(one, 5)
			mm, t := neweyWest(multi, 5)
			fmt.Printf("%-6s %4d %5d %10.1f %10.1f %10.0f %10.0f t %.2f\n", family, dte, len(one), m1, mm, mean(vol1), mean(volMulti), t)
			for _, y := range []string{"2023", "2024", "2025", "2026"} {
				var py []float64
				for i, ry := range years {
					if ry == y {
						py = append(py, multi[i])
					}
				}
				if len(py) > 0 {
					fmt.Printf("       %s n %3d multi %8.1f\n", y, len(py), mean(py))
				}
			}
		}
	}
	return nil
}

func putAt(opts []*option, strike float64) *option {
	for _, o := range opts {
		if o.typ == "P" && o.strike == strike {
			return o
		}
	}
	return nil
}

func filterThresholds(s string) []float64 {
	var out []float64
	for _, part := range splitStrings(s, ",") {
		if v, err := strconv.ParseFloat(part, 64); err == nil {
			out = append(out, v)
		}
	}
	return out
}

func splitStrings(s, sep string) []string {
	var out []string
	start := 0
	for i := 0; i+len(sep) <= len(s); i++ {
		if s[i:i+len(sep)] == sep {
			out = append(out, s[start:i])
			start = i + len(sep)
			i += len(sep) - 1
		}
	}
	out = append(out, s[start:])
	return out
}

func mustPrem(p params, call, put *option, day string) float64 {
	v, _ := premiumAt(call, put, day, p.useSettle)
	return v
}
