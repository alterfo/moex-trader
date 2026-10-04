package model

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/dividends"
	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/features"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

const minFeatureCandles = 64

const benchmarkTicker = "IMOEX"

const dateKeyLayout = "2006-01-02"

type LabeledSample struct {
	Feature   domain.FeatureContext
	Label     float64
	LabelDate time.Time
}

type LabelMode string

const (
	LabelModeExcess     LabelMode = "excess"
	LabelModeAbsolute   LabelMode = "absolute"
	LabelModeAbsoluteTR LabelMode = "absolute_tr"
)

// HorizonMode selects how the forward-return window is measured. HorizonModeBars
// (the default) advances a fixed number of candle indices, which is the only
// sensible unit on intraday bars. HorizonModeCalendarDays advances to the candle
// nearest entry+horizonDays in wall-clock time - necessary on daily equity bars
// because MOEX runs weekend sessions, so a fixed bar count spans a different
// number of calendar days per ticker.
type HorizonMode string

const (
	HorizonModeBars         HorizonMode = ""
	HorizonModeCalendarDays HorizonMode = "calendar_days"
)

// forwardExitIndex returns the index of the candle that closes a forward window
// opened at entryIdx. HorizonModeBars returns entryIdx+horizonDays (the
// historical behavior). HorizonModeCalendarDays returns the candle whose Begin
// is nearest entry.Begin+horizonDays, so every ticker measures the same
// wall-clock window even when weekend sessions make bar counts diverge. Returns
// -1 when the window reaches past the end of the series.
func forwardExitIndex(candles []moex.Candle, entryIdx, horizonDays int, mode HorizonMode) int {
	if entryIdx < 0 || entryIdx >= len(candles) {
		return -1
	}
	if mode != HorizonModeCalendarDays {
		exitIdx := entryIdx + horizonDays
		if exitIdx >= len(candles) {
			return -1
		}
		return exitIdx
	}
	target := candles[entryIdx].Begin.AddDate(0, 0, horizonDays)
	rel := sort.Search(len(candles)-entryIdx, func(j int) bool {
		return !candles[entryIdx+j].Begin.Before(target)
	})
	after := entryIdx + rel
	if after >= len(candles) || after <= entryIdx {
		return -1
	}
	before := after - 1
	if before <= entryIdx {
		return after
	}
	if target.Sub(candles[before].Begin) < candles[after].Begin.Sub(target) {
		return before
	}
	return after
}

// BuildSamples labels each decision day/bar by the sign of its forward
// return (see LabelMode). commissionPct is the one-way commission rate (e.g.
// 0.0005 for 0.05%); pass 0 to disable cost-adjustment and keep prior
// behavior exactly. When positive, a sample's dead zone is widened by the
// round-trip commission (2x) plus that bar's own (High-Low)/Close spread
// proxy, so labels aren't assigned to moves too small to trade profitably -
// important on short (intraday) horizons where typical moves can be
// comparable to round-trip costs; on multi-day horizons this cost floor is
// negligible next to deadbandPct and changes nothing in practice.
func BuildSamples(ctx context.Context, source backtest.HistoricalSource, tickers []string, from, till time.Time, horizonDays int, deadbandPct float64, mode LabelMode, commissionPct float64) ([]LabeledSample, error) {
	return buildSamples(ctx, source, tickers, from, till, horizonDays, deadbandPct, mode, commissionPct, features.PriceFeatureConfig{}, HorizonModeBars, nil)
}

// BuildSamplesWithFeatureConfig behaves exactly like BuildSamples but scales
// the day-denominated feature windows to the candle interval in use (see
// features.PriceFeatureConfig) - needed for intraday sources where a bar is
// minutes, not a day. The zero config is identical to BuildSamples.
func BuildSamplesWithFeatureConfig(ctx context.Context, source backtest.HistoricalSource, tickers []string, from, till time.Time, horizonDays int, deadbandPct float64, mode LabelMode, commissionPct float64, featureCfg features.PriceFeatureConfig) ([]LabeledSample, error) {
	return buildSamples(ctx, source, tickers, from, till, horizonDays, deadbandPct, mode, commissionPct, featureCfg, HorizonModeBars, nil)
}

