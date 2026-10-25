package main

import (
	"fmt"
	"math"
)

const (
	borrowPerCalendarDay = 0.00074
	holdDays             = 5
	calendarDaysPerHold  = 7
	entryZ               = 1.5
)

func economics(sr []*series, win string) {
	for _, withBorrow := range []bool{true, false} {
		var net, gross []float64
		perYear := map[string][]float64{}
		pairsN := map[string]int{}
		for k := range sr {
			s := sr[k]
			if s.spec.control {
				continue
			}
			for i := 0; i < s.n; {
				d := s.dates[i]
				if (win == "rt" && d < split) || math.IsNaN(s.z[i]) || math.IsNaN(s.y5[i]) || math.Abs(s.z[i]) < entryZ {
					i++
					continue
				}
				sign := -1.0
				if s.z[i] < 0 {
					sign = 1
				}
				g := sign * s.y5[i]
				cost := 2 * (sideCost(s.spOrd[i]) + sideCost(s.spPref[i]))
				bor := 0.0
				if withBorrow {
					bor = borrowPerCalendarDay * calendarDaysPerHold
				}
				n := g - cost - bor
				net = append(net, n)
				gross = append(gross, g)
				perYear[d[:4]] = append(perYear[d[:4]], n)
				pairsN[s.spec.id]++
				i += holdDays
			}
		}
		lo, hi := bootMean(net, 5000, 20261025)
		label := "with the broker carry on the short leg (0.074% per calendar day, 7 days)"
		if !withBorrow {
			label = "without carry (a short leg that costs nothing: a futures hedge, for illustration)"
		}
		fmt.Printf("\neconomics [%s] |z| >= %.1f, hold %d trading days, non-overlapping, both legs round trip at the day's BBO spread, %s\n", win, entryZ, holdDays, label)
		if len(net) == 0 {
			fmt.Println("  no trades")
			continue
		}
		fmt.Printf("  trades %d, mean gross %+.3f%%, mean net %+.3f%% per trade (per unit of one leg's notional), bootstrap 95%% [%+.3f%%, %+.3f%%], share of winning trades %.0f%%\n", len(net), mean(gross)*100, mean(net)*100, lo*100, hi*100, share(net)*100)
		for _, y := range []string{"2020", "2021", "2022", "2023", "2024", "2025", "2026"} {
			if v := perYear[y]; len(v) > 0 {
				fmt.Printf("  %s: %3d trades, mean net %+.3f%%\n", y, len(v), mean(v)*100)
			}
		}
	}
}

func share(a []float64) float64 {
	c := 0
	for _, x := range a {
		if x > 0 {
			c++
		}
	}
	return float64(c) / float64(len(a))
}
