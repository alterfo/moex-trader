package model

import (
	"context"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/features"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

var horizonAuditTickers = []string{
	"YDEX", "OZON", "SBER", "LKOH", "GAZP", "GMKN", "ROSN", "NVTK",
	"TATN", "MTSS", "MGNT", "PLZL", "CHMF", "DATA", "T", "VTBR", "RUAL", "POSI",
}

func isWeekend(t time.Time) bool {
	return t.Weekday() == time.Saturday || t.Weekday() == time.Sunday
}

// TestHorizonAudit measures the EFFECTIVE horizon that a bar-index horizon
// actually buys: for every label the deployed recipe would emit, how many
// distinct WEEKDAY sessions separate entry from exit. If MOEX publishes weekend
// bars for equities, candles[d+1+horizon] lands short of `horizon` real trading
// days, and by a different amount per ticker.
func TestHorizonAudit(t *testing.T) {
	if os.Getenv("MOEX_TRADER_HORIZON_AUDIT") == "" {
		t.Skip("set MOEX_TRADER_HORIZON_AUDIT=1 to measure effective label horizon")
	}
	horizonDays := 10
	if v := os.Getenv("MOEX_TRADER_HORIZON_AUDIT_HORIZON"); v != "" {
		fmt.Sscanf(v, "%d", &horizonDays)
	}
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	till := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	cfg, err := config.Load("../../config.yaml")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	source := backtest.NewISSSource(cfg.MOEXISSBaseURL, moex.NewClient(cfg.MOEXISSBaseURL, nil))
	var featureCfg features.PriceFeatureConfig
	warmup := featureCfg.WarmupCandles()
	fetchFrom := from.AddDate(0, 0, -backtest.DefaultWarmupDays)

	type row struct {
		ticker         string
		bars           int
		weekendBars    int
		samples        int
		sumWeekday     int
		sumCalDays     float64
		minCal, maxCal float64
		dist           map[int]int
		minW, maxW     int
	}
	var rows []row
	global := map[int]int{}

	for _, ticker := range horizonAuditTickers {
		candles, err := source.History(context.Background(), ticker, fetchFrom, till)
		if err != nil {
			t.Logf("%-6s ERROR %v", ticker, err)
			continue
		}
		r := row{ticker: ticker, bars: len(candles), dist: map[int]int{}, minW: 99, maxW: -1, minCal: 999, maxCal: -1}
		for _, c := range candles {
			if isWeekend(c.Begin) {
				r.weekendBars++
			}
		}
		for d := warmup; d < len(candles); d++ {
			if candles[d].Begin.Before(from) {
				continue
			}
			if d+1+horizonDays >= len(candles) {
				continue
			}
			entry := candles[d+1]
			exit := candles[d+1+horizonDays]
			weekdays := 0
			for _, c := range candles[d+1 : d+2+horizonDays] {
				if !isWeekend(c.Begin) {
					weekdays++
				}
			}
			_ = entry
			_ = exit
			cal := exit.Begin.Sub(entry.Begin).Hours() / 24
			r.samples++
			r.sumWeekday += weekdays
			r.sumCalDays += cal
			if cal < r.minCal {
				r.minCal = cal
			}
			if cal > r.maxCal {
				r.maxCal = cal
			}
			r.dist[weekdays]++
			global[weekdays]++
			if weekdays < r.minW {
				r.minW = weekdays
			}
			if weekdays > r.maxW {
				r.maxW = weekdays
			}
		}
		rows = append(rows, r)
	}

	t.Logf("window %s..%s horizonDays=%d warmup=%d", from.Format("2006-01-02"), till.Format("2006-01-02"), horizonDays, warmup)
	t.Logf("%-6s %6s %8s %8s %8s %8s %7s  %s", "ticker", "bars", "wkndBars", "wknd%", "samples", "meanWD", "range", "distribution")
	for _, r := range rows {
		if r.samples == 0 {
			continue
		}
		meanW := float64(r.sumWeekday) / float64(r.samples)
		wkndPct := 100 * float64(r.weekendBars) / float64(r.bars)
		keys := make([]int, 0, len(r.dist))
		for k := range r.dist {
			keys = append(keys, k)
		}
		sort.Ints(keys)
		dist := ""
		for _, k := range keys {
			dist += fmt.Sprintf("%d:%d ", k, r.dist[k])
		}
		meanCal := r.sumCalDays / float64(r.samples)
		t.Logf("%-6s %6d %8d %7.1f%% %8d %8.2f %3d-%-3d  calDays mean=%.2f range=%.0f-%.0f",
			r.ticker, r.bars, r.weekendBars, wkndPct, r.samples, meanW, r.minW, r.maxW, meanCal, r.minCal, r.maxCal)
	}

	t.Logf("--- pooled distribution of effective weekday horizon (all tickers) ---")
	keys := make([]int, 0, len(global))
	total := 0
	sumW := 0
	for k, v := range global {
		keys = append(keys, k)
		total += v
		sumW += k * v
	}
	sort.Ints(keys)
	for _, k := range keys {
		bar := ""
		n := global[k]
		for i := 0; i < n*60/total; i++ {
			bar += "#"
		}
		t.Logf("  %2d weekdays : %6d (%5.1f%%) %s", k, n, 100*float64(n)/float64(total), bar)
	}
	t.Logf("total labels = %d, mean effective horizon = %.2f weekdays (nominal %d)", total, float64(sumW)/float64(total), horizonDays)
}