// BuildSamplesWithOptions is BuildSamplesWithFeatureConfig plus an explicit
// horizon unit. horizonMode "" is identical to BuildSamplesWithFeatureConfig.
func BuildSamplesWithOptions(ctx context.Context, source backtest.HistoricalSource, tickers []string, from, till time.Time, horizonDays int, deadbandPct float64, mode LabelMode, commissionPct float64, featureCfg features.PriceFeatureConfig, horizonMode HorizonMode) ([]LabeledSample, error) {
	return buildSamples(ctx, source, tickers, from, till, horizonDays, deadbandPct, mode, commissionPct, featureCfg, horizonMode, nil)
}

// BuildSamplesWithDividends is BuildSamplesWithOptions plus a dividend
// calendar used by LabelModeAbsoluteTR: for every ex-date (the first trading
// session after last_buy_date) inside (entry, exit], the net dividend is added
// to the exit price before the forward return is computed.
func BuildSamplesWithDividends(ctx context.Context, source backtest.HistoricalSource, tickers []string, from, till time.Time, horizonDays int, deadbandPct float64, mode LabelMode, commissionPct float64, featureCfg features.PriceFeatureConfig, horizonMode HorizonMode, divs []dividends.Record) ([]LabeledSample, error) {
	return buildSamples(ctx, source, tickers, from, till, horizonDays, deadbandPct, mode, commissionPct, featureCfg, horizonMode, divs)
}

func buildSamples(ctx context.Context, source backtest.HistoricalSource, tickers []string, from, till time.Time, horizonDays int, deadbandPct float64, mode LabelMode, commissionPct float64, featureCfg features.PriceFeatureConfig, horizonMode HorizonMode, divs []dividends.Record) ([]LabeledSample, error) {
	if source == nil {
		return nil, fmt.Errorf("model: historical source is required")
	}
	if horizonDays <= 0 {
		return nil, fmt.Errorf("model: horizon days must be positive, got %d", horizonDays)
	}
	if deadbandPct < 0 {
		return nil, fmt.Errorf("model: deadband pct must be non-negative, got %v", deadbandPct)
	}
	if commissionPct < 0 {
		return nil, fmt.Errorf("model: commission pct must be non-negative, got %v", commissionPct)
	}
	if mode == "" {
		mode = LabelModeExcess
	}
	if mode != LabelModeExcess && mode != LabelModeAbsolute && mode != LabelModeAbsoluteTR {
		return nil, fmt.Errorf("model: unknown label mode %q", mode)
	}
	if mode == LabelModeAbsoluteTR && divs == nil {
		return nil, fmt.Errorf("model: dividend calendar is required for absolute_tr label mode")
	}
	normalized := normalizeTickers(tickers)
	if len(normalized) == 0 {
		return nil, fmt.Errorf("model: tickers must not be empty")
	}
	if till.IsZero() {
		till = time.Now()
	}
	if from.IsZero() {
		from = till.AddDate(0, 0, -365)
	}
	fetchFrom := from.AddDate(0, 0, -backtest.DefaultWarmupDays)
	if extra := featureCfg.FetchCalendarDays(); extra > backtest.DefaultWarmupDays {
		fetchFrom = from.AddDate(0, 0, -extra)
	}

	var indexByDate map[string]moex.Candle
	if mode == LabelModeExcess {
		indexCandles, err := source.History(ctx, benchmarkTicker, fetchFrom, till)
		if err != nil {
			return nil, fmt.Errorf("model: history %s: %w", benchmarkTicker, err)
		}
		if len(indexCandles) == 0 {
			return nil, fmt.Errorf("model: benchmark %s has no history in the requested window", benchmarkTicker)
		}
		indexByDate = indexCandlesByDate(indexCandles)
	}

	builder := features.NewBuilderWithConfig(time.Now, featureCfg)
	var samples []LabeledSample
	for _, ticker := range normalized {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		candles, err := source.History(ctx, ticker, fetchFrom, till)
		if err != nil {
			return nil, fmt.Errorf("model: history %s: %w", ticker, err)
		}
		warmup := featureCfg.WarmupCandles()
		if len(candles) < warmup+1 {
			continue
		}
		divCalendar, err := buildExDividendCalendar(ticker, candles, divs)
		if err != nil {
			return nil, err
		}
		for d := warmup; d < len(candles); d++ {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			decisionDay := candles[d].Begin
			if decisionDay.Before(from) {
				continue
			}
			exitIdx := forwardExitIndex(candles, d+1, horizonDays, horizonMode)
			if exitIdx < 0 {
				continue
			}
			entryCandle := candles[d+1]
			entry := entryCandle.Open
			if entry.Sign() <= 0 {
				continue
			}
			exitCandle := candles[exitIdx]
			exit := exitCandle.Close
			if exit.Sign() <= 0 {
				continue
			}
			if mode == LabelModeAbsoluteTR {
				exit = exit.Add(dividendAdjustment(divCalendar, entryCandle.Begin, exitCandle.Begin))
			}

			indexEntry, ok := indexByDate[dateKey(entryCandle.Begin)]
			if mode == LabelModeExcess && (!ok || indexEntry.Open.Sign() <= 0) {
				continue
			}
			indexExit, ok := indexByDate[dateKey(exitCandle.Begin)]
			if mode == LabelModeExcess && (!ok || indexExit.Close.Sign() <= 0) {
				continue
			}

			forwardReturn := exit.Sub(entry).Div(entry).Mul(decimal.NewFromInt(100))
			labelReturn := forwardReturn
			if mode == LabelModeExcess {
				indexForwardReturn := indexExit.Close.Sub(indexEntry.Open).Div(indexEntry.Open).Mul(decimal.NewFromInt(100))
				labelReturn = forwardReturn.Sub(indexForwardReturn)
			}
			labelPct, _ := labelReturn.Float64()
			threshold := deadbandPct
			if commissionPct > 0 {
				spreadPct := 0.0
				if entryCandle.Close.Sign() > 0 {
					spreadPct, _ = entryCandle.High.Sub(entryCandle.Low).Div(entryCandle.Close).Mul(decimal.NewFromInt(100)).Float64()
				}
				threshold += 2*commissionPct*100 + spreadPct
			}
			if math.Abs(labelPct) < threshold {
				continue
			}
			label := 0.0
			if labelPct > 0 {
				label = 1
			}

			input := features.Input{
				Ticker: ticker,
				Price: features.PriceSnapshot{
					LastPrice: candles[d-1].Close,
					PrevClose: candles[d-2].Close,
					AsOf:      decisionDay,
				},
				Candles: candles[:d],
			}
			feature, err := builder.Build(input)
			if err != nil {
				continue
			}
			samples = append(samples, LabeledSample{Feature: feature, Label: label, LabelDate: exitCandle.Begin})
		}
	}
	return samples, nil
}

