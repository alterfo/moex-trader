package model

import (
	"context"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/features"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

// TestHorizonModeLabelDiff measures, on the real buildSamples pipeline, how many
// labels change when the forward window moves from 10 bar-index steps (current
// deployed recipe) to 10 calendar days (HorizonModeCalendarDays).
func TestHorizonModeLabelDiff(t *testing.T) {
	if os.Getenv("MOEX_TRADER_HORIZON_AUDIT") == "" {
		t.Skip("set MOEX_TRADER_HORIZON_AUDIT=1")
	}
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	till := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	horizon := 10
	deadband := 0.5
	commission := 0.0

	cfg, err := config.Load("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	source := backtest.NewISSSource(cfg.MOEXISSBaseURL, moex.NewClient(cfg.MOEXISSBaseURL, nil))

	build := func(mode HorizonMode) []LabeledSample {
		s, err := BuildSamplesWithOptions(context.Background(), source, horizonAuditTickers, from, till, horizon, deadband, LabelModeAbsolute, commission, features.PriceFeatureConfig{}, mode)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	bars := build(HorizonModeBars)
	cal := build(HorizonModeCalendarDays)

	type key struct {
		ticker string
		day    string
	}
	toMap := func(s []LabeledSample) map[key]int {
		m := make(map[key]int, len(s))
		for _, x := range s {
			m[key{x.Feature.Ticker, x.Feature.GeneratedAt.Format("2006-01-02")}] = int(x.Label)
		}
		return m
	}
	bm := toMap(bars)
	cm := toMap(cal)

	perTicker := map[string][3]int{}
	var both, same, flip, onlyBars, onlyCal int
	for k, bv := range bm {
		cv, ok := cm[k]
		if !ok {
			onlyBars++
			a := perTicker[k.ticker]
			a[2]++
			perTicker[k.ticker] = a
			continue
		}
		both++
		if bv == cv {
			same++
		} else {
			flip++
			a := perTicker[k.ticker]
			a[1]++
			perTicker[k.ticker] = a
		}
	}
	for k := range cm {
		if _, ok := bm[k]; !ok {
			onlyCal++
		}
	}
	t.Logf("bars labels=%d calendar labels=%d both=%d agree=%d flip=%d onlyBars=%d onlyCal=%d",
		len(bars), len(cal), both, same, flip, onlyBars, onlyCal)
	if both > 0 {
		t.Logf("flip rate = %.2f%% of common labels", 100*float64(flip)/float64(both))
	}

	keys := make([]string, 0)
	seen := map[string]bool{}
	for k := range perTicker {
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	t.Logf("%-6s %8s %8s", "ticker", "flip", "onlyBars")
	for _, k := range keys {
		a := perTicker[k]
		t.Logf("%-6s %8d %8d", k, a[1], a[2])
	}
}
