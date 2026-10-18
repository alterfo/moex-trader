package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/icaport"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

const (
	window       = 750
	rebalanceGap = 21
	costPerSide  = 0.0005
	seed         = 42
	bootRepeats  = 2000
	bootBlock    = 20
)

type market struct {
	dates   []time.Time
	tickers []string
	simple  [][]float64
	logPct  [][]float64
	has     [][]bool
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("icaportfolio", flag.ContinueOnError)
	configPath := fs.String("config", "config.yaml", "config path")
	fromStr := fs.String("from", "2015-01-01", "history start")
	tillStr := fs.String("till", "2026-10-07", "history end")
	testFromStr := fs.String("test-from", "2019-01-02", "first rebalance date")
	outDir := fs.String("out", "icaportfolio-out", "output directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	from, err := time.Parse("2006-01-02", *fromStr)
	if err != nil {
		return err
	}
	till, err := time.Parse("2006-01-02", *tillStr)
	if err != nil {
		return err
	}
	testFrom, err := time.Parse("2006-01-02", *testFromStr)
	if err != nil {
		return err
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	var equities []string
	for _, t := range cfg.Tickers {
		if !strings.HasSuffix(t, "_TOM") {
			equities = append(equities, t)
		}
	}
	m, err := loadMarket(cfg.MOEXISSBaseURL, equities, from, till)
	if err != nil {
		return err
	}
	log.Printf("icaportfolio: %d tickers, %d dates %s..%s", len(m.tickers), len(m.dates), m.dates[0].Format("2006-01-02"), m.dates[len(m.dates)-1].Format("2006-01-02"))

	start := sort.Search(len(m.dates), func(i int) bool { return !m.dates[i].Before(testFrom) })
	if start < window {
		return fmt.Errorf("first rebalance index %d is below the %d-day window", start, window)
	}

	rungs := []string{"R0_equal", "R1_invvol", "R2_minvar", "R3_ica_garch", "R4_ica_garchsk", "R4_rho5", "R4_mu0"}
	daily := map[string][]float64{}
	turnover := map[string]float64{}
	rebalances := 0
	held := map[string][]float64{}
	for i := start; i < len(m.dates); i += rebalanceGap {
		eligible := m.eligible(i)
		if len(eligible) < 3 {
			return fmt.Errorf("only %d eligible tickers at %s", len(eligible), m.dates[i].Format("2006-01-02"))
		}
		win := make(icaport.Mat, window)
		for r := 0; r < window; r++ {
			row := make([]float64, len(eligible))
			for k, j := range eligible {
				row[k] = m.logPct[i-window+r][j]
			}
			win[r] = row
		}
		weights := map[string][]float64{}
		weights["R0_equal"] = icaport.EqualWeights(len(eligible))
		weights["R1_invvol"] = icaport.InverseVolWeights(win, 63)
		weights["R2_minvar"] = icaport.MinVarianceWeights(win, seed)
		base := icaport.ICAConfig{Horizon: rebalanceGap, Rho: 1, Higher: true, UseMean: true, Seed: seed, Restarts: 3}
		r3 := base
		r3.Higher = false
		if w, err := icaport.ICAWeights(win, r3); err == nil {
			weights["R3_ica_garch"] = w
		}
		if obj, err := icaport.BuildICAObjective(win, base); err == nil {
			starts := icaport.DefaultStarts(len(eligible), seed, 3, icaport.InverseVolWeights(win, 63))
			weights["R4_ica_garchsk"] = icaport.MinimizeSimplex(obj, starts, 1500)
			r5 := obj
			r5.Rho = 5
			weights["R4_rho5"] = icaport.MinimizeSimplex(r5, starts, 1500)
			r0 := obj
			r0.Mu = nil
			weights["R4_mu0"] = icaport.MinimizeSimplex(r0, starts, 1500)
		}
		for _, name := range rungs {
			target, ok := weights[name]
			if !ok {
				target = held[name]
				if target == nil {
					target = icaport.EqualWeights(len(eligible))
				}
			}
			full := make([]float64, len(m.tickers))
			for k, j := range eligible {
				full[j] = target[k]
			}
			prev := held[name]
			var turn float64
			for j := range full {
				var p float64
				if prev != nil {
					p = prev[j]
				}
				turn += math.Abs(full[j] - p)
			}
			turnover[name] += turn
			held[name] = full
			end := i + rebalanceGap
			if end > len(m.dates) {
				end = len(m.dates)
			}
			cur := append([]float64(nil), full...)
			for d := i; d < end; d++ {
				var r float64
				for j := range cur {
					r += cur[j] * m.simple[d][j]
				}
				if d == i {
					r -= costPerSide * turn
				}
				daily[name] = append(daily[name], r)
				var sum float64
				for j := range cur {
					cur[j] *= 1 + m.simple[d][j]
					sum += cur[j]
				}
				if sum > 0 {
					for j := range cur {
						cur[j] /= sum
					}
				}
			}
			held[name] = cur
		}
		rebalances++
	}
	return report(rungs, daily, turnover, rebalances, *outDir)
}

func report(rungs []string, daily map[string][]float64, turnover map[string]float64, rebalances int, outDir string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	fmt.Printf("%-16s %9s %8s %8s %8s %7s %9s\n", "rung", "ann.ret", "ann.vol", "CVaR95", "maxDD", "Sharpe", "turn/reb")
	for _, name := range rungs {
		p := icaport.Evaluate(daily[name])
		fmt.Printf("%-16s %8.2f%% %7.2f%% %7.3f%% %7.2f%% %7.2f %9.3f\n", name, p.AnnReturn*100, p.AnnVol*100, p.CVaR95*100, p.MaxDD*100, p.Sharpe, turnover[name]/float64(rebalances))
	}
	cvar := func(x []float64) float64 { return icaport.CVaR(x, 0.95) }
	maxdd := func(x []float64) float64 { return icaport.Evaluate(x).MaxDD }
	pairs := [][2]string{{"R1_invvol", "R0_equal"}, {"R2_minvar", "R0_equal"}, {"R3_ica_garch", "R0_equal"}, {"R4_ica_garchsk", "R0_equal"}, {"R4_ica_garchsk", "R3_ica_garch"}, {"R4_ica_garchsk", "R2_minvar"}, {"R4_rho5", "R0_equal"}, {"R4_mu0", "R0_equal"}}
	fmt.Println()
	fmt.Printf("%-34s %24s %24s %12s\n", "pair (strategy / control)", "CVaR95 ratio [CI95]", "maxDD ratio [CI95]", "ann.ret diff")
	for _, pr := range pairs {
		s, c := daily[pr[0]], daily[pr[1]]
		cv := icaport.BootstrapRatios(s, c, bootRepeats, bootBlock, seed, cvar)
		dd := icaport.BootstrapRatios(s, c, bootRepeats, bootBlock, seed, maxdd)
		diff := (icaport.Evaluate(s).AnnReturn - icaport.Evaluate(c).AnnReturn) * 100
		fmt.Printf("%-34s %6.3f [%5.3f, %5.3f] %6.3f [%5.3f, %5.3f] %+10.2fpp\n", pr[0]+" / "+pr[1], cv.Point, cv.Lo, cv.Hi, dd.Point, dd.Lo, dd.Hi, diff)
	}
	for _, name := range rungs {
		f, err := os.Create(filepath.Join(outDir, name+".csv"))
		if err != nil {
			return err
		}
		w := csv.NewWriter(f)
		for _, r := range daily[name] {
			_ = w.Write([]string{fmt.Sprintf("%.8f", r)})
		}
		w.Flush()
		f.Close()
	}
	return nil
}

func (m *market) eligible(i int) []int {
	var out []int
	for j := range m.tickers {
		if !m.has[i][j] {
			continue
		}
		count := 0
		for r := i - window; r < i; r++ {
			if m.has[r][j] {
				count++
			}
		}
		if count >= window-10 {
			out = append(out, j)
		}
	}
	return out
}

func loadMarket(base string, tickers []string, from, till time.Time) (*market, error) {
	source := backtest.NewISSSource(base, moex.NewClient(base, nil))
	type series map[string]float64
	closes := map[string]series{}
	dateSet := map[string]time.Time{}
	var usable []string
	for _, t := range tickers {
		candles, err := source.History(context.Background(), t, from, till)
		if err != nil || len(candles) < window+300 {
			log.Printf("icaportfolio: skip %s (%d candles, err %v)", t, len(candles), err)
			continue
		}
		s := series{}
		for _, c := range candles {
			key := c.Begin.Format("2006-01-02")
			v, _ := c.Close.Float64()
			if v > 0 {
				s[key] = v
				dateSet[key] = c.Begin
			}
		}
		closes[t] = s
		usable = append(usable, t)
	}
	sort.Strings(usable)
	var dates []time.Time
	for _, d := range dateSet {
		dates = append(dates, d)
	}
	sort.Slice(dates, func(a, b int) bool { return dates[a].Before(dates[b]) })
	m := &market{dates: dates, tickers: usable}
	m.simple = make([][]float64, len(dates))
	m.logPct = make([][]float64, len(dates))
	m.has = make([][]bool, len(dates))
	last := make([]float64, len(usable))
	for i, d := range dates {
		m.simple[i] = make([]float64, len(usable))
		m.logPct[i] = make([]float64, len(usable))
		m.has[i] = make([]bool, len(usable))
		key := d.Format("2006-01-02")
		for j, t := range usable {
			p, ok := closes[t][key]
			if !ok {
				continue
			}
			if last[j] > 0 {
				m.simple[i][j] = p/last[j] - 1
				m.logPct[i][j] = 100 * math.Log(p/last[j])
				m.has[i][j] = true
			}
			last[j] = p
		}
	}
	return m, nil
}
