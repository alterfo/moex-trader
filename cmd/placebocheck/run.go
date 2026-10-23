package main

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/features"
	"github.com/olegsidorkin/moex-trader/internal/model"
	"github.com/olegsidorkin/moex-trader/internal/placebo"
)

func splitComma(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parseWindows(w2From, w2Till, w1From, w1Till string) ([]window, error) {
	parse := func(name, from, till string) (window, error) {
		f, err := time.Parse("2006-01-02", from)
		if err != nil {
			return window{}, fmt.Errorf("parse %s from: %w", name, err)
		}
		t, err := time.Parse("2006-01-02", till)
		if err != nil {
			return window{}, fmt.Errorf("parse %s till: %w", name, err)
		}
		return window{Name: name, From: f, Till: t}, nil
	}
	w2, err := parse("W2", w2From, w2Till)
	if err != nil {
		return nil, err
	}
	w1, err := parse("W1", w1From, w1Till)
	if err != nil {
		return nil, err
	}
	return []window{w2, w1}, nil
}

func selectWindows(all []window, only string) ([]window, error) {
	switch strings.ToLower(only) {
	case "":
		return all, nil
	case "w2":
		return all[:1], nil
	case "w1":
		return all[1:], nil
	}
	return nil, fmt.Errorf("unknown -only %q: want w1 or w2", only)
}

func runOnce(ctx context.Context, p runParams, w window, source backtest.SignalSource) (outcome, error) {
	engine, err := backtest.NewEngine(backtest.Config{
		Tickers:         p.tickers,
		From:            w.From,
		Till:            w.Till,
		Deposit:         p.deposit,
		MaxLots:         1000,
		MaxNetShort:     p.cap,
		MaxNetLong:      p.cap,
		SectorCaps:      p.cfg.Risk.SectorCaps,
		Sectors:         p.cfg.Risk.Sectors,
		CommissionRate:  decimal.RequireFromString("0.0005"),
		SpreadPct:       decimal.RequireFromString("0.0005"),
		SlippagePct:     decimal.RequireFromString("0.0005"),
		BorrowPctPerDay: p.borrow,
		WarmupDays:      warmupDays,
		KillSwitch:      true,
		SignalSource:    source,
		Source:          p.source,
		FeatureConfig:   features.PriceFeatureConfig{},
	})
	if err != nil {
		return outcome{}, err
	}
	result, err := engine.Run(ctx)
	if err != nil {
		return outcome{}, err
	}
	realized, _ := result.RealizedPnlNetBorrow.Float64()
	return outcome{Realized: realized, Closed: result.ClosedTrades}, nil
}

func runPlacebos(ctx context.Context, p runParams, w window, table placebo.Table, kind string, n, workers int, seed int64) ([]float64, float64, error) {
	var variants []placebo.Table
	switch kind {
	case "shift":
		for _, k := range placebo.Offsets(table, 21, n) {
			variants = append(variants, placebo.Shift(table, k))
		}
	default:
		for i := 0; i < n; i++ {
			variants = append(variants, placebo.PermuteByDate(table, seed+int64(i)))
		}
	}
	results := make([]outcome, len(variants))
	errs := make([]error, len(variants))
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for i := range variants {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			results[i], errs[i] = runOnce(ctx, p, w, model.NewScoreTableSource(variants[i], 1000, p.notional))
		}(i)
	}
	wg.Wait()
	values := make([]float64, len(results))
	closed := 0.0
	for i, r := range results {
		if errs[i] != nil {
			return nil, 0, errs[i]
		}
		values[i] = r.Realized
		closed += float64(r.Closed)
	}
	if len(results) > 0 {
		closed /= float64(len(results))
	}
	return values, closed, nil
}

func quantile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := q * float64(len(sorted)-1)
	lo := int(math.Floor(idx))
	hi := int(math.Ceil(idx))
	frac := idx - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

