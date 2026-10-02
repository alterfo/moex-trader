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
	"github.com/shopspring/decimal"
)

// TestLabelHorizonCompare asks the question that matters for label noise:
// for the SAME decision day, does the binary label change when the forward
// window is defined in weekday sessions (all tickers get exactly 10 weekday
// sessions) instead of raw bar index (all tickers get ~10.5 calendar days)?
func TestLabelHorizonCompare(t *testing.T) {
	if os.Getenv("MOEX_TRADER_HORIZON_AUDIT") == "" {
		t.Skip("set MOEX_TRADER_HORIZON_AUDIT=1")
	}
	horizonDays := 10
	deadbandPct := 0.5
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	till := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	cfg, err := config.Load("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	source := backtest.NewISSSource(cfg.MOEXISSBaseURL, moex.NewClient(cfg.MOEXISSBaseURL, nil))
	var featureCfg features.PriceFeatureConfig
	warmup := featureCfg.WarmupCandles()
	fetchFrom := from.AddDate(0, 0, -backtest.DefaultWarmupDays)

	labelOf := func(entry, exit decimal.Decimal) (int, bool) {
		if entry.Sign() <= 0 || exit.Sign() <= 0 {
			return 0, false
		}
		pct, _ := exit.Sub(entry).Div(entry).Mul(decimal.NewFromInt(100)).Float64()
		if pct > -deadbandPct && pct < deadbandPct {
			return 0, false
		}
		if pct > 0 {
			return 1, true
		}
		return 0, true
	}

	type agg struct{ samples, both, agree, flip, onlyRaw, onlyWD, sumBars int }
	tot := agg{}
	perTicker := map[string]*agg{}

	for _, ticker := range horizonAuditTickers {
		candles, err := source.History(context.Background(), ticker, fetchFrom, till)
		if err != nil {
			t.Logf("%s ERROR %v", ticker, err)
			continue
		}
		a := &agg{}
		for d := warmup; d < len(candles); d++ {
			if candles[d].Begin.Before(from) {
				continue
			}
			entryIdx := d + 1
			if entryIdx >= len(candles) {
				continue
			}
			entry := candles[entryIdx].Open
			if entry.Sign() <= 0 {
				continue
			}
			a.samples++

			rawLabel, rawOK := 0, false
			if d+1+horizonDays < len(candles) {
				rawLabel, rawOK = labelOf(entry, candles[d+1+horizonDays].Close)
			}

			wdCount := 0
			wdExit := -1
			for j := entryIdx; j < len(candles); j++ {
				if !isWeekend(candles[j].Begin) {
					wdCount++
					if wdCount == horizonDays {
						wdExit = j
						break
					}
				}
			}
			wdLabel, wdOK := 0, false
			if wdExit >= 0 {
				wdLabel, wdOK = labelOf(entry, candles[wdExit].Close)
			}
			if wdOK {
				a.sumBars += wdExit - entryIdx
			}

			switch {
			case rawOK && wdOK:
				a.both++
				if rawLabel == wdLabel {
					a.agree++
				} else {
					a.flip++
				}
			case rawOK && !wdOK:
				a.onlyRaw++
			case !rawOK && wdOK:
				a.onlyWD++
			}
		}
		perTicker[ticker] = a
		tot.samples += a.samples
		tot.both += a.both
		tot.agree += a.agree
		tot.flip += a.flip
		tot.onlyRaw += a.onlyRaw
		tot.onlyWD += a.onlyWD
		tot.sumBars += a.sumBars
	}

	t.Logf("window %s..%s horizon=%d deadband=%.1f%%", from.Format("2006-01-02"), till.Format("2006-01-02"), horizonDays, deadbandPct)
	t.Logf("%-6s %8s %8s %8s %8s %8s %8s", "ticker", "samples", "both", "agree%", "flip%", "onlyRaw", "onlyWD")
	keys := make([]string, 0, len(perTicker))
	for k := range perTicker {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		a := perTicker[k]
		ap := 0.0
		if a.both > 0 {
			ap = 100 * float64(a.agree) / float64(a.both)
		}
		t.Logf("%-6s %8d %8d %7.1f%% %7.1f%% %8d %8d", k, a.samples, a.both, ap, 100-ap, a.onlyRaw, a.onlyWD)
	}
	if tot.both > 0 {
		t.Logf("POOLED samples=%d both=%d agree=%.1f%% flip=%.1f%% onlyRaw=%d onlyWD=%d meanBarsFor10WD=%.2f",
			tot.samples, tot.both, 100*float64(tot.agree)/float64(tot.both), 100*float64(tot.flip)/float64(tot.both), tot.onlyRaw, tot.onlyWD, float64(tot.sumBars)/float64(tot.both))
	}
}