type exDividend struct {
	exDate time.Time
	net    decimal.Decimal
}

// buildExDividendCalendar maps each dividend event to its ex-date (the first
// trading session strictly after last_buy_date) for one ticker. Events whose
// ex-date falls outside the available candle series are dropped, and multiple
// events sharing an ex-date are summed.
func buildExDividendCalendar(ticker string, candles []moex.Candle, records []dividends.Record) ([]exDividend, error) {
	byExDate := make(map[time.Time]decimal.Decimal)
	for _, rec := range records {
		if !strings.EqualFold(strings.TrimSpace(rec.Ticker), ticker) {
			continue
		}
		net, err := rec.Net()
		if err != nil {
			return nil, fmt.Errorf("model: dividend %s: invalid dividend_net %q: %w", ticker, rec.DividendNet, err)
		}
		idx := sort.Search(len(candles), func(i int) bool {
			return candles[i].Begin.After(rec.LastBuyDate)
		})
		if idx >= len(candles) {
			continue
		}
		byExDate[candles[idx].Begin] = byExDate[candles[idx].Begin].Add(net)
	}
	if len(byExDate) == 0 {
		return nil, nil
	}
	out := make([]exDividend, 0, len(byExDate))
	for exDate, net := range byExDate {
		out = append(out, exDividend{exDate: exDate, net: net})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].exDate.Before(out[j].exDate) })
	return out, nil
}

func dividendAdjustment(calendar []exDividend, entry, exit time.Time) decimal.Decimal {
	total := decimal.Zero
	for _, ev := range calendar {
		if ev.exDate.After(entry) && !ev.exDate.After(exit) {
			total = total.Add(ev.net)
		}
	}
	return total
}

func indexCandlesByDate(candles []moex.Candle) map[string]moex.Candle {
	byDate := make(map[string]moex.Candle, len(candles))
	for _, c := range candles {
		byDate[dateKey(c.Begin)] = c
	}
	return byDate
}

func dateKey(t time.Time) string {
	return t.Format(dateKeyLayout)
}

func normalizeTickers(tickers []string) []string {
	out := make([]string, 0, len(tickers))
	for _, ticker := range tickers {
		ticker = strings.ToUpper(strings.TrimSpace(ticker))
		if ticker != "" {
			out = append(out, ticker)
		}
	}
	return out
}