func summarize(windowName, kind, borrow string, real outcome, placebos []float64, closedMean, secondsPerRun float64) cellReport {
	sorted := append([]float64(nil), placebos...)
	sort.Float64s(sorted)
	mean := 0.0
	for _, v := range placebos {
		mean += v
	}
	if len(placebos) > 0 {
		mean /= float64(len(placebos))
	}
	variance := 0.0
	for _, v := range placebos {
		variance += (v - mean) * (v - mean)
	}
	sd := 0.0
	if len(placebos) > 1 {
		sd = math.Sqrt(variance / float64(len(placebos)-1))
	}
	below := 0
	for _, v := range placebos {
		if v < real.Realized {
			below++
		}
	}
	percent := 0.0
	if len(placebos) > 0 {
		percent = float64(below) / float64(len(placebos))
	}
	return cellReport{
		Window:        windowName,
		Placebo:       kind,
		Borrow:        borrow,
		Real:          real.Realized,
		RealClosed:    real.Closed,
		N:             len(placebos),
		Mean:          mean,
		Sd:            sd,
		Q05:           quantile(sorted, 0.05),
		Q50:           quantile(sorted, 0.5),
		Q95:           quantile(sorted, 0.95),
		P:             placebo.PValue(real.Realized, placebos),
		MeanClosed:    closedMean,
		Placebos:      placebos,
		RealPercent:   percent,
		SecondsPerRun: secondsPerRun,
	}
}

func verdict(cells []cellReport) []string {
	find := func(kind, borrow string) (cellReport, bool) {
		for _, c := range cells {
			if c.Window == "W2" && c.Placebo == kind && c.Borrow == borrow {
				return c, true
			}
		}
		return cellReport{}, false
	}
	var borrows []string
	seen := map[string]bool{}
	for _, c := range cells {
		if !seen[c.Borrow] {
			seen[c.Borrow] = true
			borrows = append(borrows, c.Borrow)
		}
	}
	sort.Slice(borrows, func(i, j int) bool {
		a, _ := decimal.NewFromString(borrows[i])
		b, _ := decimal.NewFromString(borrows[j])
		return a.LessThan(b)
	})
	if len(borrows) == 0 {
		return []string{"no W2 cells: the criterion window did not run"}
	}
	hasW2 := false
	for _, c := range cells {
		if c.Window == "W2" {
			hasW2 = true
		}
	}
	if !hasW2 {
		return []string{"no W2 cells: the criterion window did not run"}
	}
	for _, c := range cells {
		if c.Window == "W2" && c.Real <= 0 {
			return []string{fmt.Sprintf("no skill to test: the real book lost money on W2 (%.0f RUB at borrow %s)", c.Real, c.Borrow)}
		}
	}
	high, low := borrows[len(borrows)-1], borrows[0]
	both := func(borrow string) bool {
		s, ok1 := find("shift", borrow)
		p, ok2 := find("permute", borrow)
		return ok1 && ok2 && s.P <= 0.05 && p.P <= 0.05
	}
	anyLow := func() bool {
		s, ok1 := find("shift", low)
		p, ok2 := find("permute", low)
		return (ok1 && s.P <= 0.05) || (ok2 && p.P <= 0.05)
	}
	switch {
	case both(high):
		return []string{"skill: p <= 0.05 in both placebos at borrow " + high}
	case both(low) || anyLow():
		return []string{"skill limited by the rate: p <= 0.05 at borrow " + low + " but not at " + high}
	default:
		return []string{"skill is indistinguishable from the book structure"}
	}
}

func markdown(r report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Placebo check\n\nSeed base: %d. Statistic: realized P&L net of borrow, RUB.\n\n", r.Seed)
	b.WriteString("| window | placebo | borrow/day | real | closed | n | mean | sd | q05 | q50 | q95 | p | mean closed | s/run |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, c := range r.Cells {
		fmt.Fprintf(&b, "| %s | %s | %s | %.0f | %d | %d | %.0f | %.0f | %.0f | %.0f | %.0f | %.4f | %.0f | %.1f |\n",
			c.Window, c.Placebo, c.Borrow, c.Real, c.RealClosed, c.N, c.Mean, c.Sd, c.Q05, c.Q50, c.Q95, c.P, c.MeanClosed, c.SecondsPerRun)
	}
	b.WriteString("\nVerdict (W2):\n\n")
	for _, v := range r.Verdict {
		fmt.Fprintf(&b, "- %s\n", v)
	}
	return b.String()
}
